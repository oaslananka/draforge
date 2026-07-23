#!/usr/bin/env bash
# Builds the three local images exercised by install-level E2E tests.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
IMAGE_TAG=${DRAFORGE_INSTALL_E2E_IMAGE_TAG:-local}
VERSION=${DRAFORGE_INSTALL_E2E_VERSION:-e2e}
COMMIT=${DRAFORGE_INSTALL_E2E_COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)}

command -v docker >/dev/null 2>&1 || {
  echo "ERROR: docker is required to build install-level E2E images" >&2
  exit 1
}

build_image() {
  local component=$1
  local dockerfile=$2
  local image="docker.io/draforge-e2e/${component}:${IMAGE_TAG}"
  echo "Building ${image} from ${dockerfile}..."
  docker build \
    --file "$ROOT_DIR/$dockerfile" \
    --build-arg "VERSION=$VERSION" \
    --build-arg "COMMIT=$COMMIT" \
    --tag "$image" \
    "$ROOT_DIR"
}

build_image server build/package/Dockerfile.server
build_image controller build/package/Dockerfile.controller
build_image sim-driver build/package/Dockerfile.sim-driver

echo "Install-level E2E images built with tag ${IMAGE_TAG}."
