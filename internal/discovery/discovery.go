// Package discovery queries the Kubernetes API for Dynamic Resource Allocation (DRA) resources.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/oaslananka/draforge/pkg/model"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	managedByLabel = "draforge.oaslananka/managed-by"
	syntheticLabel = "draforge.oaslananka/synthetic"
)

// DiscoverDRA queries the Kubernetes cluster for all DRA-related objects and maps them to our model.
func DiscoverDRA(ctx context.Context, clientset kubernetes.Interface) ([]model.DevicePool, []model.Device, []model.ResourceClaimInfo, error) {
	pools, devices, claims, _, err := DiscoverDRAWithStatus(ctx, clientset)
	return pools, devices, claims, err
}

// DiscoverDRAWithStatus queries the Kubernetes cluster for all DRA-related objects and maps them to our model, returning discovery status.
func DiscoverDRAWithStatus(ctx context.Context, clientset kubernetes.Interface) ([]model.DevicePool, []model.Device, []model.ResourceClaimInfo, model.DiscoveryStatus, error) {
	claims, claimAvailable, claimWarnings := discoverClaims(ctx, clientset)
	pools, devices, slicesAvailable, sliceWarnings := discoverSlices(ctx, clientset)
	podsAvailable, podWarnings := attachClaimOwners(ctx, clientset, claims)

	warnings := append([]string(nil), claimWarnings...)
	warnings = append(warnings, sliceWarnings...)
	warnings = append(warnings, podWarnings...)
	status := model.DiscoveryStatus{
		ResourceClaimsAvailable: claimAvailable,
		ResourceSlicesAvailable: slicesAvailable,
		PodsAvailable:           podsAvailable,
		IsPartial:               !claimAvailable || !slicesAvailable || !podsAvailable,
		Warnings:                warnings,
	}

	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Namespace != claims[j].Namespace {
			return claims[i].Namespace < claims[j].Namespace
		}
		return claims[i].Name < claims[j].Name
	})
	sort.Slice(pools, func(i, j int) bool {
		return poolIdentity(pools[i].DriverName, pools[i].Name, pools[i].NodeName) <
			poolIdentity(pools[j].DriverName, pools[j].Name, pools[j].NodeName)
	})
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })

	return pools, devices, claims, status, nil
}

func discoverClaims(ctx context.Context, clientset kubernetes.Interface) ([]model.ResourceClaimInfo, bool, []string) {
	claimList, err := clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		message := fmt.Sprintf("resource.k8s.io/v1 ResourceClaims API not available: %v", err)
		fmt.Printf("Warning: %s\n", message)
		return nil, false, []string{message}
	}

	claims := make([]model.ResourceClaimInfo, 0, len(claimList.Items))
	for i := range claimList.Items {
		claims = append(claims, mapResourceClaim(&claimList.Items[i]))
	}
	return claims, true, nil
}

func mapResourceClaim(claim *resourcev1.ResourceClaim) model.ResourceClaimInfo {
	requests := mapClaimRequests(claim.Spec.Devices.Requests)
	nodeName := allocationNodeName(claim.Status.Allocation)
	allocations := mapClaimAllocations(claim.Status.Allocation, nodeName)

	status := "Pending"
	if len(allocations) > 0 {
		status = "Allocated"
	}

	info := model.ResourceClaimInfo{
		Name:        claim.Name,
		Namespace:   claim.Namespace,
		Status:      status,
		Requests:    requests,
		Allocations: allocations,
		CreatedAt:   claim.CreationTimestamp.Time,
	}

	// Deprecated compatibility fields remain populated from the first complete
	// request/allocation while new consumers use Requests and Allocations.
	if len(requests) > 0 && len(requests[0].Alternatives) > 0 {
		info.DeviceClassName = requests[0].Alternatives[0].DeviceClassName
	}
	if len(allocations) > 0 {
		info.AllocatedDevice = allocations[0].DeviceName
		info.AllocatedDriver = allocations[0].DriverName
		info.AllocatedNode = allocations[0].NodeName
	} else {
		info.AllocatedNode = nodeName
	}
	return info
}

func mapClaimRequests(requests []resourcev1.DeviceRequest) []model.ClaimRequest {
	result := make([]model.ClaimRequest, 0, len(requests))
	for _, request := range requests {
		mapped := model.ClaimRequest{Name: request.Name}
		switch {
		case request.Exactly != nil:
			mapped.Mode = "Exactly"
			mapped.Alternatives = []model.ClaimRequestAlternative{mapExactRequest(request.Name, request.Exactly)}
		case len(request.FirstAvailable) > 0:
			mapped.Mode = "FirstAvailable"
			mapped.Alternatives = make([]model.ClaimRequestAlternative, 0, len(request.FirstAvailable))
			for _, alternative := range request.FirstAvailable {
				mapped.Alternatives = append(mapped.Alternatives, mapSubRequest(alternative))
			}
		default:
			mapped.Mode = "Unknown"
		}
		result = append(result, mapped)
	}
	return result
}

