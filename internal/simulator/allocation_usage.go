// Package simulator tracks committed and in-progress DRA device usage.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"fmt"
	"regexp"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var normalizedShareIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type committedDeviceUsage struct {
	exclusive bool
	shared    bool
	consumed  map[resourcev1.QualifiedName]resource.Quantity
	shares    []map[resourcev1.QualifiedName]resource.Quantity
	shareIDs  map[string]bool
}

type committedAllocationUsage map[string]*committedDeviceUsage

type allocationAttemptUsage struct {
	exclusive map[string]bool
	shared    map[string]bool
	consumed  map[string]map[resourcev1.QualifiedName]resource.Quantity
}

func collectCommittedAllocationUsage(claims []resourcev1.ResourceClaim) (committedAllocationUsage, *allocationFailure) {
	usage := make(committedAllocationUsage)
	for _, claim := range claims {
		if claim.Status.Allocation == nil {
			continue
		}
		for _, result := range claim.Status.Allocation.Devices.Results {
			if failure := usage.addResult(claim.Namespace, claim.Name, result); failure != nil {
				return nil, failure
			}
		}
	}
	return usage, nil
}

func (usage committedAllocationUsage) hasShared() bool {
	for _, deviceUsage := range usage {
		if deviceUsage.shared {
			return true
		}
	}
	return false
}

func (usage committedAllocationUsage) addResult(
	namespace, claimName string,
	result resourcev1.DeviceRequestAllocationResult,
) *allocationFailure {
	identity := deviceIdentityKey(result.Driver, result.Pool, result.Device)
	current := usage[identity]
	if current == nil {
		current = &committedDeviceUsage{}
		usage[identity] = current
	}

	hasShareID := result.ShareID != nil
	hasConsumedCapacity := result.ConsumedCapacity != nil
	if hasShareID != hasConsumedCapacity {
		return unsupportedFailure(fmt.Sprintf(
			"existing allocation %s/%s for device %s/%s/%s must set both ShareID and ConsumedCapacity or neither",
			namespace,
			claimName,
			result.Driver,
			result.Pool,
			result.Device,
		))
	}
	if !hasShareID {
		if current.exclusive || current.shared {
			return unsupportedFailure(fmt.Sprintf(
				"existing allocations contain conflicting exclusive and shared state for device %s/%s/%s",
				result.Driver,
				result.Pool,
				result.Device,
			))
		}
		current.exclusive = true
		return nil
	}
	if current.exclusive {
		return unsupportedFailure(fmt.Sprintf(
			"existing allocations contain conflicting exclusive and shared state for device %s/%s/%s",
			result.Driver,
			result.Pool,
			result.Device,
		))
	}
	shareID := string(*result.ShareID)
	if !normalizedShareIDPattern.MatchString(shareID) {
		return unsupportedFailure(fmt.Sprintf(
			"existing allocation %s/%s for device %s/%s/%s has invalid ShareID %q",
			namespace,
			claimName,
			result.Driver,
			result.Pool,
			result.Device,
			shareID,
		))
	}
	if current.shareIDs == nil {
		current.shareIDs = make(map[string]bool)
	}
	if current.shareIDs[shareID] {
		return unsupportedFailure(fmt.Sprintf(
			"existing allocations contain duplicate ShareID %q for device %s/%s/%s",
			shareID,
			result.Driver,
			result.Pool,
			result.Device,
		))
	}
	current.shareIDs[shareID] = true
	current.shared = true
	current.shares = append(current.shares, cloneQuantityMap(result.ConsumedCapacity))
	if current.consumed == nil {
		current.consumed = make(map[resourcev1.QualifiedName]resource.Quantity)
	}
	for name, amount := range result.ConsumedCapacity {
		if amount.Sign() < 0 {
			return unsupportedFailure(fmt.Sprintf(
				"existing allocation %s/%s for device %s/%s/%s has negative consumed capacity %q for %q",
				namespace,
				claimName,
				result.Driver,
				result.Pool,
				result.Device,
				amount.String(),
				name,
			))
		}
		addQuantity(current.consumed, name, amount)
	}
	return nil
}

