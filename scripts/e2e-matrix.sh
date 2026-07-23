#!/usr/bin/env bash
# Emits the kind matrix defined by the documented Kubernetes compatibility policy.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSIONS_FILE=${DRAFORGE_E2E_VERSIONS_FILE:-$ROOT_DIR/tests/install-e2e/kubernetes-versions.json}
PROFILE=${1:-pull-request}

command -v jq >/dev/null 2>&1 || {
  echo "ERROR: jq is required to read $VERSIONS_FILE" >&2
  exit 1
}

[[ -f "$VERSIONS_FILE" ]] || {
  echo "ERROR: Kubernetes E2E version policy not found: $VERSIONS_FILE" >&2
  exit 1
}

jq -e --arg profile "$PROFILE" '.profiles[$profile] | type == "array" and length > 0' "$VERSIONS_FILE" >/dev/null || {
  echo "ERROR: unknown or empty E2E matrix profile: $PROFILE" >&2
  exit 1
}

kind_version=$(jq -er '.kindVersion | select(type == "string" and length > 0)' "$VERSIONS_FILE")
jq -c --arg profile "$PROFILE" --arg kindVersion "$kind_version" '{include: [.profiles[$profile][] | {kubernetes: .kubernetes, node_image: .nodeImage, kind_version: $kindVersion}]}' "$VERSIONS_FILE"
