// Package e2echeck tests remote E2E result verification.
// SPDX-License-Identifier: Apache-2.0
package e2echeck

import (
	"strings"
	"testing"
)

func TestCheckAcceptsPassingSmokeTest(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"start","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
		`{"Action":"run","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
		`{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
		`{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
	}, "\n")

	summary, err := Check(strings.NewReader(input), "TestSmoke")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if summary.Executed != 1 || summary.Passed != 1 || !summary.RequiredPassed {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestCheckRejectsInvalidRuns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "no packages",
			input:   `go: warning: "./tests/e2e/..." matched no packages` + "\n" + `no packages to test`,
			wantErr: "no packages to test",
		},
		{
			name: "zero tests",
			input: strings.Join([]string{
				`{"Action":"start","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
				`{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
			}, "\n"),
			wantErr: "zero tests executed",
		},
		{
			name: "all skipped",
			input: strings.Join([]string{
				`{"Action":"run","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
				`{"Action":"skip","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
				`{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
			}, "\n"),
			wantErr: "all executed tests were skipped",
		},
		{
			name: "smoke test failed",
			input: strings.Join([]string{
				`{"Action":"run","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
				`{"Action":"fail","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}`,
				`{"Action":"fail","Package":"github.com/oaslananka/draforge/tests/e2e"}`,
			}, "\n"),
			wantErr: "required test TestSmoke did not pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCheckError(t, tt.input, tt.wantErr)
		})
	}
}

func assertCheckError(t *testing.T, input, wantErr string) {
	t.Helper()
	_, err := Check(strings.NewReader(input), "TestSmoke")
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("Check() error = %v, want substring %q", err, wantErr)
	}
}
