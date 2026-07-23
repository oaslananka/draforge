#!/usr/bin/env bash
set -euo pipefail

HELM_BIN=${HELM_BIN:-helm}
CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_count() {
  local file=$1
  local text=$2
  local expected=$3
  local actual
  actual=$(grep -Fc -- "$text" "$file" || true)
  [[ "$actual" -eq "$expected" ]] || fail "$file contains '$text' $actual times, want $expected"
}

manifest="$work_dir/helm.yaml"
"$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system > "$manifest"

# Server, controller, and node plugin need Kubernetes API credentials and are
# explicitly bound to dedicated least-privilege ClusterRoles.
for component in server controller node-plugin; do
  assert_contains "$manifest" "serviceAccountName: draforge-$component"
  assert_contains "$manifest" "name: draforge-$component-binding"
  assert_contains "$manifest" "name: draforge-$component-role"
done

# All three runtime workloads have bounded writable storage.
assert_count "$manifest" "ephemeral-storage: 256Mi" 3
assert_count "$manifest" "ephemeral-storage: 64Mi" 3
assert_contains "$manifest" "sizeLimit: 16Mi"

# Build and unit-test jobs do not call the Kubernetes API.
for job in build/remote/kaniko-job.yaml build/remote/test-job.yaml; do
  assert_contains "$job" "automountServiceAccountToken: false"
  assert_contains "$job" "ephemeral-storage:"
  assert_contains "$job" "sizeLimit:"
done

# Test runners and their clone init containers are non-root with immutable
# root filesystems; all writable state is mounted and size-bounded.
for job in build/remote/test-job.yaml build/remote/e2e-job.yaml; do
  assert_contains "$job" "runAsNonRoot: true"
  assert_count "$job" "readOnlyRootFilesystem: true" 2
  assert_count "$job" "allowPrivilegeEscalation: false" 2
  assert_contains "$job" "value: /cache/go-mod"
  assert_contains "$job" "mountPath: /tmp"
done

# Kaniko's executor requires root and a writable root filesystem to unpack
# base images and execute root-owned Dockerfile RUN steps. Keep that exception
# explicit while pinning the maintained fork and denying extra privileges.
assert_contains build/remote/kaniko-job.yaml "ghcr.io/kaniko-build/dist/chainguard-dev-kaniko/executor:v1.25.15@sha256:86deb96280f5020e009dc03a8c2eb482ade3541a85d65613d07f9ba30c8c5fe5"
assert_contains build/remote/kaniko-job.yaml "runAsUser: 0"
assert_contains build/remote/kaniko-job.yaml "privileged: false"
assert_contains build/remote/kaniko-job.yaml "allowPrivilegeEscalation: false"
assert_contains build/remote/kaniko-job.yaml "readOnlyRootFilesystem: false"
for capability in CHOWN DAC_OVERRIDE FOWNER MKNOD SETFCAP SETGID SETUID; do
  assert_contains build/remote/kaniko-job.yaml "- $capability"
done
assert_contains build/remote/kaniko-job.yaml "type: RuntimeDefault"

# Remote E2E has a dedicated RBAC manifest and therefore intentionally keeps
# its token, while still bounding ephemeral storage.
assert_contains build/remote/e2e-job.yaml "serviceAccountName: draforge-e2e-RUN_ID"
assert_contains build/remote/e2e-job.yaml "automountServiceAccountToken: true"
assert_contains build/remote/e2e-rbac.yaml "kind: ClusterRoleBinding"
assert_contains build/remote/e2e-job.yaml "ephemeral-storage:"
assert_contains build/remote/e2e-job.yaml "sizeLimit:"

echo "Workload security contract verified."
