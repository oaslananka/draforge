// Package main CLI tests.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	"github.com/spf13/cobra"
)

// executeCommand runs the DRAForge CLI with the given arguments and
// returns the combined stdout+stderr output and any error.
func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// --- Output contract tests ---

func TestVersionOutput(t *testing.T) {
	root := NewRootCommand()
	out, err := executeCommand(root, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if !strings.Contains(out, "DRAForge") {
		t.Errorf("version output should contain 'DRAForge', got: %q", out)
	}
	if !strings.Contains(out, "Commit:") {
		t.Errorf("version output should contain 'Commit:', got: %q", out)
	}
}

func TestVersionOutputDeterministic(t *testing.T) {
	root1 := NewRootCommand()
	out1, _ := executeCommand(root1, "version")
	root2 := NewRootCommand()
	out2, _ := executeCommand(root2, "version")
	if out1 != out2 {
		t.Error("version output is not deterministic")
	}
}

func TestClaimsTableNoNullByte(t *testing.T) {
	// claims command needs a cluster, so it will error on kubeconfig.
	// But the error path should not contain null bytes.
	root := NewRootCommand()
	out, err := executeCommand(root, "claims")
	// Expect error because no kubeconfig / no cluster
	if err == nil {
		t.Log("claims succeeded (unexpected but valid if cluster available)")
	}
	// Regardless of success/failure, output must not contain null bytes
	if strings.Contains(out, "\x00") {
		t.Error("claims output contains null byte (\\x00)")
	}
}

func TestClaimsOutputFormatJSON(t *testing.T) {
	root := NewRootCommand()
	out, err := executeCommand(root, "claims", "-o", "json")
	// Claims requires a real cluster, so expect error
	if err == nil {
		// If it succeeded, verify JSON is parseable
		if !json.Valid([]byte(out)) {
			t.Errorf("claims -o json output is not valid JSON: %q", out)
		}
		return
	}
	// Error is expected — verify no null bytes in error message
	if strings.Contains(out, "\x00") {
		t.Error("claims error output contains null byte")
	}
}

// --- Flag validation error tests ---

func TestScenarioApplyMissingFile(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "scenario", "apply")
	if err == nil {
		t.Fatal("expected error for scenario apply without -f flag, got nil")
	}
	if !strings.Contains(err.Error(), "scenario file path must be specified") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestInjectFaultMissingPool(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "inject-fault")
	if err == nil {
		t.Fatal("expected error for inject-fault without --pool flag, got nil")
	}
	if !strings.Contains(err.Error(), "--pool must be specified") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestInjectFaultMissingType(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "inject-fault", "--pool", "test-pool")
	if err == nil {
		t.Fatal("expected error for inject-fault without --type flag, got nil")
	}
	if !strings.Contains(err.Error(), "--type must be specified") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestInjectFaultInvalidType(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "inject-fault", "--pool", "test-pool", "--type", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid fault type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid fault type") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// --- Output format syntax tests ---

func TestGraphOutputFormats(t *testing.T) {
	// Graph needs a cluster, so test the output syntax via argument parsing
	// and verify error messages don't have null bytes.
	root := NewRootCommand()

	// DOT format
	out, err := executeCommand(root, "graph", "-o", "dot")
	if err == nil {
		// If it succeeded, verify DOT syntax
		if !strings.Contains(out, "digraph") {
			t.Errorf("dot output should start with 'digraph', got: %q", out)
		}
	}
	_ = out

	// Mermaid format
	out2, err2 := executeCommand(root, "graph", "-o", "mermaid")
	if err2 == nil {
		// If it succeeded, verify Mermaid syntax
		if !strings.Contains(out2, "graph LR") {
			t.Errorf("mermaid output should start with 'graph LR', got: %q", out2)
		}
	}
	_ = out2

	// JSON format
	out3, err3 := executeCommand(root, "graph", "-o", "json")
	if err3 == nil {
		if !json.Valid([]byte(out3)) {
			t.Errorf("graph -o json output is not valid JSON: %q", out3)
		}
	}
	_ = out3
}

// --- Doctor output format tests ---

func TestDoctorOutputFormats(t *testing.T) {
	root := NewRootCommand()

	// JSON format
	out, err := executeCommand(root, "doctor", "-o", "json")
	if err == nil {
		if !json.Valid([]byte(out)) {
			t.Errorf("doctor -o json output is not valid JSON: %q", out)
		}
	}
	_ = out

	// Table format — no null bytes
	out2, _ := executeCommand(root, "doctor")
	if strings.Contains(out2, "\x00") {
		t.Error("doctor table output contains null byte")
	}
}

// --- Serve command flag tests ---

func TestServeHasServerPortFlag(t *testing.T) {
	root := NewRootCommand()
	out, err := executeCommand(root, "serve", "--help")
	if err != nil {
		t.Fatalf("serve --help failed: %v", err)
	}
	if !strings.Contains(out, "--server-port") {
		t.Errorf("serve --help should mention --server-port flag, got: %q", out)
	}
	if !strings.Contains(out, "-p") {
		t.Errorf("serve --help should mention -p shorthand, got: %q", out)
	}
	if !strings.Contains(out, "8080") {
		t.Errorf("serve --help should mention default port 8080, got: %q", out)
	}
}

func TestServePortFlagRegistered(t *testing.T) {
	root := NewRootCommand()
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("could not find serve subcommand: %v", err)
	}
	flag := serveCmd.Flag("server-port")
	if flag == nil {
		t.Fatal("--server-port flag not registered on serve subcommand")
	}
	if flag.DefValue != "8080" {
		t.Errorf("expected default 8080, got %q", flag.DefValue)
	}
}

// --- Graph output no-null-byte ---

func TestGraphOutputNoNullByte(t *testing.T) {
	root := NewRootCommand()
	out, _ := executeCommand(root, "graph")
	// Error or not, no null bytes
	if strings.Contains(out, "\x00") {
		t.Error("graph output contains null byte")
	}
}

// --- Clear-faults missing pool test ---

func TestClearFaultsMissingPool(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "clear-faults")
	if err == nil {
		t.Fatal("expected error for clear-faults without --pool flag, got nil")
	}
	if !strings.Contains(err.Error(), "--pool must be specified") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// --- Discover output contract ---

func TestDiscoverOutputJSON(t *testing.T) {
	root := NewRootCommand()
	out, err := executeCommand(root, "discover", "-o", "json")
	if err == nil {
		if !json.Valid([]byte(out)) {
			t.Errorf("discover -o json output is not valid JSON: %q", out)
		}
	}
	_ = out
}

func TestDiscoverNoNullByte(t *testing.T) {
	root := NewRootCommand()
	out, _ := executeCommand(root, "discover")
	if strings.Contains(out, "\x00") {
		t.Error("discover output contains null byte")
	}
}

// --- Explain command tests ---

func TestExplainMissingClaim(t *testing.T) {
	root := NewRootCommand()
	_, err := executeCommand(root, "explain")
	if err == nil {
		t.Fatal("expected error for explain without claim argument, got nil")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestExplainOutputJSON(t *testing.T) {
	root := NewRootCommand()
	out, err := executeCommand(root, "explain", "dummy-claim", "-o", "json")
	if err == nil {
		if !json.Valid([]byte(out)) {
			t.Errorf("explain -o json output is not valid JSON: %q", out)
		}
	}
}

func TestClaimFormattingPreservesAllClassesAndAllocations(t *testing.T) {
	claim := model.ResourceClaimInfo{
		Requests: []model.ClaimRequest{
			{Name: "gpu", Mode: "Exactly", Alternatives: []model.ClaimRequestAlternative{{DeviceClassName: "gpu-class"}}},
			{Name: "accelerator", Mode: "FirstAvailable", Alternatives: []model.ClaimRequestAlternative{{DeviceClassName: "nic-class"}, {DeviceClassName: "fpga-class"}}},
		},
		Allocations: []model.ClaimAllocation{
			{Request: "gpu", DriverName: "driver-a.example", PoolName: "shared", DeviceName: "dev-0", NodeName: "node-a"},
			{Request: "accelerator/nic", DriverName: "driver-b.example", PoolName: "shared", DeviceName: "dev-0"},
		},
	}
	if got, want := formatClaimClasses(claim), "fpga-class,gpu-class,nic-class"; got != want {
		t.Fatalf("formatClaimClasses = %q, want %q", got, want)
	}
	allocations := formatClaimAllocations(claim)
	for _, want := range []string{
		"gpu=driver-a.example/shared/dev-0@node-a",
		"accelerator/nic=driver-b.example/shared/dev-0@unknown-node",
	} {
		if !strings.Contains(allocations, want) {
			t.Fatalf("formatClaimAllocations missing %q: %q", want, allocations)
		}
	}
}
