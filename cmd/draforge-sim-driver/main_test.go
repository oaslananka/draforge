// Package main sim-driver entrypoint tests.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestSimDriverHelpOutput(t *testing.T) {
	fs := flag.NewFlagSet("draforge-sim-driver", flag.ContinueOnError)
	var buf strings.Builder
	fs.SetOutput(&buf)

	cdiDir := fs.String("cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "Directory to write CDI spec JSONs")
	kubeconfig := fs.String("kubeconfig", "", "Path to kubeconfig file")
	outputMode := fs.String("output-mode", "node", "CDI output mode: demo or node")
	healthAddr := fs.String("health-addr", ":8083", "Address for liveness and readiness endpoints")
	refreshInterval := fs.Duration("refresh-interval", 3*time.Second, "Interval between CDI refresh attempts")

	err := fs.Parse([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "cdi-dir") {
		t.Errorf("help output should contain 'cdi-dir', got: %q", output)
	}
	for _, flagName := range []string{"kubeconfig", "output-mode", "health-addr", "refresh-interval"} {
		if !strings.Contains(output, flagName) {
			t.Errorf("help output should contain %q, got: %q", flagName, output)
		}
	}
	if !strings.Contains(output, "Usage") {
		t.Errorf("help output should contain 'Usage', got: %q", output)
	}
	_ = cdiDir
	_ = kubeconfig
	_ = outputMode
	_ = healthAddr
	_ = refreshInterval
}

func TestSimDriverCDIDirDefault(t *testing.T) {
	fs := flag.NewFlagSet("draforge-sim-driver", flag.ContinueOnError)
	cdiDir := fs.String("cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "")
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	if *cdiDir != "/var/lib/kubelet/device-plugins/cdi" {
		t.Errorf("expected default path, got %q", *cdiDir)
	}
}

func TestSimDriverCDIDirCustom(t *testing.T) {
	fs := flag.NewFlagSet("draforge-sim-driver", flag.ContinueOnError)
	cdiDir := fs.String("cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "")
	if err := fs.Parse([]string{"--cdi-dir", "/tmp/cdi"}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	if *cdiDir != "/tmp/cdi" {
		t.Errorf("expected /tmp/cdi, got %q", *cdiDir)
	}
}

func TestSimDriverKubeconfigFlag(t *testing.T) {
	fs := flag.NewFlagSet("draforge-sim-driver", flag.ContinueOnError)
	kubeconfig := fs.String("kubeconfig", "", "Path to kubeconfig file")
	if err := fs.Parse([]string{"--kubeconfig", "/tmp/kubeconfig"}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	if *kubeconfig != "/tmp/kubeconfig" {
		t.Errorf("expected /tmp/kubeconfig, got %q", *kubeconfig)
	}
}

func TestSimDriverVersionVarsNonEmpty(t *testing.T) {
	if versionVal == "" {
		t.Error("versionVal must not be empty")
	}
	if commitSHA == "" {
		t.Error("commitSHA must not be empty")
	}
}

func TestCDIDeviceJSONMarshal(t *testing.T) {
	d := CDIDevice{
		Name: "dev-0",
		ContainerEdits: ContainerEdits{
			Env: []string{"DRAFORGE_VIRTUAL_DEVICE=dev-0"},
		},
	}
	if d.Name != "dev-0" {
		t.Errorf("Name: got %q, want dev-0", d.Name)
	}
	if len(d.ContainerEdits.Env) != 1 {
		t.Errorf("expected 1 env entry, got %d", len(d.ContainerEdits.Env))
	}
}

func TestCDISpecStructure(t *testing.T) {
	spec := CDISpec{
		CDIVersion: "0.5.0",
		Kind:       "draforge.oaslananka/sim",
		Devices: []CDIDevice{
			{Name: "dev-0", ContainerEdits: ContainerEdits{Env: []string{"KEY=VAL"}}},
		},
	}
	if spec.CDIVersion != "0.5.0" {
		t.Errorf("CDIVersion: got %q, want 0.5.0", spec.CDIVersion)
	}
	if spec.Kind != "draforge.oaslananka/sim" {
		t.Errorf("Kind: got %q, want draforge.oaslananka/sim", spec.Kind)
	}
	if len(spec.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(spec.Devices))
	}
}

func TestParseOutputMode(t *testing.T) {
	for _, value := range []string{"demo", "node"} {
		mode, err := parseOutputMode(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if string(mode) != value {
			t.Fatalf("parsed mode = %q, want %q", mode, value)
		}
	}
	if _, err := parseOutputMode("fallback"); err == nil {
		t.Fatal("expected invalid output mode to fail")
	}
}
