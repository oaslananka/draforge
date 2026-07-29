// Package simulator tests propagation of per-object Kubernetes API failures.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	resourceSliceResource = schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceslices"}
	resourceClaimResource = schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceclaims"}
)

type apiErrorMatcher func(error) bool

func reconcileWithResourceSliceFailure(
	t *testing.T,
	sdpName, health, verb string,
	failure error,
	objects ...runtime.Object,
) error {
	t.Helper()
	sdp := sdpObj(sdpName, "default", "sim.draforge.oaslananka", "pool", "gpu", 1, health, []string{"node-a"})
	reconciler, ctx := newReconciler(sdp, objects...)
	reconciler.clientset.(*fake.Clientset).PrependReactor(verb, "resourceslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, failure
	})
	return reconciler.Reconcile(ctx)
}

func requireControllerAPIError(
	t *testing.T,
	err error,
	matches apiErrorMatcher,
	disposition controllerErrorDisposition,
) {
	t.Helper()
	if !matches(err) {
		t.Fatalf("expected matching Kubernetes API error, got %v", err)
	}
	if got := classifyControllerError(err, 0); got != disposition {
		t.Fatalf("expected %q disposition, got %q", disposition, got)
	}
}

func TestReconcilePropagatesResourceSliceOperationFailures(t *testing.T) {
	existing := allocationSlice("sim-slice-update-conflict-node-a-default", "old.driver", "old-pool", "node-a")
	tests := map[string]struct {
		sdpName     string
		health      string
		verb        string
		failure     error
		objects     []runtime.Object
		matches     apiErrorMatcher
		disposition controllerErrorDisposition
	}{
		"get forbidden": {
			sdpName:     "get-failure",
			health:      "healthy",
			verb:        "get",
			failure:     apierrors.NewForbidden(resourceSliceResource, "sim-slice-get-failure-node-a-default", errors.New("denied")),
			matches:     apierrors.IsForbidden,
			disposition: controllerErrorTerminal,
		},
		"update conflict": {
			sdpName:     "update-conflict",
			health:      "healthy",
			verb:        "update",
			failure:     apierrors.NewConflict(resourceSliceResource, existing.Name, errors.New("changed concurrently")),
			objects:     []runtime.Object{existing},
			matches:     apierrors.IsConflict,
			disposition: controllerErrorRetry,
		},
		"create already exists": {
			sdpName:     "create-race",
			health:      "healthy",
			verb:        "create",
			failure:     apierrors.NewAlreadyExists(resourceSliceResource, "sim-slice-create-race-node-a-default"),
			matches:     apierrors.IsAlreadyExists,
			disposition: controllerErrorRetry,
		},
		"delete forbidden": {
			sdpName:     "delete-failure",
			health:      "disappear",
			verb:        "delete",
			failure:     apierrors.NewForbidden(resourceSliceResource, "sim-slice-delete-failure-node-a-default", errors.New("denied")),
			matches:     apierrors.IsForbidden,
			disposition: controllerErrorTerminal,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := reconcileWithResourceSliceFailure(t, test.sdpName, test.health, test.verb, test.failure, test.objects...)
			requireControllerAPIError(t, err, test.matches, test.disposition)
		})
	}
}

func TestReconcilePropagatesCleanupListFailure(t *testing.T) {
	sdp := sdpObj("cleanup-list", "default", "sim.draforge.oaslananka", "pool", "gpu", 1, "healthy", []string{"node-a"})
	reconciler, ctx := newReconciler(sdp)
	unavailable := apierrors.NewServiceUnavailable("ResourceSlice list unavailable")
	reconciler.clientset.(*fake.Clientset).PrependReactor("list", "resourceslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, unavailable
	})

	err := reconciler.Reconcile(ctx)
	if !apierrors.IsServiceUnavailable(err) {
		t.Fatalf("expected wrapped ServiceUnavailable error, got %v", err)
	}
}

