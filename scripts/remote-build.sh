#!/usr/bin/env bash
# scripts/remote-build.sh
# Submits a Kubernetes Job to build a component container image remotely on DOKS using Kaniko.

set -euo pipefail

COMPONENT=${1:-""}
RAW_TAG=${2:-"latest"}
TAG="${COMPONENT}-${RAW_TAG}"
COMMIT_SHA=${3:-$(git rev-parse HEAD)}

case "$COMPONENT" in
    server | controller | sim-driver)
        ;;
    *)
        echo "Usage: $0 <server|controller|sim-driver> [tag] [commit-sha]" >&2
        exit 2
        ;;
esac
if [[ ! "$RAW_TAG" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$ ]]; then
    echo "ERROR: tag must be 1-100 OCI tag characters" >&2
    exit 2
fi
if [[ ! "$COMMIT_SHA" =~ ^[0-9a-fA-F]{7,40}$ ]]; then
    echo "ERROR: commit SHA must be 7-40 hexadecimal characters" >&2
    exit 2
fi

REPO_URL="https://github.com/oaslananka/draforge.git"
REGISTRY_URL="registry.digitalocean.com/draforge"
IMAGE_NAME="draforge"
RUN_ID=$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')
JOB_NAME="draforge-build-$COMPONENT-$RUN_ID"

echo "==> Preparing remote container build for $COMPONENT..."
echo "  - Component: $COMPONENT"
echo "  - Commit SHA: $COMMIT_SHA"
echo "  - Destination Image: $REGISTRY_URL/$IMAGE_NAME:$TAG"
echo "  - Run ID: $RUN_ID"

load_digitalocean_token() {
    local key value
    [[ -f .env ]] || return
    for key in DIGITALOCEAN_TOKEN DIGITAL_OCEAN_API_TOKEN DIGITAL_OCEON_API_KEY; do
        value=$(grep -E "^${key}=" .env | tail -n 1 | cut -d= -f2- || true)
        value=$(printf "%s" "$value" | tr -d '\r' | tr -d '"' | tr -d "'")
        if [[ -n "$value" ]]; then
            export DIGITALOCEAN_TOKEN=$value
            return
        fi
    done
}

# Load the standard token name from the environment or .env. Legacy names are
# accepted for compatibility with older showcase configurations.
if [[ -z "${DIGITALOCEAN_TOKEN:-}" ]]; then
    load_digitalocean_token
fi

if [ -z "${DIGITALOCEAN_TOKEN:-}" ]; then
    echo "ERROR: DIGITALOCEAN_TOKEN not found in environment or .env" >&2
    exit 1
fi

create_registry_secret() {
    local namespace
    namespace=$1
    kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
    kubectl create secret docker-registry registry-draforge \
        --namespace "$namespace" \
        --docker-server=registry.digitalocean.com \
        --docker-username="$DIGITALOCEAN_TOKEN" \
        --docker-password="$DIGITALOCEAN_TOKEN" \
        --docker-email="unused@unused.com" \
        --dry-run=client -o yaml | kubectl apply -f -
}

# 1. Create remote CI resources if they do not exist.
kubectl apply -f build/remote/namespace.yaml
kubectl apply -f build/remote/quota.yaml

# 2. Reconcile registry credentials for the Kaniko build namespace and the
#    Helm workload namespace. The public chart defaults do not require these.
create_registry_secret draforge-ci
create_registry_secret draforge-system

# 3. Generate the Job manifest
MANIFEST_TEMP=$(mktemp)
sed \
    -e "s/COMPONENT_NAME/$COMPONENT/g" \
    -e "s/RUN_ID/$RUN_ID/g" \
    -e "s|REPO_URL|$REPO_URL|g" \
    -e "s|COMMIT_SHA|$COMMIT_SHA|g" \
    -e "s|REGISTRY_URL|$REGISTRY_URL|g" \
    -e "s|IMAGE_NAME|$IMAGE_NAME|g" \
    -e "s/TAG/$TAG/g" \
    build/remote/kaniko-job.yaml > "$MANIFEST_TEMP"

cleanup_resources() {
    set +e
    echo "==> Cleaning up build Job..."
    kubectl delete -f "$MANIFEST_TEMP" --ignore-not-found=true --wait=false
    rm -f "$MANIFEST_TEMP"
}

finish() {
    local exit_code
    exit_code=$1
    trap - EXIT INT TERM
    cleanup_resources
    exit "$exit_code"
}

trap 'finish "$?"' EXIT
trap 'finish 130' INT
trap 'finish 143' TERM

# 4. Apply the Job
kubectl apply -f "$MANIFEST_TEMP"

# 5. Wait for the pod to be created and start running
echo "==> Waiting for build Pod to start..."
pod_name=""
for _ in {1..30}; do
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
for _ in {1..30}; do
    if kubectl logs -n draforge-ci "$pod_name" -c clone -f --tail=-1 2>/dev/null; then
        break
    fi
    sleep 2
done

# 7. Stream Kaniko builder logs
echo "==> Streaming Kaniko build logs..."
for _ in {1..30}; do
    if kubectl logs -n draforge-ci "$pod_name" -c kaniko -f --tail=-1 2>/dev/null; then
        break
    fi
    sleep 2
done

# 8. Check final status of the Job
echo "==> Checking build completion status..."
status=""
failed=""
for _ in {1..10}; do
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
    echo "SUCCESS: Container build completed and pushed successfully!"
    finish 0
fi

echo "FAIL: Container build failed. Check logs above." >&2
finish 1
