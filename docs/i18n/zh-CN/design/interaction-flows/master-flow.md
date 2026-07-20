# 规范交互流程 (Master Flow)

> **Status**: Stable (ADR-0017, ADR-0018 Accepted)  
> **版本**: 1.3
> **创建日期**: 2026-01-28  
> **最后更新**: 2026-03-06
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
> 示例: "所有操作都会创建审计日志。详见 [04-governance.md §7](../../../../design/phases/04-governance.md#7-audit-logging) 了解 Schema 详情。"

### 文档层级（防止内容漂移）

| 文档 | 职责 | 范围 |
|------|------|------|
| **ADRs** | 架构决策（接受后不可变） | 决策理由和权衡分析 |
| **本文档 (master-flow.md)** | 交互原理（唯一真源） | 数据来源、流程原理、用户旅程 |
| **Phase 文档** | 实现细节 | 代码模式、Schema 设计、API 设计 |
| **[CHECKLIST.md](../../../../design/CHECKLIST.md)** | ADR 约束引用 | 集中式 ADR 强制规则 |

> **写作指南**: 本文档描述"什么数据"和"为什么这样流动"。
> 关于"如何实现"，链接到 Phase 文档，而不是重复内容。
> 示例: "InstanceSize Schema 详情，参见 [01-contracts.md §InstanceSize](../../../../design/phases/01-contracts.md#deliverables)。"

