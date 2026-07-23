// Package explain implements the reasoning engine that explains ResourceClaim allocation statuses.
// SPDX-License-Identifier: Apache-2.0
package explain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/redaction"
	"github.com/oaslananka/draforge/pkg/model"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const claimRequestsFieldPath = ".spec.devices.requests"

// evaluateCEL parses and evaluates simple CEL expressions against device attributes and capacities.
func evaluateCEL(expression string, attributes map[string]string, capacities map[string]int64) (bool, error) {
	if strings.TrimSpace(expression) == "" {
		return true, nil
	}

	env, err := cel.NewEnv(
		cel.Variable("device", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, iss := env.Compile(expression)
	if iss.Err() != nil {
		return false, fmt.Errorf("CEL compilation error: %w", iss.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("CEL program creation error: %w", err)
	}

	// Prepare data
	deviceMap := map[string]any{
		"attributes": attributes,
		"capacity":   capacities,
	}

	out, _, err := prg.Eval(map[string]any{
		"device": deviceMap,
	})
	if err != nil {
		return false, fmt.Errorf("CEL evaluation error: %w", err)
	}

	if outValue, ok := out.Value().(bool); ok {
		return outValue, nil
	}

	return false, fmt.Errorf("CEL expression did not evaluate to a boolean")
}

func formatAllocations(allocations []model.ClaimAllocation) string {
	if len(allocations) == 0 {
		return "none"
	}
	formatted := make([]string, 0, len(allocations))
	for _, allocation := range allocations {
		request := allocation.Request
		if request == "" {
			request = "unknown-request"
		}
		node := allocation.NodeName
		if node == "" {
			node = "unknown-node"
		}
		formatted = append(formatted, fmt.Sprintf("%s: %s/%s/%s on %s", request, allocation.DriverName, allocation.PoolName, allocation.DeviceName, node))
	}
	return strings.Join(formatted, "; ")
}

func matchesAnyRequestedClass(device model.Device, classes []*resourcev1.DeviceClass, noClassRequested bool) (bool, error) {
	if noClassRequested {
		return true, nil
	}
	var lastError error
	for _, deviceClass := range classes {
		matches, err := matchesDeviceClass(device, deviceClass)
		if err != nil {
			lastError = err
			continue
		}
		if matches {
			return true, nil
		}
	}
	return false, lastError
}

func matchesDeviceClass(device model.Device, deviceClass *resourcev1.DeviceClass) (bool, error) {
	for _, selector := range deviceClass.Spec.Selectors {
		if selector.CEL == nil {
			continue
		}
		passed, err := evaluateCEL(selector.CEL.Expression, device.Attributes, device.Capacities)
		if err != nil || !passed {
			return false, err
		}
	}
	return true, nil
}

func allocationMatchesDevice(allocation model.ClaimAllocation, device model.Device) bool {
	if allocation.DeviceName != device.Name {
		return false
	}
	if allocation.DriverName != "" && allocation.DriverName != device.DriverName {
		return false
	}
	if allocation.PoolName != "" && allocation.PoolName != device.PoolName {
		return false
	}
	return allocation.NodeName == "" || allocation.NodeName == device.NodeName
}

// ExplainClaim analyzes a ResourceClaim and returns an explanation tree.
func ExplainClaim(ctx context.Context, clientset kubernetes.Interface, namespace, claimName string) (*model.ExplainResult, error) {
	_, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to discover cluster state: %w", err)
	}
	liveClaim, err := clientset.ResourceV1().ResourceClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ResourceClaim %s/%s: %w", namespace, claimName, err)
	}
	target := findClaim(claims, namespace, claimName)
	if target == nil {
		return nil, fmt.Errorf("ResourceClaim %s/%s not found", namespace, claimName)
	}

	result := &model.ExplainResult{
		TargetName: claimName,
		TargetType: "claim",
		Allocated:  target.Status == "Allocated",
		Remedy:     []string{},
	}
	if result.Allocated {
		result.ReasonTree = allocatedReason(*target, liveClaim)
		return result, nil
	}
	result.ReasonTree = pendingReason(ctx, clientset, *target, liveClaim, devices, claims, result)
	return result, nil
}

func findClaim(claims []model.ResourceClaimInfo, namespace, claimName string) *model.ResourceClaimInfo {
	for index := range claims {
		if claims[index].Name == claimName && claims[index].Namespace == namespace {
			return &claims[index]
		}
	}
	return nil
}

func allocatedReason(target model.ResourceClaimInfo, liveClaim *resourcev1.ResourceClaim) model.ReasonNode {
	return model.ReasonNode{
		Message:    "ResourceClaim successfully allocated.",
		Confidence: "confirmed",
		Evidence: fmt.Sprintf(
			"Claim status is Allocated. Allocations: %s. Reserved for consumers: %s",
			formatAllocations(target.EffectiveAllocations()),
			formatConsumers(liveClaim.Status.ReservedFor),
		),
		SourceType: "ResourceClaim",
		FieldPath:  ".status.allocation.devices.results",
	}
}

func pendingReason(ctx context.Context, clientset kubernetes.Interface, target model.ResourceClaimInfo, liveClaim *resourcev1.ResourceClaim, devices []model.Device, claims []model.ResourceClaimInfo, result *model.ExplainResult) model.ReasonNode {
	root := model.ReasonNode{
		Message:    "Claim could not be allocated.",
		Confidence: "confirmed",
		Evidence:   "Claim status is Pending.",
		SourceType: "ResourceClaim",
		FieldPath:  ".status.allocation",
	}
	appendAdvancedFeatureReason(&root, liveClaim)
	appendNodeSelectorReason(&root, liveClaim)
	requestedClasses := resolveRequestedClasses(ctx, clientset, target, &root, result)
	stats := evaluateCandidates(devices, claims, requestedClasses, len(target.RequestedClassNames()) == 0)
	appendCandidateReason(&root, result, len(devices), stats)
	appendEventReasons(ctx, clientset, liveClaim.Namespace, liveClaim.Name, &root)
	appendConsumerReason(&root, result, liveClaim.Status.ReservedFor)
	return root
}

func appendAdvancedFeatureReason(root *model.ReasonNode, claim *resourcev1.ResourceClaim) {
	if !usesAdvancedFeatures(claim) {
		return
	}
	root.Children = append(root.Children, model.ReasonNode{
		Message:    "Claim uses advanced v1.36 features (e.g. Tolerations, Capacity) which are only partially modeled.",
		Confidence: "informational",
		SourceType: "ResourceClaim",
		FieldPath:  claimRequestsFieldPath,
	})
}

func usesAdvancedFeatures(claim *resourcev1.ResourceClaim) bool {
	for _, request := range claim.Spec.Devices.Requests {
		if request.Exactly != nil && (len(request.Exactly.Tolerations) > 0 || request.Exactly.Capacity != nil) {
			return true
		}
		for _, alternative := range request.FirstAvailable {
			if len(alternative.Tolerations) > 0 || alternative.Capacity != nil {
				return true
			}
		}
	}
	return false
}

func appendNodeSelectorReason(root *model.ReasonNode, claim *resourcev1.ResourceClaim) {
	if claim.Status.Allocation == nil || claim.Status.Allocation.NodeSelector == nil {
		return
	}
	root.Children = append(root.Children, model.ReasonNode{
		Message:    "Node selector computed, pending final allocation",
		Confidence: "confirmed",
		Evidence:   "Claim has an active node selector indicating preliminary scheduling.",
		SourceType: "ResourceClaim",
		FieldPath:  ".status.allocation.nodeSelector",
	})
}

func resolveRequestedClasses(ctx context.Context, clientset kubernetes.Interface, target model.ResourceClaimInfo, root *model.ReasonNode, result *model.ExplainResult) []*resourcev1.DeviceClass {
	classNames := target.RequestedClassNames()
	if len(classNames) > 0 {
		root.Children = append(root.Children, model.ReasonNode{
			Message:    fmt.Sprintf("Claim requests DeviceClasses: %s", strings.Join(classNames, ", ")),
			Confidence: "confirmed",
			Evidence:   fmt.Sprintf("Preserved %d request alternatives from %s.", len(classNames), claimRequestsFieldPath),
			SourceType: "ResourceClaim",
			FieldPath:  claimRequestsFieldPath,
		})
	}

	available := listDeviceClasses(ctx, clientset)
	requested := make([]*resourcev1.DeviceClass, 0, len(classNames))
	for _, className := range classNames {
		if deviceClass, exists := available[className]; exists {
			requested = append(requested, deviceClass)
			continue
		}
		appendMissingClassReason(root, result, className)
	}
	return requested
}

func listDeviceClasses(ctx context.Context, clientset kubernetes.Interface) map[string]*resourcev1.DeviceClass {
	available := make(map[string]*resourcev1.DeviceClass)
	classList, err := clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return available
	}
	for index := range classList.Items {
		deviceClass := &classList.Items[index]
		available[deviceClass.Name] = deviceClass
	}
	return available
}