func mapExactRequest(name string, request *resourcev1.ExactDeviceRequest) model.ClaimRequestAlternative {
	return model.ClaimRequestAlternative{
		Name:            name,
		DeviceClassName: request.DeviceClassName,
		AllocationMode:  normalizedAllocationMode(request.AllocationMode),
		Count:           normalizedCount(request.AllocationMode, request.Count),
	}
}

func mapSubRequest(request resourcev1.DeviceSubRequest) model.ClaimRequestAlternative {
	return model.ClaimRequestAlternative{
		Name:            request.Name,
		DeviceClassName: request.DeviceClassName,
		AllocationMode:  normalizedAllocationMode(request.AllocationMode),
		Count:           normalizedCount(request.AllocationMode, request.Count),
	}
}

func normalizedAllocationMode(mode resourcev1.DeviceAllocationMode) string {
	if mode == "" {
		return "ExactCount"
	}
	return string(mode)
}

func normalizedCount(mode resourcev1.DeviceAllocationMode, count int64) int64 {
	if normalizedAllocationMode(mode) == "ExactCount" && count == 0 {
		return 1
	}
	return count
}

func allocationNodeName(allocation *resourcev1.AllocationResult) string {
	if allocation == nil || allocation.NodeSelector == nil || len(allocation.NodeSelector.NodeSelectorTerms) == 0 {
		return ""
	}
	candidate := ""
	for _, term := range allocation.NodeSelector.NodeSelectorTerms {
		termNode := exactNodeForTerm(term)
		if termNode == "" {
			return ""
		}
		if candidate == "" {
			candidate = termNode
			continue
		}
		if candidate != termNode {
			return ""
		}
	}
	return candidate
}

func exactNodeForTerm(term corev1.NodeSelectorTerm) string {
	candidate := ""
	for _, requirement := range term.MatchFields {
		if requirement.Key != "metadata.name" || requirement.Operator != corev1.NodeSelectorOpIn || len(requirement.Values) != 1 {
			continue
		}
		candidate = mergeExactNode(candidate, requirement.Values[0])
		if candidate == "" {
			return ""
		}
	}
	for _, requirement := range term.MatchExpressions {
		if requirement.Key != "kubernetes.io/hostname" || requirement.Operator != corev1.NodeSelectorOpIn || len(requirement.Values) != 1 {
			continue
		}
		candidate = mergeExactNode(candidate, requirement.Values[0])
		if candidate == "" {
			return ""
		}
	}
	return candidate
}

func mergeExactNode(current, next string) string {
	if current == "" || current == next {
		return next
	}
	return ""
}

func mapClaimAllocations(allocation *resourcev1.AllocationResult, nodeName string) []model.ClaimAllocation {
	if allocation == nil {
		return []model.ClaimAllocation{}
	}
	results := allocation.Devices.Results
	mapped := make([]model.ClaimAllocation, 0, len(results))
	for _, result := range results {
		mapped = append(mapped, model.ClaimAllocation{
			Request:    result.Request,
			DriverName: result.Driver,
			PoolName:   result.Pool,
			DeviceName: result.Device,
			NodeName:   nodeName,
		})
	}
	return mapped
}

func discoverSlices(ctx context.Context, clientset kubernetes.Interface) ([]model.DevicePool, []model.Device, bool, []string) {
	sliceList, err := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		message := fmt.Sprintf("resource.k8s.io/v1 ResourceSlices API not available: %v", err)
		fmt.Printf("Warning: %s\n", message)
		return nil, nil, false, []string{message}
	}

	poolMap := make(map[string]*model.DevicePool)
	devices := make([]model.Device, 0)
	for index := range sliceList.Items {
		slice := &sliceList.Items[index]
		driverName, poolName, nodeName, health := sliceIdentity(slice)
		pool := ensurePool(poolMap, slice, driverName, poolName, nodeName, health)
		devices = append(devices, mapSliceDevices(slice, pool, driverName, poolName, nodeName, health)...)
	}

	pools := make([]model.DevicePool, 0, len(poolMap))
	for _, pool := range poolMap {
		pools = append(pools, *pool)
	}
	return pools, devices, true, nil
}

func sliceIdentity(slice *resourcev1.ResourceSlice) (driverName, poolName, nodeName, health string) {
	driverName = slice.Spec.Driver
	poolName = slice.Spec.Pool.Name
	if slice.Spec.NodeName != nil {
		nodeName = *slice.Spec.NodeName
	}
	health = slice.Labels["draforge.oaslananka/health"]
	if health == "" {
		health = "healthy"
	}
	return driverName, poolName, nodeName, health
}

