# End-to-End Testing

DRAForge has a provider-neutral release gate and an optional provider adapter:

1. **Install-level kind E2E** installs the complete Helm release and is required for pull requests, scheduled compatibility checks, and tagged releases.
2. **Tagged smoke package** uses standard Kubernetes APIs and can run on any compatible cluster when supplied with a kubeconfig.
3. **Optional DOKS adapter** acquires a short-lived kubeconfig for one DigitalOcean showcase environment and then runs the same tagged smoke package.

The required kind path does not use cloud credentials or billable infrastructure. No provider-specific smoke run is a general release requirement.

## Compatibility source of truth

`tests/install-e2e/kubernetes-versions.json` defines the pinned kind version, Kubernetes node images, and two profiles:

| Profile | Targets | Use |
| --- | --- | --- |
| `pull-request` | Kubernetes v1.35.5 | Reduced gate on every pull request |
| `full` | Kubernetes v1.35.5 and v1.36.1 | Weekly schedule, manual full runs, release candidates, and final releases |

The node images are digest-pinned. The same policy pins the NetworkPolicy provider, manifest URL, and SHA-256. Update the JSON policy and this document together when the supported Kubernetes baseline or CNI changes. `scripts/e2e-matrix.sh` converts the policy to the GitHub Actions matrix; workflow YAML must not duplicate the version list.

## What the install-level suite verifies

The suite also runs two controller replicas and verifies one active Lease holder, leadership transfer after active-pod deletion, and post-failover reconciliation/allocation continuity.

`.github/workflows/e2e-matrix.yml` creates a kind cluster with Dynamic Resource Allocation enabled and the default CNI disabled, installs the digest-verified Calico manifest from the compatibility policy, builds the local server, controller, and sim-driver images, loads them into kind, and runs `scripts/run-install-e2e.sh`. This ensures NetworkPolicy assertions execute on an enforcing data plane rather than merely checking that policy objects exist.

The verifier:

- installs the chart CRD and the complete Helm release;
- waits for the server, controller, and node-plugin workloads to become ready;
- installs a hash-verified Calico CNI and proves controller metrics NetworkPolicy allow/deny behavior;
- checks positive and negative RBAC decisions for all three service accounts;
- applies a namespace-scoped simulator pool, DeviceClass, ResourceClaim, and claim-consuming Pod;
- waits for the controller to publish a ResourceSlice and allocate the claim;
- confirms the discovery API retains namespace, driver, pool, device, node, and consumer Pod identity;
- verifies summary, graph, explain, server metrics, controller metrics, readiness, and SSE output;
- fails when a core component is missing, an API assertion is broken, or namespace-qualified API/SSE identity diverges.

The deterministic contract suites run before the heavier kind job. `scripts/test-install-e2e-cni.sh` verifies the pinned CNI download, SHA-256 check, rollout waits, and fail-closed hash mismatch path. `scripts/test-install-e2e-harness.sh` exercises success, missing-component, broken-API, enforced NetworkPolicy allow/deny, and matrix-policy paths without creating a cluster.

## Pull-request and release behavior

Every pull request to `main` receives the reduced v1.35 install gate. A weekly schedule runs the full matrix. The release workflow calls the same reusable workflow with `profile: full`; the GoReleaser publishing job declares `needs: install-e2e`, so a release candidate or final release cannot publish after a failed or skipped required install test.

## Running locally

Required tools:

- Docker with daemon access;
- kind at the version declared in `tests/install-e2e/kubernetes-versions.json`;
- kubectl, Helm, jq, curl, Go, Node.js, and pnpm.

Run the fast orchestration contract:

```bash
task e2e:install-contract
```

Run the pull-request baseline on kind:

```bash
task e2e:install-kind
# equivalent:
scripts/kind-install-e2e.sh pull-request
```

Run every full-matrix target sequentially:

```bash
task e2e:install-kind-full
# equivalent:
scripts/kind-install-e2e-matrix.sh full
```

The local entry point deletes its kind cluster after the run. Preserve a failed cluster for investigation with:

```bash
DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 scripts/kind-install-e2e.sh pull-request
```

Artifacts are written under `artifacts/install-e2e/`. The directory contains the run report, API and SSE payloads, metrics, Helm manifests and values, Kubernetes resources, events, descriptions, and component logs.

## Failure artifacts in GitHub Actions