func TestReconcilePropagatesOrphanDeleteFailure(t *testing.T) {
	sdp := sdpObj("orphan-delete", "default", "sim.draforge.oaslananka", "pool", "gpu", 1, "healthy", []string{"node-a"})
	orphan := allocationSlice("orphan-slice", "sim.draforge.oaslananka", "old-pool", "node-z")
	reconciler, ctx := newReconciler(sdp, orphan)
	forbidden := apierrors.NewForbidden(resourceSliceResource, orphan.Name, errors.New("denied"))
	reconciler.clientset.(*fake.Clientset).PrependReactor("delete", "resourceslices", func(action clienttesting.Action) (bool, runtime.Object, error) {
		deleteAction, ok := action.(clienttesting.DeleteAction)
		if !ok || deleteAction.GetName() != orphan.Name {
			return false, nil, nil
		}
		return true, nil, forbidden
	})

	err := reconciler.Reconcile(ctx)
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected wrapped orphan delete Forbidden error, got %v", err)
	}
}

func TestReconcilePropagatesSDPStatusConflict(t *testing.T) {
	ctx := context.Background()
	sdp := sdpObj("status-conflict", "default", "sim.draforge.oaslananka", "pool", "gpu", 1, "healthy", []string{"node-a"})
	dynamicClient := dynfake.NewSimpleDynamicClient(runtime.NewScheme(), sdp)
	clientset := fake.NewSimpleClientset()
	reconciler := NewReconciler(clientset, dynamicClient)
	conflict := apierrors.NewConflict(sdpGVR.GroupResource(), sdp.GetName(), errors.New("changed concurrently"))
	dynamicClient.PrependReactor("update", "simulateddevicepools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "status" {
			return false, nil, nil
		}
		return true, nil, conflict
	})

	err := reconciler.Reconcile(ctx)
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected wrapped SDP status Conflict error, got %v", err)
	}
}

func TestSimulateAllocationPropagatesClaimStatusFailure(t *testing.T) {
	tests := map[string]struct {
		failure     error
		disposition controllerErrorDisposition
	}{
		"conflict retries": {
			failure:     apierrors.NewConflict(resourceClaimResource, "claim-a", errors.New("changed concurrently")),
			disposition: controllerErrorRetry,
		},
		"forbidden is terminal": {
			failure:     apierrors.NewForbidden(resourceClaimResource, "claim-a", errors.New("denied")),
			disposition: controllerErrorTerminal,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			claim := allocationClaim("claim-a", exactRequest("device", "gpu-class", 1))
			slice := allocationSlice("slice-a", "sim.draforge.oaslananka", "pool", "node-a", resourcev1.Device{Name: "dev-0"})
			clientset := fake.NewSimpleClientset(claim, slice, allocationClass("gpu-class"))
			clientset.PrependReactor("update", "resourceclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "status" {
					return false, nil, nil
				}
				return true, nil, test.failure
			})
			reconciler := NewReconciler(clientset, dynfake.NewSimpleDynamicClient(runtime.NewScheme()))

			err := reconciler.SimulateAllocation(context.Background())
			if err == nil {
				t.Fatalf("expected claim status failure %v", test.failure)
			}
			if disposition := classifyControllerError(err, 0); disposition != test.disposition {
				t.Fatalf("expected %q disposition, got %q for %v", test.disposition, disposition, err)
			}
			if got := atomic.LoadInt64(&reconciler.AllocationsSimulated); got != 0 {
				t.Fatalf("failed status update must not increment allocation counter, got %d", got)
			}
		})
	}
}

func TestReconcileTreatsMissingDeleteAsIdempotent(t *testing.T) {
	sdp := sdpObj("already-gone", "default", "sim.draforge.oaslananka", "pool", "gpu", 1, "disappear", []string{"node-a"})
	reconciler, ctx := newReconciler(sdp)
	notFound := apierrors.NewNotFound(resourceSliceResource, "sim-slice-already-gone-node-a-default")
	reconciler.clientset.(*fake.Clientset).PrependReactor("delete", "resourceslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, notFound
	})

	if err := reconciler.Reconcile(ctx); err != nil {
		t.Fatalf("missing ResourceSlice delete should be idempotent: %v", err)
	}
}

func exactRequest(name, class string, count int64) resourcev1.DeviceRequest {
	return resourcev1.DeviceRequest{
		Name: name,
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: class,
			Count:           count,
		},
	}
}
