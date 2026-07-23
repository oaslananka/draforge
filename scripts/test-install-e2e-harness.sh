#!/usr/bin/env bash
# Deterministic contract tests for install-level E2E orchestration.
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

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || {
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "expected $file to contain: $text"
  }
}

cat > "$FAKE_BIN/helm" <<'HELM'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm' >> "$FAKE_COMMAND_LOG"
printf ' %q' "$@" >> "$FAKE_COMMAND_LOG"
printf '\n' >> "$FAKE_COMMAND_LOG"
HELM

cat > "$FAKE_BIN/kubectl" <<'KUBECTL'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl' >> "$FAKE_COMMAND_LOG"
printf ' %q' "$@" >> "$FAKE_COMMAND_LOG"
printf '\n' >> "$FAKE_COMMAND_LOG"

command_name=${1:-}
shift || true
case "$command_name" in
  api-resources)
    printf '%s\n' resourceclaims.resource.k8s.io resourceslices.resource.k8s.io deviceclasses.resource.k8s.io
    ;;
  rollout)
    if [[ "${FAKE_MISSING_COMPONENT:-0}" == "1" && "$*" == *controller* ]]; then
      exit 1
    fi
    ;;
  auth)
    identity=""
    verb=""
    resource=""
    subresource=""
    while (( $# > 0 )); do
      case "$1" in
        can-i) shift ;;
        --as=*) identity=${1#--as=}; shift ;;
        --subresource=*) subresource=${1#--subresource=}; shift ;;
        --all-namespaces) shift ;;
        *)
          if [[ -z "$verb" ]]; then verb=$1; else resource=$1; fi
          shift
          ;;
      esac
    done
    if [[ "$verb/$resource" == "list/resourceclaims.resource.k8s.io" ]]; then
      printf 'yes\n'
      exit 0
    fi
    if [[ "$identity" == *:draforge-controller && "$verb/$resource/$subresource" == "patch/resourceclaims.resource.k8s.io/status" ]]; then
      printf 'yes\n'
      exit 0
    fi
    printf 'no\n'
    exit 1
    ;;
  get)
    resource=${1:-}
    case "$resource" in
      networkpolicy)
        cat <<'JSON'
{"spec":{"policyTypes":["Ingress","Egress"]}}
JSON
        ;;
      resourceslices.resource.k8s.io)
        cat <<'JSON'
{"items":[{"spec":{"driver":"sim.draforge.oaslananka","pool":{"name":"e2e-gpu-pool"},"devices":[{"name":"dev-0"}]}}]}
JSON
        ;;
      resourceclaim.resource.k8s.io)
        cat <<'JSON'
{"status":{"allocation":{"devices":{"results":[{"driver":"sim.draforge.oaslananka","pool":"e2e-gpu-pool","device":"dev-0"}]}}}}
JSON
        ;;
      pod)
        cat <<'JSON'
{"spec":{"resourceClaims":[{"name":"gpu","resourceClaimName":"e2e-gpu-claim"}]}}
JSON
        ;;
      *)
        echo "unsupported fake kubectl get resource: $resource" >&2
        exit 2
        ;;
    esac
    ;;
  apply)
    ;;
  wait)
    ;;
  port-forward)
    trap 'exit 0' TERM INT
    while true; do sleep 1; done
    ;;
  exec)
    if [[ "$*" == *network-policy-denied* && "$*" == *draforge-controller* ]]; then
      exit 1
    fi
    if [[ "$*" == *network-policy-denied* && "$*" == *draforge-server* ]]; then
      printf 'ok\n'
      exit 0
    fi
    if [[ "$*" == *network-policy-allowed* && "$*" == *draforge-controller* ]]; then
      printf 'ok\n'
      exit 0
    fi
    echo "unsupported fake kubectl exec: $*" >&2
    exit 2
    ;;
  *)
    echo "unsupported fake kubectl command: $command_name" >&2
    exit 2
    ;;
esac
KUBECTL

cat > "$FAKE_BIN/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl' >> "$FAKE_COMMAND_LOG"
printf ' %q' "$@" >> "$FAKE_COMMAND_LOG"
printf '\n' >> "$FAKE_COMMAND_LOG"
url=${!#}
case "$url" in
  */healthz|*/readyz)
    printf 'ok\n'
    ;;
  */api/claims)
    if [[ "${FAKE_BROKEN_API:-0}" == "1" ]]; then
      printf '[]\n'
    else
      cat <<'JSON'
