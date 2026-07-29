// Package simulator tests informer-driven controller execution.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"errors"
	"testing"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
)

func newEventControllerTestDynamicClient(objects ...runtime.Object) *dynfake.FakeDynamicClient {
	return dynfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{sdpGVR: "SimulatedDevicePoolList"},
		objects...,
	)
}

func startTestEventController(
	t *testing.T,
	reconciler *Reconciler,
) (*eventController, context.CancelFunc, <-chan error) {
	t.Helper()
	controller, err := newEventController(reconciler)
	if err != nil {
		t.Fatalf("new event controller: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- controller.Run(ctx)
	}()
	select {
	case <-controller.synced:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("event controller caches did not sync")
	}
	return controller, cancel, done
}

func stopTestEventController(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("event controller stopped with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event controller did not stop")
	}
}

func TestEventControllerReconcilesPoolFromWatchEvent(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	dynamicClient := newEventControllerTestDynamicClient()
	reconciler := NewReconciler(clientset, dynamicClient)
	_, cancel, done := startTestEventController(t, reconciler)
	defer stopTestEventController(t, cancel, done)

	pool := sdpObj(
		"event-pool",
		"default",
		"sim.draforge.oaslananka",
		"event-pool",
		"gpu",
		1,
		"healthy",
		[]string{"node-a"},
	)
	if _, err := dynamicClient.Resource(sdpGVR).Namespace("default").Create(
		context.Background(),
		pool,
		metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("create SimulatedDevicePool: %v", err)
	}

	requireResourceSliceState(t, clientset, 1, 1, "create watch event did not produce ResourceSlice")

	updatedPool, err := dynamicClient.Resource(sdpGVR).Namespace("default").Get(
		context.Background(),
		pool.GetName(),
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get SimulatedDevicePool: %v", err)
	}
	if err := unstructured.SetNestedField(updatedPool.Object, int64(2), "spec", "deviceCount"); err != nil {
		t.Fatalf("set updated device count: %v", err)
	}
	if _, err := dynamicClient.Resource(sdpGVR).Namespace("default").Update(
		context.Background(),
		updatedPool,
		metav1.UpdateOptions{},
	); err != nil {
		t.Fatalf("update SimulatedDevicePool: %v", err)
	}
	requireResourceSliceState(t, clientset, 1, 2, "update watch event did not update ResourceSlice")

	if err := dynamicClient.Resource(sdpGVR).Namespace("default").Delete(
		context.Background(),
		pool.GetName(),
		metav1.DeleteOptions{},
	); err != nil {
		t.Fatalf("delete SimulatedDevicePool: %v", err)
	}
	requireResourceSliceState(t, clientset, 0, 0, "delete watch event did not remove orphan ResourceSlice")
}

