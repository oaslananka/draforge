#!/usr/bin/env bash
# Captures install-level E2E diagnostics without failing the caller.
# SPDX-License-Identifier: Apache-2.0

set -u

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SYSTEM_NAMESPACE=${DRAFORGE_INSTALL_E2E_SYSTEM_NAMESPACE:-draforge-system}
FIXTURE_NAMESPACE=${DRAFORGE_INSTALL_E2E_FIXTURE_NAMESPACE:-draforge-e2e}
RELEASE_NAME=${DRAFORGE_INSTALL_E2E_RELEASE:-draforge}
ARTIFACT_DIR=${DRAFORGE_INSTALL_E2E_ARTIFACT_DIR:-$ROOT_DIR/artifacts/install-e2e}

mkdir -p "$ARTIFACT_DIR"

capture() {
  local file=$1
  shift
  "$@" > "$ARTIFACT_DIR/$file" 2>&1 || true
}

capture helm-manifest.yaml helm get manifest "$RELEASE_NAME" -n "$SYSTEM_NAMESPACE"
capture helm-values.yaml helm get values "$RELEASE_NAME" -n "$SYSTEM_NAMESPACE" --all
capture system-resources.yaml kubectl get all,networkpolicy,serviceaccount -n "$SYSTEM_NAMESPACE" -o yaml
capture fixture-resources.yaml kubectl get all,simulateddevicepools.draforge.oaslananka,resourceclaims.resource.k8s.io -n "$FIXTURE_NAMESPACE" -o yaml
capture resource-slices.yaml kubectl get resourceslices.resource.k8s.io -o yaml
capture device-classes.yaml kubectl get deviceclasses.resource.k8s.io -o yaml
capture calico-resources.yaml kubectl get daemonset/calico-node deployment/calico-kube-controllers -n kube-system -o yaml
capture calico-node.log kubectl logs -n kube-system daemonset/calico-node --all-containers=true
capture calico-kube-controllers.log kubectl logs -n kube-system deployment/calico-kube-controllers --all-containers=true
{
  for component in server controller node-plugin; do
    kubectl get clusterrole "$RELEASE_NAME-$component-role" -o yaml || true
    echo '---'
    kubectl get clusterrolebinding "$RELEASE_NAME-$component-binding" -o yaml || true
    echo '---'
  done
} > "$ARTIFACT_DIR/rbac.yaml" 2>&1
capture events.txt kubectl get events --all-namespaces --sort-by=.metadata.creationTimestamp
capture pod-descriptions.txt kubectl describe pods --all-namespaces

for component in server controller; do
  capture "${component}.log" kubectl logs -n "$SYSTEM_NAMESPACE" deployment/"$RELEASE_NAME"-"$component" --all-containers=true
  capture "${component}-previous.log" kubectl logs -n "$SYSTEM_NAMESPACE" deployment/"$RELEASE_NAME"-"$component" --all-containers=true --previous
done
capture node-plugin.log kubectl logs -n "$SYSTEM_NAMESPACE" daemonset/"$RELEASE_NAME"-node-plugin --all-containers=true
capture node-plugin-previous.log kubectl logs -n "$SYSTEM_NAMESPACE" daemonset/"$RELEASE_NAME"-node-plugin --all-containers=true --previous

printf 'Install-level E2E diagnostics collected in %s\n' "$ARTIFACT_DIR"
