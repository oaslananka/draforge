#!/usr/bin/env bash
# Verifies restricted controller metrics scraping and ServiceMonitor rendering.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
HELM_BIN=${HELM_BIN:-helm}

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

render() {
    local output
    output=$1
    shift
    "$HELM_BIN" template draforge "$CHART_DIR" \
        --namespace draforge-system "$@" > "$output"
}

extract_document() {
    local manifest kind name
    manifest=$1
    kind=$2
    name=$3
    awk -v wanted_kind="$kind" -v wanted_name="$name" '
        /^---$/ {
            if (matched) {
                printf "%s", document
                matched=0
                exit
            }
            document=""
            matched=0
            has_kind=0
            has_name=0
        }
        {
            document = document $0 ORS
        }
        $1 == "kind:" && $2 == wanted_kind {
            has_kind=1
        }
        $1 == "name:" && $2 == wanted_name {
            has_name=1
        }
        has_kind && has_name {
            matched=1
        }
        END {
            if (matched) {
                printf "%s", document
            }
        }
    ' "$manifest"
}

work_dir=$(mktemp -d)
cleanup() {
    rm -rf "$work_dir"
}
trap cleanup EXIT

# Default: metrics Service exists, controller ingress remains closed, and no
# Prometheus Operator resource is rendered.
default_manifest="$work_dir/default.yaml"
render "$default_manifest"
default_policy="$work_dir/default-policy.yaml"
extract_document "$default_manifest" NetworkPolicy draforge-controller-policy > "$default_policy"
[[ -s "$default_policy" ]] || fail "default controller NetworkPolicy was not rendered"
grep -Fq 'ingress: []' "$default_policy" || fail "default controller ingress must remain closed"
default_service="$work_dir/default-service.yaml"
extract_document "$default_manifest" Service draforge-controller > "$default_service"
[[ -s "$default_service" ]] || fail "default controller metrics Service was not rendered"
if grep -Fq 'kind: ServiceMonitor' "$default_manifest"; then
    fail "ServiceMonitor must be disabled by default"
fi

# Restricted profile: the selected namespace and pod labels may reach only the
# controller runtime port.
restricted_manifest="$work_dir/restricted.yaml"
render "$restricted_manifest" \
    --set controller.metrics.networkPolicy.enabled=true \
    --set-string 'controller.metrics.networkPolicy.namespaceSelector.matchLabels.kubernetes\.io/metadata\.name=monitoring' \
    --set-string 'controller.metrics.networkPolicy.podSelector.matchLabels.app\.kubernetes\.io/name=prometheus'
restricted_policy="$work_dir/restricted-policy.yaml"
extract_document "$restricted_manifest" NetworkPolicy draforge-controller-policy > "$restricted_policy"
for expected in \
    'kubernetes.io/metadata.name: monitoring' \
    'app.kubernetes.io/name: prometheus' \
    'port: 8082' \
    'protocol: TCP'; do
    grep -Fq -- "$expected" "$restricted_policy" || fail "restricted controller policy is missing: $expected"
done
if grep -Fq 'ingress: []' "$restricted_policy"; then
    fail "restricted controller policy unexpectedly denies all ingress"
fi

# Disabling the metrics Service must also remove the related ingress allowance.
disabled_manifest="$work_dir/disabled.yaml"
render "$disabled_manifest" \
    --set controller.metrics.service.enabled=false \
    --set controller.metrics.networkPolicy.enabled=true \
    --set-string 'controller.metrics.networkPolicy.namespaceSelector.matchLabels.kubernetes\.io/metadata\.name=monitoring'
disabled_policy="$work_dir/disabled-policy.yaml"
extract_document "$disabled_manifest" NetworkPolicy draforge-controller-policy > "$disabled_policy"
grep -Fq 'ingress: []' "$disabled_policy" || fail "disabled metrics Service must keep controller ingress closed"
disabled_service="$work_dir/disabled-service.yaml"
extract_document "$disabled_manifest" Service draforge-controller > "$disabled_service"
if [[ -s "$disabled_service" ]]; then
    fail "controller metrics Service rendered while disabled"
fi

# ServiceMonitor remains opt-in and must select the controller Service runtime
# port in the release namespace.
monitor_manifest="$work_dir/monitor.yaml"
render "$monitor_manifest" \
    --set controller.metrics.serviceMonitor.enabled=true \
    --set-string controller.metrics.serviceMonitor.labels.release=prometheus \
    --set-string controller.metrics.serviceMonitor.interval=30s \
    --set-string controller.metrics.serviceMonitor.scrapeTimeout=10s
monitor_doc="$work_dir/monitor-doc.yaml"
extract_document "$monitor_manifest" ServiceMonitor draforge-controller > "$monitor_doc"
[[ -s "$monitor_doc" ]] || fail "enabled ServiceMonitor was not rendered"
for expected in \
    'release: prometheus' \
    'port: runtime' \
    'path: /metrics' \
    'interval: 30s' \
    'scrapeTimeout: 10s' \
    '- "draforge-system"'; do
    grep -Fq -- "$expected" "$monitor_doc" || fail "ServiceMonitor is missing: $expected"
done

# Schema must reject unsafe or invalid profiles.
if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.metrics.networkPolicy.enabled=true >/dev/null 2>&1; then
    fail "networkPolicy.enabled=true without a namespace selector unexpectedly passed"
fi
if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.metrics.port=0 >/dev/null 2>&1; then
    fail "controller metrics port 0 unexpectedly passed schema validation"
fi
if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.metrics.serviceMonitor.enabled=true \
    --set-string controller.metrics.serviceMonitor.interval=invalid >/dev/null 2>&1; then
    fail "invalid ServiceMonitor interval unexpectedly passed schema validation"
fi
if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.metrics.serviceMonitor.enabled=true \
    --set-string controller.metrics.serviceMonitor.namespace=Invalid_Namespace >/dev/null 2>&1; then
    fail "invalid ServiceMonitor namespace unexpectedly passed schema validation"
fi
if "$HELM_BIN" template draforge "$CHART_DIR" \
    --set controller.metrics.service.enabled=false \
    --set controller.metrics.serviceMonitor.enabled=true >/dev/null 2>&1; then
    fail "ServiceMonitor without the metrics Service unexpectedly passed schema validation"
fi

echo "Controller metrics policy contract verified."
