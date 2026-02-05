# Master Interaction Flow

> **Status**: Stable (ADR-0017, ADR-0018 Accepted)  
> **Version**: 1.1  
> **Created**: 2026-01-28  
> **Last Updated**: 2026-02-05  
> **Language**: English (Canonical Version)  
> **Source**: Extracted from ADR-0018 Appendix
>
> 🌐 **Other Languages**: [中文版](../../i18n/zh-CN/design/interaction-flows/master-flow.md)

---

## Document Purpose

This document is the canonical reference for all Shepherd platform interaction
flows, serving as the **single source of truth** for frontend, backend, and
database development.

## Document Scope

| In Scope | Out of Scope |
|----------|--------------|
| User interaction sequences | Database DDL/Schema definitions |
| Data flow and sources | Detailed API specifications |
| Conceptual state diagrams | Implementation code examples |
| Business rules summary | Low-level technical constraints |

> **Cross-Reference Pattern**: Operations involving data persistence include
> conceptual overview here, with implementation details documented in Phase
> design documents.
>
> Example: "Audit logs are created for all operations. See [04-governance.md §7](../phases/04-governance.md#7-audit-logging) for schema details."

### Document Hierarchy (Prevents Content Drift)

| Document | Authority | Scope |
|----------|-----------|-------|
| **ADRs** | Decisions (immutable after acceptance) | Architecture decisions and rationale |
| **This document (master-flow.md)** | Interaction principles (single source of truth) | Data sources, flow rationale, user journeys |
| **Phase docs** | Implementation details | Code patterns, schemas, API design |
| **[CHECKLIST.md](../CHECKLIST.md)** | ADR constraints reference | Centralized ADR enforcement rules |

> **Writing Guideline**: This document describes "what data" and "why it flows this way".
> For "how to implement", link to Phase documents instead of duplicating content.
> Example: "For InstanceSize schema details, see [01-contracts.md §InstanceSize](../phases/01-contracts.md#instancesize-schema)."

**Related Documents**:
- [ADR-0018: Instance Size Abstraction](../../adr/ADR-0018-instance-size-abstraction.md)
- [ADR-0015: Governance Model V2](../../adr/ADR-0015-governance-model-v2.md)
- [ADR-0017: VM Request Flow](../../adr/ADR-0017-vm-request-flow-clarification.md)
- [Phase 01: Contracts](../phases/01-contracts.md) — Data contracts and naming constraints
- [Phase 04: Governance](../phases/04-governance.md) — RBAC, audit logging, approval workflows

**Critical ADR Constraints (Applies to ALL flows in this document)**:

| ADR | Constraint | Scope |
|-----|------------|-------|
| **ADR-0006** | All write operations use **unified async model** (request → 202 → River Queue) | All state-changing operations |
| **ADR-0009** | River Jobs carry **EventID only** (Claim Check); DomainEvent payload is **immutable** | All River Jobs |
| **ADR-0012** | Atomic transactions: Ent for ORM, **sqlc for core transactions only** | All DB operations |

> **CI Enforcement**: These constraints are enforced by CI checks. See [CONTRIBUTING.md](../../../CONTRIBUTING.md) for validation scripts.

---

## Appendix: Canonical Interaction Flow (English)

### Document Structure

| Part | Content | Roles Involved |
|------|---------|----------------|
| **Part 1** | Platform initialization (Schema/Mask, **First Deployment Bootstrap**, RBAC/permissions, OIDC/LDAP auth, IdP group mapping, **External Approval Systems**, Cluster/InstanceSize/Template configuration) | Developer, Platform Admin |
| **Part 2** | Resource management (System/Service create/delete and DB operations, **including audit logs**) | Regular User |
| **Part 3** | VM lifecycle (Create request → Approve → Execute → Delete and DB operations, **including audit logs**) | Regular User, Platform Admin |
| **Part 4** | State machines and data models (state transitions, table relationships, **audit log design and exceptions**) | All Developers |

---

### Core Design Principles

| Principle | Description |
|----------|-------------|
| **Schema as Single Source of Truth** | KubeVirt official JSON Schema defines all field types, constraints, and enum options. We do not duplicate these in code. |
| **Mask Only Selects Paths** | Mask only selects which Schema paths to expose. It does not define field options. |
| **Hybrid Model** | Core scheduling fields (CPU, memory, GPU) stored in indexed columns for query performance; `spec_overrides` JSONB stores remaining fields without semantic interpretation. See ADR-0018 §4. |
| **Schema-Driven Frontend** | Frontend renders UI components based on Schema types. See ADR-0020 for technology stack (React 19, Next.js 15, Ant Design 5). |

### Role Definitions

| Role | Responsibility | Layer |
|------|----------------|-------|
| **Developer** | Fetch KubeVirt Schema, define Mask (select exposed paths) | Code/config layer |
| **Platform Admin** | Create InstanceSize (fill values via schema-driven form) | Admin console |
| **Regular User** | Choose InstanceSize and submit VM create request | Business usage layer |

### Naming Policy (ADR-0019 Baseline)

> **Security Baseline**: All platform-managed logical names MUST follow RFC 1035-based rules.

