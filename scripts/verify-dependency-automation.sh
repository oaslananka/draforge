#!/usr/bin/env bash
set -euo pipefail

config=.github/dependabot.yml
workflow=.github/workflows/dependabot-automerge.yml

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || fail "$file is missing required policy: $text"
}

assert_absent() {
  local file=$1
  local text=$2
  if grep -Fq -- "$text" "$file"; then
    fail "$file contains disallowed policy: $text"
  fi
}

[[ -f "$config" ]] || fail "$config is missing"
[[ -f "$workflow" ]] || fail "$workflow is missing"
[[ ! -e renovate.json ]] || fail "renovate.json must not coexist with Dependabot automation"

for ecosystem in gomod npm github-actions; do
  assert_contains "$config" "package-ecosystem: \"$ecosystem\""
done
assert_contains "$config" 'timezone: "Europe/Istanbul"'
assert_contains "$config" 'applies-to: "security-updates"'
assert_contains "$config" 'open-pull-requests-limit: 5'
assert_contains "$config" 'update-types:'
assert_contains "$config" '- "minor"'
assert_contains "$config" '- "patch"'
assert_absent "$config" '- "major"'
assert_absent "$config" 'target-branch:'

assert_contains "$workflow" "github.repository == 'oaslananka/draforge'"
assert_contains "$workflow" "github.event.pull_request.user.login == 'dependabot[bot]'"
assert_contains "$workflow" 'contents: write'
assert_contains "$workflow" 'pull-requests: write'
assert_contains "$workflow" 'dependabot/fetch-metadata@25dd0e34f4fe68f24cc83900b1fe3fe149efef98'
assert_contains "$workflow" "version-update:semver-patch"
assert_contains "$workflow" "version-update:semver-minor"
assert_contains "$workflow" "version-update:semver-major"
assert_contains "$workflow" 'gh pr merge --auto --squash'
assert_absent "$workflow" 'pull_request_target:'
assert_absent "$workflow" 'actions/checkout@'

python3 - <<'PY'
from pathlib import Path

config = Path('.github/dependabot.yml').read_text(encoding='utf-8')
if config.count('package-ecosystem:') != 3:
    raise SystemExit('Dependabot policy must define exactly three package ecosystems')
if config.count('applies-to: "security-updates"') != 3:
    raise SystemExit('Each package ecosystem must group security updates')
if config.count('open-pull-requests-limit: 5') != 3:
    raise SystemExit('Each package ecosystem must use the bounded PR limit')
PY

echo "Dependency automation policy verified."
