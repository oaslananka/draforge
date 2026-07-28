# Maintainer Checklist

<!-- SPDX-License-Identifier: Apache-2.0 -->

## Before Merging a PR

- [ ] Tests pass: `go test ./...`
- [ ] Static analysis clean: `go vet ./...`
- [ ] Web dashboard builds: `pnpm --dir web lint && pnpm --dir web build`
- [ ] No generated artifacts included (`dist/`, `bin/`, `*.sbom.json`, `coverage.out`,
      `web/dist/`, `web/node_modules/`)
- [ ] Conventional Commits format used
- [ ] Documentation updated if behavior or configuration changed
- [ ] No unrelated formatting or refactoring mixed in
- [ ] If Dockerfiles changed, verify with `goreleaser release --snapshot --clean --skip=docker,sbom,sign`
- [ ] If Go code changed, check `golangci-lint run ./...`
- [ ] Security-sensitive changes reviewed for secret exposure and input validation
- [ ] Remote E2E changes pass the portable kind workflow; external kubeconfigs remain short-lived and environment-protected

## Before a Release

- [ ] `CHANGELOG.md` updated with all changes since last release
- [ ] Release tag is annotated, has a non-empty message, matches SemVer/RC syntax, and targets the reviewed `main` commit
- [ ] `RELEASE_TAG=<tag> RELEASE_MAIN_REF=main bash scripts/verify-release-tag.sh` passes before push
- [ ] New `v*` tag pushed once; an existing release tag is never moved, reused, or deleted
- [ ] Protected tag workflow completed the full install E2E gate and GoReleaser publish job
- [ ] Release artifacts verified locally: `task release:verify`
- [ ] GitHub release assets verified (archives, checksums, SBOMs)
- [ ] Chart `version` and `appVersion` match the release tag without the leading `v`
- [ ] After `docker logout ghcr.io`, `VERIFY_REMOTE_IMAGES=1 scripts/verify-chart-images.sh <version>` passes for all three public GHCR manifests
- [ ] Docker images pushed to `ghcr.io/oaslananka/` and multi-arch manifests created
- [ ] Release smoke-tested:
  ```bash
  docker run --rm ghcr.io/oaslananka/draforge-server:v0.x.x version
  docker run --rm ghcr.io/oaslananka/draforge-server:v0.x.x doctor --help
  ```
- [ ] `docs/release.md` readme reflects any process changes

## Before a Public Demo

- [ ] Terraform plan reviewed for cost impact
- [ ] `task demo:up` tested in a clean namespace
- [ ] Dashboard access verified (read-only, no mutation)
- [ ] Public URL does not expose admin or debug endpoints
- [ ] Default Helm render contains no Gateway, HTTPRoute, or Ingress
- [ ] `scripts/verify-sim-driver-cdi.sh` passes for non-root demo and fail-closed host-integrated node modes
- [ ] Secure public route terminates TLS and targets the identity proxy Service, not DRAForge directly
- [ ] Any repository-managed screenshots updated if UI changed
- [ ] If the optional DOKS showcase was used, `task demo:down` tears down its billable resources

## Security Review Checklist

- [ ] No secrets, tokens, or credentials committed (check git diff)
- [ ] No unsafe `os/exec` or shell injection vectors in changed code
- [ ] Input validation on all user-facing CLI flags and API parameters
- [ ] Rate limiting and timeouts configured on HTTP server endpoints
- [ ] CORS hardened (not `*` in production)
- [ ] Dashboard endpoints are read-only; no write paths exposed
- [ ] Any new dependencies reviewed for known vulnerabilities (`go tool govulncheck ./...`)

## Optional Showcase Cloud Cost Checklist

This section applies only when maintainers use the optional DigitalOcean showcase.

- [ ] Provisioned resources align with expected demo/review scope
- [ ] Unused resources destroyed after testing: `task demo:down`
- [ ] Terraform state files not committed (`infra/terraform/*/terraform.tfstate*`
      covered by `.gitignore`)
- [ ] `scripts/audit-cloud-resources.sh` run after any infrastructure change

## Generated Artifact Checklist

- [ ] `git status --short` checked before committing
- [ ] `.gitignore` entries confirmed active for:
  - `dist/`
  - `bin/`
  - `*.sbom.json`
  - `coverage.out`
  - `web/dist/`
  - `web/node_modules/`
- [ ] No `.env` or credential files staged
