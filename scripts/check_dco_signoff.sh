#!/usr/bin/env bash
# check_dco_signoff.sh — Verify DCO Signed-off-by on all PR commits.
#
# Env contract (fail-fast if missing):
#   BASE_REF  — target branch name (e.g. "main")
#   HEAD_SHA  — head commit SHA of the PR
#
# Equivalent to the inline script previously in .github/workflows/dco.yaml.

set -euo pipefail

: "${BASE_REF:?BASE_REF is required (target branch name)}"
: "${HEAD_SHA:?HEAD_SHA is required (PR head commit SHA)}"

git fetch --no-tags --prune origin "${BASE_REF}:${BASE_REF}"

range="${BASE_REF}..${HEAD_SHA}"
commits=$(git rev-list --no-merges "$range")

if [ -z "$commits" ]; then
  echo "No commits to validate"
  exit 0
fi

missing=0
while IFS= read -r sha; do
  [ -z "$sha" ] && continue
  msg=$(git show -s --format=%B "$sha")
  if ! printf '%s\n' "$msg" | grep -Eiq '^Signed-off-by:\s+.+<.+>$'; then
    echo "Missing Signed-off-by in commit: $sha"
    git show -s --format='  %h %s (author: %an <%ae>)' "$sha"
    missing=1
  fi
done <<< "$commits"

if [ "$missing" -ne 0 ]; then
  echo "DCO check failed: one or more commits are missing Signed-off-by lines."
  exit 1
fi

echo "DCO check passed."
