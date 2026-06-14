// Package doctor unit tests.
// SPDX-License-Identifier: Apache-2.0
package doctor

import (
	"context"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDoctorRegistryAndChecks(t *testing.T) {
	// 1. Create a fake clientset
	clientset := fake.NewSimpleClientset()
	ctx := context.Background()

	// 2. Initialize registry
	registry := NewRegistry()
	if len(registry.checks) == 0 {
		t.Fatal("Registry initialized with 0 checks, expected default checks to be registered")
	}

	// 3. Run diagnostics
	report := registry.RunDiagnostics(ctx, clientset)

	if len(report.Results) != len(registry.checks) {
		t.Errorf("RunDiagnostics() returned %d results, expected %d", len(report.Results), len(registry.checks))
	}

	// 4. Verify report summary contains PASS/FAIL/WARN/UNKNOWN
	totalResults := 0
	for _, count := range report.Summary {
		totalResults += count
	}

	if totalResults != len(registry.checks) {
		t.Errorf("Summary total count %d does not match result count %d", totalResults, len(registry.checks))
	}

	// 5. Inspect individual check attributes
	for _, res := range report.Results {
		if res.ID == "" {
			t.Error("DoctorCheckResult ID is empty")
		}
		if res.Name == "" {
			t.Error("DoctorCheckResult Name is empty")
		}
		if res.Status != model.StatusPass && res.Status != model.StatusWarn && res.Status != model.StatusFail && res.Status != model.StatusSkip && res.Status != model.StatusUnknown {
			t.Errorf("Invalid DoctorCheckStatus: %q", res.Status)
		}
	}
}
