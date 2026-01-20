#!/bin/bash
# scripts/ci/check_no_redis_import.go
# 🚨 禁止 Redis 导入检查
#
# 2026-01-18 架构精简：移除 Redis 依赖
# Session 存储改用 PostgreSQL + alexedwards/scs

set -e

echo "🔍 检查 Redis 导入..."

VIOLATIONS=$(grep -rn 'github.com/redis/go-redis\|"go-redis"' --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor/" || true)

if [ -n "$VIOLATIONS" ]; then
    echo "❌ 发现 Redis 导入（已移除 Redis 依赖）:"
    echo "$VIOLATIONS"
    echo ""
    echo "💡 解决方案："
    echo "  - Session 存储请使用 github.com/alexedwards/scs/v2 + postgresstore"
    echo "  - 缓存需求请直接查询数据库或使用本地内存缓存"
    exit 1
else
    echo "✅ 未发现 Redis 导入"
    exit 0
fi
