# GitHub Automation Instructions

Applies to `.github/**`.

Workflow edits are governance and supply-chain changes.

- Keep third-party Actions pinned to reviewed full commit SHAs.
- Preserve least-privilege permissions and fork/untrusted-PR secret isolation.
- Do not make required jobs green with broad skips, `continue-on-error`, exclusions or threshold reductions.
- Keep release tag provenance, install-E2E gating, image publication, SBOM/signing and post-publication verification fail closed.
- Do not expose release, registry, cloud or signing credentials to pull-request code.
- Preserve separate CI evidence for Go, web, browser accessibility, Helm, GoReleaser and Terraform when their paths are affected.
- Security workflow failures at the configured severity are real failures, not advisory decoration.

Run workflow/security linters and the scripts invoked by any changed workflow. YAML parsing alone is not sufficient validation.
