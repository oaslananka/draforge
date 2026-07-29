---
title: DRAForge
layout: default
description: Provider-neutral Kubernetes Dynamic Resource Allocation observability, simulation, and diagnostics for GPU and accelerator workloads.
---

# DRAForge

**DRAForge** is a provider-neutral Kubernetes Dynamic Resource Allocation (DRA) observability, simulation, and diagnostics platform for GPU and accelerator workloads.

It helps platform engineers model virtual device pools, inspect `ResourceClaim` and `ResourceSlice` state, and explain why dynamic resource allocations succeed or fail—without requiring physical accelerator hardware or a specific cloud provider.

[Latest release](https://github.com/oaslananka/draforge/releases/latest) · [GitHub repository](https://github.com/oaslananka/draforge) · [Discussions](https://github.com/oaslananka/draforge/discussions) · [Security policy](https://github.com/oaslananka/draforge/blob/main/SECURITY.md) · [Support](https://github.com/oaslananka/draforge/blob/main/SUPPORT.md)

## Start here

| Goal | Guide |
|---|---|
| Run the verified local kind stack | [Installation guide](operations/install.md#create-and-keep-the-verified-cluster) |
| Install or operate DRAForge on Kubernetes | [Installation guide](operations/install.md) and [operations guide](operations/guide.md) |
| Understand components and data flow | [Architecture](architecture.md) |
| Check supported Kubernetes DRA behavior | [Kubernetes DRA compatibility](compatibility/kubernetes-dra.md) |
| Diagnose pending claims or runtime failures | [Troubleshooting guide](operations/troubleshooting.md) |
| Explore the dashboard, CLI, and API | [Dashboard guide](dashboard.md) and [public API/CLI surface](api.md) |

## What DRAForge includes

- Evidence-based diagnostics for `ResourceClaim`, `ResourceSlice`, drivers, and simulated device pools.
- Virtual accelerator simulation for GPU and device-style workloads.
- Explain engine for selector, capacity, node-affinity, and allocation troubleshooting.
- Terminal UI and React dashboard for live resource visualization.
- Provider-neutral Helm deployment for compatible Kubernetes clusters.
- Optional Terraform and DigitalOcean Kubernetes showcase assets that are not required for normal installation, testing, or releases.

## Documentation map

### Users and operators

- [Installation guide](operations/install.md)
- [Operations guide](operations/guide.md)
- [Troubleshooting guide](operations/troubleshooting.md)
- [Dashboard guide](dashboard.md)
- [Simulator scenarios](simulator-scenarios.md)
- [Kubernetes DRA compatibility](compatibility/kubernetes-dra.md)
- [Optional DigitalOcean showcase](demo.md)

### Contributors and maintainers

- [Architecture](architecture.md)
- [Architecture decisions](adr/0001-product-scope.md)
- [Local quality gate](local-quality-gate.md)
- [Install E2E matrix](e2e-matrix.md)
- [Release process](release.md)
- [Artifact gate](artifact-gate.md)
- [Maintainer checklist](maintainer-checklist.md)
- [Contributing guide](https://github.com/oaslananka/draforge/blob/main/CONTRIBUTING.md)
- [Governance](https://github.com/oaslananka/draforge/blob/main/GOVERNANCE.md)

## Project scope

DRAForge components use Kubernetes APIs and Helm contracts rather than provider-specific services. They can run on compatible managed Kubernetes offerings, self-managed clusters, on-premises environments, and local test clusters. Provider-specific infrastructure and registry assets in this repository are optional showcases.

## Keywords

Kubernetes DRA, Dynamic Resource Allocation, ResourceClaim, ResourceSlice, device plugin, GPU simulator, accelerator simulator, observability, diagnostics, Kubernetes troubleshooting, managed Kubernetes, on-premises Kubernetes, kind, Go, React, TypeScript, Helm, Terraform.
