// Package discovery tests complete DRA allocation identity preservation.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"reflect"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverDRAPreservesAllRequestsAndAllocations(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "team-a"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{
			{
				Name: "gpu",
				Exactly: &resourcev1.ExactDeviceRequest{
					DeviceClassName: "gpu-class",
					AllocationMode:  resourcev1.DeviceAllocationMode("ExactCount"),
					Count:           2,
				},
			},
			{
				Name: "accelerator",
				FirstAvailable: []resourcev1.DeviceSubRequest{
					{Name: "fpga", DeviceClassName: "fpga-class", Count: 1},
					{Name: "nic", DeviceClassName: "nic-class", AllocationMode: resourcev1.DeviceAllocationMode("All")},
				},
			},
		}}},
		Status: resourcev1.ResourceClaimStatus{Allocation: &resourcev1.AllocationResult{
			Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{
				{Request: "gpu", Driver: "driver-a.example", Pool: "shared", Device: "dev-0"},
				{Request: "gpu", Driver: "driver-a.example", Pool: "shared", Device: "dev-1"},
				{Request: "accelerator/nic", Driver: "driver-b.example", Pool: "shared", Device: "dev-0"},
			}},
		}},
	}

	client := fake.NewSimpleClientset(claim)
	_, _, claims, err := DiscoverDRA(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverDRA: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(claims))
	}
	got := claims[0]
	if len(got.Requests) != 2 {
		t.Fatalf("requests = %#v, want two top-level requests", got.Requests)
	}
	if got.Requests[0].Name != "gpu" || got.Requests[0].Mode != "Exactly" || len(got.Requests[0].Alternatives) != 1 {
		t.Fatalf("exact request not preserved: %#v", got.Requests[0])
	}
	if got.Requests[0].Alternatives[0].DeviceClassName != "gpu-class" || got.Requests[0].Alternatives[0].Count != 2 {
		t.Fatalf("exact request details not preserved: %#v", got.Requests[0].Alternatives[0])
	}
	if got.Requests[1].Mode != "FirstAvailable" || len(got.Requests[1].Alternatives) != 2 {
		t.Fatalf("firstAvailable request not preserved: %#v", got.Requests[1])
	}
	classes := []string{
		got.Requests[0].Alternatives[0].DeviceClassName,
		got.Requests[1].Alternatives[0].DeviceClassName,
		got.Requests[1].Alternatives[1].DeviceClassName,
	}
	if !reflect.DeepEqual(classes, []string{"gpu-class", "fpga-class", "nic-class"}) {
		t.Fatalf("classes = %v", classes)
	}
	if len(got.Allocations) != 3 {
		t.Fatalf("allocations = %#v, want all three results", got.Allocations)
	}
	if got.Allocations[2].Request != "accelerator/nic" || got.Allocations[2].DriverName != "driver-b.example" || got.Allocations[2].PoolName != "shared" || got.Allocations[2].DeviceName != "dev-0" {
		t.Fatalf("allocation identity lost: %#v", got.Allocations[2])
	}
	for _, allocation := range got.Allocations {
		if allocation.NodeName != "" {
			t.Fatalf("missing node identity must remain unknown, got %#v", allocation)
		}
	}
	if got.AllocatedNode != "" {
		t.Fatalf("legacy allocatedNode must not fall back to pool name: %q", got.AllocatedNode)
	}
}

func TestDiscoverDRADistinguishesDriversPoolsAndClusterScopedDevices(t *testing.T) {
	node := "node-a"
	objects := []runtime.Object{
		resourceSlice("slice-a", "driver-a.example", "shared", &node, "dev-0", map[string]string{
			"draforge.oaslananka/managed-by": "simulator",
		}),
		resourceSlice("slice-b", "driver-b.example", "shared", &node, "dev-0", nil),
		resourceSlice("slice-c", "driver-a.example", "shared", nil, "dev-cluster", nil),
		resourceSlice("sim-looking-name", "ordinary.example", "sim-pool", &node, "dev-real", nil),
	}

	pools, devices, _, err := DiscoverDRA(context.Background(), fake.NewSimpleClientset(objects...))
	if err != nil {
		t.Fatalf("DiscoverDRA: %v", err)
	}
	if len(pools) != 4 {
		t.Fatalf("pools = %d, want four driver/pool/node identities: %#v", len(pools), pools)
	}
	if len(devices) != 4 {
		t.Fatalf("devices = %d, want 4", len(devices))
	}

	seenIDs := map[string]bool{}
	for _, device := range devices {
		if seenIDs[device.ID] {
			t.Fatalf("duplicate device ID %q", device.ID)
		}
		seenIDs[device.ID] = true
		if device.Name == "dev-cluster" && device.NodeName != "" {
			t.Fatalf("cluster-scoped device node = %q, want unknown", device.NodeName)
		}
		if device.Name == "dev-real" && device.IsSynthetic {
			t.Fatalf("name substring must not mark device synthetic: %#v", device)
		}
	}

	for _, pool := range pools {
		if pool.Name == "shared" && pool.DriverName == "driver-a.example" && pool.NodeName == node && !pool.IsSynthetic {
			t.Fatal("explicit simulator label was not preserved on pool")
		}
		if pool.Name == "sim-pool" && pool.IsSynthetic {
			t.Fatalf("pool name substring must not mark pool synthetic: %#v", pool)
		}
		if pool.DeviceCount != 1 {
			t.Fatalf("pool %#v counted devices from another driver/node", pool)
		}
	}
}

func TestDiscoverDRAOrderingAndIDsAreStable(t *testing.T) {
	node := "node-a"
	objects := []runtime.Object{
		resourceSlice("z-slice", "driver-b.example", "same", &node, "dev-0", nil),
		resourceSlice("a-slice", "driver-a.example", "same", &node, "dev-0", nil),
	}
	client := fake.NewSimpleClientset(objects...)

	pools1, devices1, claims1, err := DiscoverDRA(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	pools2, devices2, claims2, err := DiscoverDRA(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pools1, pools2) || !reflect.DeepEqual(devices1, devices2) || !reflect.DeepEqual(claims1, claims2) {
		t.Fatalf("repeated discovery changed output:\n%#v\n%#v", devices1, devices2)
	}
	if devices1[0].ID == devices1[1].ID {
		t.Fatalf("different drivers produced same device ID: %#v", devices1)
	}
}

func resourceSlice(name, driver, pool string, node *string, device string, labels map[string]string) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driver,
			NodeName: node,
			Pool:     resourcev1.ResourcePool{Name: pool},
			Devices:  []resourcev1.Device{{Name: device}},
		},
	}
}
