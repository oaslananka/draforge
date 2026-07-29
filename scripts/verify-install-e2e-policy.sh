#!/usr/bin/env bash
# Verifies install-level E2E matrix, workflow, release, fixture, and documentation contracts.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSIONS_FILE="$ROOT_DIR/tests/install-e2e/kubernetes-versions.json"
WORKFLOW_FILE="$ROOT_DIR/.github/workflows/e2e-matrix.yml"
RELEASE_FILE="$ROOT_DIR/.github/workflows/release.yml"
VALUES_FILE="$ROOT_DIR/tests/install-e2e/values.yaml"
RESOURCES_FILE="$ROOT_DIR/tests/install-e2e/resources.yaml"
WORKLOAD_FILE="$ROOT_DIR/tests/install-e2e/workload.yaml"
FAILOVER_CLAIM_FILE="$ROOT_DIR/tests/install-e2e/failover-claim.yaml"
DOC_FILE="$ROOT_DIR/docs/e2e-matrix.md"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_before() {
  local file=$1
  local first=$2
  local second=$3
  local first_line second_line
  first_line=$(grep -nF -- "$first" "$file" | head -1 | cut -d: -f1)
  second_line=$(grep -nF -- "$second" "$file" | head -1 | cut -d: -f1)
  [[ -n "$first_line" && -n "$second_line" && $first_line -lt $second_line ]] || \
    fail "$file must place '$first' before '$second'"
}

command -v jq >/dev/null 2>&1 || fail "jq is required"

jq -e '
  (.kindVersion | type == "string" and startswith("v")) and
  (.profiles["pull-request"] | length == 1) and
  (.profiles.full | length >= 2) and
  ([.profiles[][].nodeImage | contains("@sha256:")] | all) and
  ([.profiles["pull-request"][].nodeImage] - [.profiles.full[].nodeImage] | length == 0) and
  (.networkPolicyProvider.name == "calico") and
  (.networkPolicyProvider.version | type == "string" and startswith("v3.")) and
  (.networkPolicyProvider.manifestUrl == ("https://raw.githubusercontent.com/projectcalico/calico/" + .networkPolicyProvider.version + "/manifests/calico.yaml")) and
  (.networkPolicyProvider.sha256 | test("^[0-9a-f]{64}$"))
' "$VERSIONS_FILE" >/dev/null || fail "invalid Kubernetes install E2E version policy"

assert_contains "$WORKFLOW_FILE" "pull_request:"
assert_contains "$WORKFLOW_FILE" "schedule:"
assert_contains "$WORKFLOW_FILE" "workflow_call:"
assert_contains "$WORKFLOW_FILE" "matrix: \${{ fromJSON(needs.matrix.outputs.matrix) }}"
assert_contains "$WORKFLOW_FILE" "disableDefaultCNI: true"
assert_contains "$WORKFLOW_FILE" "scripts/install-e2e-cni.sh"
assert_contains "$WORKFLOW_FILE" "scripts/build-install-e2e-images.sh"
assert_contains "$WORKFLOW_FILE" "scripts/run-install-e2e.sh"
assert_contains "$WORKFLOW_FILE" "scripts/collect-install-e2e-artifacts.sh"
assert_contains "$WORKFLOW_FILE" "if: failure()"
assert_contains "$WORKFLOW_FILE" "retention-days: 7"
assert_contains "$WORKFLOW_FILE" "timeout-minutes: 55"
assert_contains "$WORKFLOW_FILE" "ref: \${{ inputs.ref || github.ref }}"
assert_contains "$WORKFLOW_FILE" "target_sha: \${{ steps.target.outputs.sha }}"
assert_contains "$WORKFLOW_FILE" "DRAFORGE_INSTALL_E2E_COMMIT: \${{ needs.matrix.outputs.target_sha }}"

assert_contains "$RELEASE_FILE" "workflow_dispatch:"
assert_contains "$RELEASE_FILE" "release_tag:"
assert_contains "$RELEASE_FILE" "operation:"
assert_contains "$RELEASE_FILE" "Verify published release"
assert_contains "$RELEASE_FILE" "scripts/verify-published-release.sh"
assert_contains "$RELEASE_FILE" "Checkout verification tooling"
assert_contains "$RELEASE_FILE" "Checkout immutable release source"
assert_contains "$RELEASE_FILE" "path: release-source"
assert_contains "$RELEASE_FILE" "RELEASE_SOURCE_DIR: \${{ github.workspace }}/release-source"
assert_contains "$RELEASE_FILE" "uses: ./.github/workflows/e2e-matrix.yml"
assert_contains "$RELEASE_FILE" "profile: full"
assert_contains "$RELEASE_FILE" "ref: \${{ inputs.release_tag || github.ref }}"
assert_contains "$RELEASE_FILE" "needs: install-e2e"
assert_contains "$RELEASE_FILE" "install-only: true"
assert_contains "$RELEASE_FILE" "run: goreleaser release --clean"
assert_before "$RELEASE_FILE" "install-only: true" "python3 scripts/verify-goreleaser-docker-v2.py --self-test --check"

