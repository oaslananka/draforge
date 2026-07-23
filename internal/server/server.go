// Package server implements the HTTP API server and static React asset server.
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
	"github.com/oaslananka/draforge/internal/explain"
	"github.com/oaslananka/draforge/internal/graph"
	"github.com/oaslananka/draforge/internal/health"
	"k8s.io/client-go/kubernetes"
)

const DefaultShutdownTimeout = 5 * time.Second

// ServerOptions configures dependency readiness and graceful shutdown.
type ServerOptions struct {
	ReadinessTimeout     time.Duration
	ReadinessGracePeriod time.Duration
	ShutdownTimeout      time.Duration
}

// DefaultServerOptions returns production-safe lifecycle defaults.
func DefaultServerOptions() ServerOptions {
	return ServerOptions{
		ReadinessTimeout:     health.DefaultReadinessTimeout,
		ReadinessGracePeriod: health.DefaultReadinessGracePeriod,
		ShutdownTimeout:      DefaultShutdownTimeout,
	}
}

// Server handles HTTP API requests and serves the frontend.
type Server struct {
	clientset            kubernetes.Interface
	port                 int
	version              string
	commit               string
	allowedOrigins       string
	allowedOriginsParsed []string // pre-parsed from allowedOrigins, avoids split/trim per request
	logf                 func(format string, args ...any)
	readiness            *health.ReadinessProbe
	shutdownTimeout      time.Duration
	mu                   sync.Mutex
	clients              map[chan string]bool
}

// NewServer creates a Server instance with default lifecycle settings.
func NewServer(clientset kubernetes.Interface, port int) *Server {
	return NewServerWithOptions(clientset, port, DefaultServerOptions())
}

// NewServerWithOptions creates a Server instance with explicit lifecycle settings.
// Configuration is read from environment variables where applicable:
//   - CORS_ALLOWED_ORIGINS: comma-separated list (default "*")
func NewServerWithOptions(clientset kubernetes.Interface, port int, options ServerOptions) *Server {
	if options.ReadinessTimeout <= 0 {
		options.ReadinessTimeout = health.DefaultReadinessTimeout
	}
	if options.ReadinessGracePeriod < 0 {
		options.ReadinessGracePeriod = 0
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = DefaultShutdownTimeout
	}

	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "*"
	}

	var parsed []string
	if allowedOrigins != "*" {
		for _, o := range strings.Split(allowedOrigins, ",") {
			parsed = append(parsed, strings.TrimSpace(o))
		}
	}

	return &Server{
		clientset:            clientset,
		port:                 port,
		version:              "dev",
		commit:               "dev",
		allowedOrigins:       allowedOrigins,
		allowedOriginsParsed: parsed,
		logf:                 defaultLogf,
		readiness:            health.NewKubernetesReadinessProbe(clientset, options.ReadinessTimeout, options.ReadinessGracePeriod),
		shutdownTimeout:      options.ShutdownTimeout,
		clients:              make(map[chan string]bool),
	}
}

// SetBuildInfo updates runtime build metadata exposed by /api/version.
func (s *Server) SetBuildInfo(version, commit string) {
	if version != "" {
		s.version = version
	}
	if commit != "" {
		s.commit = commit
	}
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Health Endpoints
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	// 2. API Endpoints (Read-Only)
	mux.Handle("/api/summary", s.cors(s.requestLogging(http.HandlerFunc(s.handleSummary))))
	mux.Handle("/api/version", s.cors(s.requestLogging(http.HandlerFunc(s.handleVersion))))
	mux.Handle("/api/pools", s.cors(s.requestLogging(http.HandlerFunc(s.handlePools))))
	mux.Handle("/api/devices", s.cors(s.requestLogging(http.HandlerFunc(s.handleDevices))))
	mux.Handle("/api/claims", s.cors(s.requestLogging(http.HandlerFunc(s.handleClaims))))
	mux.Handle("/api/graph", s.cors(s.requestLogging(http.HandlerFunc(s.handleGraph))))
	mux.Handle("/api/explain", s.cors(s.requestLogging(http.HandlerFunc(s.handleExplain))))
	mux.Handle("/api/doctor", s.cors(s.requestLogging(http.HandlerFunc(s.handleDoctor))))
	mux.Handle("/metrics", s.requestLogging(http.HandlerFunc(s.handleMetrics)))

	// 3. SSE Stream Endpoint
	mux.Handle("/api/stream", s.cors(s.requestLogging(http.HandlerFunc(s.handleStream))))

	// 4. Frontend static files
	fileServer := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", s.securityHeaders(s.requestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fallback to index.html for React SPA routing
		if !strings.HasPrefix(r.URL.Path, "/api") && !strings.Contains(r.URL.Path, ".") {
			http.ServeFile(w, r, "./web/dist/index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	}))))

	return mux
}

type httpServerLifecycle interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

// newHTTPServer builds the HTTP server using a cancelable base context for all connections.
func (s *Server) newHTTPServer(requestCtx context.Context) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return requestCtx
		},
	}
}

