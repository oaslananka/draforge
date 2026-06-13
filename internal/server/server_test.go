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

	srv.cors(srv.handleClaims)(rr, req)

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

	// 3. Test handleMetrics endpoint
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	rrMetrics := httptest.NewRecorder()

	srv.handleMetrics(rrMetrics, reqMetrics)

	if rrMetrics.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rrMetrics.Code)
	}

	bodyStr := rrMetrics.Body.String()
	if !contains(bodyStr, "draforge_claims_total") {
		t.Errorf("expected metrics to contain draforge_claims_total, got: %s", bodyStr)
	}
}

func contains(s, substr string) bool {
	// Simple slice-based contains check
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
