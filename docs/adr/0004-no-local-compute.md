# ADR-0004: No local compute

- **Status**: Superseded by ADR-0013
- **Context**: The original demonstration workflow prohibited local build and test execution in favor of a clean remote environment.
- **Decision**: Run image builds, compilation, and integration/E2E tests remotely on DOKS through Kubernetes Jobs.
- **Alternatives**: Local Docker builds, kind, or minikube.
- **Consequences**: The approach required provider-specific remote execution helpers and increased feedback time.
- **Security Considerations**: Remote CI credentials required careful handling.
- **Operational Considerations**: Caching was used to mitigate slower incremental builds.
- **Supersession**: Local development, kind install E2E, and GitHub Actions builds are supported and form part of the current provider-neutral workflow. The remote helpers remain optional showcase tools.
