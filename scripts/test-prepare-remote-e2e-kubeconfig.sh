#!/usr/bin/env bash
# Tests secure preparation of a provider-neutral remote E2E kubeconfig.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_BIN"
cat > "$FAKE_BIN/kubectl" <<'KUBECTL'
#!/usr/bin/env bash
set -euo pipefail

printf '%q ' "$@" >> "$FAKE_KUBECTL_LOG"
printf '\n' >> "$FAKE_KUBECTL_LOG"

if [[ "${1:-}" == "config" && "${2:-}" == "view" ]]; then
  [[ -n "${KUBECONFIG:-}" && -s "$KUBECONFIG" ]]
  grep -Fq 'apiVersion: v1' "$KUBECONFIG"
  exit 0
fi

echo "unsupported fake kubectl command: $*" >&2
exit 2
KUBECTL
chmod +x "$FAKE_BIN/kubectl"

valid_config="$TEST_ROOT/source-kubeconfig.yaml"
cat > "$valid_config" <<'YAML'
apiVersion: v1
kind: Config
current-context: test
clusters:
  - name: test
    cluster:
      server: https://127.0.0.1:6443
contexts:
  - name: test
    context:
      cluster: test
      user: test
users:
  - name: test
    user:
      token: redacted-test-token
YAML

payload=$(base64 < "$valid_config" | tr -d '\n')
destination="$TEST_ROOT/output/kubeconfig"
: > "$TEST_ROOT/kubectl.log"

PATH="$FAKE_BIN:$PATH" \
FAKE_KUBECTL_LOG="$TEST_ROOT/kubectl.log" \
DRAFORGE_E2E_KUBECONFIG_B64="$payload" \
  bash "$ROOT_DIR/scripts/prepare-remote-e2e-kubeconfig.sh" "$destination"

[[ -f "$destination" ]] || fail "valid kubeconfig was not written"
cmp -s "$valid_config" "$destination" || fail "written kubeconfig differs from decoded input"
[[ "$(stat -c '%a' "$destination")" == "600" ]] || fail "kubeconfig mode is not 600"
grep -Fq 'config view --minify --raw' "$TEST_ROOT/kubectl.log" || fail "kubectl validation was not executed"

missing_destination="$TEST_ROOT/missing/kubeconfig"
set +e
PATH="$FAKE_BIN:$PATH" FAKE_KUBECTL_LOG="$TEST_ROOT/kubectl.log" \
  bash "$ROOT_DIR/scripts/prepare-remote-e2e-kubeconfig.sh" "$missing_destination" \
  > "$TEST_ROOT/missing.stdout" 2> "$TEST_ROOT/missing.stderr"
missing_status=$?
set -e
[[ "$missing_status" -ne 0 ]] || fail "missing payload unexpectedly passed"
[[ ! -e "$missing_destination" ]] || fail "missing payload left a destination file"

garbage_destination="$TEST_ROOT/garbage/kubeconfig"
set +e
PATH="$FAKE_BIN:$PATH" \
FAKE_KUBECTL_LOG="$TEST_ROOT/kubectl.log" \
DRAFORGE_E2E_KUBECONFIG_B64='not-valid-base64!' \
  bash "$ROOT_DIR/scripts/prepare-remote-e2e-kubeconfig.sh" "$garbage_destination" \
  > "$TEST_ROOT/garbage.stdout" 2> "$TEST_ROOT/garbage.stderr"
garbage_status=$?
set -e
[[ "$garbage_status" -ne 0 ]] || fail "invalid base64 unexpectedly passed"
[[ ! -e "$garbage_destination" ]] || fail "invalid base64 left a destination file"

relative_destination="$TEST_ROOT/relative.stdout"
set +e
PATH="$FAKE_BIN:$PATH" \
FAKE_KUBECTL_LOG="$TEST_ROOT/kubectl.log" \
DRAFORGE_E2E_KUBECONFIG_B64="$payload" \
  bash "$ROOT_DIR/scripts/prepare-remote-e2e-kubeconfig.sh" relative-kubeconfig \
  > "$relative_destination" 2> "$TEST_ROOT/relative.stderr"
relative_status=$?
set -e
[[ "$relative_status" -ne 0 ]] || fail "relative destination unexpectedly passed"
[[ ! -e "$ROOT_DIR/relative-kubeconfig" ]] || fail "relative destination wrote into the repository"

echo "Remote E2E kubeconfig preparation tests passed."
