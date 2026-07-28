# ADR-0010: Remote build architecture

- **Status**: Superseded for required builds and tests by ADR-0013; retained for the optional DOKS showcase
- **Context**: The original showcase needed to build and test within a constrained DOKS environment.
- **Decision**: Use scripts that create sequential Kubernetes Jobs in the `draforge-ci` namespace to build binaries and container images with rootless builders.
- **Alternatives**: Dedicated Jenkins or GitLab CI runners, local builds, or hosted CI.
- **Consequences**: Sequential execution reduces showcase cluster contention but adds provider and registry integration overhead.
- **Security Considerations**: Credentials must not be logged and build jobs run non-root where supported.
- **Operational Considerations**: Completed remote jobs are cleaned up automatically.
- **Scope after ADR-0013**: These scripts are optional showcase helpers. Required builds, tests, and releases run through provider-neutral local or hosted CI paths.
