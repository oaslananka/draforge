# ADR-0009: Public Read-Only Security Model

- **Status**: Approved
- **Context**: The showcase dashboard is publicly accessible, but must not allow unauthorized changes.
- **Decision**: Expose a read-only HTTP server API for the public dashboard. Admin mutations (like fault injection or scenario resets) must be protected by authenticating against the Kubernetes API (RBAC).
- **Alternatives**: No public dashboard or public write APIs.
- **Consequences**: Public users can view, but not modify, cluster DRA state.
- **Security Considerations**: Strict CORS, CSP, secure headers, and API rate limits.
- **Operational Considerations**: Runs in a read-only container filesystem.
