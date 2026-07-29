#!/usr/bin/env bash
# Verify an already-published DRAForge release without mutating it.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root_dir=$(cd "$script_dir/.." && pwd)
release_source=${RELEASE_SOURCE_DIR:-$root_dir}
release_tag=${1:-}
repository=${GITHUB_REPOSITORY:-oaslananka/draforge}
max_attempts=${RELEASE_VERIFY_ATTEMPTS:-12}
retry_seconds=${RELEASE_VERIFY_RETRY_SECONDS:-10}
issuer=https://token.actions.githubusercontent.com

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

for tool in gh jq sha256sum cosign docker helm; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done

[[ "$release_tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.(0|[1-9][0-9]*))?$ ]] || \
  fail "release tag must match vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.NUMBER"
[[ "$max_attempts" =~ ^[1-9][0-9]*$ ]] || fail "RELEASE_VERIFY_ATTEMPTS must be a positive integer"
[[ "$retry_seconds" =~ ^[0-9]+$ ]] || fail "RELEASE_VERIFY_RETRY_SECONDS must be a non-negative integer"
[[ -f "$release_source/deploy/helm/draforge/Chart.yaml" ]] || fail "immutable release source is unavailable"

release_version=${release_tag#v}
source_version=$(awk '$1 == "appVersion:" {gsub(/"/, "", $2); print $2; exit}' \
  "$release_source/deploy/helm/draforge/Chart.yaml")
[[ "$source_version" == "$release_version" ]] || \
  fail "release source version $source_version does not match tag version $release_version"
tag_pattern=${release_tag//./\\.}
identity_pattern="^https://github\\.com/oaslananka/draforge/\\.github/workflows/release\\.yml@refs/(heads/main|tags/${tag_pattern})$"

work_dir=$(mktemp -d)
cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

release_json="$work_dir/release.json"
gh release view "$release_tag" --repo "$repository" \
  --json tagName,isDraft,isPrerelease,publishedAt,assets > "$release_json"
jq -e --arg tag "$release_tag" '
  .tagName == $tag and
  .isDraft == false and
  (.publishedAt | type == "string") and
  ([.assets[].name] | index("checksums.txt") != null) and
  ([.assets[].name] | index("checksums.txt.sig") != null)
' "$release_json" >/dev/null || fail "GitHub release metadata or required assets are incomplete"

gh release download "$release_tag" --repo "$repository" --dir "$work_dir/assets"
(
  cd "$work_dir/assets"
  sha256sum -c checksums.txt
)

sbom_count=0
for sbom in "$work_dir"/assets/*.sbom.json; do
  [[ -e "$sbom" ]] || continue
  jq -e '.bomFormat == "CycloneDX" and (.specVersion | type == "string")' "$sbom" >/dev/null || \
    fail "invalid CycloneDX SBOM: $(basename "$sbom")"
  sbom_count=$((sbom_count + 1))
done
[[ "$sbom_count" -eq 8 ]] || fail "expected 8 CycloneDX SBOMs, found $sbom_count"

cosign verify-blob \
  --certificate-identity-regexp "$identity_pattern" \
  --certificate-oidc-issuer "$issuer" \
  --bundle "$work_dir/assets/checksums.txt.sig" \
  "$work_dir/assets/checksums.txt"

verified_images=0
for attempt in $(seq 1 "$max_attempts"); do
  if CHART_DIR="$release_source/deploy/helm/draforge" VERIFY_REMOTE_IMAGES=1 \
      "$script_dir/verify-chart-images.sh" "$release_version"; then
    verified_images=1
    break
  fi
  if [[ "$attempt" -lt "$max_attempts" ]]; then
    echo "Public image manifests are not ready (attempt $attempt/$max_attempts); retrying in ${retry_seconds}s..." >&2
    sleep "$retry_seconds"
  fi
done
[[ "$verified_images" -eq 1 ]] || fail "public multi-arch manifests did not converge"

for component in server controller sim-driver; do
  cosign verify \
    --certificate-identity-regexp "$identity_pattern" \
    --certificate-oidc-issuer "$issuer" \
    "ghcr.io/oaslananka/draforge-${component}:${release_version}" >/dev/null
done

echo "Published release verified: tag=$release_tag version=$release_version assets=checksummed sboms=$sbom_count images=3"