| Rule | Constraint |
|------|------------|
| **Character Set** | Lowercase letters, digits, hyphen only (`a-z`, `0-9`, `-`) |
| **Start Character** | MUST start with a letter (`a-z`) |
| **End Character** | MUST end with a letter or digit |
| **Consecutive Hyphens** | MUST NOT contain `--` (reserved for Punycode) |
| **Length** | System/Service/Namespace: max 15 chars each (ADR-0015 §16) |
| **Reserved Names** | `default`, `system`, `admin`, `root`, `internal`, prefixes `kube-*`, `kubevirt-shepherd-*`. See [01-contracts.md §1.1](../phases/01-contracts.md#11-naming-constraints-adr-0019). |

**Applies to**: System name, Service name, Namespace name, VM name components.

### API Design Principles (ADR-0021, ADR-0023)

| Principle | Description |
|-----------|-------------|
| **Contract-First** | OpenAPI 3.1 spec is the single source of truth. See ADR-0021. |
| **Code Generation** | Go server types via `oapi-codegen`; TypeScript types via `openapi-typescript`. |
| **Pagination** | List APIs use standardized pagination (`page`, `per_page`, `sort_by`, `sort_order`). See ADR-0023. |
| **Error Codes** | Granular error codes (e.g., `NAMESPACE_PERMISSION_DENIED`). See ADR-0023 §3. |

> **Full API Contract Governance**: For OpenAPI 3.1 vs 3.0 compatibility, CI toolchain constraints, and spec-code sync enforcement, see [01-contracts.md §API Contract-First Design](../phases/01-contracts.md#api-contract-first-design-adr-0021).
>
> **Capability Detection**: For Dry Run Fallback strategy when static capability detection is insufficient, see [02-providers.md §Dry Run Fallback](../phases/02-providers.md#dry-run-fallback-adr-0014).

### Schema Cache Lifecycle (ADR-0023)

> **Purpose**: KubeVirt Schema caching enables offline validation, multi-version compatibility, and frontend performance.

| Stage | Trigger | Action |
|-------|---------|--------|
| **1. Startup** | Application boot | Load embedded schemas (bundled at compile time) |
| **2. Cluster Registration** | New cluster added | Detect KubeVirt version → check cache → queue fetch if missing |
| **3. Version Detection** | Health check loop (60s) | Piggyback: compare `clusters.kubevirt_version` with detected version |
| **4. Schema Update** | Version change detected | Queue `SchemaUpdateJob` (River) → async fetch → cache update |

**Expiration Policy**: Schemas are **immutable per version** (v1.5.0 never changes). Cache indefinitely; update only on version change.

**Graceful Degradation**: If schema fetch fails → use embedded fallback → retry on next health check cycle.

**Frontend Schema Fallback Strategy**:

> ⚠️ **Critical Dependency**: Schema-Driven UI + Mask pattern relies heavily on Schema Cache. The following fallback strategy ensures stable frontend rendering when cache fails or version drifts.

| Scenario | API Response | Frontend UI Behavior |
|----------|--------------|---------------------|
| **Schema available** | `200 OK` with `schema_version` header | Normal form rendering with dynamic components |
| **Schema cache miss** | `200 OK` with `X-Schema-Fallback: embedded-v1.5.x` | Render with embedded fallback; show ⚠️ banner |
| **Schema fetch in progress** | `200 OK` with `X-Schema-Status: updating` | Render with stale schema; show 🔄 loading indicator |
| **No schema available** | Error in `/api/v1/schema/{version}` | **Fallback UI Mode** (see below) |

**Fallback UI Mode** (when no schema is available):

```
┌────────────────────────────────────────────────────────────────────────────┐
│  ⚠️ Schema Unavailable                                                     │
│                                                                            │
│  Unable to load KubeVirt v1.6.x schema for this cluster.                   │
│  Dynamic field rendering is temporarily unavailable.                       │
│                                                                            │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │  Fallback Mode: Basic Fields Only                                  │   │
│  │                                                                    │   │
│  │  CPU Cores:    [4        ]    (integer input, no validation)      │   │
│  │  Memory:       [8Gi      ]    (text input, no validation)         │   │
│  │                                                                    │   │
│  │  ⚠️ Advanced fields (GPU, Hugepages, SR-IOV) are hidden.         │   │
│  │     Contact admin or wait for schema sync.                        │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│                                                                            │
│  [Proceed with Basic Config]    [Cancel]                                   │
│                                                                            │
│  ℹ️ Schema will auto-retry on next health check cycle (60s).              │
└────────────────────────────────────────────────────────────────────────────┘
```

**Admin Alert Integration**:

| Alert Condition | Alert Level | Notification Target |
|-----------------|-------------|---------------------|
| Schema cache miss (using embedded fallback) | Warning | Admin dashboard widget |
| Schema fetch failed 3+ consecutive times | Error | DomainEvent + In-app notification |
| Version drift detected (cluster upgraded) | Info | Audit log only |

> **Implementation Note**: Frontend should cache schema locally (localStorage/IndexedDB) as secondary fallback. Check `X-Schema-Version` header to detect staleness.

> **Implementation Standard**: For detailed frontend code patterns, i18n keys, and mandatory UI components, see [FRONTEND.md §Schema Cache Degradation Strategy](../FRONTEND.md#schema-cache-degradation-strategy-adr-0023).

See ADR-0023 §1 for complete cache lifecycle diagram.

---

## Part 1: Platform Initialization Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 1: Platform Initialization (Developer Operations)                  │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Developer:                                                                                   │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. Fetch KubeVirt official JSON Schema                                                   │ │
│  │    - Source: KubeVirt CRD OpenAPI Schema or official docs                               │ │
│  │    - Includes: all field types, constraints, enum options                               │ │
│  │                                                                                          │ │
│  │ 2. Define Mask configuration (select paths only, do not define options)                  │ │
│  │                                                                                          │ │
│  │    mask:                                                                                 │ │
│  │      quick_fields:                                                                       │ │
│  │        - path: "spec.template.spec.domain.cpu.cores"                                     │ │
│  │          display_name: "CPU Cores"                                                       │ │
│  │      advanced_fields:                                                                    │ │
│  │        - path: "spec.template.spec.domain.devices.gpus"                                  │ │
│  │          display_name: "GPU Devices"                                                     │ │
│  │        - path: "spec.template.spec.domain.memory.hugepages.pageSize"                     │ │
│  │          display_name: "Hugepages Size"                                                  │ │
│  │                                                                                          │ │
│  │    👉 Mask references Schema paths only; field types and options come from Schema       │ │
│  │                                                                                          │ │
│  │ 3. Frontend renders UI automatically based on Schema + Mask                              │ │
│  │    - integer → numeric input                                                            │ │
│  │    - string → text input                                                                │ │
│  │    - boolean → checkbox                                                                 │ │
│  │    - enum → dropdown (options from Schema, not developer-defined)                       │ │
│  │    - array → dynamic add/remove table                                                    │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Stage 1.5: First Deployment Bootstrap {#stage-1-5}

> **Added 2026-01-26**: First deployment flow for configuration storage strategy.
>
> **Detailed Rules**: See [ADR-0025 (Bootstrap Secrets)](../../adr/ADR-0025-secret-bootstrap.md) for secrets priority and auto-generation, [01-contracts.md §3.2.2](../phases/01-contracts.md#322-system-secrets-table-adr-0025) for implementation details.

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                         Stage 1.5: First Deployment Bootstrap                               │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  🔧 Deployment config (choose one):                                                         │
│                                                                                              │
│  📁 Option A: config.yaml (local dev / traditional deploy)                                   │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  # config.yaml                                                                          │ │
│  │  database:                                                                              │ │
│  │    url: "postgresql://user:pass@localhost:5432/shepherd"                                │ │
│  │                                                                                          │ │
│  │  server:                                                                                 │ │
│  │    port: 8080                                                                            │ │
│  │    log_level: "info"                     # optional, default: info                       │ │
│  │                                                                                          │ │
│  │  worker:                                                                                 │ │
│  │    max_workers: 10                       # optional, default: 10                         │ │
│  │                                                                                          │ │
│  │  security:                                                                               │
│  │    encryption_key: "32-byte-random"      # optional, strongly recommended                │ │
│  │    session_secret: "32-byte-random"      # optional, strongly recommended                │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  🐳 Option B: Environment variables (containerized deploy)                                   │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  DATABASE_URL=postgresql://user:pass@host:5432/shepherd    # required                   │ │
│  │  SERVER_PORT=8080                        # optional, default: 8080                      │ │
│  │  LOG_LEVEL=info                          # optional, default: info                       │ │
│  │  RIVER_MAX_WORKERS=10                    # optional, default: 10                         │ │
│  │  ENCRYPTION_KEY=<32-byte-random>         # optional, strongly recommended                │ │
│  │  SESSION_SECRET=<32-byte-random>         # optional, strongly recommended                │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ⚡ **Single Priority Chain** (IMPORTANT - avoid ambiguity):                                   │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  Configuration Type    │  Priority Chain (highest → lowest)                            │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  General config        │  env vars → config.yaml → code defaults                       │ │
│  │  (ports, log level)    │  e.g., SERVER_PORT env overrides config.yaml server.port      │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  Secrets/Keys          │  env vars → DB-generated (system_secrets table)               │ │
│  │  (encryption, session) │  If ENCRYPTION_KEY env set → use it (no DB generation)        │ │
│  │                        │  If ENCRYPTION_KEY not set → auto-generate and store in DB    │ │
│  │  ──────────────────────┼─────────────────────────────────────────────────────────────  │ │
│  │  🔮 V2+ (RFC-0017)     │  External KMS → env vars → DB-generated                       │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ⚠️ **Key Principle**: config.yaml is NOT a source for secrets (12-factor app compliance).   │
│     Secrets must come from: env vars OR DB-generated OR external secret manager.             │
│                                                                                              │
│  🔐 Auto-generation (ADR-0025 - if missing):                                                 │
│  - Generate strong random ENCRYPTION_KEY and SESSION_SECRET on first boot (32-byte CSPRNG)   │
│  - Persist to PostgreSQL `system_secrets` table (no ephemeral in-memory-only keys)           │
│  - If external key is introduced later, explicit re-encryption step required                 │
│  - 🔄 Key rotation deferred to RFC-0016 (not in V1 scope)                                    │
│                                                                                              │
│                                                                                              │
│  📦 App auto-initialization:                                                                 │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  1. Run migrations                                                                       │
│  │  2. Seed built-in roles (ON CONFLICT DO NOTHING - do not overwrite)                      │
│  │  3. Seed default admin admin/admin (force_password_change=true)                          │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│                                                                                              │
│  🖥️ First login prompt:                                                                      │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                                      │   │
│  │                    ⚠️ First Login                                                    │   │
│  │                                                                                      │   │
│  │    Please use the default admin account:                                              │   │
│  │                                                                                      │   │
│  │    Username: admin                                                                    │   │
│  │    Password: admin                                                                    │   │
│  │                                                                                      │   │
│  │    ⚠️ Change the password immediately after login!                                    │   │
│  │                                                                                      │   │
│  │    [Login]                                                                           │   │
│  │                                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  🔐 Forced password change:                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                                      │   │
│  │                    🔐 Set a new password                                              │   │
│  │                                                                                      │   │
│  │    You are using the default password. Change it immediately for security.           │   │
│  │                                                                                      │   │
│  │    New password:     [••••••••••••                ]                                   │   │
│  │    Confirm:          [••••••••••••                ]                                   │   │
│  │                                                                                      │   │
│  │    Password requirements (NIST 800-63B):                                              │   │
│  │    ✓ Minimum 8 characters (15+ recommended)                                          │   │
│  │    ✓ Not in common password blocklist                                                │   │
│  │    ○ Complexity rules not enforced (configurable for legacy compliance)              │   │
│  │                                                                                      │   │
│  │    [Confirm]                                                                          │   │
│  │                                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                    │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Seed default admin (first startup)                                              │
│  │  INSERT INTO users (id, username, password_hash, auth_type, force_password_change) │
│  │  VALUES ('admin', 'admin', bcrypt('admin'), 'local', true)                          │
│  │  ON CONFLICT (username) DO NOTHING;                                                 │
│  │                                                                                    │
│  │  -- Bind PlatformAdmin role                                                         │
│  │  INSERT INTO role_bindings (id, user_id, role_id, scope_type, source)               │
│  │  VALUES ('rb-admin', 'admin', 'role-platform-admin', 'global', 'seed')              │
│  │  ON CONFLICT DO NOTHING;                                                            │
│  │                                                                                    │
│  │  -- After password change                                                           │
│  │  UPDATE users SET                                                                   │
│  │    password_hash = bcrypt('new_password'),                                          │
│  │    force_password_change = false,                                                   │
│  │    updated_at = NOW()                                                               │
│  │  WHERE id = 'admin';                                                                │
│  │                                                                                    │
│  │  -- Audit log                                                                        │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)     │
│  │  VALUES ('user.password_change', 'admin', 'user', 'admin',                           │
│  │          '{"reason": "first_login_forced"}');                                     │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ After completion, enter the admin console and continue Stage 2                           │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Stage 2: Security Configuration (Initial Deployment) {#stage-2}

> **Reference**: ADR-0015 §22 (Authentication & RBAC Strategy)

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.A: Built-in Roles and Permissions Initialization                 │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  🔧 System auto-exec (Seed Data):                                                            │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- 1. Built-in permissions                                                        │
│  │  INSERT INTO permissions (id, resource, action, name) VALUES                      │
│  │    ('system:read', 'system', 'read', 'View system'),                               │
│  │    ('system:write', 'system', 'write', 'Edit system'),                             │
│  │    ('system:delete', 'system', 'delete', 'Delete system'),                         │
│  │    ('service:read', 'service', 'read', 'View service'),                            │
│  │    ('service:create', 'service', 'create', 'Create service'),                      │
│  │    ('service:delete', 'service', 'delete', 'Delete service'),                      │
│  │    ('vm:read', 'vm', 'read', 'View VM'),                                           │
│  │    ('vm:create', 'vm', 'create', 'Create VM request'),                             │
│  │    ('vm:operate', 'vm', 'operate', 'VM ops (start/stop)'),                          │
│  │    ('vm:delete', 'vm', 'delete', 'Delete VM'),                                     │
│  │    ('vnc:access', 'vnc', 'access', 'VNC console'),                                 │
│  │    ('approval:approve', 'approval', 'approve', 'Approve request'),                 │
│  │    ('approval:view', 'approval', 'view', 'View pending approvals'),                │
│  │    ('cluster:manage', 'cluster', 'manage', 'Manage clusters'),                     │
│  │    ('template:manage', 'template', 'manage', 'Manage templates'),                  │
│  │    ('rbac:manage', 'rbac', 'manage', 'Manage permissions'),                        │
│  │    ('platform:admin', 'platform', 'admin', 'Super-admin permission (explicit)');   │
│  │    -- ⚠️ ADR-0019 RBAC Compliance:                                                   │
│  │    -- All roles use explicit permissions. Wildcard patterns (*:*) are PROHIBITED.   │
│  │    -- platform:admin is an explicit super-admin permission (compile-time constant). │
│  │    -- The bootstrap role uses platform:admin and MUST be disabled after init.       │
│  │    -- See docs/operations/bootstrap-role-sop.md for security verification.         │
│  │                                                                                    │
│  │  -- 2. Built-in roles (ADR-0019 compliant)                                   │       │
│  │  INSERT INTO roles (id, name, is_builtin, description) VALUES                      │
│  │    ('role-bootstrap', 'Bootstrap', true, 'Initial setup only - DISABLE AFTER INIT'), │
│  │    ('role-platform-admin', 'PlatformAdmin', true, 'Platform admin'),                │
│  │    ('role-system-admin', 'SystemAdmin', true, 'System admin'),                      │
│  │    ('role-approver', 'Approver', true, 'Approver'),                                 │
│  │    ('role-operator', 'Operator', true, 'Operator'),                                 │
│  │    ('role-viewer', 'Viewer', true, 'Read-only user');                               │
│  │                                                                                    │
│  │  -- 3. Role-permission bindings (ADR-0019: NO wildcards, explicit only)             │
│  │  INSERT INTO role_permissions (role_id, permission_id) VALUES                      │
│  │    -- Bootstrap role: platform:admin (explicit super-admin, DISABLE after init)    │
│  │    ('role-bootstrap', 'platform:admin'),                                            │
│  │    -- PlatformAdmin: platform:admin (explicit super-admin permission per ADR-0019) │
│  │    ('role-platform-admin', 'platform:admin'),                                       │
│  │    -- Approver: explicit permissions (no wildcards per ADR-0019)                    │
│  │    ('role-approver', 'approval:approve'), ('role-approver', 'approval:view'),       │
│  │    ('role-approver', 'vm:read'), ('role-approver', 'system:read'),                  │
│  │    ('role-approver', 'service:read'),                                               │
│  │    -- SystemAdmin, Operator, Viewer: explicit permissions                           │
│  │    ('role-system-admin', 'system:read'), ('role-system-admin', 'system:write'),     │
│  │    ('role-system-admin', 'system:delete'), ('role-system-admin', 'service:read'),   │
│  │    ('role-system-admin', 'service:create'), ('role-system-admin', 'service:delete'),│
│  │    ('role-system-admin', 'vm:read'), ('role-system-admin', 'vm:create'),            │
│  │    ('role-system-admin', 'vm:operate'), ('role-system-admin', 'vm:delete'),         │
│  │    ('role-system-admin', 'vnc:access'), ('role-system-admin', 'rbac:manage'),       │
│  │    ('role-operator', 'system:read'), ('role-operator', 'service:read'),             │
│  │    ('role-operator', 'vm:read'), ('role-operator', 'vm:create'),                    │
│  │    ('role-operator', 'vm:operate'), ('role-operator', 'vnc:access'),                │
│  │    ('role-viewer', 'system:read'), ('role-viewer', 'service:read'),                 │
│  │    ('role-viewer', 'vm:read');                                                      │
│  │                                                                                    │
│  │  -- ⚠️ ADR-0019 Security SOP:                                                       │
│  │  -- After platform initialization, DISABLE the bootstrap role:                      │
│  │  --   DELETE FROM role_bindings WHERE role_id = 'role-bootstrap';                  │
│  │  -- See docs/operations/bootstrap-role-sop.md for full procedure.                  │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.A+: Custom Role Management (Optional)                             │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin actions (before or after OIDC setup):                                         │
│                                                                                              │
│  ┌─ Step 1: Create custom role ───────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Role Management                                                                       │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Role list:                                                                       │   │   │
│  │  │  ──────────────────────────────────────────────────────────────────────────    │   │   │
│  │  │  [🔒] PlatformAdmin          Built-in    Platform admin - all access             │   │   │
│  │  │  [🔒] SystemAdmin            Built-in    System admin                            │   │   │
│  │  │  [🔒] Approver               Built-in    Approver                                │   │   │
│  │  │  [🔒] Operator               Built-in    Operator                                │   │   │
│  │  │  [🔒] Viewer                 Built-in    Read-only user                          │   │   │
│  │  │  [  ] DevLead                Custom      Dev lead (editable/deletable)           │   │   │
│  │  │  [  ] QA-Manager             Custom      QA manager (editable/deletable)         │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [+ Create custom role]                                                          │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: Configure permissions for custom role ─────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Create Custom Role                                                                     │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Role name:       [DevLead              ]                                         │   │   │
│  │  │  Description:     [Dev lead - manage system/service]                              │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Select permissions (global):                                                   │   │   │
│  │  │  ┌─ System management ────────────────┐  ┌─ Approval management ─────────────┐    │   │   │
│  │  │  │ ☑ system:read                     │  │ ☐ approval:approve                │    │   │   │
│  │  │  │ ☑ system:write                    │  │ ☐ approval:view                   │    │   │   │
│  │  │  │ ☐ system:delete                   │  └──────────────────────────────────┘    │   │   │
│  │  │  └──────────────────────────────────┘                                             │   │   │
│  │  │  ┌─ Service management ─────────────┐  ┌─ Platform management ───────────────┐    │   │   │
│  │  │  │ ☑ service:read                   │  │ ☐ cluster:manage                    │    │   │   │
│  │  │  │ ☑ service:create                 │  │ ☐ template:manage                   │    │   │   │
│  │  │  │ ☐ service:delete                 │  │ ☐ rbac:manage                       │    │   │   │
│  │  │  └──────────────────────────────────┘  └──────────────────────────────────────┘    │   │   │
│  │  │  ┌─ VM management ─────────────────┐                                               │   │   │
│  │  │  │ ☑ vm:read                       │                                               │   │   │
│  │  │  │ ☑ vm:create                     │                                               │   │   │
│  │  │  │ ☑ vm:operate                    │                                               │   │   │
│  │  │  │ ☐ vm:delete                     │                                               │   │   │
│  │  │  │ ☑ vnc:access                    │                                               │   │   │
│  │  │  └──────────────────────────────────┘                                               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Save role]                                                                     │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Create custom role                                                            │
│  │  INSERT INTO roles (id, name, is_builtin, description) VALUES                      │
│  │    ('role-dev-lead', 'DevLead', false, 'Dev lead - manage system/service');        │
│  │                                                                                    │
│  │  -- Bind permissions                                                               │
│  │  INSERT INTO role_permissions (role_id, permission_id) VALUES                      │
│  │    ('role-dev-lead', 'system:read'), ('role-dev-lead', 'system:write'),            │
│  │    ('role-dev-lead', 'service:read'), ('role-dev-lead', 'service:create'),         │
│  │    ('role-dev-lead', 'vm:read'), ('role-dev-lead', 'vm:create'),                   │
│  │    ('role-dev-lead', 'vm:operate'), ('role-dev-lead', 'vnc:access');               │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 After creating a custom role, it can be used in IdP group mapping (Stage 2.C)            │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
> **Standard Provider Output**: All auth providers (OIDC/LDAP/SSO) are normalized via adapter layer into a common payload for RBAC mapping. See ADR-0026.
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.B: Configure Authentication (OIDC/LDAP)                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin actions:                                                                      │
│                                                                                              │
│  ┌─ Step 1: Choose auth type ────────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Authentication Configuration                                                         │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Auth type:                                                                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ◉ OIDC (recommended) - Azure AD, Okta, Keycloak, Google Workspace               │   │   │
│  │  │  ○ LDAP               - Active Directory, OpenLDAP                               │   │   │
│  │  │  ○ Built-in users     - dev/test only                                            │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Next →]                                                                         │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: OIDC configuration ───────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  OIDC Provider Configuration                                                          │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Provider name:  [Corp-SSO                    ]                                  │   │   │
│  │  │  Issuer URL:     [https://sso.company.com/realms/main]                           │   │   │
│  │  │  Client ID:      [shepherd-platform           ]                                  │   │   │
│  │  │  Client Secret:  [••••••••••••                ] 👁                               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Callback URL (copy to IdP):                                                     │   │   │
│  │  │  📋 https://shepherd.company.com/api/v1/auth/oidc/callback                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Test connection]  [Save config]                                                │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  INSERT INTO auth_providers (id, type, name, enabled, issuer, client_id,           │
│  │    client_secret_encrypted, scopes, claims_mapping, default_role_id,               │
│  │    default_allowed_environments) VALUES                                            │
│  │  ('idp-001', 'oidc', 'Corp-SSO', true, 'https://sso.company.com/realms/main',       │
│  │   'shepherd-platform', 'encrypted:xxx', ARRAY['openid','profile','email'],         │
│  │   '{"groups":"groups","groups_format":"array"}', 'role-viewer', ARRAY['test']);    │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.C: IdP Group Mapping                                              │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin actions:                                                                      │
│                                                                                              │
│  ┌─ Step 1: Fetch sample user data ─────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  API: GET /api/v1/admin/auth-providers/{id}/sample                                                │
│  │  System pulls 10 users' token data from IdP and extracts available fields:            │
│  │                                                                                        │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Detected fields:                                                                 │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ◉ groups (array, 5 unique values)                                               │   │   │
│  │  │     sample: ["DevOps-Team", "QA-Team", "Platform-Admin", ...]                    │   │   │
│  │  │  ○ department (string, 3 unique values)                                          │   │   │
│  │  │     sample: ["Engineering", "IT", "QA"]                                           │   │   │
│  │  │  ○ custom_roles (array, 2 unique values)                                         │   │   │
│  │  │     sample: ["admin", "developer"]                                                │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Sync selected fields →]                                                        │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: Configure group-to-role mappings ────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  IdP Group → Shepherd Role mapping                                                    │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  IdP group            Shepherd role       Allowed envs                          │   │   │
│  │  │  ──────────────────────────────────────────────────────────────────────────    │   │   │
│  │  │  Platform-Admin       [PlatformAdmin ▼]  ☑ test  ☑ prod                         │   │   │
│  │  │  DevOps-Team          [SystemAdmin ▼]    ☑ test  ☑ prod                         │   │   │
│  │  │  QA-Team              [Operator ▼]       ☑ test  ☐ prod                         │   │   │
│  │  │  IT-Support           [Viewer ▼]         ☑ test  ☐ prod                         │   │   │
│  │  │  HR-Department        [Unmapped ▼]       -                                       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  💡 Unmapped groups default to Viewer + test-only                                 │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Save mapping]                                                                   │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Sync IdP groups                                                             │
│  │  INSERT INTO idp_synced_groups (id, auth_provider_id, group_id, source_field)    │
│  │  VALUES ('sg-001', 'idp-001', 'Platform-Admin', 'groups'),                       │
│  │         ('sg-002', 'idp-001', 'DevOps-Team', 'groups'),                          │
│  │         ('sg-003', 'idp-001', 'QA-Team', 'groups');                              │
│  │                                                                                    │
│  │  -- Save mappings                                                                   │
│  │  INSERT INTO idp_group_mappings (id, auth_provider_id, idp_group_id, role_id,      │
│  │                                  scope_type, allowed_environments) VALUES          │
│  │    ('map-001', 'idp-001', 'Platform-Admin', 'role-platform-admin',                 │
│  │     'global', ARRAY['test', 'prod']),                                              │
│  │    ('map-002', 'idp-001', 'DevOps-Team', 'role-system-admin',                      │
│  │     'global', ARRAY['test', 'prod']),                                              │
│  │    ('map-003', 'idp-001', 'QA-Team', 'role-operator',                              │
│  │     'global', ARRAY['test']);                                                     │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.D: User Login Flow                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  First login flow:                                                                            │
│                                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  1. User visits https://shepherd.company.com                                           │
│  │                                                                                        │
│  │  2. Redirect to IdP login                                                              │
│  │     → https://sso.company.com/realms/main/protocol/openid-connect/auth?                │
│  │       client_id=shepherd-platform&redirect_uri=...                                    │
│  │                                                                                        │
│  │  3. User completes IdP authentication                                                  │
│  │                                                                                        │
│  │  4. IdP calls back Shepherd                                                            │
│  │     ← https://shepherd.company.com/api/v1/auth/oidc/callback?code=xxx                  │
│  │                                                                                        │
│  │  5. Shepherd processing:                                                               │
│  │     a. Validate token (signature, issuer, audience)                                   │
│  │     b. Extract user info (sub, email, name, groups)                                   │
│  │     c. Lookup idp_group_mappings by groups                                             │
│  │     d. Create/update user record                                                      │
│  │     e. Create RoleBindings (based on mapping)                                         │
│  │     f. Return JWT session token                                                       │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  📦 Database operations (first login):                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │
│  │                                                                                    │
│  │  -- 1. Create user record (if not exists)                                          │
│  │  INSERT INTO users (id, external_id, email, name, auth_provider_id, created_at)   │
│  │  VALUES ('user-001', 'oidc|abc123', 'zhang.san@company.com', 'Zhang San',          │
│  │          'idp-001', NOW())                                                         │
│  │  ON CONFLICT (external_id) DO UPDATE SET last_login_at = NOW();                   │
│  │                                                                                    │
│  │  -- 2. Remove old auto-assigned RoleBindings                                        │
│  │  DELETE FROM role_bindings                                                         │
│  │  WHERE user_id = 'user-001' AND source = 'idp_mapping';                            │
│  │                                                                                    │
│  │  -- 3. Recreate RoleBindings based on groups                                        │
│  │  -- (user groups: ['DevOps-Team'] → map to role-system-admin)                       │
│  │  INSERT INTO role_bindings (id, user_id, role_id, scope_type,                       │
│  │                             allowed_environments, source) VALUES                  │
│  │    ('rb-auto-001', 'user-001', 'role-system-admin', 'global',                       │
│  │     ARRAY['test', 'prod'], 'idp_mapping');                                          │
│  │                                                                                    │
│  │  COMMIT;                                                                           │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Login Methods Summary

| Login Method | Use Case | Permission Source |
|-------------|----------|-------------------|
| **OIDC** | Production (recommended) | IdP group → mapping rules → RoleBindings |
| **LDAP** | Legacy AD environment | LDAP group → mapping rules → RoleBindings |
| **Built-in users** | Dev/test | Manual user + RoleBindings |

#### Dual-layer Permission Model Summary

| Dimension | Global RBAC | Resource-level RBAC |
|----------|-------------|---------------------|
| **Tables** | `role_bindings` | `resource_role_bindings` |
| **Scope** | Platform-level operations | Access to specific resources |
| **Role Types** | PlatformAdmin, SystemAdmin, Approver, Operator, Viewer, custom | Owner, Admin, Member, Viewer |
| **Assignment** | Admin via IdP mapping or manual | Resource owner adds members |
| **Typical Case** | "User can approve VM requests" | "User can access this system" |
| **Visibility Control** | None (global) | Yes (members only) |
| **Inheritance** | N/A | ✅ Service/VM inherit System permissions |

#### Permission Check Logic

> **Two-layer permission system**:
> - **Global RBAC (role_bindings)**: platform-level ops (clusters, templates, approvals)
> - **Resource-level RBAC (resource_role_bindings)**: access to specific resources

```
Full permission check flow:

User requests access to resource R (e.g., GET /api/v1/systems/sys-001)

┌─ Step 1: Global permission check ───────────────────────────────────────────────┐
│  Query role_bindings → aggregate permissions                                    │
│  - Has platform:admin permission → allow all resources (explicit super-admin)   │
│  - Has required global permission (system:read) → proceed to Step 2             │
│  - Otherwise → deny                                                            │
└────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─ Step 2: Resource-level permission check ───────────────────────────────────────┐
│  Query resource_role_bindings WHERE resource_id = 'sys-001' AND user_id = ?     │
│  - Found (owner/admin/member/viewer) → allow per role                           │
│  - Not found → check inheritance (VM → Service → System)                        │
│  - Still not found → deny (resource invisible)                                  │
└────────────────────────────────────────────────────────────────────────────────┘

Example 1: Zhang San (DevOps-Team) accesses own System
1. Global permission: system:read ∈ SystemAdmin → proceed
2. Resource permission: role='owner' → ✅ allow

Example 2: Li Si (IT-Support) accesses Zhang San's System
1. Global permission: system:read ∈ Viewer → proceed
2. Resource permission: not found → ❌ invisible

Example 3: Li Si added as System member
1. Global permission: system:read ∈ Viewer → proceed
2. Resource permission: role='member' → ✅ allow view

Example 4: Li Si accesses VM under Zhang San's System (inherit)
Target: vm-001 (svc-redis → sys-shop)
1. Global permission: vm:read ∈ Viewer → proceed
2. Resource permission (walk up):
   a. VM binding → none
   b. Service binding → none
   c. System binding → found role='member'
3. Result: inherit System member → ✅ can view VM
```

### Stage 2.E: External Approval System Configuration (Optional) {#stage-2-e}

> **Added 2026-01-26**: External approval system integration configuration

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 2.E: External Approval System Configuration                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin actions:                                                                      │
│                                                                                              │
│  ┌─ Step 1: Add external approval system ────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  External approval systems list                                                       │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Name            Type            Status       Actions                           │   │   │
│  │  │  ────────────────────────────────────────────────────────────────────────────  │   │   │
│  │  │  OA-Approval     Webhook         ✅ Enabled   [Edit] [Disable] [Delete]          │   │   │
│  │  │  ServiceNow      ServiceNow      ⚪ Disabled  [Edit] [Enable] [Delete]           │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [+ Add approval system]                                                        │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: Configure Webhook type ────────────────────────────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Add External Approval System - Webhook                                                │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │                                                                                  │   │   │
│  │  │  Name:         [OA-Approval                ]                                     │   │   │
│  │  │  Type:         ( ) Webhook   (●) ServiceNow   ( ) Jira                            │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ── Webhook Config ─────────────────────────────────────────────────────────    │   │   │
│  │  │  Webhook URL:  [https://oa.company.com/api/approval/callback               ]     │   │   │
│  │  │  Secret:       [••••••••••••                                ] 👁               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Custom Headers (JSON):                                                          │   │   │
│  │  │  ┌──────────────────────────────────────────────────────────────────────────┐   │   │   │
│  │  │  │  {                                                                        │   │   │   │
│  │  │  │    "X-API-Key": "your-api-key",                                           │   │   │   │
│  │  │  │    "X-Tenant-ID": "company-001"                                           │   │   │   │
│  │  │  │  }                                                                        │   │   │   │
│  │  │  └──────────────────────────────────────────────────────────────────────────┘   │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Timeout (sec): [30             ]                                               │   │   │
│  │  │  Retry count:   [3              ]                                               │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Test Connection]  [Save]                                                    │   │   │
│  │  │                                                                                  │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  INSERT INTO external_approval_systems                                            │
│  │    (id, name, type, enabled, webhook_url, webhook_secret, webhook_headers,        │
│  │     timeout_seconds, retry_count, created_by, created_at)                         │
│  │  VALUES                                                                            │
│  │    ('eas-001', 'OA-Approval', 'webhook', true,                                     │
│  │     'https://oa.company.com/api/approval/callback',                                │
│  │     'encrypted:AES256:xxxx',                   -- encrypted storage                │
│  │     '{"X-API-Key": "xxx", "X-Tenant-ID": "company-001"}',                      │
│  │     30, 3, 'admin', NOW());                                                        │
│  │                                                                                    │
│  │  -- Audit log                                                                       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)    │
│  │  VALUES ('external_approval_system.create', 'admin',                               │
│  │         'external_approval_system', 'eas-001',                                     │
│  │         '{"name": "OA-Approval", "type": "webhook", "url": "https://oa.company.com..."}');
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 Sensitive data encryption:                                                              │
│  - webhook_secret stored encrypted with AES-256-GCM                                         │
│  - decryption key from external/env if provided; otherwise from DB-generated key            │
│  - sensitive fields must not be logged                                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

<a id="stage-3"></a>

---

```
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 3: Admin Configuration (Cluster/InstanceSize/Template)             │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin:                                                                             │
│                                                                                              │
│  ┌─ Step 1: Register clusters (auto-detect capabilities) ────────────────────────────────┐  │
│  │                                                                                        │  │
│  │  Admin provides:                                                                      │  │
│  │  POST /api/v1/admin/clusters                                                          │  │
│  │  { "name": "cluster-a", "kubeconfig": "...", "environment": "prod" }          │  │
│  │                                                                                        │  │
│  │  System auto-detects (ADR-0014), admin does not configure manually:                    │  │
│  │  ┌───────────────────────────────────────────────────────────────────────────────────┐ │
│  │  │  Item               Detection method                         Example result       │ │
│  │  │  ─────────────────────────────────────────────────────────────────────────────── │ │
│  │  │  GPU devices         node.status.capacity (nvidia.com/gpu)     nvidia.com/gpu: 2  │ │
│  │  │                    💡 requires NVIDIA Device Plugin                              │ │
│  │  │                                                                                   │ │
│  │  │  Hugepages          node.status.allocatable                   hugepages-2Mi: 4Gi  │ │
│  │  │                    (hugepages-2Mi, hugepages-1Gi)             hugepages-1Gi: 2Gi  │ │
│  │  │                    💡 may be empty if not configured                              │ │
│  │  │                                                                                   │ │
│  │  │  SR-IOV networks     kubectl get net-attach-def -A             sriov-net-1         │ │
│  │  │                    (NetworkAttachmentDefinition CRD)           sriov-net-2         │ │
│  │  │                    💡 requires Multus CNI + SR-IOV device plugin │ │
│  │  │                                                                                   │ │
│  │  │  StorageClass        kubectl get storageclasses                ceph-rbd, local-path │ │
│  │  │                                                                                   │ │
│  │  │  KubeVirt version    kubevirt.status.observedKubeVirtVersion   v1.2.0              │ │
│  │  │                    kubectl get kv -n kubevirt -o jsonpath=                         │ │
│  │  │                    '{.items[0].status.observedKubeVirtVersion}'                    │ │
│  │  └───────────────────────────────────────────────────────────────────────────────────┘ │
│  │                                                                                        │  │
│  │  Detected results stored (admin can view, no manual input):                            │  │
│  │  cluster.detected_capabilities = {                                                     │  │
│  │      "gpu_devices": ["nvidia.com/GA102GL_A10"],                                      │  │
│  │      "hugepages": ["2Mi", "1Gi"],                                                   │  │
│  │      "sriov_networks": ["sriov-net-1"],                                              │  │
│  │      "storage_classes": ["ceph-rbd", "local-path"],                                │  │
│  │      "kubevirt_version": "v1.2.0"                                                   │  │
│  │  }                                                                                    │  │
│  │                                                                                        │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                              │
│  ┌─ Step 2: Configure Namespace (ADR-0017 Compliant) ─────────────────────────────────────┐ │
│  │                                                                                          │
│  │  ⚠️ KEY PRINCIPLE (ADR-0017):                                                            │
│  │  - Namespace is a **global logical entity**, NOT bound to a specific cluster             │
│  │  - Actual K8s namespace is created JIT (Just-In-Time) when approved VM is provisioned   │
│  │  - Namespace is **IMMUTABLE after VM request submission**                                │
│  │                                                                                          │
│  │  Platform responsibility boundary:                                                      │
│  │  - ✅ Manage logical namespace registry (environment labels, ownership)                  │
│  │  - ❌ Not managed: Kubernetes RBAC / ResourceQuota (owned by K8s admins)                 │
│  │                                                                                          │
│  │  Admin action (register logical namespace):                                              │
│  │  POST /api/v1/admin/namespaces                    👈 NOT cluster-scoped                 │
│  │  {                                                                                       │
│  │      "name": "prod-shop",                                                              │
│  │      "environment": "prod",                       👈 drives approval and cluster match │
│  │      "owner_id": "user-001"                                                            │
│  │  }                                                                                       │
│  │                                                                                          │
│  │  💡 When user selects a Namespace, system uses environment label to determine:           │
│  │     - Approval policy (test can be fast, prod is strict)                                 │
│  │     - Overcommit warnings (warn in prod)                                                 │
│  │     - Cluster matching (namespace env must match cluster env: test→test, prod→prod)       │
│  │                                                                                          │
│  │  💡 JIT Namespace Creation (during approval execution):                                  │
│  │     When admin approves a VM request and selects target cluster:                         │
│  │     1. Check if K8s namespace exists on target cluster                                   │
│  │     2. If not exists → create namespace with standard labels                             │
│  │     3. Error handling (K8s API errors are classified and reported):                      │
│  │        - Permission denied → NAMESPACE_PERMISSION_DENIED (403)                           │
│  │        - ResourceQuota exceeded → NAMESPACE_QUOTA_EXCEEDED (403) ¹                       │
│  │        - Other errors → NAMESPACE_CREATION_FAILED (500)                                  │
│  │     See ADR-0017 §142-221 for full JIT creation flow.                                   │
│  │                                                                                          │
│  │     ¹ K8s may reject namespace creation if cluster has ResourceQuota policy.             │
│  │       Failure handling: Ticket → FAILED_PROVISIONING, retry with exponential backoff.    │
│  │       See ADR-0017 §142-221 for complete JIT error handling and recovery strategies.     │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
│  ┌─ Step 3: Configure Template (ADR-0015 §5, §17) ─────────────────────────────────────────┐ │
│  │                                                                                          │
│  │  Template defines base VM OS configuration:                                              │
│  │  - OS image source (DataVolume / PVC reference)                                          │
│  │  - cloud-init config (admin customizable)                                                │
│  │  - field visibility control (quick_fields / advanced_fields)                             │
│  │                                                                                          │
│  │  💡 Hardware capability requirements (GPU/SR-IOV/Hugepages) moved to InstanceSize         │
│  │  💡 Seed data preloads common templates into PostgreSQL                                  │
│  │                                                                                          │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐   │
│  │  │  Create Template                                                                    │   │
│  │  │                                                                                    │   │
│  │  │  Name:         [centos7-standard    ]                                               │   │
│  │  │  Category:     [OS ▼]                                                               │   │
│  │  │  Status:       [active ▼]                                                           │   │
│  │  │                                                                                    │   │
│  │  │  ── Image Source ────────────────────────────────────────────────────────────     │   │
│  │  │  Type:         (●) containerdisk   ( ) pvc                                          │   │
│  │  │                                                                                    │   │
│  │  │  ┌─ containerdisk mode ───────────────────────────────────────────────────────┐    │   │
│  │  │  │  Image:     [docker.io/kubevirt/centos:7                    ]                │    │   │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘    │   │
│  │  │                                                                                    │   │
│  │  │  ┌─ pvc mode (after toggle) ───────────────────────────────────────────────────┐   │   │
│  │  │  │  Namespace:  [default           ]                                           │   │   │
│  │  │  │  PVC Name:   [centos7-base-disk ]                                           │   │   │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘   │   │
│  │  │                                                                                    │   │
│  │  │  ── cloud-init config (YAML) ─────────────────────────────────────────────────   │   │
│  │  │  ┌────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  │  #cloud-config                                                               │   │   │
│  │  │  │  users:                                                                      │   │   │
│  │  │  │    - name: admin                                                             │   │   │
│  │  │  │      sudo: ALL=(ALL) NOPASSWD:ALL                                            │   │   │
│  │  │  │  chpasswd:                                                                   │   │   │
│  │  │  │    expire: true                         👈 force change on first login       │   │   │
│  │  │  │    users:                                                                    │   │   │
│  │  │  │      - name: admin                                                           │   │   │
│  │  │  │        password: changeme123            👈 one-time initial password          │   │   │
│  │  │  └────────────────────────────────────────────────────────────────────────────┘   │   │
│  │  │                                                                                    │   │
│  │  │  💡 Platform responsibility: provide one-time password for first login            │   │
│  │  │  💡 Subsequent mgmt: user/admin/bastion (custom cloud-init if needed)             │   │
│  │  │                                                                                    │   │
│  │  │  [Save]                                                                           │   │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘   │
│  │                                                                                          │
│  │  Template versioning (ADR-0015 §17):                                                    │
│  │  - User sees active version when submitting request                                    │
│  │  - Admin may select a different version during approval                               │
│  │  - Final template snapshotted into ApprovalTicket; VM not affected by later updates   │
│  │                                                                                          │
│  │  👉 Regular user: selects template, cannot edit cloud-init                              │
│  │  👉 Admin: can create/edit templates (image source + cloud-init)                        │
│  │             (custom cloud-init allowed for bastion integration)                         │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ┌─ Step 4: Create InstanceSize (schema-driven form) ──────────────────────────────────────┐ │
│  │                                                                                          │
│  │  Admin UI (frontend renders from Schema):                                               │
│  │                                                                                          │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐   │
│  │  │  Create InstanceSize                                                               │   │
│  │  │                                                                                    │   │
│  │  │  Name:         [gpu-workstation    ]                                               │   │
│  │  │  Display name: [GPU Workstation (8 cores 32GB)]                                     │   │
│  │  │                                                                                    │   │
│  │  │  ── Resource Configuration ──────────────────────────────────────────────────     │   │
│  │  │  CPU cores:    [8        ]                                                         │   │
│  │  │  [✓] Enable CPU overcommit    👈 show request/limit when enabled                   │   │
│  │  │      ┌────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │      │  CPU Request: [4    ] cores   CPU Limit: [8    ] cores (2x)               │   │   │
│  │  │      └────────────────────────────────────────────────────────────────────────┘   │   │
│  │  │                                                                                    │   │
│  │  │  Memory:       [32Gi     ]                                                         │   │
│  │  │  [✓] Enable memory overcommit                                                      │   │
│  │  │      ┌────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │      │  Mem Request: [16Gi ]   Mem Limit: [32Gi ]   (2x)                         │   │   │
│  │  │      └────────────────────────────────────────────────────────────────────────┘   │   │
│  │  │                                                                                    │   │
│  │  │  ── Advanced Settings ──                                                            │   │
│  │  │  Hugepages:   [None ▼]   👈 options from KubeVirt Schema enum + default None      │   │
│  │  │               [None ]    ← default: no Hugepages                                   │   │
│  │  │               [2Mi  ]                                                              │   │
│  │  │               [1Gi  ]                                                              │   │
│  │  │                                                                                    │   │
│  │  │  Dedicated CPU: [✓]       👈 checkbox (Schema type: boolean)                        │   │
│  │  │                                                                                    │   │
│  │  │  GPU devices:            👈 dynamic table (Schema type: array)                      │   │
│  │  │  ┌──────────────────────────────────────────────────────────────────────────┐    │   │
│  │  │  │  Name     Device Name                                                     │    │   │
│  │  │  │  [gpu1 ]  [nvidia.com/GA102GL_A10         ]  ← admin input                 │    │   │
│  │  │  │                                                                          │    │   │
│  │  │  │  [+ Add GPU]                                                              │    │   │
│  │  │  └──────────────────────────────────────────────────────────────────────────┘    │   │
│  │  │                                                                                    │   │
│  │  │  [Save]                                                                            │   │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘   │
│  │                                                                                          │
│  │  Store in PostgreSQL (backend does not interpret, stores JSON):                          │
│  │  {                                                                                       │
│  │      "name": "gpu-workstation",                                                      │
│  │      "cpu_overcommit": { "enabled": true, "request": "4", "limit": "8" },      │
│  │      "mem_overcommit": { "enabled": true, "request": "16Gi", "limit": "32Gi" },│
│  │      "spec_overrides": {                                                               │
│  │          "spec.template.spec.domain.cpu.cores": 8,                                     │
│  │          "spec.template.spec.domain.resources.requests.memory": "32Gi",              │
│  │          "spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",               │
│  │          "spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,                  │
│  │          "spec.template.spec.domain.devices.gpus": [                                   │
│  │              {"name": "gpu1", "deviceName": "nvidia.com/GA102GL_A10"}              │
│  │          ]                                                                             │
│  │      }                                                                                 │
│  │  }                                                                                       │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  ⚠️ Dry-Run Validation (ADR-0018):                                                          │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  Before saving, admin can validate InstanceSize against target clusters:                 │ │
│  │                                                                                          │ │
│  │  POST /api/v1/admin/instance-sizes?dryRun=All                                            │ │
│  │  POST /api/v1/admin/instance-sizes?dryRun=All&targetCluster={cluster_id}                 │ │
│  │                                                                                          │ │
│  │  Validation Stages:                                                                      │ │
│  │  ┌────────────────────────────────────────────────────────────────────────────────────┐  │ │
│  │  │  Stage 1: Structural Check      → YAML/JSON syntax valid                            │  │ │
│  │  │  Stage 2: Schema Validation     → KubeVirt VirtualMachine Schema compatible         │  │ │
│  │  │  Stage 3: Cluster Dry-Run (opt) → kubectl apply --dry-run=server on target cluster  │  │ │
│  │  └────────────────────────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                                          │ │
│  │  Response (dry-run mode):                                                               │ │
│  │  {                                                                                       │ │
│  │      "valid": true,                                                                     │ │
│  │      "rendered_yaml": "...",     👈 preview of generated VM spec                        │ │
│  │      "compatible_clusters": ["cluster-a", "cluster-c"]   👈 matching clusters           │ │
│  │  }                                                                                       │ │
│  │                                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 2: Resource Management Flow

> **Note**: Before creating VMs, users must create System and Service to organize resources.

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 4: User Creates Resource Hierarchy                                 │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Order: System → Service → VM                                                               │
│                                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  System                                                                               │ │
│  │    ├── Service                                                                        │ │
│  │    │     ├── VM 1                                                                      │ │
│  │    │     └── VM 2                                                                      │ │
│  │    └── Service                                                                        │ │
│  │          └── VM 3                                                                      │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 4.A: User Creates System                                           │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  User actions:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  Create System                                                                     │       │
│  │                                                                                    │       │
│  │  System name:     [shop                ]    👈 globally unique, max 15 chars        │       │
│  │  Description:     [E-commerce core system] 👈 Markdown supported                    │       │
│  │               [Preview] [Upload .md file]    ← or upload existing Markdown file     │       │
│  │                                                                                    │       │
│  │  [Create]                                                                           │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 Database operations (single transaction):                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Create system                                                               │       │
│  │  INSERT INTO systems (id, name, description, created_by, tenant_id, created_at)   │       │
│  │  VALUES ('sys-001', 'shop', 'E-commerce core system', 'zhang.san', 'default', NOW());│      │
│  │                                                                                    │       │
│  │  -- 2. Auto permission inheritance (ResourceRoleBinding)                           │       │
│  │  INSERT INTO resource_role_bindings                                               │       │
│  │    (id, user_id, role, resource_type, resource_id, granted_by, created_at)        │       │
│  │  VALUES ('rrb-001', 'zhang.san', 'owner', 'system', 'sys-001', 'zhang.san', NOW()); │       │
│  │                                                                                    │       │
│  │  -- 3. 📝 Audit log                                                                │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)   │       │
│  │  VALUES ('system.create', 'zhang.san', 'system', 'sys-001',                        │       │
│  │          '{"name": "shop", "description": "E-commerce core system"}');       │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ No approval required: any user can create a System                                       │
│                                                                                              │
│  👆 Creator becomes the System Owner with full control                                       │
│     Other users cannot see this System or its Services/VMs by default                        │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 4.A+: Resource-level Member Management (Owner)                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  💡 Core design: resource creators can add users to their System/Service                      │
│     without platform admin involvement (team self-service).                                  │
│                                                                                              │
│  Owner actions (System settings → Member management):                                        │
│                                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  System Members - shop                                                             │       │
│  │                                                                                    │       │
│  │  Current members:                                                                  │       │
│  │  ┌────────────────────────────────────────────────────────────────────────────┐   │       │
│  │  │  User             Role               Actions                               │   │       │
│  │  │  ────────────────────────────────────────────────────────────────────────  │   │       │
│  │  │  Zhang San         Owner (creator)     -                                   │   │       │
│  │  │  Li Si             Admin               [⚙ Edit] [🗑 Remove]                 │   │       │
│  │  │  Wang Wu           Member              [⚙ Edit] [🗑 Remove]                 │   │       │
│  │  │  Zhao Liu          Viewer              [⚙ Edit] [🗑 Remove]                 │   │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘   │       │
│  │                                                                                    │       │
│  │  [+ Add Member]                                                                     │       │
│  │                                                                                    │       │
│  │  ┌─ Add member ───────────────────────────────────────────────────────────────┐    │       │
│  │  │  Search user:  [li.si@company.com      ] 🔍                                │    │       │
│  │  │                                                                            │    │       │
│  │  │  Role:         [Member ▼]                                                  │    │       │
│  │  │                                                                            │    │       │
│  │  │  Available roles:                                                          │    │       │
│  │  │    • Owner  - full control (transfer ownership)                             │    │       │
│  │  │    • Admin  - manage members, create/delete services and VMs                 │    │       │
│  │  │    • Member - create services and VMs, cannot manage members                 │    │       │
│  │  │    • Viewer - read-only access                                              │    │       │
│  │  │                                                                            │    │       │
│  │  │  [Add]  [Cancel]                                                            │    │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘    │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 Database design (resource-level permissions):                                            │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Resource role bindings table (distinct from global role_bindings)              │       │
│  │  CREATE TABLE resource_role_bindings (                                            │       │
│  │    id VARCHAR PRIMARY KEY,                                                        │       │
│  │    user_id VARCHAR NOT NULL,                                                      │       │
│  │    role VARCHAR NOT NULL,          -- owner, admin, member, viewer                │       │
│  │    resource_type VARCHAR NOT NULL, -- system, service, vm                         │       │
│  │    resource_id VARCHAR NOT NULL,   -- resource ID                                 │       │
│  │    granted_by VARCHAR NOT NULL,    -- grantor                                     │       │
│  │    created_at TIMESTAMP                                                           │       │
│  │  );                                                                               │       │
│  │                                                                                    │       │
│  │  -- Example: Zhang San adds Li Si as Admin for system shop                         │       │
│  │  INSERT INTO resource_role_bindings                                               │       │
│  │    (id, user_id, role, resource_type, resource_id, granted_by, created_at)        │       │
│  │  VALUES                                                                           │       │
│  │    ('rrb-001', 'user-002', 'admin', 'system', 'sys-001', 'user-001', NOW());       │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔍 Permission inheritance model (see Google Cloud IAM, GitHub Teams):                       │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  ⭐ Core principle: child resources fully inherit parent permissions                │       │
│  │                                                                                    │       │
│  │  ┌─ Configure permissions once at System level ────────────────────────────────┐   │       │
│  │  │                                                                            │   │       │
│  │  │  System (shop)                ← add members here                           │   │       │
│  │  │    ├─ Admin: Li Si                                                       │   │       │
│  │  │    ├─ Member: Wang Wu, Zhao Liu                                           │   │       │
│  │  │    │                                                                       │   │       │
│  │  │    ├── Service (redis)        ← inherits Li/Wang/Zhao                       │   │       │
│  │  │    │     ├── VM (redis-01)    ← inherits                                    │   │       │
│  │  │    │     └── VM (redis-02)    ← inherits                                    │   │       │
│  │  │    │                                                                       │   │       │
│  │  │    └── Service (mysql)        ← inherits                                    │   │       │
│  │  │          └── VM (mysql-01)    ← inherits                                    │   │       │
│  │  │                                                                            │   │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘   │       │
│  │                                                                                    │       │
│  │  ✅ Benefits:                                                                       │       │
│  │    - Add/remove members once at System; Service/VM update automatically             │       │
│  │    - Avoid maintaining memberships for many Services/VMs                            │       │
│  │    - Consistent with Google Cloud IAM / GitHub inheritance model                    │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔍 Permission check algorithm:                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  User requests access to resource R:                                                │       │
│  │                                                                                    │       │
│  │  1. Global permission check:                                                       │       │
│  │     - Has platform:admin permission → allow immediately (explicit super-admin)          │       │
│  │                                                                                    │       │
│  │  2. Resource-level permission check (walk inheritance chain):                      │       │
│  │     ┌──────────────────────────────────────────────────────────────────────────┐  │       │
│  │     │  Access VM (vm-001):                                                     │  │       │
│  │     │    1. Check vm-001 resource_role_binding → none                          │  │       │
│  │     │    2. Up to Service (svc-001) binding → none                             │  │       │
│  │     │    3. Up to System (sys-001) binding → found! role=member                │  │       │
│  │     │    4. Return role=member perms → ✅ allow view                           │  │       │
│  │     └──────────────────────────────────────────────────────────────────────────┘  │       │
│  │                                                                                    │       │
│  │  Pseudocode:                                                                       │       │
│  │  ```                                                                              │       │
│  │  func checkPermission(user, resource) Role:                                       │       │
│  │      current = resource                                                           │       │
│  │      while current != nil:                                                        │       │
│  │          binding = findBinding(user, current)                                     │       │
│  │          if binding != nil:                                                       │       │
│  │              return binding.role                                                  │       │
│  │          current = current.parent  // VM→Service→System→nil                       │       │
│  │      return nil  // no permission, resource invisible                             │       │
│  │  ```                                                                              │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 Permission matrix (roles inherited from System):                                         │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │     ┌────────────┬────────┬────────┬────────┬────────┐                             │       │
│  │     │ Action     │ Owner  │ Admin  │ Member │ Viewer │                             │       │
│  │     ├────────────┼────────┼────────┼────────┼────────┤                             │       │
│  │     │ View       │   ✅   │   ✅   │   ✅   │   ✅   │                             │       │
│  │     │ Create     │   ✅   │   ✅   │   ✅   │   ❌   │                             │       │
│  │     │ Update     │   ✅   │   ✅   │   ❌   │   ❌   │                             │       │
│  │     │ Delete     │   ✅   │   ✅   │   ❌   │   ❌   │                             │       │
│  │     │ Manage members │ ✅ │   ✅   │   ❌   │   ❌   │  ← only at System level      │       │
│  │     │ Transfer ownership │ ✅ │ ❌  │   ❌   │   ❌   │                             │       │
│  │     └────────────┴────────┴────────┴────────┴────────┘                             │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  💡 Design notes:                                                                           │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  • Service and VM layers do not manage members separately; inherit from System    │       │
│  │  • Manage permissions at System scope to reduce ops complexity                     │       │
│  │  • For finer isolation, split resources into different Systems                     │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ⚠️ Permission boundary:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  Shepherd platform governs:                                                        │       │
│  │    ✅ Who can see these VMs (visibility)                                            │       │
│  │    ✅ Who can create/start/stop/delete VMs (lifecycle)                              │       │
│  │    ✅ Who can access via VNC console (web console)                                  │       │
│  │                                                                                    │       │
│  │  Shepherd does NOT govern:                                                         │       │
│  │    ❌ Who can SSH/RDP into VMs (handled by bastion/enterprise control)              │       │
│  │    ❌ VM internal user/permission management (handled by OS)                        │       │
│  │                                                                                    │       │
│  │  Typical enterprise architecture:                                                  │       │
│  │    User → Bastion (auth/audit/record) → VM                                         │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 4.B: User Creates Service                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  User actions:                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  Create Service                                                                     │       │
│  │                                                                                    │       │
│  │  System:        [shop ▼]                                                            │       │
│  │  Service name:  [redis              ]    👈 unique within System, max 15 chars      │       │
│  │  Description:   [Cache service        ]    👈 Markdown supported                    │       │
│  │               [Preview] [Upload .md file]    ← or upload existing Markdown file     │       │
│  │                                                                                    │       │
│  │  [Create]                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 Database operations (single transaction):                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Create service                                                             │       │
│  │  INSERT INTO services (id, name, description, system_id, created_by, created_at)  │       │
│  │  VALUES ('svc-001', 'redis', 'Cache service', 'sys-001', 'zhang.san', NOW());      │       │
│  │                                                                                    │       │
│  │  -- 2. Permissions inherit from System (no extra RoleBinding)                      │       │
│  │                                                                                    │       │
│  │  -- 3. 📝 Audit log                                                                │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id,            │       │
│  │                          parent_type, parent_id, details) VALUES                  │       │
│  │    ('service.create', 'zhang.san', 'service', 'svc-001', 'system', 'sys-001',      │       │
│  │     '{"name": "redis", "description": "Cache service"}');                  │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ✅ No approval required: system members can create services                                 │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 3: VM Lifecycle Flow

> **Note**: This section describes the full VM lifecycle: request → approval → execution → running → deletion.
>
> **⚠️ ADR-0017 Responsibility Boundary**:
>
> | Field Category | Provided By | Forbidden For User | Rationale |
> |----------------|-------------|-------------------|-----------|
> | **ServiceID, TemplateID, Namespace** | ✅ User | - | Business context, user's domain |
> | **ClusterID** | ❌ User | ✅ Forbidden | Admin determines during approval |
> | **Name** | ❌ User | ✅ Forbidden | Platform-generated (`{ns}-{sys}-{svc}-{idx}`) |
> | **Labels** | ❌ User | ✅ Forbidden | Platform-managed for governance integrity |
> | **CloudInit** | ❌ User | ✅ Forbidden | Template-defined, security-controlled |
>
> See [ADR-0017 §Decision](../../adr/ADR-0017-vm-request-flow-clarification.md) for complete rationale.

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 5.A: User Submits VM Request                                       │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Regular user:                                                                               │
│                                                                                              │
│  ┌─ Submit VM Create Request ─────────────────────────────────────────────────────────────┐ │
│  │                                                                                        │ │
│  │  UI shown to user:                                                                   │ │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Create Virtual Machine                                                         │ │ │
│  │  │                                                                                │ │ │
│  │  │  Service:       [shop / redis ▼]                                                │ │ │
│  │  │  Namespace:     [prod-shop ▼]                                                   │ │ │
│  │  │  Template:      [centos7-docker ▼]                                              │ │ │
│  │  │                                                                                │ │ │
│  │  │  InstanceSize:  [gpu-workstation ▼]                                             │ │ │
│  │  │                                                                                │ │ │
│  │  │  ┌── InstanceSize details ──────────────────────────────────────────────────┐  │ │
│  │  │  │  CPU: 8 cores   Memory: 32 GB                                            │  │ │
│  │  │  │  ⚠️ This size includes GPU: nvidia.com/GA102GL_A10                        │  │ │
│  │  │  │     Confirm your workload needs GPU resources.                           │  │ │
│  │  │  └──────────────────────────────────────────────────────────────────────────┘  │ │
│  │  │                                                                                │ │ │
│  │  │  ── Quick config ──                                                            │ │ │
│  │  │  Disk size:    [====●==========] [100] GB   👈 default from InstanceSize       │ │ │
│  │  │                50 ─────────── 500           adjust by slider or input         │ │ │
│  │  │                                                                                │ │ │
│  │  │  Reason:       [Production deployment]                                         │ │ │
│  │  │                                                                                │ │ │
│  │  │  [Submit Request]                                                              │ │ │
│  │  └────────────────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                                        │ │
│  │  👆 InstanceSize dropdown shows key info:                                             │ │
│  │     - Standard: "medium (4 cores 8GB)" → show CPU+memory                            │ │
│  │     - GPU size: "gpu-workstation (8 cores 32GB)" + ⚠️ GPU notice                    │ │
│  │                                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 5.B: Admin Approval                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin:                                                                             │
│                                                                                              │
│  System extracts resource requirements from InstanceSize.spec_overrides and matches clusters:
│                                                                                              │
│  1. Extract requirements:                                                                    │
│     - GPU: nvidia.com/GA102GL_A10                                                           │
│     - Hugepages: hugepages-2Mi                                                              │
│                                                                                              │
│  2. Match clusters:                                                                          │
│     - Cluster-A: supports nvidia.com/GA102GL_A10, hugepages-2Mi → ✅ match                   │
│     - Cluster-B: no GPU support → ❌ filtered                                                │
│                                                                                              │
│  3. Admin approval UI:                                                                       │
│                                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  Approve VM Request                                                                    │ │
│  │                                                                                       │ │
│  │  Request details:                                                                     │ │
│  │  ──────────────────────────────────────────────────────────────────────────────────  │ │
│  │  Requester:     zhang.san                                                              │ │
│  │  Namespace:     prod-shop              👈 production env                                │ │
│  │  Service:       shop/redis                                                         │ │
│  │  InstanceSize:  gpu-workstation (8 cores 32GB)                                        │ │
│  │                                                                                       │ │
│  │  ── Disk config ───────────────────────────────────────────────────────────────────  │ │
│  │  Disk size:     [100     ] GB   (requested: 100GB, range: 50-500GB)                   │ │
│  │                                                                                       │ │
│  │  ── Resource allocation (shown if overcommit enabled; can override) ───────────────  │ │
│  │                                                                                       │ │
│  │  [✓] Enable override    👈 admin can override default request/limit                    │ │
│  │                                                                                       │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────┐ │ │
│  │  │                                                                                │ │ │
│  │  │  CPU:    Request [4    ] cores   Limit [8    ] cores                             │ │ │
│  │  │  Memory: Request [16Gi ]       Limit [32Gi ]                                      │ │ │
│  │  │                                                                                │ │ │
│  │  │  ⚠️ Warning: overcommit enabled in prod!   👈 prod-only warning                    │ │ │
│  │  │     High load may impact VM performance.                                          │ │ │
│  │  │                                                                                │ │ │
│  │  │  ❌ ERROR: dedicated CPU + overcommit incompatible! ²                               │ │ │
│  │  │     VM CANNOT start. Approval blocked. Fix: disable overcommit OR dedicated CPU.   │ │ │
│  │  │                                                                                │ │ │
│  │  └──────────────────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                                       │ │
│  │  Cluster:   [cluster-a ▼]     👈 non-matching clusters already filtered               │ │
│  │                                                                                       │ │
│  │  [Approve]  [Reject]                                                                  │ │
│  │                                                                                       │ │
│  └────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                              │
│  👆 Display logic:                                                                           │
│     - Disk config: always shown; admin can adjust                                           │
│     - Resource allocation (request/limit): shown when size enables overcommit               │
│                                                                                              │
│  👆 Validation logic:                                                                        │
│     1. request ≠ limit and env=prod → ⚠️ yellow warning (informational only)                 │
│     2. overcommit + dedicated CPU → ❌ ERROR (blocking) ²                                     │
│        KubeVirt requires requests.cpu == limits.cpu for dedicatedCpuPlacement (Guaranteed QoS)│
│                                                                                              │
│  ² **Technical Constraint**: For `dedicatedCpuPlacement` to work, KubeVirt requires          │
│    Guaranteed QoS class, meaning CPU request must equal limit. This is a hard K8s/KubeVirt   │
│    constraint and cannot be bypassed. See KubeVirt compute documentation.                   │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 5.C: VM Creation Execution                                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  System auto-exec:                                                                           │
│                                                                                              │
│  1. Generate VM name: prod-shop-shop-redis-01                                                │
│                                                                                              │
│  2. Merge final YAML:                                                                        │
│     Template (base) + InstanceSize.spec_overrides + user params (disk_gb)                    │
│                                                                                              │
│  3. Render output:                                                                           │
│     apiVersion: kubevirt.io/v1                                                               │
│     kind: VirtualMachine                                                                     │
│     spec:                                                                                    │
│       template:                                                                              │
│         spec:                                                                                │
│           domain:                                                                            │
│             cpu:                                                                             │
│               cores: 8                                   ← from spec_overrides               │
│               dedicatedCpuPlacement: true                ← from spec_overrides               │
│             memory:                                                                          │
│               hugepages:                                                                     │
│                 pageSize: 2Mi                            ← from spec_overrides               │
│             devices:                                                                         │
│               gpus:                                                                          │
│                 - name: gpu1                             ← from spec_overrides               │
│                   deviceName: nvidia.com/GA102GL_A10                                        │
│                                                                                              │
│  4. Submit to K8s cluster                                                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Parameter Source Summary

| Parameter | Provided By | Source | Notes |
|------|--------|------|------|
| **Schema field types/options** | KubeVirt official | JSON Schema | Developer does not define; use official Schema |
| **Mask paths** | Developer | config/mask.yaml | Select exposed paths only |
| **InstanceSize values** | Admin | Admin UI (schema-driven) | Stored as spec_overrides |
| **Cluster/StorageClass** | Admin | Selected during approval | System filters eligible clusters |
| **VM Name/Labels** | System | Auto-generated | User cannot alter |

### Key Differences From Previous Design

| Area | Before (wrong) | Now (correct) |
|------|---------------|--------------|
| **Field options source** | Developer-defined in Mask | KubeVirt official Schema |
| **Storage structure** | `requirements map[string]string` | `spec_overrides map[string]interface{}` |
| **UI rendering** | Predefined dropdown options | Frontend renders by Schema type |
| **Backend responsibility** | KV subset matching | Store JSON, extract resources for matching |

---

### Stage 5.A (continued): VM Request - Database Operations

> **Note**: DB transaction after user submits VM request
>
> **⚠️ ADR Compliance**:
> - [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md): DomainEvent must be created in same transaction; **payload is immutable** (modifications via `ApprovalTicket.modified_spec` only)
> - [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md): Atomic Ent + sqlc transaction; **do not mix `tx` (sqlc) and `entTx` (Ent) contexts**
>
> **Audit Logs vs Domain Events**:
> - `audit_logs`: Human-readable compliance records (WHO did WHAT, WHEN)
> - `domain_events`: Machine-readable state transitions (system replay/projection)
> Both are required and serve distinct purposes.
>
> **⚠️ SQL Examples Notice**: SQL examples below are illustrative. Always refer to [Ent Schema definitions](../phases/01-contracts.md) for current field requirements. Use `go generate ./ent` to regenerate code after schema changes.
>
> **Security References**:
> - **Audit Log Sensitive Data Redaction**: See [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging) for redaction rules (ADR-0019)
> - **Secrets Table Access Control**: See [01-contracts.md §System Secrets Table](../phases/01-contracts.md#322-system-secrets-table-adr-0025) — DB roles only, no admin UI/API exposure (ADR-0025)

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     VM Request Submission - Database Operations                              │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  User clicks [Submit Request]:                                                               │
│                                                                                              │
│  📦 Database operations (single transaction - ADR-0012):                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  -- Pre-check: Duplicate Request Prevention (ADR-0015 §10)                         │       │
│  │  -- Same resource + same operation type cannot have duplicate pending requests     │       │
│  │  SELECT EXISTS(                                                                    │       │
│  │      SELECT 1 FROM approval_tickets                                               │       │
│  │      WHERE service_id = 'svc-001'                                                 │       │
│  │        AND type = 'VM_CREATE'                                                     │       │
│  │        AND status = 'PENDING_APPROVAL'                                             │       │
│  │  ) AS has_pending;                                                                 │       │
│  │                                                                                    │       │
│  │  -- If has_pending = true:                                                         │       │
│  │  --   Return error: DUPLICATE_PENDING_REQUEST                                      │       │
│  │  --   Response: {"code": "DUPLICATE_PENDING_REQUEST",                              │       │
│  │  --              "existing_ticket_id": "TKT-xxx", "operation": "VM_CREATE"}        │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 Main transaction (only if no duplicate):                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Create domain event (ADR-0009) 👈 REQUIRED                                  │       │
│  │  INSERT INTO domain_events (                                                      │       │
│  │      id, type, aggregate_type, aggregate_id,                                       │       │
│  │      payload, status, created_at                                                   │       │
│  │  ) VALUES (                                                                        │       │
│  │      'evt-001',                                                                    │       │
│  │      'VM_CREATE_REQUESTED',             👈 event type                              │       │
│  │      'vm', NULL,                        👈 aggregate (VM not yet created)          │       │
│  │      '{\"service_id\": \"svc-001\", \"instance_size_id\": \"is-gpu\"...}',       │       │
│  │      'PENDING',                         👈 awaiting approval (ADR-0009 L156)       │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 2. Create approval ticket (linked to event)                                    │       │
│  │  INSERT INTO approval_tickets (                                                   │       │
│  │      id, event_id, type, status, requester_id,                                    │       │
│  │      service_id, namespace, instance_size_id, template_id,                        │       │
│  │      request_params, reason, created_at                                           │       │
│  │  ) VALUES (                                                                        │       │
│  │      'ticket-001',                                                                │       │
│  │      'evt-001',                         👈 link to domain event                    │       │
│  │      'VM_CREATE',                                                                 │       │
│  │      'PENDING_APPROVAL',                👈 initial status                          │       │
│  │      'zhang.san',                                                                 │       │
│  │      'svc-001',                                                                   │       │
│  │      'prod-shop',                                                                 │       │
│  │      'is-gpu-workstation',                                                        │       │
│  │      'tpl-centos7',                                                               │       │
│  │      '{\"disk_gb\": 100}',               👈 user-adjustable params                │       │
│  │      'Production deployment',                                                     │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 3. Audit log (human-readable compliance)                                       │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      id, action, actor_id, resource_type, resource_id, details, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'log-001', 'REQUEST_SUBMITTED', 'zhang.san',                                  │       │
│  │      'approval_ticket', 'ticket-001',                                              │       │
│  │      '{\"action\": \"VM_CREATE\", \"namespace\": \"prod-shop\"}',                │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 4. Notify admins via NotificationSender interface (ADR-0015 §20)               │       │
│  │  --    V1: InboxNotificationSender (platform-internal inbox)                        │       │
│  │  --    V2+: External adapters (Email, Webhook, Slack) via plugin layer              │       │
│  │  INSERT INTO notifications (                                                      │       │
│  │      id, recipient_role, type, title, content, related_ticket_id, created_at      │       │
│  │  ) VALUES (                                                                        │       │
│  │      'notif-001', 'admin', 'APPROVAL_REQUIRED',                                    │       │
│  │      'New VM request', 'User zhang.san requested VM...',                           │       │
│  │      'ticket-001', NOW()                                                          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 State transition:                                                                       │
│     - ApprovalTicket: (none) → PENDING_APPROVAL                                              │
│     - DomainEvent: (none) → PENDING                                                          │
│                                                                                              │
│  🚫 Note: NO River Job inserted at this stage (awaiting approval)                           │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### Stage 5.B (continued): Admin Approval - Database Operations

> **Note**: DB transaction after admin approves/rejects request
>
> **⚠️ ADR Compliance**:
> - [ADR-0006](../../adr/ADR-0006-unified-async-model.md): River Job must be inserted in same transaction
> - [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md): DomainEvent status must be updated
> - [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md): Atomic Ent + sqlc + River InsertTx

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Admin Approves VM Request - Database Operations                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Admin clicks [Approve]:                                                                     │
│                                                                                              │
│  📦 Database operations (single transaction - ADR-0012):                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Update ticket status                                                       │       │
│  │  UPDATE approval_tickets SET                                                      │       │
│  │      status = 'APPROVED',                  👈 PENDING → APPROVED                   │       │
│  │      approver_id = 'admin.li',                                                    │       │
│  │      approved_at = NOW(),                                                         │       │
│  │      selected_cluster_id = 'cluster-a',     👈 admin-selected cluster (ADR-0017)    │       │
│  │      selected_storage_class = 'ceph-rbd',   👈 admin-selected storage class          │       │
│  │      template_snapshot = '{...}',          👈 template snapshot (ADR-0015 §17)     │       │
│  │      instance_size_snapshot = '{...}',     👈 InstanceSize snapshot (ADR-0018)     │       │
│  │      final_cpu_request = '4',              👈 final CPU request (after overcommit)│       │
│  │      final_cpu_limit = '8',                                                       │       │
│  │      final_mem_request = '16Gi',           👈 final memory request                 │       │
│  │      final_mem_limit = '32Gi',                                                    │       │
│  │      final_disk_gb = 100                   👈 final disk size                      │       │
│  │  WHERE id = 'ticket-001';                                                         │       │
│  │                                                                                    │       │
│  │  -- 2. Update domain event status (ADR-0009) 👈 REQUIRED                           │       │
│  │  UPDATE domain_events SET                                                         │       │
│  │      status = 'PROCESSING',               👈 PENDING → PROCESSING                  │       │
│  │      updated_at = NOW()                                                           │       │
│  │  WHERE id = 'evt-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 3. Generate VM name and create VM record                                       │       │
│  │  INSERT INTO vms (                                                                │       │
│  │      id, name, service_id, namespace, cluster_id,                                 │       │
│  │      instance_size_id, template_id, status,                                       │       │
│  │      ticket_id, tenant_id, created_at                                             │       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm-001',                                                                    │       │
│  │      'prod-shop-shop-redis-01',            👈 auto: {ns}-{sys}-{svc}-{index}        │       │
│  │      'svc-001', 'prod-shop', 'cluster-a',                                         │       │
│  │      'is-gpu-workstation', 'tpl-centos7',                                         │       │
│  │      'CREATING',                           👈 initial status: creating              │       │
│  │      'ticket-001', 'default', NOW()        👈 tenant_id default (ADR-0015)          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 4. Insert River Job (ADR-0006/0009) 👈 REQUIRED - triggers async execution     │       │
│  │  -- ⚠️ Claim Check Pattern: Job args contain ONLY event_id (ADR-0009)              │       │
│  │  INSERT INTO river_job (                                                          │       │
│  │      id, kind, args, queue, state, created_at                                     │       │
│  │  ) VALUES (                                                                        │       │
│  │      'job-001',                                                                   │       │
│  │      'VMCreateJob',                        👈 River worker type                     │       │
│  │      '{"event_id": "evt-001"}',           👈 Claim Check: event_id ONLY (ADR-0009) │       │
│  │      'default',                                                                   │       │
│  │      'available',                          👈 ready for worker consumption          │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │  -- Note: Use riverClient.InsertTx() in code, NOT raw INSERT                       │       │
│  │                                                                                    │       │
│  │  -- 5. Audit log                                                                   │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      id, action, actor_id, resource_type, resource_id, details, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'log-002', 'REQUEST_APPROVED', 'admin.li',                                    │       │
│  │      'approval_ticket', 'ticket-001',                                              │       │
│  │      '{"cluster": "cluster-a", "vm_name": "prod-shop-shop-redis-01"}',           │       │
│  │      NOW()                                                                        │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 6. Notify user                                                                 │       │
│  │  INSERT INTO notifications (                                                      │       │
│  │      id, recipient_id, type, title, content, related_ticket_id, created_at        │       │
│  │  ) VALUES (                                                                        │       │
│  │      'notif-002', 'zhang.san', 'REQUEST_APPROVED',                                 │       │
│  │      'Your VM request is approved', 'VM prod-shop-shop-redis-01 is creating...',  │       │
│  │      'ticket-001', NOW()                                                          │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 State transitions:                                                                       │
│     - ApprovalTicket: PENDING_APPROVAL → APPROVED                                            │
│     - DomainEvent: PENDING → PROCESSING                                                      │
│     - VM: (none) → CREATING                                                                  │
│     - RiverJob: (none) → available                                                           │
│                                                                                              │
│  🔄 Async execution: River worker picks up job and calls KubeVirt API                        │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Admin Rejects VM Request - Database Operations                           │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Admin clicks [Reject]:                                                                      │
│                                                                                              │
│  📦 Database operations (single transaction - ADR-0012):                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Update ticket status                                                       │       │
│  │  UPDATE approval_tickets SET                                                      │       │
│  │      status = 'REJECTED',                  👈 PENDING → REJECTED                   │       │
│  │      approver_id = 'admin.li',                                                    │       │
│  │      rejected_at = NOW(),                                                         │       │
│  │      rejection_reason = 'Insufficient resources, choose another size'             │       │
│  │  WHERE id = 'ticket-001';                                                         │       │
│  │                                                                                    │       │
│  │  -- 2. Update domain event status (ADR-0009) 👈 REQUIRED                           │       │
│  │  UPDATE domain_events SET                                                         │       │
│  │      status = 'CANCELLED',                👈 PENDING → CANCELLED (rejected)        │       │
│  │      updated_at = NOW()                                                           │       │
│  │  WHERE id = 'evt-001';                                                            │       │
│  │                                                                                    │       │
│  │  -- 3. Audit log                                                                   │       │
│  │  INSERT INTO audit_logs (...) VALUES (...);                                       │       │
│  │                                                                                    │       │
│  │  -- 4. Notify user                                                                 │       │
│  │  INSERT INTO notifications (...) VALUES (...);                                    │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📊 State transitions:                                                                       │
│     - ApprovalTicket: PENDING_APPROVAL → REJECTED                                            │
│     - DomainEvent: PENDING → CANCELLED                                                       │
│  ❌ No VM record created, no River Job inserted                                              │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### Stage 5.D: Delete Operations

> **Note**: VM/Service/System delete flows and DB operations

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Delete Flow - Hierarchical Dependencies                                  │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Hierarchy (see ADR-0015):                                                                   │
│                                                                                              │
│      System (shop)                                                                           │
│         │                                                                                    │
│         ├── Service (redis)                                                                  │
│         │      ├── VM (prod-shop-shop-redis-01)                                              │
│         │      └── VM (prod-shop-shop-redis-02)                                              │
│         │                                                                                    │
│         └── Service (mysql)                                                                  │
│                └── VM (prod-shop-shop-mysql-01)                                              │
│                                                                                              │
│  Delete rules (Cascade Restrict - ADR-0015 §13.1):                                           │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │                                                                                    │       │
│  │  Level        Precondition                  Approval   Confirmation                │       │
│  │  ────────────────────────────────────────────────────────────────────────────────  │       │
│  │  VM (test)    None                          ✅ Yes     confirm=true param           │       │
│  │  VM (prod)    None                          ✅ Yes     confirm_name in body ¹       │       │
│  │  Service      All VMs deleted first         ✅ Yes     confirm=true param           │       │
│  │  System       All Services deleted first    ❌ No      confirm_name in body         │       │
│  │                                                                                    │       │
│  │  ¹ Production VMs require typing the exact VM name to prevent accidental deletion  │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Delete VM - Database Operations                                          │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  🔹 Test VM Delete (simple confirmation):                                                    │
│  DELETE /api/v1/vms/{vm_id}?confirm=true                                                     │
│                                                                                              │
│  🔹 Production VM Delete (requires typing VM name - ADR-0015 §13.1):                         │
│  DELETE /api/v1/vms/{vm_id}                                                                  │
│  Content-Type: application/json                                                              │
│  { "confirm_name": "prod-shop-shop-redis-01" }  👈 must match VM name exactly                │
│                                                                                              │                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Create delete approval ticket                                              │       │
│  │  INSERT INTO approval_tickets (                                                   │       │
│  │      id, type, status, requester_id, resource_type, resource_id, created_at       │       │
│  │  ) VALUES (                                                                        │       │
│  │      'ticket-002', 'VM_DELETE', 'PENDING_APPROVAL',                               │       │
│  │      'zhang.san', 'vm', 'vm-001', NOW()                                           │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- 2. Audit log                                                                   │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm.delete_request', 'zhang.san', 'vm', 'vm-001', 'service', 'svc-001',       │       │
│  │      '{"name": "prod-shop-shop-redis-01", "reason": "resource cleanup"}'     │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  After admin approval:                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Update ticket status                                                       │       │
│  │  UPDATE approval_tickets SET status = 'APPROVED', ... WHERE id = 'ticket-002';    │       │
│  │                                                                                    │       │
│  │  -- 2. Update VM status to DELETING (no hard delete)                               │       │
│  │  UPDATE vms SET status = 'DELETING' WHERE id = 'vm-001';                           │       │
│  │                                                                                    │       │
│  │  -- 3. Audit log                                                                   │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'vm.delete', 'admin.li', 'vm', 'vm-001', 'service', 'svc-001',                │       │
│  │      '{"name": "prod-shop-shop-redis-01", "approved_by": "admin.li"}'         │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  🔄 Async task: worker runs kubectl delete vm; on success set status='DELETED'               │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Delete Service - Database Operations                                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  DELETE /api/v1/services/{service_id}?confirm=true                                           │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Pre-check: active VM count                                                    │       │
│  │  SELECT COUNT(*) FROM vms                                                         │       │
│  │  WHERE service_id = 'svc-001' AND status NOT IN ('DELETED', 'DELETING');           │       │
│  │                                                                                    │       │
│  │  IF count > 0 THEN                                                                │       │
│  │      RETURN ERROR("{count} active VMs exist under service; delete them first");   │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- Create delete approval ticket (same as VM delete flow)                         │       │
│  │  INSERT INTO approval_tickets (...);                                              │       │
│  │                                                                                    │       │
│  │  -- Audit log                                                                      │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'service.delete_request', 'zhang.san', 'service', 'svc-001', 'system', 'sys-001',│     │
│  │      '{"name": "redis", "reason": "service migration"}'                      │       │
│  │  );                                                                                │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  After admin approval:                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  UPDATE services SET status = 'DELETED', deleted_at = NOW()                        │       │
│  │  WHERE id = 'svc-001';                                                             │       │
│  │                                                                                    │       │
│  │  -- Audit log                                                                      │       │
│  │  INSERT INTO audit_logs (                                                         │       │
│  │      action, actor_id, resource_type, resource_id, parent_type, parent_id, details│       │
│  │  ) VALUES (                                                                        │       │
│  │      'service.delete', 'admin.li', 'service', 'svc-001', 'system', 'sys-001',       │       │
│  │      '{"name": "redis", "approved_by": "admin.li"}'                            │       │
│  │  );                                                                                │       │
│  │                                                                                    │       │
│  │  -- Soft delete: record preserved for audit                                        │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Delete System - Database Operations (No Approval)                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  DELETE /api/v1/systems/{system_id}                                                          │
│  Body: { "confirm_name": "shop" }    👈 must type system name                              │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  -- Pre-check 1: confirm name match                                               │       │
│  │  IF confirm_name != system.name THEN                                              │       │
│  │      RETURN ERROR("confirmation name mismatch");                                  │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- Pre-check 2: active Service count                                              │       │
│  │  SELECT COUNT(*) FROM services                                                    │       │
│  │  WHERE system_id = 'sys-001' AND status != 'DELETED';                              │       │
│  │                                                                                    │       │
│  │  IF count > 0 THEN                                                                │       │
│  │      RETURN ERROR("{count} services exist under system; delete first");           │       │
│  │  END IF;                                                                           │       │
│  │                                                                                    │       │
│  │  -- Execute soft delete (no approval)                                              │       │
│  │  UPDATE systems SET status = 'DELETED', deleted_at = NOW()                         │       │
│  │  WHERE id = 'sys-001';                                                             │       │
│  │                                                                                    │       │
│  │  -- Audit log                                                                      │       │
│  │  INSERT INTO audit_logs (...) VALUES (...);                                        │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ❌ No approval ticket: system deletion guarded by name confirmation only                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### Stage 5.E: Batch Operations (ADR-0015 §19)

> **Design Reference**: [04-governance.md §5.6](../phases/04-governance.md#56-batch-operations-adr-0015-19)

Batch operations are **UX convenience**, not atomic transactions. Each item is processed independently via River Queue.

> **Idempotency & Retry**:
> - Each batch item generates an independent River Job with unique `event_id`
> - River handles retry logic (default: 3 retries with exponential backoff)
> - Idempotency key = `event_id` — re-processing same event is safe (ADR-0009 Claim Check)
> - Partial failures do NOT rollback successful items; aggregate status reported

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ BATCH APPROVAL FLOW                                                                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. Admin selects multiple pending tickets in UI                                                │
│     └── [TKT-001, TKT-002, TKT-003] (max 50)                                                    │
│                                                                                                  │
│  2. Frontend: POST /api/v1/approvals/batch                                                      │
│     └── {ticket_ids: [...], action: "approve", cluster_id: "...", reason: "..."}               │
│                                                                                                  │
│  3. Backend validates each ticket independently:                                                 │
│     ┌─────────────────────────────────────────────────────────────────────────────────────┐     │
│     │ FOR each ticket_id IN request.ticket_ids:                                           │     │
│     │   • Check ticket exists and status = PENDING_APPROVAL                               │     │
│     │   • Check user has approval permission                                              │     │
│     │   • Check target cluster matches ticket's environment                               │     │
│     │   • IF valid → Enqueue River job (ApprovalJob)                                      │     │
│     │   • IF invalid → Mark as rejected in response                                       │     │
│     └─────────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                                  │
│  4. Response (202 Accepted):                                                                    │
│     └── {batch_id: "BATCH-123", total: 3, accepted: 2, rejected: 1, items: [...]}              │
│                                                                                                  │
│  5. Frontend can poll: GET /api/v1/batches/{batch_id}/status                                    │
│                                                                                                  │
│  ⚠️ Batch Delete NOT supported in V1 (ADR-0015 §13.1 requires individual confirmation)          │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ BATCH POWER OPERATION FLOW                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. User selects multiple VMs in UI                                                             │
│     └── [vm-001, vm-002] (max 50)                                                               │
│                                                                                                  │
│  2. Frontend: POST /api/v1/vms/batch/power                                                      │
│     └── {vm_ids: [...], action: "start", reason: "..."}                                        │
│                                                                                                  │
│  3. Backend:                                                                                    │
│     • Validate user has vm:operate permission for each VM                                       │
│     • For prod environment VMs, check if approval required (ADR-0015 §7)                        │
│     • Enqueue individual River jobs (PowerOperationJob)                                         │
│                                                                                                  │
│  4. Each job executes independently:                                                            │
│     └── Success/failure tracked per VM                                                          │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### Stage 5.F: Notification System (ADR-0015 §20)

> **Design Reference**: [04-governance.md §6.3](../phases/04-governance.md#63-notification-system-adr-0015-20)

V1 implements platform-internal inbox. No external push channels (email/webhook) in V1.

> **ADR-0006 Compliance**: Notification inserts are **synchronous** (within the same DB transaction as business operations), NOT via River Queue. See [04-governance.md §6.3](../phases/04-governance.md#63-notification-system-adr-0015-20) for rationale.
>
> **V2+ External Channels**: Email/Webhook/Slack planned in [RFC-0018](../../rfc/RFC-0018-external-notification.md).

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ NOTIFICATION TRIGGER POINTS                                                                      │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  Event: VM Request Submitted                                                                    │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, body, metadata)               │        │
│  │ SELECT user_id, 'APPROVAL_PENDING', 'New VM request pending approval',              │        │
│  │        'User X submitted a request for VM in namespace Y',                           │        │
│  │        '{"ticket_id": "TKT-001", "requester": "user-a"}'                             │        │
│  │ FROM role_bindings                                                                   │        │
│  │ WHERE role_id IN (SELECT id FROM roles WHERE permissions @> 'approval:approve');    │        │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
│  Event: Request Approved/Rejected                                                               │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, metadata)                     │        │
│  │ VALUES (ticket.requested_by, 'APPROVAL_COMPLETED',                                  │        │
│  │         'Your VM request was approved', '{"ticket_id": "TKT-001"}');                │        │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
│  Event: VM State Changed                                                                        │
│  ────────────────────────────────────────────────────────                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────┐        │
│  │ INSERT INTO notifications (recipient_id, type, title, metadata)                     │        │
│  │ VALUES (vm.owner_id, 'VM_STATUS_CHANGE',                                            │        │
│  │         'VM vm-name-01 is now Running', '{"vm_id": "...", "new_state": "Running"}');│        │
│  └─────────────────────────────────────────────────────────────────────────────────────┘        │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ USER NOTIFICATION INTERACTION                                                                    │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  Frontend Header:                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐                        │
│  │  🔔 (3)  ← Badge shows unread count                                │                        │
│  │    ↓ Poll every 30s: GET /api/v1/notifications/unread-count        │                        │
│  └─────────────────────────────────────────────────────────────────────┘                        │
│                                                                                                  │
│  Click notification bell → Dropdown panel:                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐                        │
│  │  GET /api/v1/notifications?page=1&per_page=10                       │                        │
│  │                                                                     │                        │
│  │  • 🔵 New VM request pending (2 min ago)                           │                        │
│  │  • 🔵 Your request was approved (1 hour ago)                       │                        │
│  │  • VM shop-redis-01 is now Running (3 hours ago)                   │                        │
│  │                                                                     │                        │
│  │  [Mark all as read]  [View all →]                                  │                        │
│  └─────────────────────────────────────────────────────────────────────┘                        │
│                                                                                                  │
│  Mark as read: PATCH /api/v1/notifications/{id}/read                                           │
│  Mark all read: POST /api/v1/notifications/mark-all-read                                       │
│                                                                                                  │
│  ⚠️ V1 Constraint: Poll-based only, no WebSocket push                                           │
│  ⚠️ V1 Constraint: No external channels (email/webhook) - see RFC for V2+                      │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 4: State Machines & Data Models

> **Note**: This section defines state machines and DB relationships for core entities.
> It is a critical reference for frontend and backend development.

### Approval Ticket Status State Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     ApprovalTicket Status Transitions                                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│                        ┌───────────────────┐                                                 │
│                        │  PENDING_APPROVAL │                                                 │
│                        │     (pending)     │                                                 │
│                        └─────────┬─────────┘                                                 │
│                                  │                                                           │
│              ┌───────────────────┼───────────────────┐                                      │
│              │                   │                   │                                      │
│              ▼                   ▼                   ▼                                      │
│     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                                 │
│     │  APPROVED   │     │  REJECTED   │     │  CANCELLED  │                                 │
│     │  (approved) │     │  (rejected) │     │ (cancelled) │                                 │
│     └──────┬──────┘     └─────────────┘     └─────────────┘                                 │
│            │                 (terminal)          (terminal)                                 │
│            ▼                                                                                 │
│     ┌─────────────┐                                                                          │
│     │  EXECUTING  │                                                                          │
│     │ (executing) │                                                                          │
│     └──────┬──────┘                                                                          │
│            │                                                                                 │
│     ┌──────┴──────┐                                                                          │
│     ▼             ▼                                                                          │
│  ┌─────────┐  ┌─────────┐                                                                    │
│  │ SUCCESS │  │ FAILED  │                                                                    │
│  │ (ok)    │  │ (fail)  │                                                                    │
│  └─────────┘  └─────────┘                                                                    │
│    (terminal)   (terminal)                                                                   │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### VM Status State Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     VM Status Transitions                                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                                 │
│     │  CREATING   │────▶│   RUNNING   │◀────│   STOPPED   │                                 │
│     │  (creating) │     │  (running)  │     │  (stopped)  │                                 │
│     └─────────────┘     └──────┬──────┘     └─────────────┘                                 │
│            │                   │                   ▲                                        │
│            │                   ▼                   │                                        │
│            │            ┌─────────────┐            │                                        │
│            │            │  STOPPING   │────────────┘                                        │
│            │            │  (stopping) │                                                     │
│            │            └─────────────┘                                                     │
│            │                                                                                │
│            │                   │                                                            │
│            ▼                   ▼                                                            │
│     ┌─────────────┐     ┌─────────────┐                                                     │
│     │   FAILED    │     │  DELETING   │                                                     │
│     │  (failed)   │     │ (deleting)  │                                                     │
│     └─────────────┘     └──────┬──────┘                                                     │
│                                │                                                            │
│                                ▼                                                            │
│                         ┌─────────────┐                                                     │
│                         │   DELETED   │                                                     │
│                         │  (deleted)  │                                                     │
│                         └─────────────┘                                                     │
│                           (terminal)                                                        │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### Database Table Relationship Overview

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Core Table Relationship Diagram                                          │
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
│                                                 │ final_*          │ ← final values at approval
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

### Audit Log Design

> **Reference**: ADR-0015 §7 (Deletion & Cascade Constraints) - "audit records are preserved"

> 📦 **Schema**: See [04-governance.md §7 Storage Schema](../phases/04-governance.md#storage-schema) for full DDL and indexes.

#### Operations That Must Be Audited

| Category | Action | Trigger | Details (details) |
|------|---------------|----------|---------------------|
| **Auth** | `user.login` | Login success | `{method: "oidc", idp: "Corp-SSO"}` |
| **Auth** | `user.login_failed` | Login failed | `{reason: "invalid_token"}` |
| **Auth** | `user.logout` | Logout | `{}` |
| **System** | `system.create` | Create system | `{name: "shop", description: "..."}` |
| **System** | `system.update` | Update system | `{changes: {description: {old: "...", new: "..."}}}` |
| **System** | `system.delete` | Delete system | `{confirmation: "shop"}` |
| **Service** | `service.create` | Create service | `{name: "redis", system_id: "..."}` |
| **Service** | `service.delete_request` | Submit delete request | `{name: "redis", reason: "service migration"}` |
| **Service** | `service.delete` | Delete service (after approval) | `{approved_by: "..."}` |
| **VM** | `vm.request` | Submit VM create request | `{instance_size: "...", template: "...", count: 3}` |
| **VM** | `vm.create` | VM created | `{cluster: "...", namespace: "..."}` |
| **VM** | `vm.start` | Start VM | `{}` |
| **VM** | `vm.stop` | Stop VM | `{graceful: true}` |
| **VM** | `vm.restart` | Restart VM | `{}` |
| **VM** | `vm.delete_request` | Submit delete request | `{name: "...", reason: "cleanup"}` |
| **VM** | `vm.delete` | Delete VM (after approval) | `{approved_by: "..."}`  |
| **VNC** | `vnc.access` | Access VNC console | `{vm_id: "...", session_duration: 3600}` |
| **Approval** | `approval.approve` | Approve request | `{ticket_id: "...", final_cluster: "...", final_disk_gb: 100}` |
| **Approval** | `approval.reject` | Reject request | `{ticket_id: "...", reason: "insufficient resources"}` |
| **Approval** | `approval.cancel` | Cancel request | `{ticket_id: "...", reason: "no longer needed"}` |
| **RBAC** | `role.create` | Create custom role | `{name: "CustomViewer", permissions: [...]}` |
| **RBAC** | `role.update` | Update role permissions | `{role: "Operator", changes: {permissions: {added: [...], removed: [...]}}}` |
| **RBAC** | `role.delete` | Delete custom role | `{name: "CustomViewer"}` |
| **RBAC** | `role.assign` | Assign role to user | `{user_id: "...", role: "SystemAdmin", scope: "system:shop"}` |
| **RBAC** | `role.revoke` | Revoke role | `{user_id: "...", role: "Operator"}` |
| **RBAC** | `permission.create` | Create permission | `{code: "vm:vnc", description: "..."}` |
| **RBAC** | `permission.delete` | Delete permission | `{code: "vm:vnc"}` |
| **Cluster** | `cluster.register` | Register cluster | `{name: "prod-01", environment: "prod", api_server: "..."}` |
| **Cluster** | `cluster.update` | Update cluster config | `{name: "prod-01", changes: {environment: {old: "test", new: "prod"}}}` |
| **Cluster** | `cluster.delete` | Delete/deregister cluster | `{name: "prod-01", reason: "cluster offboarding"}` |
| **Cluster** | `cluster.credential_rotate` | Rotate cluster credentials | `{name: "prod-01", rotated_at: "..."}` |
| **Template** | `template.create` | Create template | `{name: "centos7-docker", version: 1}` |
| **Template** | `template.update` | Update template (version+1) | `{name: "centos7-docker", version: 2, changes: {...}}` |
| **Template** | `template.deprecate` | Deprecate template | `{name: "centos6-base", successor: "centos7-base"}` |
| **Template** | `template.delete` | Delete template | `{name: "centos6-base", version: 3}` |
| **InstanceSize** | `instance_size.create` | Create size | `{name: "medium-gpu", cpu: 4, memory: "8Gi", gpu: 1}` |
| **InstanceSize** | `instance_size.update` | Update size | `{name: "medium-gpu", changes: {memory: {old: "8Gi", new: "16Gi"}}}` |
| **InstanceSize** | `instance_size.deprecate` | Deprecate size | `{name: "small-legacy"}` |
| **InstanceSize** | `instance_size.delete` | Delete size | `{name: "small-legacy"}` |
| **Namespace** | `namespace.create` | Create namespace | `{name: "prod-shop", cluster: "prod-01"}` |
| **Namespace** | `namespace.delete` | Delete namespace | `{name: "prod-shop"}` |
| **IdP** | `idp.configure` | Configure IdP | `{type: "oidc", issuer: "...", client_id: "..."}` |
| **IdP** | `idp.update` | Update IdP config | `{changes: {issuer: {...}}}` |
| **IdP** | `idp.delete` | Delete IdP config | `{type: "oidc"}` |
| **IdP** | `idp.sync` | Manually sync IdP groups | `{synced_groups: 15, new_users: 3}` |
| **IdP** | `idp.mapping_create` | Create group-role mapping | `{idp_group: "DevOps", role: "SystemAdmin", env: "prod"}` |
| **IdP** | `idp.mapping_update` | Update mapping | `{idp_group: "DevOps", changes: {role: {old: "Viewer", new: "Operator"}}}` |
| **IdP** | `idp.mapping_delete` | Delete mapping | `{idp_group: "DevOps"}` |
| **Config** | `config.update` | Update platform config | `{key: "approval.timeout_hours", old: 24, new: 48}` |

#### Operations That Do NOT Require Audit (Exceptions)

The following operations are high-frequency or low sensitivity and are **not** audited:

| Category | Operation | Reason |
|------|------|-----------|
| **System checks** | K8s cluster health checks | periodic, no user trigger |
| **System checks** | VM status sync polling | every minute, too much data |
| **System checks** | Resource quota checks | internal, low business value |
| **Read-only** | list queries (`GET /api/v1/*`) | read-only, no state change |
| **Read-only** | detail queries (`GET /api/v1/*/id`) | read-only, no state change |
| **Internal** | Worker heartbeats | internal comms |
| **Internal** | Metrics collection | monitoring data |

> **Exception principles**:
> - All **write** operations (CREATE/UPDATE/DELETE) must be logged
> - All **sensitive read** operations (e.g., VNC access) must be logged
> - Pure **system automation** and **read-only queries** may be exempt

#### Audit Log Examples

```
Example 1: User submits VM create request
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, parent_type, parent_id, details) VALUES
    ('vm.request', 'user-001', 'Zhang San', 'approval_ticket', 'ticket-001',
     'service', 'svc-001',
     '{"instance_size": "medium-gpu", "template": "centos7-docker",
       "count": 3, "namespace": "prod-shop"}');

