// Package simulator plans synthetic allocations against the supported Kubernetes DRA subset.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"

	"github.com/oaslananka/draforge/internal/draeval"
	resourcev1 "k8s.io/api/resource/v1"
)

const (
	reasonDeviceClassNotFound = "SimulationDeviceClassNotFound"
	reasonNoMatch             = "SimulationNoMatch"
	reasonSelectorError       = "SimulationSelectorError"
	reasonUnsupportedRequest  = "SimulationUnsupportedRequest"
)

type allocationFailure struct {
	reason  string
	message string
}

type allocationPlan struct {
	results  []resourcev1.DeviceRequestAllocationResult
	nodeName string
}

type requestAlternative struct {
	requestName     string
	deviceClassName string
	selectors       []resourcev1.DeviceSelector
	mode            resourcev1.DeviceAllocationMode
	count           int
}

type alternativeContract struct {
	requestName     string
	deviceClassName string
	selectors       []resourcev1.DeviceSelector
	mode            resourcev1.DeviceAllocationMode
	count           int64
	adminAccess     *bool
	tolerations     []resourcev1.DeviceToleration
	capacity        *resourcev1.CapacityRequirements
}

type requestSelection struct {
	results             []resourcev1.DeviceRequestAllocationResult
	nodeName            string
	matched             bool
	resultLimitExceeded bool
}

type selectionInput struct {
	alternative       requestAlternative
	deviceClass       *resourcev1.DeviceClass
	allocatedNodeName string
	maxResults        int
}

type claimPlanner struct {
	reconciler    *Reconciler
	claims        []resourcev1.ResourceClaim
	slices        []resourcev1.ResourceSlice
	classes       map[string]*resourcev1.DeviceClass
	evaluator     *draeval.Evaluator
	chosenDevices map[string]bool
}

type candidateAccumulator struct {
	count         int
	resultsByNode map[string][]resourcev1.DeviceRequestAllocationResult
	seen          map[string]bool
}

type poolGeneration struct {
	driver        string
	pool          string
	generation    int64
	expectedCount int64
	slices        []*resourcev1.ResourceSlice
}

func (r *Reconciler) planClaimAllocation(
	ctx context.Context,
	claim *resourcev1.ResourceClaim,
	claims []resourcev1.ResourceClaim,
	slices []resourcev1.ResourceSlice,
	classes map[string]*resourcev1.DeviceClass,
) (allocationPlan, *allocationFailure) {
	if failure := validateSupportedClaim(claim); failure != nil {
		return allocationPlan{}, failure
	}

	planner := newClaimPlanner(r, claims, slices, classes)
	plan := allocationPlan{}
	for _, request := range claim.Spec.Devices.Requests {
		remainingResults := resourcev1.AllocationResultsMaxSize - len(plan.results)
		selection, failure := planner.selectRequest(ctx, request, plan.nodeName, remainingResults)
		if failure != nil {
			return allocationPlan{}, failure
		}
		if failure := validateClaimResultLimit(len(plan.results), len(selection.results)); failure != nil {
			return allocationPlan{}, failure
		}
		planner.reserve(selection.results)
		plan.append(selection)
	}
	return plan, nil
}

func validateSupportedClaim(claim *resourcev1.ResourceClaim) *allocationFailure {
	if len(claim.Spec.Devices.Constraints) > 0 {
		return unsupportedFailure("claim-level device constraints are not supported")
	}
	return nil
}

func validateClaimResultLimit(existing, additional int) *allocationFailure {
	total := existing + additional
	if total <= resourcev1.AllocationResultsMaxSize {
		return nil
	}
	return unsupportedFailure(fmt.Sprintf(
		"claim requires %d allocation results, exceeding the Kubernetes limit of %d",
		total,
		resourcev1.AllocationResultsMaxSize,
	))
}

