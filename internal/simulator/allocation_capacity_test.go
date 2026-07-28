// Package simulator tests exclusive-device capacity requirements.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func capacityRequirements(values map[resourcev1.QualifiedName]string) *resourcev1.CapacityRequirements {
	requests := make(map[resourcev1.QualifiedName]resource.Quantity, len(values))
	for name, value := range values {
		requests[name] = resource.MustParse(value)
	}
	return &resourcev1.CapacityRequirements{Requests: requests}
}

func capacityDevice(name string, values map[resourcev1.QualifiedName]string) resourcev1.Device {
	capacities := make(map[resourcev1.QualifiedName]resourcev1.DeviceCapacity, len(values))
	for capacityName, value := range values {
		capacities[capacityName] = resourcev1.DeviceCapacity{Value: resource.MustParse(value)}
	}
	return resourcev1.Device{Name: name, Capacity: capacities}
}

func exactCapacityRequest(name, class string, count int64, values map[resourcev1.QualifiedName]string) resourcev1.DeviceRequest {
	return resourcev1.DeviceRequest{
		Name: name,
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: class,
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           count,
			Capacity:        capacityRequirements(values),
		},
	}
}

func TestSimulateAllocationSelectsExclusiveDeviceWithSufficientCapacity(t *testing.T) {
	claim := allocationClaim("capacity-filter", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi"},
	))
	slice := allocationSlice(
		"capacity-devices",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("small", map[resourcev1.QualifiedName]string{"memory": "4Gi"}),
		capacityDevice("large", map[resourcev1.QualifiedName]string{"memory": "16Gi"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Device != "large" {
		t.Fatalf("expected capacity-sufficient device, got %#v", result)
	}
	if result.ShareID != nil || len(result.ConsumedCapacity) != 0 {
		t.Fatalf("exclusive allocation must not populate sharing status: %#v", result)
	}
}

func TestSimulateAllocationRequiresEveryRequestedCapacity(t *testing.T) {
	claim := allocationClaim("capacity-all-keys", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi", "network.example.com/bandwidth": "100"},
	))
	slice := allocationSlice(
		"capacity-devices",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("missing-bandwidth", map[resourcev1.QualifiedName]string{"memory": "16Gi"}),
		capacityDevice("complete", map[resourcev1.QualifiedName]string{"memory": "16Gi", "network.example.com/bandwidth": "200"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	if result := allocatedResult(t, clientset, claim.Name); result.Device != "complete" {
		t.Fatalf("expected device satisfying every capacity key, got %#v", result)
	}
}

func TestSimulateAllocationFirstAvailableFallsBackAfterCapacityMiss(t *testing.T) {
	claim := allocationClaim("capacity-fallback", resourcev1.DeviceRequest{
		Name: "device",
		FirstAvailable: []resourcev1.DeviceSubRequest{
			{
				Name:            "large",
				DeviceClassName: "capacity-class",
				AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
				Count:           1,
				Capacity:        capacityRequirements(map[resourcev1.QualifiedName]string{"memory": "32Gi"}),
			},
			{
				Name:            "small",
				DeviceClassName: "capacity-class",
				AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
				Count:           1,
				Capacity:        capacityRequirements(map[resourcev1.QualifiedName]string{"memory": "8Gi"}),
			},
		},
	})
	slice := allocationSlice(
		"capacity-device",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("device-0", map[resourcev1.QualifiedName]string{"memory": "16Gi"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "device/small" {
		t.Fatalf("expected capacity-compatible fallback, got %#v", result)
	}
}

func TestSimulateAllocationAllFiltersExclusiveDevicesByCapacity(t *testing.T) {
	claim := allocationClaim("capacity-all", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "capacity-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
			Capacity:        capacityRequirements(map[resourcev1.QualifiedName]string{"memory": "8Gi"}),
		},
	})
	slice := allocationSlice(
		"capacity-devices",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("small", map[resourcev1.QualifiedName]string{"memory": "4Gi"}),
		capacityDevice("large-a", map[resourcev1.QualifiedName]string{"memory": "8Gi"}),
		capacityDevice("large-b", map[resourcev1.QualifiedName]string{"memory": "16Gi"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "large-a" || results[1].Device != "large-b" {
		t.Fatalf("expected all capacity-compatible exclusive devices, got %#v", results)
	}
}

func TestSimulateAllocationConstraintsBacktrackAcrossCapacityCandidates(t *testing.T) {
	groupA := "a"
	groupB := "b"
	claim := constrainedClaim(
		"capacity-constraint-backtracking",
		[]resourcev1.DeviceConstraint{matchConstraint("driver.example.com/group")},
		exactCapacityRequest("first", "capacity-class", 1, map[resourcev1.QualifiedName]string{"memory": "8Gi"}),
		exactCapacityRequest("second", "capacity-class", 1, map[resourcev1.QualifiedName]string{"memory": "8Gi"}),
	)
	slice := allocationSlice(
		"capacity-devices",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "a-large", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"group": {StringValue: &groupA}}, Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("16Gi")}}},
		resourcev1.Device{Name: "b-small", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"group": {StringValue: &groupB}}, Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("4Gi")}}},
		resourcev1.Device{Name: "b-large-0", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"group": {StringValue: &groupB}}, Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("8Gi")}}},
		resourcev1.Device{Name: "b-large-1", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"group": {StringValue: &groupB}}, Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("12Gi")}}},
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "b-large-0" || results[1].Device != "b-large-1" {
		t.Fatalf("expected constraint backtracking over capacity-compatible candidates, got %#v", results)
	}
}

