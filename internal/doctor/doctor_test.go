// Package doctor unit tests.
// SPDX-License-Identifier: Apache-2.0
package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestAPIAvailabilityMissingInFakeCluster(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	ctx := context.Background()

	check := &APIAvailabilityCheck{}
	res := check.Run(ctx, clientset)

	if res.Status != model.StatusFail {
		t.Errorf("expected FAIL for missing DRA API, got %q", res.Status)
	}
	if !strings.Contains(res.Remediation, "v1.34+") {
		t.Errorf("remediation should mention v1.34+, got %q", res.Remediation)
	}
	if res.ID != "DRA-001" {
		t.Errorf("expected ID DRA-001, got %q", res.ID)
	}
}

func TestAPIAvailabilityCheckDeterministic(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	ctx := context.Background()
	check := &APIAvailabilityCheck{}

	res1 := check.Run(ctx, clientset)
	res2 := check.Run(ctx, clientset)

	if res1.Status != res2.Status {
		t.Errorf("not deterministic: %q != %q", res1.Status, res2.Status)
	}
	if res1.Remediation != res2.Remediation {
		t.Errorf("remediation not deterministic: %q != %q", res1.Remediation, res2.Remediation)
	}
}

func TestStaleResourceSliceCheckWithNoSlices(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	ctx := context.Background()

	check := &StaleResourceSliceCheck{}
	res := check.Run(ctx, clientset)

	if res.Status != model.StatusPass && res.Status != model.StatusUnknown {
		t.Errorf("expected PASS or UNKNOWN (no slices), got %q", res.Status)
	}
}

func TestStaleResourceSliceWithDriver(t *testing.T) {
	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "valid-slice",
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "test-driver.example.com",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "test-pool",
			},
		},
	}

	clientset := fake.NewSimpleClientset(slice)
	ctx := context.Background()

	check := &StaleResourceSliceCheck{}
	res := check.Run(ctx, clientset)

	if res.Status != model.StatusPass {
		t.Errorf("expected PASS for valid slice, got %q (evidence: %q)", res.Status, res.Evidence)
	}
}
