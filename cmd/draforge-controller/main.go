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
	versionVal = "dev"
	commitSHA  = "unknown"
)

func main() {
	var kubeconfig string
	var metricsAddr string
	var readinessTimeout time.Duration
	var readinessGracePeriod time.Duration
	var shutdownTimeout time.Duration
	leaderOptions := leaderElectionOptions{}
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8082", "Address for controller health and metrics endpoints")
	flag.DurationVar(&readinessTimeout, "readiness-timeout", health.DefaultReadinessTimeout, "Maximum duration of one Kubernetes API readiness check")
	flag.DurationVar(&readinessGracePeriod, "readiness-grace-period", health.DefaultReadinessGracePeriod, "Grace period for transient Kubernetes API readiness failures")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 5*time.Second, "Maximum graceful runtime server shutdown duration")
	flag.BoolVar(&leaderOptions.Enabled, "leader-elect", true, "Use a Kubernetes Lease so only one controller replica performs writes")
	flag.StringVar(&leaderOptions.LeaseName, "leader-election-lease-name", defaultLeaderLeaseName, "Name of the Kubernetes Lease used for controller leadership")
	flag.StringVar(&leaderOptions.LeaseNamespace, "leader-election-lease-namespace", "", "Namespace of the leader Lease; defaults to POD_NAMESPACE or default")
	flag.StringVar(&leaderOptions.Identity, "leader-election-identity", "", "Leader candidate identity; defaults to POD_NAME or the hostname")
	flag.DurationVar(&leaderOptions.LeaseDuration, "leader-election-lease-duration", defaultLeaderLeaseDuration, "Duration non-leaders wait before attempting lease takeover")
	flag.DurationVar(&leaderOptions.RenewDeadline, "leader-election-renew-deadline", defaultLeaderRenewDeadline, "Maximum time the leader retries lease renewal")
	flag.DurationVar(&leaderOptions.RetryPeriod, "leader-election-retry-period", defaultLeaderRetryPeriod, "Delay between leader-election attempts")
	flag.Parse()

	resolvedLeaderOptions, err := resolveProcessLeaderElectionOptions(leaderOptions)
	if err != nil {
		fmt.Printf("Invalid leader-election configuration: %v\n", err)
		os.Exit(1)
	}

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
	defer reconciler.Close()

	readiness := health.NewKubernetesReadinessProbe(clientset, readinessTimeout, readinessGracePeriod)
	runtimeServer := newRuntimeServer(metricsAddr, reconciler, readiness)
	go func() {
		fmt.Printf("Controller runtime server listening on %s...\n", metricsAddr)
		if err := runtimeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("Controller runtime server error: %v\n", err)
			stop()
		}
	}()

	activeController := func(activeContext context.Context) error {
		return reconciler.StartEventDrivenController(activeContext)
	}

	lifecycleDone := make(chan error, 1)
	go func() {
		lifecycleDone <- runControllerLifecycle(ctx, clientset, resolvedLeaderOptions, reconciler, activeController)
	}()

	select {
	case lifecycleErr := <-lifecycleDone:
		if lifecycleErr != nil {
			fmt.Printf("Controller lifecycle error: %v\n", lifecycleErr)
		}
		stop()
	case <-ctx.Done():
		if lifecycleErr := <-lifecycleDone; lifecycleErr != nil {
			fmt.Printf("Controller lifecycle stop error: %v\n", lifecycleErr)
		}
	}

	ctx2, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := runtimeServer.Shutdown(ctx2); err != nil {
		fmt.Printf("Controller runtime server stop error: %v\n", err)
	}
	fmt.Println("DRAForge Simulation Controller stopped.")
}
