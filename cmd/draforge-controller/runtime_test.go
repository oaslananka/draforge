// Package main tests controller runtime health behavior.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oaslananka/draforge/internal/health"
	"github.com/oaslananka/draforge/internal/simulator"
)

func TestControllerRuntimeKeepsLivenessIndependentFromReadiness(t *testing.T) {
	probe := health.NewReadinessProbe(func(context.Context) error {
		return errors.New("kubernetes API unavailable")
	}, time.Millisecond, 0)
	runtimeServer := newRuntimeServer(":0", &simulator.Reconciler{}, probe)

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResp := httptest.NewRecorder()
	runtimeServer.Handler.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want 200", healthResp.Code)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResp := httptest.NewRecorder()
	runtimeServer.Handler.ServeHTTP(readyResp, readyReq)
	if readyResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want 503", readyResp.Code)
	}
}

func TestControllerMetricsExposeLeaderState(t *testing.T) {
	reconciler := &simulator.Reconciler{}
	runtimeServer := newRuntimeServer(":0", reconciler, nil)

	for _, test := range []struct {
		name   string
		leader bool
		want   string
	}{
		{name: "standby", leader: false, want: "draforge_controller_leader 0\n"},
		{name: "leader", leader: true, want: "draforge_controller_leader 1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reconciler.SetLeader(test.leader)
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			response := httptest.NewRecorder()
			runtimeServer.Handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("metrics status = %d, want 200", response.Code)
			}
			if body := response.Body.String(); !strings.Contains(body, test.want) {
				t.Fatalf("metrics body missing %q:\n%s", test.want, body)
			}
		})
	}
}

func TestControllerMetricsExposePipelineRuntimeMetrics(t *testing.T) {
	reconciler := &simulator.Reconciler{
		PoolMetrics: simulator.PipelineMetrics{
			Attempts:            2,
			DurationNanoseconds: int64(1500 * time.Millisecond),
			InFlight:            1,
			QueueDepth:          3,
		},
		AllocationMetrics: simulator.PipelineMetrics{
			Attempts:            4,
			DurationNanoseconds: int64(2500 * time.Millisecond),
			InFlight:            0,
			QueueDepth:          1,
		},
	}
	runtimeServer := newRuntimeServer(":0", reconciler, nil)
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	runtimeServer.Handler.ServeHTTP(response, request)

	body := response.Body.String()
	for _, header := range []string{
		"# TYPE draforge_controller_sync_attempts_total counter\n",
		"# TYPE draforge_controller_sync_duration_seconds_total counter\n",
		"# TYPE draforge_controller_sync_in_flight gauge\n",
		"# TYPE draforge_controller_queue_depth gauge\n",
	} {
		if count := strings.Count(body, header); count != 1 {
			t.Fatalf("metrics header %q count = %d, want 1:\n%s", header, count, body)
		}
	}

	for _, expected := range []string{
		`draforge_controller_sync_attempts_total{pipeline="pool"} 2`,
		`draforge_controller_sync_attempts_total{pipeline="allocation"} 4`,
		`draforge_controller_sync_duration_seconds_total{pipeline="pool"} 1.500000000`,
		`draforge_controller_sync_duration_seconds_total{pipeline="allocation"} 2.500000000`,
		`draforge_controller_sync_in_flight{pipeline="pool"} 1`,
		`draforge_controller_queue_depth{pipeline="pool"} 3`,
		`draforge_controller_queue_depth{pipeline="allocation"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics body missing %q:\n%s", expected, body)
		}
	}
}

func TestControllerMetricsExposeQueueErrorPolicyCounters(t *testing.T) {
	reconciler := &simulator.Reconciler{
		ReconcileRetriesCount: 3,
		TerminalErrorsCount:   2,
	}
	runtimeServer := newRuntimeServer(":0", reconciler, nil)
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	runtimeServer.Handler.ServeHTTP(response, request)

	for _, expected := range []string{
		"draforge_controller_reconcile_retries_total 3\n",
		"draforge_controller_terminal_errors_total 2\n",
	} {
		if body := response.Body.String(); !strings.Contains(body, expected) {
			t.Fatalf("metrics body missing %q:\n%s", expected, body)
		}
	}
}
