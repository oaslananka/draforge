// Package health tests dependency-aware readiness behavior.
// SPDX-License-Identifier: Apache-2.0
package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestReadinessAllowsGraceThenFailsAndRecovers(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0)}
	dependencyErr := errors.New("cluster unavailable")
	probe := NewReadinessProbe(func(context.Context) error {
		return dependencyErr
	}, 50*time.Millisecond, 30*time.Second)
	probe.now = clock.Now
	probe.startedAt = clock.Now()

	withinGrace := probe.Check(context.Background())
	if !withinGrace.Ready || !withinGrace.Degraded {
		t.Fatalf("expected degraded readiness during grace period, got %+v", withinGrace)
	}
	if withinGrace.Reason != ReasonKubernetesAPIUnavailable {
		t.Fatalf("unexpected degraded reason: %q", withinGrace.Reason)
	}

	clock.Advance(31 * time.Second)
	beyondGrace := probe.Check(context.Background())
	if beyondGrace.Ready || beyondGrace.Degraded {
		t.Fatalf("expected not-ready after grace period, got %+v", beyondGrace)
	}

	dependencyErr = nil
	recovered := probe.Check(context.Background())
	if !recovered.Ready || recovered.Degraded {
		t.Fatalf("expected readiness recovery, got %+v", recovered)
	}
	if recovered.LastSuccess == nil || recovered.LastSuccess.IsZero() {
		t.Fatal("expected recovery to record last successful check")
	}
}

func TestReadinessCheckIsBoundedAndRedactsDependencyError(t *testing.T) {
	probe := NewReadinessProbe(func(ctx context.Context) error {
		<-ctx.Done()
		return errors.New("token=super-secret kubeconfig=/private/config")
	}, 15*time.Millisecond, 0)

	started := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	probe.ServeHTTP(rr, req)
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("readiness check exceeded bounded timeout: %s", elapsed)
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "super-secret") || strings.Contains(body, "/private/config") {
		t.Fatalf("readiness response leaked dependency error: %s", body)
	}
	if !strings.Contains(body, ReasonKubernetesAPIUnavailable) {
		t.Fatalf("readiness response is missing stable failure reason: %s", body)
	}
}

func TestReadinessCheckHonorsParentCancellation(t *testing.T) {
	probe := NewReadinessProbe(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, time.Second, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	result := probe.Check(ctx)
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled readiness check returned too slowly: %s", elapsed)
	}
	if result.Ready || result.Reason != ReasonCheckCanceled {
		t.Fatalf("unexpected canceled readiness result: %+v", result)
	}
}

func TestKubernetesReadinessProbeCancelsInFlightRequest(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	requestCanceled := make(chan struct{}, 1)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted <- struct{}{}
		<-r.Context().Done()
		requestCanceled <- struct{}{}
	}))
	defer apiServer.Close()

	client, err := kubernetes.NewForConfig(&rest.Config{Host: apiServer.URL})
	if err != nil {
		t.Fatalf("create kubernetes client: %v", err)
	}
	probe := NewKubernetesReadinessProbe(client, 100*time.Millisecond, 0)

	type checkResult struct {
		result  ReadinessResult
		elapsed time.Duration
	}
	done := make(chan checkResult, 1)
	go func() {
		started := time.Now()
		done <- checkResult{result: probe.Check(context.Background()), elapsed: time.Since(started)}
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("Kubernetes readiness request did not start")
	}

	checked := <-done
	if checked.elapsed > 500*time.Millisecond {
		t.Fatalf("Kubernetes readiness request exceeded timeout: %s", checked.elapsed)
	}
	if checked.result.Ready || checked.result.Reason != ReasonKubernetesAPIUnavailable {
		t.Fatalf("unexpected Kubernetes readiness result: %+v", checked.result)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("Kubernetes API request context was not canceled")
	}
}
