// Package simulator implements the supported stable DRA claim-constraint subset.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"strings"

	resourcev1 "k8s.io/api/resource/v1"
)

type matchAttributeConstraint struct {
	attributeName resourcev1.FullyQualifiedName
	requestNames  map[string]struct{}
}

type constrainedDevice struct {
	result resourcev1.DeviceRequestAllocationResult
	device *resourcev1.Device
}

type constrainedSelection struct {
	devices  []constrainedDevice
	nodeName string
}

type constrainedSearchState struct {
	devices  []constrainedDevice
	nodeName string
	usage    allocationAttemptUsage
}

type constrainedCandidateSet struct {
	nodeOrder []string
	byNode    map[string][]constrainedDevice
}

const maxConstraintSearchBranches = 100_000

type constraintSearchBudget struct {
	limit     int
	remaining int
}

type scalarConstraintValue struct {
	kind    byte
	string  string
	integer int64
	boolean bool
}

func newConstraintSearchBudget(limit int) *constraintSearchBudget {
	return &constraintSearchBudget{limit: limit, remaining: limit}
}

func (budget *constraintSearchBudget) consume() *allocationFailure {
	if budget.remaining <= 0 {
		return unsupportedFailure(fmt.Sprintf(
			"MatchAttribute search exceeded the deterministic limit of %d candidate branches",
			budget.limit,
		))
	}
	budget.remaining--
	return nil
}

func supportedMatchAttributeConstraints(claim *resourcev1.ResourceClaim) ([]matchAttributeConstraint, *allocationFailure) {
	if len(claim.Spec.Devices.Constraints) == 0 {
		return nil, nil
	}

	validReferences := constraintRequestReferences(claim)
	constraints := make([]matchAttributeConstraint, 0, len(claim.Spec.Devices.Constraints))
	for index, constraint := range claim.Spec.Devices.Constraints {
		if constraint.DistinctAttribute != nil {
			return nil, unsupportedFailure(fmt.Sprintf("constraint %d uses unsupported DistinctAttribute", index))
		}
		if constraint.MatchAttribute == nil {
			return nil, unsupportedFailure(fmt.Sprintf("constraint %d has no supported constraint type", index))
		}
		if !isFullyQualifiedAttributeName(*constraint.MatchAttribute) {
			return nil, unsupportedFailure(fmt.Sprintf("constraint %d uses invalid MatchAttribute %q", index, *constraint.MatchAttribute))
		}

		requestNames := make(map[string]struct{}, len(constraint.Requests))
		for _, requestName := range constraint.Requests {
			if !validReferences[requestName] {
				return nil, unsupportedFailure(fmt.Sprintf("constraint %d references unknown request %q", index, requestName))
			}
			requestNames[requestName] = struct{}{}
		}
		constraints = append(constraints, matchAttributeConstraint{
			attributeName: *constraint.MatchAttribute,
			requestNames:  requestNames,
		})
	}
	return constraints, nil
}

func constraintRequestReferences(claim *resourcev1.ResourceClaim) map[string]bool {
	references := make(map[string]bool)
	for _, request := range claim.Spec.Devices.Requests {
		references[request.Name] = true
		for _, subrequest := range request.FirstAvailable {
			references[request.Name+"/"+subrequest.Name] = true
		}
	}
	return references
}

func isFullyQualifiedAttributeName(name resourcev1.FullyQualifiedName) bool {
	domain, identifier, found := strings.Cut(string(name), "/")
	return found && domain != "" && identifier != "" && !strings.Contains(identifier, "/")
}

func (planner *claimPlanner) planConstrainedClaimAllocation(
	ctx context.Context,
	claim *resourcev1.ResourceClaim,
	constraints []matchAttributeConstraint,
) (allocationPlan, *allocationFailure) {
	state := constrainedSearchState{usage: newAllocationAttemptUsage()}
	budget := newConstraintSearchBudget(maxConstraintSearchBranches)
	plan, found, failure := planner.searchConstrainedRequests(ctx, claim, constraints, 0, state, budget)
	if failure != nil {
		return allocationPlan{}, failure
	}
	if !found {
		message := fmt.Sprintf("no supported device allocation satisfies claim %q", claim.Name)
		if len(constraints) > 0 {
			message += " and its constraints"
		}
		return allocationPlan{}, &allocationFailure{reason: reasonNoMatch, message: message}
	}
	return plan, nil
}

