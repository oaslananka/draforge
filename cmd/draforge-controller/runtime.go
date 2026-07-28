package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/oaslananka/draforge/internal/health"
	"github.com/oaslananka/draforge/internal/simulator"
)

func newRuntimeServer(addr string, reconciler *simulator.Reconciler, readiness *health.ReadinessProbe) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", okHandler)
	if readiness == nil {
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "readiness probe is not configured", http.StatusServiceUnavailable)
		})
	} else {
		mux.Handle("/readyz", readiness)
	}
	mux.HandleFunc("/metrics", metricsHandler(reconciler))
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func metricsHandler(reconciler *simulator.Reconciler) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "# HELP draforge_controller_reconcile_errors_total Total number of reconciliation errors\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_reconcile_errors_total counter\n")
		_, _ = fmt.Fprintf(w, "draforge_controller_reconcile_errors_total %d\n", atomic.LoadInt64(&reconciler.ReconcileErrorsCount))
		_, _ = fmt.Fprintf(w, "draforge_controller_allocations_simulated_total %d\n", atomic.LoadInt64(&reconciler.AllocationsSimulated))
		_, _ = fmt.Fprintf(w, "draforge_controller_active_faults %d\n", atomic.LoadInt64(&reconciler.ActiveFaultsCount))
		_, _ = fmt.Fprintf(w, "# HELP draforge_controller_leader Whether this process currently owns the controller leader lease\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_leader gauge\n")
		leader := 0
		if reconciler.IsLeader() {
			leader = 1
		}
		_, _ = fmt.Fprintf(w, "draforge_controller_leader %d\n", leader)
	}
}
