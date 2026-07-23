// Package discovery tests complete DRA allocation identity preservation.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"reflect"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverDRAPreservesAllRequestsAndAllocations(t *testing.T) {
	claims := discoverClaimsForTest(t, multiRequestClaim())
	claim := claims[0]
	assertRequestCollections(t, claim)
	assertAllocationCollections(t, claim)
}

func multiRequestClaim() *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
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
}

func discoverClaimsForTest(t *testing.T, claim *resourcev1.ResourceClaim) []model.ResourceClaimInfo {
	t.Helper()
	_, _, claims, err := DiscoverDRA(context.Background(), fake.NewSimpleClientset(claim))
	if err != nil {
		t.Fatalf("DiscoverDRA: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(claims))
	}
	return claims
}

func assertRequestCollections(t *testing.T, claim model.ResourceClaimInfo) {
	t.Helper()
	if len(claim.Requests) != 2 {
		t.Fatalf("requests = %#v, want two top-level requests", claim.Requests)
	}
	exact := claim.Requests[0]
	if exact.Name != "gpu" || exact.Mode != "Exactly" || len(exact.Alternatives) != 1 {
		t.Fatalf("exact request not preserved: %#v", exact)
	}
	if exact.Alternatives[0].DeviceClassName != "gpu-class" || exact.Alternatives[0].Count != 2 {
		t.Fatalf("exact request details not preserved: %#v", exact.Alternatives[0])
	}
	firstAvailable := claim.Requests[1]
	if firstAvailable.Mode != "FirstAvailable" || len(firstAvailable.Alternatives) != 2 {
		t.Fatalf("firstAvailable request not preserved: %#v", firstAvailable)
	}
	classes := []string{
		exact.Alternatives[0].DeviceClassName,
		firstAvailable.Alternatives[0].DeviceClassName,
		firstAvailable.Alternatives[1].DeviceClassName,
	}
	if !reflect.DeepEqual(classes, []string{"gpu-class", "fpga-class", "nic-class"}) {
		t.Fatalf("classes = %v", classes)
	}
}

func assertAllocationCollections(t *testing.T, claim model.ResourceClaimInfo) {
	t.Helper()
	if len(claim.Allocations) != 3 {
		t.Fatalf("allocations = %#v, want all three results", claim.Allocations)
	}
	allocation := claim.Allocations[2]
	if allocation.Request != "accelerator/nic" || allocation.DriverName != "driver-b.example" || allocation.PoolName != "shared" || allocation.DeviceName != "dev-0" {
		t.Fatalf("allocation identity lost: %#v", allocation)
	}
	for _, result := range claim.Allocations {
		if result.NodeName != "" {
			t.Fatalf("missing node identity must remain unknown, got %#v", result)
		}
	}
	if claim.AllocatedNode != "" {
		t.Fatalf("legacy allocatedNode must not fall back to pool name: %q", claim.AllocatedNode)
	}
}

func TestDiscoverDRADistinguishesDriversPoolsAndClusterScopedDevices(t *testing.T) {
	pools, devices := discoverIdentityFixtures(t)
	assertIdentityCounts(t, pools, devices)
	assertDeviceIdentities(t, devices)
	assertPoolIdentities(t, pools)
}

func discoverIdentityFixtures(t *testing.T) ([]model.DevicePool, []model.Device) {
	t.Helper()
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
	return pools, devices
}

func assertIdentityCounts(t *testing.T, pools []model.DevicePool, devices []model.Device) {
	t.Helper()
	if len(pools) != 4 {
		t.Fatalf("pools = %d, want four driver/pool/node identities: %#v", len(pools), pools)
	}
	if len(devices) != 4 {
		t.Fatalf("devices = %d, want 4", len(devices))
	}
}

func assertDeviceIdentities(t *testing.T, devices []model.Device) {
	t.Helper()
	seenIDs := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		if _, exists := seenIDs[device.ID]; exists {
			t.Fatalf("duplicate device ID %q", device.ID)
		}
		seenIDs[device.ID] = struct{}{}
		assertDeviceScopeAndOrigin(t, device)
	}
}

func assertDeviceScopeAndOrigin(t *testing.T, device model.Device) {
	t.Helper()
	if device.Name == "dev-cluster" && device.NodeName != "" {
		t.Fatalf("cluster-scoped device node = %q, want unknown", device.NodeName)
	}
	if device.Name == "dev-real" && device.IsSynthetic {
		t.Fatalf("name substring must not mark device synthetic: %#v", device)
	}
}

func assertPoolIdentities(t *testing.T, pools []model.DevicePool) {
	t.Helper()
	for _, pool := range pools {
		assertPoolOrigin(t, pool)
		if pool.DeviceCount != 1 {
			t.Fatalf("pool %#v counted devices from another driver/node", pool)
		}
	}
}

func assertPoolOrigin(t *testing.T, pool model.DevicePool) {
	t.Helper()
	if pool.Name == "shared" && pool.DriverName == "driver-a.example" && pool.NodeName == "node-a" && !pool.IsSynthetic {
		t.Fatal("explicit simulator label was not preserved on pool")
	}
	if pool.Name == "sim-pool" && pool.IsSynthetic {
		t.Fatalf("pool name substring must not mark pool synthetic: %#v", pool)
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
