# 规范交互流程 (Master Flow)

> **Status**: Stable (ADR-0017, ADR-0018 Accepted)  
> **版本**: 1.0  
> **日期**: 2026-01-28  
> **语言**: 中文 (翻译版本)  
> **规范版本**: [English Canonical Version](../../../../design/interaction-flows/master-flow.md)
>
> 🌐 **Other Languages**: [English (Canonical)](../../../../design/interaction-flows/master-flow.md)

---

## 文档说明

本文档是 Shepherd 平台所有交互流程的**中文翻译版本**，是前后端和数据库开发的**参考文档**。

> **注意**: 英文版本 (`docs/design/interaction-flows/master-flow.md`) 是规范版本 (Canonical Version)。
> 如有不一致，以英文版本为准。

## 文档范围

| 包含内容 | 不包含内容 |
|----------|------------|
| 用户交互流程 | 数据库 DDL/Schema 定义 |
| 数据流向与来源 | 详细 API 规范 |
| 概念状态图 | 实现代码示例 |
| 业务规则概述 | 底层技术约束 |

> **交叉引用模式**: 涉及数据持久化的操作在本文档提供概念概述，实现细节详见 Phase 设计文档。
>
> 示例: "所有操作都会创建审计日志。详见 [04-governance.md §7](../phases/04-governance.md#7-audit-logging) 了解 Schema 详情。"

**相关文档**:
- [ADR-0018: Instance Size Abstraction](../../../../adr/ADR-0018-instance-size-abstraction.md)
- [ADR-0015: Governance Model V2](../../../../adr/ADR-0015-governance-model-v2.md)
- [ADR-0017: VM Request Flow](../../../../adr/ADR-0017-vm-request-flow-clarification.md)
- [Phase 01: 契约](../phases/01-contracts.md) — 数据契约和命名约束
- [Phase 04: 治理](../phases/04-governance.md) — RBAC、审计日志、审批流程

---

## 附录：规范交互流程（中文版）

> **重要**: 本节为 `docs/design/interaction-flows/master-flow.md` 的中文翻译版本。
> 如有不一致，以英文规范版本为准。
### 文档结构

| Part | 内容 | 涉及角色 |
|------|------|----------|
| **Part 1** | 平台初始化（Schema/Mask、**首次部署引导**、RBAC/权限、OIDC/LDAP 认证、IdP 组映射、**外部审批系统**、集群/InstanceSize/Template 配置） | 开发者、平台管理员 |
| **Part 2** | 资源管理（System/Service 创建删除及数据库操作、**含审计日志**） | 普通用户 |
| **Part 3** | VM 生命周期（创建请求 → 审批 → 执行 → 删除及数据库操作、**含审计日志**） | 普通用户、平台管理员 |
| **Part 4** | 状态机与数据模型（状态流转图、表关系图、**审计日志设计与例外**） | 所有开发人员 |

---

### 核心设计原则

| 原则 | 说明 |
|------|------|
| **Schema 为唯一数据源** | KubeVirt 官方 JSON Schema 定义字段类型、约束、enum 选项，我们不在代码中重复定义 |
| **Mask 只选择路径** | Mask 只指定暴露哪些 Schema 路径，不定义字段选项 |
| **混合模型 (Hybrid Model)** | 核心调度字段（CPU、内存、GPU）存储在索引列以优化查询性能；`spec_overrides` JSONB 存储剩余字段，不进行语义解释。参见 ADR-0018 §4。 |
| **Schema 驱动前端** | 前端根据 Schema 类型自动渲染对应 UI 组件。技术栈详见 ADR-0020（React 19, Next.js 15, Ant Design 5）。 |

### 角色定义

| 角色 | 职责 | 接触层级 |
|------|------|---------|
| **开发者** | 获取 KubeVirt Schema，定义 Mask（选择暴露哪些路径） | 代码/配置层 |
| **平台管理员** | 创建 InstanceSize（通过 Schema 驱动的表单填写值） | 后台管理层 |
| **普通用户** | 选择 InstanceSize，提交 VM 创建请求 | 业务使用层 |

### 命名规范 (ADR-0019 安全基线)

> **安全基线**: 所有平台管理的逻辑名称必须遵循 RFC 1035 规则。

| 规则 | 约束 |
|------|------|
| **字符集** | 仅小写字母、数字、连字符 (`a-z`, `0-9`, `-`) |
| **起始字符** | 必须以字母开头 (`a-z`) |
| **结束字符** | 必须以字母或数字结尾 |
| **连续连字符** | 禁止 `--` (为 Punycode 保留) |
| **长度限制** | System/Service/Namespace: 每个最多 15 字符 (ADR-0015 §16) |

**适用范围**: System 名称、Service 名称、Namespace 名称、VM 名称组件。

### API 设计原则 (ADR-0021, ADR-0023)

| 原则 | 说明 |
|------|------|
| **契约优先** | OpenAPI 3.1 规范为唯一真理来源。参见 ADR-0021。 |
| **代码生成** | Go 服务端类型通过 `oapi-codegen` 生成；TypeScript 类型通过 `openapi-typescript` 生成。 |
| **分页标准** | 列表 API 使用标准分页参数 (`page`, `per_page`, `sort_by`, `sort_order`)。参见 ADR-0023。 |
| **错误码** | 粒度化错误码（如 `NAMESPACE_PERMISSION_DENIED`）。参见 ADR-0023 §3。 |

### Schema 缓存生命周期 (ADR-0023)

> **目的**: KubeVirt Schema 缓存支持离线验证、多版本兼容和前端性能优化。

| 阶段 | 触发时机 | 操作 |
|------|----------|------|
| **1. 启动** | 应用启动 | 加载编译时嵌入的 Schema |
| **2. 集群注册** | 新增集群 | 检测 KubeVirt 版本 → 检查缓存 → 缺失时排队获取 |
| **3. 版本检测** | 健康检查循环 (60s) | 搭载模式: 比较 `clusters.kubevirt_version` 与检测到的版本 |
| **4. Schema 更新** | 检测到版本变更 | 排队 `SchemaUpdateJob` (River) → 异步获取 → 更新缓存 |

**过期策略**: Schema **按版本不可变**（v1.5.0 永不改变）。无限期缓存，仅在版本变更时更新。

**优雅降级**: Schema 获取失败 → 使用嵌入的回退版本 → 下次健康检查时重试。

详见 ADR-0023 §1 完整缓存生命周期图。

---

