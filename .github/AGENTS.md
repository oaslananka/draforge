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

For protected CI contract changes, reproduce the repository-owned checks that do not have a Task alias:

```bash
python3 scripts/verify-release-metadata.py --self-test --root .
scripts/verify-github-action-pins.sh
scripts/verify-ci-privilege-boundaries.sh
scripts/verify-workload-security.sh
scripts/verify-frontend-dependency-policy.sh
scripts/test-remote-build.sh

shellcheck \
  scripts/prepare-remote-e2e-kubeconfig.sh \
  scripts/remote-e2e.sh \
  scripts/run-remote-e2e-tests.sh \
  scripts/test-prepare-remote-e2e-kubeconfig.sh \
  scripts/test-remote-e2e-harness.sh \
  scripts/e2e-matrix.sh \
  scripts/install-e2e-cni.sh \
  scripts/build-install-e2e-images.sh \
  scripts/run-install-e2e.sh \
  scripts/collect-install-e2e-artifacts.sh \
  scripts/kind-install-e2e.sh \
  scripts/kind-install-e2e-matrix.sh \
  scripts/test-install-e2e-cni.sh \
  scripts/test-install-e2e-harness.sh \
  scripts/verify-install-e2e-policy.sh \
  scripts/verify-release-tag.sh \
  scripts/test-verify-release-tag.sh \
  scripts/verify-kubernetes-module-alignment.sh
```

The broad workstation equivalent is `bash scripts/local-quality-gate.sh`. GitHub Actions remains authoritative for protected checks and environment-dependent lanes.
