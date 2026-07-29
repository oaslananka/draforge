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

DRAForge intentionally operates with one human maintainer. A second maintainer is
not required by the project's continuity model, and automation identities are not
treated as human maintainers or independent reviewers.

Continuity is handled through a private, encrypted succession package kept
outside the public repository. The package records the non-secret recovery and
transfer procedures for GitHub administration, releases and packages, private
security advisories, project domains and documentation, and the Doppler project
inventory. Secret values remain in Doppler or the relevant platform's protected
recovery mechanism and are never copied into repository files or issue text.

The succession package also identifies where emergency recovery material is held,
provides the release and security-response runbooks needed to continue operations,
and includes the legal authorization required for a designated executor to
transfer project stewardship if the maintainer dies or becomes incapacitated.
The maintainer reviews this continuity material after material access or release
process changes and at least annually.

If the maintainer becomes unavailable, the designated executor uses that package
to restore administrative access, preserve security response, and transfer
stewardship to a successor. This continuity mechanism is separate from the
project's bus factor: DRAForge may remain a single-maintainer project while still
maintaining a documented succession path.
