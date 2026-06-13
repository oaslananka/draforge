# DigitalOcean Account Discovery Report

This report documents the current state of the DigitalOcean account prior to resource provisioning for the **DRAForge** project.

## Account Quotas and Limits
- **Droplet Limit**: 3 Droplets
- **Volume Limit**: 5000 volumes
- **Account Status**: Active

## Existing Resources
- **Projects**: 
  - `oaslananka` (Default, ID: `8dac321f-xxxx-xxxx-xxxx-xxxxxxxxxxxx`)
- **Kubernetes Clusters**: 
  - None
- **Droplets (Compute)**: 
  - `ops-vps-fra1` (Region: `fra1`, Size: `s-2vcpu-4gb`, Status: `active`, IP: `167.172.175.156`, VPC UUID: `f3790f60-xxxx-xxxx-xxxx-xxxxxxxxxxxx`)
  - *Note*: This is an unrelated droplet and will not be modified or deleted. Since the account has a limit of 3 Droplets, our 2-node DOKS cluster (adding 2 worker droplets) will bring the total to 3, fitting exactly within the limit.
- **Container Registry**:
  - `draforge` (Region: `tor1`, Created: `2026-06-12T21:45:59Z`)
  - *Note*: An existing container registry named `draforge` is already present. We will integrate this registry into our Terraform configuration.
- **VPCs, Firewalls, Load Balancers, Domains, Volumes**:
  - Checked. A dedicated VPC will be provisioned in the `fra1` region for the cluster.

## Target Architecture Specifications
- **Region**: `fra1` (Frankfurt)
- **Kubernetes Version**: `1.36.0-do.1` (Latest supported 1.36 version)
- **Worker Node Size**: `s-4vcpu-8gb` (4 vCPUs, 8 GiB RAM)
- **Worker Node Count**: 2 (Autoscaling: disabled, HA: disabled)
