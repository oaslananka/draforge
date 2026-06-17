# Security Policy

<!-- SPDX-License-Identifier: Apache-2.0 -->

## Supported Versions

Only the latest release is actively supported with security updates.

| Version | Supported |
| ------- | --------- |
| v0.1.x  | Yes       |
| < v0.1  | No        |

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

The DRAForge web dashboard is intentionally **read-only**:

- The Go web server exposes only read API endpoints and SSE event streams.
- No write, create, update, or delete mutations are possible through the web UI.
- All cluster mutations (scenario apply, fault injection, resource changes) require
  CLI commands authenticated via the administrator's local `kubeconfig`.
- The dashboard binds to all interfaces by default and **must** be placed behind
  an ingress controller or identity-aware proxy in production deployments.

## Security Best Practices

### Secrets and Credentials
- **Never commit secrets, tokens, passwords, or API keys** to the repository.
- **Never include kubeconfig contents** in issue reports, logs, examples, or
  documentation.
- If you must share configuration for debugging, redact all sensitive values
  before posting.
- Use environment variables or secret management tools (e.g., GitHub Secrets,
  Vault, 1Password) for all credentials.
- The repository is configured with Dependabot for dependency updates and expects
  GitHub secret scanning to be enabled on the repository.

### Cloud Demo Risks
- The Terraform showcase deployment provisions billable DigitalOcean resources.
- `task demo:up` and `task demo:down` manage cloud infrastructure — review
  the Terraform plan before applying.
- Demo deployments expose the dashboard publicly — do not use in production
  or with sensitive data.
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

### GitHub Security Features (Recommended)

To fully secure this repository, enable the following GitHub settings:

| Feature | Purpose | How to Enable |
|---------|---------|---------------|
| Branch protection | Prevent force-pushes and deletion of `main` | Settings > Branches > Add rule |
| Required status checks | Block merges if CI fails | Branch protection rule > "Require status checks" |
| PR reviews | Require at least one review before merge | Branch protection rule > "Require pull request reviews" |
| Dependabot security updates | Auto-merge dependency fixes | Settings > Security & analysis > Dependabot security updates |
| Secret scanning | Detect accidental credential commits | Settings > Security & analysis > Secret scanning |
| Code scanning (CodeQL) | Automated vulnerability detection | Already configured in `.github/workflows/security.yml` |

### CI/CD Security

- Normal CI (`ci.yml`) does not require any cloud credentials and runs safely
  on forks and PRs.
- E2E tests (`e2e.yml`) require `DIGITALOCEAN_TOKEN` and are gated to the
  upstream repository. They only trigger on `workflow_dispatch` (manual) or
  nightly schedule.
- Release workflow (`release.yml`) requires `GITHUB_TOKEN` with `packages: write`
  and is gated to the upstream repository.

## Responsible Disclosure

We ask that you:
1. Report vulnerabilities privately via email.
2. Allow reasonable time for a fix before public disclosure.
3. Do not exploit the vulnerability beyond what is necessary to demonstrate it.

We commit to:
1. Acknowledge receipt within 48 hours.
2. Provide an estimated fix timeline.
3. Credit researchers in public advisories (if desired).