**相关文档**:
- [ADR-0018: Instance Size Abstraction §User Interaction Flow](../../../../adr/ADR-0018-instance-size-abstraction.md#user-interaction-flow)
- [ADR-0015: Governance Model V2 §Decision](../../../../adr/ADR-0015-governance-model-v2.md#decision)
- [ADR-0017: VM Request Flow §Decision](../../../../adr/ADR-0017-vm-request-flow-clarification.md#decision)
- [Phase 01: 契约 §API Contract-First Design](../../../../design/phases/01-contracts.md#api-contract-first-design-adr-0021) — 数据契约和命名约束
- [Phase 04: 治理 §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging) — RBAC、审计日志、审批流程
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../../../../design/frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023) — 前端基线实现标准
- [frontend/features/batch-operations-queue.md §2 Parent/Child UI Model](../../../../design/frontend/features/batch-operations-queue.md#2-parentchild-ui-model) — 父子队列 UI 与轮询语义

**关键 ADR 约束（适用于本文档所有流程）**:

| ADR | 约束 | 适用范围 |
|-----|------|---------|
| **ADR-0006** | 所有写操作使用**统一异步模型**（请求 → 202 → River 队列） | 所有状态变更操作 |
| **ADR-0009** | DomainEvent 工作流仅携带 **EventID**；非 DomainEvent worker 可携带一个持久化归属行主键（Claim Check）；DomainEvent payload **不可变** | 所有 River Job |
| **ADR-0012** | 原子事务：Ent 用于 ORM，**sqlc 仅用于核心事务** | 所有数据库操作 |

> **CI 概览**: 上述约束由自动化检查保障。完整门禁定义和脚本清单见 [docs/design/ci/README.md §Scope Boundary](../../../../design/ci/README.md#scope-boundary)。

---

## 统一写作契约

本节定义全文固定写作风格，确保所有 Stage 章节具备一致的阅读体验，
同时保留必要结论，不把读者完全丢给外链。

### Stage 固定结构（必须）

每个 `Stage` 章节必须按以下顺序组织：

1. `Purpose`（本阶段目标，1-2 行）
2. `Actors & Trigger`（谁触发、前置条件）
3. `Interaction Flow`（仅交互流程图）
4. `State Transitions`（实体状态变化与归属边界）
5. `Failure & Edge Cases`（重复请求、无权限、状态冲突等）
6. `Authority Links`（可点击 ADR/phase/database/frontend/CI 链接）
7. `Scope Boundary`（明确本节不展开的实现细节）

### Part 地图（规范）

| Part | 主要关注点 | 主要读者 |
|------|-----------|---------|
| **Part 1** | 平台初始化与安全基线 | 开发者、平台管理员 |
| **Part 2** | 资源层级与归属边界 | 普通用户、平台管理员 |
| **Part 3** | VM 申请/审批/执行/删除全生命周期 | 普通用户、平台管理员 |
| **Part 4** | 状态机与共享数据模型语义 | 前后端工程师 |
| **Part 5/6** | 批量、通知、VNC 等专项流程 | 全栈工程师 |

### 全局设计结论（各 Stage 不得覆盖）

| 主题 | 规范结论 |
|------|---------|
| **命名治理** | 平台管理的逻辑名必须通过 ADR-0019 统一校验。 |
| **写入模型** | 状态变更操作遵循统一异步模型（`request -> 202 -> River`），见 [ADR-0006 §Decision](../../../../adr/ADR-0006-unified-async-model.md#decision)。 |
| **事件完整性** | DomainEvent worker 采用 EventID-only claim-check；非 DomainEvent worker 可携带一个持久化归属行主键；事件 payload 不可变，见 [ADR-0009 §Constraint 1](../../../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only)。 |
| **事务边界** | 跨聚合核心写入采用 Ent+sqlc 原子事务模型，见 [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode)。 |
| **删除语义** | 主资源表硬删除（可短暂 `DELETING`）；审计/工单/事件独立保留并归档，见 [ADR-0015 §13](../../../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)。 |
| **批量基线** | V1 批量采用父子工单 + 两层限流，见 [ADR-0015 §19](../../../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)。 |

### 跨层权威关系

| 文档层 | 权威范围 |
|------|---------|
| [ADRs §Reading Order](../../../../adr/README.md#reading-order) | 已接受架构决策与权衡 |
| `master-flow.md` | 交互意图与端到端预期行为 |
| [docs/design/README.md §Implementation Phases](../../../../design/README.md#implementation-phases) | 实现契约与运行约束 |
| [database/README.md §Document Map](../../../../design/database/README.md#document-map) | 持久化生命周期、一致性与 Schema 归属 |
| [frontend/README.md §Reading Order](../../../../design/frontend/README.md#reading-order) | UI 交互规范与功能级 UX 行为 |
| [ci/README.md §Scope Boundary](../../../../design/ci/README.md#scope-boundary) | 可执行门禁与防漂移检查 |

### 边界声明

- `master-flow.md` 负责交互意图与行为预期。
- SQL/DDL/索引/迁移等数据库细节必须写入 `docs/design/database/`。
- 组件实现与代码级模式必须写入 `docs/design/phases/` 与 `docs/design/frontend/`。

---

## Part 1: 平台初始化流程 {#stage-1}

### Purpose

定义 Schema 驱动平台初始化和首次安全部署的预期行为。

### Actors & Trigger

- 触发：首次部署或平台配置变更。
- 参与方：开发者、平台管理员、启动引导流程。

### Interaction Flow

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
│  │      professional_fields:                                                                │ │
│  │        - path: "spec.template.spec.domain.features.hyperv.relaxed.enabled"               │ │
│  │          display_name: "Hyper-V Relaxed"                                                 │ │
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

### 状态流转（阶段 1）

| 领域 | 之前 | 之后 |
|------|------|------|
| Schema 缓存 | 未知/空 | 具备版本化 schema |
| Mask 配置 | 未定义 | 暴露路径通过校验 |
| UI 渲染能力 | 静态/手工 | Schema 驱动 |

### 失败与边界（阶段 1）

- Schema 获取失败必须降级到嵌入式 schema 基线。
- 无效 mask 路径必须在部署前校验失败。

### Authority Links (Part 1 baseline)

- [ADR-0023 §1 Schema Cache Management Policy](../../../../adr/ADR-0023-schema-cache-and-api-standards.md#1-schema-cache-management-policy)
- [01-contracts.md §API Contract-First Design](../../../../design/phases/01-contracts.md#api-contract-first-design-adr-0021)
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../../../../design/frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023)

### 边界声明（阶段 1）

本阶段定义初始化流程预期行为。迁移步骤与代码生成命令细节在 phase/CI 文档中维护。

#### Schema Cache 生命周期引用 {#schema-cache-lifecycle-adr-0023}

Schema 缓存生命周期与降级行为，请以以下文档为准：

- [ADR-0023 §1 Schema Cache Management Policy](../../../../adr/ADR-0023-schema-cache-and-api-standards.md#1-schema-cache-management-policy)
- [02-providers.md §6 Schema Cache Lifecycle](../../../../design/phases/02-providers.md#6-schema-cache-lifecycle-adr-0023)
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../../../../design/frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023)

### 阶段 1.5: 首次部署引导 (Bootstrap) {#stage-1-5}

> **Added 2026-01-26**: 配置存储策略的首次部署流程。
>
> **详细规则**: Bootstrap Secrets 优先级和自动生成详见 [ADR-0025 §Decision Outcome](../../../../adr/ADR-0025-secret-bootstrap.md#decision-outcome)，实现细节详见 [01-contracts.md §3.2.2](../../../../design/phases/01-contracts.md#322-system-secrets-table-adr-0025)。

#### Purpose

统一首次启动时的配置优先级与密钥引导行为。

#### Actors & Trigger

- 触发：系统首次启动且运行态密钥不存在。
- 参与方：部署操作者、引导逻辑、数据库持久化层。

#### Interaction Flow

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
│  ⚡ **单一优先级链** (重要 - 避免歧义):                                                          │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  配置类型              │  优先级链 (从高到低)                                          │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  一般配置              │  环境变量 → config.yaml → 代码默认值                            │ │
│  │  (端口、日志级别)       │  例: SERVER_PORT 环境变量覆盖 config.yaml 中的 server.port    │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  密钥/敏感配置          │  环境变量 → 数据库生成 (system_secrets 表)                     │ │
│  │  (加密密钥、会话密钥)     │  若设置了 ENCRYPTION_KEY 环境变量 → 使用它 (不生成)          │ │
│  │                        │  若未设置 ENCRYPTION_KEY → 自动生成并存入数据库               │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  🔮 V2+ (RFC-0017)     │  外部 KMS → 环境变量 → 数据库生成                             │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ⚠️ **关键原则**: config.yaml 不是密钥来源 (12-factor app 合规)。                             │
│     密钥必须来自: 环境变量 或 数据库生成 或 外部密钥管理器。                                      │
│                                                                                              │
│  🔐 自动生成（缺省时）:                                                                      │
│  - 首次启动若缺少 ENCRYPTION_KEY / SESSION_SECRET，自动生成 32 字节强随机密钥（CSPRNG）        │
│  - 持久化存入 PostgreSQL `system_secrets` 表 (禁止仅内存临时密钥)                             │
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

#### State Transitions

| 范围 | 之前 | 之后 |
|------|------|------|
| 默认管理员 | 无 | 已初始化（`force_password_change=true`） |
| 核心密钥 | 未设置 | 来自环境变量或自动生成并持久化 |
| 内置角色 | 未初始化 | 基线角色已落库（幂等 seed） |

#### Failure & Edge Cases

- 缺少必需数据库连接时必须在部分写入前中止启动。
- 密钥生成与持久化必须原子完成，避免进入不可恢复状态。

#### Authority Links

- [ADR-0025 §Decision Outcome](../../../../adr/ADR-0025-secret-bootstrap.md#decision-outcome)
- [01-contracts.md §3.2.2 System Secrets Table](../../../../design/phases/01-contracts.md#322-system-secrets-table-adr-0025)
- [00-prerequisites.md §7 CI Pipeline](../../../../design/phases/00-prerequisites.md#7-ci-pipeline)
- [00-prerequisites.md §8 Data Initialization](../../../../design/phases/00-prerequisites.md#8-data-initialization-adr-0018)

#### Scope Boundary

本节仅定义首次部署行为与结果。密钥轮换与高级运维流程不在本节展开。

### 阶段 2: 平台安全配置 (首次部署) {#stage-2}

> **参考**: ADR-0015 §22 (Authentication & RBAC Strategy)

<a id="stage-2-a"></a>
<a id="stage-2-a-plus"></a>
<a id="stage-2-b"></a>
<a id="stage-2-c"></a>
<a id="stage-2-d"></a>

#### Purpose

建立业务流量进入前必须具备的认证、鉴权与安全基线。

#### Actors & Trigger

- 触发：首次部署后执行安全初始化。
- 参与方：引导进程、平台管理员、身份源集成配置。

#### Interaction Flow

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
│  │    ('builtin_approval:approve', 'builtin_approval', 'approve', '审批请求'),        │       │
│  │    ('builtin_approval:view', 'builtin_approval', 'view', '查看待审批'),             │       │
│  │    ('cluster:read', 'cluster', 'read', '查看集群'),                                │       │
│  │    ('cluster:write', 'cluster', 'write', '创建/更新/删除集群'),                    │       │
│  │    ('template:read', 'template', 'read', '查看模板'),                              │       │
│  │    ('template:write', 'template', 'write', '创建/更新/删除模板'),                  │       │
│  │    ('rbac:manage', 'rbac', 'manage', '管理权限'),                                  │       │
│  │    ('platform:admin', 'platform', 'admin', '超级管理员权限（显式）');               │       │
│  │    -- ⚠️ ADR-0019 RBAC 合规:                                                        │       │
│  │    -- 所有角色使用显式权限。通配符模式 (*:*) 已禁止。                                │       │
│  │    -- platform:admin 是显式超管权限（编译时常量）。                                  │       │
│  │    -- 环境范围通过 RoleBindings.allowed_environments 表达。                         │       │
│  │    -- 预上线清理已移除 bootstrap 专用和兼容时代的内置角色。                          │       │
│  │                                                                                    │       │
│  │  -- 2. 内置角色 (ADR-0019 合规)                                                    │       │
│  │  INSERT INTO roles (id, name, is_builtin, description) VALUES                     │       │
│  │    ('role-platform-admin', 'PlatformAdmin', true, '平台管理员'),                   │       │
│  │    ('role-approval-admin', 'ApprovalAdmin', true, '审批管理员'),                    │       │
│  │    ('role-development-engineer', 'DevelopmentEngineer', true, '开发工程师'),        │       │
│  │    ('role-test-engineer', 'TestEngineer', true, '测试工程师'),                      │       │
│  │    ('role-system-operator', 'SystemOperator', true, '系统运维'),                    │       │
│  │    ('role-viewer', 'Viewer', true, '只读用户');                                    │       │
│  │                                                                                    │       │
│  │  -- 3. 角色-权限关联 (ADR-0019: 禁止通配符，仅使用显式权限)                          │       │
│  │  INSERT INTO role_permissions (role_id, permission_id) VALUES                     │       │
│  │    -- PlatformAdmin: platform:admin (ADR-0019 显式超管权限)                         │       │
│  │    ('role-platform-admin', 'platform:admin'),                                      │       │
│  │    -- ApprovalAdmin: 审批队列负责人，不承担平台管理                                  │       │
│  │    ('role-approval-admin', 'builtin_approval:approve'),                             │       │
│  │    ('role-approval-admin', 'builtin_approval:view'),                                │       │
│  │    ('role-approval-admin', 'ticket:view'), ('role-approval-admin', 'vm:read'),     │       │
│  │    ('role-approval-admin', 'system:read'), ('role-approval-admin', 'service:read'),│       │
│  │    -- 工程/运维角色: 相同能力包，由绑定控制 test/prod 环境                           │       │
│  │    ('role-development-engineer', 'system:read'), ('role-development-engineer', 'system:write'),│
│  │    ('role-development-engineer', 'service:create'), ('role-development-engineer', 'service:read'),│
│  │    ('role-development-engineer', 'vm:read'), ('role-development-engineer', 'vm:create'),│
│  │    ('role-development-engineer', 'vm:operate'), ('role-development-engineer', 'vm:delete'),│
│  │    ('role-development-engineer', 'vnc:access'),                                     │       │
│  │    ('role-test-engineer', 'system:read'), ('role-test-engineer', 'system:write'),  │       │
│  │    ('role-test-engineer', 'service:create'), ('role-test-engineer', 'service:read'),│       │
│  │    ('role-test-engineer', 'vm:read'), ('role-test-engineer', 'vm:create'),         │       │
│  │    ('role-test-engineer', 'vm:operate'), ('role-test-engineer', 'vm:delete'),      │       │
│  │    ('role-test-engineer', 'vnc:access'),                                            │       │
│  │    ('role-system-operator', 'system:read'), ('role-system-operator', 'system:write'),│      │
│  │    ('role-system-operator', 'service:create'), ('role-system-operator', 'service:read'),│      │
│  │    ('role-system-operator', 'vm:read'), ('role-system-operator', 'vm:create'),     │       │
│  │    ('role-system-operator', 'vm:operate'), ('role-system-operator', 'vm:delete'),  │       │
│  │    ('role-system-operator', 'vnc:access'),                                          │       │
│  │    ('role-viewer', 'system:read'), ('role-viewer', 'service:read'),                │       │
│  │    ('role-viewer', 'vm:read');                                                     │       │
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
│  │  │  [🔒] ApprovalAdmin          内置    审批管理员                                   │   │   │
│  │  │  [🔒] DevelopmentEngineer    内置    开发工程师                                   │   │   │
│  │  │  [🔒] TestEngineer           内置    测试工程师                                   │   │   │
│  │  │  [🔒] SystemOperator         内置    系统运维                                     │   │   │
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
│  │  │  │ ☑ system:read              │  │ ☐ builtin_approval:approve │                   │   │   │
│  │  │  │ ☑ system:write             │  │ ☐ builtin_approval:view    │                   │   │   │
│  │  │  │ ☐ system:delete            │  └────────────────────────────┘                   │   │   │
│  │  │  └────────────────────────────┘                                                    │   │   │
│  │  │  ┌─ 服务管理 ─────────────────┐  ┌─ 平台管理 ─────────────────┐                   │   │   │
│  │  │  │ ☑ service:read             │  │ ☐ cluster:write           │                   │   │   │
│  │  │  │ ☑ service:create           │  │ ☐ template:write          │                   │   │   │
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

> **标准化 Provider 输出**：所有认证提供方（OIDC/LDAP/SSO）通过适配层统一成标准输出，用于 RBAC 映射。见 [ADR-0026 §Standard Provider Output](../../../../adr/ADR-0026-idp-config-naming.md#standard-provider-output-contract)。

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
│  │  API: GET /api/v1/admin/auth-providers/{id}/sample                                               │   │
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
│  │  │  Platform-Admin     [PlatformAdmin ▼]      ☑ test  ☑ prod                      │   │   │
│  │  │  DevOps-Team        [SystemOperator ▼]     ☑ test  ☑ prod                      │   │   │
│  │  │  QA-Team            [TestEngineer ▼]       ☑ test  ☐ prod                      │   │   │
│  │  │  IT-Support         [Viewer ▼]          ☑ test  ☐ prod                         │   │   │
│  │  │  HR-Department      [无映射 ▼]          -                                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  💡 未映射的组默认不授予权限，必须显式映射后才能获得平台访问                        │   │   │
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
│  │  INSERT INTO external_cohort_mappings (id, auth_provider_id, cohort_kind, cohort_key, │     │
│  │                                       role_id, scope_type, allowed_environments) VALUES │     │
│  │    ('map-001', 'idp-001', 'Platform-Admin', 'role-platform-admin',                │       │
│  │     'global', ARRAY['test', 'prod']),                                             │       │
│  │    ('map-002', 'idp-001', 'DevOps-Team', 'role-system-operator',                  │       │
│  │     'global', ARRAY['test', 'prod']),                                             │       │
│  │    ('map-003', 'idp-001', 'QA-Team', 'role-test-engineer',                        │       │
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
│  │     c. 根据 groups / cohorts 查找 external_cohort_mappings                           │ │
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
│  │  -- 2. 删除旧的自动分配 RoleBindings                                                 │       │
│  │  DELETE FROM role_bindings                                                         │       │
│  │  WHERE user_id = 'user-001' AND source = 'external_cohort';                       │       │
│  │                                                                                    │       │
│  │  -- 3. 根据用户的 groups / cohorts 重新创建 RoleBindings                             │       │
│  │  -- (用户 group:DevOps-Team → 映射到 role-system-operator)                          │       │
│  │  INSERT INTO role_bindings (id, user_id, role_id, scope_type,                     │       │
│  │                             allowed_environments, source) VALUES                  │       │
│  │    ('rb-auto-001', 'user-001', 'role-system-operator', 'global',                  │       │
│  │     ARRAY['test', 'prod'], 'external_cohort');                                    │       │
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
| **角色类型** | PlatformAdmin, ApprovalAdmin, DevelopmentEngineer, TestEngineer, SystemOperator, Viewer, 自定义角色 | Owner, Admin, Member, Viewer |
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
1. 全局权限: system:read ∈ SystemOperator 权限 → 继续
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

#### 阶段 2 平台管理员引导说明

- 预上线 RBAC 基线中不再存在独立的 `role-bootstrap`。
- 首次初始化使用默认管理员账号，然后移交给命名的 `PlatformAdmin` 绑定。
- 操作流程：
  [operations/platform-admin-sop.md](../../../../operations/platform-admin-sop.md)
- 治理与审计基线：
  [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)

#### State Transitions

| 领域 | 典型状态变化 |
|------|-------------|
| 用户认证档案 | 首次登录后 `uninitialized -> active` |
| 角色绑定 | `absent -> assigned`（全局或资源级） |
| 审批能力 | 配置完成后 `disabled -> enabled` |

#### Failure & Edge Cases

- 默认管理员账号在初始化完成后必须按运维策略收口，避免长期共享超管入口。
- 外部 IdP 映射漂移不得导致静默提权。
- 继承链无绑定时必须默认拒绝可见性与操作。

#### Authority Links

- [ADR-0015 §22 Authentication and RBAC Strategy](../../../../adr/ADR-0015-governance-model-v2.md#22-authentication-rbac-strategy)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)
- [01-contracts.md §1.1 Naming Constraints](../../../../design/phases/01-contracts.md#11-naming-constraints-adr-0019)

#### Scope Boundary

本节定义安全交互语义与权限边界。协议细节与加固清单在 phase/operations 文档中定义。

### 阶段 2.E: 审批 Provider 标准 (V1 内置，V2+ 外部插件) {#stage-2-e}

> **Added 2026-01-26**: 审批 Provider 模型与外部集成边界

#### Purpose

定义统一的审批 Provider 契约。V1 只内置一个 Provider；
外部系统通过插件适配接入，且不改变审批状态机语义。

#### Actors & Trigger

- 触发：平台管理员制定审批 Provider 策略与路由规则。
- 参与方：平台管理员、审批 Provider 路由层、内置 Provider、可选外部 Provider 适配器。

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 阶段 2.E：审批 Provider 边界（统一契约 + 可插拔实现）                                           │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  V1 上线路径（必需）：                                                                          │
│    1) 用户提交请求 -> tickets=PENDING                                                         │
│    2) 路由层选择内置 Provider（`builtin-default`，V1 唯一实现）                                │
│    3) 内置审批人做 APPROVED / REJECTED 决策                                                    │
│    4) Shepherd 执行后续路径并记录审计                                                          │
│                                                                                              │
│  外部插件路径（V2+ 路线图）：                                                                  │
│    1) 外部适配器插件按策略注册并启用                                                           │
│    2) 路由层通过 ExternalApprovalProvider.SubmitForApproval 委派工单                           │
│    3) 回调/轮询结果映射到统一的 APPROVED / REJECTED                                            │
│    4) 外部超时/不可用 -> 可控回退到内置审批队列                                                │
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
│  │  系统自动探测，管理员无需手动配置:                                                          │ │
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
│  ┌─ 步骤 2: 配置 Namespace ───────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  ⚠️ 核心原则:                                                                              │ │
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
│  │     3. 对 K8s API 错误做分类并返回标准错误码（细节见框后 Markdown 说明）。                  │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ┌─ 步骤 3: 配置 Template ─────────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  模板定义 VM 的启动盘基础配置:                                                              │ │
│  │  - `containerdisk` 临时根盘                                                                │ │
│  │  - `cdi_image_import` 通过 CDI 导入的持久化根盘                                            │ │
│  │  - `cdi_pvc_clone` 通过 CDI 从源 PVC 克隆的持久化根盘                                      │ │
│  │  - cloud-init 配置 (管理员可自定义)                                                        │ │
│  │  - 字段可见性控制 (quick_fields / advanced_fields / professional_fields)                  │ │
│  │                                                                                          │ │
│  │  💡 注意: 硬件能力要求 (GPU/SR-IOV/Hugepages) 已移至 InstanceSize 配置                     │ │
│  │  💡 直接从已有 PVC 启动 VM 不是受支持的产品模式                                             │ │
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
│  │  │  类型:         (●) containerdisk   ( ) cdi_image_import   ( ) cdi_pvc_clone      │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌─ containerdisk 模式 ──────────────────────────────────────────────────────┐    │   │ │
│  │  │  │  镜像地址:   [docker.io/kubevirt/centos:7                    ]             │    │   │ │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘    │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌─ cdi_image_import 模式 ────────────────────────────────────────────────────┐   │   │ │
│  │  │  │  镜像地址:   [quay.io/containerdisks/centos:stream9         ]              │   │   │ │
│  │  │  │  来源:       [registry ▼]                                                   │   │   │ │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘   │   │ │
│  │  │                                                                                    │   │ │
│  │  │  ┌─ cdi_pvc_clone 模式 ───────────────────────────────────────────────────────┐   │   │ │
│  │  │  │  Namespace:  [golden-images      ]                                         │   │   │ │
│  │  │  │  PVC 名称:   [centos9-base-root  ]                                         │   │   │ │
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
│  │  模板版本说明:                                                                            │ │
│  │  - 用户提交请求时看到当前活跃版本                                                          │ │
│  │  - 管理员审批时可选择不同版本                                                              │ │
│  │  - 最终模板内容快照到审批工单 Ticket（`tickets`），VM 创建后不受模板更新影响               │ │
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
│  │      "cpu_cores": 8,                                                                      │ │
│  │      "cpu_request": 4,                    👈 request < cores 时表示超卖                   │ │
│  │      "memory_gi": 32,                                                                     │ │
│  │      "memory_request_gi": 16,           👈 request < limit 时表示超卖                    │ │
│  │      "dedicated_cpu": true,                                                               │ │
│  │      "requires_hugepages": true,                                                          │ │
│  │      "hugepages_size": "2Mi",                                                             │ │
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
│  ⚠️ Dry-Run 校验：                                                                           │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  保存前，管理员可对目标集群执行 InstanceSize 兼容性校验：                                 │ │
│  │                                                                                          │ │
│  │  POST /api/v1/admin/instance-sizes?dryRun=All                                            │ │
│  │  POST /api/v1/admin/instance-sizes?dryRun=All&targetCluster={cluster_id}                 │ │
│  │                                                                                          │ │
│  │  校验阶段：                                                                              │ │
│  │  ┌────────────────────────────────────────────────────────────────────────────────────┐  │ │
│  │  │  阶段 1: 结构检查            → YAML/JSON 语法合法                                  │  │ │
│  │  │  阶段 2: Schema 校验         → 与 KubeVirt VirtualMachine Schema 兼容               │  │ │
│  │  │  阶段 3: 集群 Dry-Run (可选) → 在目标集群执行 kubectl apply --dry-run=server        │  │ │
│  │  └────────────────────────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                                          │ │
│  │  响应（dry-run 模式）：                                                                  │ │
│  │  {                                                                                       │ │
│  │      "valid": true,                                                                     │ │
│  │      "rendered_yaml": "...",     👈 生成 VM 规格预览                                    │ │
│  │      "compatible_clusters": ["cluster-a", "cluster-c"]   👈 匹配的集群列表              │ │
│  │  }                                                                                       │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### 阶段 3 JIT Namespace 执行说明 {#stage-3-jit-namespace}

<a id="stage-3-c"></a>

- 错误分类（标准错误码）：
  - `NAMESPACE_PERMISSION_DENIED (403)`：目标集群拒绝 namespace 创建权限。
  - `NAMESPACE_QUOTA_EXCEEDED (403)`：被集群 ResourceQuota 策略拒绝。
  - `NAMESPACE_CREATION_FAILED (500)`：其他未归类的 K8s/API 错误。
- 失败处理基线：
  - 工单状态转为 `FAILED_PROVISIONING`。
  - Worker 使用指数退避策略重试。
- 规范来源：
  - [ADR-0017 §Namespace Just-In-Time Creation (Added 2026-01-27)](../../../../adr/ADR-0017-vm-request-flow-clarification.md#namespace-just-in-time-creation-added-2026-01-27)
  - [01-contracts.md §Error Code Standard (ADR-0023)](../../../../design/phases/01-contracts.md#error-code-standard-adr-0023)

#### State Transitions

| 领域 | 典型状态变化 |
|------|-------------|
| 审批 Provider 集合 | 内置实现隐式存在 -> 显式 Provider 注册表（V1 仅内置） |
| 决策契约 | 存在提供方差异解读风险 -> 统一 `APPROVED/REJECTED` 契约 |
| 故障回退模式 | 隐式 -> 外部适配器故障时显式回退到内置审批 |

#### Failure & Edge Cases

- 外部适配器不可用不得阻塞内置 Provider 主路径。
- 回调签名或状态映射不合法必须拒绝并记录审计。
- 外部审批超时必须保持工单可恢复（回退或继续 pending），不得形成孤儿状态。

#### Authority Links

- [ADR-0005 §Decision](../../../../adr/ADR-0005-workflow-extensibility.md#decision)
- [ADR-0015 §21 Scope Exclusions (V1)](../../../../adr/ADR-0015-governance-model-v2.md#21-scope-exclusions-v1)
- [04-governance.md §9 External Approval Systems](../../../../design/phases/04-governance.md#9-external-approval-systems-v1-interface-only)
- [04-governance.md §9.1 Interface Definition](../../../../design/phases/04-governance.md#91-interface-definition)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)
- [RFC-0004 External Approval Systems Integration](../../../../rfc/RFC-0004-external-approval.md)

#### Scope Boundary

本节只定义审批 Provider 模型与 V1 边界。
提供方 payload/callback/安全细节归属
[Part 4 §审批 Provider 插件化架构 (V2+ 路线图)](#external-approval-v2-roadmap)
与 RFC-0004。


---

## Part 2: 资源管理流程

<a id="stage-4-a"></a>
<a id="stage-4-a-plus"></a>
<a id="stage-4-b"></a>
<a id="stage-4-c"></a>

> **说明**: 用户在创建 VM 之前，必须先创建 System 和 Service 来组织资源。

### Purpose

定义 System/Service 资源层级创建与归属关系的交互行为。

### Actors & Trigger

- 触发：普通用户为后续 VM 工作负载创建资源层级。
- 参与方：资源拥有者、团队成员、RBAC 评估层、审计子系统。

### Interaction Flow

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

### State Transitions (Part 2)

| 实体 | 典型状态变化 |
|------|-------------|
| System | `none -> ACTIVE`（创建成功） |
| Service | 父 System 校验通过后 `none -> ACTIVE` |
| 资源成员关系 | `none -> owner/admin/member/viewer`（遵循继承语义） |

### Failure & Edge Cases (Part 2)

- 未授权或不可见的父 System 下创建 Service 必须失败。
- 同一作用域内逻辑名冲突必须在提交前失败。
- 删除必须满足级联约束与确认参数要求。

### Authority Links (Part 2)

- [ADR-0015 §13 Deletion Cascade Constraints](../../../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0019 §Baseline Controls (Normative)](../../../../adr/ADR-0019-governance-security-baseline-controls.md#baseline-controls-normative)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../../../../design/phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [database/schema-catalog.md §Table Domains](../../../../design/database/schema-catalog.md#table-domains)

### Scope Boundary (Part 2)

本部分定义层级与权限交互预期。DDL、索引策略与 SQL 实现细节以 database/phase 文档为准。

---

## Part 3: VM 生命周期流程

> **说明**: 本节描述 VM 的完整生命周期：创建请求 → 审批 → 执行 → 运行 → 删除
>
### Purpose

描述 VM 从提交申请到审批、执行、运行、删除的端到端交互预期。

### Actors & Trigger

- 触发：普通用户在 Service 作用域提交 VM 创建请求。
- 参与方：申请用户、平台审批管理员、异步 worker、provider 集成层。

### Interaction Flow

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
│  当前基线: 集群选择先做 capability 匹配，再执行显式 cluster policy 校验，然后才继续审批或执行。 │
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
│  │  │  ❌ 错误: 专用 CPU 与超卖不兼容！²                 👈 检测到冲突时显示 (红色错误)       │ │ │
│  │  │     VM 无法启动。审批已阻止。修复: 禁用超卖 或 取消专用 CPU。                     │ │ │
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
│  👆 验证逻辑:                                                                                  │
│     1. request ≠ limit 且环境为 prod → ⚠️ 黄色警告 (仅提示)                                     │
│     2. 超卖 + 专用 CPU 同时启用 → ❌ 错误 (阻止审批) ²                                          │
│        KubeVirt 要求 requests.cpu == limits.cpu 才能启用 dedicatedCpuPlacement (Guaranteed QoS)│
│                                                                                              │
│  ² **技术约束**: `dedicatedCpuPlacement` 要求 KubeVirt 使用 Guaranteed QoS 类，               │
│    意味着 CPU request 必须等于 limit。这是 K8s/KubeVirt 硬性约束，无法绕过。                       │
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
│     Template 启动盘定义 + InstanceSize.spec_overrides + 用户参数 (disk_gb)                    │
│                                                                                              │
│     启动盘渲染约定:                                                                           │
│     - `containerdisk`    -> `volumes[].containerDisk`                                        │
│     - `cdi_image_import` -> `dataVolumeTemplates` + `volumes[].dataVolume`                   │
│     - `cdi_pvc_clone`    -> `dataVolumeTemplates` + `volumes[].dataVolume`                   │
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

### 阶段 5.B 约束说明：专用 CPU 与超卖

- 硬约束：`dedicatedCpuPlacement` 要求 Guaranteed QoS，因此 CPU request 必须等于 CPU limit。
- 该校验在审批流中是阻断型错误（不是提示型警告）。
- 参考：
  [KubeVirt Compute resource requests and limits](https://kubevirt.io/user-guide/compute/resources_requests_and_limits/)

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

### State Transitions (阶段 5.A-5.C)

| 阶段 | 工单 | 领域事件 | VM | Worker Job |
|------|------|----------|----|------------|
| 5.A 提交 | 创建为 `PENDING` | 创建为 `PENDING` | 无 | 无 |
| 5.B 批准 | `PENDING -> APPROVED` | `PENDING -> PROCESSING` | 创建为 `CREATING` | 插入 |
| 5.B 拒绝 | `PENDING -> REJECTED` | `PENDING -> CANCELLED` | 无 | 无 |
| 5.C 执行 | 不变 | 随执行推进 | `CREATING -> RUNNING|FAILED` | 消费并完成 |

### Failure & Edge Cases (阶段 5.A-5.C)

- 同一操作的重复待审批提交必须在写入前拦截。
- 审批阶段集群能力不匹配必须阻断审批，不得调度 worker。
- 执行失败必须保留可审计轨迹并支持确定性重试。

### Authority Links (阶段 5.A-5.C)

- [ADR-0017 §Decision](../../../../adr/ADR-0017-vm-request-flow-clarification.md#decision)
- [ADR-0018 §User Interaction Flow](../../../../adr/ADR-0018-instance-size-abstraction.md#user-interaction-flow)
- [database/vm-lifecycle-write-model.md §Stage 5.A](../../../../design/database/vm-lifecycle-write-model.md#stage-5a-vm-request-submission-pending-approval)
- [frontend/FRONTEND.md §API Type Integration](../../../../design/frontend/FRONTEND.md#api-type-integration-adr-0021)

### Scope Boundary (阶段 5.A-5.C)

本节定义交互顺序与状态预期。SQL/DDL/迁移与 worker 实现细节以 database/phase 文档为准。

---

### 阶段 5.A：持久化摘要 {#stage-5-a}

#### Purpose

总结用户提交 VM 请求后的持久化意图，同时将实现细节下沉到数据库文档层。

#### Actors & Trigger

- 触发：用户提交 VM 创建请求。
- 参与方：申请人、审批工作流子系统、通知子系统。

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 阶段 5.A 持久化意图（提交写集）                                                                 │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  申请人提交 VM 请求                                                                           │
│        │                                                                                     │
│        ▼                                                                                     │
│  API 预检查（RBAC + 重复待审批拦截）                                                          │
│        │                                                                                     │
│        ▼                                                                                     │
│  单事务写入：                                                                                │
│    1) tickets: 以 operation_type=`CREATE` 创建 `PENDING`                                    │
│    2) domain_events: 创建 `PENDING`                                                          │
│        │                                                                                     │
│        ▼                                                                                     │
│  提交后：best-effort 审计 + 审批路由 + 通知触发                                              │
│        │                                                                                     │
│        ▼                                                                                     │
│  返回 `202 Accepted`，前端据此轮询工单状态                                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| 实体 | 之前 | 之后 |
|------|------|------|
| `tickets` | 无 | `PENDING` |
| `domain_events` | 无 | `PENDING` |
| `vms` | 无 | 无 |
| `river_job` | 无 | 无 |

#### Failure & Edge Cases

- 同类待审批请求重复提交必须返回冲突并给出已有工单引用。
- 事务内任一写入失败必须整体回滚。

#### Authority Links

- [database/vm-lifecycle-write-model.md §Stage 5.A](../../../../design/database/vm-lifecycle-write-model.md#stage-5a-vm-request-submission-pending-approval)
- [ADR-0009 §Constraint 1 DomainEvent Payload Immutability](../../../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only)
- [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode)

#### Scope Boundary

本节不定义 SQL 语句、索引和迁移细节。

### 阶段 5.B：持久化摘要 {#stage-5-b}

#### Purpose

总结管理员批准/拒绝路径下的写入结果与一致性保证。

#### Actors & Trigger

- 触发：管理员对待审批 VM 请求执行批准或拒绝。
- 参与方：审批人、事务编排层、River 调度器。

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 阶段 5.B 持久化意图（决策写集）                                                                 │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  审批人打开待审批工单                                                                          │
│        │                                                                                     │
│        ├── 批准路径                                                                            │
│        │      1) 工单：`PENDING -> APPROVED`                                                 │
│        │      2) 事件：`PENDING -> PROCESSING`                                               │
│        │      3) VM：插入并置为 `CREATING`                                                   │
│        │      4) River：入队执行任务                                                         │
│        │      5) 审计：记录批准动作                                                          │
│        │                                                                                     │
│        └── 拒绝路径                                                                            │
│               1) 工单：`PENDING -> REJECTED`                                                 │
│               2) 事件：`PENDING -> CANCELLED`                                                │
│               3) 不创建 VM / 不入队 River                                                    │
│               4) 审计：记录拒绝动作                                                          │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| 路径 | 工单 | 领域事件 | VM | River Job |
|------|------|----------|----|-----------|
| 批准 | `PENDING -> APPROVED` | `PENDING -> PROCESSING` | 创建并置为 `CREATING` | 插入（`available`） |
| 拒绝 | `PENDING -> REJECTED` | `PENDING -> CANCELLED` | 不创建 | 不插入 |

#### Failure & Edge Cases

- 批准路径必须保持 claim-check（River 仅携带 EventID 引用）。
- 拒绝路径不得创建 VM 记录或异步任务。

#### Authority Links

- [database/vm-lifecycle-write-model.md §Stage 5.B](../../../../design/database/vm-lifecycle-write-model.md#stage-5b-admin-approval-rejection)
- [ADR-0006 §Decision](../../../../adr/ADR-0006-unified-async-model.md#decision)
- [ADR-0009 §Constraint 1 DomainEvent Payload Immutability](../../../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only)
- [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode)

#### Scope Boundary

本节仅定义状态结果与事务保证，不展开 SQL/DDL。

### 阶段 5.D: 删除操作 {#stage-5-d}

#### Purpose

定义 VM/Service/System 删除的交互行为与状态预期。

#### Actors & Trigger

- 触发：用户或管理员发起删除 API，并携带必需确认参数。
- 参与方：请求方、审批工作流（仅 VM）、异步 worker、审计子系统。

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 删除用户旅程（交互意图）                                                                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  资源详情页（VM / Service / System）                                                           │
│        │                                                                                     │
│        ▼                                                                                     │
│  点击删除 -> UI 二次确认（`confirm=true` 或 `confirm_name`）                                 │
│        │                                                                                     │
│        ▼                                                                                     │
│  API 校验：RBAC + 级联前置条件 + 环境策略                                                      │
│        │                                                                                     │
│        ├── VM 路径：创建删除审批工单 -> 审批决策                                                │
│        │                                                                                     │
│        └── Service/System 路径：无删除审批工单                                                 │
│        │                                                                                     │
│        ▼                                                                                     │
│  执行路径可短暂进入 `DELETING`，完成清理后主表硬删除                                           │
│  （审计日志/审批记录/领域事件按保留策略独立留存）                                              │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

实体规则矩阵：

| 实体 | 前置条件 | 审批 | 确认方式 | 主表行为 |
|------|------|------|------|------|
| VM（测试） | 无 | ✅ 需要 | `confirm=true` | `DELETING`（短暂）-> 硬删除 |
| VM（生产） | 无 | ✅ 需要 | `confirm_name` | `DELETING`（短暂）-> 硬删除 |
| Service | 子 VM 必须为 0 | ❌ 不需要 | `confirm=true` | `DELETING`（短暂）-> 硬删除 |
| System | 子 Service 必须为 0 | ❌ 不需要 | `confirm_name` | 直接硬删除 |

#### State Transitions

| 流程 | 工单 | 资源 | 最终持久化结果 |
|------|------|------|----------------|
| VM 删除（审批通过） | `PENDING -> APPROVED` | `RUNNING/STOPPED -> DELETING -> (主表删除)` | VM 主记录硬删除，审计/工单/事件独立保留 |
| Service 删除 | 无工单 | `ACTIVE -> DELETING -> (主表删除)` | 清理完成后硬删除 Service 主记录 |
| System 删除 | 无工单 | `ACTIVE -> (主表删除)` | 校验通过后事务内硬删除 |

#### Failure & Edge Cases

- 级联前置条件不满足必须阻断删除。
- 确认参数不匹配必须在写入前失败。
- 进入 `DELETING` 后若 worker 失败，必须可重试且可审计。

#### Authority Links

- [ADR-0015 §13 Deletion Cascade Constraints](../../../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0015 §13.1 Confirmation Mechanism](../../../../adr/ADR-0015-governance-model-v2.md#131-delete-confirmation-mechanism)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../../../../design/phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)
- [database/lifecycle-retention.md §Retention Classes](../../../../design/database/lifecycle-retention.md#retention-classes-table-centric)
- [database/vm-lifecycle-write-model.md §Stage 5.D](../../../../design/database/vm-lifecycle-write-model.md#stage-5d-delete-write-model)

#### Scope Boundary

本节只定义删除交互意图和结果，不展开 Schema/DDL/归档作业细节。

> **删除动作命名规范**:
> - V1 规范动作：`*.delete_submitted`、`*.delete_approved`（若适用）、`*.delete_executed`。
> - `*.delete_request` / `*.delete` 属于旧命名，只允许出现在历史描述中；新增设计内容必须使用上述规范命名。

---

### 阶段 5.E: 批量操作

#### Purpose

定义父子工单模型下的规范批量提交流程、执行语义与前端可见行为，包括两层限流、稳定幂等键、有限逻辑尝试，以及结果不明确的 VM 重启必须失败关闭。

#### Actors & Trigger

- 触发：用户/管理员提交包含多个子项的批量操作。
- 参与方：前端队列 UI、API 层、治理事务层、River worker。

#### Interaction Flow

UI 故事板（父子队列）：

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 批量队列 UI 故事板                                                                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  [批量操作页面]                                                                                  │
│     选择 VM + 选择操作 + 提交                                                                     │
│                                  │                                                               │
│                                  ▼                                                               │
│  [队列列表页]                                                                                     │
│     新父工单出现：`PENDING_APPROVAL`                                                             │
│     列表展示：操作 + 总数/成功/失败/待处理 + 创建时间                                             │
│                                  │                                                               │
│                                  ▼                                                               │
│  [展开父工单]                                                                                     │
│     详情补充：申请人 + 更新时间；子任务展示：单项状态 + attempt_count + last_error                 │
│                                  │                                                               │
│                                  ▼                                                               │
│  [进行中/终态处理]                                                                                │
│     `IN_PROGRESS`      -> 操作：终止未开始子项                                                    │
│     `PARTIAL_SUCCESS`  -> 操作：重试失败子项                                                      │
│     `FAILED`           -> 操作：重试失败子项                                                      │
│     `COMPLETED`        -> 操作：导出结果                                                          │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 批量提交流程（规范）                                                                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. 用户/管理员在 UI 选择批量项                                                                   │
│  2. 前端: POST /vms/batch                                                                         │
│     └── 提交稳定的 request_id 与操作载荷                                                          │
│  3. 后端在业务事务中按规范顺序获取全局、用户和请求幂等守卫：                                        │
│     • 获取锁后先检查 request_id 重放，再读取当前配额与冷却快照                                     │
│     • Layer 1（全局）: 父工单积压阈值 + API 速率                                                   │
│     • Layer 2（用户）: 用户父/子工单阈值 + 提交冷却时间                                             │
│  4. 同一原子事务:                                                                                  │
│     • 插入父工单                                                                                  │
│     • 插入全部子工单                                                                              │
│     • 任一子工单失败则整体回滚                                                                      │
│  5. 返回 202: {batch_id, status, status_url, retry_after_seconds}                                │
│  6. 前端轮询: GET /vms/batch/{batch_id}                                                          │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 批量执行流程                                                                                      │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. 审批事务将原始父 Ticket/Event 置为 EXECUTING/PROCESSING，公开投影置为 IN_PROGRESS              │
│  2. Worker 独立消费子任务；首次分派记录为第 1 次逻辑尝试                                          │
│  3. 聚合终态计算:                                                                                 │
│     • COMPLETED: 全部成功                                                                         │
│     • FAILED: 无成功子项的其他终态组合                                                             │
│     • PARTIAL_SUCCESS: 部分成功                                                                    │
│     • CANCELLED: 所有子任务均被终止                                                                │
│  4. 前端可操作:                                                                                   │
│     • POST /vms/batch/{batch_id}/retry  （仅重试未耗尽三次逻辑尝试的执行失败子项）                  │
│     • POST /vms/batch/{batch_id}/cancel （终止待执行子项）                                        │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 兼容接口说明                                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  规范入口：POST /vms/batch                                                                         │
│  兼容保留：POST /vms/batch/power                                                                  │
│                                                                                                  │
│  内部统一归一到同一套父子工单流水线。                                                               │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| 范围 | 状态变化模式 |
|------|-------------|
| 原始工作流父 Ticket | `PENDING -> EXECUTING -> SUCCESS|FAILED|CANCELLED` |
| 原始工作流父 Event | `PENDING -> PROCESSING -> COMPLETED|FAILED|CANCELLED` |
| 批量 API/公开投影 | `PENDING_APPROVAL -> IN_PROGRESS -> COMPLETED|PARTIAL_SUCCESS|FAILED|CANCELLED`；公开父批次状态绝不包含 `APPROVED`。 |
| 已批准子工单执行分支 | `PENDING -> APPROVED -> EXECUTING -> SUCCESS|FAILED` |
| 子工单拒绝/终止分支 | `PENDING -> REJECTED|CANCELLED`，两者均为该子工单的终态 |
| 显式子工单重试 | 仅当 `attempt_count < 3` 时允许 `FAILED -> PENDING/EXECUTING`；`REJECTED` 不得重新打开 |
| 重启分派围栏 | Event 的 `PENDING -> PROCESSING` 是单向提供方分派声明；原 worker 持久化终态前不得清除或重新派发 |

提交与重试不变量：

- 同一用户意图在超时、网络错误、`5xx`、`429` 或 `409` 后重试时必须复用同一个不透明 `request_id`；只有请求成功或用户明确开始新的意图时才能轮换。
- `request_id` 按申请人和请求操作划分作用域；电源 `START`、`STOP`、`RESTART` 使用互相独立的作用域，重放返回首次接受的父批次。
- `VMPowerPayload.dispatch_mode` 是不可变来源字段，仅允许 `direct|ticket`。启动任何会自动注册 worker 的新版本实例前，维护窗必须冻结旧版本全部 direct、普通/外部审批和 batch 电源生产路径。旧 worker 独占排空 legacy job；admission 报告必须同时证明缺失/非法 mode 的未解决 `PENDING` Event（无论有无 Ticket）为零，并且引用此类 payload 的可运行 `vm_power` job 为零。剩余 `PENDING` 请求须通过审核的正常应用流程终止，且只能由新版本重新提交。无 Ticket 且无旧 worker 可运行 job 的 direct orphan `PENDING` Event 在当前版本没有安全转换路径，因此是升级硬阻断项，不能作为隔离项放行。无法证明收敛的 `PROCESSING` START/STOP 以及所有 `PROCESSING` RESTART 必须连同证据隔离；绝不回填不可变 payload，也绝不重放或重新派发操作。完整 admission 流程以[数据库迁移指南](../../../../design/database/migrations.md#batch-retry-and-idempotency-rollout)为准。
- `attempt_count` 统计逻辑子任务分派而非 River 内部投递；首次分派计为 1，每个子项最多允许三次逻辑尝试。
- 审批为 `REJECTED` 的子工单是终态，绝不能进入执行或重试。
- 重启 worker 必须在调用提供方之前原子声明 `PENDING -> PROCESSING`。超时、响应丢失、worker 救援或分派后持久化失败都不能证明提供方未接受请求，因此必须保留 `PROCESSING/EXECUTING` 围栏并失败关闭。
- 歧义冲突返回 `operator_action_required=true`、既有 Event ID 和 `operator-runbook:ambiguous-vm-restart`；前端仅展示该只读证据并指引运维人员独立核验 KubeVirt 状态。
- 任何公开或管理 API 都不能清除或重新派发结果不明确的重启围栏。在提供方回执、幂等或可证明取消协议落地前，普通重试端点也不能重新打开该尝试。

#### Failure & Edge Cases

- 全局或用户级限流拒绝必须通过 `Retry-After` 和 `retry_after_seconds` 返回可执行重试窗口。
- 子任务失败不得回滚已成功子任务。
- 重试/终止操作仅作用于符合条件子任务，并重算父工单聚合状态。
- 并发重试/终止若丢失条件状态转换，必须返回可执行冲突且不得覆盖更新后的子任务状态。

#### Authority Links

- [ADR-0015 §19 Batch Operations V1](../../../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)
- [04-governance.md §5.6 Batch Operations](../../../../design/phases/04-governance.md#56-batch-operations-adr-0015-19)
- [database/vm-lifecycle-write-model.md §Stage 5.E](../../../../design/database/vm-lifecycle-write-model.md#stage-5e-batch-parent-child-write-model)
- [frontend/features/batch-operations-queue.md §2.0 End-to-End UI Storyboard](../../../../design/frontend/features/batch-operations-queue.md#20-end-to-end-ui-storyboard)

#### Scope Boundary

本节仅定义交互行为与状态语义，队列内部实现、表结构和 worker 调优细节以 phase/database 文档为准。

---

### 阶段 5.F: 通知系统

#### Purpose

定义审批与 VM 生命周期事件的用户可见通知行为。

#### Actors & Trigger

- 触发：审批流程事件和 VM 状态变更。
- 参与方：事务编排层、站内信通知服务、前端轮询界面。

#### Interaction Flow

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 通知触发点                                                                                        │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  事件: VM 请求已提交                                                                              │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, body, metadata)               │        │
│  │ SELECT user_id, 'APPROVAL_PENDING', '新 VM 请求待审批',                               │        │
│  │        '用户 X 在命名空间 Y 提交了一个请求',                                            │        │
│  │        '{"ticket_id": "TKT-001", "requester": "user-a"}'                             │        │
│  │ FROM role_bindings                                                                   │        │
│  │ WHERE role_id IN (SELECT id FROM roles WHERE permissions @> 'builtin_approval:approve'); │  │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
│  事件: 请求已批准/拒绝                                                                            │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, metadata)                     │        │
│  │ VALUES (ticket.requested_by, 'APPROVAL_COMPLETED',                                  │        │
│  │         '您的 VM 请求已批准', '{"ticket_id": "TKT-001"}');                            │        │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
│  事件: VM 状态变更                                                                                │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, metadata)                     │        │
│  │ VALUES (vm.owner_id, 'VM_STATUS_CHANGE',                                            │        │
│  │         'VM vm-name-01 现已运行中', '{"vm_id": "...", "new_state": "Running"}');     │        │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 用户通知交互                                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  前端顶栏:                                                                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐                        │
│  │  🔔 (3)  ← 徽章显示未读数量                                          │                        │
│  │    ↓ 每 30s 轮询: GET /api/v1/notifications/unread-count            │                        │
│  └─────────────────────────────────────────────────────────────────────┘                        │
│                                                                                                  │
│  点击通知铃铛 → 下拉面板:                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐                        │
│  │  GET /api/v1/notifications?page=1&per_page=10                       │                        │
│  │                                                                     │                        │
│  │  • 🔵 新 VM 请求待处理 (2 分钟前)                                    │                        │
│  │  • 🔵 您的请求已批准 (1 小时前)                                      │                        │
│  │  • VM shop-redis-01 现已运行中 (3 小时前)                            │                        │
│  │                                                                     │                        │
│  │  [全部标为已读]  [查看全部 →]                                         │                        │
│  └─────────────────────────────────────────────────────────────────────┘                        │
│                                                                                                  │
│  标记已读: PATCH /api/v1/notifications/{notification_id}/read                                   │
│  全部已读: POST /api/v1/notifications/mark-all-read                                             │
│                                                                                                  │
│  ⚠️ V1 限制: 仅支持轮询，不支持 WebSocket 推送                                                     │
│  ⚠️ V1 限制: 不支持外部渠道（邮件/Webhook）；V2+ 规划见下方链接                                    │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| 事件类型 | 交付预期 |
|------|----------|
| 需要审批 | 工单提交后立即通知审批人 |
| 审批结果 | 批准/拒绝后通知申请人 |
| 运行态变更 | VM 状态变化后通知资源拥有者 |

#### Failure & Edge Cases

- 通知写入失败不得静默丢弃，必须可观测。
- V1 仅轮询，前端需容忍最终一致性延迟。
- 通知 payload 的敏感信息必须遵循脱敏策略。

#### Authority Links

- [ADR-0015 §20 Notification System](../../../../adr/ADR-0015-governance-model-v2.md#20-notification-system)
- [04-governance.md §6.3 Notification System](../../../../design/phases/04-governance.md#63-notification-system-adr-0015-20)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)
- [RFC-0018 §Proposed Solution](../../../../rfc/RFC-0018-external-notification.md#proposed-solution)

#### Scope Boundary

本节定义用户可见通知行为；渠道适配器、重试策略和供应商集成细节在治理文档与 RFC 中定义。

---

## Part 4: 状态机与数据模型

> **说明**: 本节定义系统中核心实体的状态机和数据库表关系，是前后端开发的重要参考。

### Purpose

为前后端和运维提供统一状态语义与共享数据模型意图，避免跨层理解偏差。

### Actors & Trigger

- 触发：工程实现或评审需要统一解释工单/VM/审计状态。
- 参与方：后端工程师、前端工程师、SRE/运维评审人员。

### Interaction Flow

Part 4 属于参考视图，不是用户操作流程。
它汇总实体状态、关系语义与审计规则，供其它流程章节复用。

### 审批工单状态流转图

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                            审批工单 Ticket 状态流转                                            │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│                        ┌───────────────────┐                                                 │
│                        │      PENDING      │                                                 │
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
│                     (由 worker 执行硬删除，主资源表不保留 DELETED 持久状态)                   │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

兼容性说明：
- API 响应中可能出现额外运行态：`PENDING`、`MIGRATING`、`PAUSED`、`UNKNOWN`。
  这些用于兼容运行时/Provider 状态，不改变本节的规范主流程语义，前端需安全渲染。

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
│  │ role_bindings│                               │     tickets      │                        │
│  │──────────────│                               │──────────────────│                        │
│  │ user_id      │                               │ id               │                        │
│  │ role         │                               │ type             │                        │
│  │ resource_type│                               │ status           │                        │
│  │ resource_id  │                               │ event_id         │                        │
│  └──────────────┘                               │ operation_type   │                        │
│                                                 │ requester        │                        │
│                                                 │ approver         │                        │
│                                                 │ selected_cluster │                        │
│                                                 │ snapshot fields  │ ← 审批时确定的最终值    │
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

> **边界说明**: 本节仅定义审计语义。
> Schema/DDL/索引以以下文档为权威来源：
> - [04-governance.md §7](../../../../design/phases/04-governance.md#7-audit-logging)
> - [database/schema-catalog.md §Table Domains](../../../../design/database/schema-catalog.md#table-domains)
> - [database/lifecycle-retention.md §Retention Classes](../../../../design/database/lifecycle-retention.md#retention-classes-table-centric)

#### 必须覆盖范围

- 所有状态变化操作（CREATE/UPDATE/DELETE）
- 敏感读操作（例如 `vnc.access`）
- 提交/审批/执行链路中的成功与失败路径

#### 规范动作命名

| 领域 | V1 规范动作 | 说明 |
|------|------|------|
| 认证 | `user.login`, `user.login_failed`, `user.logout` | 认证事件 |
| System | `system.create`, `system.update`, `system.delete_submitted`, `system.delete_executed` | System 删除无审批工单 |
| Service | `service.create`, `service.delete_submitted`, `service.delete_executed` | Service 删除无审批工单 |
| VM | `vm.request`, `vm.create`, `vm.start`, `vm.stop`, `vm.restart`, `vm.delete_submitted`, `vm.delete_approved`, `vm.delete_executed` | VM 删除需审批 |
| VNC | `vnc.access` | 敏感读 |
| Approval | `approval.approve`, `approval.reject`, `approval.cancel` | 工单决策 |
| RBAC | `role.create`, `role.update`, `role.delete`, `role.assign`, `role.revoke`, `permission.create`, `permission.delete` | 权限治理 |
| Cluster | `cluster.register`, `cluster.update`, `cluster.delete`, `cluster.credential_rotate` | 集群生命周期 |
| Template | `template.create`, `template.update`, `template.deprecate`, `template.delete` | 模板生命周期 |
| InstanceSize | `instance_size.create`, `instance_size.update`, `instance_size.deprecate`, `instance_size.delete` | 规格生命周期 |
| Namespace | `namespace.create`, `namespace.delete` | 命名空间生命周期 |
| Auth Provider | `auth_provider.configure`, `auth_provider.update`, `auth_provider.delete`, `auth_provider.sync`, `auth_provider.mapping_create`, `auth_provider.mapping_update`, `auth_provider.mapping_delete` | ADR-0015 修订：使用 `auth_provider.*`，不再用 `idp.*` |
| Config | `config.update` | 平台配置变更 |

#### 每条审计记录必备字段

- `action`、`actor_id`、`resource_type`、`resource_id`、`created_at`
- 可选但建议：`parent_type`、`parent_id`、`environment`
- `details` 必须按 ADR-0019 脱敏后落库

#### 常见可豁免审计的操作

| 类别 | 操作 | 原因 |
|------|------|------|
| 系统巡检 | 集群健康轮询、VM 状态轮询 | 高频且无直接用户意图 |
| 只读查询 | 列表/详情 `GET` API | 不改变状态 |
| 内部流量 | Worker 心跳、指标采集 | 内部可观测流量 |

> **豁免原则**:
> - 写操作默认必须审计。
> - 豁免必须显式定义并经过评审。
> - 敏感读即使不改状态也应审计。

#### 保留策略基线

| 环境 | 保留时间 | 说明 |
|------|------|------|
| 生产环境 | >= 1 年 | 合规基线 |
| 测试环境 | >= 90 天 | 可按策略缩短 |
| 敏感操作 | >= 3 年 | `*.delete*`、`approval.*`、`rbac.*` |

---

### 审计日志 JSON 导出 (v1+)

> **场景**: 将审计日志集成到企业级 SIEM 系统（Elasticsearch、Datadog、Splunk 等）

> 📦 **API 规范**: 完整 API 和响应格式见 [04-governance.md §7 JSON Export API](../../../../design/phases/04-governance.md#7-json-export-api)

**主要功能**:
- 支持时间范围过滤的分页导出
- Webhook 推送集成，实现实时流式传输
- 结构化 JSON 格式，兼容主流日志聚合器

---

<a id="external-approval-v2-roadmap"></a>

### 审批 Provider 插件化架构 (V2+ 路线图)

> **场景**: 与企业现有 ITSM 系统（Jira Service Management、ServiceNow 等）集成。
>
> **V1 边界**: V1 基于统一审批 Provider 契约，仅实现一个内置 Provider（`builtin-default`）。
> 外部系统作为插件适配器在 V2+ 引入。

#### 设计原则

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         审批 Provider 插件化架构                                               │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐   │
│  │   Shepherd   │  ──── Webhook ───▶ │  外部系统    │  ──── Callback ──▶ │   Shepherd   │   │
│  │   Platform   │                    │ (Jira/SNOW)  │                    │   Platform   │   │
│  └──────────────┘                    └──────────────┘                    └──────────────┘   │
│                                                                                              │
│  关键原则:                                                                                    │
│  1. Shepherd 负责统一工单状态机与审计轨迹                                                      │
│  2. 内置/外部 Provider 共享稳定契约，避免核心流程分叉                                           │
│  3. 异步集成 + 失败安全回退，外部故障不得阻塞内置主路径                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### 审批 Provider 配置（外部适配器，Web UI）

> 管理员在 **设置 → 外部审批系统 → 添加** 进行配置。
> 外部适配器注册信息存储于 `external_approval_systems`。

**Webhook 安全最佳实践**：
- 所有 webhook URL 必须使用 HTTPS。
- 使用共享密钥进行签名校验，并采用常量时间比较。
- 在签名载荷中包含时间戳，拒绝过期请求，防止重放。
- webhook 密钥需加密存储，泄露时及时轮换。

参考：
- https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
- https://docs.stripe.com/webhooks/test

关键落库对象（字段权威定义在 phase/database 文档）：

| 对象 | 代表字段 | 作用 |
|----------|------|------|
| `external_approval_systems` | `id`, `name`, `provider_type`, `enabled`, `webhook_url`, `signing_key_ciphertext`, `timeout_seconds`, `retry_count`, `retry_backoff_seconds` | 外部适配器注册与投递保护策略 |
| `audit_logs` | `action`, `resource_type`, `resource_id`, `result`, `metadata` | 外部决策/回退动作的本地不可变审计轨迹 |

#### Webhook 发送格式 (Shepherd → 外部系统)

```json
// POST https://jira.company.com/api/v2/tickets
{
  "shepherd_ticket_id": "ticket-001",
  "type": "VM_CREATE",
  "callback_url": "https://shepherd.company.com/api/v1/webhooks/approval-callback",
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
// POST https://shepherd.company.com/api/v1/webhooks/approval-callback
// Headers:
//   X-External-Approval-System-ID: external-approval-001
//   X-Signature-256: sha256=<原始请求体的十六进制 HMAC-SHA256 签名>
//   X-Ticket-ID: ticket-001
//   Content-Type: application/json
{
  "ticket_id": "ticket-001",
  "approved": true,
  "approver": "admin.li",
  "provider_decision_id": "JIRA-12345",
  "decided_at": "2026-01-26T11:30:00Z",
  "execution": {
    "selected_cluster_id": "cluster-prod-a",
    "selected_storage_class": "rook-ceph",
    "selected_dv_access_modes": ["ReadWriteOnce"],
    "selected_dv_volume_mode": "Filesystem"
  }
}
```

#### Shepherd 处理回调

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         Callback 处理流程                                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  1. 验证 HMAC 签名                                                                           │
│  2. 根据 Header ID 加载启用的外部审批系统                                                    │
│  3. 从请求体解析 ticket_id / approved / approver                                             │
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
| **状态同步** | 原生连接器的 provider-specific 状态同步属于后续范围 |
| **超时处理** | V1: 不自动取消。超时后外部系统可调用拒绝 API（参见 [ADR-0015 §11](../../../../adr/ADR-0015-governance-model-v2.md#11-approval-timeout-handling)） |
| **安全性** | 始终验证 HMAC 签名，防止伪造回调 |
| **回退机制** | 外部系统不可用时，自动回退到内置审批 |

### State Transitions (Part 4 参考)

| 领域 | 规范状态集合 |
|------|-------------|
| 审批工单 | `PENDING`, `APPROVED`, `REJECTED`, `CANCELLED`, `EXECUTING`, `SUCCESS`, `FAILED` |
| VM 运行态 | `CREATING`, `RUNNING`, `STOPPING`, `STOPPED`, `FAILED`, `DELETING`, `PENDING`, `MIGRATING`, `PAUSED`, `UNKNOWN` |
| 审计记录生命周期 | 仅追加写入，按策略保留/归档 |

### Failure & Edge Cases (Part 4 参考)

- API/UI/worker 的状态机语义漂移属于设计缺陷，禁止发生。
- 新增终态必须同时更新 flow、governance、API 合同文档。
- 审计脱敏违规属于安全事件，而非普通文档问题。

### Authority Links (Part 4)

- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)
- [database/schema-catalog.md §Relationship Baseline](../../../../design/database/schema-catalog.md#relationship-baseline)
- [database/lifecycle-retention.md §Database Guardrails](../../../../design/database/lifecycle-retention.md#database-guardrails)
- [ADR-0015 §11 Approval Timeout Handling](../../../../adr/ADR-0015-governance-model-v2.md#11-approval-timeout-handling)

### Scope Boundary (Part 4)

本部分定义语义模型与跨组件约束，不替代 DDL、API 契约或 worker 实现规范文档。

---

## 阶段 6: VNC 控制台访问

### Purpose

定义测试/生产环境下的安全控制台访问交互行为。

### Actors & Trigger

- 触发：用户在 VM 详情页请求控制台访问。
- 参与方：请求用户、RBAC 校验层、审批流程（生产环境）、Token 签发器、VNC 代理层。

### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 阶段 6 控制台访问总览                                                                            │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  VM 详情页 -> 用户点击“控制台”或“申请控制台访问”                                                │
│        │                                                                                     │
│        ▼                                                                                     │
│  后端统一校验：RBAC（`vnc:access`）+ VM 状态（`RUNNING`）                                     │
│        │                                                                                     │
│        ├── 测试环境：签发 token -> 直接打开 noVNC                                              │
│        │                                                                                     │
│        └── 生产环境：创建审批工单 -> 管理员批准/拒绝                                            │
│                 ├── 批准：签发 token -> 打开 noVNC                                             │
│                 └── 拒绝：无控制台会话                                                         │
│                                                                                              │
│  两条路径都必须落审计（请求与访问结果）                                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

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
│  │       "exp": now + 2h,             👈 有效期                                              │
│  │       "jti": "vnc-token-789",      👈 唯一 ID 用于审计                                   │
│  │       "single_use": true           👈 首次连接后失效                                     │
│  │     }                                                                                    │
│  │                                                                                          │
│  │  4. 通过安全引导通道在新标签页/弹窗打开 noVNC:                                            │
│  │     Set-Cookie: vnc_bootstrap=<opaque>; HttpOnly; Secure; SameSite=Strict; Max-Age=60   │
│  │     GET /api/v1/vms/{vm_id}/vnc                                                          │
│  │     （URL 查询参数中不允许携带 bearer token）                                             │
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
│  │     INSERT INTO tickets (event_id, operation_type, status, requester, ...)              │
│  │     VALUES ('event-123', 'VNC_ACCESS', 'PENDING', 'user-123', ...)                      │
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

### State Transitions

| 环境 | 工单 | 访问结果 |
|------|------|---------|
| 测试环境 | 无审批工单 | RBAC 通过 -> 签发 token -> 建立会话 |
| 生产环境 | 原始 Ticket `PENDING -> APPROVED/REJECTED` | 批准后签发 token；拒绝则无控制台访问 |

### Failure & Edge Cases

- VM 非 `RUNNING` 状态必须阻断 token 签发。
- 生产环境重复待审批请求必须幂等拒绝。
- token 首次成功连接后再次重放必须拒绝并记录审计。

### Authority Links

- [ADR-0015 §18 VNC Console Access](../../../../adr/ADR-0015-governance-model-v2.md#18-vnc-console-access-permissions)
- [RFC-0011 §V1 Implementation Scope](../../../../rfc/RFC-0011-vnc-console.md#v1-implementation-scope)
- [database/vm-lifecycle-write-model.md §Stage 6](../../../../design/database/vm-lifecycle-write-model.md#stage-6-vnc-access-write-model)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)

### Scope Boundary

本节定义交互流程与 token 策略，不展开 WebSocket 代理内部实现和存储细节。

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
→ 响应: { "vnc_url": "/api/v1/vms/{vm_id}/vnc", "bootstrap": "set-cookie" }  (测试)

# VNC WebSocket 端点
GET /api/v1/vms/{vm_id}/vnc
Upgrade: websocket
Cookie: vnc_bootstrap=<一次性凭据>
→ 代理到 KubeVirt VNC 子资源

# 检查控制台访问状态（轮询用）
GET /api/v1/vms/{vm_id}/console/status
→ 响应: { "status": "APPROVED", "vnc_url": "..." } | { "status": "PENDING" }
```

### 数据库操作

| 环境 | 持久化行为 |
|------|------------|
| 测试环境 | 不创建审批工单；访问行为必须写审计。 |
| 生产环境 | 先创建 `VNC_ACCESS_REQUESTED` 审批工单，审批通过后签发 token 并追加审计记录。 |

实现细节和写集边界以以下文档为准：

- [database/vm-lifecycle-write-model.md §Stage 6](../../../../design/database/vm-lifecycle-write-model.md#stage-6-vnc-access-write-model)
- [04-governance.md §7 Audit Logging](../../../../design/phases/04-governance.md#7-audit-logging)

---
