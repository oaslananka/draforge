# Policy and Operations Script Instructions

These instructions apply to `scripts/**` and supplement the root instructions.

This directory contains policy-as-code, release validation, e2e orchestration, cloud/showcase helpers, install checks, security verification, and artifact validation.

## Fail closed

- A missing, malformed, stale, or unverifiable policy/evidence input must not become success.
- Do not weaken thresholds, allowlists, required checks, security controls, install policy, or release validation merely to make CI green.
- When a script enforces a documented policy, update the script, focused tests, and owning documentation together.

## Remote and privileged operations

- Separate local/kind credential-free paths from protected external-cluster or cloud paths.
- Do not run billable/provisioning/destructive scripts without explicit user intent.
- Kubeconfigs and cloud credentials must never be logged or persisted outside their reviewed temporary/protected path.
- Cleanup must be scoped to resources created/owned by the current operation.

## Filesystem and artifact safety

- Preserve canonical path, traversal, symlink, bounded-size, and digest/identity checks in plan/artifact validators.
- Keep generated output deterministic.
- Validate downloaded/published artifacts against the intended version/source identity before treating them as evidence.

## Verification

Every substantive checker/orchestrator change needs a focused regression test where the repository provides one. Run shellcheck or the language-specific test plus the higher-level CI policy gate that consumes the script.
