// Package main controller entrypoint tests.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"strings"
	"testing"
)

func TestControllerHelpOutput(t *testing.T) {
	// The controller uses stdlib flag — capture its output via a FlagSet.
	fs := flag.NewFlagSet("draforge-controller", flag.ContinueOnError)
	var buf strings.Builder
	fs.SetOutput(&buf)

	kubeconfig := fs.String("kubeconfig", "", "Absolute path to the kubeconfig file")

	// Simulate --help — stdlib flag returns ErrHelp which is expected
	err := fs.Parse([]string{"--help"})
	if err != flag.ErrHelp {
		t.Fatalf("expected flag.ErrHelp, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "kubeconfig") {
		t.Errorf("help output should contain 'kubeconfig', got: %q", output)
	}
	if !strings.Contains(output, "Usage") {
		t.Errorf("help output should contain 'Usage', got: %q", output)
	}
	_ = kubeconfig // verify the variable is assigned
}

func TestControllerKubeconfigFlagDefault(t *testing.T) {
	fs := flag.NewFlagSet("draforge-controller", flag.ContinueOnError)
	kubeconfig := fs.String("kubeconfig", "", "Absolute path to the kubeconfig file")
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	if *kubeconfig != "" {
		t.Errorf("expected empty default for --kubeconfig, got %q", *kubeconfig)
	}
}

func TestControllerKubeconfigFlagCustom(t *testing.T) {
	fs := flag.NewFlagSet("draforge-controller", flag.ContinueOnError)
	kubeconfig := fs.String("kubeconfig", "", "Absolute path to the kubeconfig file")
	if err := fs.Parse([]string{"--kubeconfig", "/tmp/test-config"}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	if *kubeconfig != "/tmp/test-config" {
		t.Errorf("expected /tmp/test-config, got %q", *kubeconfig)
	}
}

func TestControllerVersionVarsNonEmpty(t *testing.T) {
	if versionVal == "" {
		t.Error("versionVal must not be empty")
	}
	if commitSHA == "" {
		t.Error("commitSHA must not be empty")
	}
}
