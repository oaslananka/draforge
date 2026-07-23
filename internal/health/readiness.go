// Package health provides process liveness and dependency readiness helpers.
// SPDX-License-Identifier: Apache-2.0
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// DefaultReadinessTimeout bounds one Kubernetes API readiness check.
	DefaultReadinessTimeout = 2 * time.Second
	// DefaultReadinessGracePeriod tolerates short dependency outages.
	DefaultReadinessGracePeriod = 15 * time.Second

	ReasonKubernetesAPIUnavailable = "kubernetes_api_unavailable"
	ReasonCheckCanceled            = "readiness_check_canceled"
)

// CheckFunc verifies a required runtime dependency.
type CheckFunc func(context.Context) error

// ReadinessResult is the stable, credential-safe readiness response contract.
type ReadinessResult struct {
	Status      string     `json:"status"`
	Ready       bool       `json:"ready"`
	Degraded    bool       `json:"degraded,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Message     string     `json:"message,omitempty"`
	CheckedAt   time.Time  `json:"checkedAt"`
	LastSuccess *time.Time `json:"lastSuccess,omitempty"`
}

// ReadinessProbe performs bounded checks and tracks the last successful result.
type ReadinessProbe struct {
	check   CheckFunc
	timeout time.Duration
	grace   time.Duration

	mu          sync.Mutex
	now         func() time.Time
	startedAt   time.Time
	lastSuccess time.Time
}

// NewReadinessProbe creates a dependency readiness probe.
func NewReadinessProbe(check CheckFunc, timeout, grace time.Duration) *ReadinessProbe {
	if timeout <= 0 {
		timeout = DefaultReadinessTimeout
	}
	if grace < 0 {
		grace = 0
	}
	now := time.Now
	return &ReadinessProbe{
		check:     check,
		timeout:   timeout,
		grace:     grace,
		now:       now,
		startedAt: now(),
	}
}

// NewKubernetesReadinessProbe performs a bounded, read-only Kubernetes namespace list.
func NewKubernetesReadinessProbe(client kubernetes.Interface, timeout, grace time.Duration) *ReadinessProbe {
	return NewReadinessProbe(func(ctx context.Context) error {
		if client == nil {
			return errors.New("kubernetes client is unavailable")
		}
		_, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
		return err
	}, timeout, grace)
}

// Check evaluates dependency readiness without exposing the dependency error.
func (p *ReadinessProbe) Check(ctx context.Context) ReadinessResult {
	if ctx.Err() != nil {
		checkedAt := p.now()
		return ReadinessResult{
			Status:    "not_ready",
			Ready:     false,
			Reason:    ReasonCheckCanceled,
			Message:   "Readiness check was canceled.",
			CheckedAt: checkedAt,
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var err error
	if p.check == nil {
		err = errors.New("readiness dependency check is not configured")
	} else {
		err = p.check(checkCtx)
	}
	checkedAt := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if err == nil {
		p.lastSuccess = checkedAt
		return ReadinessResult{
			Status:      "ready",
			Ready:       true,
			CheckedAt:   checkedAt,
			LastSuccess: timePointer(p.lastSuccess),
		}
	}
	if ctx.Err() != nil {
		return ReadinessResult{
			Status:      "not_ready",
			Ready:       false,
			Reason:      ReasonCheckCanceled,
			Message:     "Readiness check was canceled.",
			CheckedAt:   checkedAt,
			LastSuccess: timePointer(p.lastSuccess),
		}
	}

	reference := p.lastSuccess
	if reference.IsZero() {
		reference = p.startedAt
	}
	withinGrace := p.grace > 0 && checkedAt.Sub(reference) < p.grace
	if withinGrace {
		return ReadinessResult{
			Status:      "degraded",
			Ready:       true,
			Degraded:    true,
			Reason:      ReasonKubernetesAPIUnavailable,
			Message:     "Kubernetes API is temporarily unavailable within the readiness grace period.",
			CheckedAt:   checkedAt,
			LastSuccess: timePointer(p.lastSuccess),
		}
	}

	return ReadinessResult{
		Status:      "not_ready",
		Ready:       false,
		Reason:      ReasonKubernetesAPIUnavailable,
		Message:     "Kubernetes API readiness check failed.",
		CheckedAt:   checkedAt,
		LastSuccess: timePointer(p.lastSuccess),
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value
	return &copyValue
}

// ServeHTTP exposes the readiness result as JSON.
func (p *ReadinessProbe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result := p.Check(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if !result.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(result)
}
