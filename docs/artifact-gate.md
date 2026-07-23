# Artifact Gate Checklist

Before publishing a version, confirm the target commit has green CI.

Required checks:

- Go Lint & Test
- Web Lint & Build
- Helm Chart Lint & Template
- GoReleaser Dry-Run
- Terraform Validate
- CI Pass

The CI gate verifies these outputs:

- `dist/checksums.txt`
- `dist/draforge_*`
- `dist/draforge-controller_*`
- `dist/draforge-sim-driver_*`
- `charts/draforge-*.tgz`

Manual commands:

```bash
pnpm --dir web install --frozen-lockfile --ignore-scripts
pnpm --dir web build
helm lint deploy/helm/draforge
scripts/verify-github-action-pins.sh
scripts/verify-workload-security.sh
scripts/verify-chart-images.sh
scripts/test-chart-image-verifier.sh
scripts/verify-dashboard-exposure.sh
scripts/verify-sim-driver-cdi.sh
mkdir -p charts
helm package deploy/helm/draforge --destination charts
goreleaser release --snapshot --clean --skip=docker,sbom,sign
task release:verify
```

If a publishing job fails before assets are available, fix the issue and rerun from a clean commit. If assets are already available to users, publish a new patch version rather than replacing them silently.
