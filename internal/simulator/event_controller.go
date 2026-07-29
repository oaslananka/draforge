// Package simulator implements informer-driven controller execution.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	poolSyncKey       = "pool-sync"
	allocationSyncKey = "allocation-sync"
)

var (
	controllerQueueBaseDelay = 100 * time.Millisecond
	controllerQueueMaxDelay  = 30 * time.Second
)

type eventController struct {
	reconciler      *Reconciler
	typedFactory    informers.SharedInformerFactory
	dynamicFactory  dynamicinformer.DynamicSharedInformerFactory
	poolQueue       workqueue.TypedRateLimitingInterface[string]
	allocationQueue workqueue.TypedRateLimitingInterface[string]
	informerSynced  []cache.InformerSynced
	synced          chan struct{}
	syncedOnce      sync.Once
}

// StartEventDrivenController runs informer-backed, rate-limited controller workers until ctx is canceled.
func (r *Reconciler) StartEventDrivenController(ctx context.Context) error {
	controller, err := newEventController(r)
	if err != nil {
		return err
	}
	return controller.Run(ctx)
}

func newEventController(reconciler *Reconciler) (*eventController, error) {
	if reconciler == nil {
		return nil, fmt.Errorf("event controller requires a reconciler")
	}
	if reconciler.clientset == nil {
		return nil, fmt.Errorf("event controller requires a Kubernetes client")
	}
	if reconciler.dynamicClient == nil {
		return nil, fmt.Errorf("event controller requires a dynamic Kubernetes client")
	}

	typedFactory := informers.NewSharedInformerFactory(reconciler.clientset, 0)
	dynamicFactory := dynamicinformer.NewDynamicSharedInformerFactory(reconciler.dynamicClient, 0)
	poolQueue := newControllerQueue("draforge-pool-sync")
	allocationQueue := newControllerQueue("draforge-allocation-sync")

	controller := &eventController{
		reconciler:      reconciler,
		typedFactory:    typedFactory,
		dynamicFactory:  dynamicFactory,
		poolQueue:       poolQueue,
		allocationQueue: allocationQueue,
		synced:          make(chan struct{}),
	}

	nodeInformer := typedFactory.Core().V1().Nodes().Informer()
	claimInformer := typedFactory.Resource().V1().ResourceClaims().Informer()
	sliceInformer := typedFactory.Resource().V1().ResourceSlices().Informer()
	classInformer := typedFactory.Resource().V1().DeviceClasses().Informer()
	poolInformer := dynamicFactory.ForResource(sdpGVR).Informer()

	if err := addQueueEventHandler(poolInformer, poolQueue, poolSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register SimulatedDevicePool event handler: %w", err)
	}
	if err := addQueueEventHandler(nodeInformer, poolQueue, poolSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register Node event handler: %w", err)
	}
	if err := addQueueEventHandler(sliceInformer, poolQueue, poolSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register ResourceSlice pool event handler: %w", err)
	}
	if err := addQueueEventHandler(claimInformer, allocationQueue, allocationSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register ResourceClaim event handler: %w", err)
	}
	if err := addQueueEventHandler(sliceInformer, allocationQueue, allocationSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register ResourceSlice allocation event handler: %w", err)
	}
	if err := addQueueEventHandler(classInformer, allocationQueue, allocationSyncKey); err != nil {
		controller.shutDownQueues()
		return nil, fmt.Errorf("register DeviceClass event handler: %w", err)
	}

	controller.informerSynced = []cache.InformerSynced{
		poolInformer.HasSynced,
		nodeInformer.HasSynced,
		claimInformer.HasSynced,
		sliceInformer.HasSynced,
		classInformer.HasSynced,
	}
	return controller, nil
}

func newControllerQueue(name string) workqueue.TypedRateLimitingInterface[string] {
	return workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](
			controllerQueueBaseDelay,
			controllerQueueMaxDelay,
		),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: name},
	)
}

func addQueueEventHandler(
	informer cache.SharedIndexInformer,
	queue workqueue.TypedRateLimitingInterface[string],
	key string,
) error {
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(any) {
			queue.Add(key)
		},
		UpdateFunc: func(_, _ any) {
			queue.Add(key)
		},
		DeleteFunc: func(any) {
			queue.Add(key)
		},
	})
	return err
}

func (controller *eventController) Run(ctx context.Context) error {
	controller.typedFactory.Start(ctx.Done())
	controller.dynamicFactory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), controller.informerSynced...) {
		controller.shutDownQueues()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("event controller informer caches did not sync")
	}

	controller.syncedOnce.Do(func() { close(controller.synced) })
	controller.poolQueue.Add(poolSyncKey)
	controller.allocationQueue.Add(allocationSyncKey)

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		controller.runWorker(ctx, controller.poolQueue, controller.reconciler.Reconcile)
	}()
	go func() {
		defer workers.Done()
		controller.runWorker(ctx, controller.allocationQueue, controller.reconciler.SimulateAllocation)
	}()

	<-ctx.Done()
	controller.shutDownQueues()
	workers.Wait()
	return nil
}

func (controller *eventController) runWorker(
	ctx context.Context,
	queue workqueue.TypedRateLimitingInterface[string],
	syncFn func(context.Context) error,
) {
	for {
		key, shutdown := queue.Get()
		if shutdown {
			return
		}
		controller.processQueueItem(ctx, queue, key, syncFn)
	}
}

func (controller *eventController) processQueueItem(
	ctx context.Context,
	queue workqueue.TypedRateLimitingInterface[string],
	key string,
	syncFn func(context.Context) error,
) {
	defer queue.Done(key)
	if err := syncFn(ctx); err != nil {
		if ctx.Err() != nil {
			queue.Forget(key)
			return
		}
		atomic.AddInt64(&controller.reconciler.ReconcileErrorsCount, 1)
		requeues := queue.NumRequeues(key)
		if classifyControllerError(err, requeues) == controllerErrorRetry {
			atomic.AddInt64(&controller.reconciler.ReconcileRetriesCount, 1)
			klog.Errorf("Controller queue item %q failed; retry %d/%d scheduled: %v", key, requeues+1, maxControllerQueueRetries, err)
			queue.AddRateLimited(key)
			return
		}
		atomic.AddInt64(&controller.reconciler.TerminalErrorsCount, 1)
		queue.Forget(key)
		klog.Errorf("Controller queue item %q reached a terminal failure after %d retries: %v", key, requeues, err)
		return
	}
	queue.Forget(key)
}

func (controller *eventController) shutDownQueues() {
	controller.poolQueue.ShutDown()
	controller.allocationQueue.ShutDown()
}
