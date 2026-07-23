# Public API and CLI Surface

Before v1.0, it is crucial to establish clear boundaries for the DRAForge public API, CLI, Helm chart, and configurations.

This document outlines the exposed surfaces of DRAForge, their stability guarantees, and the change policies that govern them.

## HTTP API Surface

DRAForge exposes a strictly read-only HTTP API from its server component. This API is consumed by the web dashboard and potentially other visualization tools.

All endpoints are prefixed with `/api/` (except for `/metrics`, `/healthz`, and `/readyz`).

| Endpoint | Method | Stability | Description |
|---|---|---|---|
| `/api/summary` | GET | Stable | Cluster-wide overview statistics. |
| `/api/pools` | GET | Stable | List of active device pools (simulated or real). |
| `/api/devices` | GET | Stable | Discovered hardware/simulated devices. |
| `/api/claims` | GET | Stable | ResourceClaims and allocation status. |
| `/api/graph` | GET | Stable | Snapshot of the resource relationship graph. |
| `/api/explain?claim=X&namespace=default` | GET | Stable | Explanation tree for a claim's status. |
| `/api/doctor` | GET | Stable | Cluster diagnostic checks (PASS/WARN/FAIL). |
| `/api/stream` | SSE | Beta | Server-Sent Events stream for graph updates. |
| `/api/version` | GET | Stable | Binary build version and commit. |
| `/metrics` | GET | Stable | Prometheus-compatible telemetry. |
| `/healthz` | GET | Stable | Process liveness endpoint; independent from transient Kubernetes API outages. |
| `/readyz` | GET | Stable | Bounded Kubernetes API dependency readiness with failure grace and credential-safe JSON reasons. |

### ResourceClaim collection contract (v0.3)

`GET /api/claims`, `discover -o json`, graph claim metadata, and SSE graph snapshots expose complete DRA request and allocation identity. The v0.3 payload adds these fields to each `ResourceClaimInfo` object:

- `requests[]`: every top-level request, its mode (`Exactly` or `FirstAvailable`), and every alternative's name, `deviceClassName`, `allocationMode`, and count.
- `allocations[]`: every allocation result with `request`, `driverName`, `poolName`, `deviceName`, and optional `nodeName`.

Allocation identity is the tuple `<driver>/<pool>/<device>` plus a node only when the claim's `NodeSelector` identifies one exact node. Pool names are never treated as node names. Cluster-scoped or ambiguous allocations therefore omit `nodeName` instead of inventing one.

The legacy `deviceClassName`, `allocatedDevice`, `allocatedDriver`, and `allocatedNode` fields remain as deprecated projections of the first request/allocation for pre-v0.3 clients. They may be removed in v1.0; new clients must consume the collections. This is an additive minor-version change under the API Change Policy below.

`GET /api/devices` now derives `id` from the complete driver, node, pool, and device tuple using a deterministic length-encoded representation. Graph pool/device/allocation IDs use the same collision-safe principle. Clients must treat these IDs as opaque stable identifiers rather than parse their textual encoding. This replaces the former slice-name-based device ID and is the documented v0.3 identity migration.

### Health endpoint contract

`/healthz` returns HTTP `200` while the process and HTTP handler are alive. It does not contact Kubernetes and should be used for liveness probes.

`/readyz` performs a read-only, context-bounded Kubernetes namespace list. The default dependency timeout is `2s`. A failed dependency check remains HTTP `200` with `status: "degraded"` during the default `15s` readiness grace period; continued failures return HTTP `503` with the stable reason `kubernetes_api_unavailable`. Successful checks immediately restore `status: "ready"` and record `lastSuccess`.

Readiness responses deliberately omit raw client errors, tokens, kubeconfig paths, API URLs, and credentials. Stable payload fields are `status`, `ready`, `degraded`, `reason`, `message`, `checkedAt`, and optional `lastSuccess`.

### API Change Policy
- **Stable Endpoints**: Breaking changes to payload structures (removing fields, changing types) require a major version bump. Additive changes are permitted in minor releases.
- **Beta Endpoints**: Subject to change or removal in minor releases without prior deprecation notice.

## CLI Surface

The `draforge` command-line interface provides administration and inspection tools.

| Command | Stability | Description |
|---|---|---|
| `version` | Stable | Prints version information. |
| `discover` | Stable | Lists DRA resources. |
| `claims` | Stable | Summarizes claim allocations. |
| `graph` | Stable | Exports graph (JSON, DOT, Mermaid). |
| `explain <claim>` | Stable | Troubleshoots claim allocation. |
| `doctor` | Stable | Runs cluster diagnostics. |
| `serve` | Stable | Starts the HTTP API and dashboard. |
| `tui` | Stable | Starts the terminal UI. |
| `scenario apply -f <file>` | Stable | Applies a SimulatedDevicePool scenario. |
| `scenario reset` | Stable | Cleans up all scenarios. |
| `inject-fault` | Beta | Injects a simulation fault. |
| `clear-faults` | Beta | Clears injected faults. |

### CLI Change Policy
- **Global Flags**: Standard flags (e.g., `--kubeconfig`, `--namespace`) are stable.
- **Command Output**: Human-readable terminal output is subject to change. Machine-readable formats (`-o json`) are stable.
- **Stable Commands**: Breaking changes require a major version bump.
- **Beta Commands**: Can be modified or removed in minor releases.

## Helm Chart and Configuration

The official DRAForge Helm chart defines the installation boundary.

| Values/Config | Stability | Description |
|---|---|---|
| `values.yaml` schema | Stable | Structure of values configuring server, controller, and simulator. |
| `DRAFORGE_METRICS_DETAIL` | Beta | Env var enabling high-cardinality metrics. |
| `CORS_ALLOWED_ORIGINS` | Stable | Comma-separated browser origins. It is defense in depth, not authentication. |

### Configuration Change Policy
- **Helm Values**: Renaming or removing top-level keys in `values.yaml` requires a major version bump. Adding new keys is allowed in minor versions.
- **Environment Variables**: Stable vars are guaranteed; beta vars may be renamed.

## CRD (Custom Resource Definitions)

| Group/Version | Kind | Stability | Description |
|---|---|---|---|
| `draforge.oaslananka/v1alpha1` | `SimulatedDevicePool` | Alpha | Configures the virtual device simulator. |

### CRD Change Policy
- CRDs follow standard Kubernetes versioning (`v1alpha1` -> `v1beta1` -> `v1`).
- Breaking changes in `v1alpha1` are allowed. Upon promotion to `v1`, structural changes require a new API version and conversion webhook.
