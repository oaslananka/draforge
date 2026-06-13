// Package discovery queries the Kubernetes API for Dynamic Resource Allocation (DRA) resources.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/oaslananka/draforge/pkg/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DiscoverDRA queries the Kubernetes cluster for all DRA-related objects and maps them to our model.
func DiscoverDRA(ctx context.Context, clientset kubernetes.Interface) ([]model.DevicePool, []model.Device, []model.ResourceClaimInfo, error) {
	var pools []model.DevicePool
	var devices []model.Device
	var claims []model.ResourceClaimInfo

	// 1. Discover ResourceClaims
	claimList, err := clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, rc := range claimList.Items {
			status := "Pending"
			allocatedDevice := ""
			allocatedNode := ""
			allocatedDriver := ""

			if rc.Status.Allocation != nil {
				status = "Allocated"
				if rc.Status.Allocation.NodeSelector != nil {
					for _, term := range rc.Status.Allocation.NodeSelector.NodeSelectorTerms {
						for _, req := range term.MatchFields {
							if req.Key == "metadata.name" && len(req.Values) > 0 {
								allocatedNode = req.Values[0]
							}
						}
						for _, req := range term.MatchExpressions {
							if req.Key == "kubernetes.io/hostname" && len(req.Values) > 0 {
								allocatedNode = req.Values[0]
							}
						}
					}
				}
				// Try to extract allocation details
				for _, device := range rc.Status.Allocation.Devices.Results {
					allocatedDevice = device.Device
					allocatedDriver = device.Driver
					if allocatedNode == "" {
						allocatedNode = device.Pool
					}
				}
			}

			className := ""
			if rc.Spec.Devices != nil && len(rc.Spec.Devices.Requests) > 0 {
				if rc.Spec.Devices.Requests[0].Exactly != nil {
					className = rc.Spec.Devices.Requests[0].Exactly.DeviceClassName
				} else if len(rc.Spec.Devices.Requests[0].FirstAvailable) > 0 {
					className = rc.Spec.Devices.Requests[0].FirstAvailable[0].DeviceClassName
				}
			}

			claims = append(claims, model.ResourceClaimInfo{
				Name:            rc.Name,
				Namespace:       rc.Namespace,
				DeviceClassName: className,
				Status:          status,
				AllocatedDevice: allocatedDevice,
				AllocatedNode:   allocatedNode,
				AllocatedDriver: allocatedDriver,
				CreatedAt:       rc.CreationTimestamp.Time,
			})
		}
	} else {
		// Log warning or return empty list if API not supported
		fmt.Printf("Warning: resource.k8s.io/v1 ResourceClaims API not available: %v\n", err)
	}

	// 2. Discover ResourceSlices (to build pools and devices)
	sliceList, err := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err == nil {
		poolMap := make(map[string]*model.DevicePool)

		for _, slice := range sliceList.Items {
			driverName := slice.Spec.Driver
			nodeNameStr := ""
			if slice.Spec.NodeName != nil {
				nodeNameStr = *slice.Spec.NodeName
			}
			poolName := slice.Spec.Pool.Name

			poolKey := driverName + "/" + poolName + "/" + nodeNameStr
			pool, exists := poolMap[poolKey]
			if !exists {
				isSynth := strings.Contains(poolName, "sim") || strings.Contains(driverName, "sim")
				pool = &model.DevicePool{
					Name:        poolName,
					DriverName:  driverName,
					NodeName:    nodeNameStr,
					DeviceType:  "unknown",
					Health:      "healthy",
					IsSynthetic: isSynth,
					Labels:      slice.Labels,
				}
				poolMap[poolKey] = pool
			}

			// Process devices in the slice
			for _, devSpec := range slice.Spec.Devices {
				devType := "unknown"
				if pool.DeviceType != "unknown" {
					devType = pool.DeviceType
				}

				attrs := make(map[string]string)
				caps := make(map[string]int64)

				// Extract attributes
				for attrName, attrVal := range devSpec.Attributes {
					attrNameStr := string(attrName)
					if attrVal.StringValue != nil {
						attrs[attrNameStr] = *attrVal.StringValue
						// Infer type from model attribute if present
						if strings.EqualFold(attrNameStr, "type") || strings.EqualFold(attrNameStr, "model") {
							devType = strings.ToLower(*attrVal.StringValue)
						}
					} else if attrVal.IntValue != nil {
						attrs[attrNameStr] = fmt.Sprintf("%d", *attrVal.IntValue)
					} else if attrVal.BoolValue != nil {
						attrs[attrNameStr] = fmt.Sprintf("%t", *attrVal.BoolValue)
					}
				}
				// Extract capacities
				for capName, capVal := range devSpec.Capacity {
					caps[string(capName)] = capVal.Value.Value()
				}

				pool.DeviceType = devType
				isSynth := strings.Contains(slice.Name, "sim") || strings.Contains(driverName, "sim")

				devices = append(devices, model.Device{
					ID:          slice.Name + "/" + devSpec.Name,
					Name:        devSpec.Name,
					Type:        devType,
					Status:      "healthy", // Default, driver reports health in status field if extended
					NodeName:    nodeNameStr,
					PoolName:    poolName,
					Attributes:  attrs,
					Capacities:  caps,
					IsSynthetic: isSynth,
					LastUpdated: slice.CreationTimestamp.Time,
				})
			}
		}

		for _, p := range poolMap {
			// Count devices in this pool
			count := 0
			for _, d := range devices {
				if d.PoolName == p.Name && d.NodeName == p.NodeName {
					count++
				}
			}
			p.DeviceCount = count
			pools = append(pools, *p)
		}
	} else {
		fmt.Printf("Warning: resource.k8s.io/v1 ResourceSlices API not available: %v\n", err)
	}

	// 3. Map Claims to Owner Pods by querying Pods
	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range podList.Items {
			for _, podClaim := range pod.Spec.ResourceClaims {
				claimName := ""
				if podClaim.ResourceClaimName != nil {
					claimName = *podClaim.ResourceClaimName
				} else if podClaim.ResourceClaimTemplateName != nil {
					// Generated claim name follows pattern: <pod-name>-<claim-entry-name>
					claimName = pod.Name + "-" + podClaim.Name
				}

				if claimName != "" {
					// Update claim owner in the claims list
					for i, c := range claims {
						if c.Name == claimName && c.Namespace == pod.Namespace {
							claims[i].OwnerPodName = pod.Name
						}
					}
				}
			}
		}
	}

	return pools, devices, claims, nil
}
