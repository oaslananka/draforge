#!/usr/bin/env bash
# Deterministic tests for the hash-verified install E2E CNI installer.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_BIN"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cat > "$TEST_ROOT/calico.yaml" <<'MANIFEST'
apiVersion: v1
kind: ConfigMap
metadata:
  name: deterministic-calico-fixture
  namespace: kube-system
data:
  policy: enforced
MANIFEST
manifest_sha=$(sha256sum "$TEST_ROOT/calico.yaml" | awk '{print $1}')

write_policy() {
  local path=$1
  local sha=$2
  jq -n \
    --arg sha "$sha" \
    '{
      kindVersion: "v0.32.0",
      networkPolicyProvider: {
        name: "calico",
        version: "v3.32.1",
        manifestUrl: "https://raw.githubusercontent.com/projectcalico/calico/v3.32.1/manifests/calico.yaml",
        sha256: $sha
      },
      profiles: {
        "pull-request": [{kubernetes: "v1.35.5", nodeImage: "kindest/node:v1.35.5@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],
        full: [{kubernetes: "v1.35.5", nodeImage: "kindest/node:v1.35.5@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]
      }
    }' > "$path"
}

cat > "$FAKE_BIN/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
output=""
while (( $# > 0 )); do
  case "$1" in
    -o)
      output=$2
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "$output" ]]
cp "$FAKE_CNI_MANIFEST" "$output"
CURL

cat > "$FAKE_BIN/kubectl" <<'KUBECTL'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl' >> "$FAKE_COMMAND_LOG"
printf ' %q' "$@" >> "$FAKE_COMMAND_LOG"
printf '\n' >> "$FAKE_COMMAND_LOG"
case "${1:-}" in
  create|rollout|wait|get) exit 0 ;;
  *) exit 2 ;;
esac
KUBECTL

chmod +x "$FAKE_BIN/curl" "$FAKE_BIN/kubectl"

run_success() {
  local case_dir="$TEST_ROOT/success"
  mkdir -p "$case_dir/artifacts"
  : > "$case_dir/commands.log"
  write_policy "$case_dir/policy.json" "$manifest_sha"

  PATH="$FAKE_BIN:$PATH" \
  FAKE_CNI_MANIFEST="$TEST_ROOT/calico.yaml" \
  FAKE_COMMAND_LOG="$case_dir/commands.log" \
  DRAFORGE_E2E_VERSIONS_FILE="$case_dir/policy.json" \
  DRAFORGE_INSTALL_E2E_ARTIFACT_DIR="$case_dir/artifacts" \
    "$ROOT_DIR/scripts/install-e2e-cni.sh" > "$case_dir/stdout.log"

  grep -Fq 'kubectl create -f' "$case_dir/commands.log" || fail "CNI installer did not create the verified manifest"
  grep -Fq 'kubectl rollout status daemonset/calico-node' "$case_dir/commands.log" || fail "CNI installer did not wait for calico-node"
  grep -Fq 'kubectl rollout status deployment/calico-kube-controllers' "$case_dir/commands.log" || fail "CNI installer did not wait for controllers"
  grep -Fq 'kubectl wait nodes --all --for=condition=Ready' "$case_dir/commands.log" || fail "CNI installer did not wait for Ready nodes"
  grep -Fq 'installed and all kind nodes are Ready' "$case_dir/stdout.log" || fail "CNI installer success summary missing"
}

run_bad_hash() {
  local case_dir="$TEST_ROOT/bad-hash"
  mkdir -p "$case_dir/artifacts"
  : > "$case_dir/commands.log"
  write_policy "$case_dir/policy.json" 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'

  if PATH="$FAKE_BIN:$PATH" \
    FAKE_CNI_MANIFEST="$TEST_ROOT/calico.yaml" \
    FAKE_COMMAND_LOG="$case_dir/commands.log" \
    DRAFORGE_E2E_VERSIONS_FILE="$case_dir/policy.json" \
    DRAFORGE_INSTALL_E2E_ARTIFACT_DIR="$case_dir/artifacts" \
      "$ROOT_DIR/scripts/install-e2e-cni.sh" > "$case_dir/stdout.log" 2> "$case_dir/stderr.log"; then
    fail "CNI installer accepted a mismatched manifest hash"
  fi
  [[ ! -s "$case_dir/commands.log" ]] || fail "CNI installer contacted Kubernetes after hash verification failed"
}

run_success
run_bad_hash

echo "Install-level E2E CNI installer tests passed."
