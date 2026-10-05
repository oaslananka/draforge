# Server Boundary Instructions

These instructions apply to `internal/server/**` and supplement the root instructions.

## Authority model

The DRAForge dashboard API is intentionally read-only.

- Do not add Kubernetes create/update/delete/patch operations to the HTTP or SSE surface without an explicit reviewed architecture/security change.
- Read-only data can still be sensitive. Preserve the deployment model that keeps the service internal by default and requires operator-managed authentication/TLS for public access.
- CORS restrictions are browser policy, not authentication.
- Do not trust query parameters, path values, or client-provided identifiers as proof of Kubernetes authorization.

## Streaming and lifecycle

- SSE/watch paths must remain bounded, cancellable, and safe under client disconnects.
- Preserve server shutdown/readiness behavior and avoid goroutine/resource leaks.
- A transient watch/read error must not fabricate a healthy cluster state.
- Keep error payloads bounded and avoid leaking credentials, kubeconfig data, or unnecessary private cluster metadata.

## Contract discipline

The web UI consumes this server contract. API shape changes require focused server tests and corresponding frontend updates. Keep cluster/domain evaluation in the owning internal packages rather than duplicating policy in handlers.

## Verification

Run focused `internal/server` tests plus web/server integration and lifecycle checks when transport behavior changes.
