# ADR-0002: Go as Primary Language

- **Status**: Approved
- **Context**: Kubernetes is written in Go, and Go client-go is the primary client SDK.
- **Decision**: Use Go as the primary programming language for the CLI, controller, and server.
- **Alternatives**: Rust (good but smaller ecosystem for k8s clients), Python.
- **Consequences**: Fast build times, direct access to official Kubernetes client APIs.
- **Security Considerations**: Standard Go security audits (govulncheck, staticcheck).
- **Operational Considerations**: Standard Go toolchain.
