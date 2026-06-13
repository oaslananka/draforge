# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Standard developer quickstart and deployment documentation.
- Dynamic CDI generation for simulated devices.
- Reconciler fault honoring and custom resource status updates.
- Prometheus metrics `/metrics` handler on API server and reconciler.
- Interactive relationship SVG force-directed graph in React web dashboard.
- Conformance unit tests for explanation and discovery logic.

### Fixed
- Stable Dynamic Resource Allocation (DRA) API group references to `resource.k8s.io/v1`.
- Struct to nil comparisons for v1 API compatibility.
- Private container registry image pull secret configurations in Helm.

## [0.1.0] - 2026-06-13

### Added
- Initial release containing CLI, TUI, Simulation Controller, and private registry integration.