[{"namespace":"draforge-e2e","name":"e2e-gpu-claim","status":"Allocated","ownerPodName":"e2e-claim-consumer","allocations":[{"driverName":"sim.draforge.oaslananka","poolName":"e2e-gpu-pool","deviceName":"dev-0","nodeName":"kind-control-plane"}]}]
JSON
    fi
    ;;
  */api/summary)
    printf '%s\n' '{"poolsCount":1,"devicesCount":2,"claimsCount":1,"discoveryStatus":{"isPartial":false}}'
    ;;
  */api/graph)
    printf '%s\n' '{"nodes":[{"id":"claim/draforge-e2e/e2e-gpu-claim","type":"ResourceClaim","metadata":{"namespace":"draforge-e2e"}}],"edges":[]}'
    ;;
  */api/explain)
    printf '%s\n' '{"targetName":"e2e-gpu-claim","allocated":true}'
    ;;
  http://127.0.0.1:18082/metrics)
    cat <<'METRICS'
draforge_controller_allocations_simulated_total 1
METRICS
    ;;
  */metrics)
    cat <<'METRICS'
draforge_claims_count 1
draforge_devices_count 2
METRICS
    ;;
  */api/stream)
    printf '%s\n\n' 'data: {"nodes":[{"id":"claim/draforge-e2e/e2e-gpu-claim","type":"ResourceClaim","metadata":{"namespace":"draforge-e2e"}}],"edges":[]}'
    ;;
  *)
    echo "unsupported fake curl URL: $url" >&2
    exit 2
    ;;
esac
CURL

chmod +x "$FAKE_BIN/helm" "$FAKE_BIN/kubectl" "$FAKE_BIN/curl"

run_case() {
  local name=$1
  local expected_status=$2
  shift 2
  local case_dir="$TEST_ROOT/$name"
  mkdir -p "$case_dir/artifacts"
  : > "$case_dir/commands.log"

  set +e
  (
    cd "$ROOT_DIR"
    PATH="$FAKE_BIN:$PATH" \
    FAKE_COMMAND_LOG="$case_dir/commands.log" \
    DRAFORGE_INSTALL_E2E_ARTIFACT_DIR="$case_dir/artifacts" \
    DRAFORGE_INSTALL_E2E_WAIT_ATTEMPTS=1 \
    DRAFORGE_INSTALL_E2E_WAIT_INTERVAL=0 \
    "$@" bash scripts/run-install-e2e.sh
  ) > "$case_dir/stdout.log" 2> "$case_dir/stderr.log"
  local status=$?
  set -e

  [[ "$status" -eq "$expected_status" ]] || {
    cat "$case_dir/stdout.log" >&2 || true
    cat "$case_dir/stderr.log" >&2 || true
    fail "$name exited $status, expected $expected_status"
  }
  [[ -f "$case_dir/artifacts/report.json" ]] || fail "$name did not write report.json"

  if [[ "$expected_status" -eq 0 ]]; then
    jq -e '.passed == true' "$case_dir/artifacts/report.json" >/dev/null
    assert_contains "$case_dir/commands.log" 'helm upgrade --install draforge'
    assert_contains "$case_dir/commands.log" 'kubectl apply -f tests/install-e2e/resources.yaml'
    assert_contains "$case_dir/commands.log" 'kubectl apply -f tests/install-e2e/network-policy-probes.yaml'
    assert_contains "$case_dir/commands.log" 'kubectl apply -f tests/install-e2e/workload.yaml'
    assert_contains "$case_dir/commands.log" 'kubectl port-forward -n draforge-system service/draforge-server'
    assert_contains "$case_dir/commands.log" 'kubectl port-forward -n draforge-system service/draforge-controller'
    assert_contains "$case_dir/stdout.log" 'Install-level E2E verification passed.'
  else
    jq -e '.passed == false' "$case_dir/artifacts/report.json" >/dev/null
  fi
}

run_case success 0 env
run_case broken-api 1 env FAKE_BROKEN_API=1
run_case missing-component 1 env FAKE_MISSING_COMPONENT=1

pull_request_matrix=$("$ROOT_DIR/scripts/e2e-matrix.sh" pull-request)
full_matrix=$("$ROOT_DIR/scripts/e2e-matrix.sh" full)
[[ $(jq '.include | length' <<<"$pull_request_matrix") -eq 1 ]] || fail "pull-request matrix must contain one version"
[[ $(jq '.include | length' <<<"$full_matrix") -eq 2 ]] || fail "full matrix must contain two versions"
if "$ROOT_DIR/scripts/e2e-matrix.sh" unknown >/dev/null 2>&1; then
  fail "unknown matrix profile unexpectedly succeeded"
fi

echo "Install-level E2E harness tests passed."
