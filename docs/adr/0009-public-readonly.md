# ADR-0009: Read-Only API Is Not Anonymous-by-Default

- **Status**: Approved
- **Context**: DRAForge does not expose mutation APIs in the dashboard, but read endpoints reveal cluster topology, identities, allocation state, device attributes/capacities, diagnostics, and event evidence.
- **Decision**: Keep the dashboard API read-only while disabling external exposure by default. Secure public routing must terminate TLS and pass through an operator-managed OIDC/identity-aware proxy. Administrative mutations remain CLI-only and authenticated through Kubernetes RBAC.
- **Alternatives**: Treat read-only data as safe for anonymous access, expose no dashboard, or add application-managed identity storage.
- **Consequences**: Local access uses port-forwarding; public operators must deploy and operate the identity proxy.
- **Security Considerations**: CORS, CSP, headers, and rate limits are defense in depth and do not replace authentication. Demo HTTP exposure is explicit and non-production.
- **Operational Considerations**: The authentication proxy must support long-lived SSE connections and route upstream to the internal DRAForge Service.
