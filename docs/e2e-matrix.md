# End-to-End (E2E) Testing

This document outlines the strategy and workflows for End-to-End (E2E) testing of DRAForge across different Kubernetes environments.

## CI-Safe Local E2E Matrix

We use GitHub Actions to run a matrix of tests across supported Kubernetes versions using `kind` (Kubernetes in Docker). This allows us to test real-cluster API interactions (like Dynamic Resource Allocation) without requiring cloud secrets or provisioning billable infrastructure.

### Supported Kubernetes Versions
The matrix tests against the following Kubernetes versions:
- `v1.32.x`
- `v1.33.x`
- `v1.34.x`
- `v1.35.x`
- `v1.36.x`

### Running the Local E2E Matrix Locally
You can run the same tests locally using `kind`. Make sure the DRA feature gate is enabled.
```bash
# Create a kind cluster with DRA enabled
kind create cluster --image kindest/node:v1.35.0 --name draforge-e2e --config - <<KIND_CONFIG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
featureGates:
  DynamicResourceAllocation: true
KIND_CONFIG

task build
kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
DRAFORGE_E2E=1 go test -tags=e2e ./tests/e2e/ -v
```

## Manual Cloud E2E (DOKS)

The manual `E2E Tests` workflow runs the tagged smoke package on an **existing** DOKS cluster. It does not create or destroy the cluster, node pools, VPC, or registry.

### Required secret and approvals

- Store `DIGITALOCEAN_TOKEN` as a GitHub Actions repository secret and protect the `e2e-doks` environment with maintainer approval.
- Prefer a custom-scope DigitalOcean token with only `kubernetes:read` and `kubernetes:access_cluster`.
- Never place the token, generated kubeconfig, or cluster credentials in workflow inputs, logs, artifacts, or committed files.
- The workflow requests a one-hour kubeconfig credential using `--expiry-seconds 3600`.

### Cost impact

The workflow reconciles the shared `draforge-ci` Namespace, ResourceQuota, and LimitRange, then creates one short-lived Job, ServiceAccount, read-only ClusterRole, and ClusterRoleBinding for the run. It does not add DigitalOcean infrastructure, but the existing DOKS worker nodes remain billable while the cluster exists. The shared namespace controls remain after cleanup and do not create separate DigitalOcean charges. Review the cost-control runbook before execution and destroy unused showcase infrastructure separately.

### Running the workflow

1. Confirm the target cluster already exists and exposes the `resource.k8s.io` APIs required by the smoke test.
2. Go to **GitHub Actions → E2E Tests → Run workflow**.
3. Enter the exact confirmation phrase `run-e2e-doks`.
4. Enter the existing cluster name and optionally a 7–40 character commit SHA.
5. Approve the `e2e-doks` environment deployment.

The remote Job runs:

```bash
DRAFORGE_E2E=1 go test -count=1 -json -tags=e2e ./tests/e2e/...
```

A run succeeds only when `TestSmoke` executes and passes. The harness fails on build-tag mistakes, `no packages to test`, zero executed tests, an all-skipped result, cluster connection failure, or a failed DRA API availability check.

Run-scoped Job and RBAC resources are deleted by both the script trap and the workflow cleanup job. Cleanup is attempted after success, failure, and cancellation. The shared `draforge-ci` Namespace, ResourceQuota, and LimitRange are intentionally retained for later remote jobs.

## Log Collection

During E2E testing, logs are collected for debugging purposes:
1. The **E2E Matrix** workflow uploads cluster diagnostics when a matrix entry fails.
2. The **Manual Cloud E2E** workflow always uploads a seven-day `remote-e2e-<run-id>` artifact containing the Job and Pod YAML, clone and runner logs, namespace events, and `go-test.json` when available.
3. For local debugging, use standard `kubectl logs` commands:
   ```bash
   kubectl logs -n draforge-system -l app.kubernetes.io/name=draforge-controller
   ```
