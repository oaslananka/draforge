#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TF_DIR="$ROOT_DIR/infra/terraform/environments/showcase"
RUN_PLAN=false

usage() {
  echo "Usage: scripts/validate-terraform-showcase.sh [--plan]"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --plan) RUN_PLAN=true ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

require_tool() {
  tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing required tool: $tool" >&2
    exit 1
  fi
}

require_tool terraform
require_tool python3

if [ "$RUN_PLAN" = "true" ]; then
  require_tool doctl
  require_tool jq
  if [ -z "${DIGITALOCEAN_TOKEN:-}" ]; then
    echo "DIGITALOCEAN_TOKEN must be set before running --plan." >&2
    exit 1
  fi
fi

echo "==> Terraform showcase preflight passed"
python3 "$ROOT_DIR/scripts/validate-terraform-variables.py" --self-test "$TF_DIR"
terraform -chdir="$TF_DIR" fmt -check
terraform -chdir="$TF_DIR" init -backend=false
terraform -chdir="$TF_DIR" validate

if [ "$RUN_PLAN" = "true" ]; then
  terraform -chdir="$TF_DIR" plan -out=tfplan
  terraform -chdir="$TF_DIR" show -json tfplan > "$TF_DIR/tfplan.json"
  python3 "$ROOT_DIR/scripts/validate-plan.py" "$TF_DIR/tfplan.json"
fi
