# Makefile for DRAForge (delegates to Taskfile)

.PHONY: all build test tui server clean infra-init infra-plan infra-apply infra-output infra-audit

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

clean:
	rm -rf bin/
	rm -rf dist/

infra-init:
	task infra:init

infra-plan:
	task infra:plan
	task infra:validate

infra-apply:
	task infra:apply

infra-audit:
	task infra:audit
