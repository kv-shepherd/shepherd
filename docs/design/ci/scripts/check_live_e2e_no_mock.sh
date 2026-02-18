#!/usr/bin/env bash

set -euo pipefail

LIVE_SPEC="web/tests/e2e/master-flow-live.spec.ts"

if [[ ! -f "$LIVE_SPEC" ]]; then
  echo "FAIL: live e2e spec missing: $LIVE_SPEC"
  exit 1
fi

declare -a blocked=(
  "page.route("
  "context.route("
  "browserContext.route("
  "route.fulfill("
)

violations=()
for needle in "${blocked[@]}"; do
  if rg -nF "$needle" "$LIVE_SPEC" >/dev/null 2>&1; then
    violations+=("$needle")
  fi
done

if [[ "${#violations[@]}" -gt 0 ]]; then
  echo "FAIL: live e2e spec contains mock-network patterns"
  for v in "${violations[@]}"; do
    echo " - blocked pattern: $v"
  done
  echo "Rule: master-flow live e2e must run against real backend without route mocking."
  exit 1
fi

echo "OK: live e2e no-mock check passed"