func (planner *claimPlanner) searchConstrainedRequests(
	ctx context.Context,
	claim *resourcev1.ResourceClaim,
	constraints []matchAttributeConstraint,
	requestIndex int,
	state constrainedSearchState,
	budget *constraintSearchBudget,
) (allocationPlan, bool, *allocationFailure) {
	if err := ctx.Err(); err != nil {
		return allocationPlan{}, false, unsupportedFailure(fmt.Sprintf("constraint search canceled: %v", err))
	}
	if requestIndex == len(claim.Spec.Devices.Requests) {
		return state.allocationPlan(), true, nil
	}

	remainingResults := resourcev1.AllocationResultsMaxSize - len(state.devices)
	var solution allocationPlan
	var found bool
	var searchFailure *allocationFailure
	_, iterationFailure := planner.forEachConstrainedRequestSelection(
		ctx,
		claim.Spec.Devices.Requests[requestIndex],
		state.nodeName,
		remainingResults,
		state.usage,
		func(selection constrainedSelection) bool {
			if failure := budget.consume(); failure != nil {
				searchFailure = failure
				return true
			}
			nextState := state.withSelection(selection)
			matches, failure := matchConstraintsSatisfied(constraints, nextState.devices)
			if failure != nil {
				searchFailure = failure
				return true
			}
			if !matches {
				return false
			}

			candidatePlan, candidateFound, failure := planner.searchConstrainedRequests(
				ctx,
				claim,
				constraints,
				requestIndex+1,
				nextState,
				budget,
			)
			if failure != nil {
				searchFailure = failure
				return true
			}
			if candidateFound {
				solution = candidatePlan
				found = true
				return true
			}
			return false
		},
	)
	if iterationFailure != nil {
		return allocationPlan{}, false, iterationFailure
	}
	if searchFailure != nil {
		return allocationPlan{}, false, searchFailure
	}
	return solution, found, nil
}

func (state constrainedSearchState) withSelection(selection constrainedSelection) constrainedSearchState {
	devices := make([]constrainedDevice, 0, len(state.devices)+len(selection.devices))
	devices = append(devices, state.devices...)
	devices = append(devices, selection.devices...)

	usage := state.usage.clone()
	for _, device := range selection.devices {
		usage.reserve(device.result)
	}

	nodeName := state.nodeName
	if nodeName == "" {
		nodeName = selection.nodeName
	}
	return constrainedSearchState{devices: devices, nodeName: nodeName, usage: usage}
}

func (state constrainedSearchState) allocationPlan() allocationPlan {
	results := make([]resourcev1.DeviceRequestAllocationResult, len(state.devices))
	for index, device := range state.devices {
		results[index] = device.result
	}
	return allocationPlan{results: results, nodeName: state.nodeName}
}

type constrainedAlternativeOutcome struct {
	missingClass        string
	resultLimitExceeded bool
}

func (planner *claimPlanner) forEachConstrainedRequestSelection(
	ctx context.Context,
	request resourcev1.DeviceRequest,
	allocatedNodeName string,
	maxResults int,
	usage allocationAttemptUsage,
	yield func(constrainedSelection) bool,
) (bool, *allocationFailure) {
	alternatives, failure := supportedAlternatives(request)
	if failure != nil {
		return false, failure
	}

	missingClasses := make([]string, 0)
	resultLimitExceeded := false
	for _, alternative := range alternatives {
		stopped, outcome, alternativeFailure := planner.forEachConstrainedAlternative(
			ctx,
			alternative,
			allocatedNodeName,
			maxResults,
			usage,
			yield,
		)
		if outcome.missingClass != "" {
			missingClasses = append(missingClasses, outcome.missingClass)
		}
		resultLimitExceeded = resultLimitExceeded || outcome.resultLimitExceeded
		if alternativeFailure != nil || stopped {
			return stopped, alternativeFailure
		}
	}
	return false, constrainedRequestIterationFailure(
		request.Name,
		len(alternatives),
		missingClasses,
		resultLimitExceeded,
		maxResults,
	)
}

