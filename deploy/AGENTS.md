# Deployment Instructions

Applies to `deploy/**`.

Helm/Kubernetes assets encode runtime security and exposure contracts.

- Default services remain internal unless an explicit secure exposure profile is selected.
- Demo/showcase unauthenticated exposure must stay clearly non-production.
- Preserve NetworkPolicies, RBAC, readiness, graceful shutdown, resource limits, restricted metrics and image identity contracts.
- Host-integrated CDI/node mode stays explicit opt-in; do not silently mount host paths in default/demo-safe mode.
- Image references, digests and chart metadata must stay aligned with release verification.
- Do not weaken probes, securityContext, resource ceilings or policy mounts for convenience.

Run Helm lint/template plus the repository chart/workload/exposure/lifecycle verifiers for affected changes.
