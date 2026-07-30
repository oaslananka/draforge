#!/usr/bin/env bash
set -euo pipefail

workflow="${1:-.github/workflows/ci.yml}"
[[ -f "$workflow" ]] || { echo "Missing workflow: $workflow" >&2; exit 1; }

go_block=$(sed -n '/^  go:/,/^  coverage-upload:/p' "$workflow")
coverage_block=$(sed -n '/^  coverage-upload:/,/^  web:/p' "$workflow")
ci_pass_block=$(sed -n '/^  ci-pass:/,$p' "$workflow")

if grep -Fq 'id-token: write' <<<"$go_block"; then
  echo "The code-executing Go job must not receive OIDC write permission." >&2
  exit 1
fi

grep -Fq 'needs: go' <<<"$coverage_block" || {
  echo "Coverage upload must depend on the completed Go job." >&2
  exit 1
}
grep -Fq 'id-token: write' <<<"$coverage_block" || {
  echo "The isolated coverage job must own the OIDC permission." >&2
  exit 1
}
grep -Fq 'actions/download-artifact@634f93cb2916e3fdff6788551b99b062d0335ce0' <<<"$coverage_block" || {
  echo "The isolated coverage job must consume the pinned coverage artifact." >&2
  exit 1
}
if grep -Eq '^[[:space:]]+-[[:space:]]+run:|actions/checkout@' <<<"$coverage_block"; then
  echo "The privileged coverage job must not checkout or execute pull-request code." >&2
  exit 1
fi

grep -Fq 'actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02' <<<"$go_block" || {
  echo "The Go job must stage coverage through the pinned artifact action." >&2
  exit 1
}
grep -Fq 'coverage-upload' <<<"$ci_pass_block" || {
  echo "CI Pass must wait for the isolated coverage upload." >&2
  exit 1
}

for target_workflow in .github/workflows/*.yml; do
  if grep -Fq 'pull_request_target:' "$target_workflow" && grep -Fq 'actions/checkout@' "$target_workflow"; then
    echo "pull_request_target workflow must not checkout untrusted code: $target_workflow" >&2
    exit 1
  fi
done

echo "CI privilege boundaries verified."
