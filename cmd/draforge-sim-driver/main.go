// cmd/draforge-sim-driver/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	var cdiDir string
	flag.StringVar(&cdiDir, "cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "Directory to write CDI spec JSONs")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Starting DRAForge Synthetic Node Plugin (CDI dir: %s)...\n", cdiDir)

	// Ensure CDI directory is writable or simulate
	if err := os.MkdirAll(cdiDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create CDI directory %s: %v. Using temporary simulation path.\n", cdiDir, err)
		cdiDir = os.TempDir()
	}

	// Write basic simulated CDI specification
	cdiPath := filepath.Join(cdiDir, "draforge-sim-devices.json")
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

	if err := os.WriteFile(cdiPath, []byte(mockCDISpec), 0644); err != nil {
		fmt.Printf("Error writing simulated CDI spec: %v\n", err)
	} else {
		fmt.Printf("Successfully published simulated CDI specification to: %s\n", cdiPath)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Keep plugin alive
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down DRAForge Node Plugin...")
			// Cleanup CDI spec on shutdown
			_ = os.Remove(cdiPath)
			fmt.Println("DRAForge Node Plugin stopped.")
			return
		case <-ticker.C:
			fmt.Println("DRAForge Node Plugin: Heartbeat check ok.")
		}
	}
}
