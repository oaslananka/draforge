# DRAForge - Showcase Demo Runbook

This runbook outlines how to deploy, manage, and tear down the DRAForge showcase platform on a 2-node DigitalOcean Kubernetes (DOKS) cluster.

## Setup Requirements

Before launching the showcase:
1. Ensure the `doctl`, `terraform`, and `helm` command-line tools are installed on the host.
2. Verify you have a valid DigitalOcean API token.
3. Configure a `.env` file at the root of the repository containing:
   ```env
   DIGITAL_OCEON_API_KEY="your-digitalocean-api-token"
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
