// Package draeval tests the shared Kubernetes DRA selector evaluator.
// SPDX-License-Identifier: Apache-2.0
package draeval

import (
	"context"
	"strings"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestEvaluatorMatchesTypedKubernetesDeviceData(t *testing.T) {
	integer := int64(3)
	boolean := true
	model := "neutral-accelerator"
	version := "1.3.0"
	device := resourcev1.Device{
		Name: "device-0",
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			"generation": {IntValue: &integer},
			"secure":     {BoolValue: &boolean},
			"model":      {StringValue: &model},
			"firmware":   {VersionValue: &version},
		},
		Capacity: map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{
			"memory": {Value: resource.MustParse("16Gi")},
		},
	}
	classSelectors := []resourcev1.DeviceSelector{{
		CEL: &resourcev1.CELDeviceSelector{Expression: `device.driver == "driver.example.com" && device.attributes["driver.example.com"].model == "neutral-accelerator"`},
	}}
	requestSelectors := []resourcev1.DeviceSelector{
		{CEL: &resourcev1.CELDeviceSelector{Expression: `device.attributes["driver.example.com"].generation >= 3 && device.attributes["driver.example.com"].secure`}},
		{CEL: &resourcev1.CELDeviceSelector{Expression: `device.attributes["driver.example.com"].firmware.isGreaterThan(semver("1.2.0"))`}},
		{CEL: &resourcev1.CELDeviceSelector{Expression: `device.capacity["driver.example.com"].memory.compareTo(quantity("8Gi")) >= 0`}},
	}

	matched, err := NewEvaluator().Matches(
		context.Background(),
		"driver.example.com",
		device,
		classSelectors,
		requestSelectors,
	)
	if err != nil {
		t.Fatalf("Matches returned an error: %v", err)
	}
	if !matched {
		t.Fatal("expected all typed class and request selectors to match")
	}
}

func TestEvaluatorRequiresEverySelector(t *testing.T) {
	value := "match"
	device := resourcev1.Device{
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			"example.com/value": {StringValue: &value},
		},
	}

	matched, err := NewEvaluator().Matches(
		context.Background(),
		"driver.example.com",
		device,
		[]resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{Expression: `device.attributes["example.com"].value == "match"`}}},
		[]resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{Expression: `device.attributes["example.com"].value == "different"`}}},
	)
	if err != nil {
		t.Fatalf("Matches returned an error: %v", err)
	}
	if matched {
		t.Fatal("expected one false request selector to reject the device")
	}
}

func TestEvaluatorFailsClosed(t *testing.T) {
	listValues := []string{"one", "two"}
	tests := []struct {
		name       string
		device     resourcev1.Device
		selectors  []resourcev1.DeviceSelector
		wantErrSub string
	}{
		{
			name:       "selector type is missing",
			selectors:  []resourcev1.DeviceSelector{{}},
			wantErrSub: "unsupported selector",
		},
		{
			name: "invalid expression",
			selectors: []resourcev1.DeviceSelector{{
				CEL: &resourcev1.CELDeviceSelector{Expression: "?!"},
			}},
			wantErrSub: "compile selector",
		},
		{
			name: "list attributes are not silently approximated",
			device: resourcev1.Device{Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
				"names": {StringValues: listValues},
			}},
			selectors: []resourcev1.DeviceSelector{{
				CEL: &resourcev1.CELDeviceSelector{Expression: `device.attributes["driver.example.com"].names.size() == 2`},
			}},
			wantErrSub: "unsupported attribute value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewEvaluator().Matches(
				context.Background(),
				"driver.example.com",
				test.device,
				test.selectors,
			)
			if err == nil {
				t.Fatalf("expected an error containing %q", test.wantErrSub)
			}
			if !strings.Contains(err.Error(), test.wantErrSub) {
				t.Fatalf("error %q does not contain %q", err, test.wantErrSub)
			}
		})
	}
}
