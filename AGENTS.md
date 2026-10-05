# AGENTS.md — DRAForge Build, Lint, and Test Commands

This file documents the exact commands ephemeral workers must use to build, lint, type-check, and test DRAForge. Every command here matches a `Taskfile.yml` task or a CI step in `.github/workflows/ci.yml`. Run the fast subset locally before submitting; CI runs the full suite.

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.26.6 (per `go.mod`) | `go install golang.org/dl/go1.26.6@latest && go1.26.6 download` |
| Task | latest | `go install github.com/go-task/task/v3/cmd/task@latest` |
| pnpm | 11.5.2 (per `web/package.json`) | `corepack enable && corepack prepare pnpm@11.5.2 --activate` |
| Node | 22 (per CI) | `nvm install 22` or system package |
| Docker | current stable | Required for `goreleaser`, `kind` E2E |
| kind | v0.32.0 (policy-pinned) | `go install sigs.k8s.io/kind@v0.32.0` (pin source: `tests/install-e2e/kubernetes-versions.json`) |
| golangci-lint | v2.12.2 (per CI) | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2` |
| GoReleaser | v2.16.0 (per CI) | `go install github.com/goreleaser/goreleaser/v2@v2.16.0` |
| Syft | latest | `go install github.com/anchore/syft/v2/cmd/syft@latest` |
| Helm | v3.17.3 (per CI) | `go install helm.sh/helm/v3/cmd/helm@v3.17.3` |
| shellcheck | latest | System package (`apt-get install shellcheck`) |

Add `~/go/bin` (or `$(go env GOPATH)/bin`) to `PATH` for Task, golangci-lint, GoReleaser, Syft, Helm, kind.

## Quick Start (Fast Local Checks)

```bash
# Go
task build              # Build all three binaries
task vet                # go vet ./...
task test:unit          # go test ./... (fast, no race)
go test -race ./pkg/... # One package with race detector

