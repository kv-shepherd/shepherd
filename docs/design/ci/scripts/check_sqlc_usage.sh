#!/usr/bin/env bash
# check_sqlc_usage.sh - Enforce ADR-0012 sqlc usage restrictions
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

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Whitelisted directories where sqlc usage is allowed
ALLOWED_DIRS=(
    "internal/repository/sqlc/"
    "internal/usecase/"
    "docs/"           # Documentation examples are allowed
    "test/"           # Test files are allowed
    "*_test.go"       # Test files are allowed
)

# Check if the project root has sqlc-generated code
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

# Search for sqlc imports outside whitelisted directories
while IFS= read -r -d '' file; do
    # Check if file is in allowed directory
    is_allowed=false
    for allowed in "${ALLOWED_DIRS[@]}"; do
        if [[ "$file" == *"$allowed"* ]] || [[ "$file" == *_test.go ]]; then
            is_allowed=true
            break
        fi
    done
    
    if [ "$is_allowed" = false ]; then
        # Check if file imports sqlc
        if grep -q "repository/sqlc" "$file" 2>/dev/null; then
            VIOLATIONS+=("$file")
        fi
    fi
done < <(find "$PROJECT_ROOT" -name "*.go" -type f -print0 2>/dev/null || true)

# Report results
if [ ${#VIOLATIONS[@]} -gt 0 ]; then
    echo -e "${RED}❌ VIOLATION: sqlc usage found outside whitelisted directories!${NC}"
    echo ""
    echo "The following files import sqlc but are NOT in allowed directories:"
    echo ""
    for violation in "${VIOLATIONS[@]}"; do
        echo "  ✗ $violation"
    done
    echo ""
    echo "ADR-0012 restricts sqlc usage to:"
    echo "  - internal/repository/sqlc/ (query definitions)"
    echo "  - internal/usecase/ (atomic transaction orchestration)"
    echo ""
    echo "If you need sqlc in other locations, update ADR-0012 first."
    exit 1
else
    echo -e "${GREEN}✓ All sqlc usages are within allowed directories${NC}"
    echo ""
    echo "Checked: $(find "$PROJECT_ROOT" -name "*.go" -type f 2>/dev/null | wc -l) Go files"
fi

echo ""
echo "=========================================="
echo "Check completed successfully"
echo "=========================================="
