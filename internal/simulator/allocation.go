// Package simulator allocation simulation for fallback cluster environments.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SimulateAllocation finds pending claims and assigns them through the supported DRA selection subset.
func (r *Reconciler) SimulateAllocation(ctx context.Context) error {
	claimsClient := r.clientset.ResourceV1().ResourceClaims("")
	claims, err := claimsClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	slices, err := r.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	classList, err := r.clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	classes := make(map[string]*resourcev1.DeviceClass, len(classList.Items))
	for index := range classList.Items {
		deviceClass := &classList.Items[index]
		classes[deviceClass.Name] = deviceClass
	}

	for claimIndex := range claims.Items {
		claim := &claims.Items[claimIndex]
		if claim.Status.Allocation != nil || len(claim.Spec.Devices.Requests) == 0 {
			continue
		}

		plan, failure := r.planClaimAllocation(ctx, claim, claims.Items, slices.Items, classes)
		if failure != nil {
			r.recordAllocationFailure(claim, failure)
			continue
		}
		if len(plan.results) == 0 {
			continue
		}

		fmt.Printf("Simulating allocation for claim %s/%s -> %d devices on node %s\n",
			claim.Namespace, claim.Name, len(plan.results), plan.nodeName)

		allocatedClaim := claim.DeepCopy()
		allocatedClaim.Status.Allocation = &resourcev1.AllocationResult{
			Devices: resourcev1.DeviceAllocationResult{Results: plan.results},
		}
		if plan.nodeName != "" {
			allocatedClaim.Status.Allocation.NodeSelector = &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchFields: []corev1.NodeSelectorRequirement{{
						Key:      "metadata.name",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{plan.nodeName},
					}},
				}},
			}
		}

		updatedClaim, updateErr := r.clientset.ResourceV1().ResourceClaims(claim.Namespace).UpdateStatus(ctx, allocatedClaim, metav1.UpdateOptions{})
		if updateErr != nil {
			fmt.Printf("Failed to update status for claim %s/%s: %v\n", claim.Namespace, claim.Name, updateErr)
			continue
		}
		atomic.AddInt64(&r.AllocationsSimulated, 1)
		claims.Items[claimIndex] = *updatedClaim
	}

	return nil
}

func (r *Reconciler) recordAllocationFailure(claim *resourcev1.ResourceClaim, failure *allocationFailure) {
	fmt.Printf("Simulating allocation for claim %s/%s stopped: %s\n", claim.Namespace, claim.Name, failure.message)
	if r.eventRecorder != nil {
		r.eventRecorder.Eventf(claim, corev1.EventTypeWarning, failure.reason, "%s", failure.message)
	}
}

func (r *Reconciler) isDeviceAllocated(claims []resourcev1.ResourceClaim, driverName, poolName, deviceName string) bool {
	for _, claim := range claims {
		if claim.Status.Allocation == nil {
			continue
		}
		for _, result := range claim.Status.Allocation.Devices.Results {
			if result.Driver == driverName && result.Pool == poolName && result.Device == deviceName {
				return true
			}
		}
	}
	return false
}
