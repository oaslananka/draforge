// Package draeval provides the supported Kubernetes DRA selector semantics shared by DRAForge components.
// SPDX-License-Identifier: Apache-2.0
package draeval

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	dracel "k8s.io/dynamic-resource-allocation/cel"
)

const selectorCacheEntries = 256

// Evaluator compiles and evaluates Kubernetes DRA CEL selectors.
type Evaluator struct {
	cache *dracel.Cache
}

// NewEvaluator creates an evaluator for the currently supported DRA feature subset.
func NewEvaluator() *Evaluator {
	return &Evaluator{
		cache: dracel.NewCache(selectorCacheEntries, dracel.Features{}),
	}
}

// Matches returns true only when every selector in every selector set matches the device.
func (e *Evaluator) Matches(ctx context.Context, driver string, device resourcev1.Device, selectorSets ...[]resourcev1.DeviceSelector) (bool, error) {
	input := dracel.Device{
		Driver:                   driver,
		AllowMultipleAllocations: device.AllowMultipleAllocations,
		Attributes:               device.Attributes,
		Capacity:                 device.Capacity,
	}

	for setIndex, selectors := range selectorSets {
		for selectorIndex, selector := range selectors {
			if selector.CEL == nil {
				return false, fmt.Errorf("unsupported selector at set %d index %d: CEL selector is required", setIndex, selectorIndex)
			}
			compiled := e.cache.GetOrCompile(selector.CEL.Expression)
			if compiled.Error != nil {
				return false, fmt.Errorf("compile selector at set %d index %d: %w", setIndex, selectorIndex, compiled.Error)
			}
			matched, _, err := compiled.DeviceMatches(ctx, input)
			if err != nil {
				return false, fmt.Errorf("evaluate selector at set %d index %d: %w", setIndex, selectorIndex, err)
			}
			if !matched {
				return false, nil
			}
		}
	}
	return true, nil
}
