#!/usr/bin/env bash
# scripts/remote-test.sh
# Submits a Kubernetes Job to run Go unit and race tests remotely on DOKS.

set -euo pipefail

COMMIT_SHA=${1:-$(git rev-parse HEAD)}

REPO_URL="https://github.com/oaslananka/draforge.git"
RUN_ID=$(head /dev/urandom | tr -dc a-z0-9 | head -c 6 ; echo '')
JOB_NAME="draforge-test-$RUN_ID"

echo "==> Preparing remote unit tests..."
echo "  - Commit SHA: $COMMIT_SHA"
echo "  - Run ID: $RUN_ID"

# 1. Ensure namespace and quotas exist
kubectl apply -f build/remote/namespace.yaml
kubectl apply -f build/remote/quota.yaml

# 2. Generate the Job manifest
MANIFEST_TEMP=$(mktemp)
cat build/remote/test-job.yaml \
    | sed "s/RUN_ID/$RUN_ID/g" \
    | sed "s|REPO_URL|$REPO_URL|g" \
    | sed "s|COMMIT_SHA|$COMMIT_SHA|g" \
    > "$MANIFEST_TEMP"

# 3. Apply the Job
kubectl apply -f "$MANIFEST_TEMP"

# Helper function for cleanup
cleanup() {
    echo "==> Cleaning up test Job..."
    kubectl delete -f "$MANIFEST_TEMP" --ignore-not-found=true
    rm -f "$MANIFEST_TEMP"
}
trap cleanup EXIT

# 4. Wait for the pod to start
echo "==> Waiting for test Pod to start..."
pod_name=""
for i in {1..30}; do
    pod_name=$(kubectl get pods -n draforge-ci -l job-name="$JOB_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -n "$pod_name" ]; then
        break
    fi
    sleep 2
done

if [ -z "$pod_name" ]; then
    echo "ERROR: Timeout waiting for Job Pod to be created" >&2
    exit 1
fi

# 5. Stream clone logs
echo "==> Streaming checkout logs..."
for i in {1..30}; do
    if kubectl logs -n draforge-ci "$pod_name" -c clone -f --tail=-1 2>/dev/null; then
        break
    fi
    sleep 2
done

# 6. Stream test runner logs
echo "==> Streaming test runner logs..."
for i in {1..30}; do
    if kubectl logs -n draforge-ci "$pod_name" -c test-runner -f --tail=-1 2>/dev/null; then
        break
    fi
    sleep 2
done

# 7. Check final status of the Job
echo "==> Checking test completion status..."
status=""
for i in {1..10}; do
    status=$(kubectl get job -n draforge-ci "$JOB_NAME" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)
    if [ "$status" = "1" ]; then
        break
    fi
    failed=$(kubectl get job -n draforge-ci "$JOB_NAME" -o jsonpath='{.status.failed}' 2>/dev/null || true)
    if [ -n "$failed" ] && [ "$failed" -gt 0 ]; then
        break
    fi
    sleep 1
done

if [ "$status" = "1" ]; then
    echo "SUCCESS: Remote tests passed!"
    exit 0
else
    echo "FAIL: Remote tests failed. Check logs above." >&2
    exit 1
fi
