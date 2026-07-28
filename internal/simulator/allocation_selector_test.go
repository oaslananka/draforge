// Package simulator tests typed DRA allocation selection.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"testing"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func selector(expression string) resourcev1.DeviceSelector {
	return resourcev1.DeviceSelector{
		CEL: &resourcev1.CELDeviceSelector{Expression: expression},
	}
}

func allocationClass(name string, selectors ...resourcev1.DeviceSelector) *resourcev1.DeviceClass {
	return &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       resourcev1.DeviceClassSpec{Selectors: selectors},
	}
}

func allocationClaim(name string, request resourcev1.DeviceRequest) *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{request}},
		},
	}
}

func allocationSlice(name, driver, pool, node string, devices ...resourcev1.Device) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"draforge.oaslananka/managed-by": "simulator",
				"draforge.oaslananka/health":     "healthy",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driver,
			NodeName: &node,
			Pool:     resourcev1.ResourcePool{Name: pool, ResourceSliceCount: 1},
			Devices:  devices,
		},
	}
}

func runAllocation(t *testing.T, objects ...runtime.Object) (*Reconciler, *fake.Clientset) {
	t.Helper()
	clientset := fake.NewSimpleClientset(objects...)
	reconciler := NewReconciler(clientset, dynfake.NewSimpleDynamicClient(runtime.NewScheme()))
	if err := reconciler.SimulateAllocation(context.Background()); err != nil {
		t.Fatalf("SimulateAllocation failed: %v", err)
	}
	return reconciler, clientset
}

func requiredAllocation(t *testing.T, clientset *fake.Clientset, claimName string) *resourcev1.AllocationResult {
	t.Helper()
	claim, err := clientset.ResourceV1().ResourceClaims("default").Get(context.Background(), claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if claim.Status.Allocation == nil {
		t.Fatalf("expected claim %q to be allocated", claimName)
	}
	return claim.Status.Allocation
}

func allocatedResult(t *testing.T, clientset *fake.Clientset, claimName string) *resourcev1.DeviceRequestAllocationResult {
	t.Helper()
	allocation := requiredAllocation(t, clientset, claimName)
	if len(allocation.Devices.Results) != 1 {
		t.Fatalf("expected one allocation result, got %#v", allocation)
	}
	return &allocation.Devices.Results[0]
}

func assertPendingWithEvent(t *testing.T, clientset *fake.Clientset, claimName, reason string) {
	t.Helper()
	claim, err := clientset.ResourceV1().ResourceClaims("default").Get(context.Background(), claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if claim.Status.Allocation != nil {
		t.Fatalf("expected claim to remain pending, got allocation %#v", claim.Status.Allocation)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Millisecond, time.Second, true, func(ctx context.Context) (bool, error) {
		events, listErr := clientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return false, listErr
		}
		for _, event := range events.Items {
			if event.InvolvedObject.Name == claimName && event.Reason == reason {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("expected %s event for claim %s: %v", reason, claimName, err)
	}
}

func TestSimulateAllocationUsesTypedSelectorsInsteadOfProductNames(t *testing.T) {
	wrongKind := "storage"
	rightKind := "accelerator"
	request := resourcev1.DeviceRequest{
		Name: "device",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "gpu-class",
			Selectors: []resourcev1.DeviceSelector{
				selector(`device.capacity["driver.example.com"].memory.compareTo(quantity("8Gi")) >= 0`),
			},
		},
	}
	claim := allocationClaim("typed-selection", request)
	class := allocationClass("gpu-class", selector(`device.attributes["driver.example.com"].kind == "accelerator"`))
	misleading := allocationSlice(
		"misleading",
		"driver.example.com",
		"nvidia-gpu-pool",
		"node-a",
		resourcev1.Device{
			Name:       "wrong-device",
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &wrongKind}},
			Capacity:   map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("32Gi")}},
		},
	)
	neutral := allocationSlice(
		"neutral",
		"driver.example.com",
		"plain-pool",
		"node-a",
		resourcev1.Device{
			Name:       "right-device",
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &rightKind}},
			Capacity:   map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"memory": {Value: resource.MustParse("16Gi")}},
		},
	)

	_, clientset := runAllocation(t, claim, class, misleading, neutral)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Pool != "plain-pool" || result.Device != "right-device" {
		t.Fatalf("expected typed selectors to choose plain-pool/right-device, got %s/%s", result.Pool, result.Device)
	}
}

