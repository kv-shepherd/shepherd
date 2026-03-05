#!/bin/bash
# openapi-compat.sh - Enforce presence/freshness of OpenAPI 3.0-compatible spec
# Usage:
#   REQUIRE_OPENAPI_COMPAT=1 ./docs/design/ci/scripts/openapi-compat.sh
#
# Behavior:
# - If REQUIRE_OPENAPI_COMPAT=1: fail when compat spec is missing or stale.
# - If REQUIRE_OPENAPI_COMPAT!=1: warn on missing compat spec, but still fail if stale.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

CANONICAL_SPEC="${PROJECT_ROOT}/api/openapi.yaml"
COMPAT_SPEC="${PROJECT_ROOT}/api/openapi.compat.yaml"

RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

if [ ! -f "${CANONICAL_SPEC}" ]; then
    echo -e "${YELLOW}⚠️  Canonical spec not found at api/openapi.yaml. Skipping compat check.${NC}"
    exit 0
fi

if [ ! -f "${COMPAT_SPEC}" ]; then
    if [ "${REQUIRE_OPENAPI_COMPAT:-0}" = "1" ]; then
        echo -e "${RED}❌ OpenAPI compat spec is required but missing: api/openapi.compat.yaml${NC}"
        echo "Generate a 3.0-compatible artifact from the canonical 3.1 spec."
        exit 1
    fi
    echo -e "${YELLOW}⚠️  OpenAPI compat spec missing: api/openapi.compat.yaml${NC}"
    echo "If OpenAPI 3.1-only features are used, generate the compat spec and re-run."
    exit 0
fi

# Compare by regenerated content instead of file mtimes.
# Git checkouts may assign non-meaningful timestamps and cause false negatives in CI.
TMP_COMPAT="$(mktemp)"
trap 'rm -f "${TMP_COMPAT}"' EXIT
if ! (cd "${PROJECT_ROOT}" && go run ./cmd/openapi-compat-gen/main.go "${CANONICAL_SPEC}" "${TMP_COMPAT}") >/dev/null 2>&1; then
    echo -e "${RED}❌ Failed to generate OpenAPI compat artifact for verification.${NC}"
    echo "Ensure cmd/openapi-compat-gen builds and canonical spec is valid."
    exit 1
fi

if ! cmp -s "${COMPAT_SPEC}" "${TMP_COMPAT}"; then
    echo -e "${RED}❌ OpenAPI compat spec is stale.${NC}"
    echo "Canonical: api/openapi.yaml"
    echo "Compat:    api/openapi.compat.yaml"
    echo "Regenerate the compat spec to match the canonical spec."
    exit 1
fi

echo -e "${GREEN}✅ OpenAPI compat spec is present and up to date.${NC}"