## Part 1: 平台初始化流程

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 1: 平台初始化 (开发者操作)                                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  开发者:                                                                                      │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. 获取 KubeVirt 官方 JSON Schema                                                       │ │
│  │    - 来源: KubeVirt CRD OpenAPI Schema 或官方文档                                        │ │
│  │    - 包含: 所有字段类型、约束、enum 选项                                                  │ │
│  │                                                                                          │ │
│  │ 2. 定义 Mask 配置 (只选择路径，不定义选项)                                                │ │
│  │                                                                                          │ │
│  │    mask:                                                                                 │ │
│  │      quick_fields:                                                                       │ │
│  │        - path: "spec.template.spec.domain.cpu.cores"                                     │ │
│  │          display_name: "CPU 核数"                                                        │ │
│  │      advanced_fields:                                                                    │ │
│  │        - path: "spec.template.spec.domain.devices.gpus"                                  │ │
│  │          display_name: "GPU 设备"                                                        │ │
│  │        - path: "spec.template.spec.domain.memory.hugepages.pageSize"                     │ │
│  │          display_name: "Hugepages 大小"                                                  │ │
│  │                                                                                          │ │
│  │    👉 Mask 只引用 Schema 路径，字段类型和选项由 Schema 定义                               │ │
│  │                                                                                          │ │
│  │ 3. 前端根据 Schema + Mask 自动渲染 UI                                                    │ │
│  │    - integer → 数字输入框                                                                │ │
│  │    - string → 文本输入框                                                                 │ │
│  │    - boolean → 复选框                                                                    │ │
│  │    - enum → 下拉框 (选项来自 Schema，不是开发者定义)                                       │ │
│  │    - array → 动态添加/删除表格                                                            │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 阶段 1.5: 首次部署引导 (Bootstrap) {#stage-1-5}

> **Added 2026-01-26**: 配置存储策略的首次部署流程

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 1.5: 首次部署引导                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  🔧 部署配置 (二选一):                                                                         │
│                                                                                              │
│  📁 方式 A: config.yaml (本地开发 / 传统部署)                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  # config.yaml                                                                          │ │
│  │  database:                                                                              │ │
│  │    url: "postgresql://user:pass@localhost:5432/shepherd"                                │ │
│  │                                                                                          │ │
│  │  server:                                                                                 │ │
│  │    port: 8080                                                                            │ │
│  │    log_level: "info"                     # 可选，默认 info                                │ │
│  │                                                                                          │ │
│  │  worker:                                                                                 │ │
│  │    max_workers: 10                       # 可选，默认 10                                  │ │
│  │                                                                                          │ │
│  │  security:                                                                               │ │
│  │    encryption_key: "32-byte-random"      # 可选，强烈建议                                │ │
│  │    session_secret: "32-byte-random"      # 可选，强烈建议                                │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  🐳 方式 B: 环境变量 (容器化部署)                                                               │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  DATABASE_URL=postgresql://user:pass@host:5432/shepherd    # 必需                       │ │
│  │  SERVER_PORT=8080                        # 可选，默认 8080                               │ │
│  │  LOG_LEVEL=info                          # 可选，默认 info                                │ │
│  │  RIVER_MAX_WORKERS=10                    # 可选，默认 10                                  │ │
│  │  ENCRYPTION_KEY=<32-byte-random>         # 可选，强烈建议                                │ │
│  │  SESSION_SECRET=<32-byte-random>         # 可选，强烈建议                                │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ⚡ 优先级: 环境变量 > config.yaml > 默认值                                                    │
│  💡 环境变量始终覆盖 config.yaml (12-factor app 原则)                                          │
│                                                                                              │
│  🔐 自动生成 (ADR-0025 - 缺省时):                                                            │
│  - 首次启动若缺少 ENCRYPTION_KEY / SESSION_SECRET，自动生成 32 字节强随机密钥（CSPRNG）        │
│  - 持久化存入 PostgreSQL `system_secrets` 表 (禁止仅内存临时密钥)                             │
│  - ⚡ V1 优先级: 环境变量 > 数据库生成 (ADR-0025)                                             │
│  - 🔮 未来优先级: KMS/外部密钥管理器 > 环境变量 > 数据库生成 (RFC-0017, 不在 V1 范围)          │
│  - 后续引入外部密钥时，需执行显式重加密步骤                                                   │
│  - 🔄 密钥轮换推迟到 RFC-0016 (不在 V1 范围内)                                                │
│                                                                                              │
│  📦 应用自动初始化:                                                                          │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  1. 运行 migrations                                                                    │ │
│  │  2. Seed 内置角色 (ON CONFLICT DO NOTHING - 不覆盖已有)                                 │ │
│  │  3. Seed 默认管理员 admin/admin (force_password_change=true)                           │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│
│                                                                                              │
│  🖥️ 首次登录提示:                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                                      │   │
│  │                    ⚠️ 首次登录                                                       │   │
│  │                                                                                      │   │
│  │    请使用默认管理员账户登录:                                                          │   │
│  │                                                                                      │   │
│  │    用户名: admin                                                                     │   │
│  │    密码:   admin                                                                     │   │
│  │                                                                                      │   │
│  │    ⚠️ 登录后请立即修改密码!                                                          │   │
│  │                                                                                      │   │
│  │    [登录]                                                                            │   │
│  │                                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  🔐 强制修改密码:                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                                      │   │
│  │                    🔐 请设置新密码                                                   │   │
│  │                                                                                      │   │
│  │    您正在使用默认密码，请立即修改以保证账户安全。                                      │   │
│  │                                                                                      │   │
│  │    新密码:     [••••••••••••                ]                                        │   │
│  │    确认密码:   [••••••••••••                ]                                        │   │
│  │                                                                                      │   │
│  │    密码要求 (NIST 800-63B):                                                          │   │
│  │    ✓ 最少 8 个字符（建议 15+ 字符）                                                   │   │
│  │    ✓ 不在常见密码黑名单中                                                             │   │
│  │    ○ 复杂度规则默认不强制（可配置以满足合规要求）                                       │   │
│  │                                                                                      │   │
│  │    [确认修改]                                                                        │   │
│  │                                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Seed 默认管理员 (首次启动)                                                      │       │
│  │  INSERT INTO users (id, username, password_hash, auth_type, force_password_change) │       │
│  │  VALUES ('admin', 'admin', bcrypt('admin'), 'local', true)                         │       │
│  │  ON CONFLICT (username) DO NOTHING;                                                 │       │
│  │                                                                                    │       │
│  │  -- 关联 PlatformAdmin 角色                                                         │       │
│  │  INSERT INTO role_bindings (id, user_id, role_id, scope_type, source)              │       │
│  │  VALUES ('rb-admin', 'admin', 'role-platform-admin', 'global', 'seed')             │       │
│  │  ON CONFLICT DO NOTHING;                                                            │       │
│  │                                                                                    │       │
│  │  -- 修改密码后                                                                       │       │
│  │  UPDATE users SET                                                                   │       │
│  │    password_hash = bcrypt('new_password'),                                          │       │
│  │    force_password_change = false,                                                   │       │
│  │    updated_at = NOW()                                                               │       │
│  │  WHERE id = 'admin';                                                                │       │
│  │                                                                                    │       │
│  │  -- 审计日志                                                                         │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)    │       │
│  │  VALUES ('user.password_change', 'admin', 'user', 'admin',                         │       │
│  │          '{"reason": "first_login_forced"}');                                       │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ 完成后进入管理后台，继续阶段 2                                                            │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 阶段 2: 平台安全配置 (首次部署) {#stage-2}

> **参考**: ADR-0015 §22 (Authentication & RBAC Strategy)

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.A: 内置角色与权限初始化                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  🔧 系统自动执行 (Seed Data):                                                                 │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 1. 内置权限 (Permissions)                                                      │       │
│  │  INSERT INTO permissions (id, resource, action, name) VALUES                      │       │
│  │    ('system:read', 'system', 'read', '查看系统'),                                  │       │
│  │    ('system:write', 'system', 'write', '修改系统'),                                │       │
│  │    ('system:delete', 'system', 'delete', '删除系统'),                              │       │
│  │    ('service:read', 'service', 'read', '查看服务'),                                │       │
│  │    ('service:create', 'service', 'create', '创建服务'),                            │       │
│  │    ('service:delete', 'service', 'delete', '删除服务'),                            │       │
│  │    ('vm:read', 'vm', 'read', '查看VM'),                                           │       │
│  │    ('vm:create', 'vm', 'create', '创建VM请求'),                                   │       │
│  │    ('vm:operate', 'vm', 'operate', 'VM操作(启停)'),                                │       │
│  │    ('vm:delete', 'vm', 'delete', '删除VM'),                                       │       │
│  │    ('vnc:access', 'vnc', 'access', 'VNC控制台'),                                   │       │
│  │    ('approval:approve', 'approval', 'approve', '审批请求'),                        │       │
│  │    ('approval:view', 'approval', 'view', '查看待审批'),                             │       │
│  │    ('cluster:manage', 'cluster', 'manage', '管理集群'),                            │       │
│  │    ('template:manage', 'template', 'manage', '管理模板'),                          │       │
│  │    ('rbac:manage', 'rbac', 'manage', '管理权限'),                                  │       │
│  │    ('platform:admin', 'platform', 'admin', '超级管理员权限（显式）'),               │       │
│  │    -- ⚠️ 已废弃: *:* 通配符仅限 bootstrap 角色使用 (ADR-0019)                       │       │
│  │    ('*:*', '*', '*', 'Bootstrap专用通配符 - 初始化后必须禁用');                      │       │
│  │                                                                                    │       │
│  │  -- 2. 内置角色 (ADR-0019 合规)                                                    │       │
│  │  INSERT INTO roles (id, name, is_builtin, description) VALUES                     │       │
│  │    ('role-bootstrap', 'Bootstrap', true, '初始化专用 - 部署后必须禁用'),            │       │
│  │    ('role-platform-admin', 'PlatformAdmin', true, '平台管理员'),                   │       │
│  │    ('role-system-admin', 'SystemAdmin', true, '系统管理员'),                        │       │
│  │    ('role-approver', 'Approver', true, '审批员'),                                  │       │
│  │    ('role-operator', 'Operator', true, '运维人员'),                                 │       │
│  │    ('role-viewer', 'Viewer', true, '只读用户');                                    │       │
│  │                                                                                    │       │
│  │  -- 3. 角色-权限关联 (ADR-0019: 仅 bootstrap 可使用通配符)                           │       │
│  │  INSERT INTO role_permissions (role_id, permission_id) VALUES                     │       │
│  │    -- Bootstrap 角色: 通配符 (平台初始化后必须禁用)                                  │       │
│  │    ('role-bootstrap', '*:*'),                                                      │       │
│  │    -- PlatformAdmin: 显式权限列表 (ADR-0019 禁止通配符)                             │       │
│  │    ('role-platform-admin', 'system:read'), ('role-platform-admin', 'system:write'),│       │
│  │    ('role-platform-admin', 'system:delete'), ('role-platform-admin', 'service:read'),│     │
│  │    ('role-platform-admin', 'service:create'), ('role-platform-admin', 'service:delete'),│  │
│  │    ('role-platform-admin', 'vm:read'), ('role-platform-admin', 'vm:create'),       │       │
│  │    ('role-platform-admin', 'vm:operate'), ('role-platform-admin', 'vm:delete'),    │       │
│  │    ('role-platform-admin', 'vnc:access'), ('role-platform-admin', 'approval:approve'),│    │
│  │    ('role-platform-admin', 'approval:view'), ('role-platform-admin', 'cluster:manage'),│   │
│  │    ('role-platform-admin', 'template:manage'), ('role-platform-admin', 'rbac:manage'),│    │
│  │    -- Approver: 显式权限 (ADR-0019 禁止通配符)                                       │       │
│  │    ('role-approver', 'approval:approve'), ('role-approver', 'approval:view'),      │       │
│  │    ('role-approver', 'vm:read'), ('role-approver', 'system:read'),                 │       │
│  │    ('role-approver', 'service:read'),                                              │       │
│  │    -- SystemAdmin, Operator, Viewer: 显式权限                                       │       │
│  │    ('role-system-admin', 'system:read'), ('role-system-admin', 'system:write'),    │       │
│  │    ('role-system-admin', 'system:delete'), ('role-system-admin', 'service:read'),  │       │
│  │    ('role-system-admin', 'service:create'), ('role-system-admin', 'service:delete'),│      │
│  │    ('role-system-admin', 'vm:read'), ('role-system-admin', 'vm:create'),           │       │
│  │    ('role-system-admin', 'vm:operate'), ('role-system-admin', 'vm:delete'),        │       │
│  │    ('role-system-admin', 'vnc:access'), ('role-system-admin', 'rbac:manage'),      │       │
│  │    ('role-operator', 'system:read'), ('role-operator', 'service:read'),            │       │
│  │    ('role-operator', 'vm:read'), ('role-operator', 'vm:create'),                   │       │
│  │    ('role-operator', 'vm:operate'), ('role-operator', 'vnc:access'),               │       │
│  │    ('role-viewer', 'system:read'), ('role-viewer', 'service:read'),                │       │
│  │    ('role-viewer', 'vm:read');                                                     │       │
│  │                                                                                    │       │
│  │  -- ⚠️ ADR-0019 安全 SOP:                                                           │       │
│  │  -- 平台初始化完成后，必须禁用 bootstrap 角色:                                        │       │
│  │  --   DELETE FROM role_bindings WHERE role_id = 'role-bootstrap';                  │       │
│  │  -- 详见 docs/operations/bootstrap-role-sop.md                                      │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.A+: 自定义角色管理 (可选)                                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员操作 (在 OIDC 配置之前或之后均可):                                                    │
│                                                                                              │
│  ┌─ Step 1: 创建自定义角色 ─────────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  角色管理                                                                               │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  角色列表:                                                                        │   │   │
│  │  │  ──────────────────────────────────────────────────────────────────────────    │   │   │
│  │  │  [🔒] PlatformAdmin          内置    平台管理员-全部权限                          │   │   │
│  │  │  [🔒] SystemAdmin            内置    系统管理员                                   │   │   │
│  │  │  [🔒] Approver               内置    审批员                                       │   │   │
│  │  │  [🔒] Operator               内置    运维人员                                     │   │   │
│  │  │  [🔒] Viewer                 内置    只读用户                                     │   │   │
│  │  │  [  ] DevLead                自定义   开发主管 (可编辑/删除)                       │   │   │
│  │  │  [  ] QA-Manager             自定义   QA 管理员 (可编辑/删除)                      │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [+ 创建自定义角色]                                                               │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: 配置自定义角色权限 ─────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  创建自定义角色                                                                         │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  角色名称:     [DevLead              ]                                           │   │   │
│  │  │  角色描述:     [开发主管-可管理系统和服务]                                          │   │   │
│  │  │                                                                                  │   │   │
│  │  │  选择权限 (全局权限):                                                              │   │   │
│  │  │  ┌─ 系统管理 ─────────────────┐  ┌─ 审批管理 ─────────────────┐                   │   │   │
│  │  │  │ ☑ system:read              │  │ ☐ approval:approve        │                   │   │   │
│  │  │  │ ☑ system:write             │  │ ☐ approval:view           │                   │   │   │
│  │  │  │ ☐ system:delete            │  └────────────────────────────┘                   │   │   │
│  │  │  └────────────────────────────┘                                                    │   │   │
│  │  │  ┌─ 服务管理 ─────────────────┐  ┌─ 平台管理 ─────────────────┐                   │   │   │
│  │  │  │ ☑ service:read             │  │ ☐ cluster:manage          │                   │   │   │
│  │  │  │ ☑ service:create           │  │ ☐ template:manage         │                   │   │   │
│  │  │  │ ☐ service:delete           │  │ ☐ rbac:manage             │                   │   │   │
│  │  │  └────────────────────────────┘  └────────────────────────────┘                   │   │   │
│  │  │  ┌─ VM 管理 ──────────────────┐                                                    │   │   │
│  │  │  │ ☑ vm:read                  │                                                    │   │   │
│  │  │  │ ☑ vm:create                │                                                    │   │   │
│  │  │  │ ☑ vm:operate               │                                                    │   │   │
│  │  │  │ ☐ vm:delete                │                                                    │   │   │
│  │  │  │ ☑ vnc:access               │                                                    │   │   │
│  │  │  └────────────────────────────┘                                                    │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [保存角色]                                                                        │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 创建自定义角色                                                                  │       │
│  │  INSERT INTO roles (id, name, is_builtin, description) VALUES                     │       │
│  │    ('role-dev-lead', 'DevLead', false, '开发主管-可管理系统和服务');                 │       │
│  │                                                                                    │       │
│  │  -- 关联权限                                                                        │       │
│  │  INSERT INTO role_permissions (role_id, permission_id) VALUES                     │       │
│  │    ('role-dev-lead', 'system:read'), ('role-dev-lead', 'system:write'),           │       │
│  │    ('role-dev-lead', 'service:read'), ('role-dev-lead', 'service:create'),        │       │
│  │    ('role-dev-lead', 'vm:read'), ('role-dev-lead', 'vm:create'),                  │       │
│  │    ('role-dev-lead', 'vm:operate'), ('role-dev-lead', 'vnc:access');              │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 自定义角色创建后，可在 OIDC 组映射 (阶段 2.C) 中选择使用                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼

> **标准化 Provider 输出**：所有认证提供方（OIDC/LDAP/SSO）通过适配层统一成标准输出，用于 RBAC 映射。见 ADR-0026。

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.B: 配置认证方式 (OIDC/LDAP)                                      │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员操作:                                                                               │
│                                                                                              │
│  ┌─ Step 1: 选择认证方式 ─────────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  配置身份认证                                                                           │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  认证方式:                                                                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ◉ OIDC (推荐)   - 适用于: Azure AD, Okta, Keycloak, Google Workspace           │   │   │
│  │  │  ○ LDAP          - 适用于: Active Directory, OpenLDAP                           │   │   │
│  │  │  ○ 内置用户       - 仅用于测试环境                                                │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [下一步 →]                                                                      │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: OIDC 配置 ────────────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  OIDC Provider 配置                                                                    │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Provider 名称:  [Corp-SSO                    ]                                  │   │   │
│  │  │  Issuer URL:     [https://sso.company.com/realms/main]                           │   │   │
│  │  │  Client ID:      [shepherd-platform           ]                                  │   │   │
│  │  │  Client Secret:  [••••••••••••                ] 👁                               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Callback URL (复制到 IdP):                                                       │   │   │
│  │  │  📋 https://shepherd.company.com/api/v1/auth/oidc/callback                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [测试连接]  [保存配置]                                                           │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  INSERT INTO auth_providers (id, type, name, enabled, issuer, client_id,           │       │
│  │    client_secret_encrypted, scopes, claims_mapping, default_role_id,               │       │
│  │    default_allowed_environments) VALUES                                            │       │
│  │  ('idp-001', 'oidc', 'Corp-SSO', true, 'https://sso.company.com/realms/main',       │       │
│  │   'shepherd-platform', 'encrypted:xxx', ARRAY['openid','profile','email'],         │       │
│  │   '{"groups":"groups","groups_format":"array"}', 'role-viewer', ARRAY['test']);    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.C: IdP 组映射配置                                               │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员操作:                                                                               │
│                                                                                              │
│  ┌─ Step 1: 获取样本用户数据 ─────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  API: GET /api/v1/admin/idp/{id}/sample                                               │   │
│  │  系统从 IdP 拉取 10 个用户的 Token 数据，提取可用字段:                                    │   │
│  │                                                                                        │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  检测到的字段:                                                                    │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ◉ groups (array, 5 个唯一值)                                                    │   │   │
│  │  │     样本: ["DevOps-Team", "QA-Team", "Platform-Admin", ...]                      │   │   │
│  │  │  ○ department (string, 3 个唯一值)                                                │   │   │
│  │  │     样本: ["Engineering", "IT", "QA"]                                             │   │   │
│  │  │  ○ custom_roles (array, 2 个唯一值)                                               │   │   │
│  │  │     样本: ["admin", "developer"]                                                  │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [同步选中字段 →]                                                                 │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: 配置组-角色映射 ──────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  IdP Group → Shepherd Role 映射                                                        │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  IdP 组              Shepherd 角色      可访问环境                               │   │   │
│  │  │  ──────────────────────────────────────────────────────────────────────────    │   │   │
│  │  │  Platform-Admin     [PlatformAdmin ▼]   ☑ test  ☑ prod                         │   │   │
│  │  │  DevOps-Team        [SystemAdmin ▼]     ☑ test  ☑ prod                         │   │   │
│  │  │  QA-Team            [Operator ▼]        ☑ test  ☐ prod                         │   │   │
│  │  │  IT-Support         [Viewer ▼]          ☑ test  ☐ prod                         │   │   │
│  │  │  HR-Department      [无映射 ▼]          -                                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  💡 未映射的组默认获得: Viewer 权限 + 仅 test 环境                                 │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [保存映射]                                                                       │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 同步 IdP 组                                                                    │       │
│  │  INSERT INTO idp_synced_groups (id, auth_provider_id, group_id, source_field)     │       │
│  │  VALUES ('sg-001', 'idp-001', 'Platform-Admin', 'groups'),                        │       │
│  │         ('sg-002', 'idp-001', 'DevOps-Team', 'groups'),                           │       │
│  │         ('sg-003', 'idp-001', 'QA-Team', 'groups');                               │       │
│  │                                                                                    │       │
│  │  -- 保存映射关系                                                                    │       │
│  │  INSERT INTO idp_group_mappings (id, auth_provider_id, idp_group_id, role_id,     │       │
│  │                                  scope_type, allowed_environments) VALUES         │       │
│  │    ('map-001', 'idp-001', 'Platform-Admin', 'role-platform-admin',                │       │
│  │     'global', ARRAY['test', 'prod']),                                             │       │
│  │    ('map-002', 'idp-001', 'DevOps-Team', 'role-system-admin',                     │       │
│  │     'global', ARRAY['test', 'prod']),                                             │       │
│  │    ('map-003', 'idp-001', 'QA-Team', 'role-operator',                             │       │
│  │     'global', ARRAY['test']);                                                     │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.D: 用户登录流程                                                 │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  用户首次登录:                                                                                │
│                                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  1. 用户访问 https://shepherd.company.com                                              │ │
│  │                                                                                        │ │
│  │  2. 重定向到 IdP 登录页面                                                               │ │
│  │     → https://sso.company.com/realms/main/protocol/openid-connect/auth?                │ │
│  │       client_id=shepherd-platform&redirect_uri=...                                     │ │
│  │                                                                                        │ │
│  │  3. 用户在 IdP 完成认证                                                                 │ │
│  │                                                                                        │ │
│  │  4. IdP 回调 Shepherd                                                                  │ │
│  │     ← https://shepherd.company.com/api/v1/auth/oidc/callback?code=xxx                  │ │
│  │                                                                                        │ │
│  │  5. Shepherd 处理:                                                                      │ │
│  │     a. 验证 Token (签名、issuer、audience)                                             │ │
│  │     b. 提取用户信息 (sub, email, name, groups)                                         │ │
│  │     c. 根据 groups 查找 idp_group_mappings                                             │ │
│  │     d. 创建/更新用户记录                                                                │ │
│  │     e. 创建 RoleBindings (基于映射)                                                     │ │
│  │     f. 返回 JWT Session Token                                                          │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  📦 数据库操作 (用户首次登录):                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 创建用户记录 (如果不存在)                                                     │       │
│  │  INSERT INTO users (id, external_id, email, name, auth_provider_id, created_at)   │       │
│  │  VALUES ('user-001', 'oidc|abc123', 'zhang.san@company.com', '张三',               │       │
│  │          'idp-001', NOW())                                                         │       │
│  │  ON CONFLICT (external_id) DO UPDATE SET last_login_at = NOW();                   │       │
│  │                                                                                    │       │
│  │  -- 2. 删除旧的 IdP 自动分配的 RoleBindings (标记为 auto_assigned)                   │       │
│  │  DELETE FROM role_bindings                                                         │       │
│  │  WHERE user_id = 'user-001' AND source = 'idp_mapping';                           │       │
│  │                                                                                    │       │
│  │  -- 3. 根据用户的 groups 重新创建 RoleBindings                                       │       │
│  │  -- (用户 groups: ['DevOps-Team'] → 映射到 role-system-admin)                       │       │
│  │  INSERT INTO role_bindings (id, user_id, role_id, scope_type,                     │       │
│  │                             allowed_environments, source) VALUES                  │       │
│  │    ('rb-auto-001', 'user-001', 'role-system-admin', 'global',                     │       │
│  │     ARRAY['test', 'prod'], 'idp_mapping');                                        │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### 用户登录方式总结

| 登录方式 | 适用场景 | 权限来源 |
|----------|----------|----------|
| **OIDC** | 生产环境（推荐） | IdP 组 → 映射规则 → RoleBindings |
| **LDAP** | 遗留 AD 环境 | LDAP 组 → 映射规则 → RoleBindings |
| **内置用户** | 开发/测试 | 手动创建用户和 RoleBindings |

#### 双层权限体系总结

| 维度 | 全局 RBAC | 资源级 RBAC |
|------|-----------|-------------|
| **存储表** | `role_bindings` | `resource_role_bindings` |
| **权限范围** | 平台级操作 | 特定资源访问 |
| **角色类型** | PlatformAdmin, SystemAdmin, Approver, Operator, Viewer, 自定义角色 | Owner, Admin, Member, Viewer |
| **授权方式** | 管理员通过 OIDC 组映射或手动分配 | 资源创建者自行添加成员 |
| **典型场景** | "张三可以审批 VM 请求" | "李四可以访问张三的 shop 系统" |
| **可见性控制** | 无（全局权限） | 有（仅成员可见） |
| **继承模型** | N/A | ✅ Service/VM 完全继承 System 权限 |

#### 权限检查逻辑

> **两层权限体系**: Shepherd 采用双层权限设计：
> - **全局 RBAC (role_bindings)**: 控制平台级操作权限（管理集群、模板、审批等）
> - **资源级 RBAC (resource_role_bindings)**: 控制具体资源的访问权限（我的 System 对你不可见）

```
完整权限检查流程:

用户请求访问资源 R (例如: GET /api/v1/systems/sys-001)

┌─ Step 1: 全局权限检查 ────────────────────────────────────────────────────────────┐
│  查询 role_bindings → 聚合 Permissions                                            │
│  - 如果用户有 platform:admin 权限 → 允许访问所有资源（显式超级管理员）               │
│  - 如果用户有对应全局权限 (system:read) → 进入 Step 2                               │
│  - 否则 → 拒绝访问                                                                 │
└───────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─ Step 2: 资源级权限检查 ──────────────────────────────────────────────────────────┐
│  查询 resource_role_bindings WHERE resource_id = 'sys-001' AND user_id = ?        │
│  - 如果找到记录 (owner/admin/member/viewer) → 根据角色决定操作权限                  │
│  - 如果未找到 → 检查资源继承链 (VM → Service → System)                              │
│  - 最终未找到 → 拒绝访问 (资源对此用户不可见)                                        │
└───────────────────────────────────────────────────────────────────────────────────┘

示例 1: 张三 (DevOps-Team) 访问自己创建的 System
1. 全局权限: system:read ∈ SystemAdmin 权限 → 继续
2. 资源权限: resource_role_bindings 中 role='owner' → ✅ 允许

示例 2: 李四 (IT-Support) 访问张三的 System
1. 全局权限: system:read ∈ Viewer 权限 → 继续
2. 资源权限: 未找到 resource_role_binding 记录 → ❌ 资源不可见

示例 3: 李四被张三添加为 System 成员后
1. 全局权限: system:read ∈ Viewer 权限 → 继续
2. 资源权限: resource_role_bindings 中 role='member' → ✅ 允许查看

示例 4: 李四访问张三 System 下的 VM (权限继承)
访问目标: vm-001 (属于 svc-redis → 属于 sys-shop)
1. 全局权限: vm:read ∈ Viewer 权限 → 继续
2. 资源权限 (向上遍历):
   a. 检查 vm-001 的 binding → 无 (VM 层不配置成员)
   b. 检查 svc-redis 的 binding → 无 (Service 层不配置成员)
   c. 检查 sys-shop 的 binding → 找到! role='member'
3. 结果: 李四继承 System 的 member 权限 → ✅ 可以查看该 VM
```

### 阶段 2.E: 外部审批系统配置 (可选) {#stage-2-e}

> **Added 2026-01-26**: 外部审批系统集成配置

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 2.E: 外部审批系统配置 (可选)                                       │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员操作:                                                                               │
│                                                                                              │
│  ┌─ Step 1: 添加外部审批系统 ──────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  外部审批系统列表                                                                       │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  名称              类型            状态         操作                            │   │   │
│  │  │  ────────────────────────────────────────────────────────────────────────────  │   │   │
│  │  │  OA-审批           Webhook         ✅ 启用      [编辑] [禁用] [删除]             │   │   │
│  │  │  ServiceNow        ServiceNow      ⚪ 禁用      [编辑] [启用] [删除]             │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [+ 添加审批系统]                                                                 │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: 配置 Webhook 类型 ─────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  添加外部审批系统 - Webhook                                                             │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │                                                                                  │   │   │
│  │  │  名称:         [OA-审批                      ]                                   │   │   │
│  │  │  类型:         ( ) Webhook   (●) ServiceNow   ( ) Jira                           │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ── Webhook 配置 ───────────────────────────────────────────────────────────    │   │   │
│  │  │  Webhook URL:  [https://oa.company.com/api/approval/callback                ]   │   │   │
│  │  │  Secret:       [••••••••••••                                ] 👁               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  自定义 Headers (JSON):                                                          │   │   │
│  │  │  ┌──────────────────────────────────────────────────────────────────────────┐   │   │   │
│  │  │  │  {                                                                        │   │   │   │
│  │  │  │    "X-API-Key": "your-api-key",                                          │   │   │   │
│  │  │  │    "X-Tenant-ID": "company-001"                                          │   │   │   │
│  │  │  │  }                                                                        │   │   │   │
│  │  │  └──────────────────────────────────────────────────────────────────────────┘   │   │   │
│  │  │                                                                                  │   │   │
│  │  │  超时 (秒):    [30             ]                                                │   │   │
│  │  │  重试次数:     [3              ]                                                │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [测试连接]  [保存]                                                              │   │   │
│  │  │                                                                                  │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  INSERT INTO external_approval_systems                                            │       │
│  │    (id, name, type, enabled, webhook_url, webhook_secret, webhook_headers,        │       │
│  │     timeout_seconds, retry_count, created_by, created_at)                         │       │
│  │  VALUES                                                                            │       │
│  │    ('eas-001', 'OA-审批', 'webhook', true,                                         │       │
│  │     'https://oa.company.com/api/approval/callback',                                │       │
│  │     'encrypted:AES256:xxxx',                   -- 加密存储                          │       │
│  │     '{"X-API-Key": "xxx", "X-Tenant-ID": "company-001"}',                          │       │
│  │     30, 3, 'admin', NOW());                                                        │       │
│  │                                                                                    │       │
│  │  -- 审计日志                                                                         │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)   │       │
│  │  VALUES ('external_approval_system.create', 'admin',                               │       │
│  │          'external_approval_system', 'eas-001',                                    │       │
│  │          '{"name": "OA-审批", "type": "webhook", "url": "https://oa.company.com..."}');  │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 敏感数据加密:                                                                             │
│  - webhook_secret 使用 AES-256-GCM 加密存储                                                  │
│  - 解密密钥优先来自外部/环境变量，缺省则使用数据库生成的密钥                                 │
│  - 日志中不记录敏感字段值                                                                      │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

<a id="stage-3"></a>

---

```
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 3: 管理员配置 (Cluster/InstanceSize/Template)                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员:                                                                                  │
│                                                                                              │
│  ┌─ 步骤 1: 注册集群 (系统自动探测能力) ─────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  管理员只需提供:                                                                          │ │
│  │  POST /api/v1/admin/clusters                                                             │ │
│  │  { "name": "cluster-a", "kubeconfig": "...", "environment": "prod" }                     │ │
│  │                                                                                          │ │
│  │  系统自动探测 (参考 ADR-0014)，管理员无需手动配置:                                          │ │
│  │  ┌───────────────────────────────────────────────────────────────────────────────────────┐ │ │
│  │  │  探测项目          探测方式                                     结果示例              │ │ │
│  │  │  ────────────────────────────────────────────────────────────────────────────────────│ │ │
│  │  │  GPU 设备          node.status.capacity (nvidia.com/gpu)        nvidia.com/gpu: 2    │ │ │
│  │  │                    💡 需集群预装 NVIDIA Device Plugin                                 │ │ │
│  │  │                                                                                      │ │ │
│  │  │  Hugepages         node.status.allocatable                      hugepages-2Mi: 4Gi   │ │ │
│  │  │                    (hugepages-2Mi, hugepages-1Gi)               hugepages-1Gi: 2Gi   │ │ │
│  │  │                    💡 可能为空 (未配置 Hugepages 时)                                  │ │ │
│  │  │                                                                                      │ │ │
│  │  │  SR-IOV 网络       kubectl get net-attach-def -A                sriov-net-1          │ │ │
│  │  │                    (NetworkAttachmentDefinition CRD)            sriov-net-2          │ │ │
│  │  │                    💡 需集群预装 Multus CNI + SR-IOV Device Plugin                   │ │ │
│  │  │                                                                                      │ │ │
│  │  │  StorageClass      kubectl get storageclasses                   ceph-rbd, local-path │ │ │
│  │  │                                                                                      │ │ │
│  │  │  KubeVirt 版本     kubevirt.status.observedKubeVirtVersion      v1.2.0               │ │ │
│  │  │                    kubectl get kv -n kubevirt -o jsonpath=                           │ │ │
│  │  │                    '{.items[0].status.observedKubeVirtVersion}'                      │ │ │
│  │  └───────────────────────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                                          │ │
│  │  探测结果自动存储 (管理员可查看，但无需手动输入):                                           │ │
│  │  cluster.detected_capabilities = {                                                       │ │
│  │      "gpu_devices": ["nvidia.com/GA102GL_A10"],                                          │ │
│  │      "hugepages": ["2Mi", "1Gi"],                                                        │ │
│  │      "sriov_networks": ["sriov-net-1"],                                                  │ │
│  │      "storage_classes": ["ceph-rbd", "local-path"],                                      │ │
│  │      "kubevirt_version": "v1.2.0"                                                        │ │
│  │  }                                                                                       │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ┌─ 步骤 2: 配置 Namespace (ADR-0017 合规) ──────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  ⚠️ 核心原则 (ADR-0017):                                                                  │ │
│  │  - Namespace 是**全局逻辑实体**，不绑定到特定集群                                           │ │
│  │  - 实际 K8s namespace 在审批通过的 VM 部署时 JIT (即时) 创建                                │ │
│  │  - **VM 请求提交后 Namespace 不可变**                                                      │ │
│  │                                                                                          │ │
│  │  平台职责边界:                                                                            │ │
│  │  - ✅ 管理逻辑 namespace 注册表（环境标签、所有权）                                          │ │
│  │  - ❌ 不管理: Kubernetes RBAC / ResourceQuota (由 K8s 管理员负责)                        │ │
│  │                                                                                          │ │
│  │  管理员操作（注册逻辑 namespace）:                                                         │ │
│  │  POST /api/v1/admin/namespaces                    👈 非集群绑定                           │ │
│  │  {                                                                                       │ │
│  │      "name": "prod-shop",                                                                │ │
│  │      "environment": "prod",                       👈 决定审批策略和集群匹配                 │ │
│  │      "owner_id": "user-001"                                                              │ │
│  │  }                                                                                       │ │
│  │                                                                                          │ │
│  │  💡 提示: 用户选择 Namespace 时，系统根据 environment 标签确定:                            │ │
│  │     - 审批策略 (test 环境可快速审批，prod 环境需严格审批)                                   │ │
│  │     - 超卖警告 (prod 环境超卖时显示警告)                                                   │ │
│  │     - 集群匹配 (namespace 环境类型必须与集群环境类型匹配: test→test, prod→prod)            │ │
│  │                                                                                          │ │
│  │  💡 JIT Namespace 创建（审批执行阶段）:                                                    │ │
│  │     管理员审批 VM 请求并选择目标集群后:                                                     │ │
│  │     1. 检查目标集群上是否存在 K8s namespace                                                │ │
│  │     2. 如不存在 → 创建带有标准标签的 namespace                                             │ │
│  │     3. 错误处理（K8s API 错误被分类后报告）:                                               │ │
│  │        - 权限不足 → NAMESPACE_PERMISSION_DENIED (403)                                     │ │
│  │        - ResourceQuota 超限 → NAMESPACE_QUOTA_EXCEEDED (403) ¹                            │ │
│  │        - 其他错误 → NAMESPACE_CREATION_FAILED (500)                                       │ │
│  │     详见 ADR-0017 §142-221 完整 JIT 创建流程。                                            │ │
│  │                                                                                          │ │
│  │     ¹ K8s 可能因集群 ResourceQuota 策略拒绝创建 namespace。                                │ │
│  │       平台只报告此错误，不负责管理 K8s 配额。                                              │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ┌─ 步骤 3: 配置 Template (参考 ADR-0015 §5, §17) ───────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  模板定义 VM 的操作系统基础配置:                                                            │ │
│  │  - OS 镜像来源 (DataVolume / PVC 引用)                                                    │ │
│  │  - cloud-init 配置 (管理员可自定义)                                                        │ │
│  │  - 字段可见性控制 (quick_fields / advanced_fields)                                        │ │
│  │                                                                                          │ │
│  │  💡 注意: 硬件能力要求 (GPU/SR-IOV/Hugepages) 已移至 InstanceSize 配置                     │ │
│  │  💡 系统初始化时会预填充常用模板 (从 seed data 导入到 PostgreSQL)                           │ │
│  │                                                                                          │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐   │ │
│  │  │  创建模板                                                                          │   │ │
│  │  │                                                                                    │   │ │
│  │  │  名称:         [centos7-standard    ]                                              │   │ │
│  │  │  分类:         [操作系统 ▼]                                                         │   │ │
│  │  │  状态:         [active ▼]                                                          │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ── 镜像来源 ──────────────────────────────────────────────────────────────────   │   │ │
│  │  │  类型:         (●) containerdisk   ( ) pvc                                         │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌─ containerdisk 模式 ──────────────────────────────────────────────────────┐    │   │ │
│  │  │  │  镜像地址:   [docker.io/kubevirt/centos:7                    ]             │    │   │ │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘    │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌─ pvc 模式 (切换后显示) ─────────────────────────────────────────────────────┐   │   │ │
│  │  │  │  Namespace:  [default           ]                                          │   │   │ │
│  │  │  │  PVC 名称:   [centos7-base-disk ]                                          │   │   │ │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘   │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ── cloud-init 配置 (YAML) ───────────────────────────────────────────────────   │   │ │
│  │  │  ┌────────────────────────────────────────────────────────────────────────────┐   │   │ │
│  │  │  │  #cloud-config                                                             │   │   │ │
│  │  │  │  users:                                                                    │   │   │ │
│  │  │  │    - name: admin                                                           │   │   │ │
│  │  │  │      sudo: ALL=(ALL) NOPASSWD:ALL                                          │   │   │ │
│  │  │  │  chpasswd:                                                                 │   │   │ │
│  │  │  │    expire: true                         👈 首次登录后强制修改密码            │   │   │ │
│  │  │  │    users:                                                                  │   │   │ │
│  │  │  │      - name: admin                                                         │   │   │ │
│  │  │  │        password: changeme123            👈 一次性初始密码                    │   │   │ │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘   │   │ │
│  │  │                                                                                    │   │ │
│  │  │  💡 平台职责: 提供一次性密码确保首次登录                                            │   │ │
│  │  │  💡 后续管理: 由用户/管理员/堡垒机负责 (可通过自定义 cloud-init 对接)               │   │ │
│  │  │                                                                                    │   │ │
│  │  │  [保存]                                                                            │   │ │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘   │ │
│  │                                                                                          │ │
│  │  模板版本说明 (ADR-0015 §17):                                                            │ │
│  │  - 用户提交请求时看到当前活跃版本                                                          │ │
│  │  - 管理员审批时可选择不同版本                                                              │ │
│  │  - 最终模板内容快照到 ApprovalTicket，VM 创建后不受模板更新影响                            │ │
│  │                                                                                          │ │
│  │  👉 普通用户: 选择模板，但不能修改 cloud-init 内容                                         │ │
│  │  👉 管理员: 可创建/编辑模板，包括镜像来源和 cloud-init 配置                                │ │
│  │             (如需对接堡垒机，管理员可自定义 cloud-init 配置)                               │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ┌─ 步骤 4: 创建 InstanceSize (通过 Schema 驱动的表单) ──────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  管理员看到的 UI (前端根据 Schema 自动渲染):                                               │ │
│  │                                                                                          │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐   │ │
│  │  │  创建InstanceSize（规格）                                                                          │   │ │
│  │  │                                                                                    │   │ │
│  │  │  名称:         [gpu-workstation    ]                                              │   │ │
│  │  │  显示名称:     [GPU 工作站 (8核 32GB)]                                             │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ── 资源配置 ──────────────────────────────────────────────────────────────────   │   │ │
│  │  │  CPU 核数:     [8        ]                                                        │   │ │
│  │  │  [✓] 启用 CPU 超卖     👈 勾选后显示 request/limit                                │   │ │
│  │  │      ┌─────────────────────────────────────────────────────────────────────────┐  │   │ │
│  │  │      │  CPU Request: [4    ] 核   CPU Limit: [8    ] 核   (2x 超卖)            │  │   │ │
│  │  │      └─────────────────────────────────────────────────────────────────────────┘  │   │ │
│  │  │                                                                                    │   │ │
│  │  │  内存:         [32Gi     ]                                                        │   │ │
│  │  │  [✓] 启用内存超卖                                                                  │   │ │
│  │  │      ┌─────────────────────────────────────────────────────────────────────────┐  │   │ │
│  │  │      │  Mem Request: [16Gi ] 核   Mem Limit: [32Gi ]   (2x 超卖)               │  │   │ │
│  │  │      └─────────────────────────────────────────────────────────────────────────┘  │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ── 高级设置 ──                                                                    │   │ │
│  │  │  Hugepages:    [无 (None) ▼]   👈 下拉框选项来自 KubeVirt Schema enum + 默认无    │   │ │
│  │  │                [无 (None) ]    ← 默认选项: 不使用 Hugepages                       │   │ │
│  │  │                [2Mi        ]                                                      │   │ │
│  │  │                [1Gi        ]                                                      │   │ │
│  │  │                                                                                    │   │ │
│  │  │  专用 CPU:     [✓]        👈 复选框 (Schema 类型: boolean)                         │   │ │
│  │  │                                                                                    │   │ │
│  │  │  GPU 设备:                 👈 动态表格 (Schema 类型: array)                        │   │ │
│  │  │  ┌──────────────────────────────────────────────────────────────────────────┐    │   │ │
│  │  │  │  名称       设备名称                                                      │    │   │ │
│  │  │  │  [gpu1   ]  [nvidia.com/GA102GL_A10         ]  ← 管理员自己输入           │    │   │ │
│  │  │  │                                                                            │    │   │ │
│  │  │  │  [+ 添加 GPU]                                                              │    │   │ │
│  │  │  └──────────────────────────────────────────────────────────────────────────┘    │   │ │
│  │  │                                                                                    │   │ │
│  │  │  [保存]                                                                            │   │ │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘   │ │
│  │                                                                                          │ │
│  │  存储到 PostgreSQL (后端不理解内容，只存储 JSON):                                          │ │
│  │  {                                                                                       │ │
│  │      "name": "gpu-workstation",                                                          │ │
│  │      "cpu_overcommit": { "enabled": true, "request": "4", "limit": "8" },                │ │
│  │      "mem_overcommit": { "enabled": true, "request": "16Gi", "limit": "32Gi" },          │ │
│  │      "spec_overrides": {                                                                 │ │
│  │          "spec.template.spec.domain.cpu.cores": 8,                                       │ │
│  │          "spec.template.spec.domain.resources.requests.memory": "32Gi",                  │ │
│  │          "spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",                   │ │
│  │          "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,                    │ │
│  │          "spec.template.spec.domain.devices.gpus": [                                     │ │
│  │              {"name": "gpu1", "deviceName": "nvidia.com/GA102GL_A10"}                    │ │
│  │          ]                                                                               │ │
│  │      }                                                                                   │ │
│  │  }                                                                                       │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```


---

## Part 2: 资源管理流程

> **说明**: 用户在创建 VM 之前，必须先创建 System 和 Service 来组织资源。

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 4: 用户创建组织结构                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  顺序: System → Service → VM                                                                │
│                                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  System (系统)                                                                          │ │
│  │    ├── Service (服务)                                                                   │ │
│  │    │     ├── VM 1                                                                       │ │
│  │    │     └── VM 2                                                                       │ │
│  │    └── Service (服务)                                                                   │ │
│  │          └── VM 3                                                                       │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 4.A: 用户创建系统 (System)                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  用户操作:                                                                                    │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  创建系统                                                                          │       │
│  │                                                                                    │       │
│  │  系统名称:     [shop                ]    👈 全局唯一, 最长 15 字符                   │       │
│  │  系统描述:     [电商核心系统          ]    👈 支持 Markdown 格式                       │       │
│  │               [预览] [上传 .md 文件]        ← 或上传已有 Markdown 文件                │       │
│  │                                                                                    │       │
│  │  [创建]                                                                             │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 数据库操作 (单事务):                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 创建系统                                                                    │       │
│  │  INSERT INTO systems (id, name, description, created_by, created_at)              │       │
│  │  VALUES ('sys-001', 'shop', '电商核心系统', 'zhang.san', NOW());                   │       │
│  │                                                                                    │       │
│  │  -- 2. 用户权限自动继承 (资源级权限)                                                │       │
│  │  INSERT INTO resource_role_bindings                                               │       │
│  │    (id, user_id, role, resource_type, resource_id, granted_by, created_at)        │       │
│  │  VALUES ('rrb-001', 'zhang.san', 'owner', 'system', 'sys-001', 'zhang.san', NOW()); │       │
│  │                                                                                    │       │
│  │  -- 3. 📝 记录审计日志                                                             │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)   │       │
│  │  VALUES ('system.create', 'zhang.san', 'system', 'sys-001',                       │       │
│  │          '{"name": "shop", "description": "电商核心系统"}');                        │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ 无需审批: 任何用户都可以创建系统                                                          │
│                                                                                              │
│  👆 创建者自动成为该 System 的 Owner，拥有完全控制权                                            │
│     其他用户默认看不到此 System 及其下的 Service/VM                                            │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 4.A+: 资源级成员管理 (Owner 操作)                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  💡 核心设计: 资源创建者可以将其他用户添加到自己的 System/Service 中                              │
│     无需平台管理员参与，实现团队自服务                                                           │
│                                                                                              │
│  Owner 操作 (系统设置 → 成员管理):                                                              │
│                                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  系统成员管理 - shop                                                              │       │
│  │                                                                                    │       │
│  │  当前成员:                                                                          │       │
│  │  ┌────────────────────────────────────────────────────────────────────────────┐   │       │
│  │  │  用户              角色                操作                                  │   │       │
│  │  │  ────────────────────────────────────────────────────────────────────────  │   │       │
│  │  │  张三              Owner (创建者)      -                                     │   │       │
│  │  │  李四              Admin               [⚙ 编辑] [🗑 移除]                     │   │       │
│  │  │  王五              Member              [⚙ 编辑] [🗑 移除]                     │   │       │
│  │  │  赵六              Viewer              [⚙ 编辑] [🗑 移除]                     │   │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘   │       │
│  │                                                                                    │       │
│  │  [+ 添加成员]                                                                       │       │
│  │                                                                                    │       │
│  │  ┌─ 添加成员 ─────────────────────────────────────────────────────────────────┐   │       │
│  │  │  搜索用户:   [li.si@company.com      ] 🔍                                    │   │       │
│  │  │                                                                              │   │       │
│  │  │  权限角色:   [Member ▼]                                                       │   │       │
│  │  │                                                                              │   │       │
│  │  │  可选角色:                                                                    │   │       │
│  │  │    • Owner  - 完全控制 (转让所有权)                                           │   │       │
│  │  │    • Admin  - 可管理成员、创建/删除服务和 VM                                   │   │       │
│  │  │    • Member - 可创建服务和 VM，不能管理成员                                    │   │       │
│  │  │    • Viewer - 只读访问                                                        │   │       │
│  │  │                                                                              │   │       │
│  │  │  [添加]  [取消]                                                               │   │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘   │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 数据库设计 (资源级权限):                                                                    │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 资源角色绑定表 (区别于全局 role_bindings)                                       │       │
│  │  CREATE TABLE resource_role_bindings (                                            │       │
│  │    id VARCHAR PRIMARY KEY,                                                        │       │
│  │    user_id VARCHAR NOT NULL,                                                      │       │
│  │    role VARCHAR NOT NULL,          -- owner, admin, member, viewer                │       │
│  │    resource_type VARCHAR NOT NULL, -- system, service, vm                         │       │
│  │    resource_id VARCHAR NOT NULL,   -- 具体资源 ID                                  │       │
│  │    granted_by VARCHAR NOT NULL,    -- 授权人                                       │       │
│  │    created_at TIMESTAMP                                                           │       │
│  │  );                                                                               │       │
│  │                                                                                    │       │
│  │  -- 示例: 张三把李四添加为 shop 系统的 Admin                                        │       │
│  │  INSERT INTO resource_role_bindings                                               │       │
│  │    (id, user_id, role, resource_type, resource_id, granted_by, created_at)        │       │
│  │  VALUES                                                                           │       │
│  │    ('rrb-001', 'user-002', 'admin', 'system', 'sys-001', 'user-001', NOW());      │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔍 权限继承模型 (参考: Google Cloud IAM, GitHub Teams):                                       │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  ⭐ 核心原则: 子资源完全继承父资源的权限                                              │       │
│  │                                                                                    │       │
│  │  ┌─ 权限只需要在 System 层配置一次 ──────────────────────────────────────────────┐ │       │
│  │  │                                                                                │ │       │
│  │  │  System (shop)                    ← 在这里添加成员                              │ │       │
│  │  │    ├─ Admin: 李四                                                             │ │       │
│  │  │    ├─ Member: 王五, 赵六                                                       │ │       │
│  │  │    │                                                                           │ │       │
│  │  │    ├── Service (redis)            ← 自动继承李四、王五、赵六的权限              │ │       │
│  │  │    │     ├── VM (redis-01)        ← 自动继承                                   │ │       │
│  │  │    │     └── VM (redis-02)        ← 自动继承                                   │ │       │
│  │  │    │                                                                           │ │       │
│  │  │    └── Service (mysql)            ← 自动继承                                   │ │       │
│  │  │          └── VM (mysql-01)        ← 自动继承                                   │ │       │
│  │  │                                                                                │ │       │
│  │  └────────────────────────────────────────────────────────────────────────────────┘ │       │
│  │                                                                                    │       │
│  │  ✅ 好处:                                                                           │       │
│  │    - 添加/移除成员只需修改 System，Service 和 VM 自动生效                            │       │
│  │    - 避免了维护几十个 Service/VM 的成员配置                                          │       │
│  │    - 与 Google Cloud IAM 和 GitHub 的继承模型一致                                   │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔍 权限检查算法:                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  用户请求访问资源 R:                                                               │       │
│  │                                                                                    │       │
│  │  1. 全局权限检查:                                                                  │       │
│  │     - 拥有 platform:admin 权限 → 直接允许（显式超级管理员）                      │       │
│  │                                                                                    │       │
│  │  2. 资源级权限检查 (向上遍历继承链):                                                 │       │
│  │     ┌──────────────────────────────────────────────────────────────────────────┐ │       │
│  │     │  访问 VM (vm-001):                                                        │ │       │
│  │     │    1. 检查 vm-001 的 resource_role_binding → 未找到                        │ │       │
│  │     │    2. 向上: 检查所属 Service (svc-001) 的 binding → 未找到                  │ │       │
│  │     │    3. 再向上: 检查所属 System (sys-001) 的 binding → 找到! role=member     │ │       │
│  │     │    4. 返回 role=member 的权限 → ✅ 允许查看                                 │ │       │
│  │     └──────────────────────────────────────────────────────────────────────────┘ │       │
│  │                                                                                    │       │
│  │  伪代码:                                                                           │       │
│  │  ```                                                                               │       │
│  │  func checkPermission(user, resource) Role:                                        │       │
│  │      current = resource                                                            │       │
│  │      while current != nil:                                                         │       │
│  │          binding = findBinding(user, current)                                      │       │
│  │          if binding != nil:                                                        │       │
│  │              return binding.role                                                   │       │
│  │          current = current.parent  // VM→Service→System→nil                        │       │
│  │      return nil  // 无权限，资源不可见                                              │       │
│  │  ```                                                                               │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 权限矩阵 (继承自 System 的角色):                                                           │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │     ┌────────────┬────────┬────────┬────────┬────────┐                            │       │
│  │     │ 操作       │ Owner  │ Admin  │ Member │ Viewer │                            │       │
│  │     ├────────────┼────────┼────────┼────────┼────────┤                            │       │
│  │     │ 查看资源   │   ✅   │   ✅   │   ✅   │   ✅   │                            │       │
│  │     │ 创建子资源 │   ✅   │   ✅   │   ✅   │   ❌   │                            │       │
│  │     │ 修改资源   │   ✅   │   ✅   │   ❌   │   ❌   │                            │       │
│  │     │ 删除资源   │   ✅   │   ✅   │   ❌   │   ❌   │                            │       │
│  │     │ 管理成员   │   ✅   │   ✅   │   ❌   │   ❌   │  ← 仅在 System 层可操作       │       │
│  │     │ 转让所有权 │   ✅   │   ❌   │   ❌   │   ❌   │                            │       │
│  │     └────────────┴────────┴────────┴────────┴────────┘                            │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 设计说明:                                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  • Service 和 VM 层不单独配置成员，权限完全继承自 System                            │       │
│  │  • 以 System 为单位管理权限，简化运维                                               │       │
│  │  • 如需更细粒度隔离，可将资源拆分到不同 System                                       │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ⚠️ 权限边界:                                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  Shepherd 治理平台负责:                                                             │       │
│  │    ✅ 谁可以看到这些 VM (可见性)                                                    │       │
│  │    ✅ 谁可以创建/启停/删除 VM (生命周期管理)                                         │       │
│  │    ✅ 谁可以通过 VNC 控制台访问 (Web 控制台)                                         │       │
│  │                                                                                    │       │
│  │  Shepherd 不负责:                                                                   │       │
│  │    ❌ 谁可以 SSH/RDP 登录 VM (由企业堡垒机控制)                                      │       │
│  │    ❌ VM 内部的用户权限管理 (由 OS 自身管理)                                         │       │
│  │                                                                                    │       │
│  │  典型企业架构:                                                                       │       │
│  │    用户 → 堡垒机 (认证/审计/录屏) → VM                                              │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 4.B: 用户创建服务 (Service)                                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  用户操作:                                                                                    │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  创建服务                                                                          │       │
│  │                                                                                    │       │
│  │  所属系统:     [shop ▼]                                                             │       │
│  │  服务名称:     [redis              ]    👈 系统内唯一, 最长 15 字符                  │       │
│  │  服务描述:     [缓存服务            ]    👈 支持 Markdown 格式                       │       │
│  │               [预览] [上传 .md 文件]        ← 或上传已有 Markdown 文件                │       │
│  │                                                                                    │       │
│  │  [创建]                                                                             │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 数据库操作 (单事务):                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 创建服务                                                                    │       │
│  │  INSERT INTO services (id, name, description, system_id, created_by, created_at)  │       │
│  │  VALUES ('svc-001', 'redis', '缓存服务', 'sys-001', 'zhang.san', NOW());           │       │
│  │                                                                                    │       │
│  │  -- 2. 权限自动继承自 System (不需要额外 RoleBinding)                               │       │
│  │                                                                                    │       │
│  │  -- 3. 📝 记录审计日志                                                             │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id,            │       │
│  │                          parent_type, parent_id, details) VALUES                  │       │
│  │    ('service.create', 'zhang.san', 'service', 'svc-001', 'system', 'sys-001',     │       │
│  │     '{"name": "redis", "description": "缓存服务"}');                               │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ 无需审批: 系统成员可以创建服务                                                             │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 3: VM 生命周期流程

