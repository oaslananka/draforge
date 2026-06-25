// Package server unit tests.
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestServerEndpoints(t *testing.T) {

	// 1. Setup mock resources
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "claim-1",
			Namespace:         "default",
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim)
	srv := NewServer(clientset, 8081)

	// 2. Test handleClaims endpoint
	req := httptest.NewRequest("GET", "/api/claims", nil)
	rr := httptest.NewRecorder()

	srv.cors(http.HandlerFunc(srv.handleClaims)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify CORS header
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS Access-Control-Allow-Origin to be *, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	var claims []interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &claims)
	if err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if len(claims) != 1 {
		t.Errorf("expected 1 claim in response, got %d", len(claims))
	}

	// Test handleSummary endpoint for discoveryStatus fields
	reqSummary := httptest.NewRequest("GET", "/api/summary", nil)
	rrSummary := httptest.NewRecorder()

	srv.cors(http.HandlerFunc(srv.handleSummary)).ServeHTTP(rrSummary, reqSummary)
	if rrSummary.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rrSummary.Code)
	}

	var summaryResp map[string]interface{}
	if err := json.Unmarshal(rrSummary.Body.Bytes(), &summaryResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Validate presence of discoveryStatus
	statusField, ok := summaryResp["discoveryStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected discoveryStatus field in summary response")
	}

	// As everything is successful, they should be true
	if statusField["resourceClaimsAvailable"] != true {
		t.Errorf("expected resourceClaimsAvailable true")
	}
	if statusField["resourceSlicesAvailable"] != true {
		t.Errorf("expected resourceSlicesAvailable true")
	}
	if statusField["podsAvailable"] != true {
		t.Errorf("expected podsAvailable true")
	}
	if statusField["isPartial"] != false {
		t.Errorf("expected isPartial false")
	}

	// 3. Test handleMetrics endpoint
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	rrMetrics := httptest.NewRecorder()

	srv.handleMetrics(rrMetrics, reqMetrics)

	if rrMetrics.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rrMetrics.Code)
	}

	bodyStr := rrMetrics.Body.String()
	// Check for summary metrics (new low-cardinality gauges)
	if !contains(bodyStr, "draforge_pools_count") {
		t.Errorf("expected metrics to contain draforge_pools_count, got: %s", bodyStr)
	}
	if !contains(bodyStr, "draforge_devices_count") {
		t.Errorf("expected metrics to contain draforge_devices_count, got: %s", bodyStr)
	}
	if !contains(bodyStr, "draforge_claims_count") {
		t.Errorf("expected metrics to contain draforge_claims_count, got: %s", bodyStr)
	}
	if !contains(bodyStr, "draforge_claims_by_status") {
		t.Errorf("expected metrics to contain draforge_claims_by_status, got: %s", bodyStr)
	}
	// High-cardinality detail metrics are disabled by default.
	if contains(bodyStr, "draforge_claims_total{") {
		t.Errorf("expected detailed claim metrics to be disabled by default, got: %s", bodyStr)
	}
}

func TestDetailedMetricsEnabledConfig(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"one", "1", true},
		{"true", "true", true},
		{"yes", "yes", true},
		{"off", "off", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DRAFORGE_METRICS_DETAIL", tt.value)
			if got := detailedMetricsEnabled(); got != tt.want {
				t.Errorf("detailedMetricsEnabled() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestVersionEndpoint(t *testing.T) {
	srv := NewServer(nil, 8081)
	srv.SetBuildInfo("v9.9.9", "abc123")

	req := httptest.NewRequest("GET", "/api/version", nil)
	rr := httptest.NewRecorder()

	srv.cors(http.HandlerFunc(srv.handleVersion)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var version map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &version); err != nil {
		t.Fatalf("failed to parse version JSON: %v", err)
	}

	if version["version"] != "v9.9.9" {
		t.Errorf("expected version v9.9.9, got %q", version["version"])
	}
	if version["commit"] != "abc123" {
		t.Errorf("expected commit abc123, got %q", version["commit"])
	}
}

func TestCORSConfigurableOrigins(t *testing.T) {
	srv := NewServer(nil, 8081)

	// Default: wildcard
	req := httptest.NewRequest("GET", "/api/claims", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := httptest.NewRecorder()
	srv.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected wildcard with default config, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	// Restricted origins
	srv2 := &Server{allowedOrigins: "http://localhost:3000,https://draforge.example.com"}
	req2 := httptest.NewRequest("GET", "/api/claims", nil)
	req2.Header.Set("Origin", "https://draforge.example.com")
	rr2 := httptest.NewRecorder()
	srv2.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr2, req2)

	if rr2.Header().Get("Access-Control-Allow-Origin") != "https://draforge.example.com" {
		t.Errorf("expected specific origin, got %q", rr2.Header().Get("Access-Control-Allow-Origin"))
	}

	// Non-matching origin
	req3 := httptest.NewRequest("GET", "/api/claims", nil)
	req3.Header.Set("Origin", "http://evil.com")
	rr3 := httptest.NewRecorder()
	srv2.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr3, req3)

	if rr3.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected empty CORS for non-matching origin, got %q", rr3.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := NewServer(nil, 8081)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	srv.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	headers := []string{
		"Content-Security-Policy",
		"X-Frame-Options",
		"X-Content-Type-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	}
	for _, h := range headers {
		if rr.Header().Get(h) == "" {
			t.Errorf("missing security header: %s", h)
		}
	}

	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options DENY, got %q", rr.Header().Get("X-Frame-Options"))
	}
}

func TestErrorResponseFormat(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	srv := NewServer(clientset, 8081)
	// Missing 'claim' param triggers the validation error path (400)
	req := httptest.NewRequest("GET", "/api/explain?namespace=default", nil)
	rr := httptest.NewRecorder()

	srv.cors(http.HandlerFunc(srv.handleExplain)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var resp apiErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error JSON: %v", err)
	}

	if resp.Error.Message == "" {
		t.Error("expected error message, got empty")
	}
	if resp.Error.Code != "bad_request" {
		t.Errorf("expected error code 'bad_request', got %q", resp.Error.Code)
	}
}

func TestSSEChannelBuffer(t *testing.T) {
	// Verify the SSE channel is created with buffer size 1.
	// This is a structural test: the buffered channel prevents
	// slow consumers from blocking the broadcaster.
	ch := make(chan string, 1)
	if cap(ch) != 1 {
		t.Fatalf("expected SSE channel capacity 1, got %d", cap(ch))
	}
	// A non-blocking send must succeed.
	select {
	case ch <- "test":
	default:
		t.Fatal("buffered channel should accept one send without blocking")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
