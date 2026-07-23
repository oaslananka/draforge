#!/usr/bin/env bash
# Runs the tagged E2E package and verifies that TestSmoke executed and passed.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

RESULTS_FILE=${DRAFORGE_E2E_RESULTS:-/artifacts/go-test.json}
RESULTS_DIR=$(dirname "$RESULTS_FILE")
mkdir -p "$RESULTS_DIR"

echo "Running tagged remote E2E tests..."
set +e
DRAFORGE_E2E=1 go test -count=1 -json -tags=e2e ./tests/e2e/... 2>&1 | tee "$RESULTS_FILE"
test_status=${PIPESTATUS[0]}
set -e

checker_status=0
go run ./cmd/draforge-e2e-check --file "$RESULTS_FILE" --required-test TestSmoke || checker_status=$?

if (( test_status != 0 )); then
    echo "ERROR: go test exited with status $test_status" >&2
fi
if (( checker_status != 0 )); then
    echo "ERROR: E2E result verification exited with status $checker_status" >&2
fi
if (( test_status != 0 || checker_status != 0 )); then
    exit 1
fi
