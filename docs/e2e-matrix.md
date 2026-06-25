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

For production-like environments, we can run manual remote E2E tests against a DigitalOcean Kubernetes cluster (DOKS).

**Requirements**: This requires the `DIGITALOCEAN_TOKEN` secret and provisions billable resources. It is gated behind manual confirmation in GitHub Actions.

To run:
1. Go to GitHub Actions -> **E2E Tests** workflow.
2. Click **Run workflow**.
3. You **must** enter `run-e2e-doks` in the confirmation field.
4. Specify the cluster name (e.g., `draforge-cluster`).

The workflow will provision resources, run the test via `scripts/remote-e2e.sh`, stream logs, and then automatically clean up the billable resources.

## Log Collection

During E2E testing, logs are collected for debugging purposes. If a test fails:
1. In the **E2E Matrix** workflow, artifacts are uploaded containing cluster logs.
2. In the **Manual Cloud E2E**, logs are streamed directly to the GitHub Actions console.
3. For local debugging, use standard `kubectl logs` commands:
   ```bash
   kubectl logs -n draforge-system -l app.kubernetes.io/name=draforge-controller
   ```
