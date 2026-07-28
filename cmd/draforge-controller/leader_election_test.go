// Package main tests controller leader-election lifecycle behavior.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oaslananka/draforge/internal/simulator"
	"k8s.io/client-go/kubernetes/fake"
)

func validLeaderElectionOptions(identity string) leaderElectionOptions {
	return leaderElectionOptions{
		Enabled:        true,
		LeaseName:      "draforge-controller",
		LeaseNamespace: "draforge-system",
		Identity:       identity,
		LeaseDuration:  time.Second,
		RenewDeadline:  600 * time.Millisecond,
		RetryPeriod:    100 * time.Millisecond,
	}
}

func TestLeaderElectionOptionsValidate(t *testing.T) {
	valid := validLeaderElectionOptions("controller-a")
	if err := valid.validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}

	tests := map[string]func(*leaderElectionOptions){
		"empty lease name":       func(options *leaderElectionOptions) { options.LeaseName = "" },
		"empty namespace":        func(options *leaderElectionOptions) { options.LeaseNamespace = "" },
		"empty identity":         func(options *leaderElectionOptions) { options.Identity = "" },
		"zero lease duration":    func(options *leaderElectionOptions) { options.LeaseDuration = 0 },
		"zero renew deadline":    func(options *leaderElectionOptions) { options.RenewDeadline = 0 },
		"zero retry period":      func(options *leaderElectionOptions) { options.RetryPeriod = 0 },
		"renew exceeds lease":    func(options *leaderElectionOptions) { options.RenewDeadline = options.LeaseDuration },
		"retry exceeds deadline": func(options *leaderElectionOptions) { options.RetryPeriod = options.RenewDeadline },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			if err := options.validate(); err == nil {
				t.Fatalf("expected invalid leader-election options: %#v", options)
			}
		})
	}

	disabled := leaderElectionOptions{}
	if err := disabled.validate(); err != nil {
		t.Fatalf("disabled leader election should not require lease settings: %v", err)
	}
}

func TestResolveLeaderElectionOptionsUsesPodIdentityAndNamespace(t *testing.T) {
	options := leaderElectionOptions{Enabled: true, LeaseName: "draforge-controller"}
	resolved, err := resolveLeaderElectionOptions(
		options,
		func(name string) string {
			return map[string]string{"POD_NAME": "controller-0", "POD_NAMESPACE": "draforge-system"}[name]
		},
		func() (string, error) { return "host-fallback", nil },
	)
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if resolved.Identity != "controller-0" || resolved.LeaseNamespace != "draforge-system" {
		t.Fatalf("unexpected resolved options: %#v", resolved)
	}
	if resolved.LeaseDuration != defaultLeaderLeaseDuration ||
		resolved.RenewDeadline != defaultLeaderRenewDeadline ||
		resolved.RetryPeriod != defaultLeaderRetryPeriod {
		t.Fatalf("unexpected resolved leader-election durations: %#v", resolved)
	}
}

func TestResolveLeaderElectionOptionsFallsBackToHostnameAndDefaultNamespace(t *testing.T) {
	options := leaderElectionOptions{Enabled: true, LeaseName: "draforge-controller"}
	resolved, err := resolveLeaderElectionOptions(
		options,
		func(string) string { return "" },
		func() (string, error) { return "controller-host", nil },
	)
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if resolved.Identity != "controller-host" || resolved.LeaseNamespace != "default" {
		t.Fatalf("unexpected resolved options: %#v", resolved)
	}

	_, err = resolveLeaderElectionOptions(
		options,
		func(string) string { return "" },
		func() (string, error) { return "", errors.New("hostname unavailable") },
	)
	if err == nil {
		t.Fatal("expected hostname resolution failure")
	}
}

func TestControllerLifecycleRejectsMissingActiveCallback(t *testing.T) {
	err := runControllerLifecycle(
		context.Background(),
		fake.NewSimpleClientset(),
		leaderElectionOptions{},
		&simulator.Reconciler{},
		nil,
	)
	if err == nil {
		t.Fatal("expected missing active callback to be rejected")
	}
}