Failed matrix entries upload a seven-day artifact named `install-e2e-<kubernetes-version>-<run-id>`. Diagnostics are collected with `if: always()` before upload, including:

- rendered Helm manifests and effective values;
- server, controller, and node-plugin logs, including previous-container logs when available;
- ResourceClaims, ResourceSlices, DeviceClasses, simulator resources, NetworkPolicies, and RBAC;
- cluster events and Pod descriptions;
- discovery, graph, explain, metrics, and SSE payloads produced before the failure.

## Portable remote smoke workflow

`.github/workflows/e2e-kubernetes.yml` exercises the same provider-neutral `scripts/remote-e2e.sh` core in two modes:

- **kind mode** creates an ephemeral Kubernetes v1.35.5 cluster with DRA enabled, using the pinned kind version, node image, and hash-verified Calico CNI from `tests/install-e2e/kubernetes-versions.json`. Relevant pull requests run this mode automatically, which proves Pod networking, the remote Job, least-privilege RBAC, result verification, artifacts, and cleanup on a non-DigitalOcean cluster.
- **external mode** consumes a standard kubeconfig from the protected `e2e-kubernetes` GitHub Environment. Store the base64-encoded short-lived kubeconfig as `KUBECONFIG_B64`, sourced from Doppler rather than a committed file or workflow input. The preparation script decodes to a runner-temporary absolute path, enforces mode `0600`, validates the configuration with kubectl, and removes it after the run.

Manual runs require the exact confirmation phrase `run-e2e-kubernetes`. External credentials should grant only the access required to create the run-scoped Namespace controls, Job, ServiceAccount, ClusterRole, and ClusterRoleBinding and to read the DRA/core resources used by `TestSmoke`. The portable smoke workflow supplements the required install E2E matrix; it does not replace or broaden the release support matrix.

Run the credential-handling and orchestration contracts locally without a cluster:

```bash
scripts/test-prepare-remote-e2e-kubeconfig.sh
scripts/test-remote-e2e-harness.sh
```

## Optional DOKS smoke adapter

The manual `Optional DOKS E2E` workflow is one provider-specific adapter for the provider-neutral tagged Go smoke package. It runs on an **existing** DOKS cluster and does not create or destroy the cluster, node pools, VPC, or registry. A missing DOKS credential may block this optional adapter, but it does not invalidate the kind-based release gate or the Kubernetes portability contract.

### Required secret and approvals

- Store `DIGITALOCEAN_TOKEN` as a GitHub Actions repository secret and protect the `e2e-doks` environment with maintainer approval.
- Prefer a custom-scope DigitalOcean token with only `kubernetes:read` and `kubernetes:access_cluster`.
- Never place the token, generated kubeconfig, or cluster credentials in workflow inputs, logs, artifacts, or committed files.
- The workflow requests a one-hour kubeconfig credential using `--expiry-seconds 3600`.

### Cost impact

The workflow reconciles the shared `draforge-ci` Namespace, ResourceQuota, and LimitRange, then creates one short-lived Job, ServiceAccount, read-only ClusterRole, and ClusterRoleBinding for the run. It does not create DigitalOcean infrastructure, but the existing DOKS worker nodes remain billable while the cluster exists. The shared namespace controls remain after cleanup and do not create separate DigitalOcean charges.

### Running the workflow

1. Confirm the target cluster exists and serves the required `resource.k8s.io/v1` APIs.
2. Open **GitHub Actions → Optional DOKS E2E → Run workflow**.
3. Enter the exact confirmation phrase `run-e2e-doks`.
4. Enter the existing cluster name and, optionally, a 7–40 character commit SHA.
5. Approve the protected `e2e-doks` environment deployment.

The remote Job runs:

```bash
DRAFORGE_E2E=1 go test -count=1 -json -tags=e2e ./tests/e2e/...
```

A run succeeds only when `TestSmoke` executes and passes. The harness rejects build-tag mistakes, `no packages to test`, zero executed tests, all-skipped output, cluster connection failure, and failed DRA API availability checks.

Run-scoped Job and RBAC resources are deleted by both the script trap and the workflow cleanup job. Cleanup is attempted after success, failure, and cancellation. The shared `draforge-ci` namespace controls are intentionally retained for later runs. Every run uploads a seven-day `remote-e2e-<run-id>` artifact containing Job and Pod YAML, clone and runner logs, namespace events, and `go-test.json` when available.
