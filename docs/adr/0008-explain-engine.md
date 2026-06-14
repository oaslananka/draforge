# ADR-0008: Explain-Engine Evidence Model

- **Status**: Approved
- **Context**: Pending ResourceClaims are hard to debug without looking at internal scheduler details.
- **Decision**: Build a rule-based explain engine that inspects device class selectors, CEL expressions, ResourceSlices, capacity, taints, and events, producing a deterministic reason tree.
- **Alternatives**: Simple string parsing of event messages.
- **Consequences**: Provides high-confidence, actionable debugging steps.
- **Security Considerations**: Redacts secrets from explanation outputs.
- **Operational Considerations**: Evaluated on-demand via CLI or Dashboard.
