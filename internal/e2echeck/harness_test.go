// Package e2echeck tests the checked-in remote E2E harness contract.
// SPDX-License-Identifier: Apache-2.0
package e2echeck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join("..", "..", relative)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(data)
}

func TestRemoteHarnessExecutesTaggedSmokeTest(t *testing.T) {
	runner := readRepoFile(t, "scripts/run-remote-e2e-tests.sh")
	for _, required := range []string{
		"DRAFORGE_E2E=1",
		"go test -count=1 -json -tags=e2e ./tests/e2e/...",
		"--required-test TestSmoke",
	} {
		if !strings.Contains(runner, required) {
			t.Fatalf("runner is missing %q", required)
		}
	}

	job := readRepoFile(t, "build/remote/e2e-job.yaml")
	for _, required := range []string{
		"serviceAccountName: draforge-e2e-RUN_ID",
		"./scripts/run-remote-e2e-tests.sh",
		"value: /artifacts/go-test.json",
	} {
		if !strings.Contains(job, required) {
			t.Fatalf("job manifest is missing %q", required)
		}
	}
}

func TestRemoteHarnessRBACIsReadOnlyAndScoped(t *testing.T) {
	rbac := readRepoFile(t, "build/remote/e2e-rbac.yaml")
	for _, required := range []string{
		`resources: ["pods", "nodes"]`,
		`resources: ["resourceclaims", "resourceslices"]`,
		`verbs: ["get", "list"]`,
		`verbs: ["get"]`,
	} {
		if !strings.Contains(rbac, required) {
			t.Fatalf("RBAC manifest is missing %q", required)
		}
	}
	for _, forbidden := range []string{`"create"`, `"update"`, `"patch"`, `"delete"`, `"watch"`} {
		if strings.Contains(rbac, forbidden) {
			t.Fatalf("RBAC manifest contains write or unnecessary verb %s", forbidden)
		}
	}
}

func TestRemoteSmokeTestDoesNotSkipClusterFailures(t *testing.T) {
	smoke := readRepoFile(t, "tests/e2e/e2e_test.go")
	if strings.Contains(smoke, "no active cluster connection configured") {
		t.Fatal("cluster connection failures must fail the remote smoke test, not skip it")
	}
	if !strings.Contains(smoke, "t.Fatalf(\"E2E smoke test requires an active cluster connection") {
		t.Fatal("remote smoke test must fail when the cluster client cannot be created")
	}
}
