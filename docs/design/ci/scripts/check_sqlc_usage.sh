#!/usr/bin/env bash
# docs/design/ci/scripts/check_sqlc_usage.sh - Enforce ADR-0012 sqlc usage restrictions
#
# This script ensures sqlc is only used in whitelisted directories.
# ADR-0012 specifies that sqlc should ONLY be used for core atomic transactions.
#
# Allowed directories:
#   - internal/repository/sqlc/  (sqlc query definitions)
#   - internal/usecase/          (core atomic transactions)
#
# Blocks CI: YES

set -euo pipefail

SQLC_IMPORT='kv-shepherd.io/shepherd/internal/repository/sqlc'

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

echo "✅ All sqlc usages are within allowed directories"

echo ""
echo "=========================================="
echo "Check completed successfully"
echo "=========================================="
