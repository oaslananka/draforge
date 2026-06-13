# Makefile for DRAForge

.PHONY: all build test tui server clean infra-init infra-plan infra-apply infra-output infra-audit infra-destroy-plan

all: build

build:
	go build -o bin/draforge ./cmd/draforge
	go build -o bin/draforge-server ./cmd/draforge-server
	go build -o bin/draforge-controller ./cmd/draforge-controller
	go build -o bin/draforge-sim-driver ./cmd/draforge-sim-driver

test:
	go test -v -race ./...

clean:
	rm -rf bin/
	rm -rf dist/

# Terraform Targets
infra-init:
	cd infra/terraform/environments/showcase && terraform init

infra-plan:
	cd infra/terraform/environments/showcase && terraform plan -out=tfplan
	cd infra/terraform/environments/showcase && terraform show -json tfplan > tfplan.json
	python scripts/validate-plan.py infra/terraform/environments/showcase/tfplan.json


infra-apply:
	./scripts/audit-cloud-resources.sh
	cd infra/terraform/environments/showcase && terraform apply tfplan
	./scripts/audit-cloud-resources.sh

infra-output:
	cd infra/terraform/environments/showcase && terraform output

infra-audit:
	./scripts/audit-cloud-resources.sh

infra-destroy-plan:
	cd infra/terraform/environments/showcase && terraform plan -destroy -out=tfdestroyplan
