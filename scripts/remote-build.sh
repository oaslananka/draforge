#!/usr/bin/env bash
# scripts/remote-build.sh
# Submits a Kubernetes Job to build a component container image remotely on DOKS using Kaniko.

set -euo pipefail

COMPONENT=${1:-""}
TAG=${2:-"latest"}
COMMIT_SHA=${3:-$(git rev-parse HEAD)}

if [ -z "$COMPONENT" ]; then
    echo "Usage: $0 <server|controller|sim-driver> [tag] [commit-sha]" >&2
    exit 1
fi

REPO_URL="https://github.com/oaslananka/draforge.git"
REGISTRY_URL="registry.digitalocean.com/draforge"
IMAGE_NAME="draforge-$COMPONENT"
RUN_ID=$(head /dev/urandom | tr -dc a-z0-9 | head -c 6 ; echo '')
JOB_NAME="draforge-build-$COMPONENT-$RUN_ID"

echo "==> Preparing remote container build for $COMPONENT..."
echo "  - Component: $COMPONENT"
echo "  - Commit SHA: $COMMIT_SHA"
echo "  - Destination Image: $REGISTRY_URL/$IMAGE_NAME:$TAG"
echo "  - Run ID: $RUN_ID"

# 1. Create remote CI resources if they do not exist
kubectl apply -f build/remote/namespace.yaml
kubectl apply -f build/remote/quota.yaml

# 2. Re-create the registry credentials secret in the namespace
doctl registry kubernetes-manifest --namespace draforge-ci | kubectl apply -f -

# 3. Generate the Job manifest
MANIFEST_TEMP=$(mktemp)
cat build/remote/kaniko-job.yaml \
    | sed "s/COMPONENT_NAME/$COMPONENT/g" \
    | sed "s/RUN_ID/$RUN_ID/g" \
    | sed "s|REPO_URL|$REPO_URL|g" \
    | sed "s/COMMIT_SHA/$COMMIT_SHA/g" \
    | sed "s|REGISTRY_URL|$REGISTRY_URL|g" \
    | sed "s|IMAGE_NAME|$IMAGE_NAME|g" \
    | sed "s/TAG/$TAG/g" \
    > "$MANIFEST_TEMP"

# 4. Apply the Job
kubectl apply -f "$MANIFEST_TEMP"

# Helper function for cleanup
cleanup() {
    echo "==> Cleaning up build Job..."
    kubectl delete -f "$MANIFEST_TEMP" --ignore-not-found=true
    rm -f "$MANIFEST_TEMP"
}
trap cleanup EXIT

# 5. Wait for the pod to be created and start running
echo "==> Waiting for build Pod to start..."
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

# 6. Stream init container clone logs if it runs
echo "==> Streaming checkout logs..."
kubectl logs -n draforge-ci "$pod_name" -c clone -f --tail=-1 || true

# 7. Stream Kaniko builder logs
echo "==> Streaming Kaniko build logs..."
kubectl logs -n draforge-ci "$pod_name" -c kaniko -f --tail=-1 || true

# 8. Check final status of the Job
echo "==> Checking build completion status..."
status=$(kubectl get job -n draforge-ci "$JOB_NAME" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)

if [ "$status" = "1" ]; then
    echo "SUCCESS: Container build completed and pushed successfully!"
    exit 0
else
    echo "FAIL: Container build failed. Check logs above." >&2
    exit 1
fi
