#!/usr/bin/env bash
# Installs the digest-verified NetworkPolicy-capable CNI used by install E2E.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSIONS_FILE=${DRAFORGE_E2E_VERSIONS_FILE:-$ROOT_DIR/tests/install-e2e/kubernetes-versions.json}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}
CNI_TIMEOUT=${DRAFORGE_INSTALL_E2E_CNI_TIMEOUT:-7m}

for tool in jq curl sha256sum kubectl; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "ERROR: missing required tool: $tool" >&2
    exit 1
  }
done

mkdir -p "$ARTIFACT_DIR"
provider=$(jq -er '.networkPolicyProvider.name' "$VERSIONS_FILE")
version=$(jq -er '.networkPolicyProvider.version' "$VERSIONS_FILE")
manifest_url=$(jq -er '.networkPolicyProvider.manifestUrl' "$VERSIONS_FILE")
manifest_sha=$(jq -er '.networkPolicyProvider.sha256' "$VERSIONS_FILE")
manifest_file="$ARTIFACT_DIR/${provider}-${version}.yaml"

[[ "$provider" == "calico" ]] || {
  echo "ERROR: unsupported install E2E NetworkPolicy provider: $provider" >&2
  exit 1
}
[[ "$manifest_url" == https://raw.githubusercontent.com/projectcalico/calico/*/manifests/calico.yaml ]] || {
  echo "ERROR: unexpected Calico manifest URL: $manifest_url" >&2
  exit 1
}
[[ "$manifest_sha" =~ ^[0-9a-f]{64}$ ]] || {
  echo "ERROR: invalid Calico manifest SHA-256: $manifest_sha" >&2
  exit 1
}

echo "Downloading $provider $version manifest..."
curl --proto '=https' --tlsv1.2 -fsSL "$manifest_url" -o "$manifest_file"
printf '%s  %s\n' "$manifest_sha" "$manifest_file" | sha256sum --check --strict

kubectl create -f "$manifest_file"
kubectl rollout status daemonset/calico-node -n kube-system --timeout="$CNI_TIMEOUT"
kubectl rollout status deployment/calico-kube-controllers -n kube-system --timeout="$CNI_TIMEOUT"
kubectl wait nodes --all --for=condition=Ready --timeout="$CNI_TIMEOUT"

kubectl get pods -n kube-system -l k8s-app=calico-node -o wide
kubectl get deployment/calico-kube-controllers -n kube-system

echo "$provider $version installed and all kind nodes are Ready."
