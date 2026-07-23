#!/usr/bin/env bash
# Runs the install-level E2E entry point for every target in one matrix profile.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROFILE=${1:-full}
MATRIX_JSON=$("$ROOT_DIR/scripts/e2e-matrix.sh" "$PROFILE")

while IFS= read -r node_image; do
  echo "Running install E2E profile $PROFILE with $node_image..."
  DRAFORGE_INSTALL_E2E_NODE_IMAGE="$node_image" \
  DRAFORGE_INSTALL_E2E_CLUSTER_NAME=draforge-install-e2e \
    "$ROOT_DIR/scripts/kind-install-e2e.sh" "$PROFILE"
done < <(jq -r '.include[].node_image' <<<"$MATRIX_JSON")