func TestControllerLifecycleWithoutLeaderElectionRunsActiveController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reconciler := &simulator.Reconciler{}
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- runControllerLifecycle(ctx, fake.NewSimpleClientset(), leaderElectionOptions{}, reconciler, func(activeContext context.Context) {
			close(started)
			<-activeContext.Done()
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active controller did not start")
	}
	if !reconciler.IsLeader() {
		t.Fatal("single-controller mode must report leader state")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("controller lifecycle returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("controller lifecycle did not stop")
	}
	if reconciler.IsLeader() {
		t.Fatal("controller must clear leader state after shutdown")
	}
}

type leaderActivityTracker struct {
	active  atomic.Int64
	maximum atomic.Int64
	lock    sync.Mutex
	order   []string
}

func (tracker *leaderActivityTracker) callback(identity string, started chan<- struct{}) controllerActiveFunc {
	return func(activeContext context.Context) {
		current := tracker.active.Add(1)
		tracker.recordMaximum(current)
		tracker.lock.Lock()
		tracker.order = append(tracker.order, identity)
		tracker.lock.Unlock()
		started <- struct{}{}
		<-activeContext.Done()
		tracker.active.Add(-1)
	}
}

func (tracker *leaderActivityTracker) recordMaximum(current int64) {
	for {
		observed := tracker.maximum.Load()
		if current <= observed || tracker.maximum.CompareAndSwap(observed, current) {
			return
		}
	}
}

func (tracker *leaderActivityTracker) leadershipOrder() []string {
	tracker.lock.Lock()
	defer tracker.lock.Unlock()
	return append([]string(nil), tracker.order...)
}

func startLeaderCandidate(
	ctx context.Context,
	clientset *fake.Clientset,
	identity string,
	reconciler *simulator.Reconciler,
	active controllerActiveFunc,
) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- runControllerLifecycle(
			ctx,
			clientset,
			validLeaderElectionOptions(identity),
			reconciler,
			active,
		)
	}()
	return done
}

func waitForLeaderStart(t *testing.T, started <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func requireCandidateStopped(t *testing.T, done <-chan error, message string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", message, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func TestControllerLeaderElectionTransfersWithoutOverlap(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelA()
	defer cancelB()

	reconcilerA := &simulator.Reconciler{}
	reconcilerB := &simulator.Reconciler{}
	startedA := make(chan struct{}, 1)
	startedB := make(chan struct{}, 1)
	tracker := &leaderActivityTracker{order: make([]string, 0, 2)}

	doneA := startLeaderCandidate(ctxA, clientset, "controller-a", reconcilerA, tracker.callback("controller-a", startedA))
	waitForLeaderStart(t, startedA, 2*time.Second, "controller-a did not acquire leadership")

	doneB := startLeaderCandidate(ctxB, clientset, "controller-b", reconcilerB, tracker.callback("controller-b", startedB))
	select {
	case <-startedB:
		t.Fatal("controller-b became active while controller-a still held the lease")
	case <-time.After(150 * time.Millisecond):
	}
	if !reconcilerA.IsLeader() || reconcilerB.IsLeader() {
		t.Fatalf("unexpected leader state before failover: a=%t b=%t", reconcilerA.IsLeader(), reconcilerB.IsLeader())
	}

	cancelA()
	requireCandidateStopped(t, doneA, "controller-a did not stop")
	waitForLeaderStart(t, startedB, 3*time.Second, "controller-b did not acquire leadership after lease expiry")
	if reconcilerA.IsLeader() || !reconcilerB.IsLeader() {
		t.Fatalf("unexpected leader state after failover: a=%t b=%t", reconcilerA.IsLeader(), reconcilerB.IsLeader())
	}
	if got := tracker.maximum.Load(); got != 1 {
		t.Fatalf("expected at most one active controller, observed %d", got)
	}

	cancelB()
	requireCandidateStopped(t, doneB, "controller-b did not stop")
	if order := tracker.leadershipOrder(); len(order) != 2 || order[0] != "controller-a" || order[1] != "controller-b" {
		t.Fatalf("unexpected leadership order: %#v", order)
	}
}
