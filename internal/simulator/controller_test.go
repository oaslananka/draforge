// Package simulator unit tests.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReconcile(t *testing.T) {
	ctx := context.Background()

	// 1. Create a fake SimulatedDevicePool Custom Resource
	sdp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "draforge.oaslananka/v1alpha1",
			"kind":       "SimulatedDevicePool",
			"metadata": map[string]interface{}{
				"name":      "gpu-pool",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"driverName":  "sim.draforge.oaslananka",
				"poolName":    "gpu-pool",
				"deviceCount": int64(2),
				"deviceType":  "gpu",
				"targetNodes": []interface{}{"node-0"},
				"health":      "healthy",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme, sdp)
	clientset := fake.NewSimpleClientset()

	reconciler := NewReconciler(clientset, dynamicClient)
	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify ResourceSlice was created
	slices, err := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list ResourceSlices: %v", err)
	}

	if len(slices.Items) != 1 {
		t.Errorf("expected 1 ResourceSlice, got %d", len(slices.Items))
	} else {
		slice := slices.Items[0]
		if slice.Spec.Driver != "sim.draforge.oaslananka" {
			t.Errorf("unexpected driver: %s", slice.Spec.Driver)
		}
		if len(slice.Spec.Devices) != 2 {
			t.Errorf("expected 2 devices in slice, got %d", len(slice.Spec.Devices))
		}
	}
}

func TestSimulateAllocation(t *testing.T) {
	ctx := context.Background()

	// 1. Create a pending ResourceClaim
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-claim",
			Namespace: "default",
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
	}

	// 2. Create a simulator ResourceSlice
	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sim-slice-gpu-pool",
			Labels: map[string]string{
				"draforge.oaslananka/managed-by": "simulator",
				"draforge.oaslananka/health":     "healthy",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "sim.draforge.oaslananka",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "dev-0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim, slice)

	reconciler := NewReconciler(clientset, dynamicClient)
	err := reconciler.SimulateAllocation(ctx)
	if err != nil {
		t.Fatalf("SimulateAllocation failed: %v", err)
	}

	// Verify that claim is now allocated
	allocatedClaim, err := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "pending-claim", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get claim: %v", err)
	}

	if allocatedClaim.Status.Allocation == nil {
		t.Error("expected claim to be allocated, but Status.Allocation is nil")
	} else {
		results := allocatedClaim.Status.Allocation.Devices.Results
		if len(results) != 1 {
			t.Errorf("expected 1 allocation result, got %d", len(results))
		} else if results[0].Device != "dev-0" {
			t.Errorf("expected device dev-0, got %s", results[0].Device)
		}
	}
}
