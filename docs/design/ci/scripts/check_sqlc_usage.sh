#!/usr/bin/env bash
# docs/design/ci/scripts/check_sqlc_usage.sh - Enforce ADR-0012 sqlc usage restrictions
#
# This script ensures sqlc is only used in whitelisted directories and that
# sqlc production call sites do not also import Ent. ADR-0012 sqlc writers own
# atomic pgx transactions; mixing Ent in the same file makes transaction
# ownership ambiguous and has caused consistency gaps before.
#
# Allowed directories:
#   - internal/repository/sqlc/  (sqlc query definitions)
#   - internal/usecase/          (core atomic transactions)
#
# Blocks CI: YES

set -euo pipefail

SQLC_IMPORT='kv-shepherd.io/shepherd/internal/repository/sqlc'
ENT_IMPORT='kv-shepherd.io/shepherd/ent'

# Whitelisted directories where sqlc usage is allowed (ADR-0012).
ALLOWED_DIRS=(
    "internal/repository/sqlc/"
    "internal/usecase/"
)

PROJECT_ROOT="${1:-.}"

echo "=========================================="
echo "ADR-0012: sqlc Usage Scope Check"
echo "=========================================="
echo ""
echo "Allowed directories:"
for dir in "${ALLOWED_DIRS[@]}"; do
    echo "  ✓ $dir"
done
echo ""

# Find all Go files that import sqlc package
VIOLATIONS=()
MIXED_TX_VIOLATIONS=()

# Find all .go files that import the sqlc package outside the allowlist.
matches=$(grep -rl --include="*.go" "${SQLC_IMPORT}" "${PROJECT_ROOT}/internal" "${PROJECT_ROOT}/cmd" 2>/dev/null | grep -v "_test.go" || true)

# Report results
if [ -n "${matches}" ]; then
    while IFS= read -r file; do
        rel_path="${file#"${PROJECT_ROOT}/"}"

        allowed=false
        for allowed_dir in "${ALLOWED_DIRS[@]}"; do
            if [[ "${rel_path}" == ${allowed_dir}* ]]; then
                allowed=true
                break
            fi
        done

        if [ "${allowed}" = false ]; then
            VIOLATIONS+=("${rel_path}")
        elif grep -q "${ENT_IMPORT}" "${PROJECT_ROOT}/${rel_path}" 2>/dev/null; then
            MIXED_TX_VIOLATIONS+=("${rel_path}")
        fi
    done <<< "${matches}"
fi

if [ ${#VIOLATIONS[@]} -gt 0 ]; then
    echo "❌ VIOLATION: sqlc usage found outside whitelisted directories!"
    echo ""
    echo "The following files import sqlc but are NOT in allowed directories:"
    echo ""
    for violation in "${VIOLATIONS[@]}"; do
        echo "  ✗ ${violation}"
        grep -n "${SQLC_IMPORT}" "${PROJECT_ROOT}/${violation}" --max-count=3 2>/dev/null | sed 's/^/    /'
        echo ""
    done
    echo "ADR-0012 restricts sqlc usage to:"
    echo "  - internal/repository/sqlc/ (query definitions)"
    echo "  - internal/usecase/ (atomic transaction orchestration)"
    exit 1
fi

if [ ${#MIXED_TX_VIOLATIONS[@]} -gt 0 ]; then
    echo "❌ VIOLATION: sqlc and Ent imports found in the same production file!"
    echo ""
    echo "The following files import both sqlc and Ent:"
    echo ""
    for violation in "${MIXED_TX_VIOLATIONS[@]}"; do
        echo "  ✗ ${violation}"
        grep -n -e "${SQLC_IMPORT}" -e "${ENT_IMPORT}" "${PROJECT_ROOT}/${violation}" --max-count=6 2>/dev/null | sed 's/^/    /'
        echo ""
    done
    echo "ADR-0012 atomic writers must keep pgx/sqlc transaction ownership separate from Ent code."
    exit 1
fi

echo "✅ All sqlc usages are within allowed directories"
echo "✅ No production file mixes sqlc and Ent imports"

echo ""
echo "=========================================="
echo "Check completed successfully"
echo "=========================================="
