# Go Runtime Instructions

Applies to `internal/**`.

This subtree contains the provider-neutral runtime, Kubernetes clients/controllers, server logic, simulator and node-plugin behavior.

- Preserve Kubernetes DRA API/version compatibility and exact resource identity handling.
- Unsupported/missing API capability must not be converted into optimistic success.
- Keep simulator behavior clearly distinct from physical-device or production-driver behavior.
- Cluster mutations must respect documented RBAC and namespace/resource ownership.
- Node-plugin CDI output defaults remain isolated/demo-safe; host-integrated output stays explicit opt-in and fail closed when prerequisites are absent.
- API/server read paths may expose operational cluster metadata; do not broaden public/auth assumptions beyond the documented deployment model.
- Keep retries, watches, polling and background loops bounded/cancellable.
- Deterministic explain/diagnostic output should remain stable where tests/contracts depend on it.

Run focused Go tests, `go vet ./...`, race tests when concurrency changes, and relevant E2E contracts for Kubernetes-facing behavior.
