# Local Quality Gate

Use this checklist before a release candidate.

Tool versions: Go 1.26.5, Node 22, pnpm 11.5.2, Helm 3.17.3, Terraform 1.7.0, golangci-lint v2.12.2, and GoReleaser v2.16.0. The `govulncheck` tool version is locked in `go.mod`.

Run `bash scripts/local-quality-gate.sh` from the repository root. For the documentation-only slice, run `task docs:verify`.

The gate covers release and documentation metadata contracts, Markdown relative links, documented Task/script/path/CLI references, E2E mode descriptions, module hygiene, lint, vet, vulnerability scanning, unit and race tests, frontend advisory/test/lint/build checks, Helm contracts, Terraform validation and policy fixtures, and the GoReleaser snapshot dry run.
