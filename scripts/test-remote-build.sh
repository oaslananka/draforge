#!/usr/bin/env bash
# Tests DOCR build and workload registry integration without a live cluster.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
cleanup() {
    rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    local file text
    file=$1
    text=$2
    grep -Fq -- "$text" "$file" || fail "expected $file to contain: $text"
}

cat > "$TEST_ROOT/kubectl" <<'KUBECTL'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >> "$FAKE_KUBECTL_LOG"
printf '\n' >> "$FAKE_KUBECTL_LOG"

command_name=${1:-}
shift || true
case "$command_name" in
    create)
        kind=${1:-}
        shift || true
        if [[ "$kind" == "namespace" ]]; then
            printf 'apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n' "${1:-}"
        elif [[ "$kind" == "secret" ]]; then
            namespace=default
            while (( $# > 0 )); do
                if [[ "$1" == "--namespace" ]]; then
                    namespace=$2
                    shift 2
                    continue
                fi
                shift
            done
            printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: registry-draforge\n  namespace: %s\n' "$namespace"
        else
            exit 2
        fi
        ;;
    apply)
        if [[ "${1:-}" == "-f" && "${2:-}" == "-" ]]; then
            cat >/dev/null
        elif [[ "${1:-}" == "-f" ]]; then
            manifest=${2:-}
            if grep -Fq 'kind: Job' "$manifest"; then
                cp "$manifest" "$FAKE_RENDERED_JOB"
            fi
        fi
        ;;
    get)
        resource=${1:-}
        if [[ "$resource" == "pods" ]]; then
            printf 'draforge-build-server-test-pod'
        elif [[ "$resource" == "job" ]]; then
            if printf '%s\n' "$*" | grep -Fq '.status.succeeded'; then
                printf '1'
            fi
        fi
        ;;
    logs)
        printf 'fake build log\n'
        ;;
    delete)
        ;;
    *)
        echo "unsupported fake kubectl command: $command_name" >&2
        exit 2
        ;;
esac
KUBECTL
chmod +x "$TEST_ROOT/kubectl"

: > "$TEST_ROOT/kubectl.log"
(
    cd "$ROOT_DIR"
    PATH="$TEST_ROOT:$PATH" \
    DIGITALOCEAN_TOKEN=test-token \
    FAKE_KUBECTL_LOG="$TEST_ROOT/kubectl.log" \
    FAKE_RENDERED_JOB="$TEST_ROOT/job.yaml" \
    bash scripts/remote-build.sh server latest deadbeef
) > "$TEST_ROOT/stdout.log" 2> "$TEST_ROOT/stderr.log"

assert_contains "$TEST_ROOT/kubectl.log" 'create namespace draforge-ci'
assert_contains "$TEST_ROOT/kubectl.log" 'create namespace draforge-system'
assert_contains "$TEST_ROOT/kubectl.log" '--namespace draforge-ci'
assert_contains "$TEST_ROOT/kubectl.log" '--namespace draforge-system'
assert_contains "$TEST_ROOT/kubectl.log" 'delete -f'
assert_contains "$TEST_ROOT/job.yaml" 'registry.digitalocean.com/draforge/draforge:server-latest'

set +e
(
    cd "$ROOT_DIR"
    DIGITALOCEAN_TOKEN=test-token bash scripts/remote-build.sh invalid latest deadbeef
) > "$TEST_ROOT/invalid.stdout" 2> "$TEST_ROOT/invalid.stderr"
invalid_status=$?
set -e
[[ "$invalid_status" -eq 2 ]] || fail "invalid component exit status $invalid_status, expected 2"
assert_contains "$TEST_ROOT/invalid.stderr" 'Usage:'

echo "Remote DOCR build integration tests passed."
