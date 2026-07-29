#!/usr/bin/env bash
# Deterministic contract tests for the two-replica controller HA verifier.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_BIN"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cat > "$FAKE_BIN/kubectl" <<'KUBECTL'
#!/usr/bin/env bash
set -euo pipefail

printf 'kubectl' >> "$FAKE_COMMAND_LOG"
for argument in "$@"; do
  printf ' %q' "$argument" >> "$FAKE_COMMAND_LOG"
done
printf '\n' >> "$FAKE_COMMAND_LOG"

state_dir=$FAKE_STATE_DIR
mkdir -p "$state_dir"
failed_over=0
[[ -f "$state_dir/failed-over" ]] && failed_over=1
command_name=${1:-}
shift || true

original_claim='[{"driver":"sim.draforge.oaslananka","pool":"e2e-gpu-pool","device":"dev-0"},{"driver":"sim.draforge.oaslananka","pool":"e2e-gpu-pool","device":"dev-1"}]'

case "$command_name" in
  get)
    if [[ "${1:-}" == "--raw" ]]; then
      path=${2:-}
      pod=${path#*/pods/}
      pod=${pod%%:*}
      leader=controller-a
      [[ "$failed_over" == "1" ]] && leader=controller-b
      if [[ "${FAKE_TWO_LEADERS:-0}" == "1" ]]; then
        leader_value=1
      elif [[ "$pod" == "$leader" ]]; then
        leader_value=1
      else
        leader_value=0
      fi
      allocation_value=0
      [[ "$failed_over" == "1" && "$pod" == "$leader" ]] && allocation_value=1
      cat <<METRICS
draforge_controller_leader ${leader_value}
draforge_controller_allocations_simulated_total ${allocation_value}
draforge_controller_sync_attempts_total{pipeline="pool"} 2
draforge_controller_sync_attempts_total{pipeline="allocation"} 2
METRICS
      exit 0
    fi

    resource=${1:-}
    shift || true
    case "$resource" in
      pods)
        if [[ "$failed_over" == "1" ]]; then
          pods='["controller-b","controller-c"]'
        else
          pods='["controller-a","controller-b"]'
        fi
        jq -n --argjson pods "$pods" '{items: [$pods[] | {metadata:{name:.},status:{phase:"Running",conditions:[{type:"Ready",status:"True"}]}}]}'
        ;;
      lease.coordination.k8s.io)
        holder=controller-a
        [[ "$failed_over" == "1" ]] && holder=controller-b
        jq -n --arg holder "$holder" '{spec:{holderIdentity:$holder}}'
        ;;
      resourceclaim.resource.k8s.io)
        claim=${1:-}
        if [[ "$claim" == "e2e-failover-claim" ]]; then
          [[ -f "$state_dir/failover-claim" ]] || { printf '{"status":{}}\n'; exit 0; }
          cat <<JSON
{"status":{"allocation":{"devices":{"results":[{"driver":"sim.draforge.oaslananka","pool":"e2e-gpu-pool","device":"dev-2"}]}}}}
JSON
        else
          if [[ "${FAKE_ORIGINAL_ALLOCATION_DRIFT:-0}" == "1" && "$failed_over" == "1" ]]; then
            original_claim='[{"driver":"sim.draforge.oaslananka","pool":"e2e-gpu-pool","device":"dev-1"}]'
          fi
          jq -n --argjson results "$original_claim" '{status:{allocation:{devices:{results:$results}}}}'
        fi
        ;;
      resourceslices.resource.k8s.io)
        cat <<JSON
{"items":[{"metadata":{"name":"sim-slice-e2e-gpu-pool","labels":{"draforge.oaslananka/sdp-name":"e2e-gpu-pool","draforge.oaslananka/sdp-namespace":"draforge-e2e"}},"spec":{"driver":"sim.draforge.oaslananka","pool":{"name":"e2e-gpu-pool"},"devices":[{"name":"dev-0"},{"name":"dev-1"},{"name":"dev-2"}]}}]}
JSON
        ;;
      *)
        echo "unsupported fake kubectl get resource: $resource" >&2
        exit 2
        ;;
    esac
    ;;
  delete)
    resource=${1:-}
    [[ "$resource" == "pod" ]] || { echo "unsupported delete: $resource" >&2; exit 2; }
    touch "$state_dir/failed-over"
    ;;
  patch)
    # Concurrent intermediate patches are intentionally accepted. The verifier
    # follows them with one authoritative deviceCount=3 patch.
    ;;
  apply)
    touch "$state_dir/failover-claim"
    ;;
  *)
    echo "unsupported fake kubectl command: $command_name" >&2
    exit 2
    ;;
esac
KUBECTL
chmod +x "$FAKE_BIN/kubectl"

run_case() {
  local name=$1
  local expected_status=$2
  shift 2
  local case_dir="$TEST_ROOT/$name"
  mkdir -p "$case_dir/state" "$case_dir/artifacts"
  : > "$case_dir/commands.log"

  set +e
  (
    cd "$ROOT_DIR"
    PATH="$FAKE_BIN:$PATH" \
    FAKE_COMMAND_LOG="$case_dir/commands.log" \
    FAKE_STATE_DIR="$case_dir/state" \
    DRAFORGE_INSTALL_E2E_ARTIFACT_DIR="$case_dir/artifacts" \
    DRAFORGE_INSTALL_E2E_WAIT_ATTEMPTS=1 \
    DRAFORGE_INSTALL_E2E_WAIT_INTERVAL=0 \
    "$@" bash scripts/verify-controller-ha-e2e.sh
  ) > "$case_dir/stdout.log" 2> "$case_dir/stderr.log"
  status=$?
  set -e

  [[ "$status" -eq "$expected_status" ]] || {
    cat "$case_dir/stdout.log" >&2 || true
    cat "$case_dir/stderr.log" >&2 || true
    fail "$name exited $status, expected $expected_status"
  }
  [[ -f "$case_dir/artifacts/controller-ha-report.json" ]] || fail "$name did not write an HA report"

  if [[ "$expected_status" -eq 0 ]]; then
    jq -e '.passed == true and .originalLeader == "controller-a" and .replacementLeader == "controller-b"' \
      "$case_dir/artifacts/controller-ha-report.json" >/dev/null
    grep -Fq 'kubectl delete pod controller-a' "$case_dir/commands.log" || fail "leader pod was not deleted"
    grep -Eq 'deviceCount[^0-9]*3' "$case_dir/commands.log" || fail "authoritative pool patch was not applied"
    grep -Fq 'kubectl apply -f' "$case_dir/commands.log" || fail "failover claim was not applied"
    grep -Fq 'Controller HA E2E verification passed.' "$case_dir/stdout.log" || fail "success marker missing"
  else
    jq -e '.passed == false' "$case_dir/artifacts/controller-ha-report.json" >/dev/null
  fi
}

run_case success 0 env
run_case overlapping-leaders 1 env FAKE_TWO_LEADERS=1
run_case allocation-drift 1 env FAKE_ORIGINAL_ALLOCATION_DRIFT=1

grep -Fq 'timed out waiting for two ready controller replicas with exactly one Lease leader' \
  "$TEST_ROOT/overlapping-leaders/stderr.log" || fail "overlap failure was not detected"
grep -Fq 'original ResourceClaim allocation changed during leadership transfer' \
  "$TEST_ROOT/allocation-drift/stderr.log" || fail "allocation drift failure was not detected"

echo "Controller HA E2E harness tests passed."
