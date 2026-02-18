#!/bin/bash
# openapi-compat-generate.sh - Generate OpenAPI 3.0-compatible spec from 3.1 canonical
#
# Minimal compatibility generator:
# - copies canonical spec to compat path
# - rewrites `openapi: 3.1.x` -> `openapi: 3.0.3`
#
# If 3.1-only keywords are detected, exits with an error and asks for a
# real overlay-based conversion tool before enabling REQUIRE_OPENAPI_COMPAT=1.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

CANONICAL_SPEC="${PROJECT_ROOT}/api/openapi.yaml"
COMPAT_SPEC="${PROJECT_ROOT}/api/openapi.compat.yaml"
if [ ! -f "${CANONICAL_SPEC}" ]; then
    echo "❌ Canonical spec not found at api/openapi.yaml"
    exit 1
fi

# Detect common OpenAPI 3.1 / JSON Schema 2020-12 keywords that 3.0 tooling
# cannot safely consume with a version-string rewrite alone.
if grep -Eq '^[[:space:]]*(jsonSchemaDialect|unevaluatedProperties|dependentSchemas|prefixItems|minContains|maxContains|contentEncoding|contentMediaType):' "${CANONICAL_SPEC}"; then
    echo "❌ Detected 3.1-only keywords in api/openapi.yaml."
    echo "   A simple 3.0.3 rewrite is unsafe; use a real overlay transform first."
    exit 1
fi

echo "==> Generating compat spec: ${COMPAT_SPEC}"
cp "${CANONICAL_SPEC}" "${COMPAT_SPEC}"

# Normalize OpenAPI version for 3.0-only toolchain.
if ! sed -i -E 's/^openapi:[[:space:]]*3\.1(\.[0-9]+)?$/openapi: 3.0.3/' "${COMPAT_SPEC}"; then
    echo "❌ Failed to rewrite OpenAPI version in compat spec."
    exit 1
fi

echo "✅ Compat spec generated."
