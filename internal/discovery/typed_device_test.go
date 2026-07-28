// Package discovery tests preservation of typed Kubernetes DRA selector inputs.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverDRAPreservesTypedSelectorDataOutsidePublicJSON(t *testing.T) {
	nodeName := "node-a"
	integer := int64(3)
	boolean := true
	version := "1.3.0"
	allowMultiple := true
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "typed-slice"},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "driver.example.com",
			NodeName: &nodeName,
			Pool:     resourcev1.ResourcePool{Name: "plain-pool"},
			Devices: []resourcev1.Device{{
				Name: "device-0",
				Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
					"generation": {IntValue: &integer},
					"secure":     {BoolValue: &boolean},
					"firmware":   {VersionValue: &version},
				},
				Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{
					"memory": {Value: resource.MustParse("16Gi")},
				},
				AllowMultipleAllocations: &allowMultiple,
			}},
		},
	}

	_, devices, _, err := DiscoverDRA(context.Background(), fake.NewSimpleClientset(slice))
	if err != nil {
		t.Fatalf("DiscoverDRA failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device, got %d", len(devices))
	}
	device := devices[0]
	if got := device.DRAAttributes["generation"].IntValue; got == nil || *got != integer {
		t.Fatalf("typed integer attribute was not preserved: %#v", got)
	}
	if got := device.DRAAttributes["firmware"].VersionValue; got == nil || *got != version {
		t.Fatalf("typed version attribute was not preserved: %#v", got)
	}
	if got := device.DRACapacity["memory"].Value; got.Cmp(resource.MustParse("16Gi")) != 0 {
		t.Fatalf("quantity capacity was not preserved: %s", got.String())
	}
	if device.DRAAllowMultipleAllocations == nil || !*device.DRAAllowMultipleAllocations {
		t.Fatal("allowMultipleAllocations was not preserved")
	}

	encoded, err := json.Marshal(device)
	if err != nil {
		t.Fatalf("marshal device: %v", err)
	}
	for _, internalField := range []string{"DRAAttributes", "DRACapacity", "DRAAllowMultipleAllocations"} {
		if strings.Contains(string(encoded), internalField) {
			t.Fatalf("internal selector data leaked into public JSON: %s", encoded)
		}
	}
}
