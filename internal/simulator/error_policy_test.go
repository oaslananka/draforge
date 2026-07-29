// Package simulator tests controller queue error classification.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var controllerTestResource = schema.GroupResource{
	Group:    "resource.k8s.io",
	Resource: "resourceclaims",
}

func TestControllerQueueErrorDisposition(t *testing.T) {
	invalid := apierrors.NewInvalid(
		schema.GroupKind{Group: "resource.k8s.io", Kind: "ResourceClaim"},
		"claim-a",
		field.ErrorList{field.Invalid(field.NewPath("spec"), "bad", "invalid fixture")},
	)
	tests := map[string]struct {
		err  error
		want controllerErrorDisposition
	}{
		"conflict":          {err: apierrors.NewConflict(controllerTestResource, "claim-a", errors.New("conflict")), want: controllerErrorRetry},
		"already exists":    {err: apierrors.NewAlreadyExists(controllerTestResource, "claim-a"), want: controllerErrorRetry},
		"server timeout":    {err: apierrors.NewServerTimeout(controllerTestResource, "list", 1), want: controllerErrorRetry},
		"request timeout":   {err: apierrors.NewTimeoutError("timeout", 1), want: controllerErrorRetry},
		"wrapped conflict":  {err: fmt.Errorf("update claim: %w", apierrors.NewConflict(controllerTestResource, "claim-a", errors.New("conflict"))), want: controllerErrorRetry},
		"too many requests": {err: apierrors.NewTooManyRequests("busy", 1), want: controllerErrorRetry},
		"service unavailable": {
			err:  apierrors.NewServiceUnavailable("unavailable"),
			want: controllerErrorRetry,
		},
		"internal error":    {err: apierrors.NewInternalError(errors.New("internal")), want: controllerErrorRetry},
		"forbidden":         {err: apierrors.NewForbidden(controllerTestResource, "claim-a", errors.New("forbidden")), want: controllerErrorTerminal},
		"wrapped forbidden": {err: fmt.Errorf("update claim: %w", apierrors.NewForbidden(controllerTestResource, "claim-a", errors.New("forbidden"))), want: controllerErrorTerminal},
		"unauthorized":      {err: apierrors.NewUnauthorized("unauthorized"), want: controllerErrorTerminal},
		"invalid":           {err: invalid, want: controllerErrorTerminal},
		"bad request":       {err: apierrors.NewBadRequest("bad request"), want: controllerErrorTerminal},
		"not found":         {err: apierrors.NewNotFound(controllerTestResource, "claim-a"), want: controllerErrorTerminal},
		"method unsupported": {
			err:  apierrors.NewMethodNotSupported(controllerTestResource, "patch"),
			want: controllerErrorTerminal,
		},
		"request too large": {
			err:  apierrors.NewRequestEntityTooLargeError("too large"),
			want: controllerErrorTerminal,
		},
		"unknown error": {err: errors.New("connection reset"), want: controllerErrorRetry},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := classifyControllerError(test.err, 0); got != test.want {
				t.Fatalf("classification = %q, want %q for %v", got, test.want, test.err)
			}
		})
	}
}

func TestControllerQueueErrorRetryLimit(t *testing.T) {
	err := errors.New("unknown transport error")
	if got := classifyControllerError(err, maxControllerQueueRetries-1); got != controllerErrorRetry {
		t.Fatalf("classification before retry limit = %q, want retry", got)
	}
	if got := classifyControllerError(err, maxControllerQueueRetries); got != controllerErrorTerminal {
		t.Fatalf("classification at retry limit = %q, want terminal", got)
	}
}
