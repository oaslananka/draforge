// Package e2echeck verifies go test -json output for remote E2E runs.
// SPDX-License-Identifier: Apache-2.0
package e2echeck

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Summary describes the terminal state of test events observed in a go test JSON stream.
type Summary struct {
	Executed       int
	Passed         int
	Failed         int
	Skipped        int
	RequiredRan    bool
	RequiredPassed bool
}

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
	Output string `json:"Output"`
}

// Check validates that at least one test executed, not all tests were skipped,
// and the named required test both ran and passed.
func Check(r io.Reader, requiredTest string) (Summary, error) {
	summary, sawNoPackages, err := scanTestEvents(r, requiredTest)
	if err != nil {
		return summary, err
	}
	return validateSummary(summary, requiredTest, sawNoPackages)
}

func scanTestEvents(r io.Reader, requiredTest string) (Summary, bool, error) {
	var summary Summary
	var sawNoPackages bool

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		event, hasTest, lineHasNoPackages := parseTestEvent(scanner.Text())
		sawNoPackages = sawNoPackages || lineHasNoPackages
		if hasTest {
			applyTestEvent(&summary, event, requiredTest)
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, sawNoPackages, fmt.Errorf("read go test JSON: %w", err)
	}
	return summary, sawNoPackages, nil
}

func parseTestEvent(line string) (testEvent, bool, bool) {
	noPackages := containsNoPackagesMessage(line)
	var event testEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return testEvent{}, false, noPackages
	}
	noPackages = noPackages || containsNoPackagesMessage(event.Output)
	return event, event.Test != "", noPackages
}

func containsNoPackagesMessage(value string) bool {
	return strings.Contains(value, "no packages to test") || strings.Contains(value, "matched no packages")
}

func applyTestEvent(summary *Summary, event testEvent, requiredTest string) {
	switch event.Action {
	case "run":
		summary.Executed++
		summary.RequiredRan = summary.RequiredRan || event.Test == requiredTest
	case "pass":
		summary.Passed++
		summary.RequiredPassed = summary.RequiredPassed || event.Test == requiredTest
	case "fail":
		summary.Failed++
	case "skip":
		summary.Skipped++
	}
}

func validateSummary(summary Summary, requiredTest string, sawNoPackages bool) (Summary, error) {
	switch {
	case sawNoPackages:
		return summary, fmt.Errorf("no packages to test")
	case summary.Executed == 0:
		return summary, fmt.Errorf("zero tests executed")
	case summary.Passed == 0 && summary.Skipped == summary.Executed:
		return summary, fmt.Errorf("all executed tests were skipped")
	case !summary.RequiredRan || !summary.RequiredPassed:
		return summary, fmt.Errorf("required test %s did not pass", requiredTest)
	case summary.Failed > 0:
		return summary, fmt.Errorf("%d tests failed", summary.Failed)
	default:
		return summary, nil
	}
}
