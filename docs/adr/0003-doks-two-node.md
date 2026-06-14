# ADR-0003: DOKS Two-Node Architecture

- **Status**: Approved
- **Context**: Need a real Kubernetes cluster for testing DRA without exceeding cloud budget and resource limits.
- **Decision**: Provision a single DigitalOcean Kubernetes (DOKS) cluster with exactly two worker nodes of size s-4vcpu-8gb.
- **Alternatives**: Single-node cluster (lacks multi-node scheduling validation) or larger clusters (cost prohibitive).
- **Consequences**: Limited compute capacity, requiring strict resource quotas and sequential build jobs.
- **Security Considerations**: Workloads isolated by namespaces and NetworkPolicies.
- **Operational Considerations**: Node count strictly limited to 2.
