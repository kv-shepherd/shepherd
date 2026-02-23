#!/usr/bin/env bash

set -euo pipefail

LIVE_SPEC_DIR="web/tests/e2e"

# Collect all live spec files
mapfile -t LIVE_SPECS < <(find "$LIVE_SPEC_DIR" -maxdepth 1 -name "*-live.spec.ts" | sort)

if [[ "${#LIVE_SPECS[@]}" -eq 0 ]]; then
  echo "FAIL: no live e2e specs found in $LIVE_SPEC_DIR"
  exit 1
fi

echo "INFO: checking ${#LIVE_SPECS[@]} live spec(s) for mock-network and skip patterns"

# Patterns that are NEVER allowed in live specs
declare -a blocked=(
  "page.route("
  "context.route("
  "browserContext.route("
  "route.fulfill("
)

# test.skip() is not allowed as a statement (only in comments is acceptable)
# We match the actual call: leading whitespace + test.skip()
SKIP_PATTERN='^\s*test\.skip\(\)'

overall_violations=()
for LIVE_SPEC in "${LIVE_SPECS[@]}"; do
  for needle in "${blocked[@]}"; do
    if rg -nF "$needle" "$LIVE_SPEC" > /dev/null 2>&1; then
      overall_violations+=("${LIVE_SPEC}: blocked mock pattern: $needle")
    fi
  done
  # Check for actual test.skip() calls (not in comments)
  if rg -n "$SKIP_PATTERN" "$LIVE_SPEC" > /dev/null 2>&1; then
    overall_violations+=("${LIVE_SPEC}: blocked skip pattern: test.skip() — use console.warn() instead")
  fi
done

if [[ "${#overall_violations[@]}" -gt 0 ]]; then
  echo "FAIL: live e2e spec(s) contain forbidden patterns"
  for v in "${overall_violations[@]}"; do
    echo " - $v"
  done
  echo ""
  echo "Rules:"
  echo "  1. All *-live.spec.ts files must run against real backend without route mocking."
  echo "  2. test.skip() is forbidden — use console.warn() to log missing preconditions."
  exit 1
fi

echo "OK: live e2e no-mock/no-skip check passed (${#LIVE_SPECS[@]} spec(s) checked)"
