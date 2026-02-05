#!/bin/bash
# openapi-compat-generate.sh - Generate OpenAPI 3.0-compatible spec from 3.1 canonical
#
# Uses OpenAPI Overlay to downgrade 3.1 -> 3.0 without mutating canonical spec.
# Expects an overlay file at docs/design/ci/api-templates/openapi-overlay-3.0.yaml
#
# ADR-0029: Uses libopenapi for overlay processing (replaces oas-patch)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CANONICAL_SPEC="${PROJECT_ROOT}/api/openapi.yaml"
COMPAT_SPEC="${PROJECT_ROOT}/api/openapi.compat.yaml"
OVERLAY_FILE="${PROJECT_ROOT}/docs/design/ci/api-templates/openapi-overlay-3.0.yaml"

# ADR-0029: Use libopenapi overlay support (Go-native, replaces oas-patch)
LIBOPENAPI_OVERLAY_BIN="${LIBOPENAPI_OVERLAY_BIN:-go run github.com/pb33f/libopenapi/cmd/openapi-overlay@latest}"

if [ ! -f "${CANONICAL_SPEC}" ]; then
    echo "❌ Canonical spec not found at api/openapi.yaml"
    exit 1
fi

if [ ! -f "${OVERLAY_FILE}" ]; then
    echo "❌ Overlay file not found: ${OVERLAY_FILE}"
    exit 1
fi

echo "==> Generating compat spec: ${COMPAT_SPEC}"
# ADR-0029: libopenapi overlay command
${LIBOPENAPI_OVERLAY_BIN} \
    -s "${CANONICAL_SPEC}" \
    -o "${OVERLAY_FILE}" \
    -output "${COMPAT_SPEC}"

echo "✅ Compat spec generated."