func newClaimPlanner(
	reconciler *Reconciler,
	claims []resourcev1.ResourceClaim,
	slices []resourcev1.ResourceSlice,
	classes map[string]*resourcev1.DeviceClass,
) *claimPlanner {
	evaluator := reconciler.selectorEvaluator
	if evaluator == nil {
		evaluator = draeval.NewEvaluator()
	}
	return &claimPlanner{
		reconciler:    reconciler,
		claims:        claims,
		slices:        slices,
		classes:       classes,
		evaluator:     evaluator,
		chosenDevices: make(map[string]bool),
	}
}

func (plan *allocationPlan) append(selection requestSelection) {
	plan.results = append(plan.results, selection.results...)
	if plan.nodeName == "" {
		plan.nodeName = selection.nodeName
	}
}

func (planner *claimPlanner) reserve(results []resourcev1.DeviceRequestAllocationResult) {
	for _, result := range results {
		planner.chosenDevices[deviceIdentityKey(result.Driver, result.Pool, result.Device)] = true
	}
}

func (planner *claimPlanner) selectRequest(
	ctx context.Context,
	request resourcev1.DeviceRequest,
	allocatedNodeName string,
	maxResults int,
) (requestSelection, *allocationFailure) {
	alternatives, failure := supportedAlternatives(request)
	if failure != nil {
		return requestSelection{}, failure
	}

	missingClasses := make([]string, 0)
	resultLimitExceeded := false
	for _, alternative := range alternatives {
		deviceClass, found := planner.classes[alternative.deviceClassName]
		if !found {
			missingClasses = append(missingClasses, alternative.deviceClassName)
			continue
		}
		if alternative.mode == resourcev1.DeviceAllocationModeExactCount && alternative.count > maxResults {
			resultLimitExceeded = true
			continue
		}

		selection, selectionFailure := planner.selectAlternative(ctx, selectionInput{
			alternative:       alternative,
			deviceClass:       deviceClass,
			allocatedNodeName: allocatedNodeName,
			maxResults:        maxResults,
		})
		if selectionFailure != nil {
			return requestSelection{}, selectionFailure
		}
		if selection.resultLimitExceeded {
			resultLimitExceeded = true
			continue
		}
		if selection.matched {
			return selection, nil
		}
	}

	if resultLimitExceeded && len(alternatives) == 1 {
		return requestSelection{}, unsupportedFailure(fmt.Sprintf(
			"request %q cannot fit within the remaining Kubernetes allocation result budget of %d",
			request.Name,
			maxResults,
		))
	}
	return requestSelection{}, unmatchedRequestFailure(request.Name, alternatives, missingClasses)
}

func unmatchedRequestFailure(
	requestName string,
	alternatives []requestAlternative,
	missingClasses []string,
) *allocationFailure {
	if len(missingClasses) == len(alternatives) && len(missingClasses) > 0 {
		return &allocationFailure{
			reason:  reasonDeviceClassNotFound,
			message: fmt.Sprintf("request %q references unavailable DeviceClass values: %v", requestName, missingClasses),
		}
	}
	return &allocationFailure{
		reason:  reasonNoMatch,
		message: fmt.Sprintf("no supported healthy device set satisfies request %q", requestName),
	}
}

func supportedAlternatives(request resourcev1.DeviceRequest) ([]requestAlternative, *allocationFailure) {
	switch {
	case request.Exactly != nil && len(request.FirstAvailable) == 0:
		alternative, failure := supportedAlternative(exactAlternativeContract(request))
		if failure != nil {
			return nil, failure
		}
		return []requestAlternative{alternative}, nil
	case request.Exactly == nil && len(request.FirstAvailable) > 0:
		return supportedSubrequests(request)
	default:
		return nil, unsupportedFailure(fmt.Sprintf("request %q must set exactly one of Exactly or FirstAvailable", request.Name))
	}
}

func exactAlternativeContract(request resourcev1.DeviceRequest) alternativeContract {
	exact := request.Exactly
	return alternativeContract{
		requestName:     request.Name,
		deviceClassName: exact.DeviceClassName,
		selectors:       exact.Selectors,
		mode:            exact.AllocationMode,
		count:           exact.Count,
		adminAccess:     exact.AdminAccess,
		tolerations:     exact.Tolerations,
		capacity:        exact.Capacity,
	}
}

