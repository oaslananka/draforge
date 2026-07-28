// Package simulator tests shareable-device capacity policy evaluation.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"strings"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func quantity(value string) resource.Quantity {
	return resource.MustParse(value)
}

func quantityMap(values map[resourcev1.QualifiedName]string) map[resourcev1.QualifiedName]resource.Quantity {
	result := make(map[resourcev1.QualifiedName]resource.Quantity, len(values))
	for name, value := range values {
		result[name] = quantity(value)
	}
	return result
}

func capacityMap(values map[resourcev1.QualifiedName]string) map[resourcev1.QualifiedName]resourcev1.DeviceCapacity {
	result := make(map[resourcev1.QualifiedName]resourcev1.DeviceCapacity, len(values))
	for name, value := range values {
		result[name] = resourcev1.DeviceCapacity{Value: quantity(value)}
	}
	return result
}

func requireQuantity(t *testing.T, actual map[resourcev1.QualifiedName]resource.Quantity, name resourcev1.QualifiedName, expected string) {
	t.Helper()
	value, found := actual[name]
	if !found {
		t.Fatalf("expected consumed capacity %q, got %#v", name, actual)
	}
	wanted := quantity(expected)
	if value.Cmp(wanted) != 0 {
		t.Fatalf("expected consumed capacity %q=%s, got %s", name, wanted.String(), value.String())
	}
}

func TestEvaluateShareableCapacityUsesFullCapacityWithoutRequestPolicy(t *testing.T) {
	capacities := capacityMap(map[resourcev1.QualifiedName]string{
		"memory": "16Gi",
		"cores":  "8",
	})

	consumed, fits, err := evaluateShareableCapacity(nil, capacities, nil, nil)
	if err != nil {
		t.Fatalf("evaluate shareable capacity: %v", err)
	}
	if !fits {
		t.Fatal("expected full-capacity default consumption to fit an unused device")
	}
	requireQuantity(t, consumed, "memory", "16Gi")
	requireQuantity(t, consumed, "cores", "8")
}

func TestEvaluateShareableCapacityUsesPolicyDefaultForUnrequestedCapacity(t *testing.T) {
	defaultMemory := quantity("2Gi")
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	memory := capacities["memory"]
	memory.RequestPolicy = &resourcev1.CapacityRequestPolicy{Default: &defaultMemory}
	capacities["memory"] = memory

	consumed, fits, err := evaluateShareableCapacity(nil, capacities, nil, nil)
	if err != nil {
		t.Fatalf("evaluate shareable capacity: %v", err)
	}
	if !fits {
		t.Fatal("expected policy default to fit")
	}
	requireQuantity(t, consumed, "memory", "2Gi")
}

func TestEvaluateShareableCapacityRoundsToNextValidValue(t *testing.T) {
	defaultValue := quantity("2")
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"shares": "8"})
	shares := capacities["shares"]
	shares.RequestPolicy = &resourcev1.CapacityRequestPolicy{
		Default:     &defaultValue,
		ValidValues: []resource.Quantity{quantity("2"), quantity("4"), quantity("8")},
	}
	capacities["shares"] = shares
	requirements := &resourcev1.CapacityRequirements{Requests: quantityMap(map[resourcev1.QualifiedName]string{"shares": "3"})}

	consumed, fits, err := evaluateShareableCapacity(requirements, capacities, nil, nil)
	if err != nil {
		t.Fatalf("evaluate shareable capacity: %v", err)
	}
	if !fits {
		t.Fatal("expected request to round to the next valid value")
	}
	requireQuantity(t, consumed, "shares", "4")
}

func TestEvaluateShareableCapacityRoundsWithinValidRange(t *testing.T) {
	minimum := quantity("2")
	maximum := quantity("10")
	step := quantity("2")
	defaultValue := quantity("2")
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"shares": "10"})
	shares := capacities["shares"]
	shares.RequestPolicy = &resourcev1.CapacityRequestPolicy{
		Default: &defaultValue,
		ValidRange: &resourcev1.CapacityRequestPolicyRange{
			Min:  &minimum,
			Max:  &maximum,
			Step: &step,
		},
	}
	capacities["shares"] = shares

	for name, testCase := range map[string][2]string{
		"below minimum": {"1", "2"},
		"between steps": {"5", "6"},
		"on step":       {"8", "8"},
	} {
		t.Run(name, func(t *testing.T) {
			requested, expected := testCase[0], testCase[1]
			requirements := &resourcev1.CapacityRequirements{Requests: quantityMap(map[resourcev1.QualifiedName]string{"shares": requested})}
			consumed, fits, err := evaluateShareableCapacity(requirements, capacities, nil, nil)
			if err != nil {
				t.Fatalf("evaluate shareable capacity: %v", err)
			}
			if !fits {
				t.Fatalf("expected request %s to fit", requested)
			}
			requireQuantity(t, consumed, "shares", expected)
		})
	}
}

