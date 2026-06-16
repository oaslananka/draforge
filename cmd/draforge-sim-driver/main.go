// cmd/draforge-sim-driver/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	versionVal = "v0.1.0"
	commitSHA  = "dev"
)

// CDIDevice represents a CDI device entry.
type CDIDevice struct {
	Name           string         `json:"name"`
	ContainerEdits ContainerEdits `json:"containerEdits"`
}

type ContainerEdits struct {
	Env []string `json:"env"`
}

type CDISpec struct {
	CDIVersion string      `json:"cdiVersion"`
	Kind       string      `json:"kind"`
	Devices    []CDIDevice `json:"devices"`
}

func main() {
	var cdiDir string
	var kubeconfigPath string
	flag.StringVar(&cdiDir, "cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "Directory to write CDI spec JSONs")
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		if hostname, err := os.Hostname(); err == nil {
			nodeName = hostname
		} else {
			nodeName = "node-0"
		}
	}

	fmt.Printf("Starting DRAForge Synthetic Node Plugin (CDI dir: %s, nodeName: %s)...\n", cdiDir, nodeName)

	if err := os.MkdirAll(cdiDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create CDI directory %s: %v. Using temporary simulation path.\n", cdiDir, err)
		cdiDir = os.TempDir()
	}

	cdiPath := filepath.Join(cdiDir, "draforge-sim-devices.json")

	// Try to initialize Kubernetes clientset
	clientset, _, _, err := cluster.NewClientset(kubeconfigPath)
	var fallbackMode bool
	if err != nil {
		fmt.Printf("Warning: Failed to initialize kubernetes clientset: %v. Running in fallback mode with static spec.\n", err)
		fallbackMode = true
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Run initial update
	updateCDISpec(ctx, clientset, nodeName, cdiPath, fallbackMode)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down DRAForge Node Plugin...")
			_ = os.Remove(cdiPath)
			fmt.Println("DRAForge Node Plugin stopped.")
			return
		case <-ticker.C:
			updateCDISpec(ctx, clientset, nodeName, cdiPath, fallbackMode)
		}
	}
}

func updateCDISpec(ctx context.Context, clientset kubernetes.Interface, nodeName, cdiPath string, fallbackMode bool) {
	if fallbackMode {
		writeStaticCDISpec(cdiPath)
		return
	}

	// 1. Get all ResourceClaims
	claimList, err := clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing ResourceClaims: %v. Reverting to static spec.\n", err)
		writeStaticCDISpec(cdiPath)
		return
	}

	// 2. Identify allocated devices for our sim driver on this node
	var cdiDevices []CDIDevice
	allocatedSet := make(map[string]bool)

	for _, claim := range claimList.Items {
		if claim.Status.Allocation == nil {
			continue
		}

		// Check if allocated node matches
		nodeSelMatched := false
		if claim.Status.Allocation.NodeSelector != nil {
			for _, term := range claim.Status.Allocation.NodeSelector.NodeSelectorTerms {
				for _, req := range term.MatchFields {
					if req.Key == "metadata.name" && len(req.Values) > 0 && req.Values[0] == nodeName {
						nodeSelMatched = true
					}
				}
				for _, req := range term.MatchExpressions {
					if req.Key == "kubernetes.io/hostname" && len(req.Values) > 0 && req.Values[0] == nodeName {
						nodeSelMatched = true
					}
				}
			}
		}

		// Also check device allocation results
		for _, dev := range claim.Status.Allocation.Devices.Results {
			// If target pool matches or node selection matched, we attribute it to this node
			if nodeSelMatched || dev.Pool == nodeName {
				// We also check driver name
				if strings.Contains(dev.Driver, "sim") || dev.Driver == "sim.draforge.oaslananka" {
					if !allocatedSet[dev.Device] {
						allocatedSet[dev.Device] = true
						cdiDevices = append(cdiDevices, CDIDevice{
							Name: dev.Device,
							ContainerEdits: ContainerEdits{
								Env: []string{
									fmt.Sprintf("DRAFORGE_VIRTUAL_DEVICE=%s", dev.Device),
									"DRAFORGE_VIRTUAL_TYPE=sim-device",
									fmt.Sprintf("DRAFORGE_CLAIM_NAME=%s", claim.Name),
									fmt.Sprintf("DRAFORGE_CLAIM_NAMESPACE=%s", claim.Namespace),
								},
							},
						})
					}
				}
			}
		}
	}

	// 3. Write CDI spec file
	spec := CDISpec{
		CDIVersion: "0.5.0",
		Kind:       "draforge.oaslananka/sim",
		Devices:    cdiDevices,
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling CDI spec: %v\n", err)
		return
	}

	if err := os.WriteFile(cdiPath, data, 0644); err != nil {
		fmt.Printf("Error writing CDI spec file: %v\n", err)
	} else {
		fmt.Printf("Successfully refreshed CDI specification at %s (Active devices: %d)\n", cdiPath, len(cdiDevices))
	}
}

func writeStaticCDISpec(cdiPath string) {
	mockCDISpec := `{
  "cdiVersion": "0.5.0",
  "kind": "draforge.oaslananka/sim",
  "devices": [
    {
      "name": "dev-0",
      "containerEdits": {
        "env": [
          "DRAFORGE_VIRTUAL_DEVICE=dev-0",
          "DRAFORGE_VIRTUAL_TYPE=gpu"
        ]
      }
    },
    {
      "name": "dev-1",
      "containerEdits": {
        "env": [
          "DRAFORGE_VIRTUAL_DEVICE=dev-1",
          "DRAFORGE_VIRTUAL_TYPE=camera"
        ]
      }
    }
  ]
}`
	_ = os.WriteFile(cdiPath, []byte(mockCDISpec), 0644)
}