func newAllocationAttemptUsage() allocationAttemptUsage {
	return allocationAttemptUsage{
		exclusive: make(map[string]bool),
		shared:    make(map[string]bool),
		consumed:  make(map[string]map[resourcev1.QualifiedName]resource.Quantity),
	}
}

func (usage allocationAttemptUsage) clone() allocationAttemptUsage {
	clone := newAllocationAttemptUsage()
	for identity, allocated := range usage.exclusive {
		clone.exclusive[identity] = allocated
	}
	for identity, shared := range usage.shared {
		clone.shared[identity] = shared
	}
	for identity, consumed := range usage.consumed {
		clone.consumed[identity] = cloneQuantityMap(consumed)
	}
	return clone
}

func (usage allocationAttemptUsage) reserve(result resourcev1.DeviceRequestAllocationResult) {
	identity := deviceIdentityKey(result.Driver, result.Pool, result.Device)
	if result.ShareID == nil {
		usage.exclusive[identity] = true
		return
	}
	usage.shared[identity] = true
	if usage.consumed[identity] == nil {
		usage.consumed[identity] = make(map[resourcev1.QualifiedName]resource.Quantity)
	}
	for name, amount := range result.ConsumedCapacity {
		addQuantity(usage.consumed[identity], name, amount)
	}
}

func cloneQuantityMap(source map[resourcev1.QualifiedName]resource.Quantity) map[resourcev1.QualifiedName]resource.Quantity {
	if source == nil {
		return nil
	}
	clone := make(map[resourcev1.QualifiedName]resource.Quantity, len(source))
	for name, amount := range source {
		clone[name] = amount.DeepCopy()
	}
	return clone
}

func addQuantity(
	target map[resourcev1.QualifiedName]resource.Quantity,
	name resourcev1.QualifiedName,
	amount resource.Quantity,
) {
	current := target[name]
	current.Add(amount)
	target[name] = current
}

func hasManagedShareableDevice(slices []resourcev1.ResourceSlice) bool {
	for sliceIndex := range slices {
		slice := &slices[sliceIndex]
		if !managedSimulatorSlice(slice) {
			continue
		}
		for deviceIndex := range slice.Spec.Devices {
			if deviceAllowsMultipleAllocations(&slice.Spec.Devices[deviceIndex]) {
				return true
			}
		}
	}
	return false
}

func deviceAllowsMultipleAllocations(device *resourcev1.Device) bool {
	return device.AllowMultipleAllocations != nil && *device.AllowMultipleAllocations
}

func (planner *claimPlanner) buildBacktrackingCandidate(
	requestName string,
	requirements *resourcev1.CapacityRequirements,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	attempt allocationAttemptUsage,
	seen map[string]bool,
) (resourcev1.DeviceRequestAllocationResult, bool, *allocationFailure) {
	identity := deviceIdentityKey(slice.Spec.Driver, slice.Spec.Pool.Name, device.Name)
	if seen[identity] {
		return resourcev1.DeviceRequestAllocationResult{}, false, nil
	}
	if deviceAllowsMultipleAllocations(device) {
		return planner.buildShareableCandidate(requestName, requirements, slice, device, identity, attempt, seen)
	}
	return planner.buildExclusiveCandidate(requestName, requirements, slice, device, identity, attempt, seen)
}