func appendMissingClassReason(root *model.ReasonNode, result *model.ExplainResult, className string) {
	root.Children = append(root.Children, model.ReasonNode{
		Message:    fmt.Sprintf("Requested DeviceClass '%s' does not exist in the cluster.", className),
		Confidence: "confirmed",
		Evidence:   "DeviceClass list query returned no match for this request alternative.",
		SourceType: "DeviceClass",
		FieldPath:  claimRequestsFieldPath,
	})
	result.Remedy = append(result.Remedy, fmt.Sprintf("Create the missing DeviceClass '%s' in the cluster.", className))
}

type candidateStats struct {
	selectorPassed    int
	capacityMatched   int
	unhealthy         int
	selectorErrors    int
	lastSelectorError error
}

func evaluateCandidates(devices []model.Device, claims []model.ResourceClaimInfo, requestedClasses []*resourcev1.DeviceClass, noClassRequested bool) candidateStats {
	var stats candidateStats
	for _, device := range devices {
		passed, selectorErr := matchesAnyRequestedClass(device, requestedClasses, noClassRequested)
		if selectorErr != nil {
			stats.selectorErrors++
			stats.lastSelectorError = selectorErr
		}
		if !passed {
			continue
		}
		stats.selectorPassed++
		if device.Status != "" && device.Status != "healthy" {
			stats.unhealthy++
			continue
		}
		if deviceAllocated(device, claims) {
			continue
		}
		stats.capacityMatched++
	}
	return stats
}

