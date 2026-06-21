# DRAForge - Showcase Demo Runbook

This runbook outlines how to deploy, manage, and tear down the DRAForge showcase platform on a 2-node DigitalOcean Kubernetes (DOKS) cluster.

## Setup Requirements

Before launching the showcase:
1. Ensure the `doctl`, `terraform`, and `helm` command-line tools are installed on the host.
2. Verify you have a valid DigitalOcean API token.
3. Configure a `.env` file at the root of the repository containing:
   ```env
   DIGITAL_OCEAN_API_TOKEN="your-digitalocean-api-token"
   ```

---

## Bring Up the Showcase

To completely build and deploy the showcase platform, run:

```bash
task demo:up
```

### What `task demo:up` performs:
1. **Cost & Safety Audit**: Executes `scripts/audit-cloud-resources.sh` to ensure no active worker nodes or standalone Droplets exceed budget limits. Runs `scripts/validate-plan.py` to confirm the Terraform plan complies with the maximum 2-worker node policy.
2. **Infrastructure Provisioning**: Initializes and runs `terraform apply` to spin up the Dedicated VPC, DOKS cluster, and private DigitalOcean Container Registry (DOCR).
3. **Private Registry Integration**: Configures Kubeconfig registry secrets in the namespaces.
4. **Remote Image Builds**: Submits rootless Kubernetes Kaniko jobs to build the `server`, `controller`, and `sim-driver` container images directly on the DOKS cluster.
5. **Helm Chart Installation**: Deploys the DRAForge services (with standard wildcard egress NetworkPolicies, image pull secrets, and HTTP API routing via Gateway API).
6. **Scenario Seeding**: Applies the custom simulated device pool configurations (e.g. `examples/scenarios/basic-gpu.yaml`).
7. **Exposes Endpoint**: Retrieves the LoadBalancer IP of the Cilium Gateway and outputs the live dashboard URL.

---

## Showcase Walkthrough

Once the dashboard is online:
1. **Explore the Graph**: Navigate to the **Graph** tab. You should see the interactive layout displaying the two worker nodes, their simulated GPU resource slices, and the active devices.
2. **Cluster Health Diagnostics**: Go to the **Doctor** tab to review the 5 passing checks (DRA API availability, node compatibility, and slice consistency).
3. **Observe Resource Claims**: Apply a test pod requesting a GPU (e.g., `kubectl apply -f examples/scenarios/insufficient-capacity.yaml` or a claim template). The dashboard will dynamically stream the allocation event.
4. **Simulate a Fault**:
   Inject a device failure using the CLI:
   ```bash
   draforge inject fault --pool basic-gpu-pool --type unhealthy
   ```
   Watch the graph update in real-time. The affected devices will show warning states, and the `doctor` check will highlight the failure.

---

## Tear Down the Showcase

To avoid unnecessary DigitalOcean billings, clean up all provisioned resources:

```bash
task demo:down
```

This will uninstall the Helm release, delete all scenario custom resources, and execute `terraform destroy` to tear down the DOKS cluster, VPC, and registry.

---

## GitHub Actions E2E Testing (Remote)

For continuous integration and release readiness, a manual-only remote E2E test workflow is provided under `.github/workflows/e2e.yml`. This workflow runs remote tests on a live DigitalOcean Kubernetes (DOKS) cluster.

### E2E-DOKS Environment Approval Flow
1. **GitHub Environment**: The workflow is gated by the `e2e-doks` GitHub Environment.
2. **Required Approvals**: Direct repository maintainer approval is required to trigger execution in this environment.
3. **Safety Phrase**: When triggering the workflow manually via `workflow_dispatch`, you must enter `run-e2e-doks` in the `confirm` input field.

### Secret Configuration & Token Scope
The workflow requires a GitHub repository secret named `DIGITALOCEAN_TOKEN`.
- **Required Token Scope**: The token must have **Read & Write** access to:
  - **Kubernetes**: To save kubeconfig and interact with the DOKS cluster.
  - **Droplets / VPC**: If the E2E script provisions resources.
- **Repository Visibility**: Only maintainers with access to secrets can run this workflow.

### Manual E2E Run Checklist
Before running the remote E2E workflow:
1. [ ] Confirm that the target DOKS cluster (default: `draforge-cluster`) exists and is active on DigitalOcean.
2. [ ] Verify that your `DIGITALOCEAN_TOKEN` secret is configured in the repository settings under **Secrets and variables -> Actions**.
3. [ ] Go to the **Actions** tab, select the **E2E Tests** workflow, and click **Run workflow**.
4. [ ] Enter `run-e2e-doks` as confirmation and specify the correct cluster name.

### Cleanup Behavior after Failure/Cancellation
- **Automatic Cleanup Job**: The workflow has a dedicated `cleanup` job that runs `always()` (even if the main test job fails or is cancelled).
- **Resource Destruction**: This cleanup job deletes the Helm release (`draforge`) and any applied scenario files (e.g. `examples/scenarios/basic-gpu.yaml`) from the cluster to prevent dangling resources.

### Common Failure Modes & Troubleshooting
1. **Error: `DIGITALOCEAN_TOKEN secret is not configured`**
   - *Cause*: The secret was not added or is named incorrectly.
   - *Fix*: Check repository secrets and ensure `DIGITALOCEAN_TOKEN` is defined.
2. **Error: `cluster ... not found`**
   - *Cause*: The cluster name provided in the workflow input does not exist in the DigitalOcean account.
   - *Fix*: Ensure the cluster is created first or double-check the spelling in the `cluster_name` input.
3. **Error: `Unauthorized / Forbidden` API calls**
   - *Cause*: The `DIGITALOCEAN_TOKEN` does not have write scope or has expired.
   - *Fix*: Regenerate a token with write scope in the DigitalOcean control panel and update the GitHub secret.
