//go:build e2e
// +build e2e

// Package e2e implements smoke tests for live cluster verification.
// SPDX-License-Identifier: Apache-2.0
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
)

func TestSmoke(t *testing.T) {
	// 1. Initialize cluster clients using default kubeconfig
	clientset, _, _, err := cluster.NewClientset("")
	if err != nil {
		t.Skip("Skipping E2E smoke test: no active cluster connection configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2. Discover DRA resources
	pools, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		t.Fatalf("Failed to discover DRA resources: %v", err)
	}

	t.Logf("Discovered %d pools, %d devices, %d claims", len(pools), len(devices), len(claims))

	// 3. Run doctor checks
	registry := doctor.NewRegistry()
	report := registry.RunDiagnostics(ctx, clientset)
	t.Logf("Doctor diagnostics: PASS=%d WARN=%d FAIL=%d", report.Summary["PASS"], report.Summary["WARN"], report.Summary["FAIL"])

	// 4. Assert that API availability check passes
	var apiCheckPassed bool
	for _, res := range report.Results {
		if res.ID == "DRA-001" && res.Status == "PASS" {
			apiCheckPassed = true
		}
	}
	if !apiCheckPassed {
		t.Error("Expected DRA API Availability check (DRA-001) to PASS")
	}
}
