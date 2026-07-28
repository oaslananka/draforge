#!/usr/bin/env bash
set -euo pipefail

modules=(
  k8s.io/api
  k8s.io/apimachinery
  k8s.io/apiserver
  k8s.io/client-go
  k8s.io/component-base
  k8s.io/dynamic-resource-allocation
)

expected_version=""
for module in "${modules[@]}"; do
  version="$(go list -m -f '{{.Version}}' "${module}")"
  if [[ -z "${version}" ]]; then
    echo "${module} has no resolved module version" >&2
    exit 1
  fi
  if [[ -z "${expected_version}" ]]; then
    expected_version="${version}"
    continue
  fi
  if [[ "${version}" != "${expected_version}" ]]; then
    echo "Kubernetes module version mismatch: ${module}=${version}, expected ${expected_version}" >&2
    exit 1
  fi
done

echo "Kubernetes DRA module alignment verified at ${expected_version}."
