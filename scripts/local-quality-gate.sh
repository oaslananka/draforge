#!/usr/bin/env bash
set -euo pipefail

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required tool: $1" >&2; exit 1; }
}

for tool in go helm terraform pnpm golangci-lint govulncheck goreleaser; do
  need "$tool"
done

go mod tidy
git diff --exit-code go.mod go.sum
golangci-lint run ./...
go vet ./...
govulncheck ./...
go test ./...
go test -race -coverprofile=coverage.out ./...
pnpm --dir web audit --audit-level high
pnpm --dir web lint
pnpm --dir web build
helm lint deploy/helm/draforge
helm template draforge deploy/helm/draforge >/tmp/draforge-helm-template.yaml
terraform -chdir=infra/terraform/environments/showcase init -backend=false
terraform -chdir=infra/terraform/environments/showcase fmt -check
terraform -chdir=infra/terraform/environments/showcase validate
scripts/test-validate-plan-fixtures.sh
goreleaser release --snapshot --clean --skip=docker,sbom,sign

echo ok
