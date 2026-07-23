# Contributing to DRAForge

<!-- SPDX-License-Identifier: Apache-2.0 -->

Welcome! We are excited that you want to contribute to DRAForge.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Project Scope

DRAForge is a Dynamic Resource Allocation (DRA) observability, simulation, and diagnostics
platform for Kubernetes. It helps developers model, simulate, and diagnose cluster hardware
resource allocations (GPUs, edge devices, smartNICs) without requiring physical hardware.

**In scope:**
- DRA observability and diagnostics
- Virtual device simulation via CRDs
- Relationship graph visualization (TUI + web dashboard)
- CLI tooling for DRA resource inspection

**Out of scope:**
- Implementing a real kubelet DRA plugin
- Production-grade ResourceSlice lifecycle management
- Persistent storage for dashboard history
- Authentication/authorization for the web dashboard

## Local Development Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/oaslananka/draforge.git
   cd draforge
   ```

2. **Install Go toolchain** (1.26+):
   ```bash
   go version
   ```

3. **Install Task** (task runner):
   ```bash
   go install github.com/go-task/task/v3/cmd/task@latest
   ```

4. **Install pnpm** for the web dashboard:
   ```bash
   corepack enable && corepack prepare pnpm@11.5.2 --activate
   cd web && pnpm install --frozen-lockfile
   ```
   The workspace enforces a seven-day package maturity window and only permits the allowlisted `esbuild` lifecycle step.
Automatic peer installation is disabled; required peers must be declared explicitly and `pnpm --dir web peers check` must remain clean.

5. **Verify setup:**
   ```bash
   task build
   go test ./...
   ```

## Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.26+ | Compiler and runtime |
| Task | latest | Task runner (`task build`, `task test`, etc.) |
| pnpm | 11.5.2 | Web dashboard package management |
| Docker | latest | Container image builds (release only) |
| GoReleaser | v2.x | Release automation (maintainer use) |
| Syft | latest | SBOM generation (release only) |
| Kind | optional | Local Kubernetes cluster for testing |

## Common Commands

```bash
task build          # Build all three binaries
task fmt            # Format Go and Terraform code
task lint           # Lint Go code with golangci-lint
task vet            # Run go vet
task test           # Run unit tests with race detector
task web:install    # Install web dashboard dependencies
task web:audit      # Audit frontend dependencies
task web:test       # Run frontend unit and integration tests
task web:lint       # Lint web dashboard
task web:build      # Build web dashboard for production
task security:verify-actions # Verify immutable GitHub Action references
task security:verify-workloads # Verify workload token and storage limits
task helm:lint      # Lint Helm charts
task helm:verify-images # Verify public, digest, DOCR, and multi-arch image contracts
task helm:verify-lifecycle # Verify readiness and graceful shutdown Helm settings
task helm:verify-sim-driver # Verify demo/node CDI output and health probes
task helm:verify-exposure # Verify disabled, demo, and secure public exposure profiles
task helm:verify-metrics # Verify restricted controller metrics scraping
task sbom           # Generate CycloneDX SBOM (requires syft)
task release:local  # GoReleaser snapshot build (requires goreleaser)
```

Direct Go commands (also work without Task):

```bash
go test ./...           # Run all unit tests with race detector
go vet ./...            # Static analysis
go build -o bin/ ./...  # Build all binaries
```

## Testing

### Unit Tests

Run all unit tests with race detection:

```bash
task test
# or: go test -race -coverprofile=coverage.out ./...
```

### End-to-End Tests

E2E tests require a real or Kind-based Kubernetes cluster with DRA feature gate
enabled and are guarded by the `DRAFORGE_E2E` environment variable:

```bash
DRAFORGE_E2E=1 go test -tags=e2e ./tests/e2e/... -v
```

**Note:** E2E tests are excluded from `task test` / `go test ./...` by default.
You must set `DRAFORGE_E2E=1` explicitly to run them.

## Web Dashboard Development

The web dashboard is a Vite + React + TypeScript application located in `web/`.

```bash
cd web
pnpm install --frozen-lockfile  # Install dependencies
pnpm dev                        # Start dev server (hot-reload on :5173)
pnpm audit --audit-level high     # Dependency advisory gate
pnpm test                         # Unit and integration tests
pnpm lint                         # ESLint check
pnpm build                        # Production build to web/dist/
```

The frontend test command uses Vitest, a repository-owned linkedom environment, and React Testing Library. The `web/vendor/cssom` workspace package implements only the stylesheet methods exercised by linkedom tests; do not replace it with the scanner-flagged npm `cssom` package. Critical tests mock API and EventSource boundaries, so they do not require a Kubernetes cluster. New navigation, query-state, SSE, graph-selection, or diagnostics behavior must include a deterministic regression test.

The dev server proxies API requests to the Go backend running on port 8080.
Start the Go server separately with `draforge serve` or `task build && ./bin/draforge serve`.

## Kubernetes / DRA Testing

To test against a real Kubernetes cluster:

1. Ensure your cluster has the DRA feature gate enabled (Kubernetes 1.26+ with
   `DynamicResourceAllocation` feature gate).
2. Apply the DRAForge CRDs:
   ```bash
   kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
   ```
3. Apply a test scenario:
   ```bash
   kubectl apply -f examples/scenarios/basic-gpu.yaml
   ```
4. Run the CLI:
   ```bash
   ./bin/draforge discover
   ./bin/draforge doctor
   ./bin/draforge tui
   ```

## Simulator Scenarios

See [docs/simulator-scenarios.md](docs/simulator-scenarios.md) for detailed documentation
on writing and applying simulator scenarios.

Quick reference:

```bash
# Apply a scenario
./bin/draforge scenario apply -f examples/scenarios/basic-gpu.yaml

