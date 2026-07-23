#!/usr/bin/env bash
# Local entry point for one install-level E2E run on kind.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROFILE=${1:-pull-request}
CLUSTER_NAME=${DRAFORGE_INSTALL_E2E_CLUSTER_NAME:-draforge-install-e2e}
KEEP_CLUSTER=${DRAFORGE_INSTALL_E2E_KEEP_CLUSTER:-0}
MATRIX_JSON=$("$ROOT_DIR/scripts/e2e-matrix.sh" "$PROFILE")
KIND_VERSION=$(jq -r '.include[0].kind_version' <<<"$MATRIX_JSON")
NODE_IMAGE=${DRAFORGE_INSTALL_E2E_NODE_IMAGE:-$(jq -r '.include[0].node_image' <<<"$MATRIX_JSON")}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}

for tool in docker kind kubectl helm jq curl; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "ERROR: missing required tool: $tool" >&2
    exit 1
  }
done

installed_kind=$(kind version 2>/dev/null | awk '{print $2}')
if [[ "$installed_kind" != "$KIND_VERSION" ]]; then
  echo "ERROR: kind $KIND_VERSION is required by tests/install-e2e/kubernetes-versions.json; found ${installed_kind:-unknown}" >&2
  exit 1
fi

cleanup() {
  local status=$?
  "$ROOT_DIR/scripts/collect-install-e2e-artifacts.sh" || true
  if [[ "$KEEP_CLUSTER" != "1" ]]; then
    kind delete cluster --name "$CLUSTER_NAME" || true
  else
    echo "Keeping kind cluster $CLUSTER_NAME for inspection."
  fi
  return "$status"
}
trap cleanup EXIT

rm -rf "$ARTIFACT_DIR"
mkdir -p "$ARTIFACT_DIR"

cat > "$ARTIFACT_DIR/kind-config.yaml" <<'CONFIG'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
featureGates:
  DynamicResourceAllocation: true
networking:
  disableDefaultCNI: true
  podSubnet: 192.168.0.0/16
CONFIG

kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
kind create cluster \
  --name "$CLUSTER_NAME" \
  --image "$NODE_IMAGE" \
  --config "$ARTIFACT_DIR/kind-config.yaml" \
  --wait 0s

"$ROOT_DIR/scripts/install-e2e-cni.sh"
"$ROOT_DIR/scripts/build-install-e2e-images.sh"
for image in server controller sim-driver; do
  kind load docker-image "docker.io/draforge-e2e/${image}:local" --name "$CLUSTER_NAME"
done

"$ROOT_DIR/scripts/run-install-e2e.sh"
