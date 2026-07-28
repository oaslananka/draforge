#!/usr/bin/env bash
# Verify immutable release-tag provenance before any release resources are used.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

fail() {
  echo "release tag verification failed: $*" >&2
  exit 1
}

release_tag=${RELEASE_TAG:-}
main_ref=${RELEASE_MAIN_REF:-origin/main}
expected_commit=${RELEASE_EXPECTED_COMMIT:-$(git rev-parse HEAD)}

[[ -n "$release_tag" ]] || fail "RELEASE_TAG is required"
[[ "$release_tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.(0|[1-9][0-9]*))?$ ]] || \
  fail "tag '$release_tag' must match vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.NUMBER"
[[ "$expected_commit" =~ ^[0-9a-f]{40}$ ]] || fail "RELEASE_EXPECTED_COMMIT must be a full lowercase Git commit SHA"
[[ "$main_ref" =~ ^(refs/remotes/origin/main|origin/main|main)$ ]] || fail "RELEASE_MAIN_REF must identify main"

release_ref="refs/tags/$release_tag"
git show-ref --verify --quiet "$release_ref" || fail "tag '$release_tag' does not exist locally"

object_type=$(git cat-file -t "$release_ref")
[[ "$object_type" == "tag" ]] || fail "tag '$release_tag' must be an annotated tag object; lightweight tags are not releasable"

tag_commit=$(git rev-list -n 1 "$release_ref")
[[ "$tag_commit" == "$expected_commit" ]] || \
  fail "tag '$release_tag' resolves to $tag_commit and does not resolve to expected commit $expected_commit"

git rev-parse --verify --quiet "${main_ref}^{commit}" >/dev/null || fail "main reference '$main_ref' is unavailable"
git merge-base --is-ancestor "$tag_commit" "$main_ref" || \
  fail "tag '$release_tag' target $tag_commit is not reachable from main ($main_ref)"

tag_message=$(git for-each-ref --format='%(contents)' "$release_ref")
[[ "$tag_message" =~ [^[:space:]] ]] || fail "tag '$release_tag' must contain a non-empty annotation message"

tagger_line=$(git cat-file tag "$release_ref" | grep -m1 '^tagger ' || true)
[[ -n "$tagger_line" ]] || fail "tag '$release_tag' does not contain tagger provenance"

signature_state=unsigned
if git cat-file tag "$release_ref" | grep -Eq -- '-----BEGIN (PGP|SSH) SIGNATURE-----'; then
  signature_state=signed
fi

printf 'Release tag verified: tag=%s commit=%s main=%s signature=%s\n' \
  "$release_tag" "$tag_commit" "$main_ref" "$signature_state"
