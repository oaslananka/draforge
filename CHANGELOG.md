# Changelog

All notable changes to DRAForge are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `versionVal` / `commitSHA` ldflags injection targets for `draforge-controller` and
  `draforge-sim-driver` binaries.
- Explicit `snapshot` name template in `.goreleaser.yaml`.
- Release documentation at `docs/release.md`.
- `CHANGELOG.md` now tracks known limitations for v0.1.0.

### Changed
- `.goreleaser.yaml`: snapshot behavior is now explicit with a custom name template.
- GoReleaser Dockerfiles audited — non-root user, Alpine base, minimal packages,
  correct binary paths, no local path assumptions. OCI labels handled via GoReleaser
  `build_flag_templates`.

### Fixed
- _None in this release._

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
- **GoReleaser automation**: Three-binary release pipeline with cross-platform archives,
  checksums, CycloneDX SBOM, and multi-arch Docker images.
- **Helm chart**: Kubernetes deployment for the full DRAForge stack.
- **Terraform module**: DigitalOcean Kubernetes (DOKS) showcase deployment.
- **CLI commands**: `discover`, `claims`, `graph`, `explain`, `doctor`, `tui`, `serve`,
  `scenario`, `inject-fault`, `clear-faults`, `version`.
- **Kubernetes DRA compatibility audit**: Documented support matrix, RBAC hardening.

### Fixed
- Stable Dynamic Resource Allocation (DRA) API group references to `resource.k8s.io/v1`.
- Struct to nil comparisons for v1 API compatibility.
- Private container registry image pull secret configurations in Helm.

### Known Limitations (v0.1.0)
- **Not a real DRA driver**: DRAForge simulates device pools and allocations via CRDs and
  CDI specs. It does not implement a real kubelet DRA plugin — `ResourceSlice` objects are
  manually managed by the controller, not by a running kubelet plugin.
- **Single-controller architecture**: The controller runs as a single replica and holds
  in-memory state (reconciliation and allocation counters). Restart resets counters; no
  leader election is implemented.
- **No persistent storage**: The web dashboard does not persist history. SSE streams are
  ephemeral — on reconnect the graph rebuilds from current cluster state.
- **Dashboard authentication**: The web dashboard is unprotected by default. It binds to
  all interfaces and must be placed behind an ingress/identity proxy in production.
- **Terraform showcase only**: The Terraform module targets DOKS exclusively. Other
  Kubernetes providers require manual configuration.
- **Controller and sim-driver binaries**: These are headless and do not expose a `version`
  readout at runtime — version is embedded at build time via ldflags and visible only in
  the binary metadata (`go version -m`).
- **GoReleaser signing disabled by default**: Cosign signing is configured in
  `.goreleaser.yaml` but requires manual cosign setup and key management. Snapshot builds
  skip signing.

[Unreleased]: https://github.com/oaslananka/draforge/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/oaslananka/draforge/releases/tag/v0.1.0
