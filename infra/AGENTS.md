# Infrastructure Instructions

Applies to `infra/**`.

Infrastructure here is an optional showcase path, not a required production topology.

- Treat Terraform apply/destroy as potentially billable/destructive.
- Never run live provisioning without explicit intent and credentials.
- Keep provider-specific infrastructure isolated from provider-neutral DRAForge core behavior.
- Do not commit credentials, kubeconfigs, state files or production-specific secrets.
- Preserve variable validation, formatting and plan-policy checks.
- Showcase infrastructure must remain disposable and document teardown.
- Do not present a successful DOKS showcase as universal managed-Kubernetes validation.

For changes, run Terraform init with `-backend=false`, fmt/validate and repository plan-policy fixtures. Do not apply live infrastructure unless explicitly requested.
