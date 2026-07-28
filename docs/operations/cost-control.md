# Optional DigitalOcean Showcase Cost Control

This document applies only to the optional DigitalOcean showcase. DRAForge does not require these resources or this provider. The estimates and commands below help maintainers control costs when they explicitly choose the DOKS/DOCR demo path.

## Billable Resources

| Resource | Region | Size/Type | Count | Unit Cost (Est) | Monthly Cost (Est) |
| --- | --- | --- | --- | --- | --- |
| **DOKS Cluster** | `fra1` | Control Plane (Basic) | 1 | $0.00 / month | $0.00 / month |
| **Worker Nodes** | `fra1` | `s-4vcpu-8gb` | 2 | $48.00 / month each | $96.00 / month |
| **Container Registry** | `tor1` | Basic Plan | 1 | $5.00 / month | $5.00 / month |
| **Load Balancer** | `fra1` | Regional LB | 1 | $12.00 / month | $12.00 / month |
| **Block Storage Volume** | `fra1` | 10 GiB | 1 | $1.00 / month | $1.00 / month |

**Expected Total Monthly Cost Range**: $100.00 - $115.00 USD.

## Usage Inspection Commands

To check the currently running billable resources in your account:

```bash
# List all active droplets (including worker nodes)
doctl compute droplet list --tag-name project:draforge

# List active DOKS clusters
doctl kubernetes cluster list

# List Load Balancers
doctl compute load-balancer list

# List Block Storage Volumes
doctl compute volume list
```

## Scaling Commands

Scaling must always strictly respect the maximum-node constraint (maximum of 2 worker nodes).

```bash
# Inspect the node pools
doctl kubernetes cluster node-pool list <cluster-id>

# Scale worker nodes within the safe limit (1 or 2 nodes only)
doctl kubernetes cluster node-pool update <cluster-id> <pool-id> --count 2
```

## Safe Destruction Procedure

To tear down all resources and stop billing, run the following:

```bash
# 1. Plan destruction to preview what will be removed
make infra-destroy-plan

# 2. Apply destruction (this will remove all provisioned DRAForge resources)
cd infra/terraform/environments/showcase
terraform destroy -auto-approve
```

## Terraform Showcase Validation Workflow

Run scripts/validate-terraform-showcase.sh before changing showcase infrastructure.

### Supported Terraform inputs and migration note

The showcase publishes only inputs that affect the generated infrastructure: `project_name`, `region`, `kubernetes_version`, `node_size`, `node_count`, `registry_name`, and `tags`. Worker size and count directly affect provider cost. Tags are inventory metadata only and are not an authorization boundary.

The unused `environment`, `enable_public_dashboard`, `domain_name`, and `allowed_admin_cidrs` inputs were removed. They never changed a Terraform resource, so removing them does not alter an existing plan. Dashboard exposure remains controlled by the explicit Helm profiles documented in the [installation guide](install.md); Terraform variables must not be treated as a substitute for TLS, authentication, Gateway/Ingress policy, or network access controls.
