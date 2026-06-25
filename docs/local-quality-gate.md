# Local Quality Gate

Use this checklist before a release candidate.

Tool versions: Go 1.26.4, Node 22, pnpm 11.5.2, Helm 3.17.3, Terraform 1.7.0, golangci-lint v2.12.2, GoReleaser v2.16.0, and govulncheck.

Run task quality:local from the repository root. The gate covers module hygiene, lint, vet, vulnerability scan, unit tests, race tests, web lint and build, Helm lint and template, Terraform validation, Terraform plan policy fixtures, and GoReleaser snapshot dry run.
