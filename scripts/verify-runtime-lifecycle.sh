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

custom_manifest="$work_dir/custom.yaml"
render "$custom_manifest" \
    --set-string server.lifecycle.readinessTimeout=750ms \
    --set-string server.lifecycle.readinessGracePeriod=45s \
    --set-string server.lifecycle.shutdownTimeout=12s \
    --set-string controller.lifecycle.readinessTimeout=1s \
    --set-string controller.lifecycle.readinessGracePeriod=0 \
    --set-string controller.lifecycle.shutdownTimeout=20s
for argument in \
    '            - --readiness-timeout=750ms' \
    '            - --readiness-grace-period=45s' \
    '            - --shutdown-timeout=12s' \
    '            - --readiness-timeout=1s' \
    '            - --readiness-grace-period=0' \
    '            - --shutdown-timeout=20s'; do
    assert_count "$custom_manifest" "$argument" 1
done

for invalid in \
    'server.lifecycle.readinessTimeout=0' \
    'server.lifecycle.readinessGracePeriod=invalid' \
    'server.lifecycle.shutdownTimeout=-1s' \
    'controller.lifecycle.readinessTimeout=invalid' \
    'controller.lifecycle.readinessGracePeriod=-1s' \
    'controller.lifecycle.shutdownTimeout=0'; do
    if "$HELM_BIN" template draforge "$CHART_DIR" --set-string "$invalid" >/dev/null 2>&1; then
        fail "invalid lifecycle value unexpectedly passed schema validation: $invalid"
    fi
done

echo "Runtime lifecycle Helm contract verified."
