#!/usr/bin/env bash
# Verifies two-replica controller leadership transfer and reconciliation continuity.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SYSTEM_NAMESPACE=${DRAFORGE_INSTALL_E2E_SYSTEM_NAMESPACE:-draforge-system}
FIXTURE_NAMESPACE=${DRAFORGE_INSTALL_E2E_FIXTURE_NAMESPACE:-draforge-e2e}
RELEASE_NAME=${DRAFORGE_INSTALL_E2E_RELEASE:-draforge}
CLAIM_NAME=${DRAFORGE_INSTALL_E2E_CLAIM:-e2e-gpu-claim}
FAILOVER_CLAIM_NAME=${DRAFORGE_INSTALL_E2E_FAILOVER_CLAIM:-e2e-failover-claim}
POOL_NAME=${DRAFORGE_INSTALL_E2E_POOL:-e2e-gpu-pool}
DRIVER_NAME=${DRAFORGE_INSTALL_E2E_DRIVER:-sim.draforge.oaslananka}
WAIT_ATTEMPTS=${DRAFORGE_INSTALL_E2E_WAIT_ATTEMPTS:-60}
WAIT_INTERVAL=${DRAFORGE_INSTALL_E2E_WAIT_INTERVAL:-2}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}
LEASE_NAME=${DRAFORGE_INSTALL_E2E_LEASE_NAME:-${RELEASE_NAME}-controller}
CONTROLLER_SELECTOR="app.kubernetes.io/instance=${RELEASE_NAME},app.kubernetes.io/component=controller"
FAILOVER_FIXTURE=${DRAFORGE_INSTALL_E2E_FAILOVER_FIXTURE:-$ROOT_DIR/tests/install-e2e/failover-claim.yaml}
REPORT_FILE="$ARTIFACT_DIR/controller-ha-report.json"
ORIGINAL_ALLOCATION_FILE="$ARTIFACT_DIR/original-claim-allocation.json"
CURRENT_ALLOCATION_FILE="$ARTIFACT_DIR/original-claim-allocation-after-failover.json"
CURRENT_LEADER=""
ORIGINAL_LEADER=""

mkdir -p "$ARTIFACT_DIR"

fail() {
  echo "ERROR: $*" >&2
  return 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

write_report() {
  local status=$1
  local passed=false
  [[ "$status" -eq 0 ]] && passed=true
  jq -n \
    --argjson passed "$passed" \
    --arg originalLeader "$ORIGINAL_LEADER" \
    --arg replacementLeader "$CURRENT_LEADER" \
    --arg lease "$LEASE_NAME" \
    --arg namespace "$SYSTEM_NAMESPACE" \
    '{passed: $passed, originalLeader: $originalLeader, replacementLeader: $replacementLeader, lease: $lease, namespace: $namespace}' \
    > "$REPORT_FILE"
}

cleanup() {
  local status=$?
  write_report "$status" || true
  return "$status"
}
trap cleanup EXIT

wait_for() {
  local description=$1
  shift
  for _ in $(seq 1 "$WAIT_ATTEMPTS"); do
    if "$@"; then
      echo "Verified: $description"
      return 0
    fi
    sleep "$WAIT_INTERVAL"
  done
  fail "timed out waiting for $description"
}

ready_controller_pods() {
  kubectl get pods -n "$SYSTEM_NAMESPACE" -l "$CONTROLLER_SELECTOR" -o json 2>/dev/null | jq -r '
    .items[] |
    select(.status.phase == "Running") |
    select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) |
    .metadata.name
  '
}

lease_holder() {
  kubectl get lease.coordination.k8s.io "$LEASE_NAME" -n "$SYSTEM_NAMESPACE" -o json 2>/dev/null |
    jq -r '.spec.holderIdentity // empty'
}

pod_metrics() {
  local pod=$1
  kubectl get --raw "/api/v1/namespaces/${SYSTEM_NAMESPACE}/pods/${pod}:8082/proxy/metrics" 2>/dev/null
}

stable_single_leader() {
  local excluded=${1:-}
  local holder metrics leader_count=0 leader=""
  local pods=()
  mapfile -t pods < <(ready_controller_pods)
  [[ "${#pods[@]}" -eq 2 ]] || return 1

  holder=$(lease_holder)
  [[ -n "$holder" ]] || return 1
  for pod in "${pods[@]}"; do
    metrics=$(pod_metrics "$pod") || return 1
    printf '%s\n' "$metrics" > "$ARTIFACT_DIR/controller-${pod}-metrics.txt"
    if grep -Eq '^draforge_controller_leader 1$' <<<"$metrics"; then
      leader=$pod
      leader_count=$((leader_count + 1))
    elif ! grep -Eq '^draforge_controller_leader 0$' <<<"$metrics"; then
      return 1
    fi
  done

  [[ "$leader_count" -eq 1 ]] || return 1
  [[ "$leader" == "$holder" ]] || return 1
  [[ -z "$excluded" || "$leader" != "$excluded" ]] || return 1
  CURRENT_LEADER=$leader
  return 0
}