func TestSimulateAllocationAllSelectsEveryMatchingDeviceOnOneCompleteNode(t *testing.T) {
	accelerator := "accelerator"
	storage := "storage"
	claim := allocationClaim("all-matching", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "accelerator-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
		},
	})
	class := allocationClass(
		"accelerator-class",
		selector(`device.attributes["driver.example.com"].kind == "accelerator"`),
	)
	nodeAFirst := allocationSlice(
		"node-a-first",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "a-0", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &accelerator}}},
		resourcev1.Device{Name: "a-storage", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &storage}}},
	)
	nodeASecond := allocationSlice(
		"node-a-second",
		"driver.example.com",
		"pool-b",
		"node-a",
		resourcev1.Device{Name: "a-1", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &accelerator}}},
	)
	nodeB := allocationSlice(
		"node-b",
		"driver.example.com",
		"pool-c",
		"node-b",
		resourcev1.Device{Name: "b-0", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &accelerator}}},
	)

	_, clientset := runAllocation(t, claim, class, nodeAFirst, nodeASecond, nodeB)
	allocation := requiredAllocation(t, clientset, claim.Name)
	results := allocation.Devices.Results
	if len(results) != 2 || results[0].Device != "a-0" || results[1].Device != "a-1" {
		t.Fatalf("expected every matching node-a device in encounter order, got %#v", results)
	}
	if got := allocation.NodeSelector.NodeSelectorTerms[0].MatchFields[0].Values[0]; got != "node-a" {
		t.Fatalf("expected node-a selector, got %q", got)
	}
}

func TestSimulateAllocationAllSkipsNodeAboveResultLimit(t *testing.T) {
	claim := allocationClaim("all-node-limit", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "neutral-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
		},
	})
	overflowDevices := make([]resourcev1.Device, resourcev1.AllocationResultsMaxSize+1)
	for index := range overflowDevices {
		overflowDevices[index].Name = fmt.Sprintf("overflow-%02d", index)
	}
	overflow := allocationSlice("overflow", "driver.example.com", "overflow-pool", "node-a", overflowDevices...)
	valid := allocationSlice(
		"valid",
		"driver.example.com",
		"valid-pool",
		"node-b",
		resourcev1.Device{Name: "valid-0"},
		resourcev1.Device{Name: "valid-1"},
	)

	_, clientset := runAllocation(t, claim, allocationClass("neutral-class"), overflow, valid)
	results := requiredAllocation(t, clientset, claim.Name).Devices.Results
	if len(results) != 2 || results[0].Device != "valid-0" || results[1].Device != "valid-1" {
		t.Fatalf("expected node-b devices after node-a exceeded the limit, got %#v", results)
	}
}