func supportedSubrequests(request resourcev1.DeviceRequest) ([]requestAlternative, *allocationFailure) {
	alternatives := make([]requestAlternative, 0, len(request.FirstAvailable))
	for _, subrequest := range request.FirstAvailable {
		alternative, failure := supportedAlternative(subrequestContract(request.Name, subrequest))
		if failure != nil {
			return nil, failure
		}
		alternatives = append(alternatives, alternative)
	}
	return alternatives, nil
}

func subrequestContract(parentName string, subrequest resourcev1.DeviceSubRequest) alternativeContract {
	return alternativeContract{
		requestName:     fmt.Sprintf("%s/%s", parentName, subrequest.Name),
		deviceClassName: subrequest.DeviceClassName,
		selectors:       subrequest.Selectors,
		mode:            subrequest.AllocationMode,
		count:           subrequest.Count,
		tolerations:     subrequest.Tolerations,
		capacity:        subrequest.Capacity,
	}
}

func supportedAlternative(contract alternativeContract) (requestAlternative, *allocationFailure) {
	if contract.deviceClassName == "" {
		return requestAlternative{}, unsupportedFailure(fmt.Sprintf("request %q has no DeviceClassName", contract.requestName))
	}
	mode, failure := supportedAllocationMode(contract)
	if failure != nil {
		return requestAlternative{}, failure
	}
	if contract.adminAccess != nil && *contract.adminAccess {
		return requestAlternative{}, unsupportedFailure(fmt.Sprintf("request %q uses admin access", contract.requestName))
	}
	if len(contract.tolerations) > 0 {
		return requestAlternative{}, unsupportedFailure(fmt.Sprintf("request %q uses device tolerations", contract.requestName))
	}
	if contract.capacity != nil {
		return requestAlternative{}, unsupportedFailure(fmt.Sprintf("request %q uses consumable capacity requirements", contract.requestName))
	}

	return requestAlternative{
		requestName:     contract.requestName,
		deviceClassName: contract.deviceClassName,
		selectors:       contract.selectors,
		mode:            mode,
		count:           normalizedAlternativeCount(mode, contract.count),
	}, nil
}

func supportedAllocationMode(contract alternativeContract) (resourcev1.DeviceAllocationMode, *allocationFailure) {
	mode := contract.mode
	if mode == "" {
		mode = resourcev1.DeviceAllocationModeExactCount
	}
	switch mode {
	case resourcev1.DeviceAllocationModeExactCount:
		if contract.count < 0 {
			return "", unsupportedFailure(fmt.Sprintf("request %q has invalid negative count %d", contract.requestName, contract.count))
		}
		if contract.count > int64(resourcev1.AllocationResultsMaxSize) {
			return "", unsupportedFailure(fmt.Sprintf("request %q count %d exceeds Kubernetes allocation result limit %d", contract.requestName, contract.count, resourcev1.AllocationResultsMaxSize))
		}
	case resourcev1.DeviceAllocationModeAll:
		if contract.count != 0 {
			return "", unsupportedFailure(fmt.Sprintf("request %q uses All allocation mode with nonzero count %d", contract.requestName, contract.count))
		}
	default:
		return "", unsupportedFailure(fmt.Sprintf("request %q uses unsupported allocation mode %q", contract.requestName, mode))
	}
	return mode, nil
}

func normalizedAlternativeCount(mode resourcev1.DeviceAllocationMode, count int64) int {
	if mode == resourcev1.DeviceAllocationModeAll {
		return 0
	}
	return normalizedCount(count)
}

func normalizedCount(count int64) int {
	if count == 0 {
		return 1
	}
	return int(count)
}

func (planner *claimPlanner) selectAlternative(ctx context.Context, input selectionInput) (requestSelection, *allocationFailure) {
	if input.alternative.mode == resourcev1.DeviceAllocationModeAll {
		return planner.selectAllAlternative(ctx, input)
	}
	return planner.selectExactCountAlternative(ctx, input)
}

