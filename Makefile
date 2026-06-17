# Makefile for DRAForge (delegates to Taskfile)
# SPDX-License-Identifier: Apache-2.0

.DEFAULT_GOAL := all

.PHONY: all build test fmt lint vet vuln clean
.PHONY: web-install web-lint web-build
.PHONY: helm-lint sbom release-local
.PHONY: infra-init infra-plan infra-apply infra-audit

all: build

build:
	task build

test:
	task test

fmt:
	task fmt

lint:
	task lint

vet:
	task vet

vuln:
	task vuln

clean:
	rm -rf bin/
	rm -rf dist/

# --- Web ---

web-install:
	task web:install

web-lint:
	task web:lint

web-build:
	task web:build

# --- Helm ---

helm-lint:
	task helm:lint

# --- Release ---

sbom:
	task sbom

release-local:
	task release:local

# --- Infrastructure ---

infra-init:
	task infra:init

infra-plan:
	task infra:plan
	task infra:validate

infra-apply:
	task infra:apply

infra-audit:
	task infra:audit
