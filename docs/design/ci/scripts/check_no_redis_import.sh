#!/bin/bash
# scripts/ci/check_no_redis_import.go
# Redis import prohibition check
#
# 2026-01-18 architecture simplification: Redis dependency removed.
# Session storage uses PostgreSQL + alexedwards/scs.

set -e

echo "Checking Redis imports..."

VIOLATIONS=$(grep -rn 'github.com/redis/go-redis\|"go-redis"' --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor/" || true)

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