func TestSimulateAllocationFirstAvailableFallsBackAfterAllExceedsResultLimit(t *testing.T) {
	claim := allocationClaim("all-limit-fallback", resourcev1.DeviceRequest{
		Name: "devices",
		FirstAvailable: []resourcev1.DeviceSubRequest{
			{Name: "all", DeviceClassName: "neutral-class", AllocationMode: resourcev1.DeviceAllocationModeAll},
			{Name: "one", DeviceClassName: "neutral-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
		},
	})
	devices := make([]resourcev1.Device, resourcev1.AllocationResultsMaxSize+1)
	for index := range devices {
		devices[index].Name = fmt.Sprintf("device-%02d", index)
	}
	slice := allocationSlice("devices", "driver.example.com", "pool-a", "node-a", devices...)

	_, clientset := runAllocation(t, claim, allocationClass("neutral-class"), slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "devices/one" {
		t.Fatalf("expected ExactCount fallback after All exceeded the limit, got %q", result.Request)
	}
}

func TestSimulateAllocationFirstAvailableRespectsRemainingClaimResultBudget(t *testing.T) {
	tests := []struct {
		name        string
		alternative resourcev1.DeviceSubRequest
	}{
		{
			name: "All alternative",
			alternative: resourcev1.DeviceSubRequest{
				Name: "too-many", DeviceClassName: "bulk-class", AllocationMode: resourcev1.DeviceAllocationModeAll,
			},
		},
		{
			name: "ExactCount alternative",
			alternative: resourcev1.DeviceSubRequest{
				Name: "too-many", DeviceClassName: "bulk-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: resourcev1.AllocationResultsMaxSize,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claim := &resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "remaining-budget", Namespace: "default"},
				Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{
					{
						Name: "first",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "first-class",
							AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
							Count:           1,
						},
					},
					{
						Name: "bulk",
						FirstAvailable: []resourcev1.DeviceSubRequest{
							test.alternative,
							{Name: "one", DeviceClassName: "bulk-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
						},
					},
				}}},
			}
			firstClass := allocationClass("first-class", selector(`device.driver == "first.example.com"`))
			bulkClass := allocationClass("bulk-class", selector(`device.driver == "bulk.example.com"`))
			firstSlice := allocationSlice("first", "first.example.com", "first-pool", "node-a", resourcev1.Device{Name: "first-0"})
			bulkDevices := make([]resourcev1.Device, resourcev1.AllocationResultsMaxSize)
			for index := range bulkDevices {
				bulkDevices[index].Name = fmt.Sprintf("bulk-%02d", index)
			}
			bulkSlice := allocationSlice("bulk", "bulk.example.com", "bulk-pool", "node-a", bulkDevices...)

			_, clientset := runAllocation(t, claim, firstClass, bulkClass, firstSlice, bulkSlice)
			results := requiredAllocation(t, clientset, claim.Name).Devices.Results
			if len(results) != 2 || results[0].Request != "first" || results[1].Request != "bulk/one" {
				t.Fatalf("expected first + bulk/one allocation, got %#v", results)
			}
		})
	}
}

func TestSimulateAllocationAllUsesLatestCompleteGenerationAndExcludesAllocatedDevices(t *testing.T) {
	claim := allocationClaim("all-current-generation", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "neutral-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
		},
	})
	allocated := allocationClaim("already-allocated", resourcev1.DeviceRequest{
		Name: "existing",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "neutral-class",
		},
	})
	allocated.Status.Allocation = &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{{
			Request: "existing",
			Driver:  "driver.example.com",
			Pool:    "pool-a",
			Device:  "current-allocated",
		}}},
	}
	old := allocationSlice(
		"old-generation",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "old-device"},
	)
	old.Spec.Pool.Generation = 1
	currentFirst := allocationSlice(
		"current-first",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "current-free"},
	)
	currentFirst.Spec.Pool.Generation = 2
	currentFirst.Spec.Pool.ResourceSliceCount = 2
	currentSecond := allocationSlice(
		"current-second",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "current-allocated"},
	)
	currentSecond.Spec.Pool.Generation = 2
	currentSecond.Spec.Pool.ResourceSliceCount = 2

	_, clientset := runAllocation(t, claim, allocated, allocationClass("neutral-class"), old, currentFirst, currentSecond)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Device != "current-free" {
		t.Fatalf("expected only the free device from the latest complete generation, got %#v", result)
	}
}

func TestSimulateAllocationAllRequiresAtLeastOneMatchingDevice(t *testing.T) {
	claim := allocationClaim("all-empty", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "never-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
		},
	})
	class := allocationClass("never-class", selector("false"))
	slice := allocationSlice("devices", "driver.example.com", "pool-a", "node-a", resourcev1.Device{Name: "device-0"})

	_, clientset := runAllocation(t, claim, class, slice)
	assertPendingWithEvent(t, clientset, claim.Name, "SimulationNoMatch")
}