func requireResourceSliceState(
	t *testing.T,
	clientset *fake.Clientset,
	expectedSlices, expectedDevices int,
	message string,
) {
	t.Helper()
	err := wait.PollUntilContextTimeout(
		context.Background(),
		10*time.Millisecond,
		2*time.Second,
		true,
		func(ctx context.Context) (bool, error) {
			slices, listErr := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
			if listErr != nil || len(slices.Items) != expectedSlices {
				return false, listErr
			}
			return expectedSlices == 0 || len(slices.Items[0].Spec.Devices) == expectedDevices, nil
		},
	)
	if err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func TestEventControllerAllocatesClaimFromWatchEvent(t *testing.T) {
	pool := sdpObj(
		"event-pool",
		"default",
		"sim.draforge.oaslananka",
		"event-pool",
		"gpu",
		1,
		"healthy",
		[]string{"node-a"},
	)
	slice := allocationSlice(
		"sim-slice-event-pool-node-a-default",
		"sim.draforge.oaslananka",
		"event-pool",
		"node-a",
		resourcev1.Device{Name: "dev-0"},
	)
	clientset := fake.NewSimpleClientset(slice, allocationClass("event-class"))
	reconciler := NewReconciler(clientset, newEventControllerTestDynamicClient(pool))
	_, cancel, done := startTestEventController(t, reconciler)
	defer stopTestEventController(t, cancel, done)

	claim := allocationClaim("event-claim", resourcev1.DeviceRequest{
		Name: "device",
		Exactly: &resourcev1.ExactDeviceRequest{
			DeviceClassName: "event-class",
			AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
			Count:           1,
		},
	})
	if _, err := clientset.ResourceV1().ResourceClaims("default").Create(
		context.Background(),
		claim,
		metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("create ResourceClaim: %v", err)
	}

	err := wait.PollUntilContextTimeout(
		context.Background(),
		10*time.Millisecond,
		2*time.Second,
		true,
		func(ctx context.Context) (bool, error) {
			updated, getErr := clientset.ResourceV1().ResourceClaims("default").Get(
				ctx,
				claim.Name,
				metav1.GetOptions{},
			)
			return getErr == nil && updated.Status.Allocation != nil, getErr
		},
	)
	if err != nil {
		t.Fatalf("watch event did not allocate ResourceClaim: %v", err)
	}
}

func TestEventControllerRateLimitsAndForgetsFailedItems(t *testing.T) {
	queue := workqueue.NewTypedRateLimitingQueue(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Millisecond, 10*time.Millisecond),
	)
	defer queue.ShutDown()
	controller := &eventController{reconciler: &Reconciler{}}
	queue.Add(poolSyncKey)
	key, shutdown := queue.Get()
	if shutdown {
		t.Fatal("queue shut down before first item")
	}
	controller.processQueueItem(context.Background(), controllerPipelinePool, queue, key, func(context.Context) error {
		return errors.New("temporary API failure")
	})
	if retries := queue.NumRequeues(key); retries != 1 {
		t.Fatalf("requeues after transient failure = %d, want 1", retries)
	}
	if errorsCount := controller.reconciler.ReconcileErrorsCount; errorsCount != 1 {
		t.Fatalf("reconcile errors = %d, want 1", errorsCount)
	}
	if retriesCount := controller.reconciler.ReconcileRetriesCount; retriesCount != 1 {
		t.Fatalf("reconcile retries = %d, want 1", retriesCount)
	}
	if terminalCount := controller.reconciler.TerminalErrorsCount; terminalCount != 0 {
		t.Fatalf("terminal errors = %d, want 0", terminalCount)
	}

	next := make(chan string, 1)
	go func() {
		item, stopped := queue.Get()
		if !stopped {
			next <- item
		}
	}()
	select {
	case key = <-next:
	case <-time.After(time.Second):
		t.Fatal("rate-limited item was not requeued")
	}
	controller.processQueueItem(context.Background(), controllerPipelinePool, queue, key, func(context.Context) error {
		return nil
	})
	if retries := queue.NumRequeues(key); retries != 0 {
		t.Fatalf("requeues after success = %d, want 0", retries)
	}
}

func TestEventControllerForgetsTerminalItems(t *testing.T) {
	queue := workqueue.NewTypedRateLimitingQueue(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Millisecond, 10*time.Millisecond),
	)
	defer queue.ShutDown()
	controller := &eventController{reconciler: &Reconciler{}}
	queue.Add(poolSyncKey)
	key, shutdown := queue.Get()
	if shutdown {
		t.Fatal("queue shut down before terminal item")
	}
	controller.processQueueItem(context.Background(), controllerPipelinePool, queue, key, func(context.Context) error {
		return apierrors.NewForbidden(controllerTestResource, "claim-a", errors.New("forbidden"))
	})
	if retries := queue.NumRequeues(key); retries != 0 {
		t.Fatalf("terminal item requeues = %d, want 0", retries)
	}
	if controller.reconciler.ReconcileErrorsCount != 1 || controller.reconciler.ReconcileRetriesCount != 0 || controller.reconciler.TerminalErrorsCount != 1 {
		t.Fatalf("unexpected terminal counters: %#v", controller.reconciler)
	}
}

func TestEventControllerStopsRetryingAtLimit(t *testing.T) {
	queue := workqueue.NewTypedRateLimitingQueue(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Nanosecond, time.Nanosecond),
	)
	defer queue.ShutDown()
	controller := &eventController{reconciler: &Reconciler{}}
	queue.Add(allocationSyncKey)

	for attempt := 0; attempt <= maxControllerQueueRetries; attempt++ {
		key, shutdown := queue.Get()
		if shutdown {
			t.Fatalf("queue shut down at attempt %d", attempt)
		}
		controller.processQueueItem(context.Background(), controllerPipelineAllocation, queue, key, func(context.Context) error {
			return errors.New("unknown transport error")
		})
	}

	if controller.reconciler.ReconcileErrorsCount != maxControllerQueueRetries+1 {
		t.Fatalf("reconcile errors = %d, want %d", controller.reconciler.ReconcileErrorsCount, maxControllerQueueRetries+1)
	}
	if controller.reconciler.ReconcileRetriesCount != maxControllerQueueRetries {
		t.Fatalf("reconcile retries = %d, want %d", controller.reconciler.ReconcileRetriesCount, maxControllerQueueRetries)
	}
	if controller.reconciler.TerminalErrorsCount != 1 {
		t.Fatalf("terminal errors = %d, want 1", controller.reconciler.TerminalErrorsCount)
	}
	if retries := queue.NumRequeues(allocationSyncKey); retries != 0 {
		t.Fatalf("retry history after terminal exhaustion = %d, want 0", retries)
	}
}
