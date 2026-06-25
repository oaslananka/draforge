---
title: DRAForge
layout: default
description: Kubernetes Dynamic Resource Allocation observability, simulator, and diagnostics for GPU and accelerator workloads.
---

# DRAForge

**DRAForge** is a Kubernetes Dynamic Resource Allocation (DRA) observability, simulation, and diagnostics platform for GPU and accelerator workloads.

It helps Kubernetes platform engineers model DRA resources, simulate virtual device pools, inspect ResourceClaims and ResourceSlices, and diagnose why dynamic resource allocations succeed or fail.

## Features

- Kubernetes DRA diagnostics for ResourceClaims, ResourceSlices, drivers, and simulated device pools.
- Virtual accelerator simulation for GPU and device-style workloads.
- Explain engine for allocation troubleshooting and remediation.
- Terminal UI and React dashboard for resource visualization.
- Helm, Terraform, and DigitalOcean Kubernetes showcase assets.

## Documentation

- [Architecture](architecture.md)
- [Installation Guide](operations/install.md)
- [Operations Guide](operations/guide.md)
- [Troubleshooting Guide](operations/troubleshooting.md)
- [Dashboard guide](dashboard.md)
- [Demo guide](demo.md)
- [Simulator scenarios](simulator-scenarios.md)
- [Kubernetes DRA compatibility](compatibility/kubernetes-dra.md)
- [Release process](release.md)

## Showcase Demo Notes

When running the DRAForge showcase demo, the dashboard provides a real-time view of the cluster allocation state.

### Demo Walkthrough Steps
1. **Interactive Graph**: Navigate to the **GRAPH** tab to visualize the two worker nodes, resource slices, and claims.
2. **Cluster Health**: Open the **DOCTOR** tab to see running status diagnostics (such as DRA API availability and version compatibility).
3. **Simulation Diagnostics**: Go to the **EXPLAIN** tab and select any claim from the dropdown list to see the reason tree and suggested remediation steps.
4. **Relationship Scanning**: Click on any node in the graph, and use the **Relationships** panel in the bottom-right corner to jump directly between pods, resource claims, and devices.

## Repository

- [GitHub repository](https://github.com/oaslananka/draforge)
- [Security policy](https://github.com/oaslananka/draforge/blob/main/SECURITY.md)
- [Contributing guide](https://github.com/oaslananka/draforge/blob/main/CONTRIBUTING.md)

## Keywords

Kubernetes DRA, Dynamic Resource Allocation, ResourceClaim, ResourceSlice, device plugin, GPU simulator, accelerator simulator, observability, diagnostics, Kubernetes troubleshooting, Go, React, TypeScript, Helm, Terraform, DigitalOcean Kubernetes.
