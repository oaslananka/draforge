// cmd/draforge-sim-driver/main.go
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
	"path/filepath"
	"syscall"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"k8s.io/client-go/kubernetes"
)

var (
	versionVal = "dev"
	commitSHA  = "unknown"
)

type simDriverOptions struct {
	CDIDir          string
	KubeconfigPath  string
	OutputMode      outputMode
	HealthAddr      string
	RefreshInterval time.Duration
	NodeName        string
}

var newKubernetesClient = func(kubeconfigPath string) (kubernetes.Interface, error) {
	clientset, _, _, err := cluster.NewClientset(kubeconfigPath)
	return clientset, err
}

func main() {
	if err := runMain(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "draforge-sim-driver: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	options, err := parseSimDriverOptions(os.Args[1:], os.Getenv, os.Hostname)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runSimDriver(ctx, options)
}

func parseSimDriverOptions(
	args []string,
	getenv func(string) string,
	hostname func() (string, error),
) (simDriverOptions, error) {
	fs := flag.NewFlagSet("draforge-sim-driver", flag.ContinueOnError)
	var options simDriverOptions
	var modeValue string
	fs.StringVar(&options.CDIDir, "cdi-dir", "/var/lib/kubelet/device-plugins/cdi", "Directory to write CDI spec JSONs")
	fs.StringVar(&options.KubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	fs.StringVar(&modeValue, "output-mode", string(outputModeNode), "CDI output mode: demo or node")
	fs.StringVar(&options.HealthAddr, "health-addr", ":8083", "Address for liveness and readiness endpoints")
	fs.DurationVar(&options.RefreshInterval, "refresh-interval", 3*time.Second, "Interval between CDI refresh attempts")
	if err := fs.Parse(args); err != nil {
		return simDriverOptions{}, err
	}

	mode, err := parseOutputMode(modeValue)
	if err != nil {
		return simDriverOptions{}, err
	}
	if options.RefreshInterval <= 0 {
		return simDriverOptions{}, errors.New("refresh interval must be positive")
	}
	options.OutputMode = mode
	options.NodeName = resolveNodeName(getenv, hostname)
	return options, nil
}

func resolveNodeName(getenv func(string) string, hostname func() (string, error)) string {
	if nodeName := getenv("NODE_NAME"); nodeName != "" {
		return nodeName
	}
	if resolved, err := hostname(); err == nil && resolved != "" {
		return resolved
	}
	return "node-0"
}

func runSimDriver(ctx context.Context, options simDriverOptions) error {
	if err := prepareCDIDirectory(options.CDIDir, options.OutputMode); err != nil {
		return err
	}

	var clientset kubernetes.Interface
	if options.OutputMode == outputModeNode {
		var err error
		clientset, err = newKubernetesClient(options.KubeconfigPath)
		if err != nil {
			return fmt.Errorf("initialize Kubernetes client in node mode: %w", err)
		}
	}

	status := newDriverStatus(options.OutputMode)
	driver := newCDIDriver(
		options.OutputMode,
		clientset,
		options.NodeName,
		filepath.Join(options.CDIDir, cdiFileName),
		newAtomicCDIWriter(),
		status,
	)
	healthServer := &http.Server{
		Addr:              options.HealthAddr,
		Handler:           newHealthHandler(status),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	healthErrors := make(chan error, 1)
	go func() {
		healthErrors <- healthServer.ListenAndServe()
	}()

	fmt.Printf("DRAForge Synthetic Node Plugin %s (commit: %s) starting (mode: %s, CDI dir: %s, nodeName: %s)...\n",
		versionVal, commitSHA, options.OutputMode, options.CDIDir, options.NodeName)
	if err := driver.Refresh(ctx); err != nil {
		fmt.Printf("Initial CDI refresh failed; preserving any last-known-good document and reporting not-ready: %v\n", err)
	}

	ticker := time.NewTicker(options.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down DRAForge Node Plugin...")
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("shutdown health server: %w", err)
			}
			fmt.Println("DRAForge Node Plugin stopped; last-known-good CDI document preserved.")
			return nil
		case err := <-healthErrors:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("health server stopped: %w", err)
		case <-ticker.C:
			if err := driver.Refresh(ctx); err != nil {
				fmt.Printf("CDI refresh failed; preserving last-known-good document and reporting not-ready: %v\n", err)
			}
		}
	}
}
