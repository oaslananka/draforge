# Changelog

All notable changes to DRAForge are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Operator-grade documentation in `docs/operations/` (install, guide, troubleshooting).

### Changed
- _None._

### Fixed
- Updated vulnerable Go and frontend development dependencies and added repeatable advisory gates to pull-request, scheduled security, and local validation workflows.
- Restored live SSE graph streaming through the full middleware chain while preserving ordinary request timeouts and non-blocking subscriber delivery.
- Preserved namespace-qualified ResourceClaim identity across dashboard selection, graph navigation, and explain API requests, including duplicate claim names across namespaces.
- Repaired the remote DOKS E2E harness with tagged test execution, result verification, run-scoped read-only RBAC, failure artifacts, and cancellation-safe cleanup.

---

## [0.2.0] - 2026-06-21

### Added
- Detailed Kubernetes DRA compatibility matrix comparing scheduling flows, parameters, and API versions.
- Quick-start troubleshooting instructions for missing APIs, feature gates, and SimulatedDevicePool CRDs.
- GitHub Actions remote E2E testing runbook documenting environment approvals, token scopes, manual checklists, cleanup logic, and common failures.
- Interactive relationship sidecard on Graph tab in dashboard to scan connections and navigate between components.
- Showcase demo walkthrough guidelines on documentation index page.
- Header Doctor diagnostics status indicator in web dashboard layout.
- `versionVal` / `commitSHA` ldflags injection targets for `draforge-controller` and `draforge-sim-driver` binaries.
- Explicit `snapshot` name template in `.goreleaser.yaml`.
- Release documentation at `docs/release.md`.
- `CHANGELOG.md` now tracks known limitations for v0.1.0.
- Unit tests for `pkg/model` (16 tests covering Device, DevicePool, ResourceClaimInfo, GraphNode, GraphEdge, ReasonNode, DoctorCheckStatus, ExplainResult, and JSON round-trips).
- Unit tests for `cmd/draforge-controller` (4 flag-contract tests).
- Unit tests for `cmd/draforge-sim-driver` (7 tests covering flag parsing, CDI data structures, and version vars).
- E2E workflow cancellation handler — cleanup step deletes Helm release and scenarios on workflow failure or cancellation to prevent DOKS resource leaks.
- E2E workflow input validation — `workflow_dispatch` now requires explicit `"run-e2e-doks"` confirmation before provisioning billable DigitalOcean resources.
- Production-readiness documentation in `SECURITY.md` (GitHub security features reference table, CI/CD security notes, secret scanning expectations).
- Local validation checklist and production readiness reference in `CONTRIBUTING.md`.
- Version injection documentation and snapshot CI reference in `docs/release.md`.
- "Install from Source" section in `README.md` with `go install` and build instructions.
- Testing and release command reference sections in `README.md`.

### Changed
- `doctor` CLI/API check (`StaleResourceSliceCheck`) now performs active Node existence and Ready status checks, reporting stale/unavailable slices.
- `explain` engine now verifies candidate device health and includes health rejections and remediation hints in the explanation tree.
- `.goreleaser.yaml`: snapshot behavior is now explicit with a custom name template.
- GoReleaser Dockerfiles audited — non-root user, Alpine base, minimal packages, correct binary paths, no local path assumptions. OCI labels handled via GoReleaser `build_flag_templates`.
- `SECURITY.md`: expanded with GitHub security feature recommendations (branch protection, required status checks, PR reviews, Dependabot, secret scanning).
- `CONTRIBUTING.md`: added local validation checklist and production readiness section referencing maintainer checklist.
- `docs/release.md`: added version injection section with ldflags table.
- `README.md`: restructured with Install, Testing, and Release sections.

### Fixed
- Candidate device health evaluation bug in explain engine.

---

## [0.1.0] - 2026-06-13

### Added
- Initial release containing CLI, TUI, Simulation Controller, and private registry integration.
- Standard developer quickstart and deployment documentation.
- Dynamic CDI generation for simulated devices.
- Reconciler fault honoring and custom resource status updates.
- Prometheus metrics `/metrics` handler on API server and reconciler.
- Interactive relationship SVG force-directed graph in React web dashboard.
- Conformance unit tests for explanation and discovery logic.
- **GoReleaser automation**: Three-binary release pipeline with cross-platform archives, checksums, CycloneDX SBOM, and multi-arch Docker images.
- **Helm chart**: Kubernetes deployment for the full DRAForge stack.
- **Terraform module**: DigitalOcean Kubernetes (DOKS) showcase deployment.
- **CLI commands**: `discover`, `claims`, `graph`, `explain`, `doctor`, `tui`, `serve`, `scenario`, `inject-fault`, `clear-faults`, `version`.
- **Kubernetes DRA compatibility audit**: Documented support matrix, RBAC hardening.

### Fixed
- Stable Dynamic Resource Allocation (DRA) API group references to `resource.k8s.io/v1`.
- Struct to nil comparisons for v1 API compatibility.
- Private container registry image pull secret configurations in Helm.

### Known Limitations (v0.1.0)
- **Not a real DRA driver**: DRAForge simulates device pools and allocations via CRDs and CDI specs. It does not implement a real kubelet DRA plugin — `ResourceSlice` objects are manually managed by the controller, not by a running kubelet plugin.
- **Single-controller architecture**: The controller runs as a single replica and holds in-memory state (reconciliation and allocation counters). Restart resets counters; no leader election is implemented.
- **No persistent storage**: The web dashboard does not persist history. SSE streams are ephemeral — on reconnect the graph rebuilds from current cluster state.
- **Dashboard authentication**: The web dashboard is unprotected by default. It binds to all interfaces and must be placed behind an ingress/identity proxy in production.
- **Terraform showcase only**: The Terraform module targets DOKS exclusively. Other Kubernetes providers require manual configuration.
- **Controller and sim-driver binaries**: These are headless and do not expose a `version` readout at runtime — version is embedded at build time via ldflags and visible only in the binary metadata (`go version -m`).
- **GoReleaser signing disabled by default**: Cosign signing is configured in `.goreleaser.yaml` but requires manual cosign setup and key management. Snapshot builds skip signing.

[Unreleased]: https://github.com/oaslananka/draforge/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/oaslananka/draforge/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/oaslananka/draforge/releases/tag/v0.1.0
