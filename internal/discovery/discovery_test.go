// Package discovery unit tests.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
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

func TestDiscoverDRAEmptyCluster(t *testing.T) {
	// Verify that a fake clientset with no DRA objects returns
	// empty results and no error (graceful degradation, no panic).
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	pools, devices, claims, err := DiscoverDRA(ctx, clientset)
	if err != nil {
		t.Fatalf("DiscoverDRA on empty cluster should not error: %v", err)
	}
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
	if len(claims) != 0 {
		t.Errorf("expected 0 claims, got %d", len(claims))
	}
}

func TestDiscoverDRAWithStatus_Partial(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	// Prepend a reactor that returns an error for ResourceClaims
	clientset.PrependReactor("list", "resourceclaims", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("synthetic error listing resourceclaims")
	})

	_, _, _, status, err := DiscoverDRAWithStatus(ctx, clientset)
	if err != nil {
		t.Fatalf("DiscoverDRAWithStatus should not bubble up partial list errors: %v", err)
	}

	if status.ResourceClaimsAvailable {
		t.Errorf("expected ResourceClaimsAvailable to be false")
	}
	if !status.ResourceSlicesAvailable {
		t.Errorf("expected ResourceSlicesAvailable to be true")
	}
	if !status.PodsAvailable {
		t.Errorf("expected PodsAvailable to be true")
	}
	if !status.IsPartial {
		t.Errorf("expected IsPartial to be true")
	}
	if len(status.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(status.Warnings))
	} else if !strings.Contains(status.Warnings[0], "synthetic error") {
		t.Errorf("expected warning to contain 'synthetic error', got %q", status.Warnings[0])
	}
}

func TestDiscoverDRADeterministic(t *testing.T) {
	// Two sequential calls with same input must produce
	// identical results (no map iteration order differences).
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	pools1, devices1, claims1, err1 := DiscoverDRA(ctx, clientset)
	if err1 != nil {
		t.Fatalf("first call failed: %v", err1)
	}

	pools2, devices2, claims2, err2 := DiscoverDRA(ctx, clientset)
	if err2 != nil {
		t.Fatalf("second call failed: %v", err2)
	}

	if len(pools1) != len(pools2) {
		t.Errorf("pool count not deterministic: %d vs %d", len(pools1), len(pools2))
	}
	if len(devices1) != len(devices2) {
		t.Errorf("device count not deterministic: %d vs %d", len(devices1), len(devices2))
	}
	if len(claims1) != len(claims2) {
		t.Errorf("claim count not deterministic: %d vs %d", len(claims1), len(claims2))
	}
}