func deviceAllocated(device model.Device, claims []model.ResourceClaimInfo) bool {
	for _, claim := range claims {
		if claim.Status != "Allocated" {
			continue
		}
		for _, allocation := range claim.EffectiveAllocations() {
			if allocationMatchesDevice(allocation, device) {
				return true
			}
		}
	}
	return false
}

func appendCandidateReason(root *model.ReasonNode, result *model.ExplainResult, totalDevices int, stats candidateStats) {
	if totalDevices == 0 {
		root.Children = append(root.Children, model.ReasonNode{
			Message:    "No devices discovered in the cluster.",
			Confidence: "confirmed",
			Evidence:   "ResourceSlice query returned 0 devices.",
			SourceType: "ResourceSlice",
		})
		result.Remedy = append(result.Remedy, "Register a DRA driver or deploy a SimulatedDevicePool scenario.")
		return
	}

	summary := candidateSummary(totalDevices, stats)
	root.Children = append(root.Children, summary)
	if stats.unhealthy > 0 {
		result.Remedy = append(result.Remedy, "Inspect and resolve health/degradation states for the unhealthy simulated devices.")
	}
	if stats.capacityMatched == 0 && stats.unhealthy == 0 {
		result.Remedy = append(result.Remedy, "Release existing allocations or increase device counts in the pool.")
	}
}