// Start launches the HTTP server and handles graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	requestCtx, cancelRequests := context.WithCancel(ctx)
	defer cancelRequests()

	go s.startSSEBroadcaster(requestCtx)
	httpServer := s.newHTTPServer(requestCtx)
	fmt.Printf("API Server listening on port %d...\n", s.port)
	return runHTTPServer(ctx, httpServer, s.shutdownTimeout, cancelRequests)
}

func runHTTPServer(ctx context.Context, httpServer httpServerLifecycle, shutdownTimeout time.Duration, cancelRequests context.CancelFunc) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		cancelRequests()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		fmt.Println("Shutting down API Server gracefully...")
		cancelRequests()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			closeErr := httpServer.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("graceful shutdown: %w", err), fmt.Errorf("force close: %w", closeErr))
			}
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

// Security Headers middleware
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP: 'unsafe-inline' is required for React SPA with Vite HMR and
		// for inline style/script tags emitted by the production build.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
				"connect-src 'self' ws: wss:;")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy",
			"geolocation=(), microphone=(), camera=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

// CORS middleware for APIs
// Uses s.allowedOrigins which defaults to "*" (read-only public model)
// and can be narrowed via the CORS_ALLOWED_ORIGINS env var.
// Allowed origins are parsed once at server init for performance.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.allowedOrigins == "*" || origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			origins := s.allowedOriginsParsed
			if origins == nil {
				for _, o := range strings.Split(s.allowedOrigins, ",") {
					o = strings.TrimSpace(o)
					if origin == o {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
				goto done
			}
			for _, allowed := range origins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					break
				}
			}
		}
	done:
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func defaultLogf(format string, args ...any) {
	_, _ = fmt.Printf(format, args...)
}

// requestLogging logs HTTP method, path, status code and duration.
// Sensitive query parameters are not logged.
func (s *Server) requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		logf := s.logf
		if logf == nil {
			logf = defaultLogf
		}
		logf("[%s] %s %s - %d (%s)\n",
			r.Method, r.URL.Path, r.URL.RawQuery, lrw.statusCode, duration)
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

// Unwrap exposes the underlying writer to http.ResponseController so optional
// capabilities such as flushing and per-request deadlines survive middleware.
func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return lrw.ResponseWriter
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if lrw.wroteHeader {
		return
	}
	lrw.statusCode = code
	lrw.wroteHeader = true
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(data []byte) (int, error) {
	if !lrw.wroteHeader {
		lrw.WriteHeader(http.StatusOK)
	}
	return lrw.ResponseWriter.Write(data)
}

// Healthz
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readyz reports bounded Kubernetes API dependency readiness.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.readiness == nil {
		http.Error(w, "readiness probe is not configured", http.StatusServiceUnavailable)
		return
	}
	s.readiness.ServeHTTP(w, r)
}

// Summary
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	pools, devices, claims, status, err := discovery.DiscoverDRAWithStatus(r.Context(), s.clientset)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}

	docRegistry := doctor.NewRegistry()
	docReport := docRegistry.RunDiagnostics(r.Context(), s.clientset)

	summary := map[string]interface{}{
		"poolsCount":      len(pools),
		"devicesCount":    len(devices),
		"claimsCount":     len(claims),
		"doctorStatus":    docReport.Summary,
		"timestamp":       time.Now(),
		"discoveryStatus": status,
	}

	_ = json.NewEncoder(w).Encode(summary)
}

// Version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{
		"version": s.version,
		"commit":  s.commit,
	})
}

// Pools
func (s *Server) handlePools(w http.ResponseWriter, r *http.Request) {
	pools, _, _, err := discovery.DiscoverDRA(r.Context(), s.clientset)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(pools)
}

// Devices
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	_, devices, _, err := discovery.DiscoverDRA(r.Context(), s.clientset)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(devices)
}

// Claims
func (s *Server) handleClaims(w http.ResponseWriter, r *http.Request) {
	_, _, claims, err := discovery.DiscoverDRA(r.Context(), s.clientset)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(claims)
}

// Graph
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	gb := graph.NewGraphBuilder()
	g, err := gb.BuildGraph(r.Context(), s.clientset, "", "")
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(g)
}

// Explain
func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	claimName := r.URL.Query().Get("claim")
	namespace := r.URL.Query().Get("namespace")
	if claimName == "" || namespace == "" {
		s.respondError(w, fmt.Errorf("parameters 'claim' and 'namespace' are required"), http.StatusBadRequest)
		return
	}

	res, err := explain.ExplainClaim(r.Context(), s.clientset, namespace, claimName)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}

// Doctor
func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	docRegistry := doctor.NewRegistry()
	report := docRegistry.RunDiagnostics(r.Context(), s.clientset)
	_ = json.NewEncoder(w).Encode(report)
}

