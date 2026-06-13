// cmd/draforge-controller/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/simulator"
)

func main() {
	var kubeconfig string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file")
	flag.Parse()

	// 1. Initialize cluster clients
	clientset, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubernetes clients: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Initializing DRAForge Simulation Controller...")

	// 2. Initialize Reconciler
	reconciler := simulator.NewReconciler(clientset, dynamicClient)

	// 3. Start reconciliation loop (sync pools to ResourceSlices)
	go reconciler.StartReconciliationLoop(ctx, 10*time.Second)

	// 4. Start allocation simulator loop (fallback scheduler logic)
	go reconciler.StartAllocationSimulator(ctx, 5*time.Second)

	// Keep running until signal received
	<-ctx.Done()
	fmt.Println("DRAForge Simulation Controller stopped.")
}
