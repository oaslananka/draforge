## AGENTS.md Created and Verified

Created `/home/runner/work/runtime-core/runtime-core/target/AGENTS.md` at the repo root with all build, lint, type-check, and test commands sourced from `Taskfile.yml`, `go.mod`, `web/package.json`, `.golangci.yml`, `Makefile`, `.github/workflows/ci.yml`, and `CONTRIBUTING.md`.

### Verified Fast Checks (All Pass)

| Command | Result |
|---------|--------|
| `task build` | ✅ Built all three binaries |
| `go vet ./...` | ✅ No issues |
| `task test:unit` | ✅ 17 packages OK |
| `go test -race ./pkg/...` | ✅ 1 package OK with race detector |
| `pnpm --dir web install --frozen-lockfile` | ✅ 228 packages installed |
| `pnpm --dir web test` | ✅ 10 test files, 34 tests passed |
| `pnpm --dir web lint` | ✅ Zero warnings/errors |

### AGENTS.md Contents Summary

- **Prerequisites**: Go 1.26.5, Task, pnpm 11.5.2, Node 22, Docker, kind, golangci-lint v2.12.2, GoReleaser, Syft, Helm 3.17.3, shellcheck
- **Quick Start**: Fast local check commands
- **Go Commands**: All 10 Task tasks with direct `go` equivalents
- **Web Commands**: All 7 Task tasks with direct `pnpm` equivalents
- **Security & Contract Verification**: 5 CI gate scripts
- **Helm Commands**: 6 verification tasks
- **E2E Commands**: 3 tasks (contract, kind, kind-full)
- **Infrastructure Commands**: 7 Terraform tasks
- **Release & SBOM**: 3 maintainer tasks
- **Demo**: 2 billable cloud tasks (explicitly marked)
- **Makefile Aliases**: 12 common targets
- **What Must Not Be Committed**: 8 categories from CONTRIBUTING.md
- **CI Pipeline Summary**: All 6 jobs with key steps

All documented commands match the corresponding `Taskfile.yml` task or CI step exactly.