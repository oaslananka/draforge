#!/usr/bin/env bash
# Exercises the remote E2E orchestration with a deterministic fake kubectl.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)

cleanup_test_root() {
    rm -rf "$TEST_ROOT"
}

trap cleanup_test_root EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_file() {
    [[ -f "$1" ]] || fail "expected file $1"
}

assert_contains() {
    local file=$1
    local text=$2
    if ! grep -Fq -- "$text" "$file"; then
        echo "--- $file ---" >&2
        cat "$file" >&2 || true
        local case_dir
        case_dir=$(dirname "$file")
        echo "--- $case_dir/stdout.log ---" >&2
        cat "$case_dir/stdout.log" >&2 || true
        echo "--- $case_dir/stderr.log ---" >&2
        cat "$case_dir/stderr.log" >&2 || true
        fail "expected $file to contain: $text"
    fi
}

FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_BIN"
cat > "$FAKE_BIN/kubectl" <<"KUBECTL"
#!/usr/bin/env bash
set -euo pipefail

printf "%q " "$@" >> "$FAKE_KUBECTL_LOG"
printf "\n" >> "$FAKE_KUBECTL_LOG"

command_name=${1:-}
shift || true
case "$command_name" in
    apply)
        manifest=""
        while (( $# > 0 )); do
            if [[ "$1" == "-f" ]]; then
                manifest=$2
                break
            fi
            shift
        done
        if [[ "$manifest" == */e2e-job.yaml ]]; then
            cp "$manifest" "$FAKE_STATE_DIR/rendered-job.yaml"
        elif [[ "$manifest" == */e2e-rbac.yaml ]]; then
            cp "$manifest" "$FAKE_STATE_DIR/rendered-rbac.yaml"
        fi
        ;;
    get)
        resource=${1:-}
        if [[ "$resource" == "pods" ]]; then
            printf "draforge-e2e-test-pod"
        elif [[ "$resource" == "job" ]]; then
            if printf "%s\n" "$*" | grep -Fq -- "-o yaml"; then
                printf "kind: Job\nmetadata:\n  name: draforge-e2e-test\n"
            elif printf "%s\n" "$*" | grep -Fq -- ".status.succeeded"; then
                [[ "$FAKE_E2E_RESULT" == "success" ]] && printf "1"
            elif printf "%s\n" "$*" | grep -Fq -- ".status.failed"; then
                [[ "$FAKE_E2E_RESULT" == "failure" ]] && printf "1"
            fi
        elif [[ "$resource" == "pod" ]]; then
            printf "kind: Pod\nmetadata:\n  name: draforge-e2e-test-pod\n"
        elif [[ "$resource" == "events" ]]; then
            printf "LAST SEEN TYPE REASON OBJECT MESSAGE\n"
        fi
        ;;
    logs)
        if printf "%s\n" "$*" | grep -Fq -- "e2e-runner"; then
            printf "remote E2E runner log\n"
        else
            printf "clone log\n"
        fi
        ;;
    cp)
        destination=${!#}
        mkdir -p "$(dirname "$destination")"
        cat > "$destination" <<"JSON"
{"Action":"run","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}
{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e","Test":"TestSmoke"}
{"Action":"pass","Package":"github.com/oaslananka/draforge/tests/e2e"}
JSON
        ;;
    delete)
        ;;
    *)
        echo "unsupported fake kubectl command: $command_name" >&2
        exit 2
        ;;
esac
KUBECTL
chmod +x "$FAKE_BIN/kubectl"

run_case() {
    local name=$1
    local fake_result=$2
    local expected_status=$3
    local case_dir="$TEST_ROOT/$name"
    mkdir -p "$case_dir/state" "$case_dir/artifacts"
    : > "$case_dir/kubectl.log"

    set +e
    (
        cd "$ROOT_DIR"
        PATH="$FAKE_BIN:$PATH" \
        FAKE_E2E_RESULT="$fake_result" \
        FAKE_KUBECTL_LOG="$case_dir/kubectl.log" \
        FAKE_STATE_DIR="$case_dir/state" \
        REMOTE_E2E_RUN_ID="test-$name" \
        REMOTE_E2E_ARTIFACT_DIR="$case_dir/artifacts" \
        REMOTE_E2E_POD_WAIT_ATTEMPTS=1 \
        REMOTE_E2E_LOG_WAIT_ATTEMPTS=1 \
        REMOTE_E2E_STATUS_WAIT_ATTEMPTS=1 \
        bash scripts/remote-e2e.sh deadbeef
    ) > "$case_dir/stdout.log" 2> "$case_dir/stderr.log"
    local status=$?
    set -e

    [[ "$status" -eq "$expected_status" ]] || fail "$name exit status $status, expected $expected_status"
    assert_file "$case_dir/state/rendered-job.yaml"
    assert_file "$case_dir/state/rendered-rbac.yaml"
    assert_file "$case_dir/artifacts/job.yaml"
    assert_file "$case_dir/artifacts/pod.yaml"
    assert_file "$case_dir/artifacts/go-test.json"
    assert_contains "$case_dir/state/rendered-job.yaml" "serviceAccountName: draforge-e2e-test-$name"
    assert_contains "$case_dir/state/rendered-rbac.yaml" "draforge.oaslananka/e2e-run-id: test-$name"
    assert_contains "$case_dir/kubectl.log" "delete -f"
    if grep -Eq "RUN_ID|RUN_LABEL|REPO_URL|COMMIT_SHA" \
        "$case_dir/state/rendered-job.yaml" "$case_dir/state/rendered-rbac.yaml"; then
        fail "$name rendered manifest contains unresolved placeholders"
    fi
}

run_cancel_case() {
    local case_dir="$TEST_ROOT/cancelled"
    mkdir -p "$case_dir/state" "$case_dir/artifacts"
    : > "$case_dir/kubectl.log"

    (
        cd "$ROOT_DIR"
        export PATH="$FAKE_BIN:$PATH"
        export FAKE_E2E_RESULT=hang
        export FAKE_KUBECTL_LOG="$case_dir/kubectl.log"
        export FAKE_STATE_DIR="$case_dir/state"
        export REMOTE_E2E_RUN_ID=test-cancelled
        export REMOTE_E2E_ARTIFACT_DIR="$case_dir/artifacts"
        export REMOTE_E2E_POD_WAIT_ATTEMPTS=1
        export REMOTE_E2E_LOG_WAIT_ATTEMPTS=1
        export REMOTE_E2E_STATUS_WAIT_ATTEMPTS=60
        exec bash scripts/remote-e2e.sh deadbeef
    ) > "$case_dir/stdout.log" 2> "$case_dir/stderr.log" &
    local pid=$!

    local observed_status_check=false
    for _ in $(seq 1 100); do
        if grep -Fq -- "status.failed" "$case_dir/kubectl.log"; then
            observed_status_check=true
            break
        fi
        sleep 0.05
    done
    if [[ "$observed_status_check" != "true" ]]; then
        kill -KILL "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
        fail "cancelled case never entered Job status polling"
    fi

    kill -TERM "$pid"
    set +e
    wait "$pid"
    local status=$?
    set -e

    [[ "$status" -eq 143 ]] || fail "cancelled exit status $status, expected 143"
    assert_file "$case_dir/artifacts/job.yaml"
    assert_file "$case_dir/artifacts/pod.yaml"
    assert_file "$case_dir/artifacts/go-test.json"
    assert_contains "$case_dir/kubectl.log" "delete -f"
}

run_case success success 0
run_case failure failure 1
run_cancel_case

echo "Remote E2E harness tests passed."