func (planner *claimPlanner) buildShareableCandidate(
	requestName string,
	requirements *resourcev1.CapacityRequirements,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	identity string,
	attempt allocationAttemptUsage,
	seen map[string]bool,
) (resourcev1.DeviceRequestAllocationResult, bool, *allocationFailure) {
	if len(device.Capacity) == 0 {
		return resourcev1.DeviceRequestAllocationResult{}, false, unsupportedFailure(fmt.Sprintf(
			"device %s/%s/%s allows multiple allocations but defines no consumable capacities",
			slice.Spec.Driver,
			slice.Spec.Pool.Name,
			device.Name,
		))
	}
	committed := planner.committedUsage[identity]
	if committed != nil && committed.exclusive {
		return resourcev1.DeviceRequestAllocationResult{}, false, unsupportedFailure(fmt.Sprintf(
			"device %s/%s/%s allows multiple allocations but has an existing exclusive allocation",
			slice.Spec.Driver,
			slice.Spec.Pool.Name,
			device.Name,
		))
	}
	if committed != nil && committed.shared {
		if failure := validateCommittedShareUsage(slice, device, committed); failure != nil {
			return resourcev1.DeviceRequestAllocationResult{}, false, failure
		}
	}
	if attempt.exclusive[identity] {
		return resourcev1.DeviceRequestAllocationResult{}, false, unsupportedFailure(fmt.Sprintf(
			"device %s/%s/%s cannot be both exclusive and shared in one allocation plan",
			slice.Spec.Driver,
			slice.Spec.Pool.Name,
			device.Name,
		))
	}

	var committedCapacity map[resourcev1.QualifiedName]resource.Quantity
	if committed != nil {
		committedCapacity = committed.consumed
	}
	consumed, fits, err := evaluateShareableCapacity(
		requirements,
		device.Capacity,
		committedCapacity,
		attempt.consumed[identity],
	)
	if err != nil {
		return resourcev1.DeviceRequestAllocationResult{}, false, unsupportedFailure(fmt.Sprintf(
			"device %s/%s/%s has invalid consumable-capacity state: %v",
			slice.Spec.Driver,
			slice.Spec.Pool.Name,
			device.Name,
			err,
		))
	}
	if !fits {
		return resourcev1.DeviceRequestAllocationResult{}, false, nil
	}

	seen[identity] = true
	shareID := uuid.NewUUID()
	return resourcev1.DeviceRequestAllocationResult{
		Request:          requestName,
		Driver:           slice.Spec.Driver,
		Pool:             slice.Spec.Pool.Name,
		Device:           device.Name,
		ShareID:          &shareID,
		ConsumedCapacity: consumed,
	}, true, nil
}

func (planner *claimPlanner) buildExclusiveCandidate(
	requestName string,
	requirements *resourcev1.CapacityRequirements,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	identity string,
	attempt allocationAttemptUsage,
	seen map[string]bool,
) (resourcev1.DeviceRequestAllocationResult, bool, *allocationFailure) {
	committed := planner.committedUsage[identity]
	if committed != nil && committed.shared {
		return resourcev1.DeviceRequestAllocationResult{}, false, unsupportedFailure(fmt.Sprintf(
			"device %s/%s/%s is exclusive but has existing shared consumption",
			slice.Spec.Driver,
			slice.Spec.Pool.Name,
			device.Name,
		))
	}
	if (committed != nil && committed.exclusive) || attempt.exclusive[identity] || attempt.shared[identity] {
		return resourcev1.DeviceRequestAllocationResult{}, false, nil
	}
	matches, failure := exclusiveCapacityMatches(device, requirements)
	if failure != nil || !matches {
		return resourcev1.DeviceRequestAllocationResult{}, matches, failure
	}

	seen[identity] = true
	return resourcev1.DeviceRequestAllocationResult{
		Request: requestName,
		Driver:  slice.Spec.Driver,
		Pool:    slice.Spec.Pool.Name,
		Device:  device.Name,
	}, true, nil
}

func validateCommittedShareUsage(
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	committed *committedDeviceUsage,
) *allocationFailure {
	for shareIndex, consumed := range committed.shares {
		for name := range device.Capacity {
			if _, found := consumed[name]; !found {
				return unsupportedFailure(fmt.Sprintf(
					"existing share %d for device %s/%s/%s does not report consumed capacity %q",
					shareIndex,
					slice.Spec.Driver,
					slice.Spec.Pool.Name,
					device.Name,
					name,
				))
			}
		}
		for name := range consumed {
			if _, found := device.Capacity[name]; !found {
				return unsupportedFailure(fmt.Sprintf(
					"existing share %d for device %s/%s/%s reports unknown consumed capacity %q",
					shareIndex,
					slice.Spec.Driver,
					slice.Spec.Pool.Name,
					device.Name,
					name,
				))
			}
		}
	}
	for name, amount := range committed.consumed {
		capacity := device.Capacity[name]
		if amount.Cmp(capacity.Value) > 0 {
			return unsupportedFailure(fmt.Sprintf(
				"existing consumed capacity %q for device %s/%s/%s exceeds device capacity %q for %q",
				amount.String(),
				slice.Spec.Driver,
				slice.Spec.Pool.Name,
				device.Name,
				capacity.Value.String(),
				name,
			))
		}
	}
	return nil
}
