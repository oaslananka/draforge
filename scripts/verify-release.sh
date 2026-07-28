#!/usr/bin/env bash
# Verifies a release artifact payload for completeness and provenance
# Usage: ./scripts/verify-release.sh

set -euo pipefail

DIST_DIR=${DIST_DIR:-"dist"}
CHARTS_DIR=${CHARTS_DIR:-"charts"}
REQUIRE_DOCKER_ARTIFACTS=${REQUIRE_DOCKER_ARTIFACTS:-"0"}

echo "==> Verifying DRAForge release artifacts..."

echo "==> Verifying release metadata contract..."
python3 scripts/verify-release-metadata.py --self-test --root .

echo "==> Verifying GoReleaser Docker v2 configuration..."
python3 scripts/verify-goreleaser-docker-v2.py --self-test

echo "==> Verifying Helm chart image contract..."
scripts/verify-chart-images.sh

fail() {
  echo "❌ ERROR: $1" >&2
  exit 1
}

warn() {
  echo "⚠️  WARNING: $1" >&2
}

# 1. Checksums Presence & Validation
if [ ! -f "$DIST_DIR/checksums.txt" ]; then
  fail "Missing $DIST_DIR/checksums.txt"
fi
echo "✅ Checksums file present"

echo "==> Checking file hashes against checksums.txt..."
cd "$DIST_DIR"
if ! sha256sum --ignore-missing --quiet -c checksums.txt; then
  fail "Checksum validation failed for one or more files"
fi
cd - >/dev/null
echo "✅ All present files match checksums"

# 2. GoReleaser Artifact Expectations (Binaries/Archives)
echo "==> Checking for expected GoReleaser binaries/archives..."
for bin in draforge draforge-controller draforge-sim-driver; do
  if ! grep -q "${bin}_" "$DIST_DIR/checksums.txt"; then
    fail "No $bin artifacts found in checksums.txt"
  fi
done
echo "✅ All core binaries accounted for in checksums.txt"

# 3. SBOM Presence
echo "==> Checking SBOM presence..."
if ls "$DIST_DIR"/*.sbom 1> /dev/null 2>&1 || ls "$DIST_DIR"/*.sbom.json 1> /dev/null 2>&1; then
  sbom_count=$(find "$DIST_DIR" -maxdepth 1 -type f \( -name '*.sbom' -o -name '*.sbom.json' \) -print | wc -l)
  echo "✅ Found $sbom_count SBOM file(s)"
else
  warn "No SBOM files found in $DIST_DIR/ (expected for --skip=sbom or snapshot dry-runs)"
fi

# 4. Chart Package Verification
echo "==> Checking Helm chart packages..."
if [ -d "$CHARTS_DIR" ]; then
  if ls "$CHARTS_DIR"/draforge-*.tgz 1> /dev/null 2>&1; then
    chart_count=$(find "$CHARTS_DIR" -maxdepth 1 -type f -name 'draforge-*.tgz' -print | wc -l)
    echo "✅ Found $chart_count Helm chart package(s)"
  else
    warn "No Helm chart packages found in $CHARTS_DIR/ (expected if Helm package step was skipped)"
  fi
else
  warn "Directory $CHARTS_DIR/ not found. Skipping Helm chart check."
fi

# 5. Container Image Metadata (from GoReleaser artifacts.json)
echo "==> Checking container image metadata expectations..."
if [[ -f "$DIST_DIR/artifacts.json" ]]; then
  docker_artifact_count=$(python3 - "$DIST_DIR/artifacts.json" <<'PY'
import json
import sys
from pathlib import Path

component_ids = {
    "draforge-server-image",
    "draforge-controller-image",
    "draforge-sim-driver-image",
}
artifacts = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
count = 0
if isinstance(artifacts, list):
    for artifact in artifacts:
        if not isinstance(artifact, dict) or artifact.get("type") != "Docker Image":
            continue
        extra = artifact.get("extra")
        if isinstance(extra, dict) and extra.get("ID") in component_ids:
            count += 1
print(count)
PY
  )
  if [[ "$docker_artifact_count" -gt 0 ]]; then
    python3 scripts/verify-goreleaser-docker-artifacts.py --self-test --validate
  elif [[ "$REQUIRE_DOCKER_ARTIFACTS" == "1" ]]; then
    fail "No GoReleaser Docker v2 artifacts found in $DIST_DIR/artifacts.json"
  else
    warn "No Docker v2 image metadata found in artifacts.json (expected for --skip=docker)"
  fi
elif [[ "$REQUIRE_DOCKER_ARTIFACTS" == "1" ]]; then
  fail "$DIST_DIR/artifacts.json not found"
else
  warn "$DIST_DIR/artifacts.json not found"
fi

echo "==> ✅ Release verification complete!"
