# Governance of DRAForge

<!-- SPDX-License-Identifier: Apache-2.0 -->

DRAForge is an open-source project founded and maintained by Osman Aslan.

## Decision Making

Decisions are made through consensus among maintainers. Major changes — including
API redesigns, new subsystem introductions, or breaking CLI changes — are proposed
via GitHub Issues and discussed before implementation.

## Roles

- **Maintainer**: Active contributors who have demonstrated commitment and expertise.
  Maintainers have write access to the repository and participate in PR approvals.
  The current list is in [MAINTAINERS.md](MAINTAINERS.md).
- **Contributor**: Anyone who submits code, documentation, issues, or reviews.
  Contributors follow the guidelines in [CONTRIBUTING.md](CONTRIBUTING.md).

## Contribution Process

1. Open an issue or discussion to propose the change.
2. Reach consensus on the approach with maintainers.
3. Submit a PR following the [contributing guidelines](CONTRIBUTING.md).
4. PR is reviewed and approved by at least one maintainer.
5. Maintainer merges after all checks pass.

## Code of Conduct

All participants are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Maintainer and Access Continuity

DRAForge currently has one human maintainer. Automation identities do not count
as human maintainers, independent reviewers, or continuity successors.

The project recognizes two valid ways to satisfy access continuity after the
loss or incapacity of the maintainer:

1. A second qualified human has verified least-privilege access to repository
   administration, releases and packages, private security advisories, and the
   Doppler-managed recovery path; or
2. A designated executor holds a private encrypted succession package outside
   the public repository. That package must identify the protected recovery
   material, include the non-secret release and security-response runbooks, and
   provide the legal authority needed to transfer project stewardship.

Secret values, tokens, recovery codes, and private infrastructure details remain
in Doppler or the relevant platform recovery mechanism and are never copied into
public repository files or issues.

Neither continuity path is considered verified until a non-destructive exercise
shows that issues can be managed, a change can be accepted, and a release can be
recovered within the required timeframe. DRAForge therefore keeps OpenSSF
`access_continuity` and `bus_factor` marked as unmet until the evidence tracked in
[issue #144](https://github.com/oaslananka/draforge/issues/144) is complete.
