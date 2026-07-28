// Package simulator tests stable Kubernetes DRA claim constraints.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"fmt"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const constraintDriver = "driver.example.com"

func matchConstraint(attribute string, requests ...string) resourcev1.DeviceConstraint {
	name := resourcev1.FullyQualifiedName(attribute)
	return resourcev1.DeviceConstraint{Requests: requests, MatchAttribute: &name}
}

func constrainedClaim(name string, constraints []resourcev1.DeviceConstraint, requests ...resourcev1.DeviceRequest) *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{Requests: requests, Constraints: constraints},
		},
	}
}

func exactConstraintRequest(name string, count int64) resourcev1.DeviceRequest {
	return resourcev1.DeviceRequest{
		Name: name,
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "constraint-class",
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           count,
		},
	}
}

func constraintSlice(name, pool, node string, devices ...resourcev1.Device) *resourcev1.ResourceSlice {
	return allocationSlice(name, constraintDriver, pool, node, devices...)
}

func stringConstraintDevice(name, attribute, value string) resourcev1.Device {
	return resourcev1.Device{
		Name: name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			resourcev1.QualifiedName(attribute): {StringValue: &value},
		},
	}
}

func TestSimulateAllocationBacktracksForMatchAttribute(t *testing.T) {
	fabricA := "a"
	fabricB := "b"
	claim := constrainedClaim(
		"constraint-backtracking",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/fabric")},
		exactConstraintRequest("first", 1),
		exactConstraintRequest("second", 1),
	)
	slice := constraintSlice(
		"constraint-devices", "pool-a", "node-a",
		stringConstraintDevice("device-a", "fabric", fabricA),
		stringConstraintDevice("device-b0", "fabric", fabricB),
		stringConstraintDevice("device-b1", "fabric", fabricB),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "device-b0" || results[1].Device != "device-b1" {
		t.Fatalf("expected backtracking to choose the matching b devices, got %#v", results)
	}
}

func TestSimulateAllocationBacktracksToOneCompatibleNode(t *testing.T) {
	groupA := "a"
	groupB := "b"
	claim := constrainedClaim(
		"constraint-node-backtracking",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/group")},
		exactConstraintRequest("first", 1),
		exactConstraintRequest("second", 1),
	)
	nodeA := constraintSlice(
		"node-a-devices", "pool-a", "node-a",
		stringConstraintDevice("device-a", "group", groupA),
	)
	nodeB := constraintSlice(
		"node-b-devices", "pool-b", "node-b",
		stringConstraintDevice("device-b0", "group", groupB),
		stringConstraintDevice("device-b1", "group", groupB),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), nodeA, nodeB)
	allocation := requiredAllocation(t, clientset, claim.Name)
	results := allocation.Devices.Results
	if len(results) != 2 || results[0].Device != "device-b0" || results[1].Device != "device-b1" {
		t.Fatalf("expected backtracking to the compatible node-b devices, got %#v", results)
	}
	if got := allocation.NodeSelector.NodeSelectorTerms[0].MatchFields[0].Values[0]; got != "node-b" {
		t.Fatalf("expected node-b selector, got %q", got)
	}
}