func ensurePool(poolMap map[string]*model.DevicePool, slice *resourcev1.ResourceSlice, driverName, poolName, nodeName, health string) *model.DevicePool {
	key := poolIdentity(driverName, poolName, nodeName)
	pool, exists := poolMap[key]
	if !exists {
		pool = &model.DevicePool{
			Name:        poolName,
			DriverName:  driverName,
			NodeName:    nodeName,
			DeviceType:  "unknown",
			Health:      health,
			IsSynthetic: hasSyntheticLabel(slice.Labels),
			Labels:      slice.Labels,
		}
		poolMap[key] = pool
		return pool
	}
	pool.IsSynthetic = pool.IsSynthetic || hasSyntheticLabel(slice.Labels)
	if health != "healthy" {
		pool.Health = health
	}
	return pool
}

func mapSliceDevices(slice *resourcev1.ResourceSlice, pool *model.DevicePool, driverName, poolName, nodeName, health string) []model.Device {
	devices := make([]model.Device, 0, len(slice.Spec.Devices))
	for _, specification := range slice.Spec.Devices {
		device := mapDevice(slice, specification, driverName, poolName, nodeName, health)
		devices = append(devices, device)
		pool.DeviceCount++
		if device.Type != "unknown" {
			pool.DeviceType = device.Type
		}
	}
	return devices
}

func mapDevice(slice *resourcev1.ResourceSlice, specification resourcev1.Device, driverName, poolName, nodeName, health string) model.Device {
	attributes := make(map[string]string)
	capacities := make(map[string]int64)
	deviceType := "unknown"

	for attributeName, attributeValue := range specification.Attributes {
		name := string(attributeName)
		switch {
		case attributeValue.StringValue != nil:
			attributes[name] = *attributeValue.StringValue
			if strings.EqualFold(name, "type") || strings.EqualFold(name, "model") {
				deviceType = strings.ToLower(*attributeValue.StringValue)
			}
		case attributeValue.IntValue != nil:
			attributes[name] = fmt.Sprintf("%d", *attributeValue.IntValue)
		case attributeValue.BoolValue != nil:
			attributes[name] = fmt.Sprintf("%t", *attributeValue.BoolValue)
		}
	}
	for capacityName, capacityValue := range specification.Capacity {
		capacities[string(capacityName)] = capacityValue.Value.Value()
	}

	typedSpecification := specification.DeepCopy()

	return model.Device{
		ID:                          deviceIdentity(driverName, poolName, nodeName, specification.Name),
		Name:                        specification.Name,
		Type:                        deviceType,
		Status:                      health,
		DriverName:                  driverName,
		NodeName:                    nodeName,
		PoolName:                    poolName,
		Attributes:                  attributes,
		Capacities:                  capacities,
		IsSynthetic:                 hasSyntheticLabel(slice.Labels),
		LastUpdated:                 slice.CreationTimestamp.Time,
		DRAAttributes:               typedSpecification.Attributes,
		DRACapacity:                 typedSpecification.Capacity,
		DRAAllowMultipleAllocations: typedSpecification.AllowMultipleAllocations,
	}
}

func attachClaimOwners(ctx context.Context, clientset kubernetes.Interface, claims []model.ResourceClaimInfo) (bool, []string) {
	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		message := fmt.Sprintf("core/v1 Pods API not available: %v", err)
		fmt.Printf("Warning: %s\n", message)
		return false, []string{message}
	}

	claimIndexes := make(map[string]int, len(claims))
	for index := range claims {
		claimIndexes[claims[index].Namespace+"\x00"+claims[index].Name] = index
	}
	for _, pod := range podList.Items {
		for _, podClaim := range pod.Spec.ResourceClaims {
			claimName := ""
			if podClaim.ResourceClaimName != nil {
				claimName = *podClaim.ResourceClaimName
			} else if podClaim.ResourceClaimTemplateName != nil {
				claimName = pod.Name + "-" + podClaim.Name
			}
			if index, exists := claimIndexes[pod.Namespace+"\x00"+claimName]; exists {
				claims[index].OwnerPodName = pod.Name
			}
		}
	}
	return true, nil
}

func hasSyntheticLabel(labels map[string]string) bool {
	return strings.EqualFold(labels[managedByLabel], "simulator") || strings.EqualFold(labels[syntheticLabel], "true")
}

func poolIdentity(driverName, poolName, nodeName string) string {
	return stableIdentity("pool", driverName, poolName, nodeName)
}

func deviceIdentity(driverName, poolName, nodeName, deviceName string) string {
	return stableIdentity("device", driverName, nodeName, poolName, deviceName)
}

func stableIdentity(kind string, parts ...string) string {
	encoded := make([]string, 0, len(parts)+1)
	encoded = append(encoded, kind)
	for _, part := range parts {
		encoded = append(encoded, fmt.Sprintf("%d:%s", len(part), part))
	}
	return strings.Join(encoded, "/")
}