func (planner *claimPlanner) selectExactCountAlternative(ctx context.Context, input selectionInput) (requestSelection, *allocationFailure) {
	candidates := newCandidateAccumulator(input.alternative.count)
	for sliceIndex := range planner.slices {
		selection, failure := planner.selectFromSlice(ctx, input, &planner.slices[sliceIndex], candidates)
		if failure != nil {
			return requestSelection{}, failure
		}
		if selection.matched {
			return selection, nil
		}
	}
	return requestSelection{}, nil
}

func (planner *claimPlanner) selectAllAlternative(ctx context.Context, input selectionInput) (requestSelection, *allocationFailure) {
	nodeOrder, slicesByNode := planner.managedSlicesByNode(input.allocatedNodeName)
	var deferredFailure *allocationFailure
	resultLimitExceeded := false
	for _, nodeName := range nodeOrder {
		currentSlices, failure := completeCurrentPoolSlices(slicesByNode[nodeName])
		if failure != nil {
			deferredFailure = failure
			continue
		}
		selection, selectionFailure := planner.selectAllFromNode(ctx, input, nodeName, currentSlices)
		if selectionFailure != nil {
			return requestSelection{}, selectionFailure
		}
		if selection.matched && len(selection.results) <= input.maxResults {
			return selection, nil
		}
		if selection.matched {
			resultLimitExceeded = true
		}
	}
	if resultLimitExceeded {
		return requestSelection{resultLimitExceeded: true}, nil
	}
	if deferredFailure != nil {
		return requestSelection{}, deferredFailure
	}
	return requestSelection{}, nil
}

func (planner *claimPlanner) managedSlicesByNode(allocatedNodeName string) ([]string, map[string][]*resourcev1.ResourceSlice) {
	nodeOrder := make([]string, 0)
	slicesByNode := make(map[string][]*resourcev1.ResourceSlice)
	seenNodes := make(map[string]bool)
	for index := range planner.slices {
		slice := &planner.slices[index]
		if !managedSimulatorSlice(slice) {
			continue
		}
		nodeName, compatible := compatibleNodeName(slice, allocatedNodeName)
		if !compatible {
			continue
		}
		if !seenNodes[nodeName] {
			seenNodes[nodeName] = true
			nodeOrder = append(nodeOrder, nodeName)
		}
		slicesByNode[nodeName] = append(slicesByNode[nodeName], slice)
	}
	return nodeOrder, slicesByNode
}

func completeCurrentPoolSlices(slices []*resourcev1.ResourceSlice) ([]*resourcev1.ResourceSlice, *allocationFailure) {
	poolOrder := make([]string, 0)
	pools := make(map[string]*poolGeneration)
	for _, slice := range slices {
		key := slice.Spec.Driver + "\x00" + slice.Spec.Pool.Name
		pool := pools[key]
		if pool == nil {
			pool = &poolGeneration{driver: slice.Spec.Driver, pool: slice.Spec.Pool.Name, generation: slice.Spec.Pool.Generation}
			pools[key] = pool
			poolOrder = append(poolOrder, key)
		}
		if slice.Spec.Pool.Generation > pool.generation {
			pool.generation = slice.Spec.Pool.Generation
			pool.expectedCount = 0
			pool.slices = nil
		}
		if slice.Spec.Pool.Generation != pool.generation {
			continue
		}
		if pool.expectedCount == 0 {
			pool.expectedCount = slice.Spec.Pool.ResourceSliceCount
		}
		if slice.Spec.Pool.ResourceSliceCount != pool.expectedCount {
			return nil, unsupportedFailure(fmt.Sprintf("resource pool %s/%s generation %d reports inconsistent slice counts", pool.driver, pool.pool, pool.generation))
		}
		pool.slices = append(pool.slices, slice)
	}

	current := make([]*resourcev1.ResourceSlice, 0, len(slices))
	for _, key := range poolOrder {
		pool := pools[key]
		if pool.expectedCount <= 0 || int64(len(pool.slices)) != pool.expectedCount {
			return nil, unsupportedFailure(fmt.Sprintf("resource pool %s/%s generation %d is incomplete: observed %d of %d slices", pool.driver, pool.pool, pool.generation, len(pool.slices), pool.expectedCount))
		}
		current = append(current, pool.slices...)
	}
	return current, nil
}