func TestSimulateAllocationConstraintsPreserveRemainingResultBudgetFallback(t *testing.T) {
	group := "shared"
	attributeName := resourcev1.QualifiedName("topology.example.com/group")
	claim := constrainedClaim(
		"constraint-result-budget",
		[]resourcev1.DeviceConstraint{matchConstraint(string(attributeName))},
		exactConstraintRequest("first", 1),
		resourcev1.DeviceRequest{
			Name: "bulk",
			FirstAvailable: []resourcev1.DeviceSubRequest{
				{Name: "all", DeviceClassName: "constraint-class", AllocationMode: resourcev1.DeviceAllocationModeAll},
				{Name: "one", DeviceClassName: "constraint-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
			},
		},
	)
	devices := make([]resourcev1.Device, resourcev1.AllocationResultsMaxSize+1)
	for index := range devices {
		devices[index] = resourcev1.Device{
			Name:       fmt.Sprintf("device-%02d", index),
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{attributeName: {StringValue: &group}},
		}
	}
	slice := constraintSlice("budget-devices", "pool-a", "node-a", devices...)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Request != "first" || results[1].Request != "bulk/one" {
		t.Fatalf("expected first + budget-fitting fallback, got %#v", results)
	}
}

func TestSimulateAllocationBacktracksWithinExactCountCombination(t *testing.T) {
	groupA := "a"
	groupB := "b"
	claim := constrainedClaim(
		"constraint-combination-backtracking",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/group")},
		exactConstraintRequest("devices", 2),
	)
	slice := constraintSlice(
		"combination-devices", "pool-a", "node-a",
		stringConstraintDevice("device-a", "group", groupA),
		stringConstraintDevice("device-b0", "group", groupB),
		stringConstraintDevice("device-b1", "group", groupB),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "device-b0" || results[1].Device != "device-b1" {
		t.Fatalf("expected combination backtracking to choose the matching b devices, got %#v", results)
	}
}

func TestSimulateAllocationMatchAttributeScalarTypes(t *testing.T) {
	stringValue := "zone-a"
	intValue := int64(7)
	boolValue := true
	versionValue := "1.2.3"
	tests := []struct {
		name      string
		attribute string
		first     resourcev1.DeviceAttribute
		second    resourcev1.DeviceAttribute
		allocated bool
	}{
		{name: "string", attribute: "zone", first: resourcev1.DeviceAttribute{StringValue: &stringValue}, second: resourcev1.DeviceAttribute{StringValue: &stringValue}, allocated: true},
		{name: "int", attribute: "numa", first: resourcev1.DeviceAttribute{IntValue: &intValue}, second: resourcev1.DeviceAttribute{IntValue: &intValue}, allocated: true},
		{name: "bool", attribute: "secure", first: resourcev1.DeviceAttribute{BoolValue: &boolValue}, second: resourcev1.DeviceAttribute{BoolValue: &boolValue}, allocated: true},
		{name: "version", attribute: "firmware", first: resourcev1.DeviceAttribute{VersionValue: &versionValue}, second: resourcev1.DeviceAttribute{VersionValue: &versionValue}, allocated: true},
		{name: "type mismatch", attribute: "mixed", first: resourcev1.DeviceAttribute{StringValue: &stringValue}, second: resourcev1.DeviceAttribute{IntValue: &intValue}, allocated: false},
		{name: "missing attribute", attribute: "missing", first: resourcev1.DeviceAttribute{StringValue: &stringValue}, allocated: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claim := constrainedClaim(
				"constraint-"+test.name,
				[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/" + test.attribute)},
				exactConstraintRequest("devices", 2),
			)
			firstAttributes := map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{resourcev1.QualifiedName(test.attribute): test.first}
			secondAttributes := map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{}
			if test.second.StringValue != nil || test.second.IntValue != nil || test.second.BoolValue != nil || test.second.VersionValue != nil {
				secondAttributes[resourcev1.QualifiedName(test.attribute)] = test.second
			}
			slice := allocationSlice(
				"typed-devices",
				constraintDriver,
				"pool-a",
				"node-a",
				resourcev1.Device{Name: "device-0", Attributes: firstAttributes},
				resourcev1.Device{Name: "device-1", Attributes: secondAttributes},
			)

			_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
			if test.allocated {
				results := requiredAllocation(t, clientset, claim.Name).Devices.Results
				if len(results) != 2 {
					t.Fatalf("expected two matching devices, got %#v", results)
				}
				return
			}
			assertPendingWithEvent(t, clientset, claim.Name, reasonNoMatch)
		})
	}
}

func TestSimulateAllocationScopesMatchAttributeRequests(t *testing.T) {
	leftValue := "left"
	rightValue := "right"
	claim := constrainedClaim(
		"constraint-request-scope",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver+"/group", "left")},
		exactConstraintRequest("left", 1),
		exactConstraintRequest("right", 1),
	)
	slice := constraintSlice(
		"scoped-devices", "pool-a", "node-a",
		stringConstraintDevice("left-device", "group", leftValue),
		stringConstraintDevice("right-device", "group", rightValue),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Request != "left" || results[1].Request != "right" {
		t.Fatalf("expected request-scoped constraint to leave right unaffected, got %#v", results)
	}
}

func TestSimulateAllocationUsesFullyQualifiedThirdPartyMatchAttribute(t *testing.T) {
	zone := "zone-a"
	attributeName := "topology.example.com/zone"
	claim := constrainedClaim(
		"constraint-third-party",
		[]resourcev1.DeviceConstraint{matchConstraint(attributeName)},
		exactConstraintRequest("devices", 2),
	)
	slice := constraintSlice(
		"third-party-devices", "pool-a", "node-a",
		stringConstraintDevice("device-0", attributeName, zone),
		stringConstraintDevice("device-1", attributeName, zone),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	if results := requiredAllocation(t, clientset, claim.Name).Devices.Results; len(results) != 2 {
		t.Fatalf("expected fully-qualified third-party attribute match, got %#v", results)
	}
}

func TestSimulateAllocationScopesExactSubrequest(t *testing.T) {
	claim := constrainedClaim(
		"constraint-exact-subrequest-scope",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver+"/group", "choice/standard")},
		resourcev1.DeviceRequest{
			Name: "choice",
			FirstAvailable: []resourcev1.DeviceSubRequest{
				{Name: "premium", DeviceClassName: "premium-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
				{Name: "standard", DeviceClassName: "constraint-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
			},
		},
	)
	slice := constraintSlice("subrequest-device", "pool-a", "node-a", resourcev1.Device{Name: "device-0"})

	_, clientset := runAllocation(t, claim, allocationClass("premium-class"), allocationClass("constraint-class"), slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "choice/premium" {
		t.Fatalf("expected exact standard scope not to affect premium, got %q", result.Request)
	}
}

func TestSimulateAllocationScopesParentRequestToSelectedSubrequest(t *testing.T) {
	groupA := "a"
	groupB := "b"
	claim := constrainedClaim(
		"constraint-parent-scope",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver+"/group", "choice")},
		resourcev1.DeviceRequest{
			Name: "choice",
			FirstAvailable: []resourcev1.DeviceSubRequest{{
				Name: "standard", DeviceClassName: "constraint-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 2,
			}},
		},
	)
	slice := constraintSlice(
		"subrequest-devices", "pool-a", "node-a",
		stringConstraintDevice("device-a", "group", groupA),
		stringConstraintDevice("device-b", "group", groupB),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, reasonNoMatch)
}

func TestSimulateAllocationAllRequiresConstraintMatchAcrossEveryDevice(t *testing.T) {
	groupA := "a"
	groupB := "b"
	claim := constrainedClaim(
		"constraint-all",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/group")},
		resourcev1.DeviceRequest{
			Name: "devices",
			Exactly: &resourcev1.ExactDeviceRequest{
				DeviceClassName: "constraint-class",
				AllocationMode:  resourcev1.DeviceAllocationModeAll,
			},
		},
	)
	slice := constraintSlice(
		"all-devices", "pool-a", "node-a",
		stringConstraintDevice("device-a", "group", groupA),
		stringConstraintDevice("device-b", "group", groupB),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, reasonNoMatch)
}

func TestSimulateAllocationRejectsListValuedMatchAttribute(t *testing.T) {
	claim := constrainedClaim(
		"constraint-list-valued",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver + "/group")},
		exactConstraintRequest("devices", 1),
	)
	slice := constraintSlice(
		"list-device", "pool-a", "node-a",
		resourcev1.Device{Name: "device-0", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			"group": {StringValues: []string{"a"}},
		}},
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, reasonUnsupportedRequest)
}

