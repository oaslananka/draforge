# Security Policy

<!-- SPDX-License-Identifier: Apache-2.0 -->

## Supported Versions

Only the latest release is actively supported with security updates.

| Version | Supported |
| ------- | --------- |
| v0.2.x  | Yes       |
| < v0.2  | No        |

## Reporting a Vulnerability

If you discover a security vulnerability, **do not open a public issue**.

Please email `oaslananka@gmail.com` with details. We aim to respond within 48 hours
and will coordinate a public advisory and fix timeline with you.

Include in your report:
- Affected version(s) and component(s)
- Description of the vulnerability
- Steps to reproduce (if applicable)
- Any proposed mitigation (optional)

## Public Dashboard Security Model

The DRAForge web dashboard is intentionally **read-only**, but read-only data can still be sensitive:

- The Go web server exposes read API endpoints and SSE streams containing cluster topology, node and namespace identity, pod/claim relationships, DRA device attributes and capacities, allocation status, diagnostics, and selected event evidence.
- No write, create, update, or delete mutations are possible through the web UI. Cluster mutations require CLI commands authenticated with the administrator's local `kubeconfig`.
- A default Helm install creates no Gateway or Ingress listener. Use local port-forwarding for operator access.
- Production external access must terminate TLS and route through an operator-managed OIDC or identity-aware proxy. The secure public example routes to that proxy Service instead of directly to DRAForge.
- CORS limits browser origins; it is not authentication or authorization.

## Security Best Practices

### Secrets and Credentials
- **Never commit secrets, tokens, passwords, or API keys** to the repository.
- **Never include kubeconfig contents** in issue reports, logs, examples, or
  documentation.
- If you must share configuration for debugging, redact all sensitive values
  before posting.
- Use Doppler as the source of truth for repository, cloud, and environment secrets. GitHub Environment or Actions secrets are runtime delivery targets, not the canonical store.
- Dependency advisories are enforced by `govulncheck`, pnpm audit, OSV/security workflows, Dependency Review, and pinned-action checks. Automated dependency-update tooling is not treated as a security boundary.
- GitHub secret scanning and push protection are enabled; never rely on scanners as permission to commit a credential.

### Host-Integrated CDI Mode

`nodePlugin.outputMode=node` writes to a kubelet hostPath and is disabled by default. The container uses UID 0 only to access the root-owned CDI directory; it remains non-privileged with no Linux capabilities, no privilege escalation, a read-only root filesystem, and RuntimeDefault seccomp. Use node mode only on controlled nodes. Demo mode remains non-root and host-isolated.

### Cloud Demo Risks
- The Terraform showcase deployment provisions billable DigitalOcean resources.
- `task demo:up` and `task demo:down` manage cloud infrastructure — review
  the Terraform plan before applying.
- Demo profiles explicitly expose the dashboard over unauthenticated HTTP — do not use them in production or with sensitive data.
- The E2E workflow (`.github/workflows/e2e.yml`) requires the `DIGITALOCEAN_TOKEN`
  secret and provisions cloud resources. It is gated to the upstream repository
  and requires explicit confirmation before running.

### RBAC Minimum Permissions
- DRAForge follows the principle of least privilege.
- Helm chart RBAC is scoped to the minimum resources required:
  - `SimulatedDevicePool` CRD read/write
  - `ResourceClaim`, `ResourceSlice`, `DeviceClass` read
  - Pod and event read access
- If extending RBAC, ensure permissions are no broader than necessary.

### GitHub Security Features

The following controls reflect the current public repository configuration:

| Feature | Repository state | Security purpose |
|---------|------------------|------------------|
| `main` ruleset | Active | Blocks force-push/deletion and requires the repository's merge checks |
| Release-tag ruleset | Active for `refs/tags/v*` | Blocks update and deletion of published release tags |
| Required status checks | Active through rulesets | Blocks merge when required CI or security checks fail |
| Secret scanning and push protection | Enabled | Detects and blocks supported secret patterns before publication |
| Code and dependency scanning | Active workflows and PR checks | Runs CodeQL/security analysis, Dependency Review, Semgrep, Socket, and language advisory gates |
| Automated security fixes | Not enabled | Optional automation; CI advisory gates and reviewed updates remain authoritative |

### Supply-Chain and Workload Controls

- Every external GitHub Action is pinned to a full 40-character commit SHA. `scripts/verify-github-action-pins.sh` is required by CI and the local quality gate.
- pnpm resolution enforces a seven-day minimum package release age and fails closed when registry publish-time metadata is missing.
- Any package-maturity exception must pin an exact package version and be justified by clean OSV Scanner and pnpm audit results; package-wide exceptions are not permitted.
- CI, release, and container frontend installs use `--ignore-scripts`; local development permits only the explicitly allowlisted `esbuild` build step.
- Remote build and unit-test Jobs disable service-account token automounting because they do not call the Kubernetes API. Remote E2E uses a dedicated, run-scoped read-only ClusterRoleBinding.
- Remote test and E2E containers run as UID/GID 1000 with read-only root filesystems and bounded writable cache/tmp volumes. The Kaniko build executor is the documented exception: image unpacking and root-owned Dockerfile steps require UID 0 and a writable rootfs, so it uses an immutable maintained-fork digest with no service-account token, no privilege escalation, RuntimeDefault seccomp, and the unnecessary `NET_RAW` capability removed.
- Helm and remote Jobs define ephemeral-storage requests, limits, and bounded `emptyDir` volumes to reduce node disk exhaustion risk.
- Terraform plan validation accepts only bounded JSON files that resolve inside the repository, preventing path traversal and symlink escapes.

### CI/CD Security

- Normal CI (`.github/workflows/ci.yml`) and the install matrix (`.github/workflows/e2e-matrix.yml`) require no cloud credentials. The reduced install matrix runs on pull requests; the full matrix runs weekly, manually, and as the required tagged-release gate.
- Portable smoke testing (`.github/workflows/e2e-kubernetes.yml`) runs credential-free kind for relevant pull requests. Its manual external mode uses a short-lived `KUBECONFIG_B64` delivered through the protected `e2e-kubernetes` Environment from Doppler and removes the kubeconfig after the run.
- The optional DOKS adapter (`.github/workflows/e2e.yml`) is manual-only, targets an existing billable cluster, and requires the exact confirmation phrase plus protected-environment approval. `DIGITALOCEAN_TOKEN` is sourced from Doppler and should have only `kubernetes:read` and `kubernetes:access_cluster`.
- The release workflow (`.github/workflows/release.yml`) accepts only immutable annotated release tags on `main`, requires the full credential-free install matrix, and uses the repository-provided `GITHUB_TOKEN` plus GitHub OIDC for package publication and Cosign signing.

## Responsible Disclosure

We ask that you:
1. Report vulnerabilities privately via email.
2. Allow reasonable time for a fix before public disclosure.
3. Do not exploit the vulnerability beyond what is necessary to demonstrate it.

We commit to:
1. Acknowledge receipt within 48 hours.
2. Provide an estimated fix timeline.
3. Credit researchers in public advisories (if desired).
