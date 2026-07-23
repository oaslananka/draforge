#!/usr/bin/env bash
# Verifies disabled, local-demo, and secure public dashboard exposure profiles.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

CHART_DIR=${CHART_DIR:-deploy/helm/draforge}
HELM_BIN=${HELM_BIN:-helm}

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

assert_contains() {
    local file text
    file=$1
    text=$2
    grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_not_contains() {
    local file text
    file=$1
    text=$2
    if grep -Fq -- "$text" "$file"; then
        fail "$file unexpectedly contains: $text"
    fi
}

render() {
    local output
    output=$1
    shift
    "$HELM_BIN" template draforge "$CHART_DIR" --namespace draforge-system "$@" > "$output"
}

extract_kind() {
    local manifest kind output
    manifest=$1
    kind=$2
    output=$3
    awk -v kind="$kind" '
        /^---$/ { if (found) exit; capture=0; doc=""; next }
        { doc=doc $0 ORS }
        $0 == "kind: " kind { capture=1; found=1 }
        END { if (found && capture) printf "%s", doc }
    ' "$manifest" > "$output"
    [[ -s "$output" ]] || fail "could not extract kind $kind from $manifest"
}

expect_invalid() {
    local name values_file
    name=$1
    values_file=$2
    if "$HELM_BIN" template draforge "$CHART_DIR" -f "$values_file" >/dev/null 2>&1; then
        fail "invalid exposure profile unexpectedly passed: $name"
    fi
}

work_dir=$(mktemp -d)
cleanup() {
    rm -rf "$work_dir"
}
trap cleanup EXIT

default_manifest="$work_dir/default.yaml"
render "$default_manifest"
for kind in 'kind: Gateway' 'kind: HTTPRoute' 'kind: Ingress'; do
    assert_not_contains "$default_manifest" "$kind"
done
assert_not_contains "$default_manifest" 'name: CORS_ALLOWED_ORIGINS'

local_manifest="$work_dir/local-demo.yaml"
render "$local_manifest" -f "$CHART_DIR/values-local-demo.yaml"
local_gateway="$work_dir/local-gateway.yaml"
local_route="$work_dir/local-route.yaml"
extract_kind "$local_manifest" Gateway "$local_gateway"
extract_kind "$local_manifest" HTTPRoute "$local_route"
assert_contains "$local_gateway" 'protocol: HTTP'
assert_contains "$local_gateway" 'port: 80'
assert_not_contains "$local_gateway" 'protocol: HTTPS'
assert_not_contains "$local_gateway" 'certificateRefs:'
assert_contains "$local_route" '        - name: draforge-server'
assert_contains "$local_route" 'port: 8080'
assert_contains "$local_manifest" 'name: CORS_ALLOWED_ORIGINS'
assert_contains "$local_manifest" 'value: "*"'

public_manifest="$work_dir/secure-public.yaml"
render "$public_manifest" -f "$CHART_DIR/values-public-example.yaml"
public_gateway="$work_dir/public-gateway.yaml"
public_route="$work_dir/public-route.yaml"
extract_kind "$public_manifest" Gateway "$public_gateway"
extract_kind "$public_manifest" HTTPRoute "$public_route"
assert_contains "$public_gateway" 'protocol: HTTPS'
assert_contains "$public_gateway" 'port: 443'
assert_contains "$public_gateway" 'hostname: "draforge.example.com"'
assert_contains "$public_gateway" 'certificateRefs:'
assert_contains "$public_gateway" 'name: draforge-example-tls'
assert_contains "$public_route" '        - name: oauth2-proxy'
assert_contains "$public_route" 'port: 4180'
assert_not_contains "$public_route" '        - name: draforge-server'
assert_contains "$public_manifest" 'name: CORS_ALLOWED_ORIGINS'
assert_contains "$public_manifest" 'value: "https://draforge.example.com"'

cat > "$work_dir/secure-ingress.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - https://draforge.example.com
gateway:
  enabled: false
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-example-tls
  authentication:
    proxyService:
      name: oauth2-proxy
      port: 4180
  ingress:
    enabled: true
YAML
ingress_manifest="$work_dir/secure-ingress-rendered.yaml"
render "$ingress_manifest" -f "$work_dir/secure-ingress.yaml"
ingress_doc="$work_dir/ingress.yaml"
extract_kind "$ingress_manifest" Ingress "$ingress_doc"
assert_contains "$ingress_doc" 'host: "draforge.example.com"'
assert_contains "$ingress_doc" 'secretName: draforge-example-tls'
assert_contains "$ingress_doc" 'name: oauth2-proxy'
assert_contains "$ingress_doc" 'number: 4180'
assert_not_contains "$ingress_doc" 'name: draforge-server'

showcase_manifest="$work_dir/showcase.yaml"
render "$showcase_manifest" -f "$CHART_DIR/values-showcase-docr.yaml"
for image in \
    'registry.digitalocean.com/draforge/draforge:server-latest' \
    'registry.digitalocean.com/draforge/draforge:controller-latest' \
    'registry.digitalocean.com/draforge/draforge:sim-driver-latest'; do
    assert_contains "$showcase_manifest" "image: \"$image\""
done
assert_contains "$showcase_manifest" 'protocol: HTTP'
assert_contains "$showcase_manifest" 'value: "*"'

cat > "$work_dir/enabled-without-security.yaml" <<'YAML'
gateway:
  enabled: true
YAML
expect_invalid enabled-without-security "$work_dir/enabled-without-security.yaml"

cat > "$work_dir/insecure-with-tls.yaml" <<'YAML'
gateway:
  enabled: true
  allowInsecureHTTP: true
  tls:
    enabled: true
    secretName: demo-tls
YAML
expect_invalid insecure-with-tls "$work_dir/insecure-with-tls.yaml"

cat > "$work_dir/secure-wildcard-cors.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - "*"
gateway:
  enabled: true
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-tls
  authentication:
    proxyService:
      name: oauth2-proxy
      port: 4180
YAML
expect_invalid secure-wildcard-cors "$work_dir/secure-wildcard-cors.yaml"

cat > "$work_dir/secure-cors-with-path.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - https://draforge.example.com/path
gateway:
  enabled: true
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-tls
  authentication:
    proxyService:
      name: oauth2-proxy
      port: 4180
YAML
expect_invalid secure-cors-with-path "$work_dir/secure-cors-with-path.yaml"

cat > "$work_dir/secure-http-cors.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - http://draforge.example.com
gateway:
  enabled: true
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-tls
  authentication:
    proxyService:
      name: oauth2-proxy
      port: 4180
YAML
expect_invalid secure-http-cors "$work_dir/secure-http-cors.yaml"

cat > "$work_dir/both-providers.yaml" <<'YAML'
gateway:
  enabled: true
  allowInsecureHTTP: true
  ingress:
    enabled: true
YAML
expect_invalid both-providers "$work_dir/both-providers.yaml"

cat > "$work_dir/no-exposure-insecure-ack.yaml" <<'YAML'
gateway:
  allowInsecureHTTP: true
YAML
expect_invalid no-exposure-insecure-ack "$work_dir/no-exposure-insecure-ack.yaml"

cat > "$work_dir/secure-without-proxy.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - https://draforge.example.com
gateway:
  enabled: true
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-tls
YAML
expect_invalid secure-without-proxy "$work_dir/secure-without-proxy.yaml"

cat > "$work_dir/secure-direct-backend.yaml" <<'YAML'
server:
  cors:
    allowedOrigins:
      - https://draforge.example.com
gateway:
  enabled: true
  hostname: draforge.example.com
  tls:
    enabled: true
    secretName: draforge-tls
  authentication:
    proxyService:
      name: draforge-server
      port: 8080
YAML
expect_invalid secure-direct-backend "$work_dir/secure-direct-backend.yaml"

echo "Dashboard exposure Helm contract verified."