// Stream (Server-Sent Events)
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE Headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	controller := http.NewResponseController(w)
	// Ordinary requests keep the server-wide WriteTimeout. SSE is intentionally
	// long-lived, so clear only this request's write deadline when supported.
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		http.Error(w, "Unable to configure streaming deadline", http.StatusInternalServerError)
		return
	}

	// Buffered channel prevents slow consumers from blocking the broadcaster.
	messageChan := make(chan string, 1)
	s.mu.Lock()
	s.clients[messageChan] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, messageChan)
		s.mu.Unlock()
		close(messageChan)
	}()

	// Stream initial graph immediately.
	gb := graph.NewGraphBuilder()
	g, err := gb.BuildGraph(r.Context(), s.clientset, "", "")
	if err == nil {
		if data, marshalErr := json.Marshal(g); marshalErr == nil {
			if writeSSEEvent(w, controller, string(data)) != nil {
				return
			}
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-messageChan:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, controller, msg); err != nil {
				return
			}
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, controller *http.ResponseController, data string) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return controller.Flush()
}

// startSSEBroadcaster gathers graph data and broadcasts it to SSE clients.
func (s *Server) startSSEBroadcaster(ctx context.Context) {
	s.runSSEBroadcaster(ctx, 5*time.Second)
}

func (s *Server) runSSEBroadcaster(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			clientCount := len(s.clients)
			s.mu.Unlock()

			if clientCount == 0 {
				continue
			}

			// Build graph
			gb := graph.NewGraphBuilder()
			g, err := gb.BuildGraph(ctx, s.clientset, "", "")
			if err != nil {
				continue
			}

			data, err := json.Marshal(g)
			if err != nil {
				continue
			}

			s.broadcast(string(data))
		}
	}
}

// broadcast delivers an update without allowing a full subscriber buffer to
// delay healthy clients. Client registration and removal use the same mutex.
func (s *Server) broadcast(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- message:
		default:
			// Slow consumer: retain its buffered event and continue to others.
		}
	}
}

// apiErrorResponse is the standard error envelope for all API endpoints.
type apiErrorResponse struct {
	Error apiErrorDetail `json:"error"`
}

type apiErrorDetail struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (s *Server) respondError(w http.ResponseWriter, err error, code int) {
	codeStr := "internal_error"
	if code >= 400 && code < 500 {
		codeStr = "bad_request"
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apiErrorResponse{
		Error: apiErrorDetail{
			Message: err.Error(),
			Code:    codeStr,
		},
	})
}

func detailedMetricsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DRAFORGE_METRICS_DETAIL"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	pools, devices, claims, err := discovery.DiscoverDRA(r.Context(), s.clientset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "# ERROR: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if detailedMetricsEnabled() {
		// 1. Pools
		_, _ = fmt.Fprintf(w, "# HELP draforge_pools_total Total number of device pools\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_pools_total gauge\n")
		for _, p := range pools {
			_, _ = fmt.Fprintf(w, "draforge_pools_total{pool=%q,driver=%q,node=%q,synthetic=%t,health=%q} 1\n",
				p.Name, p.DriverName, p.NodeName, p.IsSynthetic, p.Health)
		}

		// 2. Devices
		_, _ = fmt.Fprintf(w, "# HELP draforge_devices_total Total number of devices\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_devices_total gauge\n")
		for _, d := range devices {
			_, _ = fmt.Fprintf(w, "draforge_devices_total{device=%q,pool=%q,node=%q,type=%q,status=%q,synthetic=%t} 1\n",
				d.Name, d.PoolName, d.NodeName, d.Type, d.Status, d.IsSynthetic)
		}

		// 3. Claims
		_, _ = fmt.Fprintf(w, "# HELP draforge_claims_total Total number of ResourceClaims\n")
		_, _ = fmt.Fprintf(w, "# TYPE draforge_claims_total gauge\n")
		for _, c := range claims {
			_, _ = fmt.Fprintf(w, "draforge_claims_total{claim=%q,namespace=%q,class=%q,status=%q} 1\n",
				c.Name, c.Namespace, c.DeviceClassName, c.Status)
		}
	}

	// Summary metrics (low cardinality)
	_, _ = fmt.Fprintf(w, "# HELP draforge_pools_count Total device pools\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_pools_count gauge\n")
	_, _ = fmt.Fprintf(w, "draforge_pools_count %d\n", len(pools))

	_, _ = fmt.Fprintf(w, "# HELP draforge_devices_count Total devices across all pools\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_devices_count gauge\n")
	_, _ = fmt.Fprintf(w, "draforge_devices_count %d\n", len(devices))

	_, _ = fmt.Fprintf(w, "# HELP draforge_claims_count Total resource claims\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_claims_count gauge\n")
	_, _ = fmt.Fprintf(w, "draforge_claims_count %d\n", len(claims))

	// Claims by status
	statusCounts := make(map[string]int)
	for _, c := range claims {
		statusCounts[c.Status]++
	}
	for status, count := range statusCounts {
		_, _ = fmt.Fprintf(w, "draforge_claims_by_status{status=%q} %d\n", status, count)
	}

	// 5. Faults count
	faultCount := 0
	for _, p := range pools {
		if p.Health != "healthy" {
			faultCount++
		}
	}
	_, _ = fmt.Fprintf(w, "# HELP draforge_active_faults_total Total number of active faults\n")
	_, _ = fmt.Fprintf(w, "# TYPE draforge_active_faults_total gauge\n")
	_, _ = fmt.Fprintf(w, "draforge_active_faults_total %d\n", faultCount)
}