func TestSimulateAllocationAllRejectsIncompletePool(t *testing.T) {
	claim := allocationClaim("all-incomplete", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "neutral-class",
			AllocationMode:  resourcev1.DeviceAllocationModeAll,
		},
	})
	slice := allocationSlice("partial", "driver.example.com", "pool-a", "node-a", resourcev1.Device{Name: "device-0"})
	slice.Spec.Pool.ResourceSliceCount = 2

	_, clientset := runAllocation(t, claim, allocationClass("neutral-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, "SimulationUnsupportedRequest")
}

func TestSimulateAllocationFirstAvailableFallsBackAfterEmptyAll(t *testing.T) {
	claim := allocationClaim("all-fallback", resourcev1.DeviceRequest{
		Name: "devices",
		FirstAvailable: []resourcev1.DeviceSubRequest{
			{Name: "all-premium", DeviceClassName: "premium-class", AllocationMode: resourcev1.DeviceAllocationModeAll},
			{Name: "one-standard", DeviceClassName: "standard-class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1},
		},
	})
	premium := allocationClass("premium-class", selector("false"))
	standard := allocationClass("standard-class", selector("true"))
	slice := allocationSlice("devices", "driver.example.com", "pool-a", "node-a", resourcev1.Device{Name: "device-0"})

	_, clientset := runAllocation(t, claim, premium, standard, slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "devices/one-standard" {
		t.Fatalf("expected ExactCount fallback after empty All, got %q", result.Request)
	}
}

func TestSimulateAllocationEvaluatesFirstAvailableInDeclaredOrder(t *testing.T) {
	tier := "standard"
	claim := allocationClaim("ordered-alternatives", resourcev1.DeviceRequest{
		Name: "accelerator",
		FirstAvailable: []resourcev1.DeviceSubRequest{
			{Name: "premium", DeviceClassName: "premium-class"},
			{Name: "standard", DeviceClassName: "standard-class"},
		},
	})
	premium := allocationClass("premium-class", selector(`device.attributes["driver.example.com"].tier == "premium"`))
	standard := allocationClass("standard-class", selector(`device.attributes["driver.example.com"].tier == "standard"`))
	slice := allocationSlice("devices", "driver.example.com", "plain-pool", "node-a", resourcev1.Device{
		Name:       "device-0",
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"tier": {StringValue: &tier}},
	})

	_, clientset := runAllocation(t, claim, premium, standard, slice)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Request != "accelerator/standard" {
		t.Fatalf("expected second alternative after first selector rejected the device, got %q", result.Request)
	}
}

func TestSimulateAllocationIgnoresUnsupportedFieldsOnSelectorRejectedDevices(t *testing.T) {
	storage := "storage"
	accelerator := "accelerator"
	allNodes := true
	claim := allocationClaim("ignore-unrelated", resourcev1.DeviceRequest{
		Name: "device",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "accelerator-class",
		},
	})
	class := allocationClass(
		"accelerator-class",
		selector(`device.attributes["driver.example.com"].kind == "accelerator"`),
	)
	unrelated := allocationSlice(
		"unrelated",
		"driver.example.com",
		"unrelated-pool",
		"node-a",
		resourcev1.Device{
			Name:       "storage-0",
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &storage}},
		},
	)
	unrelated.Spec.NodeName = nil
	unrelated.Spec.AllNodes = &allNodes
	valid := allocationSlice(
		"valid",
		"driver.example.com",
		"plain-pool",
		"node-a",
		resourcev1.Device{
			Name:       "accelerator-0",
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &accelerator}},
		},
	)

	_, clientset := runAllocation(t, claim, class, unrelated, valid)
	result := allocatedResult(t, clientset, claim.Name)
	if result.Pool != "plain-pool" || result.Device != "accelerator-0" {
		t.Fatalf("expected selector-rejected unsupported slice to be ignored, got %s/%s", result.Pool, result.Device)
	}
}

