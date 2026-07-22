# DRAForge

DRAForge is a Dynamic Resource Allocation (DRA) observability, simulation, and diagnostics platform for Kubernetes. It allows developers and administrators to model, simulate, and diagnose cluster hardware resource allocations (GPUs, edge devices, smartNICs) dynamically—without requiring physical accelerator hardware.

[![Go Version](https://img.shields.io/github/go-mod/go-version/oaslananka/draforge)](https://golang.org)
[![License](https://img.shields.io/github/license/oaslananka/draforge)](https://www.apache.org/licenses/LICENSE-2.0)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/oaslananka/draforge/badge)](https://securityscorecards.dev)

---

## Why DRAForge?

Kubernetes Dynamic Resource Allocation (DRA) offers fine-grained, driver-controlled accelerator sharing. However, developing and debugging DRA configurations presents a major challenge:
1. **Hardware Scarcity**: Acquiring and configuring dedicated accelerator nodes (e.g., NVIDIA H100 GPUs) for test environments is costly and slow.
2. **Observability Gap**: Native Kubernetes scheduling logs make it difficult to visualize why a resource claim failed to bind to a pod.

DRAForge bridges this gap by providing an evidence-based diagnostics registry, a dynamic virtual device simulator, a terminal user interface (TUI), and a real-time interactive relationship graph dashboard.

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
    subgraph DOKS Cluster
        Server[DRAForge Server] <--> WebSPA[Vite + React SPA Dashboard]
        Controller[DRAForge Controller] <--> SimulatedDevicePool[SimulatedDevicePool CRD]
        Plugin[Node Plugin DaemonSet] --> ResourceSlice[ResourceSlice Spec]
        APIServer[Kubernetes API Server] <--> Server
        APIServer <--> Controller
        APIServer <--> Plugin
    end
    CLI[DRAForge CLI] <--> APIServer
```

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

## Quickstart

### Quickstart A: Local Kind cluster development
To run DRAForge locally using a Go development environment and a `kind` cluster:

1. **Prerequisites**:
   - Install Go 1.26+
   - Install `kind` (with DRA feature gate enabled)
   - Install `task`

2. **Build and Deploy**:
   ```bash
   task build
   kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
   kubectl apply -f examples/scenarios/basic-gpu.yaml
   ```

3. **Launch TUI**:
   ```bash
   ./bin/draforge tui
   ```

### Quickstart B: DigitalOcean Kubernetes (DOKS) Showcase
> **⚠️ Billable Resources**: This task provisions a live DOKS cluster and DOCR registry on your DigitalOcean account and incurs cloud costs. Always run `task demo:down` when finished to destroy all billable resources.

To deploy a live read-only public showcase to DOKS:

```bash
task demo:up
```
This script audits resource limits, runs Terraform provisioners, builds images remotely via Kaniko, installs the Helm release, and outputs the live external URL.

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

End-to-end tests require a real Kubernetes cluster with DRA feature gate and are gated by `DRAFORGE_E2E=1`:

```bash
DRAFORGE_E2E=1 go test ./tests/e2e -v
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
git tag v0.x.x
git push origin v0.x.x
goreleaser release --clean
```

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

## Screenshots

Screenshots of the terminal UI and web dashboard are stored in [docs/assets/](docs/assets/) when available.

---

## Security Model

DRAForge enforces a read-only public API model. The Go web server exposes read-only endpoints and SSE streams to the dashboard, preventing any write actions or pod execution from the web UI. Cluster modifications (scenario application and fault injections) are strictly restricted to the CLI and authenticated using the administrator's local `kubeconfig` (see [ADR-0009](docs/adr/0009-public-readonly.md)).

For Kubernetes DRA API support and known limitations, see [Kubernetes DRA Compatibility](docs/compatibility/kubernetes-dra.md).

---

## License

This project is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full license text.