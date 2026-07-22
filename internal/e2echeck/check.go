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
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// Check validates that at least one test executed, not all tests were skipped,
// and the named required test both ran and passed.
func Check(r io.Reader, requiredTest string) (Summary, error) {
	var summary Summary
	var sawNoPackages bool

	scanner := bufio.NewScanner(r)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "no packages to test") || strings.Contains(line, "matched no packages") {
			sawNoPackages = true
		}

		var event testEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if strings.Contains(event.Output, "no packages to test") || strings.Contains(event.Output, "matched no packages") {
			sawNoPackages = true
		}
		if event.Test == "" {
			continue
		}

		switch event.Action {
		case "run":
			summary.Executed++
			if event.Test == requiredTest {
				summary.RequiredRan = true
			}
		case "pass":
			summary.Passed++
			if event.Test == requiredTest {
				summary.RequiredPassed = true
			}
		case "fail":
			summary.Failed++
		case "skip":
			summary.Skipped++
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, fmt.Errorf("read go test JSON: %w", err)
	}
	if sawNoPackages {
		return summary, fmt.Errorf("no packages to test")
	}
	if summary.Executed == 0 {
		return summary, fmt.Errorf("zero tests executed")
	}
	if summary.Passed == 0 && summary.Skipped == summary.Executed {
		return summary, fmt.Errorf("all executed tests were skipped")
	}
	if !summary.RequiredRan || !summary.RequiredPassed {
		return summary, fmt.Errorf("required test %s did not pass", requiredTest)
	}
	if summary.Failed > 0 {
		return summary, fmt.Errorf("%d tests failed", summary.Failed)
	}
	return summary, nil
}
