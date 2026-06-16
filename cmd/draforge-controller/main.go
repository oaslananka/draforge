// cmd/draforge-controller/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/simulator"
)

var (
	versionVal = "v0.1.0"
	commitSHA  = "dev"
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

	// Expose Prometheus metrics endpoint on port 8082
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "# HELP draforge_controller_reconcile_errors_total Total number of reconciliation errors\n")
			fmt.Fprintf(w, "# TYPE draforge_controller_reconcile_errors_total counter\n")
			fmt.Fprintf(w, "draforge_controller_reconcile_errors_total %d\n", atomic.LoadInt64(&reconciler.ReconcileErrorsCount))

			fmt.Fprintf(w, "# HELP draforge_controller_allocations_simulated_total Total number of simulated allocations\n")
			fmt.Fprintf(w, "# TYPE draforge_controller_allocations_simulated_total counter\n")
			fmt.Fprintf(w, "draforge_controller_allocations_simulated_total %d\n", atomic.LoadInt64(&reconciler.AllocationsSimulated))

			fmt.Fprintf(w, "# HELP draforge_controller_active_faults Total number of active faults\n")
			fmt.Fprintf(w, "# TYPE draforge_controller_active_faults gauge\n")
			fmt.Fprintf(w, "draforge_controller_active_faults %d\n", atomic.LoadInt64(&reconciler.ActiveFaultsCount))
		})
		fmt.Println("Controller metrics server listening on :8082...")
		_ = http.ListenAndServe(":8082", nil)
	}()

	// 3. Start reconciliation loop (sync pools to ResourceSlices)
	go reconciler.StartReconciliationLoop(ctx, 10*time.Second)

	// 4. Start allocation simulator loop (fallback scheduler logic)
	go reconciler.StartAllocationSimulator(ctx, 5*time.Second)

	// Keep running until signal received
	<-ctx.Done()
	fmt.Println("DRAForge Simulation Controller stopped.")
}
