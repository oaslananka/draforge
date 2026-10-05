# Deployment Boundary Instructions

These instructions apply to `deploy/**` and supplement the root instructions.

This tree owns Kubernetes CRDs, Helm deployment assets, and related configuration. Provider-specific showcase infrastructure is optional and must remain separate from the provider-neutral core contract.

## Helm and Kubernetes manifests

- Default installation must not expose the read-only dashboard publicly.
- Production public access requires TLS plus operator-managed authentication/identity-aware proxying.
- Preserve least-privilege RBAC, security contexts, resource bounds, probes, service-account-token settings, and fail-closed node-plugin modes.
- Keep CRD/schema changes backward-compatible when the project promises compatibility; update controller/tests/docs together.
- Do not add broad cluster-admin permissions to solve a local RBAC failure.

## Showcase profiles

The DigitalOcean/DOKS showcase is billable and non-production.

- Unauthenticated HTTP exposure remains clearly labeled as showcase-only.
- Do not make showcase values the default production path.
- Terraform changes require reviewed plans and safe destroy/cleanup behavior.
- Do not run billable create/destroy operations without explicit user intent and protected credentials.

## Generated state

Do not hand-edit generated CRDs/chart packages when repository generators own them. Run Helm lint/template, workload-security, dashboard-exposure, image, and install-policy checks relevant to the change.
