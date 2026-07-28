#!/usr/bin/env bash
# Verifies readiness and graceful shutdown lifecycle values rendered by Helm.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
HELM_BIN=${HELM_BIN:-helm}

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

render() {
    local output
    output=$1
    shift
    "$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system "$@" > "$output"
}

assert_count() {
    local file text want got
    file=$1
    text=$2
    want=$3
    got=$(grep -Fxc -- "$text" "$file" || true)
    [[ "$got" -eq "$want" ]] || fail "expected $want occurrences of '$text', got $got"
}

assert_contains() {
    local file text
    file=$1
    text=$2
    grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_not_contains() {
    local file text
    file=$1
    text=$2
    if grep -Fq -- "$text" "$file"; then
        fail "$file unexpectedly contains: $text"
    fi
}

work_dir=$(mktemp -d)
cleanup() {
    rm -rf "$work_dir"
}
trap cleanup EXIT

default_manifest="$work_dir/default.yaml"
render "$default_manifest"
for argument in \
    '            - --readiness-timeout=2s' \
    '            - --readiness-grace-period=15s' \
    '            - --shutdown-timeout=5s'; do
    assert_count "$default_manifest" "$argument" 2
done
for argument in \
    '            - --leader-elect=true' \
    '            - --leader-election-lease-name=draforge-controller' \
    '            - --leader-election-lease-duration=15s' \
    '            - --leader-election-renew-deadline=10s' \
    '            - --leader-election-retry-period=2s'; do
    assert_count "$default_manifest" "$argument" 1
done
for contract in \
    '  name: draforge-controller-leader-election' \
    '  - apiGroups: ["coordination.k8s.io"]' \
    '    resources: ["leases"]' \
    '    verbs: ["get", "create", "update"]' \
    '            - name: POD_NAME' \
    '                  fieldPath: metadata.name' \
    '            - name: POD_NAMESPACE' \
    '                  fieldPath: metadata.namespace'; do
    assert_contains "$default_manifest" "$contract"
done

custom_manifest="$work_dir/custom.yaml"
render "$custom_manifest" \
    --set-string server.lifecycle.readinessTimeout=750ms \
    --set-string server.lifecycle.readinessGracePeriod=45s \
    --set-string server.lifecycle.shutdownTimeout=12s \
    --set-string controller.lifecycle.readinessTimeout=1s \
    --set-string controller.lifecycle.readinessGracePeriod=0 \
    --set-string controller.lifecycle.shutdownTimeout=20s \
    --set controller.leaderElection.enabled=false \
    --set-string controller.leaderElection.leaseDuration=30s \
    --set-string controller.leaderElection.renewDeadline=20s \
    --set-string controller.leaderElection.retryPeriod=4s
assert_not_contains "$custom_manifest" 'draforge-controller-leader-election'

for argument in \
    '            - --readiness-timeout=750ms' \
    '            - --readiness-grace-period=45s' \
    '            - --shutdown-timeout=12s' \
    '            - --readiness-timeout=1s' \
    '            - --readiness-grace-period=0' \
    '            - --shutdown-timeout=20s' \
    '            - --leader-elect=false' \
    '            - --leader-election-lease-duration=30s' \
    '            - --leader-election-renew-deadline=20s' \
    '            - --leader-election-retry-period=4s'; do
    assert_count "$custom_manifest" "$argument" 1
done

for invalid in \
    'server.lifecycle.readinessTimeout=0' \
    'server.lifecycle.readinessGracePeriod=invalid' \
    'server.lifecycle.shutdownTimeout=-1s' \
    'controller.lifecycle.readinessTimeout=invalid' \
    'controller.lifecycle.readinessGracePeriod=-1s' \
    'controller.lifecycle.shutdownTimeout=0' \
    'controller.leaderElection.leaseDuration=0' \
    'controller.leaderElection.leaseDuration=500ms' \
    'controller.leaderElection.renewDeadline=invalid' \
    'controller.leaderElection.retryPeriod=-1s'; do
    if "$HELM_BIN" template draforge "$CHART_DIR" --set-string "$invalid" >/dev/null 2>&1; then
        fail "invalid lifecycle value unexpectedly passed schema validation: $invalid"
    fi
done

if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.replicaCount=2 \
    --set controller.leaderElection.enabled=false >/dev/null 2>&1; then
    fail "multiple controller replicas unexpectedly rendered with leader election disabled"
fi

ha_manifest="$work_dir/ha.yaml"
render "$ha_manifest" --set controller.replicaCount=2
assert_contains "$ha_manifest" '  replicas: 2'
assert_contains "$ha_manifest" '            - --leader-elect=true'

echo "Runtime lifecycle Helm contract verified."
