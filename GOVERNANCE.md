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

The current human maintainers and their public contact details are listed in
[MAINTAINERS.md](MAINTAINERS.md). Repository administration, package publication,
security response, and Doppler access are granted to named humans only through
the relevant platform; automation identities do not count as substitute
maintainers or reviewers.

A maintainer who becomes unavailable should be replaced through a public
nomination and consensus decision recorded in an issue or pull request. Before a
new maintainer is granted access, the existing maintainer verifies the minimum
required GitHub, release, security-advisory, and secret-management roles and
records the non-secret access inventory in the private operations system.

DRAForge currently has one human maintainer. This is an acknowledged continuity
and bus-factor risk: the project will not claim multi-person access continuity or
independent review until a second qualified human maintainer has accepted the
role and the required platform access has been verified.