capture_claim_allocation() {
  local claim=$1
  local output=$2
  kubectl get resourceclaim.resource.k8s.io "$claim" -n "$FIXTURE_NAMESPACE" -o json 2>/dev/null |
    jq -S '.status.allocation.devices.results // []' > "$output"
  jq -e 'length > 0' "$output" >/dev/null
}

resource_slice_converged() {
  kubectl get resourceslices.resource.k8s.io \
    -l "draforge.oaslananka/sdp-name=${POOL_NAME},draforge.oaslananka/sdp-namespace=${FIXTURE_NAMESPACE}" \
    -o json 2>/dev/null | jq -e \
      --arg pool "$POOL_NAME" \
      --arg driver "$DRIVER_NAME" \
      '.items | length == 1 and .[0].spec.pool.name == $pool and .[0].spec.driver == $driver and (.[0].spec.devices | length) == 3' \
      >/dev/null
}

failover_claim_allocated() {
  kubectl get resourceclaim.resource.k8s.io "$FAILOVER_CLAIM_NAME" \
    -n "$FIXTURE_NAMESPACE" -o json 2>/dev/null | jq -e \
      --arg pool "$POOL_NAME" \
      --arg driver "$DRIVER_NAME" \
      '(.status.allocation.devices.results // []) as $results |
       ($results | length) == 1 and
       ($results | all(.pool == $pool and .driver == $driver and (.device | length) > 0))' \
      >/dev/null
}

for tool in kubectl jq; do
  need "$tool"
done
[[ -f "$FAILOVER_FIXTURE" ]] || fail "missing failover claim fixture: $FAILOVER_FIXTURE"

wait_for "two ready controller replicas with exactly one Lease leader" stable_single_leader ""
ORIGINAL_LEADER=$CURRENT_LEADER
capture_claim_allocation "$CLAIM_NAME" "$ORIGINAL_ALLOCATION_FILE"
echo "Initial controller leader: $ORIGINAL_LEADER"

kubectl delete pod "$ORIGINAL_LEADER" -n "$SYSTEM_NAMESPACE" --wait=false
wait_for "leadership transfer to a replacement controller" stable_single_leader "$ORIGINAL_LEADER"
echo "Replacement controller leader: $CURRENT_LEADER"

# Submit concurrent writes, then establish one authoritative final state. This
# exercises informer coalescing and conflict-safe reconciliation after failover.
patch_pids=()
for device_count in 4 5 6; do
  kubectl patch simulateddevicepool.draforge.oaslananka "$POOL_NAME" \
    -n "$FIXTURE_NAMESPACE" --type=merge \
    -p "{\"spec\":{\"deviceCount\":${device_count}}}" \
    > "$ARTIFACT_DIR/pool-patch-${device_count}.log" 2>&1 &
  patch_pids+=("$!")
done
for pid in "${patch_pids[@]}"; do
  wait "$pid"
done
kubectl patch simulateddevicepool.draforge.oaslananka "$POOL_NAME" \
  -n "$FIXTURE_NAMESPACE" --type=merge \
  -p '{"spec":{"deviceCount":3}}' \
  > "$ARTIFACT_DIR/pool-patch-final.log"
wait_for "one authoritative three-device ResourceSlice after the update burst" resource_slice_converged

kubectl apply -f "$FAILOVER_FIXTURE"
wait_for "new ResourceClaim allocation after controller failover" failover_claim_allocated
capture_claim_allocation "$CLAIM_NAME" "$CURRENT_ALLOCATION_FILE"
cmp -s "$ORIGINAL_ALLOCATION_FILE" "$CURRENT_ALLOCATION_FILE" || \
  fail "original ResourceClaim allocation changed during leadership transfer"

wait_for "one stable controller leader after reconciliation" stable_single_leader "$ORIGINAL_LEADER"
pod_metrics "$CURRENT_LEADER" > "$ARTIFACT_DIR/controller-leader-metrics.txt"
grep -Eq '^draforge_controller_allocations_simulated_total [1-9][0-9]*$' \
  "$ARTIFACT_DIR/controller-leader-metrics.txt" || \
  fail "replacement leader did not report the post-failover allocation"

echo "Controller HA E2E verification passed."
