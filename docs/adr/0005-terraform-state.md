# ADR-0005: Terraform State Strategy

- **Status**: Approved
- **Context**: Provisioning resources using Terraform requires a state store.
- **Decision**: Store Terraform state in a gitignored local state directory (infra/terraform/state/) for initial bootstrapping.
- **Alternatives**: DigitalOcean Spaces S3-compatible backend (requires complex credential bootstrapping).
- **Consequences**: Developers must backup their state directory. Plaintext state is never committed.
- **Security Considerations**: State contains sensitive metadata and must be kept secure and gitignored.
- **Operational Considerations**: Idempotent state runs.
