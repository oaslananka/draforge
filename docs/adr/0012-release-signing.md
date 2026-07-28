# ADR-0012: Release and signing strategy

- **Status**: Superseded in part by ADR-0013; signing and provenance remain approved
- **Context**: Open-source binaries and container images require verifiable supply-chain provenance.
- **Decision**: Use GoReleaser, Cosign, checksums, and SBOMs for release artifacts.
- **Alternatives**: Manual releases or unsigned artifacts.
- **Consequences**: Users can verify release integrity and provenance; release automation must preserve reproducibility and public artifact checks.
- **Security Considerations**: Current release signing uses GitHub Actions OIDC and must not rely on committed or long-lived signing keys.
- **Operational Considerations**: The release workflow runs after the provider-neutral full install E2E gate and verifies public multi-architecture images.
- **Supersession**: The earlier requirement to execute releases inside a DOKS Job is removed by ADR-0013. The signing, checksum, and SBOM decisions remain in force.
