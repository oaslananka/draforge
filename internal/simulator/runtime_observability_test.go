// Package simulator tests controller runtime observability and cleanup.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

type countingEventBroadcaster struct {
	record.EventBroadcaster
	shutdowns atomic.Int64
}

func (b *countingEventBroadcaster) Shutdown() {
	b.shutdowns.Add(1)
	b.EventBroadcaster.Shutdown()
}

func TestReconcilerCloseShutsDownEventBroadcasterOnce(t *testing.T) {
	broadcaster := &countingEventBroadcaster{EventBroadcaster: record.NewBroadcaster()}
	reconciler := newReconcilerWithBroadcaster(nil, nil, broadcaster)

	reconciler.Close()
	reconciler.Close()

	if got := broadcaster.shutdowns.Load(); got != 1 {
		t.Fatalf("event broadcaster shutdown count = %d, want 1", got)
	}
}

func TestEventControllerRecordsPipelineMetrics(t *testing.T) {
	queue := workqueue.NewTypedRateLimitingQueue(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Millisecond, 10*time.Millisecond),
	)
	defer queue.ShutDown()
	reconciler := &Reconciler{}
	controller := &eventController{reconciler: reconciler}

	controller.enqueue(controllerPipelinePool, queue, poolSyncKey)
	if got := atomic.LoadInt64(&reconciler.PoolMetrics.QueueDepth); got != 1 {
		t.Fatalf("pool queue depth after enqueue = %d, want 1", got)
	}
	key, shutdown := queue.Get()
	if shutdown {
		t.Fatal("queue shut down before metrics item")
	}
	controller.processQueueItem(
		context.Background(),
		controllerPipelinePool,
		queue,
		key,
		func(context.Context) error {
			if got := atomic.LoadInt64(&reconciler.PoolMetrics.InFlight); got != 1 {
				t.Fatalf("pool in-flight during sync = %d, want 1", got)
			}
			time.Sleep(time.Millisecond)
			return nil
		},
	)

	if got := atomic.LoadInt64(&reconciler.PoolMetrics.Attempts); got != 1 {
		t.Fatalf("pool sync attempts = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&reconciler.PoolMetrics.DurationNanoseconds); got <= 0 {
		t.Fatalf("pool cumulative duration = %d, want positive", got)
	}
	if got := atomic.LoadInt64(&reconciler.PoolMetrics.InFlight); got != 0 {
		t.Fatalf("pool in-flight after sync = %d, want 0", got)
	}
	if got := atomic.LoadInt64(&reconciler.PoolMetrics.QueueDepth); got != 0 {
		t.Fatalf("pool queue depth after dequeue = %d, want 0", got)
	}
	if got := atomic.LoadInt64(&reconciler.AllocationMetrics.Attempts); got != 0 {
		t.Fatalf("allocation attempts changed during pool sync: %d", got)
	}
}
