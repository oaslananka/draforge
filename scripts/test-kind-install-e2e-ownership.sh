#!/usr/bin/env bash
# Regression for local kind quickstart: never delete a pre-existing cluster.
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_PROJECT="$TEST_ROOT/project"
FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_PROJECT/scripts" "$FAKE_BIN"
cp "$ROOT_DIR/scripts/kind-install-e2e.sh" "$FAKE_PROJECT/scripts/"
cat > "$FAKE_PROJECT/scripts/e2e-matrix.sh" <<'SCRIPT'
#!/usr/bin/env bash
printf '%s\n' '{"include":[{"kind_version":"v0.32.0","node_image":"kindest/node:mock"}]}'
SCRIPT

for script in install-e2e-cni.sh build-install-e2e-images.sh run-install-e2e.sh collect-install-e2e-artifacts.sh; do
  cat > "$FAKE_PROJECT/scripts/$script" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'stage %s\n' "$(basename "$0")" >> "$FAKE_LOG"
if [[ "$(basename "$0")" == 'install-e2e-cni.sh' && "${FAKE_CNI_FAILURE:-0}" == 1 ]]; then
  exit 17
fi
SCRIPT
done

cat > "$FAKE_BIN/kind" <<'KIND'
#!/usr/bin/env bash
set -euo pipefail
printf 'kind %s\n' "$*" >> "$FAKE_LOG"
case "${1:-} ${2:-}" in
  'version ') printf 'kind v0.32.0\n' ;;
  'get clusters')
    if [[ "${FAKE_LIST_FAILURE:-0}" == 1 ]]; then exit 2; fi
    if [[ "${FAKE_EXISTING:-0}" == 1 ]]; then
      printf '%s\n' "${FAKE_CLUSTER_NAME:-draforge-install-e2e}"
    else
      printf 'unrelated-existing-cluster\n'
    fi
    ;;
  'create cluster')
    if [[ "${FAKE_CREATE_FAILURE:-0}" == 1 ]]; then exit 42; fi
    ;;
  'delete cluster' | 'load docker-image') ;;
  *) echo "unexpected fake kind command: $*" >&2; exit 10 ;;
esac
KIND

for tool in docker kubectl helm curl; do
  cat > "$FAKE_BIN/$tool" <<'STUB'
#!/usr/bin/env bash
printf 'other %s %s\n' "$(basename "$0")" "$*" >> "$FAKE_LOG"
if [[ "$(basename "$0")" == docker && "${1:-}" == info && "${FAKE_DOCKER_UNAVAILABLE:-0}" == 1 ]]; then
  exit 7
fi
STUB
done
chmod +x "$FAKE_BIN"/* "$FAKE_PROJECT/scripts"/*.sh

fail() { echo "FAIL: $*" >&2; exit 1; }
must_contain() {
  grep -Fq -- "$2" "$1" || { cat "$1" >&2; fail "missing $2 in $1"; }
}
must_not_contain() {
  if grep -Fq -- "$2" "$1"; then cat "$1" >&2; fail "unexpected $2 in $1"; fi
}

run_case() {
  local name=$1 expected=$2
  shift 2
  local dir="$TEST_ROOT/$name" actual
  mkdir -p "$dir"
  : > "$dir/calls.log"
  if env PATH="$FAKE_BIN:$PATH" FAKE_LOG="$dir/calls.log" \
      DRAFORGE_INSTALL_E2E_ARTIFACT_DIR="$dir/artifacts" "$@" \
      "$FAKE_PROJECT/scripts/kind-install-e2e.sh" pull-request \
      > "$dir/out.log" 2>&1; then
    actual=0
  else
    actual=$?
  fi
  [[ "$actual" == "$expected" ]] || { cat "$dir/out.log" >&2; fail "$name exited $actual (expected $expected)"; }
}

run_case docker-unavailable 1 FAKE_DOCKER_UNAVAILABLE=1
must_contain "$TEST_ROOT/docker-unavailable/out.log" 'Docker daemon unavailable from this shell'
must_contain "$TEST_ROOT/docker-unavailable/out.log" 'No kind cluster created.'
must_contain "$TEST_ROOT/docker-unavailable/calls.log" 'other docker info --format'
must_not_contain "$TEST_ROOT/docker-unavailable/calls.log" 'kind '
must_not_contain "$TEST_ROOT/docker-unavailable/calls.log" 'stage '

run_case already-exists 1 FAKE_EXISTING=1
must_contain "$TEST_ROOT/already-exists/out.log" 'already exists; refusing to replace it'
must_not_contain "$TEST_ROOT/already-exists/calls.log" 'kind delete cluster'
must_not_contain "$TEST_ROOT/already-exists/calls.log" 'kind create cluster'

run_case custom-name 1 FAKE_EXISTING=1 FAKE_CLUSTER_NAME=my-kind DRAFORGE_INSTALL_E2E_CLUSTER_NAME=my-kind
must_contain "$TEST_ROOT/custom-name/out.log" "kind cluster 'my-kind' already exists"
must_not_contain "$TEST_ROOT/custom-name/calls.log" 'kind delete cluster'

run_case cannot-list 1 FAKE_LIST_FAILURE=1
must_contain "$TEST_ROOT/cannot-list/out.log" 'cannot list kind clusters'
must_not_contain "$TEST_ROOT/cannot-list/calls.log" 'kind create cluster'
must_not_contain "$TEST_ROOT/cannot-list/calls.log" 'kind delete cluster'

run_case cannot-create 42 FAKE_CREATE_FAILURE=1
must_contain "$TEST_ROOT/cannot-create/calls.log" 'kind create cluster'
must_not_contain "$TEST_ROOT/cannot-create/calls.log" 'kind delete cluster'

run_case successful-run 0
must_contain "$TEST_ROOT/successful-run/calls.log" 'kind create cluster'
must_contain "$TEST_ROOT/successful-run/calls.log" 'stage run-install-e2e.sh'
must_contain "$TEST_ROOT/successful-run/calls.log" 'kind delete cluster --name draforge-install-e2e'

run_case keep-cluster 0 DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1
must_contain "$TEST_ROOT/keep-cluster/out.log" 'Keeping kind cluster draforge-install-e2e for inspection.'
must_not_contain "$TEST_ROOT/keep-cluster/calls.log" 'kind delete cluster'

run_case failed-cni 17 FAKE_CNI_FAILURE=1
must_contain "$TEST_ROOT/failed-cni/calls.log" 'kind delete cluster --name draforge-install-e2e'
must_not_contain "$TEST_ROOT/failed-cni/calls.log" 'stage run-install-e2e.sh'

echo 'Kind local quickstart ownership harness passed.'
