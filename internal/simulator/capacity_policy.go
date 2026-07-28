// Package simulator contains typed DRA allocation helpers.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const maxCapacityPolicyValidValues = 10

func evaluateShareableCapacity(
	requirements *resourcev1.CapacityRequirements,
	capacities map[resourcev1.QualifiedName]resourcev1.DeviceCapacity,
	committed map[resourcev1.QualifiedName]resource.Quantity,
	pending map[resourcev1.QualifiedName]resource.Quantity,
) (map[resourcev1.QualifiedName]resource.Quantity, bool, error) {
	eligible, err := validateShareableCapacityInput(requirements, capacities, committed, pending)
	if err != nil || !eligible {
		return nil, eligible, err
	}

	consumed := make(map[resourcev1.QualifiedName]resource.Quantity, len(capacities))
	for name, capacity := range capacities {
		amount, fits, evaluationErr := evaluateShareableCapacityEntry(name, capacity, requirements, committed, pending)
		if evaluationErr != nil || !fits {
			return nil, fits, evaluationErr
		}
		consumed[name] = amount
	}
	return consumed, true, nil
}

func validateShareableCapacityInput(
	requirements *resourcev1.CapacityRequirements,
	capacities map[resourcev1.QualifiedName]resourcev1.DeviceCapacity,
	committed map[resourcev1.QualifiedName]resource.Quantity,
	pending map[resourcev1.QualifiedName]resource.Quantity,
) (bool, error) {
	if err := validateCapacityUsage("committed", committed, capacities); err != nil {
		return false, err
	}
	if err := validateCapacityUsage("pending", pending, capacities); err != nil {
		return false, err
	}
	return validateCapacityRequirements(requirements, capacities)
}

func validateCapacityRequirements(
	requirements *resourcev1.CapacityRequirements,
	capacities map[resourcev1.QualifiedName]resourcev1.DeviceCapacity,
) (bool, error) {
	if requirements == nil {
		return true, nil
	}
	for name, requested := range requirements.Requests {
		if requested.Sign() < 0 {
			return false, fmt.Errorf("capacity %q has negative requested quantity %s", name, requested.String())
		}
		if _, found := capacities[name]; !found {
			return false, nil
		}
	}
	return true, nil
}

func evaluateShareableCapacityEntry(
	name resourcev1.QualifiedName,
	capacity resourcev1.DeviceCapacity,
	requirements *resourcev1.CapacityRequirements,
	committed map[resourcev1.QualifiedName]resource.Quantity,
	pending map[resourcev1.QualifiedName]resource.Quantity,
) (resource.Quantity, bool, error) {
	if err := validateDeviceCapacityPolicy(name, capacity); err != nil {
		return resource.Quantity{}, false, err
	}
	requested, hasRequest := requestedCapacity(requirements, name)
	amount, eligible := consumedCapacityForRequest(capacity, requested, hasRequest)
	if !eligible || capacityWouldBeExceeded(capacity.Value, committed[name], pending[name], amount) {
		return resource.Quantity{}, false, nil
	}
	return amount, true, nil
}

func validateCapacityUsage(
	kind string,
	usage map[resourcev1.QualifiedName]resource.Quantity,
	capacities map[resourcev1.QualifiedName]resourcev1.DeviceCapacity,
) error {
	for name, amount := range usage {
		if amount.Sign() < 0 {
			return fmt.Errorf("capacity %q has negative %s usage %s", name, kind, amount.String())
		}
		if _, found := capacities[name]; !found {
			return fmt.Errorf("capacity %q has %s usage but is not defined by the device", name, kind)
		}
	}
	return nil
}

func validateDeviceCapacityPolicy(name resourcev1.QualifiedName, capacity resourcev1.DeviceCapacity) error {
	if capacity.Value.Sign() < 0 {
		return fmt.Errorf("capacity %q has negative total quantity %s", name, capacity.Value.String())
	}
	policy := capacity.RequestPolicy
	if policy == nil {
		return nil
	}
	if len(policy.ValidValues) > 0 && policy.ValidRange != nil {
		return fmt.Errorf("capacity %q request policy sets both validValues and validRange", name)
	}
	if len(policy.ValidValues) > 0 {
		return validateCapacityValidValues(name, capacity.Value, policy)
	}
	if policy.ValidRange != nil {
		return validateCapacityValidRange(name, capacity.Value, policy)
	}
	if policy.Default != nil {
		if policy.Default.Sign() < 0 || policy.Default.Cmp(capacity.Value) > 0 {
			return fmt.Errorf("capacity %q request policy default %s is outside the device capacity", name, policy.Default.String())
		}
	}
	return nil
}

func validateCapacityValidValues(
	name resourcev1.QualifiedName,
	total resource.Quantity,
	policy *resourcev1.CapacityRequestPolicy,
) error {
	if len(policy.ValidValues) > maxCapacityPolicyValidValues {
		return fmt.Errorf("capacity %q validValues contains %d entries, exceeding the Kubernetes limit of %d", name, len(policy.ValidValues), maxCapacityPolicyValidValues)
	}
	if policy.Default == nil {
		return fmt.Errorf("capacity %q validValues policy has no default", name)
	}
	defaultFound := false
	for index, value := range policy.ValidValues {
		if value.Sign() < 0 || value.Cmp(total) > 0 {
			return fmt.Errorf("capacity %q valid value %s is outside the device capacity", name, value.String())
		}
		if index > 0 && policy.ValidValues[index-1].Cmp(value) >= 0 {
			return fmt.Errorf("capacity %q validValues are not strictly increasing", name)
		}
		if value.Cmp(*policy.Default) == 0 {
			defaultFound = true
		}
	}
	if !defaultFound {
		return fmt.Errorf("capacity %q request policy default %s is not in validValues", name, policy.Default.String())
	}
	return nil
}

