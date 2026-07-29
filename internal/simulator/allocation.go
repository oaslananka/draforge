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

type allocationSyncState struct {
	claims  []resourcev1.ResourceClaim
	slices  []resourcev1.ResourceSlice
	classes map[string]*resourcev1.DeviceClass
}

// SimulateAllocation finds pending claims and assigns them through the supported DRA selection subset.
func (r *Reconciler) SimulateAllocation(ctx context.Context) error {
	state, err := r.loadAllocationSyncState(ctx)
	if err != nil {
		return err
	}
	for index := range state.claims {
		updatedClaim, allocateErr := r.simulatePendingClaim(ctx, &state.claims[index], state)
		if allocateErr != nil {
			return allocateErr
		}
		if updatedClaim != nil {
			state.claims[index] = *updatedClaim
		}
	}
	return nil
}

func (r *Reconciler) loadAllocationSyncState(ctx context.Context) (*allocationSyncState, error) {
	claims, err := r.clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list ResourceClaims: %w", err)
	}
	slices, err := r.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list ResourceSlices for allocation: %w", err)
	}
	classList, err := r.clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list DeviceClasses: %w", err)
	}
	classes := make(map[string]*resourcev1.DeviceClass, len(classList.Items))
	for index := range classList.Items {
		deviceClass := &classList.Items[index]
		classes[deviceClass.Name] = deviceClass
	}
	return &allocationSyncState{
		claims:  claims.Items,
		slices:  slices.Items,
		classes: classes,
	}, nil
}

func (r *Reconciler) simulatePendingClaim(
	ctx context.Context,
	claim *resourcev1.ResourceClaim,
	state *allocationSyncState,
) (*resourcev1.ResourceClaim, error) {
	if claim.Status.Allocation != nil || len(claim.Spec.Devices.Requests) == 0 {
		return nil, nil
	}

	plan, failure := r.planClaimAllocation(ctx, claim, state.claims, state.slices, state.classes)
	if failure != nil {
		r.recordAllocationFailure(claim, failure)
		return nil, nil
	}
	if len(plan.results) == 0 {
		return nil, nil
	}

	fmt.Printf("Simulating allocation for claim %s/%s -> %d devices on node %s\n",
		claim.Namespace, claim.Name, len(plan.results), plan.nodeName)
	allocatedClaim := claimWithAllocationPlan(claim, plan)
	updatedClaim, err := r.clientset.ResourceV1().ResourceClaims(claim.Namespace).UpdateStatus(
		ctx,
		allocatedClaim,
		metav1.UpdateOptions{},
	)
	if err != nil {
		if r.eventRecorder != nil {
			r.eventRecorder.Eventf(claim, corev1.EventTypeWarning, "AllocationStatusUpdateFailed", "Failed to update allocation status: %v", err)
		}
		return nil, fmt.Errorf("update ResourceClaim %s/%s allocation status: %w", claim.Namespace, claim.Name, err)
	}
	atomic.AddInt64(&r.AllocationsSimulated, 1)
	return updatedClaim, nil
}

func claimWithAllocationPlan(claim *resourcev1.ResourceClaim, plan allocationPlan) *resourcev1.ResourceClaim {
	allocatedClaim := claim.DeepCopy()
	allocatedClaim.Status.Allocation = &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{Results: plan.results},
	}
	if plan.nodeName == "" {
		return allocatedClaim
	}
	allocatedClaim.Status.Allocation.NodeSelector = &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchFields: []corev1.NodeSelectorRequirement{{
				Key:      "metadata.name",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{plan.nodeName},
			}},
		}},
	}
	return allocatedClaim
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
