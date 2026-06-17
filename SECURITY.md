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

### Cloud Demo Risks
- The Terraform showcase deployment provisions billable DigitalOcean resources.
- `task demo:up` and `task demo:down` manage cloud infrastructure — review
  the Terraform plan before applying.
- Demo deployments expose the dashboard publicly — do not use in production
  or with sensitive data.

### RBAC Minimum Permissions
- DRAForge follows the principle of least privilege.
- Helm chart RBAC is scoped to the minimum resources required:
  - `SimulatedDevicePool` CRD read/write
  - `ResourceClaim`, `ResourceSlice`, `DeviceClass` read
  - Pod and event read access
- If extending RBAC, ensure permissions are no broader than necessary.

## Responsible Disclosure

We ask that you:
1. Report vulnerabilities privately via email.
2. Allow reasonable time for a fix before public disclosure.
3. Do not exploit the vulnerability beyond what is necessary to demonstrate it.

We commit to:
1. Acknowledge receipt within 48 hours.
2. Provide an estimated fix timeline.
3. Credit researchers in public advisories (if desired).
