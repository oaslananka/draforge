// Package main tests fail-closed and atomic CDI behavior.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPrepareCDIDirectoryFailsClosedInNodeMode(t *testing.T) {
	root := t.TempDir()
	notDirectory := filepath.Join(root, "cdi")
	if err := os.WriteFile(notDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := prepareCDIDirectory(notDirectory, outputModeNode); err == nil {
		t.Fatal("expected node mode to reject an unavailable CDI directory")
	}
}

func TestPrepareCDIDirectoryCreatesExplicitDemoDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo-cdi")
	if err := prepareCDIDirectory(dir, outputModeDemo); err != nil {
		t.Fatalf("prepare demo CDI directory: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat demo CDI directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("demo CDI path is not a directory: %s", dir)
	}
}

func TestAtomicCDIWriterPreservesLastKnownGoodOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cdiFileName)
	knownGood := []byte(`{"cdiVersion":"0.5.0","kind":"draforge.oaslananka/sim","devices":[]}`)
	if err := os.WriteFile(path, knownGood, 0o644); err != nil {
		t.Fatal(err)
	}

	writer := newAtomicCDIWriter()
	writer.rename = func(_, _ string) error { return errors.New("interrupted rename") }
	err := writer.Write(path, CDISpec{CDIVersion: "0.5.0", Kind: cdiKind})
	if err == nil {
		t.Fatal("expected interrupted atomic write to fail")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(knownGood) {
		t.Fatalf("last-known-good CDI document changed after failed rename: %s", got)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, ".draforge-cdi-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary CDI documents were not cleaned up: %v", matches)
	}
}

func TestAtomicCDIWriterDoesNotExposePartialTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cdiFileName)
	oldDocument := []byte(`{"cdiVersion":"0.5.0","kind":"draforge.oaslananka/sim","devices":[]}`)
	if err := os.WriteFile(path, oldDocument, 0o644); err != nil {
		t.Fatal(err)
	}

	renameStarted := make(chan struct{})
	releaseRename := make(chan struct{})
	writer := newAtomicCDIWriter()
	writer.rename = func(oldPath, newPath string) error {
		close(renameStarted)
		<-releaseRename
		return os.Rename(oldPath, newPath)
	}
	done := make(chan error, 1)
	go func() {
		done <- writer.Write(path, demoCDISpec())
	}()

	<-renameStarted
	duringWrite, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(duringWrite) != string(oldDocument) {
		t.Fatalf("reader observed a changed target before atomic rename: %s", duringWrite)
	}
	close(releaseRename)
	if writeErr := <-done; writeErr != nil {
		t.Fatalf("atomic write failed: %v", writeErr)
	}
	afterWrite, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterWrite) == string(oldDocument) || !strings.Contains(string(afterWrite), "demo-dev-0") {
		t.Fatalf("reader did not observe the complete replacement document: %s", afterWrite)
	}
}

func TestNodeRefreshRecoversAfterAtomicWriteFailure(t *testing.T) {
	claim := allocatedClaim("default", "claim-a", "node-a", []resourcev1.DeviceRequestAllocationResult{
		{Driver: simulatedDriverName, Pool: "pool-a", Device: "dev-0"},
	})
	client := fake.NewSimpleClientset(&claim)
	path := filepath.Join(t.TempDir(), cdiFileName)
	knownGood := []byte(`{"cdiVersion":"0.5.0","kind":"draforge.oaslananka/sim","devices":[]}`)
	if err := os.WriteFile(path, knownGood, 0o644); err != nil {
		t.Fatal(err)
	}
	renameCalls := 0
	writer := newAtomicCDIWriter()
	writer.rename = func(oldPath, newPath string) error {
		renameCalls++
		if renameCalls == 1 {
			return errors.New("simulated interrupted rename")
		}
		return os.Rename(oldPath, newPath)
	}
	status := newDriverStatus(outputModeNode)
	driver := newCDIDriver(outputModeNode, client, "node-a", path, writer, status)

	if err := driver.Refresh(context.Background()); err == nil {
		t.Fatal("expected the first atomic write to fail")
	}
	if ready, reason := status.Readiness(); ready || reason != readinessReasonCDIWrite {
		t.Fatalf("readiness = %v/%q, want false/%q", ready, reason, readinessReasonCDIWrite)
	}
	stillGood, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stillGood) != string(knownGood) {
		t.Fatalf("failed atomic write changed last-known-good state: %s", stillGood)
	}

	if err := driver.Refresh(context.Background()); err != nil {
		t.Fatalf("expected atomic write recovery, got %v", err)
	}
	if ready, reason := status.Readiness(); !ready || reason != readinessReasonReady {
		t.Fatalf("readiness = %v/%q, want true/%q", ready, reason, readinessReasonReady)
	}
}

