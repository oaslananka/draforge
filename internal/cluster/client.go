// Package cluster provides helpers to interact with the Kubernetes cluster.
// SPDX-License-Identifier: Apache-2.0
package cluster

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// GetConfig returns a Kubernetes rest.Config.
// It loads in-cluster config first, then falls back to kubeconfig from path or default location.
func GetConfig(kubeconfigPath string) (*rest.Config, error) {
	// Try in-cluster config first (e.g. running inside DOKS)
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fallback to kubeconfig file
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	// Check if file exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		// If custom path is empty or default doesn't exist, try loading empty config (will fail but good default)
		return nil, err
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// NewClientset returns standard typed and dynamic clients.
func NewClientset(kubeconfigPath string) (*kubernetes.Clientset, dynamic.Interface, *rest.Config, error) {
	config, err := GetConfig(kubeconfigPath)
	if err != nil {
		return nil, nil, nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, err
	}

	return clientset, dynamicClient, config, nil
}