func (planner *claimPlanner) forEachConstrainedAlternative(
	ctx context.Context,
	alternative requestAlternative,
	allocatedNodeName string,
	maxResults int,
	usage allocationAttemptUsage,
	yield func(constrainedSelection) bool,
) (bool, constrainedAlternativeOutcome, *allocationFailure) {
	deviceClass, found := planner.classes[alternative.deviceClassName]
	if !found {
		return false, constrainedAlternativeOutcome{missingClass: alternative.deviceClassName}, nil
	}
	if alternative.mode == resourcev1.DeviceAllocationModeExactCount && alternative.count > maxResults {
		return false, constrainedAlternativeOutcome{resultLimitExceeded: true}, nil
	}

	input := selectionInput{
		alternative:       alternative,
		deviceClass:       deviceClass,
		allocatedNodeName: allocatedNodeName,
		maxResults:        maxResults,
	}
	if alternative.mode == resourcev1.DeviceAllocationModeAll {
		stopped, limitExceeded, failure := planner.forEachConstrainedAllSelection(ctx, input, usage, yield)
		return stopped, constrainedAlternativeOutcome{resultLimitExceeded: limitExceeded}, failure
	}
	stopped, failure := planner.forEachConstrainedExactSelection(ctx, input, usage, yield)
	return stopped, constrainedAlternativeOutcome{}, failure
}

func constrainedRequestIterationFailure(
	requestName string,
	alternativeCount int,
	missingClasses []string,
	resultLimitExceeded bool,
	maxResults int,
) *allocationFailure {
	if len(missingClasses) == alternativeCount && len(missingClasses) > 0 {
		return &allocationFailure{
			reason:  reasonDeviceClassNotFound,
			message: fmt.Sprintf("request %q references unavailable DeviceClass values: %v", requestName, missingClasses),
		}
	}
	if resultLimitExceeded && alternativeCount == 1 {
		return unsupportedFailure(fmt.Sprintf(
			"request %q cannot fit within the remaining Kubernetes allocation result budget of %d",
			requestName,
			maxResults,
		))
	}
	return nil
}

func (planner *claimPlanner) forEachConstrainedExactSelection(
	ctx context.Context,
	input selectionInput,
	usage allocationAttemptUsage,
	yield func(constrainedSelection) bool,
) (bool, *allocationFailure) {
	candidates, failure := planner.collectConstrainedExactCandidates(ctx, input, usage)
	if failure != nil {
		return false, failure
	}
	for _, nodeName := range candidates.nodeOrder {
		devices := candidates.byNode[nodeName]
		if len(devices) < input.alternative.count {
			continue
		}
		stopped, failure := forEachDeviceCombination(ctx, devices, input.alternative.count, func(selected []constrainedDevice) bool {
			return yield(constrainedSelection{devices: selected, nodeName: nodeName})
		})
		if failure != nil || stopped {
			return stopped, failure
		}
	}
	return false, nil
}

func forEachDeviceCombination(
	ctx context.Context,
	candidates []constrainedDevice,
	count int,
	yield func([]constrainedDevice) bool,
) (bool, *allocationFailure) {
	selected := make([]constrainedDevice, 0, count)
	var visit func(start int) (bool, *allocationFailure)
	visit = func(start int) (bool, *allocationFailure) {
		if err := ctx.Err(); err != nil {
			return false, unsupportedFailure(fmt.Sprintf("constraint search canceled: %v", err))
		}
		if len(selected) == count {
			combination := append([]constrainedDevice(nil), selected...)
			return yield(combination), nil
		}
		remaining := count - len(selected)
		for index := start; index <= len(candidates)-remaining; index++ {
			selected = append(selected, candidates[index])
			stopped, failure := visit(index + 1)
			selected = selected[:len(selected)-1]
			if failure != nil || stopped {
				return stopped, failure
			}
		}
		return false, nil
	}
	return visit(0)
}

func (planner *claimPlanner) collectConstrainedExactCandidates(
	ctx context.Context,
	input selectionInput,
	usage allocationAttemptUsage,
) (constrainedCandidateSet, *allocationFailure) {
	set := constrainedCandidateSet{byNode: make(map[string][]constrainedDevice)}
	seenNodes := make(map[string]bool)
	seenDevices := make(map[string]bool)
	for sliceIndex := range planner.slices {
		slice := &planner.slices[sliceIndex]
		nodeName, eligible := constrainedSliceNode(slice, input.allocatedNodeName)
		if !eligible {
			continue
		}
		devices, failure := planner.collectConstrainedSliceDevices(ctx, input, slice, usage, seenDevices)
		if failure != nil {
			return constrainedCandidateSet{}, failure
		}
		if len(devices) == 0 {
			continue
		}
		if !seenNodes[nodeName] {
			seenNodes[nodeName] = true
			set.nodeOrder = append(set.nodeOrder, nodeName)
		}
		set.byNode[nodeName] = append(set.byNode[nodeName], devices...)
	}
	return set, nil
}