func TestEvaluateShareableCapacityRejectsMissingOrPolicyExceedingRequests(t *testing.T) {
	defaultValue := quantity("2")
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"shares": "8"})
	shares := capacities["shares"]
	shares.RequestPolicy = &resourcev1.CapacityRequestPolicy{
		Default:     &defaultValue,
		ValidValues: []resource.Quantity{quantity("2"), quantity("4"), quantity("8")},
	}
	capacities["shares"] = shares

	for name, requirements := range map[string]*resourcev1.CapacityRequirements{
		"missing capacity":   {Requests: quantityMap(map[resourcev1.QualifiedName]string{"memory": "1Gi"})},
		"above valid values": {Requests: quantityMap(map[resourcev1.QualifiedName]string{"shares": "9"})},
	} {
		t.Run(name, func(t *testing.T) {
			consumed, fits, err := evaluateShareableCapacity(requirements, capacities, nil, nil)
			if err != nil {
				t.Fatalf("evaluate shareable capacity: %v", err)
			}
			if fits || consumed != nil {
				t.Fatalf("expected request to be ineligible, got fits=%t consumed=%#v", fits, consumed)
			}
		})
	}
}

func TestEvaluateShareableCapacityIncludesCommittedAndPendingUsage(t *testing.T) {
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"memory": "16Gi"})
	requirements := &resourcev1.CapacityRequirements{Requests: quantityMap(map[resourcev1.QualifiedName]string{"memory": "4Gi"})}
	committed := quantityMap(map[resourcev1.QualifiedName]string{"memory": "8Gi"})

	for name, testCase := range map[string]struct {
		values map[resourcev1.QualifiedName]resource.Quantity
		fits   bool
	}{
		"exact remaining capacity": {values: quantityMap(map[resourcev1.QualifiedName]string{"memory": "4Gi"}), fits: true},
		"exhausted by pending use": {values: quantityMap(map[resourcev1.QualifiedName]string{"memory": "5Gi"}), fits: false},
	} {
		t.Run(name, func(t *testing.T) {
			consumed, fits, err := evaluateShareableCapacity(requirements, capacities, committed, testCase.values)
			if err != nil {
				t.Fatalf("evaluate shareable capacity: %v", err)
			}
			if fits != testCase.fits {
				t.Fatalf("expected fits=%t, got %t with consumed %#v", testCase.fits, fits, consumed)
			}
			if fits {
				requireQuantity(t, consumed, "memory", "4Gi")
			} else if consumed != nil {
				t.Fatalf("expected no consumption result for exhausted device, got %#v", consumed)
			}
		})
	}
}

func TestEvaluateShareableCapacityRejectsMalformedPolicy(t *testing.T) {
	defaultValue := quantity("2")
	minimum := quantity("1")
	maximum := quantity("8")
	step := quantity("2")

	tests := map[string]resourcev1.DeviceCapacity{
		"negative total": {Value: quantity("-1")},
		"both policy modes": {
			Value: quantity("8"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default:     &defaultValue,
				ValidValues: []resource.Quantity{quantity("2"), quantity("4")},
				ValidRange:  &resourcev1.CapacityRequestPolicyRange{Min: &minimum, Max: &maximum, Step: &step},
			},
		},
		"unsorted valid values": {
			Value: quantity("8"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default:     &defaultValue,
				ValidValues: []resource.Quantity{quantity("4"), quantity("2")},
			},
		},
		"zero range step": {
			Value: quantity("8"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default: &defaultValue,
				ValidRange: &resourcev1.CapacityRequestPolicyRange{
					Min: &minimum, Max: &maximum, Step: func() *resource.Quantity { value := quantity("0"); return &value }(),
				},
			},
		},
	}

	for name, capacity := range tests {
		t.Run(name, func(t *testing.T) {
			_, fits, err := evaluateShareableCapacity(nil, map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"shares": capacity}, nil, nil)
			if err == nil {
				t.Fatalf("expected malformed policy error, got fits=%t", fits)
			}
			if fits || !strings.Contains(err.Error(), "shares") {
				t.Fatalf("expected fail-closed shares diagnostic, got fits=%t err=%v", fits, err)
			}
		})
	}
}

