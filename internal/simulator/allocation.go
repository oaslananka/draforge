// Package simulator allocation simulation for fallback cluster environments.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/cel-go/cel"
	corev1 "k8s.io/api/core/v1"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func matchDevice(dev resourcev1.Device, classSelectors []resourcev1.DeviceSelector, reqSelectors []resourcev1.DeviceSelector, reqCap *resourcev1.CapacityRequirements) bool {
	if reqCap != nil {
		for reqKey, reqVal := range reqCap.Requests {
			devVal, ok := dev.Capacity[reqKey]
			if !ok || devVal.Value.Cmp(reqVal) < 0 {
				return false
			}
		}
	}

	env, _ := cel.NewEnv(
		cel.Variable("device", cel.MapType(cel.StringType, cel.AnyType)),
	)

	deviceMap := map[string]interface{}{
		"attributes": map[string]interface{}{},
		"capacity":   map[string]interface{}{},
	}

	for k, v := range dev.Attributes {
		if v.StringValue != nil {
			deviceMap["attributes"].(map[string]interface{})[string(k)] = *v.StringValue
		} else if v.IntValue != nil {
			deviceMap["attributes"].(map[string]interface{})[string(k)] = *v.IntValue
		} else if v.BoolValue != nil {
			deviceMap["attributes"].(map[string]interface{})[string(k)] = *v.BoolValue
		}
	}
	for k, v := range dev.Capacity {
		deviceMap["capacity"].(map[string]interface{})[string(k)] = v.Value.Value()
	}

	matchSelectors := func(selectors []resourcev1.DeviceSelector) bool {
		for _, sel := range selectors {
			if sel.CEL != nil {
				ast, iss := env.Compile(sel.CEL.Expression)
				if iss.Err() == nil {
					if prg, err := env.Program(ast); err == nil {
						if out, _, err := prg.Eval(map[string]interface{}{"device": deviceMap}); err == nil {
							if b, ok := out.Value().(bool); ok {
								if !b {
									return false
								}
								continue
							}
						}
					}
				}
				return false
			}
		}
		return true
	}
	return matchSelectors(classSelectors) && matchSelectors(reqSelectors)
}

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

	classes, err := r.clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	classMap := make(map[string]*resourcev1.DeviceClass)
	for i := range classes.Items {
		classMap[classes.Items[i].Name] = &classes.Items[i]
	}

	for _, claim := range claims.Items {
		// Only allocate pending claims
		if claim.Status.Allocation != nil {
			continue
		}

		var claimResults []resourcev1.DeviceRequestAllocationResult
		nodeSelectors := make(map[string]bool)
		claimFullySatisfied := true

		// Create a transient view of already allocated devices, including those allocated in previous requests of this claim
		allocatedDevices := make(map[string]bool)
		for _, c := range claims.Items {
			if c.Status.Allocation != nil {
				for _, res := range c.Status.Allocation.Devices.Results {
					allocatedDevices[res.Pool+"/"+res.Device] = true
				}
			}
		}

		for _, req := range claim.Spec.Devices.Requests {
			var subReqs []resourcev1.DeviceSubRequest
			if req.Exactly != nil {
				subReqs = append(subReqs, resourcev1.DeviceSubRequest{
					Name:            req.Name,
					DeviceClassName: req.Exactly.DeviceClassName,
					Selectors:       req.Exactly.Selectors,
					AllocationMode:  req.Exactly.AllocationMode,
					Count:           req.Exactly.Count,
					Capacity:        req.Exactly.Capacity,
				})
			} else if len(req.FirstAvailable) > 0 {
				subReqs = req.FirstAvailable
			}

			reqSatisfied := false

			for _, subReq := range subReqs {
				targetCount := subReq.Count
				if targetCount == 0 {
					targetCount = 1
				}

				var classSelectors []resourcev1.DeviceSelector
				if dc, ok := classMap[subReq.DeviceClassName]; ok {
					classSelectors = dc.Spec.Selectors
				} else if subReq.DeviceClassName != "" {
					continue
				}

				var reqResults []resourcev1.DeviceRequestAllocationResult

				for _, slice := range slices.Items {
					if len(reqResults) >= int(targetCount) {
						break
					}
					if slice.Labels["draforge.oaslananka/managed-by"] != "simulator" {
						continue
					}
					if slice.Labels["draforge.oaslananka/health"] == "unhealthy" {
						continue
					}

					for _, dev := range slice.Spec.Devices {
						if len(reqResults) >= int(targetCount) {
							break
						}
						devID := slice.Spec.Pool.Name + "/" + dev.Name
						if allocatedDevices[devID] {
							continue
						}

						if matchDevice(dev, classSelectors, subReq.Selectors, subReq.Capacity) {
							allocatedDevices[devID] = true
							reqResults = append(reqResults, resourcev1.DeviceRequestAllocationResult{
								Request: req.Name,
								Device:  dev.Name,
								Driver:  slice.Spec.Driver,
								Pool:    slice.Spec.Pool.Name,
							})
							if slice.Spec.NodeName != nil {
								nodeSelectors[*slice.Spec.NodeName] = true
							}
						}
					}
				}

				if len(reqResults) == int(targetCount) {
					claimResults = append(claimResults, reqResults...)
					reqSatisfied = true
					break // Break out of subReqs loop since FirstAvailable condition met
				} else {
					// Subreq failed, rollback allocated devices for this subreq
					for _, res := range reqResults {
						devID := res.Pool + "/" + res.Device
						delete(allocatedDevices, devID)
					}
				}
			}

			if !reqSatisfied {
				claimFullySatisfied = false
				break
			}
		}

		if claimFullySatisfied && len(claimResults) > 0 {
			var nodeSelectorTerms []corev1.NodeSelectorTerm
			for nodeName := range nodeSelectors {
				nodeSelectorTerms = append(nodeSelectorTerms, corev1.NodeSelectorTerm{
					MatchFields: []corev1.NodeSelectorRequirement{
						{
							Key:      "metadata.name",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{nodeName},
						},
					},
				})
			}

			allocatedClaim := claim.DeepCopy()
			allocatedClaim.Status.Allocation = &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: claimResults,
				},
			}
			if len(nodeSelectorTerms) > 0 {
				allocatedClaim.Status.Allocation.NodeSelector = &corev1.NodeSelector{
					NodeSelectorTerms: nodeSelectorTerms,
				}
			}

			_, updateErr := r.clientset.ResourceV1().ResourceClaims(claim.Namespace).UpdateStatus(ctx, allocatedClaim, metav1.UpdateOptions{})
			if updateErr != nil {
				fmt.Printf("Failed to update status for claim %s: %v\n", claim.Name, updateErr)
			} else {
				atomic.AddInt64(&r.AllocationsSimulated, 1)
				for i := range claims.Items {
					if claims.Items[i].UID == claim.UID {
						claims.Items[i] = *allocatedClaim
						break
					}
				}
				fmt.Printf("Simulating allocation for claim %s/%s -> %d devices\n", claim.Namespace, claim.Name, len(claimResults))
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
