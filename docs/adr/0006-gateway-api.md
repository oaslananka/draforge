# ADR-0006: Opt-In Gateway and Ingress Exposure

- **Status**: Approved
- **Context**: Gateway API is the preferred routing API, but automatically creating a public HTTP listener exposes operational cluster metadata without authentication.
- **Decision**: Default Helm installs create no Gateway or Ingress. An explicit local-demo profile may create an insecure HTTP Gateway. Secure public Gateway or Ingress profiles must terminate TLS and route to an operator-managed identity-aware proxy Service rather than directly to DRAForge.
- **Alternatives**: Automatic HTTP Gateway, direct public routing to the read-only API, or no supported external routing examples.
- **Consequences**: Upgrades require an explicit exposure profile; secure public deployments require an external OIDC proxy and TLS Secret.
- **Security Considerations**: TLS and authentication are mandatory for secure public profiles. Restrictive CORS is defense in depth, not identity enforcement.
- **Operational Considerations**: Gateway API CRDs are required for the Gateway profile; standard Ingress remains available as a controller-specific fallback.