> **说明**: 本节描述 VM 的完整生命周期：创建请求 → 审批 → 执行 → 运行 → 删除

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 5.A: 用户提交 VM 请求                                               │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  普通用户:                                                                                    │
│                                                                                              │
│  ┌─ 提交 VM 创建请求 ───────────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  用户看到的界面:                                                                          │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐   │ │
│  │  │  创建虚拟机                                                                        │   │ │
│  │  │                                                                                    │   │ │
│  │  │  所属服务:     [shop / redis ▼]                                                    │   │ │
│  │  │  命名空间:     [prod-shop ▼]                                                       │   │ │
│  │  │  模板:         [centos7-docker ▼]                                                  │   │ │
│  │  │                                                                                    │   │ │
│  │  │  InstanceSize（规格）:         [gpu-workstation ▼]                                                 │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌── InstanceSize（规格）详情 ──────────────────────────────────────────────────────────────────┐ │   │ │
│  │  │  │  CPU: 8 核   内存: 32 GB                                                      │ │   │ │
│  │  │  │  ⚠️ 此InstanceSize（规格）包含 GPU: nvidia.com/GA102GL_A10                                    │ │   │ │
│  │  │  │     请确认您的业务确实需要 GPU 资源                                             │ │   │ │
│  │  │  └───────────────────────────────────────────────────────────────────────────────┘ │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ── 快速配置 ──                                                                    │   │ │
│  │  │  磁盘大小:     [====●==========] [100] GB   👈 默认值来自InstanceSize（规格）预设                   │   │ │
│  │  │                 50 ─────────── 500           用户可通过滑块或输入框调整             │   │ │
│  │  │                                                                                    │   │ │
│  │  │  申请理由:     [生产环境部署                ]                                       │   │ │
│  │  │                                                                                    │   │ │
│  │  │  [提交申请]                                                                         │   │ │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘   │ │
│  │                                                                                          │ │
│  │  👆 InstanceSize（规格）下拉框显示关键信息:                                                               │ │
│  │     - 普通InstanceSize（规格）: "medium (4核 8GB)" → 用户看到 CPU 和内存                                 │ │
│  │     - GPU InstanceSize（规格）: "gpu-workstation (8核 32GB)" + ⚠️GPU 提示 → 提醒用户确认是否需要         │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 5.B: 管理员审批                                                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  平台管理员:                                                                                  │
│                                                                                              │
│  系统根据 InstanceSize.spec_overrides 提取资源需求，匹配集群能力:                              │
│                                                                                              │
│  1. 提取资源需求:                                                                             │
│     - GPU: nvidia.com/GA102GL_A10                                                            │
│     - Hugepages: hugepages-2Mi                                                               │
│                                                                                              │
│  2. 匹配集群:                                                                                 │
│     - Cluster-A: 支持 nvidia.com/GA102GL_A10, hugepages-2Mi → ✅ 匹配                        │
│     - Cluster-B: 不支持 GPU → ❌ 过滤                                                         │
│                                                                                              │
│  3. 管理员审批界面:                                                                           │
│                                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  审批 VM 请求                                                                              │ │
│  │                                                                                            │ │
│  │  请求详情:                                                                                 │ │
│  │  ─────────────────────────────────────────────────────────────────────────────────────    │ │
│  │  申请人:       zhang.san                                                                   │ │
│  │  命名空间:     prod-shop              👈 生产环境                                          │ │
│  │  服务:         shop/redis                                                                  │ │
│  │  InstanceSize（规格）:         gpu-workstation (8核 32GB)                                                  │ │
│  │                                                                                            │ │
│  │  ── 磁盘配置 ─────────────────────────────────────────────────────────────────────────    │ │
│  │  磁盘大小:     [100     ] GB   (用户申请值: 100GB, InstanceSize（规格）范围: 50-500GB)                      │ │
│  │                                                                                            │ │
│  │  ── 资源分配 (InstanceSize（规格）含超卖时显示，可覆盖) ───────────────────────────────────────────────    │ │
│  │                                                                                            │ │
│  │  [✓] 启用覆盖    👈 管理员可覆盖InstanceSize（规格）的默认 request/limit 值                                  │ │
│  │                                                                                            │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────────┐ │ │
│  │  │                                                                                      │ │ │
│  │  │  CPU:    Request [4    ] 核    Limit [8    ] 核                                      │ │ │
│  │  │  内存:   Request [16Gi ]       Limit [32Gi ]                                         │ │ │
│  │  │                                                                                      │ │ │
│  │  │  ⚠️ 警告: 生产环境启用了超卖！                 👈 仅生产环境显示此警告                   │ │ │
│  │  │     高负载时可能影响 VM 性能。                                                         │ │ │
│  │  │                                                                                      │ │ │
│  │  │  🚨 冲突: 专用 CPU 与超卖不兼容！              👈 检测到冲突时显示 (红色警告)       │ │ │
│  │  │     VM 很可能无法启动。请取消专用 CPU 或禁用超卖。                                   │ │ │
│  │  │                                                                                      │ │ │
│  │  └──────────────────────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                                            │ │
│  │  集群:         [cluster-a ▼]     👈 系统已过滤不符合要求的集群                              │ │
│  │                                                                                            │ │
│  │  [批准]  [拒绝]                                                                            │ │
│  │                                                                                            │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  👆 显示逻辑:                                                                                 │
│     - 磁盘配置: 始终显示，管理员可调整                                                          │
│     - 资源分配 (request/limit): InstanceSize（规格）启用超卖时显示，不区分环境                                   │
│                                                                                              │
│  👆 警告逻辑 (仅提示，不阻止审批):                                                             │
│     1. request ≠ limit 且环境为 prod → ⚠️ 黄色警告 (生产环境超卖)                              │
│     2. 超卖 + 专用 CPU 同时启用 → 🚨 红色警告 (严重冲突，VM 很可能无法启动)                    │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 5.C: VM 创建执行                                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  系统自动执行:                                                                                │
│                                                                                              │
│  1. 生成 VM 名称: prod-shop-shop-redis-01                                                    │
│                                                                                              │
│  2. 合并生成最终 YAML:                                                                        │
│     Template (基础模板) + InstanceSize.spec_overrides + 用户参数 (disk_gb)                    │
│                                                                                              │
│  3. 渲染输出:                                                                                 │
│     apiVersion: kubevirt.io/v1                                                               │
│     kind: VirtualMachine                                                                     │
│     spec:                                                                                    │
│       template:                                                                              │
│         spec:                                                                                │
│           domain:                                                                            │
│             cpu:                                                                             │
│               cores: 8                                   ← 来自 spec_overrides               │
│               dedicatedCpuPlacement: true                ← 来自 spec_overrides               │
│             memory:                                                                          │
│               hugepages:                                                                     │
│                 pageSize: 2Mi                            ← 来自 spec_overrides               │
│             devices:                                                                         │
│               gpus:                                                                          │
│                 - name: gpu1                             ← 来自 spec_overrides               │
│                   deviceName: nvidia.com/GA102GL_A10                                        │
│                                                                                              │
│  4. 提交到 K8s 集群                                                                           │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 参数来源总结

