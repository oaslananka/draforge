// Package simulator tests consumable-capacity planner and status integration.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func shareableDevice(
	name string,
	values map[resourcev1.QualifiedName]string,
	policies map[resourcev1.QualifiedName]*resourcev1.CapacityRequestPolicy,
) resourcev1.Device {
	device := capacityDevice(name, values)
	allowMultiple := true
	device.AllowMultipleAllocations = &allowMultiple
	for capacityName, policy := range policies {
		capacity := device.Capacity[capacityName]
		capacity.RequestPolicy = policy
		device.Capacity[capacityName] = capacity
	}
	return device
}

func validValuesPolicy(defaultValue string, values ...string) *resourcev1.CapacityRequestPolicy {
	defaultQuantity := quantity(defaultValue)
	validValues := make([]resource.Quantity, len(values))
	for index, value := range values {
		validValues[index] = quantity(value)
	}
	return &resourcev1.CapacityRequestPolicy{Default: &defaultQuantity, ValidValues: validValues}
}

func allocatedShareClaim(
	name, requestName, driver, pool, device, shareID string,
	consumed map[resourcev1.QualifiedName]string,
) *resourcev1.ResourceClaim {
	uid := types.UID(shareID)
	result := resourcev1.DeviceRequestAllocationResult{
		Request:          requestName,
		Driver:           driver,
		Pool:             pool,
		Device:           device,
		ShareID:          &uid,
		ConsumedCapacity: quantityMap(consumed),
	}
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{result}},
			},
		},
	}
}

func assertRejectedShareState(
	t *testing.T,
	claimName string,
	existing []*resourcev1.ResourceClaim,
	request resourcev1.DeviceRequest,
	device resourcev1.Device,
	fragment string,
) {
	t.Helper()
	objects := make([]runtime.Object, 0, len(existing)+3)
	for _, claim := range existing {
		objects = append(objects, claim)
	}
	pending := allocationClaim(claimName, request)
	objects = append(objects,
		pending,
		allocationClass("share-class"),
		allocationSlice("share-slice", "driver.example.com", "share-pool", "node-a", device),
	)
	_, clientset := runAllocation(t, objects...)
	assertPendingWithEventContaining(t, clientset, claimName, reasonUnsupportedRequest, fragment)
}

func requireShareResult(
	t *testing.T,
	result resourcev1.DeviceRequestAllocationResult,
	expectedDevice string,
	expected map[resourcev1.QualifiedName]string,
) {
	t.Helper()
	requireShareIdentity(t, result, expectedDevice)
	requireConsumedCapacity(t, result, expected)
}

func requireShareIdentity(
	t *testing.T,
	result resourcev1.DeviceRequestAllocationResult,
	expectedDevice string,
) {
	t.Helper()
	if result.Device != expectedDevice {
		t.Fatalf("expected device %q, got %#v", expectedDevice, result)
	}
	if result.ShareID == nil || !uuidPattern.MatchString(string(*result.ShareID)) {
		t.Fatalf("expected Kubernetes UUID shareID, got %#v", result.ShareID)
	}
}

func requireConsumedCapacity(
	t *testing.T,
	result resourcev1.DeviceRequestAllocationResult,
	expected map[resourcev1.QualifiedName]string,
) {
	t.Helper()
	if result.ConsumedCapacity == nil {
		t.Fatalf("expected consumedCapacity, got %#v", result)
	}
	for name, value := range expected {
		requireQuantity(t, result.ConsumedCapacity, name, value)
	}
}

func TestSimulateAllocationWritesRoundedShareStatus(t *testing.T) {
	claim := allocationClaim("share-rounded", exactCapacityRequest(
		"device", "share-class", 1,
		map[resourcev1.QualifiedName]string{"shares": "3"},
	))
	device := shareableDevice(
		"shared-device",
		map[resourcev1.QualifiedName]string{"shares": "8", "memory": "16Gi"},
		map[resourcev1.QualifiedName]*resourcev1.CapacityRequestPolicy{
			"shares": validValuesPolicy("2", "2", "4", "8"),
		},
	)

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	result := allocatedResult(t, clientset, claim.Name)
	requireShareResult(t, *result, "shared-device", map[resourcev1.QualifiedName]string{
		"shares": "4",
		"memory": "16Gi",
	})
}