func candidateSummary(totalDevices int, stats candidateStats) model.ReasonNode {
	summary := model.ReasonNode{
		Message:    fmt.Sprintf("%d candidate devices evaluated", totalDevices),
		Confidence: "inferred",
		Evidence:   fmt.Sprintf("Evaluated %d devices: %d rejected due to selector mismatch.", totalDevices, totalDevices-stats.selectorPassed),
		SourceType: "ResourceSlice",
		Children:   []model.ReasonNode{},
	}
	falseSelectors := totalDevices - stats.selectorPassed - stats.selectorErrors
	if falseSelectors > 0 {
		summary.Children = append(summary.Children, model.ReasonNode{
			Message: fmt.Sprintf("%d rejected because selector evaluated to false", falseSelectors), Confidence: "confirmed", SourceType: "DeviceClass",
		})
	}
	if stats.unhealthy > 0 {
		summary.Children = append(summary.Children, model.ReasonNode{
			Message: fmt.Sprintf("%d rejected because device health status was unhealthy or degraded", stats.unhealthy), Confidence: "confirmed", SourceType: "ResourceSlice",
		})
	}
	unavailable := stats.selectorPassed - stats.unhealthy - stats.capacityMatched
	if unavailable > 0 {
		summary.Children = append(summary.Children, model.ReasonNode{
			Message: fmt.Sprintf("%d rejected because requested capacity (already allocated) was unavailable", unavailable), Confidence: "inferred", SourceType: "ResourceSlice",
		})
	}
	if stats.selectorErrors > 0 {
		summary.Children = append(summary.Children, model.ReasonNode{
			Message:    fmt.Sprintf("%d rejected because selector expression failed or was unsupported", stats.selectorErrors),
			Confidence: "confirmed",
			Evidence:   stats.lastSelectorError.Error(),
			SourceType: "DeviceClass",
		})
	}
	return summary
}

func appendEventReasons(ctx context.Context, clientset kubernetes.Interface, namespace, claimName string, root *model.ReasonNode) {
	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, event := range events.Items {
		if event.InvolvedObject.Name != claimName || event.InvolvedObject.Kind != "ResourceClaim" {
			continue
		}
		root.Children = append(root.Children, model.ReasonNode{
			Message:    fmt.Sprintf("Cluster Event: %s", event.Reason),
			Confidence: "probable",
			Evidence:   fmt.Sprintf("[%s] %s", event.Type, redaction.RedactString(event.Message)),
			SourceType: "Event",
		})
	}
}

func appendConsumerReason(root *model.ReasonNode, result *model.ExplainResult, references []resourcev1.ResourceClaimConsumerReference) {
	if len(references) == 0 {
		root.Children = append(root.Children, model.ReasonNode{
			Message:    "Claim uses delayed allocation, waiting for first consumer pod.",
			Confidence: "confirmed",
			Evidence:   "status.reservedFor is empty, no active pod has claimed this resource.",
			SourceType: "ResourceClaim",
			FieldPath:  ".status.reservedFor",
		})
		result.Remedy = append(result.Remedy, "Deploy a Pod referencing this ResourceClaim to trigger allocation.")
		return
	}
	root.Children = append(root.Children, model.ReasonNode{
		Message:    "Claim has active consumers but allocation is pending.",
		Confidence: "confirmed",
		Evidence:   fmt.Sprintf("Reserved for consumers: %s", formatConsumers(references)),
		SourceType: "ResourceClaim",
		FieldPath:  ".status.reservedFor",
	})
}

func formatConsumers(references []resourcev1.ResourceClaimConsumerReference) string {
	if len(references) == 0 {
		return "none"
	}
	consumers := make([]string, 0, len(references))
	for _, reference := range references {
		consumers = append(consumers, reference.Name)
	}
	return strings.Join(consumers, ", ")
}
