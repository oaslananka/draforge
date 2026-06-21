// Package doctor unit tests.
// SPDX-License-Identifier: Apache-2.0
package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	discoveryfake "k8s.io/client-go/discovery/fake"
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

func TestKubernetesVersionCheck(t *testing.T) {
	tests := []struct {
		name     string
		major    string
		minor    string
		expected model.DoctorCheckStatus
	}{
		{name: "v1.9 should FAIL", major: "1", minor: "9", expected: model.StatusFail},
		{name: "v1.31 should FAIL", major: "1", minor: "31", expected: model.StatusFail},
		{name: "v1.32 should WARN", major: "1", minor: "32", expected: model.StatusWarn},
		{name: "v1.33 should WARN", major: "1", minor: "33", expected: model.StatusWarn},
		{name: "v1.34 should PASS", major: "1", minor: "34", expected: model.StatusPass},
		{name: "v1.35 should PASS", major: "1", minor: "35", expected: model.StatusPass},
		{name: "malformed major should UNKNOWN", major: "abc", minor: "32", expected: model.StatusUnknown},
		{name: "malformed minor should UNKNOWN", major: "1", minor: "xyz", expected: model.StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			fd, ok := clientset.Discovery().(*discoveryfake.FakeDiscovery)
			if !ok {
				t.Fatal("clientset.Discovery() is not *FakeDiscovery")
			}
			fd.FakedServerVersion = &version.Info{
				Major:      tt.major,
				Minor:      tt.minor,
				GitVersion: "v" + tt.major + "." + tt.minor + ".0",
			}

			check := &KubernetesVersionCheck{}
			res := check.Run(context.Background(), clientset)
			if res.Status != tt.expected {
				t.Errorf("expected status %q, got %q (evidence: %q)", tt.expected, res.Status, res.Evidence)
			}
		})
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

func TestStaleResourceSliceStates(t *testing.T) {
	ctx := context.Background()

	// 1. Create a node that exists but is not Ready
	notReadyNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "not-ready-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	// 2. Create a node that is Ready
	readyNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ready-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	// 3. Inconsistent slice (empty driver)
	inconsistentSlice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "inconsistent-slice"},
		Spec: resourcev1.ResourceSliceSpec{
			Driver: "",
			Pool:   resourcev1.ResourcePool{Name: "pool-1"},
		},
	}

	// 4. Stale slice (non-existent node)
	staleSlice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "stale-slice"},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "driver-1",
			NodeName: ptr("non-existent-node"),
			Pool:     resourcev1.ResourcePool{Name: "pool-1"},
		},
	}

	// 5. Unavailable slice (not Ready node)
	unavailableNodeSlice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "unavail-node-slice"},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "driver-1",
			NodeName: ptr("not-ready-node"),
			Pool:     resourcev1.ResourcePool{Name: "pool-1"},
		},
	}

	// 6. Unavailable slice (unhealthy custom label)
	unhealthyLabelSlice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unhealthy-label-slice",
			Labels: map[string]string{
				"draforge.oaslananka/health": "degraded",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "driver-1",
			NodeName: ptr("ready-node"),
			Pool:     resourcev1.ResourcePool{Name: "pool-1"},
		},
	}

	clientset := fake.NewSimpleClientset(
		readyNode, notReadyNode,
		inconsistentSlice, staleSlice, unavailableNodeSlice, unhealthyLabelSlice,
	)

	check := &StaleResourceSliceCheck{}
	res := check.Run(ctx, clientset)

	if res.Status != model.StatusFail {
		t.Errorf("expected FAIL status, got %q", res.Status)
	}

	if !strings.Contains(res.Evidence, "inconsistent-slice") {
		t.Errorf("expected evidence to mention inconsistent-slice, got %q", res.Evidence)
	}
	if !strings.Contains(res.Evidence, "stale-slice") {
		t.Errorf("expected evidence to mention stale-slice, got %q", res.Evidence)
	}
	if !strings.Contains(res.Evidence, "unavail-node-slice") {
		t.Errorf("expected evidence to mention unavail-node-slice, got %q", res.Evidence)
	}
	if !strings.Contains(res.Evidence, "unhealthy-label-slice") {
		t.Errorf("expected evidence to mention unhealthy-label-slice, got %q", res.Evidence)
	}

	if !strings.Contains(res.Remediation, "orphaned/stale") {
		t.Errorf("remediation should suggest cleaning up stale slices, got %q", res.Remediation)
	}
}

func ptr[T any](v T) *T {
	return &v
}