| 参数 | 填写者 | 来源 | 说明 |
|------|--------|------|------|
| **Schema 字段类型/选项** | KubeVirt 官方 | JSON Schema | 开发者不定义，直接使用官方 Schema |
| **Mask 路径** | 开发者 | config/mask.yaml | 只指定暴露哪些路径 |
| **InstanceSize 具体值** | 管理员 | Admin UI (Schema 驱动) | 管理员根据 UI 填写，存为 spec_overrides |
| **Cluster/StorageClass** | 管理员 | 审批时选择 | 系统自动过滤符合条件的集群 |
| **VM Name/Labels** | 系统 | 自动生成 | 用户不可干预 |

### 与之前设计的关键区别

| 方面 | 之前（错误） | 现在（正确） |
|------|-------------|-------------|
| **字段选项来源** | 开发者在 Mask 中定义 | KubeVirt 官方 Schema |
| **存储结构** | `requirements map[string]string` | `spec_overrides map[string]interface{}` |
| **UI 渲染** | 开发者预定义下拉框选项 | 前端根据 Schema 类型自动渲染 |
| **后端职责** | 做 KV 子集匹配 | 只存储 JSON，提取资源做匹配 |

---

### 阶段 5.A (续): VM 创建请求 - 数据库操作

> **说明**: 用户提交 VM 请求后的数据库事务处理
>
> **⚠️ ADR 合规要求**:
> - [ADR-0009](../../../../adr/ADR-0009-domain-event-pattern.md): 领域事件必须在同一事务中创建
> - [ADR-0012](../../../../adr/ADR-0012-hybrid-transaction.md): 原子性 Ent + sqlc 事务
>
> **审计日志 vs 领域事件**:
> - `audit_logs`: 人类可读的合规记录 (谁在何时做了什么)
> - `domain_events`: 机器可读的状态流转 (系统重放/投影)
> 二者均为必须，功能互补，不可替代。

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         用户提交 VM 请求 - 数据库操作                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  用户点击 [提交申请] 按钮:                                                                     │
│                                                                                              │
│  📦 数据库操作 (单事务 - ADR-0012):                                                            │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  -- ⚠️ 重复请求检查 (ADR-0015 §10) - 在事务开始前执行                              │       │
│  │  -- 检查是否存在同 service_id + 同用户的 PENDING 请求                               │       │
│  │  SELECT id FROM approval_tickets                                                   │       │
│  │  WHERE service_id = 'svc-001'                                                      │       │
│  │    AND requester_id = 'zhang.san'                                                  │       │
│  │    AND status IN ('PENDING_APPROVAL', 'APPROVED')  -- 未完成的请求                  │       │
│  │    AND type = 'VM_CREATE';                                                         │       │
│  │  -- 如果找到 → 返回 409 Conflict (请求已存在)                                       │       │
│  │  -- 如果未找到 → 继续下方事务                                                       │       │
│  │                                                                                    │       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 创建领域事件 (ADR-0009) 👈 必须                                              │       │
│  │  INSERT INTO domain_events (                                                      │       │
│  │      id, type, aggregate_type, aggregate_id,                                       │       │
│  │      payload, status, created_at                                                   │       │
│  │  ) VALUES (                                                                        │       │
│  │      'evt-001',                                                                    │       │
│  │      'VM_CREATE_REQUESTED',             👈 事件类型                                 │       │
│  │      'vm', NULL,                        👈 聚合类型 (VM 尚未创建)                    │       │
│  │      '{"service_id": "svc-001", "instance_size_id": "is-gpu"...}',                │       │
│  │      'PENDING',                         👈 等待审批 (ADR-0009 L156)                 │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 2. 创建审批工单 (关联事件)                                                       │       │
│  │  INSERT INTO approval_tickets (                                                   │       │
│  │      id, event_id, type, status, requester_id,                                    │       │
│  │      service_id, namespace, instance_size_id, template_id,                        │       │
│  │      request_params, reason, created_at                                           │       │
│  │  ) VALUES (                                                                        │       │
│  │      'ticket-001',                                                                │       │
│  │      'evt-001',                         👈 关联领域事件                             │       │
│  │      'VM_CREATE',                                                                 │       │
│  │      'PENDING_APPROVAL',                👈 初始状态: 待审批                          │       │
│  │      'zhang.san',                                                                 │       │
│  │      'svc-001',                                                                   │       │
│  │      'prod-shop',                                                                 │       │
│  │      'is-gpu-workstation',                                                        │       │
│  │      'tpl-centos7',                                                               │       │
│  │      '{"disk_gb": 100}',                👈 用户可调整的参数                         │       │
│  │      '生产环境部署',                                                               │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 3. 记录审计日志 (人类可读合规)                                                   │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      id, action, actor_id, resource_type, resource_id, details, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'log-001', 'REQUEST_SUBMITTED', 'zhang.san',                                 │       │
│  │      'approval_ticket', 'ticket-001',                                             │       │
│  │      '{"action": "VM_CREATE", "namespace": "prod-shop"}',                         │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 4. 通过 NotificationSender 接口通知管理员 (ADR-0015 §20)                        │       │
│  │  --    V1: InboxNotificationSender (平台内置信箱)                                   │       │
│  │  --    V2+: 外部适配器 (Email, Webhook, Slack) 通过插件层实现                       │       │
│  │  INSERT INTO notifications (                                                      │       │
│  │      id, recipient_role, type, title, content, related_ticket_id, created_at      │       │
│  │  ) VALUES (                                                                        │       │
│  │      'notif-001', 'admin', 'APPROVAL_REQUIRED',                                   │       │
│  │      '新的 VM 创建请求', '用户 zhang.san 申请创建 VM...',                          │       │
│  │      'ticket-001', NOW()                                                          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 状态流转:                                                                                │
│     - ApprovalTicket: (无) → PENDING_APPROVAL                                                │
│     - DomainEvent: (无) → PENDING                                                            │
│                                                                                              │
│  🚫 注意: 此阶段不插入 River Job (等待审批)                                                    │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### 阶段 5.B (续): 管理员审批 - 数据库操作

> **说明**: 管理员审批/拒绝请求后的数据库事务处理
>
> **⚠️ ADR 合规要求**:
> - [ADR-0006](../../../../adr/ADR-0006-unified-async-model.md): River Job 必须在同一事务中插入
> - [ADR-0009](../../../../adr/ADR-0009-domain-event-pattern.md): DomainEvent 状态必须更新
> - [ADR-0012](../../../../adr/ADR-0012-hybrid-transaction.md): 原子性 Ent + sqlc + River InsertTx

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         管理员批准 VM 请求 - 数据库操作                                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  管理员点击 [批准] 按钮:                                                                       │
│                                                                                              │
│  📦 数据库操作 (单事务 - ADR-0012):                                                            │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 更新工单状态                                                                 │       │
│  │  UPDATE approval_tickets SET                                                      │       │
│  │      status = 'APPROVED',                  👈 状态变更: PENDING → APPROVED         │       │
│  │      approver_id = 'admin.li',                                                    │       │
│  │      approved_at = NOW(),                                                         │       │
│  │      selected_cluster_id = 'cluster-a',     👈 管理员选择的集群 (ADR-0017)            │       │
│  │      selected_storage_class = 'ceph-rbd',   👈 管理员选择的存储类                      │       │
│  │      template_snapshot = '{...}',          👈 模板快照 (ADR-0015 §17)              │       │
│  │      final_cpu_request = '4',              👈 最终 CPU request (超卖调整后)        │       │
│  │      final_cpu_limit = '8',                                                       │       │
│  │      final_mem_request = '16Gi',           👈 最终内存 request                     │       │
│  │      final_mem_limit = '32Gi',                                                    │       │
│  │      final_disk_gb = 100                   👈 最终磁盘大小                          │       │
│  │  WHERE id = 'ticket-001';                                                         │       │
│  │                                                                                    │       │
│  │  -- 2. 更新领域事件状态 (ADR-0009) 👈 必须                                           │       │
│  │  UPDATE domain_events SET                                                         │       │
│  │      status = 'PROCESSING',               👈 状态变更: PENDING → PROCESSING         │       │
│  │      updated_at = NOW()                                                           │       │
│  │  WHERE id = 'evt-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 3. 生成 VM 名称并创建 VM 记录                                                    │       │
│  │  INSERT INTO vms (                                                                │       │
│  │      id, name, service_id, namespace, cluster_id,                                 │       │
│  │      instance_size_id, template_id, status,                                       │       │
│  │      ticket_id, created_at                                                        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm-001',                                                                    │       │
│  │      'prod-shop-shop-redis-01',            👈 自动生成: {ns}-{sys}-{svc}-{index}   │       │
│  │      'svc-001', 'prod-shop', 'cluster-a',                                         │       │
│  │      'is-gpu-workstation', 'tpl-centos7',                                         │       │
│  │      'CREATING',                           👈 初始状态: 创建中                      │       │
│  │      'ticket-001', NOW()                                                          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 4. 插入 River Job (ADR-0006/0012) 👈 必须 - 触发异步执行                         │       │
│  │  INSERT INTO river_job (                                                          │       │
│  │      id, kind, args, queue, state, created_at                                     │       │
│  │  ) VALUES (                                                                        │       │
│  │      'job-001',                                                                   │       │
│  │      'VMCreateJob',                        👈 River worker 类型                     │       │
│  │      '{"event_id": "evt-001", "vm_id": "vm-001", "ticket_id": "ticket-001"}',    │       │
│  │      'default',                                                                   │       │
│  │      'available',                          👈 可被 worker 消费                      │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │  -- 注意: 代码中使用 riverClient.InsertTx(), 而非原始 INSERT                         │       │
│  │                                                                                    │       │
│  │  -- 5. 记录审计日志                                                                 │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      id, action, actor_id, resource_type, resource_id, details, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'log-002', 'REQUEST_APPROVED', 'admin.li',                                   │       │
│  │      'approval_ticket', 'ticket-001',                                             │       │
│  │      '{"cluster": "cluster-a", "vm_name": "prod-shop-shop-redis-01"}',            │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 6. 发送通知给用户                                                               │       │
│  │  INSERT INTO notifications (                                                      │       │
│  │      id, recipient_id, type, title, content, related_ticket_id, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'notif-002', 'zhang.san', 'REQUEST_APPROVED',                                │       │
│  │      '您的 VM 请求已批准', 'VM prod-shop-shop-redis-01 正在创建...',               │       │
│  │      'ticket-001', NOW()                                                          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 状态流转:                                                                                │
│     - ApprovalTicket: PENDING_APPROVAL → APPROVED                                            │
│     - DomainEvent: PENDING → PROCESSING                                                      │
│     - VM: (无) → CREATING                                                                    │
│     - RiverJob: (无) → available                                                             │
│                                                                                              │
│  🔄 异步执行: River worker 读取 Job，调用 KubeVirt API                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         管理员拒绝 VM 请求 - 数据库操作                                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  管理员点击 [拒绝] 按钮:                                                                       │
│                                                                                              │
│  📦 数据库操作 (单事务 - ADR-0012):                                                            │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 更新工单状态                                                                 │       │
│  │  UPDATE approval_tickets SET                                                      │       │
│  │      status = 'REJECTED',                  👈 状态变更: PENDING → REJECTED         │       │
│  │      approver_id = 'admin.li',                                                    │       │
│  │      rejected_at = NOW(),                                                         │       │
│  │      rejection_reason = '资源不足，请选择其他InstanceSize（规格）'                  │       │
│  │  WHERE id = 'ticket-001';                                                         │       │
│  │                                                                                    │       │
│  │  -- 2. 更新领域事件状态 (ADR-0009) 👈 必须                                           │       │
│  │  UPDATE domain_events SET                                                         │       │
│  │      status = 'CANCELLED',                👈 状态变更: PENDING → CANCELLED (被拒绝) │       │
│  │      updated_at = NOW()                                                           │       │
│  │  WHERE id = 'evt-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 3. 记录审计日志                                                                 │       │
│  │  INSERT INTO audit_logs (...) VALUES (...);                                       │       │
│  │                                                                                    │       │
│  │  -- 4. 通知用户                                                                    │       │
│  │  INSERT INTO notifications (...) VALUES (...);                                    │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 状态流转:                                                                                │
│     - ApprovalTicket: PENDING_APPROVAL → REJECTED                                            │
│     - DomainEvent: PENDING → CANCELLED                                                       │
│  ❌ 无 VM 记录创建, 无 River Job 插入                                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### 阶段 5.D: 删除操作

> **说明**: VM/Service/System 的删除流程和数据库操作

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         删除流程 - 层级依赖关系                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  层级结构 (参考 ADR-0015):                                                                    │
│                                                                                              │
│      System (shop)                                                                           │
│         │                                                                                    │
│         ├── Service (redis)                                                                  │
│         │      ├── VM (prod-shop-shop-redis-01)                                             │
│         │      └── VM (prod-shop-shop-redis-02)                                             │
│         │                                                                                    │
│         └── Service (mysql)                                                                  │
│                └── VM (prod-shop-shop-mysql-01)                                             │
│                                                                                              │
│  删除规则 (Cascade Restrict):                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  删除层级      前置条件                    需要审批    确认方式                     │       │
│  │  ────────────────────────────────────────────────────────────────────────────────  │       │
│  │  VM            无                          ✅ 是       confirm=true 参数          │       │
│  │  Service       下属所有 VM 必须先删除      ✅ 是       confirm=true 参数          │       │
│  │  System        下属所有 Service 必须先删除 ❌ 否       输入系统名称确认            │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         删除 VM - 数据库操作                                                  │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  用户或管理员发起删除:                                                                         │
│  DELETE /api/v1/vms/{vm_id}?confirm=true                                                    │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 创建删除审批工单                                                             │       │
│  │  INSERT INTO approval_tickets (                                                   │       │
│  │      id, type, status, requester_id, resource_type, resource_id, created_at       │       │
│  │  ) VALUES (                                                                        │       │
│  │      'ticket-002', 'VM_DELETE', 'PENDING_APPROVAL',                               │       │
│  │      'zhang.san', 'vm', 'vm-001', NOW()                                           │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 2. 记录审计日志                                                                 │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm.delete_request', 'zhang.san', 'vm', 'vm-001', 'service', 'svc-001',      │       │
│  │      '{"name": "prod-shop-shop-redis-01", "reason": "资源回收"}'                   │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  管理员批准后:                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. 更新工单状态                                                                 │       │
│  │  UPDATE approval_tickets SET status = 'APPROVED', ... WHERE id = 'ticket-002';    │       │
│  │                                                                                    │       │
│  │  -- 2. 更新 VM 状态为 DELETING (不是直接删除记录)                                    │       │
│  │  UPDATE vms SET status = 'DELETING' WHERE id = 'vm-001';                          │       │
│  │                                                                                    │       │
│  │  -- 3. 记录审计日志                                                                 │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm.delete', 'admin.li', 'vm', 'vm-001', 'service', 'svc-001',               │       │
│  │      '{"name": "prod-shop-shop-redis-01", "approved_by": "admin.li"}'             │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔄 异步任务: Worker 执行 kubectl delete vm，成功后更新 status = 'DELETED'                    │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         删除 Service - 数据库操作                                             │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  DELETE /api/v1/services/{service_id}?confirm=true                                          │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 前置检查: 是否有活跃 VM                                                         │       │
│  │  SELECT COUNT(*) FROM vms                                                         │       │
│  │  WHERE service_id = 'svc-001' AND status NOT IN ('DELETED', 'DELETING');          │       │
│  │                                                                                    │       │
│  │  IF count > 0 THEN                                                                │       │
│  │      RETURN ERROR("服务下还有 {count} 个活跃 VM，请先删除");                        │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- 创建删除审批工单 (同 VM 删除流程)                                                │       │
│  │  INSERT INTO approval_tickets (...);                                              │       │
│  │                                                                                    │       │
│  │  -- 记录审计日志                                                                    │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'service.delete_request', 'zhang.san', 'service', 'svc-001', 'system', 'sys-001',│     │
│  │      '{"name": "redis", "reason": "服务迁移"}'                                     │       │
│  │  );                                                                                │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  管理员批准后:                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  UPDATE services SET status = 'DELETED', deleted_at = NOW()                       │       │
│  │  WHERE id = 'svc-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 记录审计日志                                                                    │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'service.delete', 'admin.li', 'service', 'svc-001', 'system', 'sys-001',     │       │
│  │      '{"name": "redis", "approved_by": "admin.li"}'                               │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 软删除: 记录保留用于审计                                                         │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         删除 System - 数据库操作 (无需审批)                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  DELETE /api/v1/systems/{system_id}                                                         │
│  Body: { "confirm_name": "shop" }    👈 必须输入系统名称确认                                  │
│                                                                                              │
│  📦 数据库操作:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 前置检查 1: 确认名称匹配                                                        │       │
│  │  IF confirm_name != system.name THEN                                              │       │
│  │      RETURN ERROR("确认名称不匹配");                                               │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- 前置检查 2: 是否有活跃 Service                                                  │       │
│  │  SELECT COUNT(*) FROM services                                                    │       │
│  │  WHERE system_id = 'sys-001' AND status != 'DELETED';                             │       │
│  │                                                                                    │       │
│  │  IF count > 0 THEN                                                                │       │
│  │      RETURN ERROR("系统下还有 {count} 个服务，请先删除");                           │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- 执行软删除 (无需审批)                                                           │       │
│  │  UPDATE systems SET status = 'DELETED', deleted_at = NOW()                        │       │
│  │  WHERE id = 'sys-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 记录审计日志                                                                    │       │
│  │  INSERT INTO audit_logs (...) VALUES (...);                                       │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ❌ 不创建审批工单: System 删除由名称确认保护，无需审批                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 4: 状态机与数据模型

> **说明**: 本节定义系统中核心实体的状态机和数据库表关系，是前后端开发的重要参考。

### 审批工单状态流转图

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         审批工单 (ApprovalTicket) 状态流转                                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│                        ┌───────────────────┐                                                 │
│                        │  PENDING_APPROVAL │                                                 │
│                        │     (待审批)       │                                                 │
│                        └─────────┬─────────┘                                                 │
│                                  │                                                           │
│              ┌───────────────────┼───────────────────┐                                      │
│              │                   │                   │                                      │
│              ▼                   ▼                   ▼                                      │
│     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                                 │
│     │  APPROVED   │     │  REJECTED   │     │  CANCELLED  │                                 │
│     │   (已批准)   │     │   (已拒绝)   │     │  (已取消)   │                                 │
│     └──────┬──────┘     └─────────────┘     └─────────────┘                                 │
│            │                 (终态)              (终态)                                      │
│            ▼                                                                                 │
│     ┌─────────────┐                                                                          │
│     │  EXECUTING  │                                                                          │
│     │   (执行中)   │                                                                          │
│     └──────┬──────┘                                                                          │
│            │                                                                                 │
│     ┌──────┴──────┐                                                                          │
│     ▼             ▼                                                                          │
│  ┌─────────┐  ┌─────────┐                                                                    │
│  │ SUCCESS │  │ FAILED  │                                                                    │
│  │ (成功)   │  │ (失败)   │                                                                    │
│  └─────────┘  └─────────┘                                                                    │
│    (终态)       (终态)                                                                        │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### VM 状态流转图

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         VM 状态流转                                                           │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                                 │
│     │  CREATING   │────▶│   RUNNING   │◀────│   STOPPED   │                                 │
│     │   (创建中)   │     │   (运行中)   │     │   (已停止)   │                                 │
│     └─────────────┘     └──────┬──────┘     └─────────────┘                                 │
│            │                   │                   ▲                                         │
│            │                   ▼                   │                                         │
│            │            ┌─────────────┐            │                                         │
│            │            │  STOPPING   │────────────┘                                         │
│            │            │   (停止中)   │                                                      │
│            │            └─────────────┘                                                      │
│            │                                                                                 │
│            │                   │                                                             │
│            ▼                   ▼                                                             │
│     ┌─────────────┐     ┌─────────────┐                                                      │
│     │   FAILED    │     │  DELETING   │                                                      │
│     │  (创建失败)  │     │   (删除中)   │                                                      │
│     └─────────────┘     └──────┬──────┘                                                      │
│                                │                                                             │
│                                ▼                                                             │
│                         ┌─────────────┐                                                      │
│                         │   DELETED   │                                                      │
│                         │   (已删除)   │                                                      │
│                         └─────────────┘                                                      │
│                              (终态)                                                           │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### 数据库表关系概览

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         核心表关系图                                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌──────────────┐         ┌──────────────┐         ┌──────────────┐                         │
│  │   systems    │ 1 ─── N │   services   │ 1 ─── N │     vms      │                         │
│  │──────────────│         │──────────────│         │──────────────│                         │
│  │ id           │         │ id           │         │ id           │                         │
│  │ name         │◀────────│ system_id    │◀────────│ service_id   │                         │
│  │ description  │         │ name         │         │ name         │                         │
│  │ status       │         │ status       │         │ status       │                         │
│  │ created_by   │         │ created_by   │         │ namespace    │                         │
│  └──────────────┘         └──────────────┘         │ cluster_id   │                         │
│         │                                          │ ticket_id    │                         │
│         │                                          └──────────────┘                         │
│         │                                                  │                                 │
│         ▼                                                  ▼                                 │
│  ┌──────────────┐                               ┌──────────────────┐                        │
│  │ role_bindings│                               │ approval_tickets │                        │
│  │──────────────│                               │──────────────────│                        │
│  │ user_id      │                               │ id               │                        │
│  │ role         │                               │ type             │                        │
│  │ resource_type│                               │ status           │                        │
│  │ resource_id  │                               │ requester_id     │                        │
│  └──────────────┘                               │ approver_id      │                        │
│                                                 │ service_id       │                        │
│                                                 │ instance_size_id │                        │
│                                                 │ template_id      │                        │
│                                                 │ final_*          │ ← 审批时确定的最终值    │
│                                                 └──────────────────┘                        │
│                                                          │                                  │
│  ┌──────────────┐         ┌──────────────┐              │                                  │
│  │instance_sizes│         │  templates   │              ▼                                  │
│  │──────────────│         │──────────────│       ┌──────────────┐                          │
│  │ id           │         │ id           │       │ audit_logs   │                          │
│  │ name         │         │ name         │       │──────────────│                          │
│  │ spec_overrides│        │ image_source │       │ action       │                          │
│  │ cpu_overcommit│        │ cloud_init   │       │ actor_id     │                          │
│  │ mem_overcommit│        │ version      │       │ resource_*   │                          │
│  │ disk_gb_*    │         │ status       │       │ details      │                          │
│  └──────────────┘         └──────────────┘       │ created_at   │                          │
│                                                  └──────────────┘                          │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 审计日志设计

> **参考**: ADR-0015 §7 (Deletion & Cascade Constraints) - "audit records are preserved"

> 📦 **Schema**: 完整 DDL 和索引定义见 [04-governance.md §7 Storage Schema](../../../design/phases/04-governance.md#storage-schema)

#### 需要记录审计的操作

| 类别 | 操作 (action) | 触发时机 | 详情 (details) 内容 |
|------|---------------|----------|---------------------|
| **认证** | `user.login` | 用户登录成功 | `{method: "oidc", idp: "Corp-SSO"}` |
| **认证** | `user.login_failed` | 登录失败 | `{reason: "invalid_token"}` |
| **认证** | `user.logout` | 用户登出 | `{}` |
| **System** | `system.create` | 创建系统 | `{name: "shop", description: "..."}` |
| **System** | `system.update` | 修改系统 | `{changes: {description: {old: "...", new: "..."}}}` |
| **System** | `system.delete` | 删除系统 | `{confirmation: "shop"}` |
| **Service** | `service.create` | 创建服务 | `{name: "redis", system_id: "..."}` |
| **Service** | `service.delete_request` | 提交服务删除请求 | `{name: "redis", reason: "服务迁移"}` |
| **Service** | `service.delete` | 删除服务 (审批后) | `{approved_by: "..."}` |
| **VM** | `vm.request` | 提交 VM 创建请求 | `{instance_size: "...", template: "...", count: 3}` |
| **VM** | `vm.create` | VM 创建成功 | `{cluster: "...", namespace: "..."}` |
| **VM** | `vm.start` | 启动 VM | `{}` |
| **VM** | `vm.stop` | 停止 VM | `{graceful: true}` |
| **VM** | `vm.restart` | 重启 VM | `{}` |
| **VM** | `vm.delete_request` | 提交 VM 删除请求 | `{name: "...", reason: "资源回收"}` |
| **VM** | `vm.delete` | 删除 VM (审批后) | `{approved_by: "..."}`  |
| **VNC** | `vnc.access` | 访问 VNC 控制台 | `{vm_id: "...", session_duration: 3600}` |
| **Approval** | `approval.approve` | 批准请求 | `{ticket_id: "...", final_cluster: "...", final_disk_gb: 100}` |
| **Approval** | `approval.reject` | 拒绝请求 | `{ticket_id: "...", reason: "资源不足"}` |
| **Approval** | `approval.cancel` | 取消请求 | `{ticket_id: "...", reason: "不再需要"}` |
| **RBAC** | `role.create` | 创建自定义角色 | `{name: "CustomViewer", permissions: [...]}` |
| **RBAC** | `role.update` | 修改角色权限 | `{role: "Operator", changes: {permissions: {added: [...], removed: [...]}}}` |
| **RBAC** | `role.delete` | 删除自定义角色 | `{name: "CustomViewer"}` |
| **RBAC** | `role.assign` | 分配角色给用户 | `{user_id: "...", role: "SystemAdmin", scope: "system:shop"}` |
| **RBAC** | `role.revoke` | 撤销用户角色 | `{user_id: "...", role: "Operator"}` |
| **RBAC** | `permission.create` | 创建权限 | `{code: "vm:vnc", description: "..."}` |
| **RBAC** | `permission.delete` | 删除权限 | `{code: "vm:vnc"}` |
| **Cluster** | `cluster.register` | 注册集群 | `{name: "prod-01", environment: "prod", api_server: "..."}` |
| **Cluster** | `cluster.update` | 修改集群配置 | `{name: "prod-01", changes: {environment: {old: "test", new: "prod"}}}` |
| **Cluster** | `cluster.delete` | 删除/注销集群 | `{name: "prod-01", reason: "集群下线"}` |
| **Cluster** | `cluster.credential_rotate` | 轮换集群凭证 | `{name: "prod-01", rotated_at: "..."}` |
| **Template** | `template.create` | 创建模板 | `{name: "centos7-docker", version: 1}` |
| **Template** | `template.update` | 更新模板 (版本+1) | `{name: "centos7-docker", version: 2, changes: {...}}` |
| **Template** | `template.deprecate` | 标记模板为弃用 | `{name: "centos6-base", successor: "centos7-base"}` |
| **Template** | `template.delete` | 删除模板 | `{name: "centos6-base", version: 3}` |
| **InstanceSize** | `instance_size.create` | 创建InstanceSize（规格） | `{name: "medium-gpu", cpu: 4, memory: "8Gi", gpu: 1}` |
| **InstanceSize** | `instance_size.update` | 修改InstanceSize（规格） | `{name: "medium-gpu", changes: {memory: {old: "8Gi", new: "16Gi"}}}` |
| **InstanceSize** | `instance_size.deprecate` | 标记InstanceSize（规格）为弃用 | `{name: "small-legacy"}` |
| **InstanceSize** | `instance_size.delete` | 删除InstanceSize（规格） | `{name: "small-legacy"}` |
| **Namespace** | `namespace.create` | 创建命名空间 | `{name: "prod-shop", cluster: "prod-01"}` |
| **Namespace** | `namespace.delete` | 删除命名空间 | `{name: "prod-shop"}` |
| **IdP** | `idp.configure` | 配置 IdP 连接 | `{type: "oidc", issuer: "...", client_id: "..."}` |
| **IdP** | `idp.update` | 更新 IdP 配置 | `{changes: {issuer: {...}}}` |
| **IdP** | `idp.delete` | 删除 IdP 配置 | `{type: "oidc"}` |
| **IdP** | `idp.sync` | 手动同步 IdP 组 | `{synced_groups: 15, new_users: 3}` |
| **IdP** | `idp.mapping_create` | 创建组-角色映射 | `{idp_group: "DevOps", role: "SystemAdmin", env: "prod"}` |
| **IdP** | `idp.mapping_update` | 更新组-角色映射 | `{idp_group: "DevOps", changes: {role: {old: "Viewer", new: "Operator"}}}` |
| **IdP** | `idp.mapping_delete` | 删除组-角色映射 | `{idp_group: "DevOps"}` |
| **Config** | `config.update` | 修改平台配置 | `{key: "approval.timeout_hours", old: 24, new: 48}` |

#### 不需要记录审计的操作 (例外)

以下操作因其高频或低敏感性，**不记录审计日志**：

| 类别 | 操作 | 不记录原因 |
|------|------|-----------|
| **系统巡检** | K8s 集群定时健康检查 | 高频定时任务，无用户触发 |
| **系统巡检** | VM 状态同步轮询 | 每分钟执行，数据量过大 |
| **系统巡检** | 资源配额检查 | 内部检查，无业务意义 |
| **只读操作** | 列表查询 (`GET /api/v1/*`) | 只读不改变状态 |
| **只读操作** | 详情查看 (`GET /api/v1/*/id`) | 只读不改变状态 |
| **内部通信** | Worker 心跳 | 系统内部通信 |
| **内部通信** | Metrics 采集 | 监控数据采集 |

> **例外处理原则**:
> - 所有 **写操作** (CREATE/UPDATE/DELETE) 必须记录
> - 所有 **敏感读操作** (如 VNC 访问) 必须记录
> - 纯 **系统自动化** 和 **只读查询** 可以豁免

#### 审计日志记录示例

```
示例 1: 用户创建 VM 请求
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, parent_type, parent_id, details) VALUES
    ('vm.request', 'user-001', '张三', 'approval_ticket', 'ticket-001',
     'service', 'svc-001',
     '{"instance_size": "medium-gpu", "template": "centos7-docker",
       "count": 3, "namespace": "prod-shop"}');

示例 2: 管理员批准请求
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, details) VALUES
    ('approval.approve', 'admin-001', '管理员李四', 'approval_ticket', 'ticket-001',
     '{"final_cluster": "prod-cluster-01", "final_disk_gb": 100,
       "final_storage_class": "ceph-ssd", "vms_created": 3}');

示例 3: VNC 访问记录
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, details, ip_address) VALUES
    ('vnc.access', 'user-001', '张三', 'vm', 'vm-redis-01',
     '{"session_id": "vnc-xxx", "duration_seconds": 1800}',
     '192.168.1.100');

示例 4: 删除资源 (保留审计)
  -- 删除 VM 时，先记录审计日志
  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id,
                          parent_type, parent_id, details) VALUES
    ('vm.delete', 'user-001', 'vm', 'vm-redis-01', 'service', 'svc-001',
     '{"name": "prod-shop-redis-01", "cluster": "prod-cluster-01",
       "existed_days": 45, "last_status": "RUNNING"}');
  
  -- 然后执行硬删除
  DELETE FROM vms WHERE id = 'vm-redis-01';
  
  💡 审计日志保留，资源记录删除
```

#### 审计日志查询示例

```sql
-- Query all actions for a user
SELECT * FROM audit_logs 
WHERE actor_id = 'user-001' 
ORDER BY created_at DESC LIMIT 50;

-- Query resource history
SELECT * FROM audit_logs 
WHERE resource_type = 'vm' AND resource_id = 'vm-redis-01'
ORDER BY created_at DESC;

-- Query all approval actions
SELECT * FROM audit_logs 
WHERE action LIKE 'approval.%' 
ORDER BY created_at DESC;

-- Query sensitive prod actions
SELECT * FROM audit_logs 
WHERE environment = 'prod' 
  AND action IN ('vm.delete', 'system.delete', 'approval.approve')
ORDER BY created_at DESC;
```

#### 审计日志保留策略

| 环境 | 保留时间 | 说明 |
|------|----------|------|
| **生产环境** | ≥ 1 年 | 满足合规要求 |
| **测试环境** | ≥ 90 天 | 可配置缩短 |
| **敏感操作** | ≥ 3 年 | `*.delete`, `approval.*`, `rbac.*` |

---

### 审计日志 JSON 导出 (v1+)

> **场景**: 将审计日志集成到企业级 SIEM 系统（Elasticsearch、Datadog、Splunk 等）

> 📦 **API 规范**: 完整 API 和响应格式见 [04-governance.md §7 JSON Export API](../../../design/phases/04-governance.md#7-json-export-api)

**主要功能**:
- 支持时间范围过滤的分页导出
- Webhook 推送集成，实现实时流式传输
- 结构化 JSON 格式，兼容主流日志聚合器

---

### 外部审批系统集成 (v1+)

> **场景**: 与企业现有 ITSM 系统（Jira Service Management、ServiceNow 等）集成

#### 设计原则

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         外部审批系统集成架构                                                   │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐   │
│  │   Shepherd   │  ──── Webhook ───▶ │  外部系统    │  ──── Callback ──▶ │   Shepherd   │   │
│  │   Platform   │                    │ (Jira/SNOW)  │                    │   Platform   │   │
│  └──────────────┘                    └──────────────┘                    └──────────────┘   │
│                                                                                              │
│  关键原则:                                                                                    │
│  1. Shepherd 只关注标准 API 接口，不关心外部系统内部流转                                         │
│  2. 采用异步事件驱动架构，不阻塞用户操作                                                        │
│  3. 外部审批是可插拔的，v1 默认使用内置审批                                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### 外部审批配置 (通过 Web UI 配置，存储于 PostgreSQL)

> 管理员在 **设置 → 外部审批系统 → 添加** 进行配置，所有配置存储在 `external_approval_systems` 表中。

**Webhook 安全最佳实践**：
- 所有 webhook URL 必须使用 HTTPS。
- 使用共享密钥进行签名校验，并采用常量时间比较。
- 在签名载荷中包含时间戳，拒绝过期请求，防止重放。
- webhook 密钥需加密存储，泄露时及时轮换。

参考：
- https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
- https://docs.stripe.com/webhooks/test

```sql
-- Example: external_approval_systems record
INSERT INTO external_approval_systems (
  id, name, type, enabled,
  webhook_url, webhook_secret, webhook_headers,
  callback_secret, status_mapping,
  timeout_seconds, retry_count,
  created_by
) VALUES (
  'eas-001', 
  'Jira Service Management',
  'webhook',
  true,
  'https://jira.company.com/api/v2/tickets',
  'encrypted:AES256:xxx',  -- encrypted with ENCRYPTION_KEY
  '{"Authorization": "Bearer ${JIRA_TOKEN}"}',
  'encrypted:AES256:xxx',  -- HMAC secret for callback verification
  '{"Approved": "APPROVED", "Rejected": "REJECTED", "Cancelled": "CANCELLED"}',
  30, 3,
  'admin'
);
```

#### Webhook 发送格式 (Shepherd → 外部系统)

```json
// POST https://jira.company.com/api/v2/tickets
{
  "shepherd_ticket_id": "ticket-001",
  "type": "VM_CREATE",
  "callback_url": "https://shepherd.company.com/api/v1/approvals/callback",
  "requester": {
    "id": "zhang.san",
    "name": "张三",
    "email": "zhang.san@company.com"
  },
  "request_details": {
    "namespace": "prod-shop",
    "service": "redis",
    "instance_size": "medium-gpu",
    "template": "centos7-docker",
    "vm_count": 3,
    "reason": "生产环境部署"
  },
  "resource_summary": {
    "cpu_cores": 8,
    "memory_gb": 32,
    "disk_gb": 100,
    "gpu_count": 1
  },
  "environment": "prod",
  "created_at": "2026-01-26T10:14:16Z"
}
```

#### Callback 接收格式 (外部系统 → Shepherd)

```json
// POST https://shepherd.company.com/api/v1/approvals/callback
// Headers:
//   X-Shepherd-Signature: HMAC-SHA256 签名
//   Content-Type: application/json
{
  "shepherd_ticket_id": "ticket-001",
  "external_ticket_id": "JIRA-12345",    // 外部系统工单 ID (用于追溯)
  "status": "Approved",                   // 外部系统状态 (将通过 status_mapping 转换)
  "approver": {
    "id": "admin.li",
    "name": "管理员李四"
  },
  "comments": "资源充足，批准创建",
  "approved_at": "2026-01-26T11:30:00Z"
}
```

#### Shepherd 处理回调

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         Callback 处理流程                                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  1. 验证 HMAC 签名                                                                           │
│  2. 查找 shepherd_ticket_id 对应的工单                                                       │
│  3. 通过 status_mapping 转换状态                                                             │
│  4. 更新工单状态和 approver 信息                                                              │
│  5. 如果 APPROVED:                                                                          │
│     a. 触发 VM 创建 Worker 任务                                                              │
│     b. 发送通知给申请人                                                                      │
│  6. 如果 REJECTED:                                                                          │
│     a. 记录拒绝原因                                                                          │
│     b. 发送通知给申请人                                                                      │
│  7. 记录审计日志                                                                              │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### 集成注意事项

| 注意事项 | 说明 |
|----------|------|
| **幂等性** | Callback 可能重试，需确保多次处理同一回调不会产生副作用 |
| **状态同步** | 定期检查外部系统中 pending 状态的工单，防止回调丢失 |
| **超时处理** | V1: 不自动取消。超时后外部系统可调用拒绝 API（参见 ADR-0015 §11） |
| **安全性** | 始终验证 HMAC 签名，防止伪造回调 |
| **回退机制** | 外部系统不可用时，自动回退到内置审批 |

---

## 阶段 6: VNC 控制台访问 (ADR-0015 §18, RFC-0011)

> **范围**: 通过 noVNC 实现浏览器端 VM 控制台访问。
>
> **ADR-0015 §18 合规性**:
> - 权限矩阵（测试/生产环境区分）
> - Token 安全性（单次使用、时间限制、用户绑定）
> - 审计日志要求

### 权限矩阵

| 环境 | 需要审批 | Token TTL | 说明 |
|------|----------|-----------|------|
| **测试** | ❌ 否 | 2 小时 | 仅 RBAC 检查（`vnc:access` 权限） |
| **生产** | ✅ 是 | 2 小时 | 需要审批工单 |

### VNC 访问流程

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         阶段 6: VNC 控制台访问                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌─ 测试环境 (无需审批) ────────────────────────────────────────────────────────────────────┐
│  │                                                                                          │
│  │  1. 用户点击 VM 详情页的 [控制台] 按钮                                                    │
│  │                                                                                          │
│  │  2. 后端检查:                                                                            │
│  │     a. 用户拥有该命名空间的 `vnc:access` 权限                                            │
│  │     b. VM 处于 RUNNING 状态                                                              │
│  │     c. 环境为测试环境（无需审批）                                                         │
│  │                                                                                          │
│  │  3. 生成 VNC Token (JWT):                                                               │
│  │     {                                                                                    │
│  │       "sub": "user-123",           👈 用户绑定                                           │
│  │       "vm_id": "vm-456",           👈 资源绑定                                           │
│  │       "cluster": "cluster-a",                                                           │
│  │       "namespace": "test-ns",                                                           │
│  │       "exp": now + 2h,             👈 有效期 (ADR-0015)                                  │
│  │       "jti": "vnc-token-789",      👈 唯一 ID 用于审计                                   │
│  │       "single_use": true           👈 首次连接后失效                                     │
│  │     }                                                                                    │
│  │                                                                                          │
│  │  4. 在新标签页/弹窗打开 noVNC:                                                           │
│  │     GET /vnc/{vm_id}?token={vnc_jwt}                                                    │
│  │                                                                                          │
│  │  5. 后端代理 WebSocket 到 KubeVirt:                                                      │
│  │     → subresources.kubevirt.io/v1/namespaces/{ns}/virtualmachineinstances/{name}/vnc    │
│  │                                                                                          │
│  │  6. 创建审计日志:                                                                        │
│  │     INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)      │
│  │     VALUES ('VNC_SESSION_STARTED', 'user-123', 'vm', 'vm-456',                          │
│  │             '{"token_id": "vnc-token-789", "environment": "test"}')                      │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
│  ┌─ 生产环境 (需要审批) ────────────────────────────────────────────────────────────────────┐
│  │                                                                                          │
│  │  1. 用户点击 VM 详情页的 [申请控制台访问] 按钮                                            │
│  │                                                                                          │
│  │  2. 后端检查:                                                                            │
│  │     a. 用户拥有该命名空间的 `vnc:access` 权限                                            │
│  │     b. VM 处于 RUNNING 状态                                                              │
│  │     c. 环境为生产环境 → 需要审批                                                         │
│  │     d. 不存在待处理的 VNC 访问请求（重复检查）                                            │
│  │                                                                                          │
│  │  3. 创建审批工单:                                                                        │
│  │     INSERT INTO approval_tickets (type, status, requester_id, resource_id, ...)         │
│  │     VALUES ('VNC_ACCESS_REQUESTED', 'PENDING_APPROVAL', 'user-123', 'vm-456', ...)      │
│  │                                                                                          │
│  │  4. 通知管理员审批                                                                       │
│  │                                                                                          │
│  │  5. 管理员审批（与 VM 请求审批流程相同）                                                   │
│  │                                                                                          │
│  │  6. 审批通过后:                                                                          │
│  │     a. 生成 VNC Token（结构与测试环境相同）                                               │
│  │     b. 通知用户并提供访问链接                                                            │
│  │     c. 用户在新标签页打开 noVNC                                                          │
│  │                                                                                          │
│  │  7. 创建审计日志（与测试环境相同）                                                         │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### VNC Token 安全性 (V1 简化版)

| 安全特性 | V1 实现 | ADR-0015 要求 |
|----------|---------|---------------|
| **单次使用** | 首次连接后标记 `used_at` | ✅ 必须 |
| **时间限制** | JWT `exp` = now + 2h | ✅ 2 小时（可配置） |
| **用户绑定** | JWT `sub` = user_id | ✅ 必须 |
| **加密** | AES-256-GCM（共享密钥管理） | ✅ 必须 |
| **审计记录** | `VNC_SESSION_STARTED` 事件 | ✅ 必须 |

> **V1 限制**: 不支持主动 Token 撤销。安全性依赖短 TTL 和单次使用标记。

### API 端点

```
# 请求 VNC 访问（生产环境创建审批工单）
POST /api/v1/vms/{vm_id}/console/request
→ 响应: { "ticket_id": "...", "status": "PENDING_APPROVAL" }  (生产)
→ 响应: { "vnc_url": "/vnc/{vm_id}?token=..." }              (测试)

# VNC WebSocket 端点
GET /api/v1/vms/{vm_id}/vnc?token={vnc_jwt}
Upgrade: websocket
→ 代理到 KubeVirt VNC 子资源

# 检查控制台访问状态（轮询用）
GET /api/v1/vms/{vm_id}/console/status
→ 响应: { "status": "APPROVED", "vnc_url": "..." } | { "status": "PENDING" }
```

### 数据库操作

```sql
-- VNC 访问请求（生产环境）
INSERT INTO approval_tickets (
    id, type, status, requester_id, resource_type, resource_id, 
    environment, created_at
) VALUES (
    'vnc-ticket-001', 'VNC_ACCESS_REQUESTED', 'PENDING_APPROVAL',
    'user-123', 'vm', 'vm-456', 'production', NOW()
);

-- VNC Token 使用追踪（V1: 内嵌 JWT，无独立表）
-- Token 验证: 检查 JWT 签名 + exp + jti 不在 used_tokens 缓存中
-- 首次使用: 将 jti 添加到 Redis used_tokens 集合（TTL = Token TTL）
```

---

