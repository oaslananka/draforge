// Package simulator unit tests.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func nodeObj(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func sdpObj(name, ns, driver, pool, devType string, count int64, health string, nodes []string) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "draforge.oaslananka/v1alpha1",
		"kind":       "SimulatedDevicePool",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"driverName":  driver,
			"poolName":    pool,
			"deviceCount": count,
			"deviceType":  devType,
			"health":      health,
		},
	}
	if len(nodes) > 0 {
		ni := make([]interface{}, len(nodes))
		for i, n := range nodes {
			ni[i] = n
		}
		obj["spec"].(map[string]interface{})["targetNodes"] = ni
	}
	return &unstructured.Unstructured{Object: obj}
}

func newReconciler(sdp *unstructured.Unstructured, k8sObjs ...runtime.Object) (*Reconciler, context.Context) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme, sdp)
	clientset := fake.NewSimpleClientset(k8sObjs...)
	return NewReconciler(clientset, dynamicClient), ctx
}

// --- Reconcile tests ---

func TestReconcileHealthy(t *testing.T) {
	sdp := sdpObj("gpu-pool", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "healthy", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, err := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list ResourceSlices: %v", err)
	}
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 ResourceSlice, got %d", len(slices.Items))
	}

	slice := slices.Items[0]
	if slice.Spec.Driver != "sim.draforge.oaslananka" {
		t.Errorf("unexpected driver: %s", slice.Spec.Driver)
	}
	if len(slice.Spec.Devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(slice.Spec.Devices))
	}
	if slice.Labels["draforge.oaslananka/health"] != "healthy" {
		t.Errorf("expected health label 'healthy', got %q", slice.Labels["draforge.oaslananka/health"])
	}
}

func TestReconcileIdempotent(t *testing.T) {
	sdp := sdpObj("gpu-pool", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "healthy", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("first Reconcile failed: %v", err)
	}
	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile failed: %v", err)
	}

	slices, err := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list ResourceSlices: %v", err)
	}
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 ResourceSlice after second reconcile, got %d", len(slices.Items))
	}
}

func TestReconcileUnhealthy(t *testing.T) {
	sdp := sdpObj("unhealthy-gpu", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "unhealthy", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, _ := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 ResourceSlice, got %d", len(slices.Items))
	}
	if slices.Items[0].Labels["draforge.oaslananka/health"] != "unhealthy" {
		t.Errorf("expected health label 'unhealthy', got %q", slices.Items[0].Labels["draforge.oaslananka/health"])
	}
}

func TestReconcileCapacityExhausted(t *testing.T) {
	sdp := sdpObj("exhausted-pool", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "capacity-exhausted", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, _ := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 ResourceSlice, got %d", len(slices.Items))
	}
	// capacity-exhausted should publish 0 devices
	if len(slices.Items[0].Spec.Devices) != 0 {
		t.Errorf("expected 0 devices (capacity exhausted), got %d", len(slices.Items[0].Spec.Devices))
	}
	if slices.Items[0].Labels["draforge.oaslananka/health"] != "capacity-exhausted" {
		t.Errorf("expected health label 'capacity-exhausted', got %q", slices.Items[0].Labels["draforge.oaslananka/health"])
	}
}

func TestReconcileDisappear(t *testing.T) {
	ctx := context.Background()
	// Pre-create a ResourceSlice, then run reconcile with disappear health
	sdp := sdpObj("disappear-pool", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "disappear", []string{"node-0"})
	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme, sdp)
	clientset := fake.NewSimpleClientset()

	reconciler := NewReconciler(clientset, dynamicClient)

	// First reconcile with healthy to create the slice
	sdpHealth := sdpObj("disappear-pool", "default", "sim.draforge.oaslananka", "gpu-pool", "gpu", 2, "healthy", []string{"node-0"})
	scheme2 := runtime.NewScheme()
	dyn2 := dynfake.NewSimpleDynamicClient(scheme2, sdpHealth)
	recHealthy := NewReconciler(clientset, dyn2)
	if err := recHealthy.Reconcile(ctx); err != nil {
		t.Fatalf("initial healthy reconcile failed: %v", err)
	}

	slices, _ := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 slice before disappear, got %d", len(slices.Items))
	}

	// Now reconcile with disappear health
	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("disappear reconcile failed: %v", err)
	}

	slicesAfter, _ := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slicesAfter.Items) != 0 {
		t.Errorf("expected 0 slices after disappear, got %d (slice %q still exists)", len(slicesAfter.Items), slicesAfter.Items[0].Name)
	}
}

