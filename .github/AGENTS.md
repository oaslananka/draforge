# GitHub Automation Instructions

These instructions apply to `.github/**` and supplement the root instructions.

Workflow edits change repository governance, supply-chain security, release identity, cloud authority, or required evidence.

## CI and required checks

- Preserve required contexts and aggregate gates unless branch protection is intentionally migrated.
- Do not use path filters, `continue-on-error`, skip logic, exclusions, or threshold changes to hide a repository-owned failure.
- Keep credential-free pull-request coverage broad; privileged cloud/manual workflows stay explicitly gated.
- Third-party Actions remain pinned to reviewed full commit SHAs.

## Permissions and secrets

- Default permissions remain minimal/read-only.
- Grant write or OIDC only to the smallest reviewed job.
- Never expose cloud, registry, signing, kubeconfig, or environment credentials to untrusted PR code.
- Protected environments and explicit confirmation gates for billable/privileged operations must remain intact.

## Release

- Release tags are immutable, annotated, and must point to reviewed main history.
- Preserve package/image/chart checksums, SBOMs, signatures/attestations, and published-release verification.
- Do not bypass the complete credential-free install matrix for tagged releases.
- Do not trigger a release or cloud e2e run unless explicitly requested.

## Verification

Run workflow lint/pin/permission/security scripts and any invoked policy-script tests. YAML validity alone is not enough.
