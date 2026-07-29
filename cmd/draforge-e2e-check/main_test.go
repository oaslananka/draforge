// Package main tests the remote E2E result verifier command.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeResultFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write result file: %v", err)
	}
	return path
}

func TestRunVerifiesPassingRemoteResults(t *testing.T) {
	path := writeResultFile(t, strings.Join([]string{
		`{"Action":"run","Test":"TestSmoke"}`,
		`{"Action":"pass","Test":"TestSmoke"}`,
		`{"Action":"pass"}`,
	}, "\n"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--file", path, "--required-test", "TestSmoke"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run code = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Remote E2E results verified") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunReportsUsageAndInputFailures(t *testing.T) {
	t.Run("missing file flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(nil, &stdout, &stderr); code != 2 {
			t.Fatalf("run code = %d, want 2", code)
		}
		if !strings.Contains(stderr.String(), "--file is required") {
			t.Fatalf("unexpected stderr: %q", stderr.String())
		}
	})

	t.Run("missing file", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--file", filepath.Join(t.TempDir(), "missing")}, &stdout, &stderr); code != 2 {
			t.Fatalf("run code = %d, want 2", code)
		}
		if !strings.Contains(stderr.String(), "open test results") {
			t.Fatalf("unexpected stderr: %q", stderr.String())
		}
	})

	t.Run("failed required test", func(t *testing.T) {
		path := writeResultFile(t, strings.Join([]string{
			`{"Action":"run","Test":"TestSmoke"}`,
			`{"Action":"fail","Test":"TestSmoke"}`,
			`{"Action":"fail"}`,
		}, "\n"))
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--file", path}, &stdout, &stderr); code != 1 {
			t.Fatalf("run code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "verification failed") {
			t.Fatalf("unexpected stderr: %q", stderr.String())
		}
	})
}