func TestSimulateAllocationIgnoresUnsupportedCapacityPolicyOnSelectorRejectedDevice(t *testing.T) {
	otherKind := "other"
	targetKind := "target"
	policyDefault := resource.MustParse("4Gi")
	policyDevice := capacityDevice("policy-device", map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	policyCapacity := policyDevice.Capacity["memory"]
	policyCapacity.RequestPolicy = &resourcev1.CapacityRequestPolicy{Default: &policyDefault}
	policyDevice.Capacity["memory"] = policyCapacity
	policyDevice.Attributes = map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &otherKind}}
	validDevice := capacityDevice("valid-device", map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	validDevice.Attributes = map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &targetKind}}
	claim := allocationClaim("capacity-policy-selector-boundary", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi"},
	))
	class := allocationClass("capacity-class", selector(`device.attributes["driver.example.com"].kind == "target"`))
	slice := allocationSlice("capacity-devices", "driver.example.com", "pool-a", "node-a", policyDevice, validDevice)

	_, clientset := runAllocation(t, claim, class, slice)
	if result := allocatedResult(t, clientset, claim.Name); result.Device != "valid-device" {
		t.Fatalf("expected selector-rejected policy device to be ignored, got %#v", result)
	}
}

func TestSimulateAllocationRejectsCapacityPolicyWithoutRequest(t *testing.T) {
	policyDefault := resource.MustParse("4Gi")
	policyDevice := capacityDevice("policy-device", map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	policyCapacity := policyDevice.Capacity["memory"]
	policyCapacity.RequestPolicy = &resourcev1.CapacityRequestPolicy{Default: &policyDefault}
	policyDevice.Capacity["memory"] = policyCapacity
	claim := allocationClaim("capacity-policy-without-request", resourcev1.DeviceRequest{
		Name:    "device",
		Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "capacity-class", Count: 1},
	})
	slice := allocationSlice("capacity-device", "driver.example.com", "pool-a", "node-a", policyDevice)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, reasonUnsupportedRequest)
}

func TestSimulateAllocationRejectsNegativeDeviceCapacity(t *testing.T) {
	claim := allocationClaim("capacity-negative-device", resourcev1.DeviceRequest{
		Name:    "device",
		Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "capacity-class", Count: 1},
	})
	slice := allocationSlice(
		"capacity-device",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("device-0", map[resourcev1.QualifiedName]string{"memory": "-1Gi"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, reasonUnsupportedRequest)
}

func TestSimulateAllocationRejectsUnsupportedCapacityContracts(t *testing.T) {
	negative := allocationClaim("capacity-negative", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "-1Gi"},
	))
	policyDefault := resource.MustParse("4Gi")
	policyDevice := capacityDevice("policy-device", map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	capacity := policyDevice.Capacity["memory"]
	capacity.RequestPolicy = &resourcev1.CapacityRequestPolicy{Default: &policyDefault}
	policyDevice.Capacity["memory"] = capacity
	policyClaim := allocationClaim("capacity-policy", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi"},
	))
	shareable := true
	shareableDevice := capacityDevice("shareable-device", map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	shareableDevice.AllowMultipleAllocations = &shareable
	shareClaim := allocationClaim("capacity-shareable", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi"},
	))

	tests := []struct {
		name    string
		claim   *resourcev1.ResourceClaim
		devices []resourcev1.Device
	}{
		{name: "negative request", claim: negative, devices: []resourcev1.Device{capacityDevice("device-0", map[resourcev1.QualifiedName]string{"memory": "16Gi"})}},
		{name: "request policy", claim: policyClaim, devices: []resourcev1.Device{policyDevice}},
		{name: "shareable device", claim: shareClaim, devices: []resourcev1.Device{shareableDevice}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			slice := allocationSlice("capacity-device", "driver.example.com", "pool-a", "node-a", test.devices...)
			_, clientset := runAllocation(t, test.claim, allocationClass("capacity-class"), slice)
			assertPendingWithEvent(t, clientset, test.claim.Name, reasonUnsupportedRequest)
		})
	}
}

func TestExclusiveCapacityAllocationLeavesSharingFieldsEmpty(t *testing.T) {
	claim := allocationClaim("capacity-status", exactCapacityRequest(
		"device",
		"capacity-class",
		1,
		map[resourcev1.QualifiedName]string{"memory": "8Gi"},
	))
	slice := allocationSlice(
		"capacity-device",
		"driver.example.com",
		"pool-a",
		"node-a",
		capacityDevice("device-0", map[resourcev1.QualifiedName]string{"memory": "16Gi"}),
	)

	_, clientset := runAllocation(t, claim, allocationClass("capacity-class"), slice)
	allocation := requiredAllocation(t, clientset, claim.Name)
	result := allocation.Devices.Results[0]
	if result.ShareID != nil || result.ConsumedCapacity != nil {
		t.Fatalf("exclusive capacity result unexpectedly contains sharing fields: %#v", result)
	}
}
