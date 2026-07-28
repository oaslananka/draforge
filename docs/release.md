# Release Process

This document describes the release workflow for DRAForge.

---

## Version Injection

All three binaries embed version and commit information at build time via GoReleaser `ldflags`:

| Binary | LDFLAGS |
|--------|---------|
| `draforge` | `-X main.versionVal={{.Version}} -X main.commitSHA={{.ShortCommit}}` |
| `draforge-controller` | `-X main.versionVal={{.Version}} -X main.commitSHA={{.ShortCommit}}` |
| `draforge-sim-driver` | `-X main.versionVal={{.Version}} -X main.commitSHA={{.ShortCommit}}` |

These values are set by GoReleaser during `goreleaser release` and are visible via `go version -m` on the compiled binary. The CLI also exposes them via `draforge version`.

Snapshot builds use a synthetic `*-SNAPSHOT-<commit>` version and do not publish anything. The pull-request `GoReleaser Dry-Run` job builds both target architectures with pinned QEMU and Buildx, validates Docker v2 artifact metadata, and still does not push images. `task release:local` is the lighter Docker-free workstation check.

## Prerequisites

- [GoReleaser](https://goreleaser.com/install/) v2.x installed and on `$PATH`.
- [Docker](https://docs.docker.com/engine/install/) installed and the daemon running
  (required for multi-arch image builds).
- [Syft](https://github.com/anchore/syft) installed (used for CycloneDX SBOM generation).
- Write access to create a new annotated tag in the GitHub repository.
- The repository-provided GitHub Actions token and OIDC identity; maintainers do not provide a long-lived publish token or Cosign key.
- Optional local Git signing configuration when creating a signed annotated tag.

Production publication runs only in GitHub Actions. Do not export or store a personal GitHub token, registry credential, or Cosign private key for the normal release path.

---

## Snapshot release (local dry-run)

Use the repository task for a Docker-free workstation check:

```bash
task release:local
# Equivalent to: goreleaser release --snapshot --clean --skip=docker,sbom,sign
```

This produces binaries, archives, and checksums in `dist/` without publishing, signing, generating SBOMs, or building containers. Verify the resulting payload with:

```bash
task release:verify
```

The pull-request `GoReleaser Dry-Run` job is the authoritative container check. It additionally builds the server, controller, and simulator-driver images for `linux/amd64` and `linux/arm64` and validates all Docker v2 snapshot artifacts.

---

## Required install E2E gate

The tagged release workflow calls `.github/workflows/e2e-matrix.yml` with the `full` profile before GoReleaser starts. The full profile installs and verifies the complete chart on the Kubernetes versions pinned in `tests/install-e2e/kubernetes-versions.json`. The `goreleaser` job depends on this gate, so release candidates and final releases do not publish when any required cluster target fails.

Run the same matrix locally before creating a tag:

```bash
task e2e:install-kind-full
```

Failure artifacts from GitHub Actions include rendered manifests, effective values, Kubernetes resources, events, component logs, API payloads, metrics, and SSE output.

---

## Tagged release flow

### Tag provenance and immutability policy

A releasable tag must:

- match `vMAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH-rc.NUMBER`;
- be an annotated Git tag object with a non-empty message and tagger provenance;
- resolve to the reviewed commit checked out by the workflow;
- be reachable from `main`;
- be pushed once and never moved, reused, or deleted.

The active `release-tag-immutability` repository ruleset blocks updates and deletions for `refs/tags/v*`. The historical `v0.1.0` and `v0.2.0` lightweight tags predate this policy. They remain unchanged because rewriting published tags would break provenance; every new release must satisfy the annotated-tag gate.

Maintainers with a configured Git signing identity may use `git tag -s` instead of `git tag -a`. A Git tag signature is additional provenance, not a substitute for the workflow gates. The release workflow uses GitHub OIDC and Cosign to sign checksums and container images so published payloads remain independently verifiable.

### 1. Prepare the release commit

Update `CHANGELOG.md`, chart `version` and `appVersion`, web package metadata, compatibility statements, and supported-version documentation in a pull request. Merge only after required CI passes.

Choose the new version after the release commit is on `main`:

```bash
release_tag=v0.3.0
release_version=${release_tag#v}
```

### 2. Create and verify the annotated tag

Create the tag on the reviewed `main` commit and verify it locally before pushing:

```bash
git switch main
git pull --ff-only origin main
git tag -a "$release_tag" -m "DRAForge $release_tag"
RELEASE_TAG="$release_tag" \
  RELEASE_MAIN_REF=main \
  bash scripts/verify-release-tag.sh
```

A signed annotated tag is also accepted:

```bash
git tag -s "$release_tag" -m "DRAForge $release_tag"
```

Use only one of the two tag commands.

### 3. Push once and let GitHub Actions publish

```bash
git push origin "$release_tag"
```

The tag push starts `.github/workflows/release.yml`. The workflow first validates the tag object and `main` ancestry, then runs the full install E2E matrix. GoReleaser publishes only after those gates pass. Do not run a second manual production GoReleaser publish for the same version.

The publish job:

1. builds all three binaries for their configured operating systems and architectures;
2. creates archives and `checksums.txt`;
3. generates CycloneDX archive SBOMs;
4. builds multi-platform server, controller, and simulator-driver images;
5. publishes release, minor, and `latest` GHCR tags;
6. signs checksums and container images with Cosign using the workflow identity;
7. creates the GitHub release and attached assets;
8. validates Docker v2 artifact metadata and anonymously inspects the public multi-platform manifests.

### 4. Verify the published release

```bash
# Check the GitHub release and attached assets
gh release view "$release_tag"

# Verify chart metadata and public multi-platform image references
scripts/verify-chart-images.sh "$release_version"
docker logout ghcr.io
VERIFY_REMOTE_IMAGES=1 scripts/verify-chart-images.sh "$release_version"

# Verify the published server version
docker run --rm "ghcr.io/oaslananka/draforge-server:$release_version" version

# Download SBOM assets for inspection
gh release download "$release_tag" -p "*.sbom"
```

---

## SBOM generation

SBOMs are generated by GoReleaser via Syft during the release pipeline. Each archive
gets a CycloneDX JSON SBOM (`{{ .ArtifactName }}.sbom`).

To generate an SBOM manually outside the release pipeline:

```bash
# Full project SBOM
task sbom
# Equivalent to: syft dir:. -o cyclonedx-json > draforge.sbom.json

# Per-binary SBOM
syft dist/draforge_linux_amd64_v1/draforge -o cyclonedx-json > draforge.sbom.json
```

---

## Artifacts

| Artifact | Location | Description |
|---|---|---|
| Binaries | `dist/` | GoReleaser staging directory (not committed) |
| Archives | `dist/*.tar.gz`, `dist/*.zip` | Compressed release archives |
| Checksums | `dist/checksums.txt` | SHA-256 checksums of all archives |
| SBOMs | `dist/*.sbom` | CycloneDX JSON software bills of materials |
| Docker images | `ghcr.io/oaslananka/` | Multi-arch container images |
| GitHub release | GitHub Releases, keyed by the immutable annotated tag | Release with assets |

---

## Rollback and superseding releases

### Before pushing the tag

Delete the local, unpublished tag and update the release commit through the normal pull-request workflow:

```bash
git tag -d "$release_tag"
```

Do not reset or force-push shared `main`.

### After pushing the tag

A pushed `v*` tag is immutable even when the workflow fails before publication. Do not delete, move, or recreate it. Diagnose the failed run, fix the repository through a pull request, increment the version, and create a new annotated tag.

### After publication

1. Mark the affected GitHub release as superseded or document the limitation without deleting provenance.
2. Leave the original tag, release assets, and container digests intact.
3. Fix the issue through a pull request.
4. Increment the patch or release-candidate number and publish a new release.
5. Verify the replacement release and clearly link it from the superseded release notes.

---

## What not to commit

The following are generated by GoReleaser and must not be committed
(already covered by `.gitignore`):

- `dist/` — GoReleaser output directory
- `bin/` — local `task build` output
- `*.sbom.json` — SBOM files
- `coverage.out` — test coverage output

Additionally, never commit:

- Secrets or tokens in any file
- Local IDE/editor config
- Compiled binaries outside `dist/` or `bin/`

---

## Troubleshooting

| Problem | Likely cause | Fix |
|---|---|---|
| `goreleaser` not found | Not installed or not on `$PATH` | Install via `go install github.com/goreleaser/goreleaser/v2@latest` |
| Docker build fails | Docker daemon not running | Start Docker Desktop / dockerd |
| `syft` command not found | Syft not installed | Install from https://github.com/anchore/syft |
| `gh` auth failure | No GitHub token or expired token | Run `gh auth login` or set `GITHUB_TOKEN` |
| Cosign signing fails | GitHub OIDC or package permissions unavailable | Verify `id-token: write`, package permissions, and the failed workflow logs |
| Release created but no assets | Missing `GITHUB_TOKEN` | Ensure token has `repo` scope |