func validateCapacityValidRange(
	name resourcev1.QualifiedName,
	total resource.Quantity,
	policy *resourcev1.CapacityRequestPolicy,
) error {
	validRange := policy.ValidRange
	if validRange.Min == nil {
		return fmt.Errorf("capacity %q validRange has no minimum", name)
	}
	if policy.Default == nil {
		return fmt.Errorf("capacity %q validRange policy has no default", name)
	}
	if err := validateCapacityRangeBounds(name, total, *policy.Default, validRange); err != nil {
		return err
	}
	return validateCapacityRangeStep(name, total, *policy.Default, validRange)
}

func validateCapacityRangeBounds(
	name resourcev1.QualifiedName,
	total resource.Quantity,
	defaultValue resource.Quantity,
	validRange *resourcev1.CapacityRequestPolicyRange,
) error {
	if validRange.Min.Sign() < 0 || validRange.Min.Cmp(total) > 0 {
		return fmt.Errorf("capacity %q validRange minimum %s is outside the device capacity", name, validRange.Min.String())
	}
	if validRange.Max != nil && (validRange.Max.Cmp(*validRange.Min) < 0 || validRange.Max.Cmp(total) > 0) {
		return fmt.Errorf("capacity %q validRange maximum %s is outside the allowed range", name, validRange.Max.String())
	}
	if !quantityWithinRange(defaultValue, validRange) {
		return fmt.Errorf("capacity %q request policy default %s is outside validRange", name, defaultValue.String())
	}
	return nil
}

func validateCapacityRangeStep(
	name resourcev1.QualifiedName,
	total resource.Quantity,
	defaultValue resource.Quantity,
	validRange *resourcev1.CapacityRequestPolicyRange,
) error {
	if validRange.Step == nil {
		return nil
	}
	if validRange.Step.Sign() <= 0 {
		return fmt.Errorf("capacity %q validRange step %s must be positive", name, validRange.Step.String())
	}
	minimumPlusStep := validRange.Min.DeepCopy()
	minimumPlusStep.Add(*validRange.Step)
	if minimumPlusStep.Cmp(total) > 0 {
		return fmt.Errorf("capacity %q validRange minimum plus step %s exceeds the device capacity", name, minimumPlusStep.String())
	}
	if !quantityAlignedToRange(defaultValue, validRange) {
		return fmt.Errorf("capacity %q request policy default %s is not aligned to validRange step", name, defaultValue.String())
	}
	if validRange.Max != nil && !quantityAlignedToRange(*validRange.Max, validRange) {
		return fmt.Errorf("capacity %q validRange maximum %s is not aligned to validRange step", name, validRange.Max.String())
	}
	return nil
}

func requestedCapacity(
	requirements *resourcev1.CapacityRequirements,
	name resourcev1.QualifiedName,
) (resource.Quantity, bool) {
	if requirements == nil {
		return resource.Quantity{}, false
	}
	value, found := requirements.Requests[name]
	return value, found
}

func consumedCapacityForRequest(
	capacity resourcev1.DeviceCapacity,
	requested resource.Quantity,
	hasRequest bool,
) (resource.Quantity, bool) {
	policy := capacity.RequestPolicy
	if !hasRequest {
		if policy != nil && policy.Default != nil {
			return policy.Default.DeepCopy(), true
		}
		return capacity.Value.DeepCopy(), true
	}
	if policy == nil {
		return requested.DeepCopy(), true
	}
	if len(policy.ValidValues) > 0 {
		for _, value := range policy.ValidValues {
			if requested.Cmp(value) <= 0 {
				return value.DeepCopy(), true
			}
		}
		return resource.Quantity{}, false
	}
	if policy.ValidRange != nil {
		return roundCapacityToValidRange(requested, policy.ValidRange)
	}
	return requested.DeepCopy(), true
}

func roundCapacityToValidRange(
	requested resource.Quantity,
	validRange *resourcev1.CapacityRequestPolicyRange,
) (resource.Quantity, bool) {
	if requested.Cmp(*validRange.Min) < 0 {
		return validRange.Min.DeepCopy(), true
	}
	if validRange.Step == nil {
		if !quantityWithinRange(requested, validRange) {
			return resource.Quantity{}, false
		}
		return requested.DeepCopy(), true
	}
	requestedValue := requested.Value()
	minimumValue := validRange.Min.Value()
	stepValue := validRange.Step.Value()
	steps := (requestedValue - minimumValue) / stepValue
	if (requestedValue-minimumValue)%stepValue != 0 {
		steps++
	}
	rounded := *resource.NewQuantity(minimumValue+steps*stepValue, resource.BinarySI)
	if !quantityWithinRange(rounded, validRange) {
		return resource.Quantity{}, false
	}
	return rounded, true
}

func quantityWithinRange(value resource.Quantity, validRange *resourcev1.CapacityRequestPolicyRange) bool {
	if value.Cmp(*validRange.Min) < 0 {
		return false
	}
	return validRange.Max == nil || value.Cmp(*validRange.Max) <= 0
}

func quantityAlignedToRange(value resource.Quantity, validRange *resourcev1.CapacityRequestPolicyRange) bool {
	if validRange.Step == nil {
		return true
	}
	return (value.Value()-validRange.Min.Value())%validRange.Step.Value() == 0
}

func capacityWouldBeExceeded(
	total, committed, pending, additional resource.Quantity,
) bool {
	used := committed.DeepCopy()
	used.Add(pending)
	used.Add(additional)
	return used.Cmp(total) > 0
}