func constrainedSliceNode(slice *resourcev1.ResourceSlice, allocatedNodeName string) (string, bool) {
	if !eligibleSimulatorSlice(slice) {
		return "", false
	}
	return compatibleNodeName(slice, allocatedNodeName)
}

func (planner *claimPlanner) collectConstrainedSliceDevices(
	ctx context.Context,
	input selectionInput,
	slice *resourcev1.ResourceSlice,
	usage allocationAttemptUsage,
	seenDevices map[string]bool,
) ([]constrainedDevice, *allocationFailure) {
	devices := make([]constrainedDevice, 0, len(slice.Spec.Devices))
	for deviceIndex := range slice.Spec.Devices {
		candidate, accepted, failure := planner.evaluateConstrainedCandidate(
			ctx,
			input,
			slice,
			&slice.Spec.Devices[deviceIndex],
			usage,
			seenDevices,
		)
		if failure != nil {
			return nil, failure
		}
		if accepted {
			devices = append(devices, candidate)
		}
	}
	return devices, nil
}

func (planner *claimPlanner) forEachConstrainedAllSelection(
	ctx context.Context,
	input selectionInput,
	usage allocationAttemptUsage,
	yield func(constrainedSelection) bool,
) (bool, bool, *allocationFailure) {
	nodeOrder, slicesByNode := planner.managedSlicesByNode(input.allocatedNodeName)
	var deferredFailure *allocationFailure
	resultLimitExceeded := false
	for _, nodeName := range nodeOrder {
		devices, failure := planner.collectConstrainedAllDevicesForNode(
			ctx,
			input,
			usage,
			slicesByNode[nodeName],
		)
		if failure != nil {
			deferredFailure = failure
			continue
		}
		if len(devices) == 0 {
			continue
		}
		if len(devices) > input.maxResults {
			resultLimitExceeded = true
			continue
		}
		if yield(constrainedSelection{devices: devices, nodeName: nodeName}) {
			return true, resultLimitExceeded, nil
		}
	}
	return constrainedAllIterationResult(resultLimitExceeded, deferredFailure)
}

func (planner *claimPlanner) collectConstrainedAllDevicesForNode(
	ctx context.Context,
	input selectionInput,
	usage allocationAttemptUsage,
	slices []*resourcev1.ResourceSlice,
) ([]constrainedDevice, *allocationFailure) {
	currentSlices, failure := completeCurrentPoolSlices(slices)
	if failure != nil {
		return nil, failure
	}

	seenDevices := make(map[string]bool)
	devices := make([]constrainedDevice, 0)
	for _, slice := range currentSlices {
		if slice.Labels["draforge.oaslananka/health"] == "unhealthy" {
			continue
		}
		sliceDevices, sliceFailure := planner.collectConstrainedSliceDevices(
			ctx,
			input,
			slice,
			usage,
			seenDevices,
		)
		if sliceFailure != nil {
			return nil, sliceFailure
		}
		devices = append(devices, sliceDevices...)
	}
	return devices, nil
}

func constrainedAllIterationResult(
	resultLimitExceeded bool,
	deferredFailure *allocationFailure,
) (bool, bool, *allocationFailure) {
	if deferredFailure != nil && !resultLimitExceeded {
		return false, false, deferredFailure
	}
	return false, resultLimitExceeded, nil
}

