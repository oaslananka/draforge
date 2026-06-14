// Package simulator allocation simulation for fallback cluster environments.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StartAllocationSimulator watches for pending claims and simulates allocation.
func (r *Reconciler) StartAllocationSimulator(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Println("Starting ResourceClaim allocation simulator...")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.SimulateAllocation(ctx); err != nil {
				fmt.Printf("Allocation simulation error: %v\n", err)
			}
		}
	}
}

// SimulateAllocation finds pending claims and assigns them to available virtual devices.
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

	for _, claim := range claims.Items {
		// Only allocate pending claims
		if claim.Status.Allocation != nil {
			continue
		}

		// Find a matching slice/pool with available capacity
		var targetSlice *resourcev1.ResourceSlice
		var targetDeviceName string
		found := false

		for _, slice := range slices.Items {
			// Ensure it is a simulator slice
			if slice.Labels["draforge.oaslananka/managed-by"] != "simulator" {
				continue
			}

			// Do not allocate from unhealthy slices (fault injection)
			if slice.Labels["draforge.oaslananka/health"] == "unhealthy" {
				continue
			}

			// Simple model/class matching: in simulator, if DeviceClassName matches or defaults
			// We find the first available device in the slice
			for _, dev := range slice.Spec.Devices {
				// Check if device is already allocated
				if r.isDeviceAllocated(claims.Items, slice.Spec.Pool.Name, dev.Name) {
					continue
				}
				targetSlice = &slice
				targetDeviceName = dev.Name
				found = true
				break
			}
			if found {
				break
			}
		}

		if found {
			nodeName := ""
			if targetSlice.Spec.NodeName != nil {
				nodeName = *targetSlice.Spec.NodeName
			}
			fmt.Printf("Simulating allocation for claim %s/%s -> device %s on node %s\n",
				claim.Namespace, claim.Name, targetDeviceName, nodeName)

			// Update claim status with allocation result
			allocatedClaim := claim.DeepCopy()
			allocatedClaim.Status.Allocation = &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Device: targetDeviceName,
							Driver: targetSlice.Spec.Driver,
							Pool:   targetSlice.Spec.Pool.Name,
						},
					},
				},
				NodeSelector: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchFields: []corev1.NodeSelectorRequirement{
								{
									Key:      "metadata.name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{nodeName},
								},
							},
						},
					},
				},
			}

			_, err = r.clientset.ResourceV1().ResourceClaims(claim.Namespace).UpdateStatus(ctx, allocatedClaim, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Failed to update status for claim %s: %v\n", claim.Name, err)
			} else {
				atomic.AddInt64(&r.AllocationsSimulated, 1)
			}
		}
	}

	return nil
}

func (r *Reconciler) isDeviceAllocated(claims []resourcev1.ResourceClaim, poolName, devName string) bool {
	for _, c := range claims {
		if c.Status.Allocation != nil {
			for _, res := range c.Status.Allocation.Devices.Results {
				if res.Pool == poolName && res.Device == devName {
					return true
				}
			}
		}
	}
	return false
}