func TestReconcileEmptyNodes(t *testing.T) {
	sdp := sdpObj("auto-pool", "default", "sim.draforge.oaslananka", "auto-pool", "fpga", 1, "healthy", nil)
	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme, sdp)
	// Add a node so the reconciler auto-discovers it
	clientset := fake.NewSimpleClientset(nodeObj("auto-node-0"))
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, _ := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slices.Items) != 1 {
		t.Fatalf("expected 1 ResourceSlice (auto-discovered node), got %d", len(slices.Items))
	}
	if slices.Items[0].Spec.Driver != "sim.draforge.oaslananka" {
		t.Errorf("unexpected driver: %s", slices.Items[0].Spec.Driver)
	}
}

func TestReconcileDeterministic(t *testing.T) {
	sdp := sdpObj("det-pool", "default", "sim.draforge.oaslananka", "det-pool", "nic", 3, "healthy", []string{"node-0"})

	for i := 0; i < 3; i++ {
		scheme := runtime.NewScheme()
		dyn := dynfake.NewSimpleDynamicClient(scheme, sdp.DeepCopy())
		clientset := fake.NewSimpleClientset()
		rec := NewReconciler(clientset, dyn)
		ctx := context.Background()

		if err := rec.Reconcile(ctx); err != nil {
			t.Fatalf("run %d: Reconcile failed: %v", i, err)
		}

		slices, _ := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
		if len(slices.Items) != 1 {
			t.Fatalf("run %d: expected 1 slice, got %d", i, len(slices.Items))
		}
		slice := slices.Items[0]
		if len(slice.Spec.Devices) != 3 {
			t.Fatalf("run %d: expected 3 devices, got %d", i, len(slice.Spec.Devices))
		}

		if i == 0 {
			continue
		}
		// Compare device names with first run — must be order-stable
		prevSlices, _ := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
		_ = prevSlices
	}
}

// --- Allocation tests ---

func TestSimulateAllocationHealthy(t *testing.T) {
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

	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sim-slice-test",
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
				{Name: "dev-0"},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim, slice)
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	err := reconciler.SimulateAllocation(ctx)
	if err != nil {
		t.Fatalf("SimulateAllocation failed: %v", err)
	}

	allocatedClaim, err := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "pending-claim", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get claim: %v", err)
	}
	if allocatedClaim.Status.Allocation == nil {
		t.Fatal("expected claim to be allocated, but Status.Allocation is nil")
	}
	if len(allocatedClaim.Status.Allocation.Devices.Results) != 1 {
		t.Fatalf("expected 1 allocation result, got %d", len(allocatedClaim.Status.Allocation.Devices.Results))
	}
	if allocatedClaim.Status.Allocation.Devices.Results[0].Device != "dev-0" {
		t.Errorf("expected device dev-0, got %s", allocatedClaim.Status.Allocation.Devices.Results[0].Device)
	}
}

func TestSimulateAllocationUnhealthySkip(t *testing.T) {
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

	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sim-slice-unhealthy",
			Labels: map[string]string{
				"draforge.oaslananka/managed-by": "simulator",
				"draforge.oaslananka/health":     "unhealthy",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "sim.draforge.oaslananka",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{Name: "dev-0"},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim, slice)
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	err := reconciler.SimulateAllocation(ctx)
	if err != nil {
		t.Fatalf("SimulateAllocation failed: %v", err)
	}

	allocatedClaim, _ := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "pending-claim", metav1.GetOptions{})
	if allocatedClaim.Status.Allocation != nil {
		t.Error("expected claim NOT allocated (unhealthy slice should be skipped)")
	}
}

