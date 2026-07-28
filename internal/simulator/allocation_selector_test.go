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
			Pool:     resourcev1.ResourcePool{Name: pool},
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

func allocatedResult(t *testing.T, clientset *fake.Clientset, claimName string) *resourcev1.DeviceRequestAllocationResult {
	t.Helper()
	claim, err := clientset.ResourceV1().ResourceClaims("default").Get(context.Background(), claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if claim.Status.Allocation == nil || len(claim.Status.Allocation.Devices.Results) != 1 {
		t.Fatalf("expected one allocation result, got %#v", claim.Status.Allocation)
	}
	return &claim.Status.Allocation.Devices.Results[0]
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
	allocated, err := clientset.ResourceV1().ResourceClaims("default").Get(context.Background(), claim.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if allocated.Status.Allocation == nil {
		t.Fatal("expected a complete two-device allocation on node-b")
	}
	results := allocated.Status.Allocation.Devices.Results
	if len(results) != 2 || results[0].Device != "b-0" || results[1].Device != "b-1" {
		t.Fatalf("expected node-b devices b-0 and b-1, got %#v", results)
	}
	terms := allocated.Status.Allocation.NodeSelector.NodeSelectorTerms
	if len(terms) != 1 || len(terms[0].MatchFields) != 1 || len(terms[0].MatchFields[0].Values) != 1 || terms[0].MatchFields[0].Values[0] != "node-b" {
		t.Fatalf("expected exact node-b selector, got %#v", allocated.Status.Allocation.NodeSelector)
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
			name: "unsupported All mode",
			claim: allocationClaim("all-mode", resourcev1.DeviceRequest{
				Name: "device",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "valid-class",
					AllocationMode:  resourcev1.DeviceAllocationModeAll,
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