func (planner *claimPlanner) selectAllFromNode(
	ctx context.Context,
	input selectionInput,
	nodeName string,
	slices []*resourcev1.ResourceSlice,
) (requestSelection, *allocationFailure) {
	candidates := newCandidateAccumulator(0)
	results := make([]resourcev1.DeviceRequestAllocationResult, 0)
	for _, slice := range slices {
		if slice.Labels["draforge.oaslananka/health"] == "unhealthy" {
			continue
		}
		for deviceIndex := range slice.Spec.Devices {
			result, accepted, failure := planner.evaluateCandidate(ctx, input, slice, &slice.Spec.Devices[deviceIndex], candidates)
			if failure != nil {
				return requestSelection{}, failure
			}
			if accepted {
				results = append(results, result)
			}
		}
	}
	if len(results) == 0 {
		return requestSelection{}, nil
	}
	return requestSelection{results: results, nodeName: nodeName, matched: true}, nil
}

func newCandidateAccumulator(count int) *candidateAccumulator {
	return &candidateAccumulator{
		count:         count,
		resultsByNode: make(map[string][]resourcev1.DeviceRequestAllocationResult),
		seen:          make(map[string]bool),
	}
}

func (planner *claimPlanner) selectFromSlice(
	ctx context.Context,
	input selectionInput,
	slice *resourcev1.ResourceSlice,
	candidates *candidateAccumulator,
) (requestSelection, *allocationFailure) {
	if !eligibleSimulatorSlice(slice) {
		return requestSelection{}, nil
	}

	nodeName, compatible := compatibleNodeName(slice, input.allocatedNodeName)
	if !compatible {
		return requestSelection{}, nil
	}

	for deviceIndex := range slice.Spec.Devices {
		result, accepted, failure := planner.evaluateCandidate(ctx, input, slice, &slice.Spec.Devices[deviceIndex], candidates)
		if failure != nil {
			return requestSelection{}, failure
		}
		if !accepted {
			continue
		}
		results, complete := candidates.add(nodeName, result)
		if complete {
			return requestSelection{results: results, nodeName: nodeName, matched: true}, nil
		}
	}
	return requestSelection{}, nil
}

func managedSimulatorSlice(slice *resourcev1.ResourceSlice) bool {
	return slice.Labels["draforge.oaslananka/managed-by"] == "simulator"
}

func eligibleSimulatorSlice(slice *resourcev1.ResourceSlice) bool {
	return managedSimulatorSlice(slice) && slice.Labels["draforge.oaslananka/health"] != "unhealthy"
}

func compatibleNodeName(slice *resourcev1.ResourceSlice, allocatedNodeName string) (string, bool) {
	nodeName := ""
	if slice.Spec.NodeName != nil {
		nodeName = *slice.Spec.NodeName
	}
	if allocatedNodeName != "" && nodeName != "" && nodeName != allocatedNodeName {
		return "", false
	}
	if nodeName == "" && allocatedNodeName != "" {
		return allocatedNodeName, true
	}
	return nodeName, true
}

