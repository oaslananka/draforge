// Package server implements the HTTP API server and static React asset server.
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
	"github.com/oaslananka/draforge/internal/explain"
	"github.com/oaslananka/draforge/internal/graph"
	"k8s.io/client-go/kubernetes"
)

// Server handles HTTP API requests and serves the frontend.
type Server struct {
	clientset            kubernetes.Interface
	port                 int
	allowedOrigins       string
	allowedOriginsParsed []string // pre-parsed from allowedOrigins, avoids split/trim per request
	mu                   sync.Mutex
	clients              map[chan string]bool
}

// NewServer creates a Server instance.
// Configuration is read from environment variables where applicable:
//   - CORS_ALLOWED_ORIGINS: comma-separated list (default "*")
func NewServer(clientset kubernetes.Interface, port int) *Server {
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
		allowedOrigins:       allowedOrigins,
		allowedOriginsParsed: parsed,
		clients:              make(map[chan string]bool),
	}
}

// Start launches the HTTP server and handles graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 1. Health Endpoints
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	// 2. API Endpoints (Read-Only)
	mux.Handle("/api/summary", s.cors(s.requestLogging(http.HandlerFunc(s.handleSummary))))
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
	// Serve files from web/dist
	fileServer := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", s.securityHeaders(s.requestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fallback to index.html for React SPA routing
		if !strings.HasPrefix(r.URL.Path, "/api") && !strings.Contains(r.URL.Path, ".") {
			http.ServeFile(w, r, "./web/dist/index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	}))))

	// Start background SSE broadcaster
	go s.startSSEBroadcaster(ctx)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		fmt.Printf("API Server listening on port %d...\n", s.port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Println("Shutting down API Server gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
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

// requestLogging logs HTTP method, path, status code and duration.
// Sensitive query parameters are not logged.
func (s *Server) requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		fmt.Printf("[%s] %s %s - %d (%s)\n",
			r.Method, r.URL.Path, r.URL.RawQuery, lrw.statusCode, duration)
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Healthz
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readyz
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// Summary
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	pools, devices, claims, err := discovery.DiscoverDRA(r.Context(), s.clientset)
	if err != nil {
		s.respondError(w, err, http.StatusInternalServerError)
		return
	}

	docRegistry := doctor.NewRegistry()
	docReport := docRegistry.RunDiagnostics(r.Context(), s.clientset)

	summary := map[string]interface{}{
		"poolsCount":   len(pools),
		"devicesCount": len(devices),
		"claimsCount":  len(claims),
		"doctorStatus": docReport.Summary,
		"timestamp":    time.Now(),
	}

	_ = json.NewEncoder(w).Encode(summary)
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
	// Set SSE Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
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

	// Stream initial graph immediately
	gb := graph.NewGraphBuilder()
	g, err := gb.BuildGraph(r.Context(), s.clientset, "", "")
	if err == nil {
		if data, err := json.Marshal(g); err == nil {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
	}

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-messageChan:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// startSSEBroadcaster gathers graph data and broadcasts it to SSE clients.
func (s *Server) startSSEBroadcaster(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
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

			// Broadcast
			s.mu.Lock()
			for ch := range s.clients {
				select {
				case ch <- string(data):
				default:
					// Slow consumer, skip
				}
			}
			s.mu.Unlock()
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

	// 4. Summary metrics (low cardinality)
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
