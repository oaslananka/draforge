#!/usr/bin/env bash
# Submits a Kubernetes Job to run tagged end-to-end tests remotely on DOKS.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

readonly REPO_URL="https://github.com/oaslananka/draforge.git"
readonly NAMESPACE="draforge-ci"
COMMIT_SHA=${1:-$(git rev-parse HEAD)}
RUN_LABEL=${REMOTE_E2E_RUN_ID:-$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')}
ARTIFACT_DIR=${REMOTE_E2E_ARTIFACT_DIR:-artifacts/remote-e2e-$RUN_LABEL}
POD_WAIT_ATTEMPTS=${REMOTE_E2E_POD_WAIT_ATTEMPTS:-60}
LOG_WAIT_ATTEMPTS=${REMOTE_E2E_LOG_WAIT_ATTEMPTS:-60}
STATUS_WAIT_ATTEMPTS=${REMOTE_E2E_STATUS_WAIT_ATTEMPTS:-60}

if [[ ! "$COMMIT_SHA" =~ ^[0-9a-fA-F]{7,40}$ ]]; then
    echo "ERROR: commit SHA must be 7-40 hexadecimal characters" >&2
    exit 2
fi
if [[ ! "$RUN_LABEL" =~ ^[A-Za-z0-9]([A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$ ]]; then
    echo "ERROR: REMOTE_E2E_RUN_ID must be a valid Kubernetes label value (1-63 characters)" >&2
    exit 2
fi

RUN_SUFFIX=$(printf "%s" "$RUN_LABEL" | tr "[:upper:]_." "[:lower:]--" | cut -c1-40)
JOB_NAME="draforge-e2e-$RUN_SUFFIX"
WORK_DIR=$(mktemp -d)
RBAC_MANIFEST="$WORK_DIR/e2e-rbac.yaml"
JOB_MANIFEST="$WORK_DIR/e2e-job.yaml"
pod_name=""
artifacts_collected=false

mkdir -p "$ARTIFACT_DIR"

render_manifest() {
    local source=$1
    local destination=$2
    sed \
        -e "s/RUN_ID/$RUN_SUFFIX/g" \
        -e "s/RUN_LABEL/$RUN_LABEL/g" \
        -e "s|REPO_URL|$REPO_URL|g" \
        -e "s/COMMIT_SHA/$COMMIT_SHA/g" \
        "$source" > "$destination"
}

collect_artifacts() {
    if [[ "$artifacts_collected" == "true" ]]; then
        return
    fi
    artifacts_collected=true
    set +e

    kubectl get job -n "$NAMESPACE" "$JOB_NAME" -o yaml > "$ARTIFACT_DIR/job.yaml" 2> "$ARTIFACT_DIR/job-get.stderr"
    if [[ -n "$pod_name" ]]; then
        kubectl get pod -n "$NAMESPACE" "$pod_name" -o yaml > "$ARTIFACT_DIR/pod.yaml" 2> "$ARTIFACT_DIR/pod-get.stderr"
        kubectl logs -n "$NAMESPACE" "$pod_name" -c clone --tail=-1 > "$ARTIFACT_DIR/clone.log" 2>&1
        kubectl logs -n "$NAMESPACE" "$pod_name" -c e2e-runner --tail=-1 > "$ARTIFACT_DIR/e2e-runner.log" 2>&1
        kubectl cp -n "$NAMESPACE" -c e2e-runner \
            "$pod_name:/artifacts/go-test.json" "$ARTIFACT_DIR/go-test.json" \
            > "$ARTIFACT_DIR/kubectl-cp.stdout" 2> "$ARTIFACT_DIR/kubectl-cp.stderr"
    fi
    kubectl get events -n "$NAMESPACE" --sort-by=.metadata.creationTimestamp \
        > "$ARTIFACT_DIR/events.txt" 2> "$ARTIFACT_DIR/events.stderr"
    set -e
}

cleanup_resources() {
    collect_artifacts
    set +e
    echo "==> Cleaning up run-scoped E2E resources..."
    kubectl delete -f "$JOB_MANIFEST" --ignore-not-found=true --wait=false
    kubectl delete -f "$RBAC_MANIFEST" --ignore-not-found=true --wait=false
    rm -rf "$WORK_DIR"
}

finish() {
    local exit_code=$1
    trap - EXIT INT TERM
    cleanup_resources
    exit "$exit_code"
}

trap 'finish "$?"' EXIT
trap 'finish 130' INT
trap 'finish 143' TERM

render_manifest build/remote/e2e-rbac.yaml "$RBAC_MANIFEST"
render_manifest build/remote/e2e-job.yaml "$JOB_MANIFEST"

if grep -Eq "RUN_ID|RUN_LABEL|REPO_URL|COMMIT_SHA" "$RBAC_MANIFEST" "$JOB_MANIFEST"; then
    echo "ERROR: rendered E2E manifests contain unresolved placeholders" >&2
    exit 2
fi

echo "==> Preparing remote E2E tests..."
echo "  - Commit SHA: $COMMIT_SHA"
echo "  - Run label: $RUN_LABEL"
echo "  - Job: $JOB_NAME"
echo "  - Artifacts: $ARTIFACT_DIR"

kubectl apply -f build/remote/namespace.yaml
kubectl apply -f build/remote/quota.yaml
kubectl apply -f "$RBAC_MANIFEST"
kubectl apply -f "$JOB_MANIFEST"

echo "==> Waiting for E2E Pod to be created..."
for ((attempt = 1; attempt <= POD_WAIT_ATTEMPTS; attempt++)); do
    pod_name=$(kubectl get pods -n "$NAMESPACE" -l "job-name=$JOB_NAME" \
        -o jsonpath="{.items[0].metadata.name}" 2>/dev/null || true)
    if [[ -n "$pod_name" ]]; then
        break
    fi
    sleep 2
done
if [[ -z "$pod_name" ]]; then
    echo "ERROR: timed out waiting for E2E Job Pod" >&2
    exit 1
fi

echo "==> Streaming checkout logs..."
clone_logs_ready=false
for ((attempt = 1; attempt <= LOG_WAIT_ATTEMPTS; attempt++)); do
    if kubectl logs -n "$NAMESPACE" "$pod_name" -c clone -f --tail=-1; then
        clone_logs_ready=true
        break
    fi
    sleep 2
done
if [[ "$clone_logs_ready" != "true" ]]; then
    echo "ERROR: unable to read checkout logs" >&2
    exit 1
fi

echo "==> Streaming E2E runner logs..."
runner_logs_ready=false
for ((attempt = 1; attempt <= LOG_WAIT_ATTEMPTS; attempt++)); do
    if kubectl logs -n "$NAMESPACE" "$pod_name" -c e2e-runner -f --tail=-1; then
        runner_logs_ready=true
        break
    fi
    sleep 2
done
if [[ "$runner_logs_ready" != "true" ]]; then
    echo "ERROR: unable to read E2E runner logs" >&2
    exit 1
fi

echo "==> Checking E2E Job completion status..."
status=""
failed=""
for ((attempt = 1; attempt <= STATUS_WAIT_ATTEMPTS; attempt++)); do
    status=$(kubectl get job -n "$NAMESPACE" "$JOB_NAME" -o jsonpath="{.status.succeeded}" 2>/dev/null || true)
    failed=$(kubectl get job -n "$NAMESPACE" "$JOB_NAME" -o jsonpath="{.status.failed}" 2>/dev/null || true)
    if [[ "$status" == "1" || ( -n "$failed" && "$failed" -gt 0 ) ]]; then
        break
    fi
    sleep 2
done

if [[ "$status" == "1" ]]; then
    echo "SUCCESS: Remote E2E tests executed and passed."
    finish 0
fi

echo "FAIL: Remote E2E tests did not complete successfully (failed=${failed:-0})." >&2
finish 1
