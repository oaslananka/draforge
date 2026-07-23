#!/usr/bin/env bash
set -euo pipefail

HELM_BIN=${HELM_BIN:-helm}
CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

render() {
  local output=$1
  shift
  "$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system "$@" >"$output"
}

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_not_contains() {
  local file=$1
  local text=$2
  if grep -Fq -- "$text" "$file"; then
    fail "$file unexpectedly contains: $text"
  fi
}

expect_invalid() {
  local name=$1
  shift
  if "$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system "$@" >"$work_dir/$name.out" 2>"$work_dir/$name.err"; then
    fail "$name unexpectedly rendered"
  fi
}

default_manifest="$work_dir/default.yaml"
render "$default_manifest"
assert_contains "$default_manifest" '            - --output-mode=demo'
assert_contains "$default_manifest" '            - --health-addr=:8083'
assert_contains "$default_manifest" '            - --refresh-interval=3s'
assert_contains "$default_manifest" '              containerPort: 8083'
assert_contains "$default_manifest" '              path: /healthz'
assert_contains "$default_manifest" '              path: /readyz'
assert_contains "$default_manifest" '          emptyDir: {}'
assert_not_contains "$default_manifest" '            path: /var/lib/kubelet/device-plugins/cdi'

node_manifest="$work_dir/node.yaml"
render "$node_manifest" --set nodePlugin.outputMode=node
assert_contains "$node_manifest" '            - --output-mode=node'
assert_contains "$node_manifest" '          runAsNonRoot: false'
assert_contains "$node_manifest" '          runAsUser: 0'
assert_contains "$node_manifest" '            path: /var/lib/kubelet/device-plugins/cdi'
assert_contains "$node_manifest" '            type: DirectoryOrCreate'
assert_not_contains "$node_manifest" '          emptyDir: {}'
assert_contains "$node_manifest" '            readOnlyRootFilesystem: true'
assert_contains "$node_manifest" '            allowPrivilegeEscalation: false'

custom_manifest="$work_dir/custom.yaml"
render "$custom_manifest" \
  --set nodePlugin.refreshInterval=7s \
  --set nodePlugin.health.port=9093
assert_contains "$custom_manifest" '            - --health-addr=:9093'
assert_contains "$custom_manifest" '            - --refresh-interval=7s'
assert_contains "$custom_manifest" '              containerPort: 9093'

expect_invalid invalid-mode --set nodePlugin.outputMode=invalid
expect_invalid zero-refresh --set nodePlugin.refreshInterval=0s
expect_invalid invalid-health-port --set nodePlugin.health.port=70000

echo "Sim-driver CDI Helm contract verified."
