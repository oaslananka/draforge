// Package simulator implements exclusive-device capacity filtering.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
)

func validateExclusiveCapacityRequirements(
	requestName string,
	requirements *resourcev1.CapacityRequirements,
) *allocationFailure {
	if requirements == nil {
		return nil
	}
	for name, requested := range requirements.Requests {
		if requested.Sign() < 0 {
			return unsupportedFailure(fmt.Sprintf(
				"request %q uses negative capacity %q for %q",
				requestName,
				requested.String(),
				name,
			))
		}
	}
	return nil
}

func exclusiveCapacityMatches(
	device *resourcev1.Device,
	requirements *resourcev1.CapacityRequirements,
) (bool, *allocationFailure) {
	if device.AllowMultipleAllocations != nil && *device.AllowMultipleAllocations {
		return false, unsupportedFailure(fmt.Sprintf("device %q allows multiple allocations", device.Name))
	}
	for name, capacity := range device.Capacity {
		if capacity.RequestPolicy != nil {
			return false, unsupportedFailure(fmt.Sprintf(
				"device %q capacity %q uses unsupported request policy",
				device.Name,
				name,
			))
		}
		if capacity.Value.Sign() < 0 {
			return false, unsupportedFailure(fmt.Sprintf(
				"device %q capacity %q has invalid negative value %q",
				device.Name,
				name,
				capacity.Value.String(),
			))
		}
	}
	if requirements == nil || len(requirements.Requests) == 0 {
		return true, nil
	}
	for name, requested := range requirements.Requests {
		capacity, found := device.Capacity[name]
		if !found || requested.Cmp(capacity.Value) > 0 {
			return false, nil
		}
	}
	return true, nil
}
