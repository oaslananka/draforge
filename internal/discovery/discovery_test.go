// Package discovery unit tests.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverDRA(t *testing.T) {
	ctx := context.Background()

	// 1. Mock claim
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
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Device: "gpu-0",
							Driver: "gpu-driver",
							Pool:   "gpu-pool",
						},
					},
				},
			},
		},
	}

	// 2. Mock slice
	nodeName := "node-0"
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "slice-1",
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "gpu-driver",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "gpu-0",
				},
			},
		},
	}

	// 3. Mock pod referencing claim
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			ResourceClaims: []corev1.PodResourceClaim{
				{
					Name:              "gpu-ref",
					ResourceClaimName: ptr("claim-1"),
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim, slice, pod)

	pools, devices, claims, err := DiscoverDRA(ctx, clientset)
	if err != nil {
		t.Fatalf("DiscoverDRA failed: %v", err)
	}

	if len(pools) != 1 {
		t.Errorf("expected 1 pool, got %d", len(pools))
	}
	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}
	if len(claims) != 1 {
		t.Errorf("expected 1 claim, got %d", len(claims))
	}

	if claims[0].OwnerPodName != "pod-1" {
		t.Errorf("expected OwnerPodName to be pod-1, got %q", claims[0].OwnerPodName)
	}
}

func ptr[T any](v T) *T {
	return &v
}
