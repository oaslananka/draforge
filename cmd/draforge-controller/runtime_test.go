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
