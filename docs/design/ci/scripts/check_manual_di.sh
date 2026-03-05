#!/bin/bash
# scripts/ci/check_manual_di.sh
# Strict Manual DI Policy Check
#
# Goal: keep dependency wiring centralized under internal/app/.
#
# Checks:
# 1. Forbid Service/Repository struct literal wiring outside internal/app/
# 2. Warn on init() usage (manual review for dependency initialization)
# 3. Forbid Redis imports (Redis dependency removed)
# 4. Forbid Wire imports (Wire dependency removed)
#
# Usage: ./check_manual_di.sh
# Exit code: 0 = pass, 1 = violations

set -e

echo "Checking manual DI policy..."

ERRORS=0

# ===========================================
# Check 1: forbid dependency wiring outside internal/app/
# ===========================================
echo "Check 1: decentralized dependency wiring..."

VIOLATIONS=$(grep -rn '&service\.\|&repository\.' --include="*.go" internal/ 2>/dev/null | grep -v "internal/app/" | grep -v "_test.go" || true)

if [ -n "$VIOLATIONS" ]; then
    echo "ERROR: found decentralized dependency wiring (must be centralized in internal/app/):"
    echo "$VIOLATIONS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 1 passed"
fi

# ===========================================
# Check 2: warn on init() dependency initialization
# ===========================================
echo "Check 2: init() dependency initialization..."

INIT_VIOLATIONS=$(grep -rn 'func init()' --include="*.go" internal/ 2>/dev/null | grep -v "_test.go" | grep -v "//.*func init()" || true)

if [ -n "$INIT_VIOLATIONS" ]; then
    echo "WARNING: found init() function(s) (verify they do not perform dependency initialization):"
    echo "$INIT_VIOLATIONS"
    echo "Hint: registration-only init() (for example Ent schema registration) can be valid"
    # Warning instead of error because init() has valid uses.
else
    echo "OK: Check 2 passed"
fi

# ===========================================
# Check 3: forbid Redis imports
# ===========================================
echo "Check 3: Redis imports..."

REDIS_IMPORTS=$(grep -rn 'go-redis\|"github.com/redis' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$REDIS_IMPORTS" ]; then
    echo "ERROR: found Redis imports (Redis dependency has been removed):"
    echo "$REDIS_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 3 passed"
fi

# ===========================================
# Check 4: forbid Wire imports
# ===========================================
echo "Check 4: Wire imports..."

WIRE_IMPORTS=$(grep -rn 'google/wire\|goforj/wire' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$WIRE_IMPORTS" ]; then
    echo "ERROR: found Wire imports (Wire dependency has been removed):"
    echo "$WIRE_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 4 passed"
fi

# ===========================================
# Result
# ===========================================
echo ""
if [ $ERRORS -gt 0 ]; then
    echo "FAILED: manual DI policy check found $ERRORS error(s)"
    exit 1
else
    echo "PASSED: manual DI policy check"
    exit 0
fi
