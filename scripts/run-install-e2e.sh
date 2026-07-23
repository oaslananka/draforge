#!/usr/bin/env bash
# Installs the complete Helm release and verifies the scenario-to-dashboard path.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SYSTEM_NAMESPACE=${DRAFORGE_INSTALL_E2E_SYSTEM_NAMESPACE:-draforge-system}
FIXTURE_NAMESPACE=${DRAFORGE_INSTALL_E2E_FIXTURE_NAMESPACE:-draforge-e2e}
RELEASE_NAME=${DRAFORGE_INSTALL_E2E_RELEASE:-draforge}
CLAIM_NAME=${DRAFORGE_INSTALL_E2E_CLAIM:-e2e-gpu-claim}
CONSUMER_POD=${DRAFORGE_INSTALL_E2E_CONSUMER_POD:-e2e-claim-consumer}
POOL_NAME=${DRAFORGE_INSTALL_E2E_POOL:-e2e-gpu-pool}
DRIVER_NAME=${DRAFORGE_INSTALL_E2E_DRIVER:-sim.draforge.oaslananka}
SERVER_PORT=${DRAFORGE_INSTALL_E2E_SERVER_PORT:-18080}
CONTROLLER_PORT=${DRAFORGE_INSTALL_E2E_CONTROLLER_PORT:-18082}
WAIT_ATTEMPTS=${DRAFORGE_INSTALL_E2E_WAIT_ATTEMPTS:-60}
WAIT_INTERVAL=${DRAFORGE_INSTALL_E2E_WAIT_INTERVAL:-2}
HELM_TIMEOUT=${DRAFORGE_INSTALL_E2E_HELM_TIMEOUT:-5m}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}
REPORT_FILE="$ARTIFACT_DIR/report.json"
LOG_FILE="$ARTIFACT_DIR/run.log"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
CONTROLLER_URL="http://127.0.0.1:${CONTROLLER_PORT}"
# Test-only service traffic contains no credentials and is isolated by the enforced NetworkPolicies.
IN_CLUSTER_SCHEME=http
SERVER_PORT_FORWARD_PID=""
CONTROLLER_PORT_FORWARD_PID=""

mkdir -p "$ARTIFACT_DIR"
: > "$LOG_FILE"
exec > >(tee -a "$LOG_FILE") 2>&1

fail() {
  echo "ERROR: $*" >&2
  return 1
}

need() {
  local tool
  tool=$1
  command -v "$tool" >/dev/null 2>&1 || fail "missing required tool: $tool"
}

write_report() {
  local status
  status=$1
  local passed=false
  [[ "$status" -eq 0 ]] && passed=true
  jq -n \
    --argjson passed "$passed" \
    --arg release "$RELEASE_NAME" \
    --arg systemNamespace "$SYSTEM_NAMESPACE" \
    --arg fixtureNamespace "$FIXTURE_NAMESPACE" \
    --arg claim "$CLAIM_NAME" \
    --arg consumerPod "$CONSUMER_POD" \
    --arg pool "$POOL_NAME" \
    --arg driver "$DRIVER_NAME" \
    '{passed: $passed, release: $release, systemNamespace: $systemNamespace, fixtureNamespace: $fixtureNamespace, claim: $claim, consumerPod: $consumerPod, pool: $pool, driver: $driver}' \
    > "$REPORT_FILE"
}