func TestAtomicCDIWriterReportsPermissionFailure(t *testing.T) {
	writer := newAtomicCDIWriter()
	writer.createTemp = func(string, string) (*os.File, error) {
		return nil, os.ErrPermission
	}
	if err := writer.Write(filepath.Join(t.TempDir(), cdiFileName), demoCDISpec()); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestAtomicCDIWriterSetsExpectedModeAndOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), cdiFileName)
	if err := newAtomicCDIWriter().Write(path, demoCDISpec()); err != nil {
		t.Fatalf("write CDI document: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("CDI mode = %o, want 644", got)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("CDI file stat does not expose ownership")
	}
	if int(stat.Uid) != os.Geteuid() {
		t.Fatalf("CDI owner uid = %d, want effective uid %d", stat.Uid, os.Geteuid())
	}
}

func TestBuildCDISpecUsesFullyQualifiedDeviceIdentity(t *testing.T) {
	claims := []resourcev1.ResourceClaim{
		allocatedClaim("default", "claim-a", "node-a", []resourcev1.DeviceRequestAllocationResult{
			{Driver: simulatedDriverName, Pool: "pool-a", Device: "dev-0"},
			{Driver: simulatedDriverName, Pool: "pool-b", Device: "dev-0"},
			{Driver: "alternate.draforge.oaslananka", Pool: "pool-a", Device: "dev-0"},
			{Driver: simulatedDriverName, Pool: "pool-a", Device: "dev-0"},
		}),
	}

	spec := buildCDISpec(claims, "node-a")
	if len(spec.Devices) != 3 {
		t.Fatalf("expected three fully-qualified devices, got %d: %+v", len(spec.Devices), spec.Devices)
	}
	names := make(map[string]struct{}, len(spec.Devices))
	for _, device := range spec.Devices {
		if _, exists := names[device.Name]; exists {
			t.Fatalf("fully-qualified devices collided at CDI name %q", device.Name)
		}
		names[device.Name] = struct{}{}
		env := strings.Join(device.ContainerEdits.Env, "\n")
		for _, expected := range []string{
			"DRAFORGE_VIRTUAL_IDENTITY=",
			"DRAFORGE_VIRTUAL_DRIVER=",
			"DRAFORGE_VIRTUAL_DEVICE=dev-0",
			"DRAFORGE_CLAIM_NAMESPACE=default",
		} {
			if !strings.Contains(env, expected) {
				t.Fatalf("device %q is missing %q in env: %s", device.Name, expected, env)
			}
		}
	}

	again := buildCDISpec(claims, "node-a")
	firstJSON, _ := json.Marshal(spec)
	secondJSON, _ := json.Marshal(again)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("CDI generation is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestNodeRefreshPreservesValidStateOnKubernetesFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), cdiFileName)
	knownGood := []byte(`{"cdiVersion":"0.5.0","kind":"draforge.oaslananka/sim","devices":[{"name":"known-good","containerEdits":{"env":[]}}]}`)
	if err := os.WriteFile(path, knownGood, 0o644); err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "resourceclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("kubernetes unavailable")
	})
	status := newDriverStatus(outputModeNode)
	driver := newCDIDriver(outputModeNode, client, "node-a", path, newAtomicCDIWriter(), status)

	if err := driver.Refresh(context.Background()); err == nil {
		t.Fatal("expected Kubernetes API failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(knownGood) {
		t.Fatalf("API failure replaced last-known-good CDI document: %s", got)
	}
	if ready, reason := status.Readiness(); ready || reason != readinessReasonKubernetesAPI {
		t.Fatalf("readiness = %v/%q, want false/%q", ready, reason, readinessReasonKubernetesAPI)
	}
}

