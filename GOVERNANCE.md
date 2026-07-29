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

DRAForge intentionally operates with one human maintainer. Appointing a second
maintainer is not part of the project's continuity model, and automation
identities do not count as human maintainers, independent reviewers, or
continuity successors.

The selected access-continuity path is a solo-maintainer succession mechanism. A
designated executor, identified outside the public repository, must be able to
use a private encrypted succession package if the maintainer dies or becomes
incapacitated. That package must identify where protected recovery material is
held, include the non-secret GitHub administration, release/package,
security-response, documentation/domain, and Doppler recovery runbooks, and
provide the legal authority needed to transfer project stewardship.

Secret values, tokens, recovery codes, and private infrastructure details remain
in Doppler or the relevant platform recovery mechanism and are never copied into
public repository files or issues.

This continuity mechanism is considered verified only after a non-destructive
exercise demonstrates that issues can be managed, a change can be accepted, and
the documented release and security-response paths can be recovered within one
week. Until that evidence is complete, OpenSSF `access_continuity` remains
unmet. The separate Silver `bus_factor` SHOULD criterion is recorded as justified
unmet for this intentionally single-maintainer project and does not block Silver.
