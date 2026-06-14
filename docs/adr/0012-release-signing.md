# ADR-0012: Release and Signing Strategy

- **Status**: Approved
- **Context**: Releasing open-source binaries requires supply chain trust.
- **Decision**: Automate the release process using GoReleaser inside a DOKS release Job. Sign all binaries and container images using Cosign and generate SBOMs.
- **Alternatives**: Manual releases or unsigned binaries.
- **Consequences**: Fully secure, verifiable, reproducible release process.
- **Security Considerations**: Release signing keys are stored securely.
- **Operational Considerations**: Releases will start at version 0.1.0-rc.1.