func TestSimulateAllocationUsesDefaultAndFullCapacityWhenRequestIsOmitted(t *testing.T) {
	for name, testCase := range map[string]struct {
		policy   *resourcev1.CapacityRequestPolicy
		expected string
	}{
		"policy default": {policy: validValuesPolicy("2", "2", "4", "8"), expected: "2"},
		"full capacity":  {expected: "8"},
	} {
		t.Run(name, func(t *testing.T) {
			claim := allocationClaim("share-default-"+strings.ReplaceAll(name, " ", "-"), resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "share-class",
					AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
					Count:           1,
				},
			})
			policies := map[resourcev1.QualifiedName]*resourcev1.CapacityRequestPolicy{}
			if testCase.policy != nil {
				policies["shares"] = testCase.policy
			}
			device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, policies)

			_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
				"share-slice", "driver.example.com", "share-pool", "node-a", device,
			))
			result := allocatedResult(t, clientset, claim.Name)
			requireShareResult(t, *result, "shared-device", map[resourcev1.QualifiedName]string{"shares": testCase.expected})
		})
	}
}

func TestSimulateAllocationSharesOneDeviceAcrossClaims(t *testing.T) {
	claimA := allocationClaim("share-a", exactCapacityRequest("device", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}))
	claimB := allocationClaim("share-b", exactCapacityRequest("device", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}))
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, claimA, claimB, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	resultA := allocatedResult(t, clientset, claimA.Name)
	resultB := allocatedResult(t, clientset, claimB.Name)
	requireShareResult(t, *resultA, "shared-device", map[resourcev1.QualifiedName]string{"shares": "4"})
	requireShareResult(t, *resultB, "shared-device", map[resourcev1.QualifiedName]string{"shares": "4"})
	if *resultA.ShareID == *resultB.ShareID {
		t.Fatalf("expected unique share IDs, got %q", *resultA.ShareID)
	}
}

func TestSimulateAllocationSharesOneDeviceAcrossRequests(t *testing.T) {
	claim := constrainedClaim(
		"share-two-requests",
		nil,
		exactCapacityRequest("first", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}),
		exactCapacityRequest("second", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}),
	)
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 {
		t.Fatalf("expected two allocation shares, got %#v", results)
	}
	for _, result := range results {
		requireShareResult(t, result, "shared-device", map[resourcev1.QualifiedName]string{"shares": "4"})
	}
	if *results[0].ShareID == *results[1].ShareID {
		t.Fatalf("expected unique share IDs, got %q", *results[0].ShareID)
	}
}

func TestSimulateAllocationFallsBackAfterShareCapacityExhaustion(t *testing.T) {
	existing := allocatedShareClaim(
		"existing", "device", "driver.example.com", "share-pool", "device-a",
		"11111111-1111-4111-8111-111111111111",
		map[resourcev1.QualifiedName]string{"shares": "6"},
	)
	claim := allocationClaim("share-fallback", exactCapacityRequest(
		"device", "share-class", 1,
		map[resourcev1.QualifiedName]string{"shares": "4"},
	))
	deviceA := shareableDevice("device-a", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)
	deviceB := shareableDevice("device-b", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, existing, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", deviceA, deviceB,
	))
	result := allocatedResult(t, clientset, claim.Name)
	requireShareResult(t, *result, "device-b", map[resourcev1.QualifiedName]string{"shares": "4"})
}

func TestSimulateAllocationFirstAvailableFallsBackAfterShareExhaustion(t *testing.T) {
	existing := allocatedShareClaim(
		"existing", "device", "driver.example.com", "share-pool", "shared-device",
		"11111111-1111-4111-8111-111111111111",
		map[resourcev1.QualifiedName]string{"shares": "6"},
	)
	claim := allocationClaim("share-first-available", firstAvailableCapacityRequest(
		"device", "share-class", "shares", "large", "4", "small", "2",
	))
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, existing, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "device/small" {
		t.Fatalf("expected capacity-compatible subrequest fallback, got %#v", result)
	}
	requireShareResult(t, *result, "shared-device", map[resourcev1.QualifiedName]string{"shares": "2"})
}