func (planner *claimPlanner) evaluateCandidate(
	ctx context.Context,
	input selectionInput,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	candidates *candidateAccumulator,
) (resourcev1.DeviceRequestAllocationResult, bool, *allocationFailure) {
	matched, err := planner.evaluator.Matches(
		ctx,
		slice.Spec.Driver,
		*device,
		input.deviceClass.Spec.Selectors,
		input.alternative.selectors,
	)
	if err != nil {
		return resourcev1.DeviceRequestAllocationResult{}, false, selectorFailure(input.alternative.requestName, err)
	}
	if !matched {
		return resourcev1.DeviceRequestAllocationResult{}, false, nil
	}
	if failure := unsupportedSliceFailure(slice); failure != nil {
		return resourcev1.DeviceRequestAllocationResult{}, false, failure
	}
	if failure := unsupportedDeviceFailure(device); failure != nil {
		return resourcev1.DeviceRequestAllocationResult{}, false, failure
	}

	identity := deviceIdentityKey(slice.Spec.Driver, slice.Spec.Pool.Name, device.Name)
	if planner.candidateUnavailable(identity, slice, device, candidates) {
		return resourcev1.DeviceRequestAllocationResult{}, false, nil
	}
	return resourcev1.DeviceRequestAllocationResult{
		Request: input.alternative.requestName,
		Driver:  slice.Spec.Driver,
		Pool:    slice.Spec.Pool.Name,
		Device:  device.Name,
	}, true, nil
}

func selectorFailure(requestName string, err error) *allocationFailure {
	return &allocationFailure{
		reason:  reasonSelectorError,
		message: fmt.Sprintf("request %q selector evaluation failed: %v", requestName, err),
	}
}

func (planner *claimPlanner) candidateUnavailable(
	identity string,
	slice *resourcev1.ResourceSlice,
	device *resourcev1.Device,
	candidates *candidateAccumulator,
) bool {
	if candidates.seen[identity] || planner.chosenDevices[identity] {
		return true
	}
	if planner.reconciler.isDeviceAllocated(planner.claims, slice.Spec.Driver, slice.Spec.Pool.Name, device.Name) {
		return true
	}
	candidates.seen[identity] = true
	return false
}

func (candidates *candidateAccumulator) add(
	nodeName string,
	result resourcev1.DeviceRequestAllocationResult,
) ([]resourcev1.DeviceRequestAllocationResult, bool) {
	candidates.resultsByNode[nodeName] = append(candidates.resultsByNode[nodeName], result)
	results := candidates.resultsByNode[nodeName]
	return results, len(results) == candidates.count
}

func unsupportedSliceFailure(slice *resourcev1.ResourceSlice) *allocationFailure {
	if slice.Spec.NodeName == nil || slice.Spec.NodeSelector != nil || slice.Spec.AllNodes != nil || slice.Spec.PerDeviceNodeSelection != nil || len(slice.Spec.SharedCounters) > 0 {
		return unsupportedFailure(fmt.Sprintf("ResourceSlice %q does not use the supported single NodeName selection", slice.Name))
	}
	return nil
}

func unsupportedDeviceFailure(device *resourcev1.Device) *allocationFailure {
	if len(device.ConsumesCounters) > 0 || device.NodeName != nil || device.NodeSelector != nil || device.AllNodes != nil {
		return unsupportedFailure(fmt.Sprintf("device %q uses unsupported partitionable-device fields", device.Name))
	}
	if len(device.Taints) > 0 {
		return unsupportedFailure(fmt.Sprintf("device %q uses unsupported taints", device.Name))
	}
	if device.BindsToNode != nil || len(device.BindingConditions) > 0 || len(device.BindingFailureConditions) > 0 {
		return unsupportedFailure(fmt.Sprintf("device %q uses unsupported binding-condition fields", device.Name))
	}
	if device.AllowMultipleAllocations != nil && *device.AllowMultipleAllocations {
		return unsupportedFailure(fmt.Sprintf("device %q allows multiple allocations", device.Name))
	}
	if len(device.NodeAllocatableResourceMappings) > 0 {
		return unsupportedFailure(fmt.Sprintf("device %q uses node-allocatable resource mappings", device.Name))
	}
	return nil
}

func unsupportedFailure(message string) *allocationFailure {
	return &allocationFailure{reason: reasonUnsupportedRequest, message: message}
}

func deviceIdentityKey(driverName, poolName, deviceName string) string {
	return driverName + "\x00" + poolName + "\x00" + deviceName
}
