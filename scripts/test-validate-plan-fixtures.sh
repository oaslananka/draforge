#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

python scripts/validate-plan.py tests/fixtures/terraform/showcase-safe-plan.json
if python scripts/validate-plan.py tests/fixtures/terraform/showcase-unsafe-plan.json; then
  echo "policy fixture unexpectedly passed" >&2
  exit 1
fi
