#!/usr/bin/env bash
set -euo pipefail

workspace=web/pnpm-workspace.yaml
lockfile=web/pnpm-lock.yaml
dockerfile=build/package/Dockerfile.server

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local text=$2
  grep -Fq -- "$text" "$file" || fail "$file is missing: $text"
}

assert_absent() {
  local file=$1
  local text=$2
  if grep -Fq -- "$text" "$file"; then
    fail "$file contains disallowed package entry: $text"
  fi
}

assert_contains "$workspace" "autoInstallPeers: false"
for adapter in cssom html-escaper uhyphen; do
  assert_contains "$workspace" "$adapter: \"link:./vendor/$adapter\""
  assert_contains "$lockfile" "$adapter: link:./vendor/$adapter"
  assert_contains "$lockfile" "$adapter: link:vendor/$adapter"
done

for package in \
  "cssom@" \
  "html-escaper@" \
  "uhyphen@" \
  "css.escape@" \
  "min-indent@" \
  "@testing-library/jest-dom@" \
  "data-urls@" \
  "rrweb-cssom@" \
  "jsdom@" \
  "happy-dom@"; do
  assert_absent "$lockfile" "$package"
done

node_base='FROM node:22.23.1-alpine@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2 AS frontend-builder'
corepack_install='npm install --global --ignore-scripts --force corepack@0.35.0'
pnpm_activate='corepack prepare pnpm@11.5.2 --activate'
assert_contains "$dockerfile" "$node_base"
assert_contains "$dockerfile" "$corepack_install"
assert_contains "$dockerfile" "$pnpm_activate"

workspace_copy='COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./'
vendor_copy='COPY web/vendor ./vendor'
install_command='RUN pnpm install --frozen-lockfile --ignore-scripts'
assert_contains "$dockerfile" "$workspace_copy"
assert_contains "$dockerfile" "$vendor_copy"
assert_contains "$dockerfile" "$install_command"

workspace_line=$(grep -Fn -- "$workspace_copy" "$dockerfile" | cut -d: -f1)
vendor_line=$(grep -Fn -- "$vendor_copy" "$dockerfile" | cut -d: -f1)
install_line=$(grep -Fn -- "$install_command" "$dockerfile" | cut -d: -f1)
if (( workspace_line >= install_line || vendor_line >= install_line )); then
  fail "Dockerfile must copy workspace policy and local vendor packages before pnpm install"
fi

node --input-type=module <<'NODE'
import { parse } from './web/vendor/cssom/index.js';
import { escape, unescape } from './web/vendor/html-escaper/index.js';
import uhyphen from './web/vendor/uhyphen/index.js';

const sheet = parse('.card { display: grid; }');
const index = sheet.insertRule('.badge { display: inline-flex; }');
if (index !== 0 || sheet.cssRules.length !== 1) {
  throw new Error('local CSSOM adapter insertRule contract failed');
}
sheet.deleteRule(0);
if (sheet.cssRules.length !== 0) {
  throw new Error('local CSSOM adapter deleteRule contract failed');
}
if (uhyphen('backgroundColor') !== 'background-color') {
  throw new Error('local uhyphen adapter contract failed');
}
const escaped = escape('<button aria-label="x&y">');
if (escaped !== '&lt;button aria-label=&quot;x&amp;y&quot;&gt;' || unescape(escaped) !== '<button aria-label="x&y">') {
  throw new Error('local html-escaper adapter contract failed');
}
NODE

echo "Frontend dependency policy verified."
