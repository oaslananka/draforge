# ADR-0003: DOKS two-node showcase architecture

- **Status**: Superseded for the general platform by ADR-0013; retained for the optional DOKS showcase
- **Context**: A real Kubernetes cluster was needed for one budget-constrained demonstration of multi-node DRA behavior.
- **Decision**: Provision a single DigitalOcean Kubernetes cluster with exactly two `s-4vcpu-8gb` worker nodes for that showcase.
- **Alternatives**: A single-node showcase, which would not exercise multi-node scheduling, or a larger showcase, which would increase cost.
- **Consequences**: The optional showcase has limited compute capacity and requires quotas and sequential remote jobs.
- **Security Considerations**: Showcase workloads are isolated by namespaces and NetworkPolicies.
- **Operational Considerations**: The showcase node count is limited to two.
- **Scope after ADR-0013**: This decision applies only to the optional DigitalOcean demonstration environment. It does not define DRAForge architecture, supported providers, installation requirements, or release gates.
