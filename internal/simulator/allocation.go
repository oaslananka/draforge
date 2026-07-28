// Package simulator allocation simulation for fallback cluster environments.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"strings"
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

		if len(claim.Spec.Devices.Requests) == 0 {
			continue
		}

		var finalResults []resourcev1.DeviceRequestAllocationResult
		var allocatedNodeName string
		allocationSuccess := true
		chosenDevices := make(map[string]bool)

		for _, req := range claim.Spec.Devices.Requests {
			reqName := req.Name

			var subReqNames []string
			var reqCounts []int
			var reqClasses []string
			var reqSelectors [][]resourcev1.DeviceSelector

			if req.Exactly != nil {
				subReqNames = append(subReqNames, reqName)
				c := int(req.Exactly.Count)
				if c <= 0 {
					c = 1
				}
				reqCounts = append(reqCounts, c)
				reqClasses = append(reqClasses, req.Exactly.DeviceClassName)
				reqSelectors = append(reqSelectors, req.Exactly.Selectors)
			} else if len(req.FirstAvailable) > 0 {
				for _, sub := range req.FirstAvailable {
					subReqNames = append(subReqNames, fmt.Sprintf("%s/%s", reqName, sub.Name))
					c := int(sub.Count)
					if c <= 0 {
						c = 1
					}
					reqCounts = append(reqCounts, c)
					reqClasses = append(reqClasses, sub.DeviceClassName)
					reqSelectors = append(reqSelectors, sub.Selectors)
				}
			}

			reqSatisfied := false

			for i, subReqName := range subReqNames {
				count := reqCounts[i]
				devClass := reqClasses[i]
				selectors := reqSelectors[i]

				var tempResults []resourcev1.DeviceRequestAllocationResult
				var tempNodeName string

				for _, slice := range slices.Items {
					if slice.Labels["draforge.oaslananka/managed-by"] != "simulator" {
						continue
					}

					if slice.Labels["draforge.oaslananka/health"] == "unhealthy" {
						continue
					}

					nodeName := ""
					if slice.Spec.NodeName != nil {
						nodeName = *slice.Spec.NodeName
					}

					// Ensure devices for a subrequest come from the same node if node is specified
					if allocatedNodeName != "" && nodeName != "" && nodeName != allocatedNodeName {
						continue
					}
					if tempNodeName != "" && nodeName != "" && nodeName != tempNodeName {
						continue
					}

					// Incorporate DeviceClass/claim selectors if available heuristically
					if devClass != "" {
						reqCls := strings.ToLower(devClass)
						poolName := strings.ToLower(slice.Spec.Pool.Name)
						// E.g., if asking for fpga but pool doesn't match, skip
						if strings.Contains(reqCls, "fpga") && !strings.Contains(poolName, "fpga") {
							continue
						}
						if strings.Contains(reqCls, "gpu") && !strings.Contains(poolName, "gpu") && !strings.Contains(poolName, "nvidia") {
							continue
						}
					}

					if len(selectors) > 0 {
						// Simple heuristic: if CEL requires an attribute we can check, do it.
						// Otherwise, let it pass in simulator.
						celMatched := true
						for _, sel := range selectors {
							if sel.CEL != nil && sel.CEL.Expression != "" {
								expr := strings.ToLower(sel.CEL.Expression)
								if strings.Contains(expr, "fpga") && !strings.Contains(strings.ToLower(slice.Spec.Pool.Name), "fpga") {
									celMatched = false
								}
							}
						}
						if !celMatched {
							continue
						}
					}

					for _, dev := range slice.Spec.Devices {
						devKey := fmt.Sprintf("%s/%s/%s", slice.Spec.Driver, slice.Spec.Pool.Name, dev.Name)
						if chosenDevices[devKey] {
							continue
						}
						if r.isDeviceAllocated(claims.Items, slice.Spec.Driver, slice.Spec.Pool.Name, dev.Name) {
							continue
						}

						tempResults = append(tempResults, resourcev1.DeviceRequestAllocationResult{
							Request: subReqName,
							Driver:  slice.Spec.Driver,
							Pool:    slice.Spec.Pool.Name,
							Device:  dev.Name,
						})

						if tempNodeName == "" {
							tempNodeName = nodeName
						}

						if len(tempResults) == count {
							break
						}
					}

					if len(tempResults) == count {
						break
					}
				}

				if len(tempResults) == count {
					// Mark as chosen
					for _, res := range tempResults {
						devKey := fmt.Sprintf("%s/%s/%s", res.Driver, res.Pool, res.Device)
						chosenDevices[devKey] = true
					}
					finalResults = append(finalResults, tempResults...)
					if allocatedNodeName == "" {
						allocatedNodeName = tempNodeName
					}
					reqSatisfied = true
					break
				}
			}

			if !reqSatisfied {
				allocationSuccess = false
				fmt.Printf("Simulating allocation for claim %s/%s failed: insufficient capacity, unavailability, or no match for request %s\n",
					claim.Namespace, claim.Name, reqName)
				break
			}
		}

		if allocationSuccess && len(finalResults) > 0 {
			fmt.Printf("Simulating allocation for claim %s/%s -> %d devices on node %s\n",
				claim.Namespace, claim.Name, len(finalResults), allocatedNodeName)

			allocatedClaim := claim.DeepCopy()
			allocatedClaim.Status.Allocation = &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: finalResults,
				},
			}

			if allocatedNodeName != "" {
				allocatedClaim.Status.Allocation.NodeSelector = &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchFields: []corev1.NodeSelectorRequirement{
								{
									Key:      "metadata.name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{allocatedNodeName},
								},
							},
						},
					},
				}
			}

			_, err = r.clientset.ResourceV1().ResourceClaims(claim.Namespace).UpdateStatus(ctx, allocatedClaim, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Failed to update status for claim %s: %v\n", claim.Name, err)
			} else {
				atomic.AddInt64(&r.AllocationsSimulated, 1)
				for i := range claims.Items {
					if claims.Items[i].UID == claim.UID {
						claims.Items[i] = *allocatedClaim
						break
					}
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) isDeviceAllocated(claims []resourcev1.ResourceClaim, driverName, poolName, devName string) bool {
	for _, c := range claims {
		if c.Status.Allocation != nil {
			for _, res := range c.Status.Allocation.Devices.Results {
				if res.Driver == driverName && res.Pool == poolName && res.Device == devName {
					return true
				}
			}
		}
	}
	return false
}
