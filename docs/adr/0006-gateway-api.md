# ADR-0006: Gateway API Choice

- **Status**: Approved
- **Context**: Traditional ingress is deprecated in favor of the newer Gateway API.
- **Decision**: Implement Gateway API resources for exposing the public read-only dashboard, with a fallback to standard Ingress if Gateway controller is not fully supported on DOKS 1.36.
- **Alternatives**: Standard Kubernetes Ingress (legacy).
- **Consequences**: Modern routing API usage.
- **Security Considerations**: Centralized security, TLS termination, and request routing.
- **Operational Considerations**: Requires Gateway API CRDs installed on the cluster.