cleanup() {
  local status=$?
  for pid in "$SERVER_PORT_FORWARD_PID" "$CONTROLLER_PORT_FORWARD_PID"; do
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
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

resource_slice_ready() {
  kubectl get resourceslices.resource.k8s.io \
    -l 'draforge.oaslananka/managed-by=simulator' \
    -o json 2>/dev/null | jq -e \
      --arg pool "$POOL_NAME" \
      --arg driver "$DRIVER_NAME" \
      '.items | any(.spec.pool.name == $pool and .spec.driver == $driver and (.spec.devices | length) >= 1)' \
      >/dev/null
}

claim_allocated() {
  kubectl get resourceclaim.resource.k8s.io "$CLAIM_NAME" \
    -n "$FIXTURE_NAMESPACE" -o json 2>/dev/null | jq -e \
      --arg pool "$POOL_NAME" \
      --arg driver "$DRIVER_NAME" \
      '.status.allocation.devices.results | any(.pool == $pool and .driver == $driver and (.device | length) > 0)' \
      >/dev/null
}

consumer_exists() {
  kubectl get pod "$CONSUMER_POD" -n "$FIXTURE_NAMESPACE" -o json 2>/dev/null | jq -e \
    --arg claim "$CLAIM_NAME" \
    '.spec.resourceClaims | any(.resourceClaimName == $claim)' >/dev/null
}

cluster_service_url() {
  local service namespace port path
  service=$1
  namespace=$2
  port=$3
  path=$4
  printf '%s://%s.%s.svc:%s%s' "$IN_CLUSTER_SCHEME" "$service" "$namespace" "$port" "$path"
}

network_policy_baseline_allowed() {
  local endpoint
  endpoint=$(cluster_service_url "$RELEASE_NAME-server" "$SYSTEM_NAMESPACE" 8080 /readyz)
  kubectl exec -n "$FIXTURE_NAMESPACE" network-policy-denied -- \
    wget -T 2 -qO- "$endpoint" >/dev/null
}

network_policy_metrics_allowed() {
  local endpoint
  endpoint=$(cluster_service_url "$RELEASE_NAME-controller" "$SYSTEM_NAMESPACE" 8082 /readyz)
  kubectl exec -n "$SYSTEM_NAMESPACE" network-policy-allowed -- \
    wget -T 2 -qO- "$endpoint" >/dev/null
}

network_policy_metrics_denied() {
  local endpoint
  endpoint=$(cluster_service_url "$RELEASE_NAME-controller" "$SYSTEM_NAMESPACE" 8082 /readyz)
  if kubectl exec -n "$FIXTURE_NAMESPACE" network-policy-denied -- \
    wget -T 2 -qO- "$endpoint" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

http_ready() {
  curl -fsS --max-time 2 "$SERVER_URL/readyz" >/dev/null
}

controller_metrics_ready() {
  curl -fsS --max-time 2 "$CONTROLLER_URL/metrics" > "$ARTIFACT_DIR/controller-metrics.txt"
}

api_claim_ready() {
  curl -fsS --max-time 5 "$SERVER_URL/api/claims" > "$ARTIFACT_DIR/claims.json" || return 1
  jq -e \
    --arg namespace "$FIXTURE_NAMESPACE" \
    --arg claim "$CLAIM_NAME" \
    --arg pod "$CONSUMER_POD" \
    --arg pool "$POOL_NAME" \
    --arg driver "$DRIVER_NAME" \
    '.[] | select(.namespace == $namespace and .name == $claim and .status == "Allocated" and .ownerPodName == $pod) | .allocations | any(.poolName == $pool and .driverName == $driver and (.deviceName | length) > 0)' \
    "$ARTIFACT_DIR/claims.json" >/dev/null
}

assert_allowed() {
  local identity=$1
  shift
  local result
  result=$(kubectl auth can-i --as="$identity" "$@")
  [[ "$result" == "yes" ]] || fail "expected $identity to be allowed: kubectl auth can-i $*"
}

assert_denied() {
  local identity=$1
  shift
  local result
  result=$(kubectl auth can-i --as="$identity" "$@" 2>/dev/null || true)
  [[ "$result" == "no" ]] || fail "expected $identity to be denied: kubectl auth can-i $*"
}

for tool in kubectl helm jq curl; do
  need "$tool"
done

cd "$ROOT_DIR"

echo "Verifying resource.k8s.io/v1 API availability..."
kubectl api-resources --api-group=resource.k8s.io -o name | grep -Fxq 'resourceclaims.resource.k8s.io' || \
  fail "resourceclaims.resource.k8s.io is not served by the cluster"
kubectl api-resources --api-group=resource.k8s.io -o name | grep -Fxq 'resourceslices.resource.k8s.io' || \
  fail "resourceslices.resource.k8s.io is not served by the cluster"

echo "Installing complete DRAForge Helm release..."
helm upgrade --install "$RELEASE_NAME" deploy/helm/draforge \
  --namespace "$SYSTEM_NAMESPACE" \
  --create-namespace \
  --values tests/install-e2e/values.yaml \
  --wait \
  --timeout "$HELM_TIMEOUT"

kubectl rollout status deployment/"$RELEASE_NAME"-server -n "$SYSTEM_NAMESPACE" --timeout="$HELM_TIMEOUT"
kubectl rollout status deployment/"$RELEASE_NAME"-controller -n "$SYSTEM_NAMESPACE" --timeout="$HELM_TIMEOUT"
kubectl rollout status daemonset/"$RELEASE_NAME"-node-plugin -n "$SYSTEM_NAMESPACE" --timeout="$HELM_TIMEOUT"

for policy in "$RELEASE_NAME-server-policy" "$RELEASE_NAME-controller-policy"; do
  kubectl get networkpolicy "$policy" -n "$SYSTEM_NAMESPACE" -o json | jq -e \
    '.spec.policyTypes | index("Ingress") != null and index("Egress") != null' >/dev/null
  echo "Verified NetworkPolicy: $policy"
done

server_identity="system:serviceaccount:${SYSTEM_NAMESPACE}:${RELEASE_NAME}-server"
controller_identity="system:serviceaccount:${SYSTEM_NAMESPACE}:${RELEASE_NAME}-controller"
plugin_identity="system:serviceaccount:${SYSTEM_NAMESPACE}:${RELEASE_NAME}-node-plugin"
assert_allowed "$server_identity" list resourceclaims.resource.k8s.io --all-namespaces
assert_denied "$server_identity" patch resourceclaims.resource.k8s.io --all-namespaces
assert_allowed "$controller_identity" patch resourceclaims.resource.k8s.io --subresource=status --all-namespaces
assert_denied "$controller_identity" delete nodes
assert_allowed "$plugin_identity" list resourceclaims.resource.k8s.io --all-namespaces
assert_denied "$plugin_identity" patch resourceclaims.resource.k8s.io --all-namespaces
echo "Verified least-privilege RBAC decisions."

echo "Applying simulator scenario and ResourceClaim..."
kubectl apply -f tests/install-e2e/resources.yaml
wait_for "simulator ResourceSlice publication" resource_slice_ready
wait_for "simulated ResourceClaim allocation" claim_allocated

echo "Verifying enforced controller metrics NetworkPolicy..."
kubectl apply -f tests/install-e2e/network-policy-probes.yaml
kubectl wait --for=condition=Ready pod/network-policy-allowed -n "$SYSTEM_NAMESPACE" --timeout=2m
kubectl wait --for=condition=Ready pod/network-policy-denied -n "$FIXTURE_NAMESPACE" --timeout=2m
wait_for "unrestricted baseline connectivity from the denied probe" network_policy_baseline_allowed
wait_for "allowed metrics client connectivity" network_policy_metrics_allowed
wait_for "denied cross-namespace controller metrics connectivity" network_policy_metrics_denied

echo "Applying claim-consuming workload..."
kubectl apply -f tests/install-e2e/workload.yaml
wait_for "claim-consuming Pod registration" consumer_exists

kubectl port-forward -n "$SYSTEM_NAMESPACE" service/"$RELEASE_NAME"-server \
  "$SERVER_PORT:8080" > "$ARTIFACT_DIR/server-port-forward.log" 2>&1 &
SERVER_PORT_FORWARD_PID=$!
wait_for "server readiness through Service" http_ready

curl -fsS --max-time 5 "$SERVER_URL/healthz" > "$ARTIFACT_DIR/healthz.txt"
curl -fsS --max-time 5 "$SERVER_URL/readyz" > "$ARTIFACT_DIR/readyz.txt"
wait_for "namespace-qualified claim in discovery API" api_claim_ready

curl -fsS --max-time 5 "$SERVER_URL/api/summary" > "$ARTIFACT_DIR/summary.json"
jq -e '.poolsCount >= 1 and .devicesCount >= 1 and .claimsCount >= 1 and .discoveryStatus.isPartial == false' \
  "$ARTIFACT_DIR/summary.json" >/dev/null

curl -fsS --max-time 5 "$SERVER_URL/api/graph" > "$ARTIFACT_DIR/graph.json"
jq -e \
  --arg id "claim/${FIXTURE_NAMESPACE}/${CLAIM_NAME}" \
  --arg namespace "$FIXTURE_NAMESPACE" \
  '.nodes | any(.id == $id and .type == "ResourceClaim" and .metadata.namespace == $namespace)' \
  "$ARTIFACT_DIR/graph.json" >/dev/null

curl -fsS --get --max-time 5 \
  --data-urlencode "namespace=$FIXTURE_NAMESPACE" \
  --data-urlencode "claim=$CLAIM_NAME" \
  "$SERVER_URL/api/explain" > "$ARTIFACT_DIR/explain.json"
jq -e --arg claim "$CLAIM_NAME" '.targetName == $claim and .allocated == true' \
  "$ARTIFACT_DIR/explain.json" >/dev/null

curl -fsS --max-time 5 "$SERVER_URL/metrics" > "$ARTIFACT_DIR/server-metrics.txt"
grep -Eq '^draforge_claims_count [1-9][0-9]*$' "$ARTIFACT_DIR/server-metrics.txt" || \
  fail "server metrics did not report claims"
grep -Eq '^draforge_devices_count [1-9][0-9]*$' "$ARTIFACT_DIR/server-metrics.txt" || \
  fail "server metrics did not report devices"

kubectl port-forward -n "$SYSTEM_NAMESPACE" service/"$RELEASE_NAME"-controller \
  "$CONTROLLER_PORT:8082" > "$ARTIFACT_DIR/controller-port-forward.log" 2>&1 &
CONTROLLER_PORT_FORWARD_PID=$!
wait_for "controller metrics through Service" controller_metrics_ready
grep -Eq '^draforge_controller_allocations_simulated_total [1-9][0-9]*$' \
  "$ARTIFACT_DIR/controller-metrics.txt" || fail "controller metrics did not report a simulated allocation"

set +e
curl -fsSN --max-time 8 "$SERVER_URL/api/stream" > "$ARTIFACT_DIR/sse.txt"
sse_status=$?
set -e
if [[ "$sse_status" -ne 0 && "$sse_status" -ne 28 ]]; then
  fail "SSE request failed with curl status $sse_status"
fi
awk '/^data: / { sub(/^data: /, ""); print; exit }' "$ARTIFACT_DIR/sse.txt" > "$ARTIFACT_DIR/sse-graph.json"
[[ -s "$ARTIFACT_DIR/sse-graph.json" ]] || fail "SSE stream did not emit a graph event"
jq -e \
  --arg id "claim/${FIXTURE_NAMESPACE}/${CLAIM_NAME}" \
  --arg namespace "$FIXTURE_NAMESPACE" \
  '.nodes | any(.id == $id and .type == "ResourceClaim" and .metadata.namespace == $namespace)' \
  "$ARTIFACT_DIR/sse-graph.json" >/dev/null

api_claim_allocation=$(jq -c \
  --arg namespace "$FIXTURE_NAMESPACE" --arg claim "$CLAIM_NAME" \
  '.[] | select(.namespace == $namespace and .name == $claim) | .allocations[0]' \
  "$ARTIFACT_DIR/claims.json")
graph_claim_namespace=$(jq -r \
  --arg id "claim/${FIXTURE_NAMESPACE}/${CLAIM_NAME}" \
  '.nodes[] | select(.id == $id) | .metadata.namespace' \
  "$ARTIFACT_DIR/graph.json")
sse_claim_namespace=$(jq -r \
  --arg id "claim/${FIXTURE_NAMESPACE}/${CLAIM_NAME}" \
  '.nodes[] | select(.id == $id) | .metadata.namespace' \
  "$ARTIFACT_DIR/sse-graph.json")
[[ "$graph_claim_namespace" == "$FIXTURE_NAMESPACE" && "$sse_claim_namespace" == "$FIXTURE_NAMESPACE" ]] || \
  fail "API and SSE namespace identity diverged"
[[ -n "$api_claim_allocation" && "$api_claim_allocation" != "null" ]] || \
  fail "API claim allocation identity was empty"

echo "Install-level E2E verification passed."
