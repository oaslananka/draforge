# DRAForge Agent Instructions

These instructions apply repository-wide. A nested `AGENTS.md` narrows or adds rules for its subtree; the closest applicable file wins.

Nested instructions may not weaken security, release, evidence, Kubernetes safety, or product-truth constraints unless the underlying policy is intentionally changed in the same PR.

## Repository purpose

DRAForge is a provider-neutral Kubernetes Dynamic Resource Allocation observability, simulation, and diagnostics project. It includes Go control-plane components, a Vite/React dashboard, Helm/Kubernetes deployment assets, optional showcase infrastructure, release tooling, and extensive E2E policy.

Do not describe the simulator as a production kubelet DRA plugin, physical accelerator evidence, or provider qualification.

## Nested boundaries

- `.github/AGENTS.md` — CI, security, release, provenance and workflow permissions.
- `scripts/AGENTS.md` — policy-as-code, E2E orchestration, verification and release checks.
- `internal/AGENTS.md` — Go runtime/controller/server/node-plugin contracts.
- `web/AGENTS.md` — dashboard/browser/accessibility/SSE behavior.
- `deploy/AGENTS.md` — Helm/Kubernetes runtime and exposure policy.
- `infra/AGENTS.md` — optional Terraform showcase infrastructure and billable-resource safety.

Do not add nested files merely because a directory exists.

## Product truth and safety

- KiCad-style hardware claims are not relevant here; DRAForge evidence concerns Kubernetes DRA behavior only.
- Simulated devices are simulation evidence, not physical accelerator validation.
- Read-only dashboard exposure remains internal by default; public production access requires the documented TLS and identity-aware proxy model.
- Cluster-modifying CLI/controller behavior must preserve Kubernetes API/RBAC intent and fail closed on unsupported DRA capability.
- Host-integrated CDI output is explicit opt-in; demo-mode behavior must not silently become node-integrated behavior.
- Optional DOKS showcase paths can incur real cost. Never run or describe them as safe default local validation.

## Development and verification

Use repository-owned commands:

```bash
task build
task fmt
task lint
task vet
task test
task web:test
task web:lint
task web:build
task helm:lint
task e2e:install-contract
```

Use `task e2e:install-kind` only when Docker/kind/kubectl/Helm prerequisites are intentionally available. Physical/external-cluster evidence must never be inferred from unit or contract tests.

## Architecture discipline

- Keep provider-neutral core behavior independent of optional showcase/provider integrations.
- Preserve deterministic diagnostics/explain behavior where tests encode contracts.
- Keep web UI as a consumer of the Go API contract rather than duplicating Kubernetes decision logic.
- Preserve Kubernetes version/DRA API compatibility and module-alignment checks.
- Do not bypass existing image, workload-security, exposure, lifecycle, metrics, E2E or release verifiers.

## Dependencies and generated state

- Go dependencies stay in `go.mod/go.sum`; frontend dependencies stay in `web/package.json` and `web/pnpm-lock.yaml`.
- Do not weaken package-maturity, peer, lifecycle-script or high-severity advisory policy.
- Third-party Actions remain pinned to reviewed immutable SHAs.
- Generated docs, fixture outputs, Helm packages, SBOMs and release artifacts must be produced by their owning tools.

## Release integrity

- Published tags are immutable.
- Release tag provenance, main ancestry, install E2E, checksums, SBOMs, signatures and public image identity must agree.
- Do not recover/publish releases from an unreviewed branch or replace released bytes under an existing tag.

## Definition of done

A change is ready when implementation, focused tests, docs/contracts, generated state and exact-head CI agree. Report any environment-dependent E2E/HIL-style evidence that did not run.