func TestEvaluateShareableCapacityRangeRoundingUsesBinarySI(t *testing.T) {
	minimum := quantity("2")
	maximum := quantity("10")
	step := quantity("2")
	defaultValue := quantity("2")
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"shares": "10"})
	shares := capacities["shares"]
	shares.RequestPolicy = &resourcev1.CapacityRequestPolicy{
		Default: &defaultValue,
		ValidRange: &resourcev1.CapacityRequestPolicyRange{
			Min:  &minimum,
			Max:  &maximum,
			Step: &step,
		},
	}
	capacities["shares"] = shares
	requirements := &resourcev1.CapacityRequirements{Requests: quantityMap(map[resourcev1.QualifiedName]string{"shares": "5"})}

	consumed, fits, err := evaluateShareableCapacity(requirements, capacities, nil, nil)
	if err != nil {
		t.Fatalf("evaluate shareable capacity: %v", err)
	}
	if !fits {
		t.Fatal("expected rounded request to fit")
	}
	if consumed["shares"].Format != resource.BinarySI {
		t.Fatalf("expected BinarySI rounding output, got %q", consumed["shares"].Format)
	}
}

func TestEvaluateShareableCapacityRejectsAdditionalMalformedPolicyShapes(t *testing.T) {
	defaultValue := quantity("1")
	minimum := quantity("1")
	maximum := quantity("8")
	step := quantity("2")
	tooLargeStep := quantity("2")
	largeMinimum := quantity("7")

	tooManyValues := make([]resource.Quantity, 11)
	for index := range tooManyValues {
		tooManyValues[index] = *resource.NewQuantity(int64(index+1), resource.DecimalSI)
	}

	tests := map[string]resourcev1.DeviceCapacity{
		"too many valid values": {
			Value: quantity("16"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default:     &defaultValue,
				ValidValues: tooManyValues,
			},
		},
		"minimum plus step exceeds total": {
			Value: quantity("8"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default: &largeMinimum,
				ValidRange: &resourcev1.CapacityRequestPolicyRange{
					Min:  &largeMinimum,
					Step: &tooLargeStep,
				},
			},
		},
		"maximum is not step aligned": {
			Value: quantity("8"),
			RequestPolicy: &resourcev1.CapacityRequestPolicy{
				Default: &defaultValue,
				ValidRange: &resourcev1.CapacityRequestPolicyRange{
					Min:  &minimum,
					Max:  &maximum,
					Step: &step,
				},
			},
		},
	}

	for name, capacity := range tests {
		t.Run(name, func(t *testing.T) {
			_, fits, err := evaluateShareableCapacity(nil, map[resourcev1.QualifiedName]resourcev1.DeviceCapacity{"shares": capacity}, nil, nil)
			if err == nil || fits {
				t.Fatalf("expected malformed policy error, got fits=%t err=%v", fits, err)
			}
		})
	}
}

func TestEvaluateShareableCapacityRejectsNegativeRequestAndUsage(t *testing.T) {
	capacities := capacityMap(map[resourcev1.QualifiedName]string{"shares": "8"})
	for name, testCase := range map[string]struct {
		requirements *resourcev1.CapacityRequirements
		committed    map[resourcev1.QualifiedName]resource.Quantity
		pending      map[resourcev1.QualifiedName]resource.Quantity
	}{
		"negative request": {
			requirements: &resourcev1.CapacityRequirements{Requests: quantityMap(map[resourcev1.QualifiedName]string{"shares": "-1"})},
		},
		"negative committed usage": {committed: quantityMap(map[resourcev1.QualifiedName]string{"shares": "-1"})},
		"negative pending usage":   {pending: quantityMap(map[resourcev1.QualifiedName]string{"shares": "-1"})},
	} {
		t.Run(name, func(t *testing.T) {
			_, fits, err := evaluateShareableCapacity(testCase.requirements, capacities, testCase.committed, testCase.pending)
			if err == nil || fits {
				t.Fatalf("expected fail-closed negative quantity error, got fits=%t err=%v", fits, err)
			}
		})
	}
}
