#!/bin/bash
# openapi-compat-generate.sh - Generate OpenAPI 3.0-compatible spec from 3.1 canonical
#
# Uses OpenAPI Overlay to downgrade 3.1 -> 3.0 without mutating canonical spec.
# Expects an overlay file at docs/design/ci/api-templates/openapi-overlay-3.0.yaml

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CANONICAL_SPEC="${PROJECT_ROOT}/api/openapi.yaml"
COMPAT_SPEC="${PROJECT_ROOT}/api/openapi.compat.yaml"
OVERLAY_FILE="${PROJECT_ROOT}/docs/design/ci/api-templates/openapi-overlay-3.0.yaml"

OAS_PATCH_BIN="${OAS_PATCH_BIN:-oas-patch}"

if [ ! -f "${CANONICAL_SPEC}" ]; then
    echo "❌ Canonical spec not found at api/openapi.yaml"
    exit 1
fi

if [ ! -f "${OVERLAY_FILE}" ]; then
    echo "❌ Overlay file not found: ${OVERLAY_FILE}"
    exit 1
fi

if ! command -v "${OAS_PATCH_BIN}" >/dev/null 2>&1; then
    echo "❌ oas-patch CLI not found: ${OAS_PATCH_BIN}"
    echo "Set OAS_PATCH_BIN to the installed binary name/path."
    exit 1
fi

echo "==> Generating compat spec: ${COMPAT_SPEC}"
"${OAS_PATCH_BIN}" overlay "${CANONICAL_SPEC}" "${OVERLAY_FILE}" -o "${COMPAT_SPEC}"
echo "✅ Compat spec generated."