func TestNodeRefreshRecoversAfterKubernetesFailure(t *testing.T) {
	claim := allocatedClaim("default", "claim-a", "node-a", []resourcev1.DeviceRequestAllocationResult{
		{Driver: simulatedDriverName, Pool: "pool-a", Device: "dev-0"},
	})
	client := fake.NewSimpleClientset(&claim)
	calls := 0
	client.PrependReactor("list", "resourceclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		if calls == 1 {
			return true, nil, errors.New("temporary outage")
		}
		return false, nil, nil
	})
	path := filepath.Join(t.TempDir(), cdiFileName)
	status := newDriverStatus(outputModeNode)
	driver := newCDIDriver(outputModeNode, client, "node-a", path, newAtomicCDIWriter(), status)

	if err := driver.Refresh(context.Background()); err == nil {
		t.Fatal("expected first refresh to fail")
	}
	if err := driver.Refresh(context.Background()); err != nil {
		t.Fatalf("expected refresh recovery, got %v", err)
	}
	if ready, reason := status.Readiness(); !ready || reason != readinessReasonReady {
		t.Fatalf("readiness = %v/%q, want true/%q", ready, reason, readinessReasonReady)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "DRAFORGE_VIRTUAL_POOL=pool-a") {
		t.Fatalf("recovered CDI document does not contain allocated device: %s", data)
	}
}

func TestDemoRefreshIsExplicitAndIndependentFromKubernetes(t *testing.T) {
	path := filepath.Join(t.TempDir(), cdiFileName)
	status := newDriverStatus(outputModeDemo)
	driver := newCDIDriver(outputModeDemo, nil, "demo-node", path, newAtomicCDIWriter(), status)
	if err := driver.Refresh(context.Background()); err != nil {
		t.Fatalf("demo refresh failed: %v", err)
	}
	if ready, _ := status.Readiness(); !ready {
		t.Fatal("demo mode should be ready after writing its explicit static CDI document")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "demo/dev-0") {
		t.Fatalf("demo CDI document is not explicit: %s", data)
	}
}

func TestDriverHealthSeparatesLivenessAndReadiness(t *testing.T) {
	status := newDriverStatus(outputModeNode)
	handler := newHealthHandler(status)

	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthResp.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want 200", healthResp.Code)
	}

	readyResp := httptest.NewRecorder()
	handler.ServeHTTP(readyResp, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if readyResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("initial readiness status = %d, want 503", readyResp.Code)
	}
	status.MarkReady(2)

	recovered := httptest.NewRecorder()
	handler.ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered readiness status = %d, want 200", recovered.Code)
	}
	if !strings.Contains(recovered.Body.String(), `"devices":2`) {
		t.Fatalf("readiness body is missing device count: %s", recovered.Body.String())
	}
}

func allocatedClaim(namespace, name, nodeName string, results []resourcev1.DeviceRequestAllocationResult) resourcev1.ResourceClaim {
	return resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				NodeSelector: nodeSelector(nodeName),
				Devices:      resourcev1.DeviceAllocationResult{Results: results},
			},
		},
	}
}

func nodeSelector(nodeName string) *corev1.NodeSelector {
	return &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
		MatchFields: []corev1.NodeSelectorRequirement{{
			Key:      "metadata.name",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{nodeName},
		}},
	}}}
}