assert_contains "$VALUES_FILE" "pullPolicy: Never"
assert_contains "$VALUES_FILE" "networkPolicies:"
assert_contains "$VALUES_FILE" "enabled: true"
assert_contains "$VALUES_FILE" "outputMode: demo"
assert_contains "$VALUES_FILE" "replicaCount: 2"
assert_contains "$VALUES_FILE" "leaseDuration: 5s"
assert_contains "$VALUES_FILE" "renewDeadline: 3s"
assert_contains "$VALUES_FILE" "retryPeriod: 1s"

for kind in Namespace DeviceClass SimulatedDevicePool ResourceClaim; do
  assert_contains "$RESOURCES_FILE" "kind: $kind"
done
assert_contains "$RESOURCES_FILE" "namespace: draforge-e2e"
assert_contains "$RESOURCES_FILE" "name: e2e-gpu-claim"
assert_contains "$WORKLOAD_FILE" "kind: Pod"
assert_contains "$WORKLOAD_FILE" "resourceClaimName: e2e-gpu-claim"
assert_contains "$WORKLOAD_FILE" "claims:"
assert_contains "$FAILOVER_CLAIM_FILE" "name: e2e-failover-claim"
assert_contains "$FAILOVER_CLAIM_FILE" "allocationMode: ExactCount"
assert_contains "$FAILOVER_CLAIM_FILE" "count: 1"
assert_contains "$ROOT_DIR/scripts/run-install-e2e.sh" 'scripts/verify-controller-ha-e2e.sh'
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "name: network-policy-allowed"
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "name: network-policy-denied"
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" 'draforge.oaslananka/metrics-client: "true"'
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "automountServiceAccountToken: false"
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "runAsNonRoot: true"
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "runAsUser: 1000"
assert_contains "$ROOT_DIR/tests/install-e2e/network-policy-probes.yaml" "ephemeral-storage: 16Mi"
assert_contains "$WORKLOAD_FILE" "automountServiceAccountToken: false"
assert_contains "$WORKLOAD_FILE" "runAsNonRoot: true"
assert_contains "$WORKLOAD_FILE" "runAsUser: 1000"
assert_contains "$WORKLOAD_FILE" "ephemeral-storage: 16Mi"

for script in \
  e2e-matrix.sh \
  install-e2e-cni.sh \
  build-install-e2e-images.sh \
  run-install-e2e.sh \
  collect-install-e2e-artifacts.sh \
  kind-install-e2e.sh \
  kind-install-e2e-matrix.sh \
  test-install-e2e-cni.sh \
  test-install-e2e-harness.sh \
  test-controller-ha-e2e-harness.sh \
  verify-controller-ha-e2e.sh \
  verify-install-e2e-policy.sh \
  verify-published-release.sh; do
  [[ -x "$ROOT_DIR/scripts/$script" ]] || fail "scripts/$script must be executable"
done

assert_contains "$DOC_FILE" "tests/install-e2e/kubernetes-versions.json"
assert_contains "$DOC_FILE" "pull-request"
assert_contains "$DOC_FILE" "full"
assert_contains "$DOC_FILE" "release cannot publish"
assert_contains "$DOC_FILE" "artifacts/install-e2e/"

assert_contains "$ROOT_DIR/.gitignore" "artifacts/install-e2e*/"
assert_contains "$ROOT_DIR/.dockerignore" "artifacts"
assert_contains "$ROOT_DIR/.dockerignore" "web/node_modules"

assert_contains "$ROOT_DIR/build/package/Dockerfile.server" "FROM golang:1.26.5 AS backend-builder"
assert_contains "$ROOT_DIR/build/package/Dockerfile.controller" "FROM golang:1.26.5 AS builder"
assert_contains "$ROOT_DIR/build/package/Dockerfile.sim-driver" "FROM golang:1.26.5 AS builder"

echo "Install-level E2E policy verified."
