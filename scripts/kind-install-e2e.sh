#!/usr/bin/env bash
# Local entry point for one install-level E2E run on kind.
# SPDX-License-Identifier: AGPL-3.0-only

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROFILE=${1:-pull-request}
CLUSTER_NAME=${DRAFORGE_INSTALL_E2E_CLUSTER_NAME:-draforge-install-e2e}
KEEP_CLUSTER=${DRAFORGE_INSTALL_E2E_KEEP_CLUSTER:-0}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}

# Check the environment before invoking any command that could create a cluster.
missing_tools=()
for tool in docker kind kubectl helm jq curl sha256sum; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    missing_tools+=("$tool")
  fi
done
# Report all missing tools *and* daemon connectivity in one pass. A Windows
# Docker Desktop daemon may be healthy while Ubuntu/WSL cannot reach it.
docker_unavailable=0
if command -v docker >/dev/null 2>&1; then
  if ! docker info --format '{{.ServerVersion}}' >/dev/null 2>&1; then
    docker_unavailable=1
  fi
fi

if (( ${#missing_tools[@]} > 0 )); then
  echo "ERROR: missing required tools: ${missing_tools[*]}." >&2
fi
if (( docker_unavailable == 1 )); then
  echo "ERROR: Docker daemon unavailable from this shell." >&2
  echo "Start Docker Engine and verify 'docker info' here. With Docker Desktop on Windows, enable the WSL 2 backend and Resources > WSL Integration for this distro (https://docs.docker.com/desktop/features/wsl/)." >&2
fi
if (( ${#missing_tools[@]} > 0 || docker_unavailable == 1 )); then
  echo "No kind cluster created." >&2
  exit 1
fi

MATRIX_JSON=$("$ROOT_DIR/scripts/e2e-matrix.sh" "$PROFILE")
KIND_VERSION=$(jq -r '.include[0].kind_version' <<<"$MATRIX_JSON")
NODE_IMAGE=${DRAFORGE_INSTALL_E2E_NODE_IMAGE:-$(jq -r '.include[0].node_image' <<<"$MATRIX_JSON")}

installed_kind=$(kind version 2>/dev/null | awk '{print $2}')
if [[ "$installed_kind" != "$KIND_VERSION" ]]; then
  echo "ERROR: kind $KIND_VERSION is required by tests/install-e2e/kubernetes-versions.json; found ${installed_kind:-unknown}" >&2
  exit 1
fi

# The documented quickstart may be rerun with KEEP_CLUSTER=1. Never remove
# an existing user's cluster, even if it has the test's default name.
if ! existing_clusters=$(kind get clusters); then
  echo "ERROR: cannot list kind clusters; refusing to create or delete anything." >&2
  exit 1
fi
if grep -Fxq -- "$CLUSTER_NAME" <<< "$existing_clusters"; then
  echo "ERROR: kind cluster '$CLUSTER_NAME' already exists; refusing to replace it." >&2
  echo "Inspect it, choose DRAFORGE_INSTALL_E2E_CLUSTER_NAME=<new-name>, or deliberately delete it with 'kind delete cluster --name $CLUSTER_NAME' before retrying." >&2
  exit 1
fi

created_cluster=0
cleanup() {
  local status=$?
  "$ROOT_DIR/scripts/collect-install-e2e-artifacts.sh" || true
  if [[ "$created_cluster" == "1" ]]; then
    if [[ "$KEEP_CLUSTER" != "1" ]]; then
      kind delete cluster --name "$CLUSTER_NAME" || true
    else
      echo "Keeping kind cluster $CLUSTER_NAME for inspection."
    fi
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

kind create cluster \
  --name "$CLUSTER_NAME" \
  --image "$NODE_IMAGE" \
  --config "$ARTIFACT_DIR/kind-config.yaml" \
  --wait 0s
created_cluster=1

"$ROOT_DIR/scripts/install-e2e-cni.sh"
"$ROOT_DIR/scripts/build-install-e2e-images.sh"
for image in server controller sim-driver; do
  kind load docker-image "docker.io/draforge-e2e/${image}:local" --name "$CLUSTER_NAME"
done

"$ROOT_DIR/scripts/run-install-e2e.sh"