func TestSimulateAllocationFindsCompleteExactCountOnOneNode(t *testing.T) {
	claim := allocationClaim("same-node-count", resourcev1.DeviceRequest{
		Name: "devices",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "neutral-class",
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           2,
		},
	})
	class := allocationClass("neutral-class")
	nodeA := allocationSlice(
		"node-a-slice",
		"driver.example.com",
		"pool-a",
		"node-a",
		resourcev1.Device{Name: "a-0"},
	)
	nodeB := allocationSlice(
		"node-b-slice",
		"driver.example.com",
		"pool-b",
		"node-b",
		resourcev1.Device{Name: "b-0"},
		resourcev1.Device{Name: "b-1"},
	)

	_, clientset := runAllocation(t, claim, class, nodeA, nodeB)
	allocation := requiredAllocation(t, clientset, claim.Name)
	results := allocation.Devices.Results
	if len(results) != 2 || results[0].Device != "b-0" || results[1].Device != "b-1" {
		t.Fatalf("expected node-b devices b-0 and b-1, got %#v", results)
	}
	terms := allocation.NodeSelector.NodeSelectorTerms
	if len(terms) != 1 || len(terms[0].MatchFields) != 1 || len(terms[0].MatchFields[0].Values) != 1 || terms[0].MatchFields[0].Values[0] != "node-b" {
		t.Fatalf("expected exact node-b selector, got %#v", allocation.NodeSelector)
	}
}

func TestSimulateAllocationRejectsClaimAboveKubernetesResultLimit(t *testing.T) {
	requests := []resourcev1.DeviceRequest{
		{
			Name: "first",
			Exactly: &resourcev1.ExactDeviceRequest{
				DeviceClassName: "neutral-class",
				AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
				Count:           17,
			},
		},
		{
			Name: "second",
			Exactly: &resourcev1.ExactDeviceRequest{
				DeviceClassName: "neutral-class",
				AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
				Count:           17,
			},
		},
	}
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-result-limit", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{Requests: requests},
		},
	}
	devices := make([]resourcev1.Device, 34)
	for index := range devices {
		devices[index].Name = fmt.Sprintf("device-%02d", index)
	}
	slice := allocationSlice("devices", "driver.example.com", "plain-pool", "node-a", devices...)

	_, clientset := runAllocation(t, claim, allocationClass("neutral-class"), slice)
	assertPendingWithEvent(t, clientset, claim.Name, "SimulationUnsupportedRequest")
}

func TestSimulateAllocationFailsClosedForInvalidContracts(t *testing.T) {
	tests := []struct {
		name        string
		claim       *resourcev1.ResourceClaim
		objects     []runtime.Object
		eventReason string
	}{
		{
			name: "missing DeviceClass",
			claim: allocationClaim("missing-class", resourcev1.DeviceRequest{
				Name:    "device",
				Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "does-not-exist"},
			}),
			eventReason: "SimulationDeviceClassNotFound",
		},
		{
			name: "invalid request CEL",
			claim: allocationClaim("invalid-cel", resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "valid-class",
					Selectors:       []resourcev1.DeviceSelector{selector("?!")},
				},
			}),
			objects:     []runtime.Object{allocationClass("valid-class")},
			eventReason: "SimulationSelectorError",
		},
		{
			name: "count exceeds Kubernetes allocation result limit",
			claim: allocationClaim("oversized-count", resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "valid-class",
					AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
					Count:           resourcev1.AllocationResultsMaxSize + 1,
				},
			}),
			objects:     []runtime.Object{allocationClass("valid-class")},
			eventReason: "SimulationUnsupportedRequest",
		},
		{
			name: "All mode with nonzero count",
			claim: allocationClaim("all-with-count", resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "valid-class",
					AllocationMode:  resourcev1.DeviceAllocationModeAll,
					Count:           1,
				},
			}),
			objects:     []runtime.Object{allocationClass("valid-class")},
			eventReason: "SimulationUnsupportedRequest",
		},
		{
			name: "unknown allocation mode",
			claim: allocationClaim("unknown-mode", resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "valid-class",
					AllocationMode:  resourcev1.DeviceAllocationMode("FutureMode"),
				},
			}),
			objects:     []runtime.Object{allocationClass("valid-class")},
			eventReason: "SimulationUnsupportedRequest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := "accelerator"
			slice := allocationSlice("devices", "driver.example.com", "plain-pool", "node-a", resourcev1.Device{
				Name:       "device-0",
				Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{"kind": {StringValue: &kind}},
			})
			objects := []runtime.Object{test.claim, slice}
			objects = append(objects, test.objects...)
			_, clientset := runAllocation(t, objects...)
			assertPendingWithEvent(t, clientset, test.claim.Name, test.eventReason)
		})
	}
}
