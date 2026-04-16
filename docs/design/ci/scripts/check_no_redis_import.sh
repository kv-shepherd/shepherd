#!/bin/bash
# docs/design/ci/scripts/check_no_redis_import.sh
# Redis import prohibition check
#
# 2026-01-18 architecture simplification: Redis dependency removed.
# Session storage uses PostgreSQL + alexedwards/scs.

set -euo pipefail

echo "Checking Redis imports..."

if ! command -v rg >/dev/null 2>&1; then
    echo "ERROR: ripgrep (rg) is required for Redis import detection."
    exit 127
fi

VIOLATIONS=$(rg -n 'github\.com/redis/go-redis|"go-redis"' \
    cmd internal pkg ent plugins \
    --glob '!**/*_test.go' \
    --glob '!**/vendor/**' || true)

if [ -n "$VIOLATIONS" ]; then
    echo "ERROR: found Redis imports (Redis dependency has been removed):"
    echo "$VIOLATIONS"
    echo ""
    echo "Suggested fixes:"
    echo "  - For sessions, use github.com/alexedwards/scs/v2 + postgresstore"
    echo "  - For caching needs, query the database directly or use local in-memory cache"
    exit 1
else
    echo "OK: no Redis imports found"
    exit 0
fi