Example 2: Admin approves request
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, details) VALUES
    ('approval.approve', 'admin-001', 'Admin Li Si', 'approval_ticket', 'ticket-001',
     '{"final_cluster": "prod-cluster-01", "final_disk_gb": 100,
       "final_storage_class": "ceph-ssd", "vms_created": 3}');

Example 3: VNC access record
  INSERT INTO audit_logs (action, actor_id, actor_name, resource_type,
                          resource_id, details, ip_address) VALUES
    ('vnc.access', 'user-001', 'Zhang San', 'vm', 'vm-redis-01',
     '{"session_id": "vnc-xxx", "duration_seconds": 1800}',
     '192.168.1.100');

Example 4: Delete resource (preserve audit)
  -- When deleting a VM, write audit log first
  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id,
                          parent_type, parent_id, details) VALUES
    ('vm.delete', 'user-001', 'vm', 'vm-redis-01', 'service', 'svc-001',
     '{"name": "prod-shop-redis-01", "cluster": "prod-cluster-01",
       "existed_days": 45, "last_status": "RUNNING"}');

  -- Then hard delete the resource
  DELETE FROM vms WHERE id = 'vm-redis-01';

  💡 Audit log preserved, resource record removed
```

#### Audit Log Query Examples

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

#### Audit Log Retention Policy

| Environment | Retention | Notes |
|------|----------|------|
| **Production** | ≥ 1 year | Compliance |
| **Test** | ≥ 90 days | Configurable shorter |
| **Sensitive ops** | ≥ 3 years | `*.delete`, `approval.*`, `rbac.*` |

---

### Audit Log JSON Export (v1+)

> **Scenario**: Integrate audit logs into enterprise SIEM (Elasticsearch, Datadog, Splunk, etc.)

> 📦 **API Specification**: See [04-governance.md §7 JSON Export API](../phases/04-governance.md#7-json-export-api) for full API and response format.

**Key Features**:
- Paginated export with time range filtering
- Webhook push integration for real-time streaming
- Structured JSON format compatible with common log aggregators

---

### External Approval System Integration (v1+)

> **Scenario**: integrate with enterprise ITSM (Jira Service Management, ServiceNow, etc.)

#### Design Principles

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     External Approval Integration Architecture                               │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐   │
│  │   Shepherd   │  ──── Webhook ───▶ │ External Sys │  ──── Callback ──▶ │   Shepherd   │   │
│  │   Platform   │                    │ (Jira/SNOW)  │                    │   Platform   │   │
│  └──────────────┘                    └──────────────┘                    └──────────────┘   │
│                                                                                              │
│  Key principles:                                                                             │
│  1. Shepherd focuses on standard APIs, not external workflow internals                        │
│  2. Async event-driven architecture; do not block users                                       │
│  3. External approval is pluggable; v1 defaults to built-in approval                          │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### External Approval Configuration (Web UI, PostgreSQL)

> Admin config via **Settings → External Approval Systems → Add**.
> All configs stored in `external_approval_systems` table.

**Webhook security (best practice)**:
- HTTPS only for all webhook URLs.
- Verify webhook signatures with shared secret and constant-time comparison.
- Include a timestamp in the signed payload and reject stale requests to prevent replay.
- Store webhook secrets encrypted at rest; rotate when compromised.

References:
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

#### Webhook Payload (Shepherd → External System)

```json
// POST https://jira.company.com/api/v2/tickets
{
  "shepherd_ticket_id": "ticket-001",
  "type": "VM_CREATE",
  "callback_url": "https://shepherd.company.com/api/v1/approvals/callback",
  "requester": {
    "id": "zhang.san",
    "name": "Zhang San",
    "email": "zhang.san@company.com"
  },
  "request_details": {
    "namespace": "prod-shop",
    "service": "redis",
    "instance_size": "medium-gpu",
    "template": "centos7-docker",
    "vm_count": 3,
    "reason": "Production deployment"
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

#### Callback Payload (External System → Shepherd)

```json
// POST https://shepherd.company.com/api/v1/approvals/callback
// Headers:
//   X-Shepherd-Signature: HMAC-SHA256 signature
//   Content-Type: application/json
{
  "shepherd_ticket_id": "ticket-001",
  "external_ticket_id": "JIRA-12345",    // external ticket ID (trace)
  "status": "Approved",                   // mapped via status_mapping
  "approver": {
    "id": "admin.li",
    "name": "Admin Li Si"
  },
  "comments": "Resources available, approved",
  "approved_at": "2026-01-26T11:30:00Z"
}
```

#### Shepherd Callback Handling

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Callback Handling Flow                                                   │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  1. Validate HMAC signature                                                                  │
│  2. Lookup ticket by shepherd_ticket_id                                                      │
│  3. Map status via status_mapping                                                            │
│  4. Update ticket status and approver                                                        │
│  5. If APPROVED:                                                                             │
│     a. Trigger VM provisioning worker job                                                    │
│     b. Notify requester                                                                      │
│  6. If REJECTED:                                                                             │
│     a. Record rejection reason                                                               │
│     b. Notify requester                                                                      │
│  7. Record audit log                                                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Integration Notes

| Note | Description |
|----------|------|
| **Idempotency** | Callback may retry; must be safe for duplicates |
| **Status sync** | Periodically check pending tickets in external system |
| **Timeout** | V1: No auto-cancel. External system may call rejection API on timeout (see ADR-0015 §11) |
| **Security** | Always verify HMAC signature to prevent forged callbacks |
| **Fallback** | If external system is unavailable, fall back to built-in approval |

---

## Stage 6: VNC Console Access (ADR-0015 §18, RFC-0011)

> **Scope**: Browser-based VM console access via noVNC.
>
> **ADR-0015 §18 Compliance**:
> - Permission matrix (test/prod environment differentiation)
> - Token security (single-use, time-bounded, user-binding)
> - Audit logging requirements

### Permission Matrix

| Environment | Approval Required | Token TTL | Notes |
|-------------|-------------------|-----------|-------|
| **Test** | ❌ No | 2 hours | RBAC check only (`vnc:access` permission) |
| **Production** | ✅ Yes | 2 hours | Requires approval ticket |

### VNC Access Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 6: VNC Console Access                                              │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌─ Test Environment (No Approval) ─────────────────────────────────────────────────────────┐
│  │                                                                                          │
│  │  1. User clicks [Console] button on VM detail page                                        │
│  │                                                                                          │
│  │  2. Backend checks:                                                                       │
│  │     a. User has `vnc:access` permission on namespace                                     │
│  │     b. VM is in RUNNING state                                                            │
│  │     c. Environment is test (no approval required)                                         │
│  │                                                                                          │
│  │  3. Generate VNC Token (JWT):                                                            │
│  │     {                                                                                     │
│  │       "sub": "user-123",           👈 user binding                                        │
│  │       "vm_id": "vm-456",           👈 resource binding                                    │
│  │       "cluster": "cluster-a",                                                            │
│  │       "namespace": "test-ns",                                                            │
│  │       "exp": now + 2h,             👈 TTL (ADR-0015)                                      │
│  │       "jti": "vnc-token-789",      👈 unique ID for audit                                 │
│  │       "single_use": true           👈 invalidated after first connection                  │
│  │     }                                                                                     │
│  │                                                                                          │
│  │  4. Open noVNC in new tab/popup:                                                         │
│  │     GET /vnc/{vm_id}?token={vnc_jwt}                                                     │
│  │                                                                                          │
│  │  5. Backend proxies WebSocket to KubeVirt:                                               │
│  │     → subresources.kubevirt.io/v1/namespaces/{ns}/virtualmachineinstances/{name}/vnc     │
│  │                                                                                          │
│  │  6. Audit log created:                                                                   │
│  │     INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)       │
│  │     VALUES ('VNC_SESSION_STARTED', 'user-123', 'vm', 'vm-456',                           │
│  │             '{"token_id": "vnc-token-789", "environment": "test"}')                       │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
│  ┌─ Production Environment (Approval Required) ─────────────────────────────────────────────┐
│  │                                                                                          │
│  │  1. User clicks [Request Console Access] button on VM detail page                         │
│  │                                                                                          │
│  │  2. Backend checks:                                                                       │
│  │     a. User has `vnc:access` permission on namespace                                     │
│  │     b. VM is in RUNNING state                                                            │
│  │     c. Environment is production → approval required                                      │
│  │     d. No pending VNC access request exists (duplicate check)                             │
│  │                                                                                          │
│  │  3. Create approval ticket:                                                              │
│  │     INSERT INTO approval_tickets (type, status, requester_id, resource_id, ...)          │
│  │     VALUES ('VNC_ACCESS_REQUESTED', 'PENDING_APPROVAL', 'user-123', 'vm-456', ...)       │
│  │                                                                                          │
│  │  4. Notify admin for approval                                                            │
│  │                                                                                          │
│  │  5. Admin approves (same flow as VM request approval)                                     │
│  │                                                                                          │
│  │  6. On approval:                                                                         │
│  │     a. Generate VNC Token (same structure as test env)                                   │
│  │     b. Notify user with access link                                                       │
│  │     c. User opens noVNC in new tab                                                       │
│  │                                                                                          │
│  │  7. Audit log created (same as test env)                                                 │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### VNC Token Security (V1 Simplified)

| Security Feature | V1 Implementation | ADR-0015 Requirement |
|------------------|-------------------|----------------------|
| **Single Use** | Token marked `used_at` on first connection | ✅ Required |
| **Time-Bounded** | JWT `exp` = now + 2h | ✅ 2 hours (configurable) |
| **User-Bound** | JWT `sub` = user_id | ✅ Required |
| **Encrypted** | AES-256-GCM (shared key management) | ✅ Required |
| **Audit Logged** | `VNC_SESSION_STARTED` event | ✅ Required |

> **V1 Limitation**: No active token revocation. Security relies on short TTL and single-use flag.

### API Endpoints

```
# Request VNC access (creates approval ticket in prod)
POST /api/v1/vms/{vm_id}/console/request
→ Response: { "ticket_id": "...", "status": "PENDING_APPROVAL" }  (prod)
→ Response: { "vnc_url": "/vnc/{vm_id}?token=..." }              (test)

# WebSocket endpoint for noVNC
GET /api/v1/vms/{vm_id}/vnc?token={vnc_jwt}
Upgrade: websocket
→ Proxies to KubeVirt VNC subresource

# Check console access status (for polling)
GET /api/v1/vms/{vm_id}/console/status
→ Response: { "status": "APPROVED", "vnc_url": "..." } | { "status": "PENDING" }
```

### Database Operations

```sql
-- VNC access request (production environment)
INSERT INTO approval_tickets (
    id, type, status, requester_id, resource_type, resource_id, 
    environment, created_at
) VALUES (
    'vnc-ticket-001', 'VNC_ACCESS_REQUESTED', 'PENDING_APPROVAL',
    'user-123', 'vm', 'vm-456', 'production', NOW()
);

-- VNC token usage tracking (inline in JWT, no separate table in V1)
-- Token validation: check JWT signature + exp + jti not in used_tokens cache
-- On first use: add jti to Redis used_tokens set (TTL = token TTL)
```

---
