// Package cluster unit tests.
// SPDX-License-Identifier: Apache-2.0
package cluster

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfig_NonExistentPath(t *testing.T) {
	_, err := GetConfig("/tmp/nonexistent/kubeconfig")
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

func TestGetConfig_ValidKubeconfigPath(t *testing.T) {
	// Create a temporary valid kubeconfig file and verify no crash.
	dir := t.TempDir()
	kpath := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kpath, []byte(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := GetConfig(kpath)
	if err != nil {
		t.Fatalf("expected no error for valid kubeconfig, got: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Host != "https://127.0.0.1:6443" {
		t.Errorf("expected host 127.0.0.1:6443, got %s", config.Host)
	}
}

func TestNewClientset_NoCluster(t *testing.T) {
	// With a non-existent path, NewClientset should return an error.
	_, _, _, err := NewClientset("/tmp/nonexistent/kubeconfig")
	if err == nil {
		t.Fatal("expected error for NewClientset with non-existent kubeconfig, got nil")
	}
}

func TestNewClientset_ValidKubeconfig(t *testing.T) {
	dir := t.TempDir()
	kpath := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kpath, []byte(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`), 0644); err != nil {
		t.Fatal(err)
	}

	clientset, dynamicClient, config, err := NewClientset(kpath)
	if err != nil {
		t.Fatalf("expected no error for valid kubeconfig, got: %v", err)
	}
	if clientset == nil {
		t.Error("expected non-nil clientset")
	}
	if dynamicClient == nil {
		t.Error("expected non-nil dynamic client")
	}
	if config == nil {
		t.Error("expected non-nil rest config")
	}
}