func (planner *claimPlanner) evaluateConstrainedCandidate(
	ctx context.Context,
	input selectionInput,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	usage allocationAttemptUsage,
	seen map[string]bool,
) (constrainedDevice, bool, *allocationFailure) {
	matched, err := planner.evaluator.Matches(
		ctx,
		slice.Spec.Driver,
		*device,
		input.deviceClass.Spec.Selectors,
		input.alternative.selectors,
	)
	if err != nil {
		return constrainedDevice{}, false, selectorFailure(input.alternative.requestName, err)
	}
	if !matched {
		return constrainedDevice{}, false, nil
	}
	if failure := unsupportedSliceFailure(slice); failure != nil {
		return constrainedDevice{}, false, failure
	}
	if failure := unsupportedDeviceFailure(device); failure != nil {
		return constrainedDevice{}, false, failure
	}

	result, accepted, failure := planner.buildBacktrackingCandidate(
		input.alternative.requestName,
		input.alternative.capacity,
		slice,
		device,
		usage,
		seen,
	)
	if failure != nil || !accepted {
		return constrainedDevice{}, accepted, failure
	}
	return constrainedDevice{result: result, device: device}, true, nil
}

func matchConstraintsSatisfied(
	constraints []matchAttributeConstraint,
	devices []constrainedDevice,
) (bool, *allocationFailure) {
	for _, constraint := range constraints {
		matches, failure := matchConstraintSatisfied(constraint, devices)
		if failure != nil || !matches {
			return matches, failure
		}
	}
	return true, nil
}

func matchConstraintSatisfied(
	constraint matchAttributeConstraint,
	devices []constrainedDevice,
) (bool, *allocationFailure) {
	var expected scalarConstraintValue
	hasExpected := false
	for _, device := range devices {
		if !constraint.appliesTo(device.result.Request) {
			continue
		}
		value, supported, failure := constrainedDeviceAttributeValue(device, constraint.attributeName)
		if failure != nil || !supported {
			return supported, failure
		}
		if !hasExpected {
			expected = value
			hasExpected = true
			continue
		}
		if value != expected {
			return false, nil
		}
	}
	return true, nil
}

func constrainedDeviceAttributeValue(
	device constrainedDevice,
	attributeName resourcev1.FullyQualifiedName,
) (scalarConstraintValue, bool, *allocationFailure) {
	attribute := lookupConstraintAttribute(device, attributeName)
	if attribute == nil {
		return scalarConstraintValue{}, false, nil
	}
	return scalarConstraintAttribute(*attribute)
}

func (constraint matchAttributeConstraint) appliesTo(resultRequest string) bool {
	if len(constraint.requestNames) == 0 {
		return true
	}
	if _, found := constraint.requestNames[resultRequest]; found {
		return true
	}
	parent, _, hasSubrequest := strings.Cut(resultRequest, "/")
	if !hasSubrequest {
		return false
	}
	_, found := constraint.requestNames[parent]
	return found
}

func lookupConstraintAttribute(
	device constrainedDevice,
	attributeName resourcev1.FullyQualifiedName,
) *resourcev1.DeviceAttribute {
	if attribute, found := device.device.Attributes[resourcev1.QualifiedName(attributeName)]; found {
		return &attribute
	}
	domain, identifier, found := strings.Cut(string(attributeName), "/")
	if !found || domain != device.result.Driver {
		return nil
	}
	if attribute, found := device.device.Attributes[resourcev1.QualifiedName(identifier)]; found {
		return &attribute
	}
	return nil
}

func scalarConstraintAttribute(attribute resourcev1.DeviceAttribute) (scalarConstraintValue, bool, *allocationFailure) {
	if len(attribute.IntValues) > 0 || len(attribute.BoolValues) > 0 || len(attribute.StringValues) > 0 || len(attribute.VersionValues) > 0 {
		return scalarConstraintValue{}, false, unsupportedFailure("MatchAttribute list-valued attributes are not supported")
	}

	setFields := 0
	value := scalarConstraintValue{}
	if attribute.StringValue != nil {
		setFields++
		value = scalarConstraintValue{kind: 's', string: *attribute.StringValue}
	}
	if attribute.IntValue != nil {
		setFields++
		value = scalarConstraintValue{kind: 'i', integer: *attribute.IntValue}
	}
	if attribute.BoolValue != nil {
		setFields++
		value = scalarConstraintValue{kind: 'b', boolean: *attribute.BoolValue}
	}
	if attribute.VersionValue != nil {
		setFields++
		value = scalarConstraintValue{kind: 'v', string: *attribute.VersionValue}
	}
	if setFields > 1 {
		return scalarConstraintValue{}, false, unsupportedFailure("MatchAttribute encountered an invalid multi-valued scalar attribute")
	}
	return value, setFields == 1, nil
}
