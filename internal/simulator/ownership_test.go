// Package simulator tests deterministic ResourceSlice ownership and finalization.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"errors"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func ownedSDP(name string, nodes ...string) *unstructured.Unstructured {
	sdp := sdpObj(name, "default", "sim.draforge.oaslananka", name, "gpu", 1, "healthy", nodes)
	sdp.SetUID(types.UID("uid-" + name))
	return sdp
}

func TestReconcileAddsCleanupFinalizerAndOwnershipLabels(t *testing.T) {
	sdp := ownedSDP("owned-pool", "node-a")
	reconciler, ctx := newReconciler(sdp)

	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile owned pool: %v", err)
	}

	stored, err := reconciler.dynamicClient.Resource(sdpGVR).Namespace("default").Get(
		ctx,
		sdp.GetName(),
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get SimulatedDevicePool: %v", err)
	}
	if !containsString(stored.GetFinalizers(), resourceSliceCleanupFinalizer) {
		t.Fatalf("cleanup finalizer missing from %#v", stored.GetFinalizers())
	}

	slice, err := reconciler.clientset.ResourceV1().ResourceSlices().Get(
		ctx,
		"sim-slice-owned-pool-node-a-default",
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get ResourceSlice: %v", err)
	}
	requireResourceSliceOwnerLabels(t, slice, stored)
	if len(slice.OwnerReferences) != 0 {
		t.Fatalf("cluster-scoped ResourceSlice must not reference namespaced owner: %#v", slice.OwnerReferences)
	}
}

func TestReconcileAdoptsLegacyResourceSliceLabels(t *testing.T) {
	sdp := ownedSDP("legacy-pool", "node-a")
	legacy := allocationSlice(
		"sim-slice-legacy-pool-node-a-default",
		"sim.draforge.oaslananka",
		"legacy-pool",
		"node-a",
		resourcev1.Device{Name: "dev-0"},
	)
	legacy.Labels[resourceSliceSDPNameLabel] = sdp.GetName()
	reconciler, ctx := newReconciler(sdp, legacy)

	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile legacy slice: %v", err)
	}
	adopted, err := reconciler.clientset.ResourceV1().ResourceSlices().Get(ctx, legacy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get adopted ResourceSlice: %v", err)
	}
	requireResourceSliceOwnerLabels(t, adopted, sdp)
}

func dynamicClientWithMainSDPUpdateFailure(
	sdp *unstructured.Unstructured,
	failure error,
) *dynfake.FakeDynamicClient {
	client := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	client.PrependReactor("update", "simulateddevicepools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "" {
			return false, nil, nil
		}
		return true, nil, failure
	})
	return client
}

func TestReconcilePropagatesFinalizerUpdateConflict(t *testing.T) {
	sdp := ownedSDP("finalizer-conflict", "node-a")
	conflict := apierrors.NewConflict(sdpGVR.GroupResource(), sdp.GetName(), errors.New("changed concurrently"))
	dynamicClient := dynamicClientWithMainSDPUpdateFailure(sdp, conflict)
	reconciler := NewReconciler(fake.NewSimpleClientset(), dynamicClient)

	err := reconciler.Reconcile(context.Background())
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected wrapped finalizer Conflict, got %v", err)
	}
	if disposition := classifyControllerError(err, 0); disposition != controllerErrorRetry {
		t.Fatalf("expected retry disposition, got %q", disposition)
	}
}

func TestFinalizeDeletingSDPDeletesOwnedAndLegacySlices(t *testing.T) {
	sdp := ownedSDP("deleting-pool", "node-a", "node-b")
	now := metav1.Now()
	sdp.SetDeletionTimestamp(&now)
	sdp.SetFinalizers([]string{resourceSliceCleanupFinalizer})
	owned := ownershipSlice("sim-slice-deleting-pool-node-a-default", sdp)
	legacy := allocationSlice(
		"sim-slice-deleting-pool-node-b-default",
		"sim.draforge.oaslananka",
		"deleting-pool",
		"node-b",
	)
	legacy.Labels[resourceSliceSDPNameLabel] = sdp.GetName()
	foreign := ownershipSlice("foreign-slice", ownedSDP("foreign-pool", "node-z"))
	dynamicClient := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	clientset := fake.NewSimpleClientset(owned, legacy, foreign)
	reconciler := NewReconciler(clientset, dynamicClient)

	if err := reconciler.finalizeDeletingSimulatedDevicePool(context.Background(), sdp); err != nil {
		t.Fatalf("finalize deleting pool: %v", err)
	}
	for _, name := range []string{owned.Name, legacy.Name} {
		if _, err := clientset.ResourceV1().ResourceSlices().Get(context.Background(), name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("expected ResourceSlice %q to be deleted, got %v", name, err)
		}
	}
	if _, err := clientset.ResourceV1().ResourceSlices().Get(context.Background(), foreign.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("foreign ResourceSlice must remain: %v", err)
	}
	stored, err := dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).Get(
		context.Background(),
		sdp.GetName(),
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get finalized pool: %v", err)
	}
	if containsString(stored.GetFinalizers(), resourceSliceCleanupFinalizer) {
		t.Fatalf("cleanup finalizer was not removed: %#v", stored.GetFinalizers())
	}
}

