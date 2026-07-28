#!/usr/bin/env bash
# Decode and validate a short-lived kubeconfig for provider-neutral remote E2E.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

DESTINATION=${1:-}
PAYLOAD=${DRAFORGE_E2E_KUBECONFIG_B64:-}

if [[ -z "$DESTINATION" ]]; then
  echo "ERROR: kubeconfig destination path is required" >&2
  exit 2
fi
if [[ "$DESTINATION" != /* ]]; then
  echo "ERROR: kubeconfig destination must be an absolute path" >&2
  exit 2
fi
if [[ -z "$PAYLOAD" ]]; then
  echo "ERROR: DRAFORGE_E2E_KUBECONFIG_B64 is not configured" >&2
  exit 1
fi
if ! command -v base64 >/dev/null 2>&1; then
  echo "ERROR: base64 is required" >&2
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "ERROR: kubectl is required" >&2
  exit 1
fi

parent_dir=$(dirname "$DESTINATION")
umask 077
mkdir -p "$parent_dir"
temporary=$(mktemp "$parent_dir/.draforge-kubeconfig.XXXXXX")

cleanup() {
  rm -f "$temporary"
}
trap cleanup EXIT INT TERM

if ! printf '%s' "$PAYLOAD" | base64 --decode > "$temporary"; then
  echo "ERROR: kubeconfig payload is not valid base64" >&2
  exit 1
fi
if [[ ! -s "$temporary" ]]; then
  echo "ERROR: decoded kubeconfig is empty" >&2
  exit 1
fi
chmod 600 "$temporary"

if ! KUBECONFIG="$temporary" kubectl config view --minify --raw >/dev/null; then
  echo "ERROR: decoded kubeconfig is not a valid Kubernetes configuration" >&2
  exit 1
fi

mv "$temporary" "$DESTINATION"
chmod 600 "$DESTINATION"
trap - EXIT INT TERM

echo "==> Prepared validated kubeconfig at $DESTINATION"
