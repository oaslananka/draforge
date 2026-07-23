// cmd/draforge-controller/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/health"
	"github.com/oaslananka/draforge/internal/simulator"
)

var (
	versionVal = "v0.1.0"
	commitSHA  = "dev"
)

func main() {
	var kubeconfig string
	var metricsAddr string
	var readinessTimeout time.Duration
	var readinessGracePeriod time.Duration
	var shutdownTimeout time.Duration
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8082", "Address for controller health and metrics endpoints")
	flag.DurationVar(&readinessTimeout, "readiness-timeout", health.DefaultReadinessTimeout, "Maximum duration of one Kubernetes API readiness check")
	flag.DurationVar(&readinessGracePeriod, "readiness-grace-period", health.DefaultReadinessGracePeriod, "Grace period for transient Kubernetes API readiness failures")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 5*time.Second, "Maximum graceful runtime server shutdown duration")
	flag.Parse()

	// 1. Initialize cluster clients
	clientset, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubernetes clients: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("DRAForge Simulation Controller %s (commit: %s) starting...\n", versionVal, commitSHA)

	// 2. Initialize Reconciler
	reconciler := simulator.NewReconciler(clientset, dynamicClient)

	readiness := health.NewKubernetesReadinessProbe(clientset, readinessTimeout, readinessGracePeriod)
	runtimeServer := newRuntimeServer(metricsAddr, reconciler, readiness)
	go func() {
		fmt.Printf("Controller runtime server listening on %s...\n", metricsAddr)
		if err := runtimeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("Controller runtime server error: %v\n", err)
			stop()
		}
	}()

	// 3. Start reconciliation loop (sync pools to ResourceSlices)
	go reconciler.StartReconciliationLoop(ctx, 10*time.Second)

	// 4. Start allocation simulator loop (fallback scheduler logic)
	go reconciler.StartAllocationSimulator(ctx, 5*time.Second)

	// Keep running until signal received
	<-ctx.Done()

	ctx2, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := runtimeServer.Shutdown(ctx2); err != nil {
		fmt.Printf("Controller runtime server stop error: %v\n", err)
	}
	fmt.Println("DRAForge Simulation Controller stopped.")
}
