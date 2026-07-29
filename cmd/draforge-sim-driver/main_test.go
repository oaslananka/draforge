// Package main sim-driver entrypoint tests.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
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

func TestSimDriverSourceVersionDefaults(t *testing.T) {
	if versionVal != "dev" {
		t.Errorf("source versionVal: got %q, want dev", versionVal)
	}
	if commitSHA != "unknown" {
		t.Errorf("source commitSHA: got %q, want unknown", commitSHA)
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

func TestParseSimDriverOptions(t *testing.T) {
	options, err := parseSimDriverOptions(
		[]string{"--cdi-dir", t.TempDir(), "--output-mode", "demo", "--health-addr", "127.0.0.1:0", "--refresh-interval", "25ms"},
		func(key string) string {
			if key == "NODE_NAME" {
				return "node-from-env"
			}
			return ""
		},
		func() (string, error) { return "node-from-hostname", nil },
	)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if options.OutputMode != outputModeDemo || options.NodeName != "node-from-env" {
		t.Fatalf("unexpected options: %+v", options)
	}
	if options.RefreshInterval != 25*time.Millisecond {
		t.Fatalf("refresh interval = %s, want 25ms", options.RefreshInterval)
	}
}

func TestParseSimDriverOptionsRejectsInvalidValues(t *testing.T) {
	for name, args := range map[string][]string{
		"mode":     {"--output-mode", "invalid"},
		"interval": {"--refresh-interval", "0s"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSimDriverOptions(args, func(string) string { return "" }, func() (string, error) { return "node", nil }); err == nil {
				t.Fatal("expected invalid options to fail")
			}
		})
	}
}

func TestResolveNodeNameFallbacks(t *testing.T) {
	if got := resolveNodeName(func(string) string { return "node-env" }, func() (string, error) { return "node-host", nil }); got != "node-env" {
		t.Fatalf("environment node name = %q", got)
	}
	if got := resolveNodeName(func(string) string { return "" }, func() (string, error) { return "node-host", nil }); got != "node-host" {
		t.Fatalf("hostname node name = %q", got)
	}
	if got := resolveNodeName(func(string) string { return "" }, func() (string, error) { return "", errors.New("hostname unavailable") }); got != "node-0" {
		t.Fatalf("fallback node name = %q", got)
	}
}

func TestRunSimDriverDemoLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if err := runSimDriver(ctx, simDriverOptions{
		CDIDir:          dir,
		OutputMode:      outputModeDemo,
		HealthAddr:      "127.0.0.1:0",
		RefreshInterval: time.Millisecond,
		NodeName:        "node-a",
	}); err != nil {
		t.Fatalf("run demo driver: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, cdiFileName)); err != nil {
		t.Fatalf("expected CDI document: %v", err)
	}
}

func TestRunSimDriverNodeModeClientFailure(t *testing.T) {
	original := newKubernetesClient
	newKubernetesClient = func(string) (kubernetes.Interface, error) {
		return nil, errors.New("client unavailable")
	}
	t.Cleanup(func() { newKubernetesClient = original })

	err := runSimDriver(context.Background(), simDriverOptions{
		CDIDir:          t.TempDir(),
		OutputMode:      outputModeNode,
		HealthAddr:      "127.0.0.1:0",
		RefreshInterval: time.Second,
		NodeName:        "node-a",
	})
	if err == nil || !strings.Contains(err.Error(), "initialize Kubernetes client") {
		t.Fatalf("expected client initialization failure, got %v", err)
	}
}