func TestSimulateAllocationSharesDeviceUnderMatchAttributeConstraint(t *testing.T) {
	group := "shared"
	claim := constrainedClaim(
		"share-constraint",
		[]resourcev1.DeviceConstraint{matchConstraint("driver.example.com/group")},
		exactCapacityRequest("first", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}),
		exactCapacityRequest("second", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}),
	)
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)
	device.Attributes = map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"group": {StringValue: &group}}

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "shared-device" || results[1].Device != "shared-device" {
		t.Fatalf("expected two constrained shares of one device, got %#v", results)
	}
}

func TestSimulateAllocationBacktracksPendingShareConsumption(t *testing.T) {
	claim := constrainedClaim(
		"share-backtracking",
		nil,
		exactCapacityRequest("flexible", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "4"}),
		resourcev1.DeviceRequest{
			Name: "large-only",
			Exactly: &resourcev1.ExactDeviceRequest{
				DeviceClassName: "share-class",
				AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
				Count:           1,
				Selectors:       []resourcev1.DeviceSelector{selector(`device.attributes["driver.example.com"].tier == "large"`)},
				Capacity:        capacityRequirements(map[resourcev1.QualifiedName]string{"shares": "6"}),
			},
		},
	)
	largeTier := "large"
	smallTier := "small"
	deviceA := shareableDevice("large-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)
	deviceA.Attributes = map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"tier": {StringValue: &largeTier}}
	deviceB := shareableDevice("small-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)
	deviceB.Attributes = map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"tier": {StringValue: &smallTier}}

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", deviceA, deviceB,
	))
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "small-device" || results[1].Device != "large-device" {
		t.Fatalf("expected capacity backtracking to preserve the large device, got %#v", results)
	}
}

func TestSimulateAllocationExactCountDoesNotDuplicateOnePhysicalDevice(t *testing.T) {
	claim := allocationClaim("share-exact-count", exactCapacityRequest(
		"devices", "share-class", 2,
		map[resourcev1.QualifiedName]string{"shares": "2"},
	))
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	assertPendingWithEvent(t, clientset, claim.Name, reasonNoMatch)
}

func TestSimulateAllocationAllCreatesOneSharePerPhysicalDevice(t *testing.T) {
	claim := allocationClaim("share-all", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "share-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
			Capacity:        capacityRequirements(map[resourcev1.QualifiedName]string{"shares": "2"}),
		},
	})
	deviceA := shareableDevice("device-a", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)
	deviceB := shareableDevice("device-b", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", deviceA, deviceB,
	))
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "device-a" || results[1].Device != "device-b" {
		t.Fatalf("expected one share per physical device, got %#v", results)
	}
}

func TestSimulateAllocationRejectsShareableDeviceWithoutCapacity(t *testing.T) {
	claim := allocationClaim("share-no-capacity", resourcev1.DeviceRequest{
		Name: "device",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "share-class",
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           1,
		},
	})
	device := shareableDevice("shared-device", nil, nil)

	_, clientset := runAllocation(t, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	assertPendingWithEventContaining(t, clientset, claim.Name, reasonUnsupportedRequest, "no consumable capacities")
}

func TestSimulateAllocationRejectsMalformedExistingShareStatus(t *testing.T) {
	uid := types.UID("11111111-1111-4111-8111-111111111111")
	existing := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "default"},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{{
					Request: "device", Driver: "driver.example.com", Pool: "share-pool", Device: "shared-device", ShareID: &uid,
				}}},
			},
		},
	}
	claim := allocationClaim("share-malformed", exactCapacityRequest(
		"device", "share-class", 1,
		map[resourcev1.QualifiedName]string{"shares": "2"},
	))
	device := shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil)

	_, clientset := runAllocation(t, existing, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	assertPendingWithEventContaining(t, clientset, claim.Name, reasonUnsupportedRequest, "ShareID and ConsumedCapacity")
}

