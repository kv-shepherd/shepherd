#!/bin/bash
# scripts/ci/check_manual_di.sh
# 🚨 严格手动 DI 规范检查
# 
# 目的：确保依赖组装集中在 internal/app/ 目录，禁止分散初始化
# 
# 检查项：
# 1. 禁止在 internal/app/ 外使用结构体字面量初始化 Service/Repository
# 2. 禁止在 init() 函数中初始化依赖
# 3. 禁止 Redis 导入（已移除 Redis 依赖）
#
# 用法：./check_manual_di.sh
# 退出码：0 = 通过，1 = 有违规

set -e

echo "🔍 检查手动 DI 规范..."

ERRORS=0

# ===========================================
# 检查 1: 禁止在 internal/app/ 外直接实例化 Service/Repository
# ===========================================
echo "📋 检查 1: 禁止分散的依赖实例化..."

# 查找在非 app/ 目录且非测试文件中直接实例化 Service/Repository 的代码
VIOLATIONS=$(grep -rn '&service\.\|&repository\.' --include="*.go" internal/ 2>/dev/null | grep -v "internal/app/" | grep -v "_test.go" || true)

if [ -n "$VIOLATIONS" ]; then
    echo "❌ 发现分散的依赖实例化（必须集中在 internal/app/）:"
    echo "$VIOLATIONS"
    ERRORS=$((ERRORS + 1))
else
    echo "✅ 检查 1 通过"
fi

# ===========================================
# 检查 2: 禁止 init() 函数中的依赖初始化
# ===========================================
echo "📋 检查 2: 禁止 init() 依赖初始化..."

INIT_VIOLATIONS=$(grep -rn 'func init()' --include="*.go" internal/ 2>/dev/null | grep -v "_test.go" | grep -v "//.*func init()" || true)

if [ -n "$INIT_VIOLATIONS" ]; then
    echo "⚠️ 发现 init() 函数（请确认不包含依赖初始化）:"
    echo "$INIT_VIOLATIONS"
    echo "提示：如果仅用于注册（如 Ent schema），可以忽略"
    # 这里用警告而非错误，因为 init() 有合法用途（如注册）
else
    echo "✅ 检查 2 通过"
fi

# ===========================================
# 检查 3: 禁止 Redis 导入
# ===========================================
echo "📋 检查 3: 禁止 Redis 导入..."

REDIS_IMPORTS=$(grep -rn 'go-redis\|"github.com/redis' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$REDIS_IMPORTS" ]; then
    echo "❌ 发现 Redis 导入（已移除 Redis 依赖）:"
    echo "$REDIS_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "✅ 检查 3 通过"
fi

# ===========================================
# 检查 4: 禁止 Wire 导入
# ===========================================
echo "📋 检查 4: 禁止 Wire 导入..."

WIRE_IMPORTS=$(grep -rn 'google/wire\|goforj/wire' --include="*.go" internal/ 2>/dev/null || true)

if [ -n "$WIRE_IMPORTS" ]; then
    echo "❌ 发现 Wire 导入（已移除 Wire 依赖）:"
    echo "$WIRE_IMPORTS"
    ERRORS=$((ERRORS + 1))
else
    echo "✅ 检查 4 通过"
fi

# ===========================================
# 结果
# ===========================================
echo ""
if [ $ERRORS -gt 0 ]; then
    echo "❌ 手动 DI 规范检查失败！发现 $ERRORS 个错误"
    exit 1
else
    echo "✅ 手动 DI 规范检查通过"
    exit 0
fi