func TestSimulateAllocationRejectsUnknownConstraintRequest(t *testing.T) {
	claim := constrainedClaim(
		"constraint-unknown-request",
		[]resourcev1.DeviceConstraint{matchConstraint(constraintDriver+"/group", "missing")},
		exactConstraintRequest("devices", 1),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"))
	assertPendingWithEvent(t, clientset, claim.Name, reasonUnsupportedRequest)
}

func TestConstraintSearchBudgetFailsClosed(t *testing.T) {
	budget := newConstraintSearchBudget(1)
	if failure := budget.consume(); failure != nil {
		t.Fatalf("first branch unexpectedly exhausted budget: %#v", failure)
	}
	failure := budget.consume()
	if failure == nil || failure.reason != reasonUnsupportedRequest {
		t.Fatalf("expected exhausted search budget to fail closed, got %#v", failure)
	}
}

func TestSimulateAllocationRejectsUnsupportedDistinctAttribute(t *testing.T) {
	name := resourcev1.FullyQualifiedName(constraintDriver + "/group")
	claim := constrainedClaim(
		"constraint-distinct",
		[]resourcev1.DeviceConstraint{{DistinctAttribute: &name}},
		exactConstraintRequest("devices", 1),
	)

	_, clientset := runAllocation(t, claim, allocationClass("constraint-class"))
	assertPendingWithEvent(t, clientset, claim.Name, reasonUnsupportedRequest)
}
