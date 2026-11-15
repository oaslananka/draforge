<div align="center">

# DRAForge

**Understand why a Kubernetes GPU or accelerator claim is pending, allocated, or failing.**

DRAForge connects Kubernetes ResourceClaims, device pools, and ResourceSlices into a readable graph, explains allocation decisions, and lets you reproduce DRA scenarios with simulated hardware—no physical GPU required.

[**Try it locally**](#quickstart) · [**Read the documentation**](https://oaslananka.github.io/draforge/) · [**Download v0.4.0**](https://github.com/oaslananka/draforge/releases/latest)

[![CI](https://github.com/oaslananka/draforge/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/oaslananka/draforge/actions/workflows/ci.yml)
[![Security Analysis](https://github.com/oaslananka/draforge/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/oaslananka/draforge/actions/workflows/security.yml)
[![Go Coverage](https://codecov.io/gh/oaslananka/draforge/branch/main/graph/badge.svg)](https://app.codecov.io/github/oaslananka/draforge)
[![Latest Release](https://img.shields.io/github/v/release/oaslananka/draforge?sort=semver&display_name=tag)](https://github.com/oaslananka/draforge/releases/latest)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/oaslananka/draforge/badge)](https://scorecard.dev/viewer/?uri=github.com/oaslananka/draforge)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13404/badge)](https://www.bestpractices.dev/projects/13404)



<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0--only-blue" alt="AGPL-3.0-only license" /></a>
  <a href="https://gitgem.org/github/oaslananka/draforge"><img src="https://gitgem.org/api/badge/github/oaslananka/draforge.svg" alt="GitGem" /></a>
</p>

<p align="center">
  <a href="https://github.com/sponsors/oaslananka"><img src="https://img.shields.io/badge/GitHub-Sponsors-ea4aaa?logo=githubsponsors" alt="GitHub Sponsors" /></a>
  <a href="https://www.buymeacoffee.com/oaslananka"><img src="https://img.shields.io/badge/Buy%20Me%20a%20Coffee-Support-ffdd00?logo=buymeacoffee&amp;logoColor=black" alt="Buy Me a Coffee" /></a>
  <a href="https://github.com/oaslananka/draforge"><img src="https://img.shields.io/github/stars/oaslananka/draforge?label=Stars&amp;color=8e65d3" alt="Stars" /></a>
</p>

</div>

## What can you do?

| Inspect allocations | Reproduce scenarios | Explain the failure |
|:---|:---|:---|
| See real Kubernetes claims, classes, pools, and devices in one relationship graph. | Create **synthetic** accelerator pools on a local kind cluster; no GPU or cloud provider needed. | Open a claim's explanation to investigate selectors, capacity, node affinity, and DRA health checks. |
| [Dashboard guide](docs/dashboard.md) | [Local quickstart](#quickstart) | [Troubleshooting](docs/operations/troubleshooting.md) |

## See the allocation path

<a href="docs/assets/dashboard-simulation.webp">
  <img src="docs/assets/dashboard-simulation.webp" alt="Real DRAForge dashboard: a selected synthetic ResourceClaim, connected resources, and a diagnostics action in the interactive graph" width="100%">
</a>

<sub>Actual DRAForge React dashboard rendered with <strong>synthetic demonstration data</strong>. This is a product preview—not a publicly connected Kubernetes cluster or proof of physical GPU support. <a href="docs/assets/dashboard-simulation.webp">View the full-size image</a>.</sub>

## Quickstart

**Try a complete local DRA flow.** You need Docker Engine or Docker Desktop with WSL integration, Bash, kind, kubectl, Helm, jq, curl, and sha256sum. No cloud account or physical accelerator is required.

```bash
git clone https://github.com/oaslananka/draforge.git
cd draforge
DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 bash scripts/kind-install-e2e.sh pull-request

# In a separate terminal, open http://localhost:8080
kubectl port-forward svc/draforge-server -n draforge-system 8080:8080
```

The disposable kind installation checks its prerequisites, refuses to overwrite an existing cluster, and leaves the cluster running for exploration. Visit **http://localhost:8080**, select **Relationships**, then **Explain a claim** to follow a simulated allocation. When finished, use `kind delete cluster --name draforge-install-e2e` only for the disposable cluster you created.

**Already have Kubernetes?** Use the [production/local installation guide](docs/operations/install.md) for read-only deployment and ingress controls. **On Windows?** Enable Docker Desktop's [WSL 2 integration](https://docs.docker.com/desktop/features/wsl/) for the distribution running the commands, then confirm `docker info` in that same terminal.

<details>
<summary>More resources — architecture, compatibility, operations, and project policies</summary>

[Architecture](docs/architecture.md) · [Kubernetes compatibility](docs/compatibility/kubernetes-dra.md) · [Operations](docs/operations/guide.md) · [Contributing](CONTRIBUTING.md) · [Governance](GOVERNANCE.md) · [Security](SECURITY.md) · [Support](SUPPORT.md)

</details>

---

## Why DRAForge?

Kubernetes Dynamic Resource Allocation (DRA) offers fine-grained, driver-controlled accelerator sharing. However, developing and debugging DRA configurations presents a major challenge:
1. **Hardware Scarcity**: Acquiring and configuring dedicated accelerator nodes (e.g., NVIDIA H100 GPUs) for test environments is costly and slow.
2. **Observability Gap**: Native Kubernetes scheduling logs make it difficult to visualize why a resource claim failed to bind to a pod.

DRAForge bridges this gap by providing an evidence-based diagnostics toolkit, a dynamic virtual device simulator, a terminal user interface (TUI), and a real-time interactive relationship graph dashboard.

---

## Features

- **Virtual Device Pools**: Simulate arbitrary hardware profiles (e.g. GPUs, FPGAs, High-Speed NICs) on worker nodes using custom attributes and capacities.
- **Diagnostics Doctor**: Honest, non-mocked configuration analysis (e.g. API availability, version compatibility, ResourceSlice consistency checks).
- **Explain Engine**: Real-time evaluation of selectors, capacity bounds, and node affinity to pinpoint why claims are pending.
- **Bubble Tea TUI**: Professional terminal-based monitor for dynamic pool capacities.
- **Interactive Graph Dashboard**: Real-time SVG visualization of relationships between Pods, Claims, Devices, and Pools.

---

## Architecture

```mermaid
graph TD
    subgraph Kubernetes Cluster
        Server[DRAForge Server] <--> WebSPA[Vite + React SPA Dashboard]
        Controller[DRAForge Controller] <--> SimulatedDevicePool[SimulatedDevicePool CRD]
        Plugin[Node Plugin DaemonSet] --> ResourceSlice[ResourceSlice Spec]
        APIServer[Kubernetes API Server] <--> Server
        APIServer <--> Controller
        APIServer <--> Plugin
    end
    CLI[DRAForge CLI] <--> APIServer
```

DRAForge components use Kubernetes APIs and Helm contracts rather than provider-specific services. They can run on compatible managed Kubernetes offerings, self-managed clusters, on-premises environments, and local test clusters. Provider-specific Terraform or registry assets in this repository are optional showcases.

---

## Install

### From source (Go 1.26+)

```bash
go install github.com/oaslananka/draforge/cmd/draforge@latest
```

Or clone and build locally:

```bash
git clone https://github.com/oaslananka/draforge.git
cd draforge
task build          # Builds all three binaries into bin/
./bin/draforge version
```

Binaries are also available as pre-built archives from the [GitHub Releases](https://github.com/oaslananka/draforge/releases) page.

## Detailed installation recipes

### Local kind cluster (expanded)

For the complete local stack, install **Bash** (WSL on Windows), a running **Docker daemon**, **kubectl**, **Helm**, **jq**, **curl**, **sha256sum**, and the exact **kind** version pinned in `tests/install-e2e/kubernetes-versions.json`. Go is only needed if you also build the standalone CLI/TUI; Node.js and pnpm are only needed for optional local frontend/browser tests. The container image build brings its own toolchains.

**Windows/WSL:** Docker Desktop working in PowerShell does not prove that Docker works inside Ubuntu/WSL. Use the [Docker Desktop WSL 2 backend and per-distribution integration](https://docs.docker.com/desktop/features/wsl/), then run **`docker info` in the same WSL terminal** before starting. The quickstart now reports missing tools together and refuses to create a kind cluster if the Docker daemon is unavailable. Native Linux Docker Engine works with the same `docker info` check.

Clone the repository and create the disposable kind stack:

```bash
git clone https://github.com/oaslananka/draforge.git
cd draforge
DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 bash scripts/kind-install-e2e.sh pull-request
```

If you already use Task, the equivalent command is `DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind`. The installer **refuses to replace an existing kind cluster**, including a previous `draforge-install-e2e` cluster: inspect the existing cluster, choose a unique `DRAFORGE_INSTALL_E2E_CLUSTER_NAME`, or deliberately remove it before retrying. It only cleans up clusters it successfully created itself.

Access the dashboard:

```bash
kubectl port-forward svc/draforge-server -n draforge-system 8080:8080
```

Open **[http://localhost:8080](http://localhost:8080)** on the same machine to explore the live, read-only DRAForge dashboard (Claims, Graph and Explain). This is a local connection, **not a publicly hosted demo**.

Build the local CLI/TUI when needed (requires Go 1.26+):

```bash
go build -o bin/draforge ./cmd/draforge
./bin/draforge doctor
./bin/draforge tui
```

Destroy the disposable cluster when finished:

```bash
kind delete cluster --name draforge-install-e2e
```

### Optional DigitalOcean Kubernetes (DOKS) showcase

This provider-specific path is an optional demonstration of DRAForge on one managed Kubernetes service. It is not required for installation, testing, or releases.

> **⚠️ Billable Resources**: This task provisions a live DOKS cluster and DOCR registry on your DigitalOcean account and incurs cloud costs. Always run `task demo:down` when finished to destroy all billable resources.

To deploy the short-lived DOKS showcase:

```bash
task demo:up
```

This task explicitly applies the non-production `values-showcase-docr.yaml` profile, which enables unauthenticated HTTP exposure. It audits resource limits, provisions infrastructure, builds images remotely, installs the Helm release, and outputs the demo URL. Do not use this profile with sensitive clusters or as a production deployment.

A normal Helm install creates only internal ClusterIP services. Production public access requires TLS and an operator-managed OIDC/identity-aware proxy; see the [installation guide](docs/operations/install.md).
The node plugin defaults to isolated `demo` CDI output. Host-integrated kubelet CDI output is an explicit `nodePlugin.outputMode=node` opt-in and fails closed when Kubernetes or the host CDI directory is unavailable.

To tear down the showcase and clean all billable resources:
```bash
task demo:down
```

## Testing

```bash
# Run all unit tests (fast, no race)
go test ./...
# or: task test:unit

# Run unit tests with race detector and coverage
go test -race -coverprofile=coverage.out ./...
# or: task test:race

# Run all Go vet checks
go vet ./...
# or: task vet

# Run frontend unit and integration tests
pnpm --dir web test
# or: task web:test

# Run full Go CI suite (unit + race)
task test
```

The tagged smoke tests run against any compatible Kubernetes cluster that serves the required DRA APIs and are gated by `DRAFORGE_E2E=1`. The `Portable Kubernetes E2E` workflow validates this path on credential-free kind for relevant pull requests and can use a protected short-lived kubeconfig for an external cluster:

```bash
DRAFORGE_E2E=1 go test -tags=e2e ./tests/e2e/... -v
```

## Release

See [docs/release.md](docs/release.md) for the full release process.

### Local snapshot build (no publishing)

```bash
task release:local
# or: goreleaser release --snapshot --clean --skip=docker,sbom,sign
```

### Tagged release

```bash
NEXT_VERSION="${NEXT_VERSION:?set NEXT_VERSION to the intended SemVer, for example 0.3.1}"
release_tag="v${NEXT_VERSION}"
git tag -a "$release_tag" -m "DRAForge $release_tag"
RELEASE_TAG="$release_tag" RELEASE_MAIN_REF=main bash scripts/verify-release-tag.sh
git push origin "$release_tag"
```

Pushing the annotated tag starts the protected release workflow. Published `v*` tags are immutable and are never moved or reused.

---

## CLI Command Reference

| Command | Description | Example |
|---------|-------------|---------|
| `draforge version` | Print binary version and commit details | `draforge version` |
| `draforge discover` | Lists active DRA pools, devices, and resource claims | `draforge discover -o json` |
| `draforge claims` | Summarizes claims status in a formatted table | `draforge claims` |
| `draforge graph` | Generates relationship graph in DOT or Mermaid formats | `draforge graph -o mermaid` |
| `draforge explain` | Troubleshoots why a ResourceClaim is pending | `draforge explain my-pending-claim` |
| `draforge doctor` | Executes cluster and driver diagnostics checks | `draforge doctor` |
| `draforge tui` | Launches the terminal-based monitor console | `draforge tui` |
| `draforge serve` | Runs HTTP API server and Vite dashboard frontend | `draforge serve` |

---

## Documentation

| Resource | Description |
|----------|-------------|
| [Contributing Guide](CONTRIBUTING.md) | Setup, testing, PR expectations, and what not to commit |
| [Security Policy](SECURITY.md) | Vulnerability reporting and security model |
| [Installation Guide](docs/operations/install.md) | Demo and production installation profiles |
| [Operations Guide](docs/operations/guide.md) | Day-two operations, upgrades, metrics, logs, and cleanup |
| [Troubleshooting Guide](docs/operations/troubleshooting.md) | Diagnostics for common issues and pending claims |
| [Maintainer Checklist](docs/maintainer-checklist.md) | Internal review and release processes |
| [Release Process](docs/release.md) | Snapshot and tagged release workflow |
| [Dashboard Guide](docs/dashboard.md) | Web dashboard setup and usage |
| [Simulator Scenarios](docs/simulator-scenarios.md) | Scenario authoring and fault injection |
| [Kubernetes DRA Compatibility](docs/compatibility/kubernetes-dra.md) | DRA API support matrix |
| [Maintainers](MAINTAINERS.md) | Current project maintainers |
| [Governance](GOVERNANCE.md) | Decision-making and roles |
| [Support](SUPPORT.md) | How to get help |

---

## Security Model

DRAForge exposes a read-only dashboard API, but Helm does not expose it externally by default. Read endpoints can reveal operational cluster metadata, so production public access must use TLS and an operator-managed identity-aware proxy. Cluster modifications remain CLI-only and authenticate with the administrator's local `kubeconfig` (see [ADR-0009](docs/adr/0009-public-readonly.md)).

For Kubernetes DRA API support and known limitations, see [Kubernetes DRA Compatibility](docs/compatibility/kubernetes-dra.md).

---

## License

Project-authored source is licensed under [GNU AGPL v3.0](LICENSE) (AGPL-3.0-only).

---