# Web (if web/ changed)
cd web
pnpm install --frozen-lockfile
pnpm test               # Type-check + unit/integration tests
pnpm lint               # ESLint
```

## Go Commands (via Task)

| Task | Direct Go Equivalent | Description |
|------|---------------------|-------------|
| `task build` | `go build -o bin/draforge ./cmd/draforge && go build -o bin/draforge-controller ./cmd/draforge-controller && go build -o bin/draforge-sim-driver ./cmd/draforge-sim-driver` | Build all binaries |
| `task build:windows` | Same with `.exe` suffix | Build for Windows |
| `task fmt` | `gofmt -w internal cmd pkg tests` | Format Go code |
| `task fmt:terraform` | `cd infra/terraform/environments/showcase && terraform fmt` | Format Terraform |
| `task lint` | `golangci-lint run ./...` | Lint Go (`.golangci.yml` config) |
| `task vet` | `go vet ./...` | Static analysis |
| `task vuln` | `go tool govulncheck ./...` | Vulnerability scan |
| `task test:unit` | `go test ./...` | Unit tests (fast, no race) |
| `task test:race` | `go test -race -coverprofile=coverage.out ./...` | Race + coverage |
| `task test` | `task test:unit && task test:race` | Default CI test flow |

**Coverage floor:** CI fails if total coverage < 80% (`go tool cover -func=coverage.out`).

## Web Commands (via Task)

| Task | Direct `pnpm` Equivalent | Description |
|------|--------------------------|-------------|
| `task web:install` | `cd web && pnpm install --frozen-lockfile` | Install deps (lockfile must match) |
| `task web:audit` | `cd web && pnpm audit --audit-level high` | High-severity advisory gate |
| `task web:test` | `cd web && pnpm test` | Type-check (`tsc --noEmit`) + Vitest |
| `task web:lint` | `cd web && pnpm lint` | ESLint (flat config, zero warnings) |
| `task web:test:browser:install` | `cd web && pnpm test:browser:install` | Install Playwright Chromium/Firefox/WebKit |
| `task web:test:browser` | `cd web && pnpm test:browser` | Real-browser accessibility/compat |
| `task web:build` | `cd web && pnpm build` | Production build to `web/dist/` |

## Security & Contract Verification (CI Gates)

These run in CI; run locally when touching related areas.

| Task | Script | Description |
|------|--------|-------------|
| `task security:verify-actions` | `scripts/verify-github-action-pins.sh` | GitHub Actions pinned to full SHAs |
| `task security:verify-kubernetes-modules` | `scripts/verify-kubernetes-module-alignment.sh` | DRA + Kubernetes modules single version |
| `task security:verify-workloads` | `scripts/verify-workload-security.sh` | Workload token + ephemeral-storage limits |
| `task security:verify-frontend-dependencies` | `scripts/verify-frontend-dependency-policy.sh` | Peer, local adapter, Docker install policy |
| `task docs:verify` | `python3 scripts/verify-documentation.py --self-test` | Links, commands, paths, versions, workflow claims |

## Helm Commands (via Task)

| Task | Direct Equivalent | Description |
|------|-------------------|-------------|
| `task helm:lint` | `helm lint deploy/helm/draforge` | Lint chart |
| `task helm:verify-images` | `scripts/verify-chart-images.sh && scripts/test-chart-image-verifier.sh` | Public, digest-pinned, DOCR, multi-arch |
| `task helm:verify-metrics` | `scripts/verify-controller-metrics-policy.sh` | Restricted metrics + ServiceMonitor |
| `task helm:verify-lifecycle` | `scripts/verify-runtime-lifecycle.sh` | Readiness + graceful shutdown |
| `task helm:verify-exposure` | `scripts/verify-dashboard-exposure.sh` | Disabled, demo, secure public profiles |
| `task helm:verify-sim-driver` | `scripts/verify-sim-driver-cdi.sh` | Demo + fail-closed node CDI modes |

## E2E Commands (via Task)

| Task | Description |
|------|-------------|
| `task e2e:install-contract` | Orchestration contract tests (no cluster) |
| `task e2e:install-kind` | Full stack on PR baseline kind cluster (requires Docker, kind, kubectl, Helm) |
| `task e2e:install-kind-full` | Run every policy-pinned local kind compatibility target sequentially |

## Infrastructure Commands (via Task)

| Task | Description |
|------|-------------|
| `task infra:preflight` | Validate Terraform showcase tools + static config |
| `task infra:init` | `terraform init` (showcase env) |
| `task infra:plan` | Generate plan + policy validation |
| `task infra:validate` | Validate plan via Python policy |
| `task infra:apply` | Audit cloud resources → apply → audit |
| `task infra:audit` | Audit DigitalOcean showcase resources |
| `task infra:cost` | Estimate showcase cost |

## Release & SBOM (Maintainer Use)

| Task | Description |
|------|-------------|
| `task sbom` | `syft dir:. -o cyclonedx-json > draforge.sbom.json` |
| `task release:local` | `goreleaser release --snapshot --clean --skip=docker,sbom,sign` |
| `task release:verify` | `scripts/verify-release.sh` (checksums, provenance, binaries, charts) |

## Demo (Billable Cloud — Do Not Run Automatically)

| Task | Description |
|------|-------------|
| `task demo:up` | Deploy live showcase to DOKS (Terraform + Helm + scenarios) |
| `task demo:down` | Tear down showcase + all billable infrastructure |

## Makefile Aliases

The `Makefile` delegates to Task. Common targets:

```bash
make build        # task build
make test         # task test
make fmt          # task fmt
make lint         # task lint
make vet          # task vet
make vuln         # task vuln
make web-install  # task web:install
make web-lint     # task web:lint
make web-build    # task web:build
make helm-lint    # task helm:lint
make sbom         # task sbom
make release-local # task release:local
make clean        # rm -rf bin/ dist/
```

## What Must Not Be Committed

Per `CONTRIBUTING.md` and `.gitignore`:

- `dist/` — GoReleaser output
- `bin/` — Local build output
- `*.sbom.json` — SBOM files
- `coverage.out` — Test coverage output
- `web/dist/` — Web production build
- `web/node_modules/` — JS dependencies
- IDE/editor config files (`.idea/`, `.vscode/`, `*.swp`, etc.)
- Compiled binaries
- Secrets, tokens, credentials, kubeconfig contents

## CI Pipeline Summary (`.github/workflows/ci.yml`)

| Job | Key Steps |
|-----|-----------|
| **go** | `go mod tidy` check → verify-kubernetes-module-alignment → verify-release-metadata → verify-documentation → golangci-lint → `go vet` → `govulncheck` → `go test ./...` → E2E smoke compile → shellcheck scripts → release tag provenance → install E2E contract/harness → `go test -race -coverprofile=coverage.out ./...` → coverage ≥ 80% |
| **coverage-upload** | Upload Go coverage to Codecov |
| **web** | Node 22 + pnpm 11.5.2 → `pnpm install --frozen-lockfile --ignore-scripts` → `pnpm audit --audit-level high` → `pnpm test` → `pnpm lint` → `pnpm build` |
| **browser** | Same setup → `playwright install --with-deps chromium firefox webkit` → `pnpm test:browser` |
| **helm** | Helm 3.17.3 → `helm lint` → template smoke tests → lint chart scripts → verify-action-pins → verify-ci-privilege-boundaries → verify-workload-security → verify-frontend-deps → verify-chart-images → test-chart-image-verifier → test-remote-build → verify-controller-metrics → verify-runtime-lifecycle → verify-dashboard-exposure → verify-sim-driver-cdi → `helm package` |
| **goreleaser** | Build web → QEMU + Buildx → GoReleaser dry-run → verify Docker v2 contract → verify snapshot artifacts |
| **infra** | Terraform 1.7.0 → validate variables → init → fmt check → validate → plan policy validation |

All jobs must pass for `ci-pass` to succeed.