#!/usr/bin/env bash
set -euo pipefail

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required tool: $1" >&2; exit 1; }
}

for tool in go helm terraform pnpm golangci-lint goreleaser python3; do
  need "$tool"
done

python3 scripts/verify-release-metadata.py --self-test --root .
python3 scripts/verify-documentation.py --self-test
python3 scripts/verify-goreleaser-docker-v2.py --self-test --check
python3 scripts/verify-goreleaser-docker-artifacts.py --self-test
bash scripts/test-verify-release-tag.sh
go mod tidy
git diff --exit-code go.mod go.sum
scripts/verify-kubernetes-module-alignment.sh
golangci-lint run ./...
go vet ./...
go tool govulncheck ./...
go test ./...
go test -race -coverprofile=coverage.out ./...
pnpm --dir web audit --audit-level high
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
helm lint deploy/helm/draforge
helm template draforge deploy/helm/draforge >/tmp/draforge-helm-template.yaml
scripts/verify-github-action-pins.sh
scripts/verify-workload-security.sh
scripts/verify-frontend-dependency-policy.sh
scripts/test-prepare-remote-e2e-kubeconfig.sh
scripts/test-remote-e2e-harness.sh
scripts/verify-install-e2e-policy.sh
scripts/test-install-e2e-cni.sh
scripts/test-install-e2e-harness.sh
scripts/test-controller-ha-e2e-harness.sh
scripts/verify-chart-images.sh
scripts/test-chart-image-verifier.sh
scripts/verify-controller-metrics-policy.sh
scripts/verify-runtime-lifecycle.sh
scripts/verify-dashboard-exposure.sh
scripts/verify-sim-driver-cdi.sh
python3 scripts/validate-terraform-variables.py --self-test infra/terraform/environments/showcase
terraform -chdir=infra/terraform/environments/showcase init -backend=false
terraform -chdir=infra/terraform/environments/showcase fmt -check
terraform -chdir=infra/terraform/environments/showcase validate
scripts/test-validate-plan-fixtures.sh
goreleaser release --snapshot --clean --skip=docker,sbom,sign

echo ok
