#!/usr/bin/env bash
# Verifies Helm image references and, optionally, published multi-arch manifests.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
HELM_BIN=${HELM_BIN:-helm}
DOCKER_BIN=${DOCKER_BIN:-docker}
VERIFY_REMOTE_IMAGES=${VERIFY_REMOTE_IMAGES:-0}
EXPECTED_VERSION=""
if (( $# > 0 )); then
    EXPECTED_VERSION=$1
fi

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

chart_value() {
    local key
    key=$1
    awk -v key="$key" '$1 == key ":" {gsub(/"/, "", $2); print $2; exit}' "$CHART_DIR/Chart.yaml"
}

assert_image_set() {
    local manifest
    manifest=$1
    shift
    local expected_file actual_file
    expected_file=$(mktemp)
    actual_file=$(mktemp)
    printf "%s\n" "$@" | sort > "$expected_file"
    awk '$1 == "image:" {gsub(/"/, "", $2); print $2}' "$manifest" | sort > "$actual_file"
    if ! diff -u "$expected_file" "$actual_file"; then
        rm -f "$expected_file" "$actual_file"
        fail "rendered image references do not match the expected set"
    fi
    rm -f "$expected_file" "$actual_file"
}

chart_version=$(chart_value version)
app_version=$(chart_value appVersion)
[[ -n "$chart_version" && -n "$app_version" ]] || fail "Chart.yaml must define version and appVersion"
[[ "$chart_version" == "$app_version" ]] || fail "chart version $chart_version does not match appVersion $app_version"

if [[ -z "$EXPECTED_VERSION" ]]; then
    EXPECTED_VERSION=$app_version
fi
[[ "$EXPECTED_VERSION" == "$app_version" ]] || fail "expected release $EXPECTED_VERSION does not match chart appVersion $app_version"

work_dir=$(mktemp -d)
cleanup() {
    rm -rf "$work_dir"
}
trap cleanup EXIT

public_manifest="$work_dir/public.yaml"
"$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system > "$public_manifest"
assert_image_set "$public_manifest" \
    "ghcr.io/oaslananka/draforge-controller:$EXPECTED_VERSION" \
    "ghcr.io/oaslananka/draforge-server:$EXPECTED_VERSION" \
    "ghcr.io/oaslananka/draforge-sim-driver:$EXPECTED_VERSION"
if grep -q '^[[:space:]]*imagePullSecrets:' "$public_manifest"; then
    fail "public chart defaults must not render imagePullSecrets"
fi

digest_a="sha256:$(printf 'a%.0s' {1..64})"
digest_b="sha256:$(printf 'b%.0s' {1..64})"
digest_c="sha256:$(printf 'c%.0s' {1..64})"
digest_manifest="$work_dir/digest.yaml"
"$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system \
    --set-string "server.image.digest=$digest_a" \
    --set-string "controller.image.digest=$digest_b" \
    --set-string "nodePlugin.image.digest=$digest_c" > "$digest_manifest"
assert_image_set "$digest_manifest" \
    "ghcr.io/oaslananka/draforge-controller@$digest_b" \
    "ghcr.io/oaslananka/draforge-server@$digest_a" \
    "ghcr.io/oaslananka/draforge-sim-driver@$digest_c"

if "$HELM_BIN" template draforge "$CHART_DIR" --set-string server.image.digest=invalid >/dev/null 2>&1; then
    fail "invalid image digest unexpectedly passed chart schema validation"
fi

showcase_manifest="$work_dir/showcase.yaml"
"$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system \
    -f "$CHART_DIR/values-showcase-docr.yaml" > "$showcase_manifest"
assert_image_set "$showcase_manifest" \
    "registry.digitalocean.com/draforge/draforge:controller-latest" \
    "registry.digitalocean.com/draforge/draforge:server-latest" \
    "registry.digitalocean.com/draforge/draforge:sim-driver-latest"
secret_count=$(grep -c 'name: registry-draforge' "$showcase_manifest" || true)
[[ "$secret_count" -eq 3 ]] || fail "showcase override must render registry-draforge for all three workloads"

if [[ "$VERIFY_REMOTE_IMAGES" == "1" ]]; then
    for component in server controller sim-driver; do
        image="ghcr.io/oaslananka/draforge-$component:$EXPECTED_VERSION"
        inspect_file="$work_dir/${component}.inspect"
        "$DOCKER_BIN" buildx imagetools inspect "$image" > "$inspect_file"
        grep -Fq 'Platform: linux/amd64' "$inspect_file" || fail "$image is missing linux/amd64"
        grep -Fq 'Platform: linux/arm64' "$inspect_file" || fail "$image is missing linux/arm64"
    done
fi

echo "Chart image contract verified for version $EXPECTED_VERSION."
