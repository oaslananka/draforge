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
		_, _ = fmt.Fprintf(w, "# HELP draforge_controller_reconcile_retries_total Total number of rate-limited controller synchronization retries\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_reconcile_retries_total counter\n")
		_, _ = fmt.Fprintf(w, "draforge_controller_reconcile_retries_total %d\n", atomic.LoadInt64(&reconciler.ReconcileRetriesCount))
		_, _ = fmt.Fprintf(w, "# HELP draforge_controller_terminal_errors_total Total number of controller synchronization failures classified as terminal\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_terminal_errors_total counter\n")
		_, _ = fmt.Fprintf(w, "draforge_controller_terminal_errors_total %d\n", atomic.LoadInt64(&reconciler.TerminalErrorsCount))
		writePipelineMetricHeaders(w)
		writePipelineMetricSamples(w, "pool", &reconciler.PoolMetrics)
		writePipelineMetricSamples(w, "allocation", &reconciler.AllocationMetrics)
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

func writePipelineMetricHeaders(w http.ResponseWriter) {
	_, _ = fmt.Fprintf(w, "# HELP draforge_controller_sync_attempts_total Total controller synchronization attempts by pipeline\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_sync_attempts_total counter\n")
	_, _ = fmt.Fprintf(w, "# HELP draforge_controller_sync_duration_seconds_total Cumulative controller synchronization duration by pipeline\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_sync_duration_seconds_total counter\n")
	_, _ = fmt.Fprintf(w, "# HELP draforge_controller_sync_in_flight Controller synchronizations currently executing by pipeline\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_sync_in_flight gauge\n")
	_, _ = fmt.Fprintf(w, "# HELP draforge_controller_queue_depth Ready controller workqueue items by pipeline\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_controller_queue_depth gauge\n")
}

func writePipelineMetricSamples(w http.ResponseWriter, pipeline string, metrics *simulator.PipelineMetrics) {
	_, _ = fmt.Fprintf(w, "draforge_controller_sync_attempts_total{pipeline=%q} %d\n", pipeline, atomic.LoadInt64(&metrics.Attempts))
	durationSeconds := float64(atomic.LoadInt64(&metrics.DurationNanoseconds)) / float64(time.Second)
	_, _ = fmt.Fprintf(w, "draforge_controller_sync_duration_seconds_total{pipeline=%q} %.9f\n", pipeline, durationSeconds)
	_, _ = fmt.Fprintf(w, "draforge_controller_sync_in_flight{pipeline=%q} %d\n", pipeline, atomic.LoadInt64(&metrics.InFlight))
	_, _ = fmt.Fprintf(w, "draforge_controller_queue_depth{pipeline=%q} %d\n", pipeline, atomic.LoadInt64(&metrics.QueueDepth))
}
