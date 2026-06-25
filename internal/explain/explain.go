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

// ExplainClaim analyzes a ResourceClaim and returns an explanation tree.
func ExplainClaim(ctx context.Context, clientset kubernetes.Interface, namespace, claimName string) (*model.ExplainResult, error) {
	// 1. Fetch live DRA resources
	_, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to discover cluster state: %w", err)
	}

	// 2. Fetch actual ResourceClaim from API to check ReservedFor status
	liveClaim, err := clientset.ResourceV1().ResourceClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ResourceClaim %s/%s: %w", namespace, claimName, err)
	}

	var target *model.ResourceClaimInfo
	for _, c := range claims {
		if c.Name == claimName && c.Namespace == namespace {
			target = &c
			break
		}
	}

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
		var consumers []string
		for _, ref := range liveClaim.Status.ReservedFor {
			consumers = append(consumers, ref.Name)
		}
		consumerList := "none"
		if len(consumers) > 0 {
			consumerList = strings.Join(consumers, ", ")
		}

		result.ReasonTree = model.ReasonNode{
			Message:    "ResourceClaim successfully allocated.",
			Confidence: "confirmed",
			Evidence:   fmt.Sprintf("Claim status is Allocated. Bound to device %s on node %s. Reserved for consumers: %s", target.AllocatedDevice, target.AllocatedNode, consumerList),
			SourceType: "ResourceClaim",
			FieldPath:  ".status.allocation",
		}
		return result, nil
	}

	// Explain Pending Claim
	rootNode := model.ReasonNode{
		Message:    "Claim could not be allocated.",
		Confidence: "confirmed",
		Evidence:   "Claim status is Pending.",
		SourceType: "ResourceClaim",
		FieldPath:  ".status.allocation",
	}

	// Check for advanced features in the claim
	hasAdvancedFeatures := false
	for _, req := range liveClaim.Spec.Devices.Requests {
		if req.Exactly != nil {
			if len(req.Exactly.Tolerations) > 0 || req.Exactly.Capacity != nil {
				hasAdvancedFeatures = true
			}
		}
		for _, subReq := range req.FirstAvailable {
			if len(subReq.Tolerations) > 0 || subReq.Capacity != nil {
				hasAdvancedFeatures = true
			}
		}
	}
	if hasAdvancedFeatures {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "Claim uses advanced v1.36 features (e.g. Tolerations, Capacity) which are only partially modeled.",
			Confidence: "informational",
			SourceType: "ResourceClaim",
			FieldPath:  ".spec.devices.requests",
		})
	}

	if liveClaim.Status.Allocation != nil && liveClaim.Status.Allocation.NodeSelector != nil {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "Node selector computed, pending final allocation",
			Confidence: "confirmed",
			Evidence:   "Claim has an active node selector indicating preliminary scheduling.",
			SourceType: "ResourceClaim",
			FieldPath:  ".status.allocation.nodeSelector",
		})
	}

	// Check DeviceClass existence
	classExists := false
	classNames, err := clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	var deviceClass *resourcev1.DeviceClass
	if err == nil {
		for _, cls := range classNames.Items {
			if cls.Name == target.DeviceClassName {
				classExists = true
				deviceClass = &cls
				break
			}
		}
	}

	if !classExists {
		child := model.ReasonNode{
			Message:    fmt.Sprintf("Requested DeviceClass '%s' does not exist in the cluster.", target.DeviceClassName),
			Confidence: "confirmed",
			Evidence:   "DeviceClass list query returned no matches.",
			SourceType: "DeviceClass",
			FieldPath:  ".spec.deviceClassName",
		}
		rootNode.Children = append(rootNode.Children, child)
		result.Remedy = append(result.Remedy, fmt.Sprintf("Create the missing DeviceClass '%s' in the cluster.", target.DeviceClassName))
	}

	// Evaluate candidate devices & drivers matching selectors
	selectorPassed := 0
	capacityMatched := 0
	unhealthyCount := 0
	selectorFailedErrorCount := 0
	var lastSelectorError error

	for _, d := range devices {
		// Evaluated against selectors in DeviceClass
		passedSelector := true
		if deviceClass != nil {
			for _, sel := range deviceClass.Spec.Selectors {
				if sel.CEL != nil {
					passed, evalErr := evaluateCEL(sel.CEL.Expression, d.Attributes, d.Capacities)
					if evalErr != nil {
						selectorFailedErrorCount++
						lastSelectorError = evalErr
						passedSelector = false
						break
					}
					if !passed {
						passedSelector = false
						break
					}
				}
			}
		}

		if !passedSelector {
			continue
		}
		selectorPassed++

		// Health Check
		if d.Status != "" && d.Status != "healthy" {
			unhealthyCount++
			continue
		}

		// Capacity / Availability check (check if already allocated)
		isAllocated := false
		for _, c := range claims {
			if c.Status == "Allocated" && c.AllocatedDevice == d.Name && c.AllocatedNode == d.NodeName {
				isAllocated = true
				break
			}
		}

		if isAllocated {
			continue
		}
		capacityMatched++
	}

	totalDevices := len(devices)
	if totalDevices == 0 {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "No devices discovered in the cluster.",
			Confidence: "confirmed",
			Evidence:   "ResourceSlice query returned 0 devices.",
			SourceType: "ResourceSlice",
		})
		result.Remedy = append(result.Remedy, "Register a DRA driver or deploy a SimulatedDevicePool scenario.")
	} else {
		summaryNode := model.ReasonNode{
			Message:    fmt.Sprintf("%d candidate devices evaluated", totalDevices),
			Confidence: "inferred",
			Evidence:   fmt.Sprintf("Evaluated %d devices: %d rejected due to selector mismatch.", totalDevices, totalDevices-selectorPassed),
			SourceType: "ResourceSlice",
			Children:   []model.ReasonNode{},
		}

		falseSelectorCount := totalDevices - selectorPassed - selectorFailedErrorCount
		if falseSelectorCount > 0 {
			summaryNode.Children = append(summaryNode.Children, model.ReasonNode{
				Message:    fmt.Sprintf("%d rejected because selector evaluated to false", falseSelectorCount),
				Confidence: "confirmed",
				SourceType: "DeviceClass",
			})
		}

		if unhealthyCount > 0 {
			summaryNode.Children = append(summaryNode.Children, model.ReasonNode{
				Message:    fmt.Sprintf("%d rejected because device health status was unhealthy or degraded", unhealthyCount),
				Confidence: "confirmed",
				SourceType: "ResourceSlice",
			})
		}

		unavailableCapacityCount := selectorPassed - unhealthyCount - capacityMatched
		if unavailableCapacityCount > 0 {
			summaryNode.Children = append(summaryNode.Children, model.ReasonNode{
				Message:    fmt.Sprintf("%d rejected because requested capacity (already allocated) was unavailable", unavailableCapacityCount),
				Confidence: "inferred",
				SourceType: "ResourceSlice",
			})
		}

		if selectorFailedErrorCount > 0 {
			summaryNode.Children = append(summaryNode.Children, model.ReasonNode{
				Message:    fmt.Sprintf("%d rejected because selector expression failed or was unsupported", selectorFailedErrorCount),
				Confidence: "confirmed",
				Evidence:   lastSelectorError.Error(),
				SourceType: "DeviceClass",
			})
		}

		rootNode.Children = append(rootNode.Children, summaryNode)

		if unhealthyCount > 0 {
			result.Remedy = append(result.Remedy, "Inspect and resolve health/degradation states for the unhealthy simulated devices.")
		}
		if capacityMatched == 0 && totalDevices > 0 && unhealthyCount == 0 {
			result.Remedy = append(result.Remedy, "Release existing allocations or increase device counts in the pool.")
		}
	}

	// 3. Incorporate cluster Events referencing the claim/pod
	eventsList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ev := range eventsList.Items {
			if ev.InvolvedObject.Name == claimName && ev.InvolvedObject.Kind == "ResourceClaim" {
				redactedMsg := redaction.RedactString(ev.Message)
				rootNode.Children = append(rootNode.Children, model.ReasonNode{
					Message:    fmt.Sprintf("Cluster Event: %s", ev.Reason),
					Confidence: "probable",
					Evidence:   fmt.Sprintf("[%s] %s", ev.Type, redactedMsg),
					SourceType: "Event",
				})
			}
		}
	}

	// 4. Check for delayed allocation (WaitForFirstConsumer)
	if len(liveClaim.Status.ReservedFor) == 0 {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "Claim uses delayed allocation, waiting for first consumer pod.",
			Confidence: "confirmed",
			Evidence:   "status.reservedFor is empty, no active pod has claimed this resource.",
			SourceType: "ResourceClaim",
			FieldPath:  ".status.reservedFor",
		})
		result.Remedy = append(result.Remedy, "Deploy a Pod referencing this ResourceClaim to trigger allocation.")
	} else {
		var consumers []string
		for _, ref := range liveClaim.Status.ReservedFor {
			consumers = append(consumers, ref.Name)
		}
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "Claim has active consumers but allocation is pending.",
			Confidence: "confirmed",
			Evidence:   fmt.Sprintf("Reserved for consumers: %s", strings.Join(consumers, ", ")),
			SourceType: "ResourceClaim",
			FieldPath:  ".status.reservedFor",
		})
	}

	result.ReasonTree = rootNode
	return result, nil
}
