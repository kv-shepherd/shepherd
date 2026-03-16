#!/usr/bin/env bash
# docs/design/ci/scripts/check_manual_di.sh
# Strict Manual DI Policy Check
#
# Goal: keep dependency wiring centralized under internal/app/.
#
# Checks:
# 1. Forbid Service/Repository struct literal wiring outside internal/app/
# 2. Forbid Service/Repository/UseCase constructor calls outside internal/app/
# 3. Warn on init() usage (manual review for dependency initialization)
# 4. Forbid Redis imports (Redis dependency removed)
# 5. Forbid Wire imports (Wire dependency removed)
#
# Usage: bash docs/design/ci/scripts/check_manual_di.sh
# Exit code: 0 = pass, 1 = violations

set -euo pipefail

echo "Checking manual DI policy..."

ERRORS=0
CONSTRUCTOR_PATTERNS='(?<!func )\bNew[A-Z][a-zA-Z0-9]*(Service|Repository|UseCase|Gateway|Sender)\('
SCAN_DIRS=(
    "internal/api"
    "internal/jobs"
    "internal/domain"
    "internal/infrastructure"
    "cmd"
)

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
# Check 2: forbid constructor calls outside internal/app/
# ===========================================
echo "Check 2: constructor calls outside composition root..."

CONSTRUCTOR_VIOLATIONS=""
for dir in "${SCAN_DIRS[@]}"; do
    [[ -d "${dir}" ]] || continue
    MATCHES=$(rg -n --pcre2 "${CONSTRUCTOR_PATTERNS}" "${dir}" \
        --glob '!**/*_test.go' \
        --glob '!**/mock.go' \
        --glob '!**/testmain_test.go' \
        --glob '!**/testutil/**' 2>/dev/null || true)
    if [[ -n "${MATCHES}" ]]; then
        CONSTRUCTOR_VIOLATIONS+="${MATCHES}"$'\n'
    fi
done

if [ -n "$CONSTRUCTOR_VIOLATIONS" ]; then
    echo "ERROR: found constructor calls outside internal/app/ (composition root):"
    printf "%s" "$CONSTRUCTOR_VIOLATIONS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 2 passed"
fi

# ===========================================
# Check 3: warn on init() dependency initialization
# ===========================================
echo "Check 3: init() dependency initialization..."

INIT_VIOLATIONS=$(grep -rn 'func init()' --include="*.go" internal/ 2>/dev/null | grep -v "_test.go" | grep -v "//.*func init()" || true)

if [ -n "$INIT_VIOLATIONS" ]; then
    echo "WARNING: found init() function(s) (verify they do not perform dependency initialization):"
    echo "$INIT_VIOLATIONS"
    echo "Hint: registration-only init() (for example Ent schema registration) can be valid"
    # Warning instead of error because init() has valid uses.
else
    echo "OK: Check 3 passed"
fi

# ===========================================
# Check 4: forbid Redis imports
# ===========================================
echo "Check 4: Redis imports..."

REDIS_IMPORTS=$(grep -rn 'go-redis\|"github.com/redis' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$REDIS_IMPORTS" ]; then
    echo "ERROR: found Redis imports (Redis dependency has been removed):"
    echo "$REDIS_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 4 passed"
fi

# ===========================================
# Check 5: forbid Wire imports
# ===========================================
echo "Check 5: Wire imports..."

WIRE_IMPORTS=$(grep -rn 'google/wire\|goforj/wire' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$WIRE_IMPORTS" ]; then
    echo "ERROR: found Wire imports (Wire dependency has been removed):"
    echo "$WIRE_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Check 5 passed"
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
