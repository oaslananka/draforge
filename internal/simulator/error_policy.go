// Package simulator classifies controller workqueue failures.
// SPDX-License-Identifier: Apache-2.0
package simulator

import apierrors "k8s.io/apimachinery/pkg/api/errors"

const maxControllerQueueRetries = 8

type controllerErrorDisposition string

const (
	controllerErrorRetry    controllerErrorDisposition = "retry"
	controllerErrorTerminal controllerErrorDisposition = "terminal"
)

func classifyControllerError(err error, requeues int) controllerErrorDisposition {
	if isTerminalKubernetesError(err) || requeues >= maxControllerQueueRetries {
		return controllerErrorTerminal
	}
	return controllerErrorRetry
}

func isTerminalKubernetesError(err error) bool {
	return apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsBadRequest(err) ||
		apierrors.IsNotFound(err) ||
		apierrors.IsMethodNotSupported(err) ||
		apierrors.IsRequestEntityTooLargeError(err)
}
