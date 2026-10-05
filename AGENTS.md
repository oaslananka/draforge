# DRAForge Agent Instructions

These instructions apply repository-wide. A nested `AGENTS.md` may narrow or add rules for its subtree; the closest applicable file wins for local implementation details.

Nested instructions may not weaken repository-wide security, Kubernetes authority, release integrity, evidence, product-truth, or CI governance unless the underlying policy is intentionally changed in the same pull request.

## Repository purpose

DRAForge is a provider-neutral Kubernetes Dynamic Resource Allocation observability, diagnostics, simulation, and test platform.

The repository contains several distinct authority surfaces:

- read-only discovery, doctor, graph, explain, TUI, and dashboard paths;
- a Kubernetes controller that reconciles `SimulatedDevicePool` and may update simulated DRA state;
- a node-side simulator/plugin with optional host-integrated CDI output;
- Helm, CRDs, optional Terraform showcase infrastructure, and install/e2e assets;
- release, security, supply-chain, and CI policy automation.

Do not turn a simulator or diagnostic result into a claim of real accelerator availability, production qualification, scheduler correctness, or provider certification.

## Nested boundaries

Read the root instructions plus the closest relevant file:

- `.github/AGENTS.md` — CI, security, required checks, release, provenance, and workflow permissions.
- `scripts/AGENTS.md` — repository policy checks, release validators, remote/e2e harnesses, and infrastructure validation.
- `internal/server/AGENTS.md` — read-only HTTP/SSE API boundary.
- `internal/simulator/AGENTS.md` — simulated allocation/controller mutation semantics.
- `cmd/draforge-sim-driver/AGENTS.md` — node plugin, readiness, and CDI output boundary.
- `deploy/AGENTS.md` — Helm, CRDs, deployment profiles, and optional Terraform showcase.

Do not add one instruction file per package. Add a nested boundary only when authority, security, release, or operational semantics materially differ.

## Read first

Before changing behavior, read the relevant implementation/tests plus:

- `README.md`
- `docs/architecture.md`
- `docs/compatibility/kubernetes-dra.md`
- `docs/operations/install.md`
- `docs/operations/guide.md`
- `docs/release.md`
- `SECURITY.md`
- the relevant ADR under `docs/adr/**`

The checked-in scripts and tests are executable policy. If documentation and enforcement drift, fix them together rather than bypassing either side.


## Verified toolchain and common commands

Use repository pins instead of floating `latest` installs when a version is governed by the repository:

- Go `1.26.6` from `go.mod`;
- Node.js `22` and pnpm `11.5.2` in CI;
- kind `v0.32.0` from `tests/install-e2e/kubernetes-versions.json`;
- golangci-lint `v2.12.2`;
- Helm `v3.17.3`;
- Terraform `1.7.0`;
- GoReleaser `v2.16.0`.

Common local checks:

```text
task build
task lint
task vet
task vuln
task test:unit
task test:race

task web:install
task web:audit
task web:test
task web:lint
task web:build

task docs:verify
task e2e:install-contract
task e2e:install-kind
```

`task e2e:install-kind-full` runs every policy-pinned local kind compatibility target sequentially; it is expensive but not a cloud/billable workflow. For the broad workstation gate, run `bash scripts/local-quality-gate.sh`.

## Product truth

- DRAForge observes and simulates Kubernetes DRA behavior; it does not prove real hardware availability or vendor-driver correctness.
- Provider-specific Terraform and registry integrations are optional showcase adapters, not the core platform contract.
- The default dashboard path is read-only. Do not add write/create/update/delete cluster operations to the web API without an explicit architecture/security decision.
- CORS is not authentication or authorization.
- The unauthenticated HTTP showcase profile is explicitly non-production and must not be presented as a secure production deployment.
- Hardware/CDI demonstrations, kind tests, synthetic scenarios, and external-cluster e2e runs are different evidence classes. Do not present one as another.

## Kubernetes authority and RBAC

- Preserve least-privilege RBAC. Do not broaden verbs/resources merely to make an operation easier.
- Kubernetes API errors that affect controller or node-plugin correctness must not be silently converted into success.
- Cluster-scoped and ambiguous allocation identity must remain explicit; do not invent a node association when the source contract does not prove one.
- Preserve full device identity where the implementation uses driver/pool/device tuples.
- Mutation paths must keep ownership, conflict, reconciliation, and status-update semantics deterministic and testable.

## Security

- Treat kubeconfigs, tokens, cloud credentials, registry credentials, signed release material, and private cluster metadata as sensitive.
- Never commit or print kubeconfig contents or secret values.
- Production public dashboard access requires operator-managed TLS and authentication/identity-aware proxying.
- Preserve hostPath, service-account-token, seccomp, read-only-rootfs, privilege-escalation, capability, and resource-limit controls documented in `SECURITY.md`.
- Do not weaken path traversal/symlink protections in Terraform-plan, archive, artifact, or filesystem validation.
- External-cluster e2e paths are privileged and environment-dependent; cloud showcase e2e paths are additionally billable. Do not run either without explicit task intent and the required protected credentials/approval.

## Local verification

Use the narrowest relevant checks first:

```text
task test:unit
task test:race
task vet
task vuln
task web:audit
task web:test
task web:lint
task web:build
task build
```

For deployment/release/security work, use the owning verification scripts and CI lanes. The full local quality gate and install/e2e matrices are more expensive and should be used when the touched boundary requires them.

Environment-dependent cluster tests must be reported accurately. A compile-only or kind result is not external-cluster evidence.

## Architecture discipline

- Keep reusable DRA/domain logic out of transport/UI code when an internal package already owns it.
- Read-only dashboard/server paths must not become a second mutation/control surface.
- Controller reconciliation must remain idempotent and safe under retries.
- Node-plugin/CDI behavior must preserve last-known-good output and readiness semantics when documented.
- Web frontend code should consume the server contract rather than duplicating Kubernetes authority logic.
- Do not introduce provider-specific assumptions into core discovery, explanation, graph, simulator, server, or controller logic.

## Generated and governed state

- Do not hand-edit generated CRDs, packaged charts, release assets, SBOMs, checksums, or other derived state when an owning generator exists.
- Do not update coverage floors, policy scripts, security controls, allowlists, support tables, or expected fixtures merely to hide a regression.
- Keep generated output deterministic and review semantic diffs.

## GitHub and release integrity

- Third-party Actions remain pinned to reviewed full SHAs.
- Required status contexts and aggregate gates must keep reporting unless branch protection is intentionally migrated.
- Publishing/signing/provenance credentials remain confined to protected release workflows.
- Release tags are immutable and must resolve to reviewed `main` history.
- Checksums, SBOMs, images, archives, chart artifacts, attestations, and release metadata must bind to the same release identity.
- Do not trigger a release or cloud deployment unless explicitly requested.

## Definition of done

A change is ready when:

1. the diff is scoped to the intended subsystem;
2. the relevant nested instructions were followed;
3. focused regression tests cover the changed contract;
4. Kubernetes/RBAC/security semantics are not weakened;
5. generated artifacts are refreshed only through their owning process;
6. docs/support/release notes are updated when the public contract changed;
7. exact-head CI is green, or any environment-dependent/manual evidence gap is stated explicitly.