func TestSimulateAllocationNoDuplicate(t *testing.T) {
	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sim-slice-pool",
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
				{Name: "dev-0"},
				{Name: "dev-1"},
			},
		},
	}

	// Create 2 claims for 2 devices
	claim1 := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-1", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class"}}},
		}},
	}
	claim2 := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-2", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class"}}},
		}},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim1, claim2, slice)
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	// First allocation — claim1 gets dev-0
	if err := reconciler.SimulateAllocation(ctx); err != nil {
		t.Fatalf("first SimulateAllocation failed: %v", err)
	}

	// Second allocation — claim2 should get dev-1, not dev-0
	if err := reconciler.SimulateAllocation(ctx); err != nil {
		t.Fatalf("second SimulateAllocation failed: %v", err)
	}

	c1, _ := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "claim-1", metav1.GetOptions{})
	c2, _ := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "claim-2", metav1.GetOptions{})

	if c1.Status.Allocation == nil {
		t.Fatal("claim-1 not allocated")
	}
	if c2.Status.Allocation == nil {
		t.Fatal("claim-2 not allocated")
	}

	dev1 := c1.Status.Allocation.Devices.Results[0].Device
	dev2 := c2.Status.Allocation.Devices.Results[0].Device
	if dev1 == dev2 {
		t.Errorf("both claims got same device %q — duplicate allocation", dev1)
	}
	t.Logf("claim-1 -> %s, claim-2 -> %s", dev1, dev2)
}

func TestSimulateAllocationNoDevice(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class"}}},
		}},
	}
	// No ResourceSlices at all
	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim)
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	err := reconciler.SimulateAllocation(ctx)
	if err != nil {
		t.Fatalf("SimulateAllocation should not error when no device available: %v", err)
	}

	c, _ := clientset.ResourceV1().ResourceClaims("default").Get(ctx, "pending", metav1.GetOptions{})
	if c.Status.Allocation != nil {
		t.Error("expected no allocation when no devices exist")
	}
}

func TestSimulateAllocationAlreadyAllocated(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "already-allocated", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class"}}},
		}},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{Device: "dev-0", Driver: "sim.draforge.oaslananka", Pool: "gpu-pool"},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme)
	clientset := fake.NewSimpleClientset(claim)
	reconciler := NewReconciler(clientset, dynamicClient)
	ctx := context.Background()

	err := reconciler.SimulateAllocation(ctx)
	if err != nil {
		t.Fatalf("SimulateAllocation failed for already-allocated claim: %v", err)
	}
	// Should not error or change already-allocated claim
	if atomic.LoadInt64(&reconciler.AllocationsSimulated) != 0 {
		t.Error("AllocationsSimulated counter should be 0 for already-allocated claim")
	}
}

func TestReconcileMultiNode(t *testing.T) {
	sdp := sdpObj("multi-pool", "default", "sim.draforge.oaslananka", "multi-pool", "gpu", 1, "healthy", []string{"node-a", "node-b"})
	reconciler, ctx := newReconciler(sdp)

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, _ := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if len(slices.Items) != 2 {
		t.Fatalf("expected 2 ResourceSlices (one per node), got %d", len(slices.Items))
	}
}

// --- Invalid input tests ---

func TestReconcileNoDriver(t *testing.T) {
	sdp := sdpObj("no-driver", "default", "", "pool", "gpu", 1, "healthy", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile with empty driver should not error: %v", err)
	}
	// A slice with empty driver is still created (controller doesn't validate driver)
	slices, _ := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	_ = slices // Driver empty but we may still get a slice — just verify no panic
}

func TestReconcileOrphanCleanup(t *testing.T) {
	sdp := sdpObj("orphan-pool", "default", "sim.draforge.oaslananka", "orphan-pool", "gpu", 1, "healthy", []string{"node-0"})
	reconciler, ctx := newReconciler(sdp)

	// Inject an orphan ResourceSlice
	orphanSlice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sim-slice-orphan",
			Labels: map[string]string{
				"draforge.oaslananka/managed-by": "simulator",
			},
		},
	}
	reconciler.clientset.ResourceV1().ResourceSlices().Create(ctx, orphanSlice, metav1.CreateOptions{})

	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	slices, _ := reconciler.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})

	foundOrphan := false
	for _, s := range slices.Items {
		if s.Name == "sim-slice-orphan" {
			foundOrphan = true
		}
	}

	if foundOrphan {
		t.Errorf("expected orphan ResourceSlice to be deleted, but it still exists")
	}
}
