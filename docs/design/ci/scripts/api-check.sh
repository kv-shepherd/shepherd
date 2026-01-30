#!/bin/bash
# api-check.sh - Verify generated API code is in sync with OpenAPI spec
# Used in CI to enforce ADR-0021 Contract-First design
#
# Usage: ./scripts/api-check.sh
# Exit codes:
#   0 - Generated code is in sync
#   1 - Generated code is out of sync (regeneration needed)
#   2 - Generation failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "  API Contract Sync Check (ADR-0021)"
echo "=========================================="
echo ""

if [ ! -f "${PROJECT_ROOT}/api/openapi.yaml" ]; then
    echo -e "${YELLOW}⚠️  OpenAPI spec not found at api/openapi.yaml. Skipping sync check.${NC}"
    exit 0
fi

# Step 1: Store current state
echo "📸 Storing current state of generated files..."
TEMP_DIR=$(mktemp -d)
trap "rm -rf ${TEMP_DIR}" EXIT

if [ -d "${PROJECT_ROOT}/internal/api/generated" ]; then
    cp -r "${PROJECT_ROOT}/internal/api/generated" "${TEMP_DIR}/go-backup"
fi

if [ -f "${PROJECT_ROOT}/web/src/types/api.gen.ts" ]; then
    mkdir -p "${TEMP_DIR}/ts-backup"
    cp "${PROJECT_ROOT}/web/src/types/api.gen.ts" "${TEMP_DIR}/ts-backup/"
fi

# Step 2: Regenerate code
echo "🔄 Regenerating API code from OpenAPI spec..."
cd "${PROJECT_ROOT}"

if ! make api-generate > /dev/null 2>&1; then
    echo -e "${RED}❌ Code generation failed!${NC}"
    echo "Please check that oapi-codegen and openapi-typescript are properly installed."
    exit 2
fi

# Step 3: Compare Go generated code
echo "🔍 Comparing Go generated code..."
GO_DIFF=""
if [ -d "${TEMP_DIR}/go-backup" ]; then
    GO_DIFF=$(diff -rq "${TEMP_DIR}/go-backup" "${PROJECT_ROOT}/internal/api/generated" 2>&1 || true)
fi

# Step 4: Compare TypeScript generated code
echo "🔍 Comparing TypeScript generated code..."
TS_DIFF=""
if [ -f "${TEMP_DIR}/ts-backup/api.gen.ts" ]; then
    TS_DIFF=$(diff "${TEMP_DIR}/ts-backup/api.gen.ts" "${PROJECT_ROOT}/web/src/types/api.gen.ts" 2>&1 || true)
fi

# Step 5: Report results
echo ""
echo "=========================================="

if [ -z "${GO_DIFF}" ] && [ -z "${TS_DIFF}" ]; then
    echo -e "${GREEN}✅ Generated code is in sync with OpenAPI spec${NC}"
    echo ""
    echo "All generated files match the current OpenAPI specification."
    exit 0
else
    echo -e "${RED}❌ Generated code is OUT OF SYNC with OpenAPI spec${NC}"
    echo ""
    
    if [ -n "${GO_DIFF}" ]; then
        echo -e "${YELLOW}Go files with differences:${NC}"
        echo "${GO_DIFF}"
        echo ""
    fi
    
    if [ -n "${TS_DIFF}" ]; then
        echo -e "${YELLOW}TypeScript files with differences:${NC}"
        echo "web/src/types/api.gen.ts"
        echo ""
    fi
    
    echo "To fix this issue:"
    echo "  1. Run: make api-generate"
    echo "  2. Commit the regenerated files"
    echo ""
    echo "This ensures the OpenAPI spec remains the single source of truth (ADR-0021)."
    
    # Restore original files (for next CI run to be accurate)
    if [ -d "${TEMP_DIR}/go-backup" ]; then
        rm -rf "${PROJECT_ROOT}/internal/api/generated"
        cp -r "${TEMP_DIR}/go-backup" "${PROJECT_ROOT}/internal/api/generated"
    fi
    if [ -f "${TEMP_DIR}/ts-backup/api.gen.ts" ]; then
        cp "${TEMP_DIR}/ts-backup/api.gen.ts" "${PROJECT_ROOT}/web/src/types/api.gen.ts"
    fi
    
    exit 1
fi
