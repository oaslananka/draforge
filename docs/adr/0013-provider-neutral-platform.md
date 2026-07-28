# ADR-0013: Provider-neutral platform and optional cloud showcases

- **Status**: Approved
- **Date**: 2026-07-28
- **Context**: DRAForge uses Kubernetes DRA APIs, standard RBAC, Helm, OCI images, and Kubernetes-native diagnostics. Earlier repository decisions selected DigitalOcean Kubernetes for a constrained demonstration environment and then described that environment as though it were the product architecture, build platform, and release path. That wording conflicts with the intended platform scope and with the current credential-free kind and GitHub Actions release gates.
- **Decision**:
  1. DRAForge is provider-neutral. Compatibility is determined by Kubernetes and DRA API behavior, not by a cloud vendor.
  2. The core server, controller, simulator, node plugin, CLI, dashboard, Helm chart, tagged smoke package, and release artifacts must not require provider-specific APIs or credentials.
  3. Required pull-request and release validation uses provider-neutral test infrastructure. The current source of truth is the kind install E2E matrix and GitHub Actions release workflow.
  4. Provider-specific Terraform, registry, kubeconfig acquisition, cost controls, and remote build helpers are optional showcase adapters. They must be clearly labeled, isolated, and removable without changing the core product.
  5. Remote cluster E2E should accept a standard, short-lived Kubernetes connection. Provider adapters may obtain that connection, but test execution, RBAC, artifacts, and cleanup remain provider-neutral.
  6. Release signing and provenance use GoReleaser, GitHub Actions OIDC, Cosign, checksums, and SBOMs without requiring a DOKS release job.
- **Alternatives**: Make one managed Kubernetes provider the supported production platform; maintain separate provider-specific product distributions; or keep provider assumptions implicit in runbooks and CI.
- **Consequences**: Users can deploy DRAForge to compatible managed, self-managed, on-premises, or local Kubernetes clusters. Showcase automation can still optimize for a particular provider, but it cannot define the general support contract or block unrelated releases. Provider adapters need explicit ownership and may have independent credentials, cost controls, and validation schedules.
- **Security Considerations**: Core CI and releases avoid long-lived cloud credentials. Optional adapters use short-lived, least-privilege access and must never expose kubeconfigs or tokens in logs, artifacts, workflow inputs, or repository files.
- **Operational Considerations**: The DOKS/DOCR Terraform path remains available as an optional demo. Generic install documentation and Helm defaults remain the primary user path. Provider-specific smoke validation is supplementary unless a provider is explicitly added to the support contract.
- **Supersedes**: ADR-0003 for the general platform architecture; ADR-0004 in full; ADR-0010 for required build and test execution; and the DOKS execution portion of ADR-0012. Those ADRs remain as historical records for the optional DigitalOcean showcase.
