#!/usr/bin/env bash
# Tests multi-architecture verification without contacting a registry.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
cleanup() {
    rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

cat > "$TEST_ROOT/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
command_group="${1:-} ${2:-}"
image="${3:-}"
[[ "$command_group" == "manifest inspect" ]] || exit 2
if [[ "${FAKE_MANIFEST_MODE:-complete}" == "complete" ]]; then
    cat <<OUT
{
  "schemaVersion": 2,
  "manifests": [
    {"platform": {"architecture": "amd64", "os": "linux"}},
    {"platform": {"architecture": "arm64", "os": "linux"}},
    {"platform": {"architecture": "unknown", "os": "unknown"}}
  ],
  "name": "$image"
}
OUT
else
    cat <<OUT
{
  "schemaVersion": 2,
  "manifests": [
    {"platform": {"architecture": "amd64", "os": "linux"}}
  ],
  "name": "$image"
}
OUT
fi
DOCKER
chmod +x "$TEST_ROOT/docker"

cd "$ROOT_DIR"
version=$(awk '$1 == "appVersion:" {gsub(/"/, "", $2); print $2}' deploy/helm/draforge/Chart.yaml)
DOCKER_BIN="$TEST_ROOT/docker" VERIFY_REMOTE_IMAGES=1 \
    scripts/verify-chart-images.sh "$version"

set +e
scripts/verify-chart-images.sh 9.9.9 \
    > "$TEST_ROOT/version.stdout" 2> "$TEST_ROOT/version.stderr"
version_status=$?
set -e
[[ "$version_status" -ne 0 ]] || {
    echo "FAIL: release/chart version mismatch was accepted" >&2
    exit 1
}
grep -Fq 'does not match chart appVersion' "$TEST_ROOT/version.stderr"

set +e
FAKE_MANIFEST_MODE=missing-arm64 DOCKER_BIN="$TEST_ROOT/docker" VERIFY_REMOTE_IMAGES=1 \
    scripts/verify-chart-images.sh "$version" \
    > "$TEST_ROOT/failure.stdout" 2> "$TEST_ROOT/failure.stderr"
status=$?
set -e
[[ "$status" -ne 0 ]] || {
    echo "FAIL: missing arm64 manifest was accepted" >&2
    exit 1
}
grep -Fq 'missing linux/arm64' "$TEST_ROOT/failure.stderr"

echo "Chart image verifier tests passed."
