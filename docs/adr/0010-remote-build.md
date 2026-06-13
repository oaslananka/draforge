# ADR-0010: Remote Build Architecture

- **Status**: Approved
- **Context**: Builds and tests must run on DigitalOcean Kubernetes.
- **Decision**: Implement a script-driven remote CI system that spawns sequential Kubernetes Jobs in a draforge-ci namespace to build binaries and container images (using rootless BuildKit).
- **Alternatives**: Heavy Jenkins or GitLab CI runners.
- **Consequences**: Minimal overhead, fully native to DOKS, sequential execution prevents cluster starvation.
- **Security Considerations**: Credentials are not logged; jobs run as non-root.
- **Operational Considerations**: Cleans up completed job pods automatically.