func TestSimulateAllocationRejectsInvalidExistingShareState(t *testing.T) {
	tests := map[string]struct {
		shareID   string
		consumed  string
		requested string
		claimName string
		message   string
	}{
		"invalid share ID": {
			shareID: "NOT-A-UUID", consumed: "2", requested: "2",
			claimName: "share-invalid-uid", message: "invalid ShareID",
		},
		"overcommitted consumption": {
			shareID: "11111111-1111-4111-8111-111111111111", consumed: "10", requested: "1",
			claimName: "share-overcommitted", message: "exceeds device capacity",
		},
	}
	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			existing := allocatedShareClaim(
				"existing", "device", "driver.example.com", "share-pool", "shared-device",
				testCase.shareID, map[resourcev1.QualifiedName]string{"shares": testCase.consumed},
			)
			assertRejectedShareState(
				t, testCase.claimName, []*resourcev1.ResourceClaim{existing},
				exactCapacityRequest("device", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": testCase.requested}),
				shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil),
				testCase.message,
			)
		})
	}
}

func TestSimulateAllocationRejectsDuplicateExistingShareID(t *testing.T) {
	shareID := "11111111-1111-4111-8111-111111111111"
	existingA := allocatedShareClaim("existing-a", "device", "driver.example.com", "share-pool", "shared-device", shareID, map[resourcev1.QualifiedName]string{"shares": "2"})
	existingB := allocatedShareClaim("existing-b", "device", "driver.example.com", "share-pool", "shared-device", shareID, map[resourcev1.QualifiedName]string{"shares": "2"})
	assertRejectedShareState(
		t, "share-duplicate-uid", []*resourcev1.ResourceClaim{existingA, existingB},
		exactCapacityRequest("device", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "2"}),
		shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8"}, nil),
		"duplicate ShareID",
	)
}

func TestSimulateAllocationRejectsIncompleteExistingConsumption(t *testing.T) {
	existing := allocatedShareClaim(
		"existing", "device", "driver.example.com", "share-pool", "shared-device",
		"11111111-1111-4111-8111-111111111111", map[resourcev1.QualifiedName]string{"shares": "2"},
	)
	assertRejectedShareState(
		t, "share-incomplete-consumption", []*resourcev1.ResourceClaim{existing},
		exactCapacityRequest("device", "share-class", 1, map[resourcev1.QualifiedName]string{"shares": "2"}),
		shareableDevice("shared-device", map[resourcev1.QualifiedName]string{"shares": "8", "memory": "16Gi"}, nil),
		"does not report consumed capacity",
	)
}

func TestSimulateAllocationRejectsSharedStatusForExclusiveDevice(t *testing.T) {
	existing := allocatedShareClaim(
		"existing", "device", "driver.example.com", "share-pool", "exclusive-device",
		"11111111-1111-4111-8111-111111111111",
		map[resourcev1.QualifiedName]string{},
	)
	claim := allocationClaim("share-exclusive-conflict", resourcev1.DeviceRequest{
		Name: "device",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "share-class",
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           1,
		},
	})
	device := resourcev1.Device{Name: "exclusive-device"}

	_, clientset := runAllocation(t, existing, claim, allocationClass("share-class"), allocationSlice(
		"share-slice", "driver.example.com", "share-pool", "node-a", device,
	))
	assertPendingWithEventContaining(t, clientset, claim.Name, reasonUnsupportedRequest, "exclusive but has existing shared consumption")
}

func assertPendingWithEventContaining(
	t *testing.T,
	clientset *fake.Clientset,
	claimName, reason, fragment string,
) {
	t.Helper()
	requirePendingClaim(t, clientset, claimName)

	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Millisecond, time.Second, true, func(ctx context.Context) (bool, error) {
		events, listErr := clientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return false, listErr
		}
		for _, event := range events.Items {
			if event.InvolvedObject.Name == claimName && event.Reason == reason && strings.Contains(event.Message, fragment) {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("expected %s event containing %q for claim %s: %v", reason, fragment, claimName, err)
	}
}