func TestFinalizeDeletingSDPPreservesSameNameSliceWithDifferentUID(t *testing.T) {
	sdp := ownedSDP("uid-isolation", "node-a")
	now := metav1.Now()
	sdp.SetDeletionTimestamp(&now)
	sdp.SetFinalizers([]string{resourceSliceCleanupFinalizer})
	foreignOwner := ownedSDP("uid-isolation", "node-a")
	foreignOwner.SetUID(types.UID("different-uid"))
	foreign := ownershipSlice("sim-slice-uid-isolation-node-a-default", foreignOwner)
	dynamicClient := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	clientset := fake.NewSimpleClientset(foreign)
	reconciler := NewReconciler(clientset, dynamicClient)

	if err := reconciler.finalizeDeletingSimulatedDevicePool(context.Background(), sdp); err != nil {
		t.Fatalf("finalize UID-isolated pool: %v", err)
	}
	if _, err := clientset.ResourceV1().ResourceSlices().Get(context.Background(), foreign.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("same-name slice with a different UID owner must remain: %v", err)
	}
}

func TestFinalizeDeletingSDPPropagatesFinalizerRemovalConflict(t *testing.T) {
	sdp := ownedSDP("remove-conflict", "node-a")
	now := metav1.Now()
	sdp.SetDeletionTimestamp(&now)
	sdp.SetFinalizers([]string{resourceSliceCleanupFinalizer})
	conflict := apierrors.NewConflict(sdpGVR.GroupResource(), sdp.GetName(), errors.New("changed concurrently"))
	dynamicClient := dynamicClientWithMainSDPUpdateFailure(sdp, conflict)
	reconciler := NewReconciler(fake.NewSimpleClientset(), dynamicClient)

	err := reconciler.finalizeDeletingSimulatedDevicePool(context.Background(), sdp)
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected wrapped finalizer removal Conflict, got %v", err)
	}
}

func TestReconcileFinalizesDeletingSDP(t *testing.T) {
	sdp := ownedSDP("reconcile-delete", "node-a")
	now := metav1.Now()
	sdp.SetDeletionTimestamp(&now)
	sdp.SetFinalizers([]string{resourceSliceCleanupFinalizer})
	owned := ownershipSlice("sim-slice-reconcile-delete-node-a-default", sdp)
	dynamicClient := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	clientset := fake.NewSimpleClientset(owned)
	reconciler := NewReconciler(clientset, dynamicClient)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile deleting SDP: %v", err)
	}
	if _, err := clientset.ResourceV1().ResourceSlices().Get(context.Background(), owned.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected owned ResourceSlice deletion, got %v", err)
	}
	stored, err := dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).Get(
		context.Background(),
		sdp.GetName(),
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get finalized SDP: %v", err)
	}
	if containsString(stored.GetFinalizers(), resourceSliceCleanupFinalizer) {
		t.Fatalf("finalizer remained after reconcile cleanup: %#v", stored.GetFinalizers())
	}
}

func TestFinalizeDeletingSDPKeepsFinalizerWhenSliceDeleteFails(t *testing.T) {
	sdp := ownedSDP("delete-blocked", "node-a")
	now := metav1.Now()
	sdp.SetDeletionTimestamp(&now)
	sdp.SetFinalizers([]string{resourceSliceCleanupFinalizer})
	owned := ownershipSlice("sim-slice-delete-blocked-node-a-default", sdp)
	dynamicClient := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	clientset := fake.NewSimpleClientset(owned)
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceslices"},
		owned.Name,
		errors.New("denied"),
	)
	clientset.PrependReactor("delete", "resourceslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, forbidden
	})
	reconciler := NewReconciler(clientset, dynamicClient)

	err := reconciler.finalizeDeletingSimulatedDevicePool(context.Background(), sdp)
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected wrapped delete Forbidden error, got %v", err)
	}
	stored, getErr := dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).Get(
		context.Background(),
		sdp.GetName(),
		metav1.GetOptions{},
	)
	if getErr != nil {
		t.Fatalf("get blocked pool: %v", getErr)
	}
	if !containsString(stored.GetFinalizers(), resourceSliceCleanupFinalizer) {
		t.Fatalf("cleanup finalizer removed before cleanup succeeded: %#v", stored.GetFinalizers())
	}
}

func ownershipSlice(name string, sdp *unstructured.Unstructured) *resourcev1.ResourceSlice {
	slice := allocationSlice(name, "sim.draforge.oaslananka", sdp.GetName(), "node-a")
	for key, value := range resourceSliceOwnershipLabels(sdp) {
		slice.Labels[key] = value
	}
	return slice
}

func requireResourceSliceOwnerLabels(t *testing.T, slice *resourcev1.ResourceSlice, sdp *unstructured.Unstructured) {
	t.Helper()
	for key, expected := range resourceSliceOwnershipLabels(sdp) {
		if got := slice.Labels[key]; got != expected {
			t.Fatalf("ResourceSlice label %q = %q, want %q", key, got, expected)
		}
	}
}
