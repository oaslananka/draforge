#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

safe_fixture=tests/fixtures/terraform/showcase-safe-plan.json
unsafe_fixture=tests/fixtures/terraform/showcase-unsafe-plan.json
outside_plan=$(mktemp --suffix=.json)
outside_link=tests/fixtures/terraform/outside-plan-link.json
wrong_extension=tests/fixtures/terraform/showcase-safe-plan.txt
cleanup() {
  rm -f "$outside_plan" "$outside_link" "$wrong_extension"
}
trap cleanup EXIT

python scripts/validate-plan.py "$safe_fixture"
if python scripts/validate-plan.py "$unsafe_fixture"; then
  echo "policy fixture unexpectedly passed" >&2
  exit 1
fi

cp "$safe_fixture" "$outside_plan"
if python scripts/validate-plan.py "$outside_plan"; then
  echo "plan outside the repository unexpectedly passed" >&2
  exit 1
fi

ln -s "$(realpath "$outside_plan")" "$outside_link"
if python scripts/validate-plan.py "$outside_link"; then
  echo "symlink escaping the repository unexpectedly passed" >&2
  exit 1
fi

cp "$safe_fixture" "$wrong_extension"
if python scripts/validate-plan.py "$wrong_extension"; then
  echo "non-JSON plan unexpectedly passed" >&2
  exit 1
fi
