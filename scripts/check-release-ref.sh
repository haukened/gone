#!/usr/bin/env bash
# Release guard, run first by every job that publishes something.
#
#   scripts/check-release-ref.sh
#
# Fails unless the workflow was started by a tag push whose name is strict
# semver (vMAJOR.MINOR.PATCH with an optional -PRERELEASE suffix, the same
# rule as scripts/release.sh) and whose commit is already on main. Reads
# GITHUB_REF_TYPE, GITHUB_REF_NAME and GITHUB_SHA. Needs a checkout with full
# history (actions/checkout with fetch-depth: 0); it fetches main itself, and
# the repository is public, so no git credentials are needed.
#
# This catches a mistaken tag. It does not stop someone who can push tags,
# since a tag push runs the workflow file from the tagged commit: restrict tag
# creation to the maintainer with a repository tag ruleset as well.
set -euo pipefail

fail() {
  echo "::error title=Release guard::$*" >&2
  exit 1
}

: "${GITHUB_REF_TYPE:?}" "${GITHUB_REF_NAME:?}" "${GITHUB_SHA:?}"

if [[ "$GITHUB_REF_TYPE" != tag ]]; then
  fail "releases are built only from tags, not a $GITHUB_REF_TYPE ($GITHUB_REF_NAME)"
fi
# Keep in sync with scripts/release.sh.
if [[ ! "$GITHUB_REF_NAME" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  fail "tag '$GITHUB_REF_NAME' is not vMAJOR.MINOR.PATCH[-PRERELEASE]"
fi

if [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
  fail "shallow checkout: set fetch-depth: 0 so the tag can be checked against main"
fi
git fetch --no-tags --quiet origin +refs/heads/main:refs/remotes/origin/main
commit="$(git rev-parse --verify "${GITHUB_SHA}^{commit}")"
if ! git merge-base --is-ancestor "$commit" refs/remotes/origin/main; then
  fail "tag $GITHUB_REF_NAME points at $commit, which is not on main. Tag a commit that has been merged to main."
fi

echo "Release guard passed: $GITHUB_REF_NAME ($commit) is on main."