# Inject a fault (unhealthy, capacity-exhausted, disappear)
./bin/draforge inject-fault --pool my-pool --type unhealthy

# Clear faults
./bin/draforge clear-faults --pool my-pool

# Reset all scenarios
./bin/draforge scenario reset
```

## Release Preparation

See [docs/release.md](docs/release.md) for the full release process.

Snapshot builds (no publishing):

```bash
goreleaser release --snapshot --clean --skip=publish --skip=sign
```

For maintainers cutting a tagged release:

```bash
git tag v0.x.x
git push origin v0.x.x
goreleaser release --clean
```

## Pull Request Expectations

- Create a feature branch from `main`.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.
- All tests must pass before requesting review.
- Add or update tests for new or changed code.
- Update documentation when adding or changing features.
- Keep the scope of each PR focused on a single concern.
- Do not include generated artifacts (`dist/`, `bin/`, `*.sbom.json`, `coverage.out`).
- Do not include unrelated formatting changes.

## Security Expectations

- If you discover a potential security vulnerability, **do not open a public issue**.
  See [SECURITY.md](SECURITY.md) for the responsible disclosure process.
- Do not commit secrets, tokens, or credentials in any file.
- Do not include kubeconfig contents or API tokens in issue reports, logs, or examples.
- The web dashboard is intentionally read-only — mutations require CLI + kubeconfig.

## What Not to Commit

The following files and directories are generated and must not be committed:

- `dist/` — GoReleaser output directory
- `bin/` — Local build output
- `*.sbom.json` — SBOM files
- `coverage.out` — Test coverage output
- `web/dist/` — Web dashboard production build
- `web/node_modules/` — JavaScript dependencies
- IDE / editor config files
- Compiled binaries
- Secrets, tokens, or credentials

These are covered by `.gitignore` — please ensure they stay excluded.

## Local Validation Checklist

Before submitting a pull request, run the following locally:

```bash
# Format and lint
task fmt && task lint

# Static analysis
task vet

# All unit tests (fast)
task test:unit

# Race detection + coverage
task test:race

# Web dashboard (if changed)
task web:audit && task web:test && task web:lint && task web:build
```

All of the above must pass without errors. The CI pipeline (`.github/workflows/ci.yml`)
runs these same checks on every push and pull request.

## Production Readiness

See the [Maintainer Checklist](docs/maintainer-checklist.md) for the full PR review
and release readiness checklist. Key principles:

- **Tests**: Every new or changed feature must include tests.
- **Docs**: Every new or changed feature must update relevant documentation.
- **No generated artifacts**: Never commit `dist/`, `bin/`, `*.sbom.json`,
  `coverage.out`, `web/dist/`, or `web/node_modules/`.
- **No secrets**: Never commit tokens, passwords, API keys, or kubeconfig contents.
- **Security**: Review changes for injection vectors, secret exposure, and
  unsafe `os/exec` usage.

The `FINAL_REPORT.md` at the repository root contains the latest production-readiness
audit with known gaps and GitHub-side configuration requirements.

## Getting Help

- **Documentation**: See the `docs/` directory.
- **GitHub Issues**: Search existing issues or open a new one for bugs.
- **Discussions**: Use GitHub Discussions for Q&A and general topics.
- **Security**: Email `oaslananka@gmail.com` for security issues only.
