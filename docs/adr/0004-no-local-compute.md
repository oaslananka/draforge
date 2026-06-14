# ADR-0004: No Local Compute

- **Status**: Approved
- **Context**: Building, testing, and running Kubernetes workloads locally is prohibited to ensure clean, reproducible cloud-based testing.
- **Decision**: Run all image builds, compilation, and integration/E2E tests remotely on DOKS via Kubernetes Jobs.
- **Alternatives**: Running local docker builds or minikube (prohibited).
- **Consequences**: Requires remote execution tooling (emote-build.sh, emote-test.sh).
- **Security Considerations**: CI secrets must be handled carefully.
- **Operational Considerations**: Incremental builds might be slower; mitigated by caching.
