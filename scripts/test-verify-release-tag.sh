#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
VERIFIER="$SCRIPT_DIR/verify-release-tag.sh"
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

fail() {
  echo "release tag verifier test failed: $*" >&2
  exit 1
}

expect_failure() {
  local expected=$1
  shift
  local output
  if output=$("$@" 2>&1); then
    fail "command unexpectedly passed: $*"
  fi
  grep -Fq "$expected" <<<"$output" || fail "missing failure text '$expected': $output"
}

repo="$TEST_ROOT/repo"

git init --quiet --initial-branch=main "$repo"
git -C "$repo" config user.name "DRAForge Test"
git -C "$repo" config user.email "test@example.invalid"
printf 'one\n' > "$repo/file.txt"
git -C "$repo" add file.txt
git -C "$repo" commit --quiet -m "test: initial"
main_commit=$(git -C "$repo" rev-parse HEAD)

run_verifier() {
  local tag=$1
  local expected_commit=$2
  (
    cd "$repo"
    RELEASE_TAG="$tag" \
      RELEASE_MAIN_REF=main \
      RELEASE_EXPECTED_COMMIT="$expected_commit" \
      bash "$VERIFIER"
  )
}

git -C "$repo" tag -a v1.2.3 -m "DRAForge v1.2.3"
run_verifier v1.2.3 "$main_commit"

git -C "$repo" update-ref refs/remotes/origin/main "$main_commit"
(
  cd "$repo"
  RELEASE_TAG=v1.2.3 RELEASE_MAIN_REF=origin/main bash "$VERIFIER"
)

git -C "$repo" tag -a v1.2.4-rc.1 -m "DRAForge v1.2.4-rc.1"
run_verifier v1.2.4-rc.1 "$main_commit"

git -C "$repo" tag v1.2.4
expect_failure "must be an annotated tag object" run_verifier v1.2.4 "$main_commit"

git -C "$repo" tag -a release-1.2.3 -m "invalid name"
expect_failure "must match vMAJOR.MINOR.PATCH" run_verifier release-1.2.3 "$main_commit"

git -C "$repo" tag -a v1.2.6 -m ""
expect_failure "must contain a non-empty annotation message" run_verifier v1.2.6 "$main_commit"

git -C "$repo" switch --quiet -c feature
echo two >> "$repo/file.txt"
git -C "$repo" commit --quiet -am "test: feature"
feature_commit=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" tag -a v1.2.5 -m "off-main tag"
git -C "$repo" switch --quiet main
expect_failure "is not reachable from main" run_verifier v1.2.5 "$feature_commit"

expect_failure "does not resolve to expected commit" run_verifier v1.2.3 "$feature_commit"

echo "Release tag verifier tests passed."
