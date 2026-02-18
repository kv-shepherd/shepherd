# Master Interaction Flow

> **Status**: Stable (ADR-0017, ADR-0018 Accepted)  
> **Version**: 1.2
> **Created**: 2026-01-28  
> **Last Updated**: 2026-02-06
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
> Example: "For InstanceSize schema details, see [01-contracts.md §InstanceSize](../phases/01-contracts.md#deliverables)."

**Related Documents**:
- [ADR-0018: Instance Size Abstraction §User Interaction Flow](../../adr/ADR-0018-instance-size-abstraction.md#user-interaction-flow)
- [ADR-0015: Governance Model V2 §Decision](../../adr/ADR-0015-governance-model-v2.md#decision)
- [ADR-0017: VM Request Flow §Decision](../../adr/ADR-0017-vm-request-flow-clarification.md#decision)
- [Phase 01: Contracts §API Contract-First Design](../phases/01-contracts.md#api-contract-first-design-adr-0021) — Data contracts and naming constraints
- [Phase 04: Governance §7 Audit Logging](../phases/04-governance.md#7-audit-logging) — RBAC, audit logging, approval workflows
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023) — Frontend baseline implementation standard
- [frontend/features/batch-operations-queue.md §2 Parent/Child UI Model](../frontend/features/batch-operations-queue.md#2-parentchild-ui-model) — Parent-child queue UI and polling semantics

**Critical ADR Constraints (Applies to ALL flows in this document)**:

| ADR | Constraint | Scope |
|-----|------------|-------|
| **ADR-0006** | External side-effect operations use **unified async model** (request → 202 → River Queue); pure PostgreSQL transactional writes may remain synchronous | State-changing operations that coordinate external systems |
| **ADR-0009** | River Jobs carry **EventID only** (Claim Check); DomainEvent payload is **immutable** | All River Jobs |
| **ADR-0012** | Atomic transactions: Ent for ORM, **sqlc for core transactions only** | All DB operations |

> **CI at a Glance**: The constraints above are enforced by automated checks. For full gate definitions and scripts, see [docs/design/ci/README.md §Scope Boundary](../ci/README.md#scope-boundary).

---

## Canonical Authoring Contract

This section defines the fixed writing style for all stages in this document.
The goal is consistent readability across all parts without losing key conclusions.

### Stage Structure (Mandatory)

Every `Stage` section MUST follow this order:

1. `Purpose` (why this stage exists; 1-2 lines)
2. `Actors & Trigger` (who initiates, required preconditions)
3. `Interaction Flow` (ASCII flow only, user-facing sequence)
4. `State Transitions` (entity status changes and ownership boundaries)
5. `Failure & Edge Cases` (duplicate request, invalid state, permission denials)
6. `Authority Links` (clickable ADR/phase/database/frontend/CI references)
7. `Scope Boundary` (what this stage intentionally does not define)

### Part Map (Canonical)

| Part | Primary Concern | Primary Audience |
|------|-----------------|------------------|
| **Part 1** | Platform initialization and security baseline | Developer, Platform Admin |
| **Part 2** | Resource hierarchy and ownership boundaries | Regular User, Platform Admin |
| **Part 3** | VM request/approval/execute/delete lifecycle | Regular User, Platform Admin |
| **Part 4** | State machines and shared data model semantics | Backend and Frontend Engineers |
| **Part 5/6** | Specialized workflows (batch, notification, VNC) | Full-stack Engineers |

### Global Design Conclusions (Do Not Override Per Stage)

| Topic | Canonical Conclusion |
|------|----------------------|
| **Name governance** | Platform-managed logical names follow ADR-0019 constraints and must pass centralized validation. |
| **Write model** | Operations with external side effects (for example K8s/provider calls and external notifications) follow unified async model (`request -> 202 -> River`) per [ADR-0006 §Decision](../../adr/ADR-0006-unified-async-model.md#decision); pure PostgreSQL writes may remain synchronous inside atomic transactions. |
| **Event integrity** | River jobs use EventID-only claim-check; event payload is immutable per [ADR-0009 §Constraint 1](../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only). |
| **Transaction boundary** | Core cross-aggregate writes use atomic Ent+sqlc transaction model per [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode). |
| **Delete semantics** | Primary resource rows are hard-deleted (with optional transient `DELETING`), while audit/workflow/event records are retained/archived per [ADR-0015 §13](../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints). |
| **Batch baseline** | V1 batch model uses parent-child tickets with two-layer throttling per [ADR-0015 §19](../../adr/ADR-0015-governance-model-v2.md#19-batch-operations). |

### Cross-Layer Authority

| Layer | Authoritative For |
|------|-------------------|
| [ADRs §Reading Order](../../adr/README.md#reading-order) | Accepted architectural decisions and rationale |
| `master-flow.md` | Interaction intent and expected end-to-end behavior |
| [docs/design/README.md §Implementation Phases](../README.md#implementation-phases) | Implementation contracts and operational constraints |
| [database/README.md §Document Map](../database/README.md#document-map) | Persistence lifecycle, consistency, and schema ownership |
| [frontend/README.md §Reading Order](../frontend/README.md#reading-order) | UI interaction standards and feature-level UX behavior |
| [ci/README.md §Scope Boundary](../ci/README.md#scope-boundary) | Enforceable project gates and anti-drift checks |

### Scope Boundary

- `master-flow.md` explains interaction intent and expected behavior.
- Detailed SQL/DDL/index/migration mechanics must be documented in `docs/design/database/`.
- Detailed component implementation and code-level patterns must be documented in `docs/design/phases/` and `docs/design/frontend/`.

---

## Part 1: Platform Initialization Flow {#stage-1}

### Purpose

Define bootstrapping behavior for schema-driven platform setup and secure first deployment.

### Actors & Trigger

- Trigger: first deployment or platform reconfiguration.
- Actors: developer, platform admin, bootstrap runtime.

### Interaction Flow

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

### State Transitions (Stage 1)

| Domain | Before | After |
|------|------|------|
| Schema cache | unknown/empty | versioned schema available |
| Mask config | undefined | validated exposure paths |
| UI rendering capability | static/manual | schema-driven |

### Failure & Edge Cases (Stage 1)

- Schema fetch failure must degrade to embedded schema baseline.
- Invalid mask paths must fail validation before deployment.

### Authority Links (Part 1 baseline)

- [ADR-0023 §1 Schema Cache Management Policy](../../adr/ADR-0023-schema-cache-and-api-standards.md#1-schema-cache-management-policy)
- [01-contracts.md API Contract-First Design](../phases/01-contracts.md#api-contract-first-design-adr-0021)
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023)

### Scope Boundary (Stage 1)

This stage defines setup flow expectations. Concrete migration steps and code generation commands are maintained in phase/CI docs.

#### Schema Cache Lifecycle Reference {#schema-cache-lifecycle-adr-0023}

For schema cache lifecycle behavior and degradation handling, use these authoritative links:

- [ADR-0023 §1 Schema Cache Management Policy](../../adr/ADR-0023-schema-cache-and-api-standards.md#1-schema-cache-management-policy)
- [02-providers.md §6 Schema Cache Lifecycle](../phases/02-providers.md#6-schema-cache-lifecycle-adr-0023)
- [frontend/FRONTEND.md §Schema Cache Degradation Strategy](../frontend/FRONTEND.md#schema-cache-degradation-strategy-adr-0023)

### Stage 1.5: First Deployment Bootstrap {#stage-1-5}

> **Added 2026-01-26**: First deployment flow for configuration storage strategy.
>
> **Detailed Rules**: See [ADR-0025 §Decision Outcome](../../adr/ADR-0025-secret-bootstrap.md#decision-outcome) for secrets priority and auto-generation, [01-contracts.md §3.2.2](../phases/01-contracts.md#322-system-secrets-table-adr-0025) for implementation details.

#### Purpose

Standardize first-run configuration and secret bootstrap behavior across deployment modes.

#### Actors & Trigger

- Trigger: first successful startup with empty runtime secret state.
- Actors: deployment operator, bootstrap logic, database persistence layer.

#### Interaction Flow

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
│  🔐 Auto-generation (if missing):                                                            │
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

#### State Transitions

| Area | Before | After |
|------|------|------|
| Bootstrap admin | none | default admin seeded (`force_password_change=true`) |
| Secrets | unset | env-provided or generated/persisted |
| Core roles | unset | baseline roles present (idempotent seed) |

#### Failure & Edge Cases

- Missing required DB connection must stop bootstrap before partial writes.
- Secret generation and persistence must be atomic to avoid unusable startup state.

#### Authority Links

- [ADR-0025 §Decision Outcome](../../adr/ADR-0025-secret-bootstrap.md#decision-outcome)
- [01-contracts.md §3.2.2 System Secrets Table](../phases/01-contracts.md#322-system-secrets-table-adr-0025)
- [00-prerequisites.md §7 CI Pipeline](../phases/00-prerequisites.md#7-ci-pipeline)
- [00-prerequisites.md §8 Data Initialization](../phases/00-prerequisites.md#8-data-initialization-adr-0018)

#### Scope Boundary

This stage specifies first-run behavior and outcomes only.
Operational rotation playbooks and advanced key management remain outside this flow.

### Stage 2: Security Configuration (Initial Deployment) {#stage-2}

> **Reference**: ADR-0015 §22 (Authentication & RBAC Strategy)

<a id="stage-2-a"></a>
<a id="stage-2-a-plus"></a>
<a id="stage-2-b"></a>
<a id="stage-2-c"></a>
<a id="stage-2-d"></a>

#### Purpose

Establish authentication, authorization, and initial security defaults required before business traffic.

#### Actors & Trigger

- Trigger: security baseline initialization after first deployment.
- Actors: bootstrap process, platform admin, identity provider integration.

#### Interaction Flow

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
│  │    -- Bootstrap-role deactivation SOP is listed in Markdown notes below.            │
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
│  │  -- Full execution steps are listed in Markdown notes below.                        │
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
> **Standard Provider Output**: All auth providers (OIDC/LDAP/SSO) are normalized via adapter layer into a common payload for RBAC mapping. See [ADR-0026 §Standard Provider Output](../../adr/ADR-0026-idp-config-naming.md#standard-provider-output-contract).
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                Stage 2.B: Configure Authentication Providers (Plugin Standard)                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Platform admin actions:                                                                      │
│                                                                                              │
│  ┌─ Step 1: Choose provider type (from registered plugins) ─────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Authentication Provider Configuration                                                │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Provider type:                                                                   │   │   │
│  │  │                                                                                  │   │   │
│  │  │  ◉ OIDC (plugin) - Azure AD, Okta, Keycloak, Google Workspace                    │   │   │
│  │  │  ○ LDAP (plugin) - Active Directory, OpenLDAP                                    │   │   │
│  │  │  ○ SSO (plugin)  - Enterprise SSO adapter                                        │   │   │
│  │  │  ○ Generic (plugin contract) - custom provider implementing standard fields       │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Next →]                                                                         │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  ┌─ Step 2: Configure provider config (schema-driven) ───────────────────────────────────┐   │
│  │                                                                                        │   │
│  │  Provider Configuration                                                               │   │
│  │  ┌────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Provider name:  [Corp-SSO                    ]                                  │   │   │
│  │  │  Auth type:      [oidc                        ]                                  │   │   │
│  │  │  Config JSON:                                                                      │   │   │
│  │  │  {                                                                                 │   │   │
│  │  │    "issuer": "https://sso.company.com/realms/main",                               │   │   │
│  │  │    "client_id": "shepherd-platform",                                               │   │   │
│  │  │    "client_secret": "••••••",                                                      │   │   │
│  │  │    "claims_mapping": {"groups":"groups"}                                           │   │   │
│  │  │  }                                                                                  │   │   │
│  │  │                                                                                  │   │   │
│  │  │  Callback URL (provider callback endpoint):                                       │   │   │
│  │  │  📋 https://shepherd.company.com/api/v1/auth/providers/{provider_id}/callback    │   │   │
│  │  │                                                                                  │   │   │
│  │  │  [Test connection]  [Save config]                                                │   │   │
│  │  └────────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                        │   │
│  └────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                              │
│  📦 Database operations:                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  INSERT INTO auth_providers (id, auth_type, name, enabled, config, created_by)      │
│  │  VALUES ('idp-001', 'oidc', 'Corp-SSO', true,                                        │
│  │          '{"issuer":"https://sso.company.com/realms/main",                           │
│  │            "client_id":"shepherd-platform",                                          │
│  │            "claims_mapping":{"groups":"groups"}}',                                   │
│  │          'admin-001');                                                                │
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
│  │  Endpoint (deferred): /api/v1/admin/auth-providers/{provider_id}/sample                          │
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
│  │  2. Redirect to provider auth endpoint                                                 │
│  │     → [provider_config.auth_url]?client_id=...&redirect_uri=...                       │
│  │                                                                                        │
│  │  3. User completes IdP authentication                                                  │
│  │                                                                                        │
│  │  4. Provider calls back Shepherd                                                       │
│  │     ← https://shepherd.company.com/api/v1/auth/providers/{provider_id}/callback?...   │
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
| **OIDC plugin** | Production (recommended) | IdP group → mapping rules → RoleBindings |
| **LDAP plugin** | Legacy AD environment | LDAP group → mapping rules → RoleBindings |
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

User requests access to resource R (e.g., GET /api/v1/systems/{system_id})

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

#### Stage 2 Bootstrap Role Safety Notes

- Bootstrap role (`role-bootstrap`) is initialization-only and must be disabled after first deployment.
- Operational procedure: [operations/bootstrap-role-sop.md](../../operations/bootstrap-role-sop.md)
- Governance and audit baseline: [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)

#### State Transitions

| Domain | Typical Transition |
|------|---------------------|
| User auth profile | `uninitialized -> active` after first successful identity sync |
| Role binding | `absent -> assigned` (global and/or resource level) |
| Approval capability | `disabled -> enabled` after policy/provider configuration |

#### Failure & Edge Cases

- Bootstrap role must be disabled after initial setup to avoid latent super-admin risk.
- External IdP mapping drift must not silently escalate privileges.
- Resource visibility must remain deny-by-default when inheritance chain has no binding.

#### Authority Links

- [ADR-0015 §22 Authentication and RBAC Strategy](../../adr/ADR-0015-governance-model-v2.md#22-authentication-rbac-strategy)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [01-contracts.md Naming Constraints](../phases/01-contracts.md#11-naming-constraints-adr-0019)

#### Scope Boundary

This stage specifies security interaction expectations and permission semantics.
Protocol details and operational hardening checklists are maintained in phase and operations docs.

### Stage 2.E: Approval Provider Standard (V1 Built-in, V2+ External Plugin) {#stage-2-e}

> **Added 2026-01-26**: Approval provider model and external integration boundary

#### Purpose

Define one canonical approval-provider contract. V1 ships with the built-in provider only;
external systems are integrated as provider plugins without changing approval state semantics.

#### Actors & Trigger

- Trigger: platform admin defines approval provider strategy and policy.
- Actors: platform admin, approval provider router, built-in provider, optional external provider adapter.

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Stage 2.E: Approval Provider Boundary (Single Contract, Pluggable Providers)                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  V1 go-live path (required):                                                                 │
│    1) User submits request -> approval_tickets=PENDING_APPROVAL                              │
│    2) Router selects built-in provider (`builtin-default`, only provider in V1)             │
│    3) Built-in approver decides APPROVED / REJECTED                                          │
│    4) Shepherd executes decision path and appends audit logs                                 │
│                                                                                              │
│  External plugin route (V2+ roadmap):                                                        │
│    1) External adapter plugin is registered and enabled by policy                            │
│    2) Router delegates ticket via ExternalApprovalProvider.SubmitForApproval                 │
│    3) Callback/polling maps external decision to canonical APPROVED/REJECTED                 │
│    4) Provider timeout/unavailable -> controlled fallback to built-in queue                  │
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
│  │  System auto-detects; admin does not configure manually:                                │  │
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
│  ┌─ Step 2: Configure Namespace ────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │
│  │  ⚠️ KEY PRINCIPLE:                                                                       │
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
│  │     3. Classify and report K8s API errors (details in Markdown notes below).             │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
│  ┌─ Step 3: Configure Template ──────────────────────────────────────────────────────────────┐ │
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
│  │  Template versioning:                                                                    │
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
│  ⚠️ Dry-Run Validation:                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                                                                                          │ │
│  │  Before saving, admin can validate InstanceSize against target clusters:                 │ │
│  │                                                                                          │ │
│  │  Deferred endpoint pattern: /api/v1/instance-sizes?dryRun=All                            │ │
│  │  Deferred endpoint pattern: /api/v1/instance-sizes?dryRun=All&targetCluster={cluster_id} │ │
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

#### Stage 3 JIT Namespace Execution Notes {#stage-3-jit-namespace}

<a id="stage-3-c"></a>

- Error classification (canonical response codes):
  - `NAMESPACE_PERMISSION_DENIED (403)`: target cluster denies namespace creation.
  - `NAMESPACE_QUOTA_EXCEEDED (403)`: namespace creation rejected by cluster quota policy.
  - `NAMESPACE_CREATION_FAILED (500)`: unexpected K8s/API error class.
- Failure handling baseline:
  - Ticket status moves to `FAILED_PROVISIONING`.
  - Worker retries with exponential backoff.
- Normative references:
  - [ADR-0017 §Namespace Just-In-Time Creation (Added 2026-01-27)](../../adr/ADR-0017-vm-request-flow-clarification.md#namespace-just-in-time-creation-added-2026-01-27)
  - [01-contracts.md §Error Code Standard (ADR-0023)](../phases/01-contracts.md#error-code-standard-adr-0023)

#### State Transitions

| Domain | Before | After |
|------|------|------|
| Approval provider set | built-in implicit | explicit provider registry; V1 = built-in only |
| Decision contract | provider-specific interpretation risk | canonical `APPROVED/REJECTED` contract across providers |
| Fallback behavior | implicit | explicit fail-safe fallback to built-in on adapter failure |

#### Failure & Edge Cases

- External adapter unavailability must not block the built-in provider path.
- Callback signature/status mapping mismatch must be rejected and audited.
- External timeout must keep ticket recoverable (fallback or pending), never orphaned.

#### Authority Links

- [ADR-0005 §Decision](../../adr/ADR-0005-workflow-extensibility.md#decision)
- [ADR-0015 §21 Scope Exclusions (V1)](../../adr/ADR-0015-governance-model-v2.md#21-scope-exclusions-v1)
- [04-governance.md §9 External Approval Systems (V1 Interface Only)](../phases/04-governance.md#9-external-approval-systems-v1-interface-only)
- [04-governance.md §9.1 Interface Definition](../phases/04-governance.md#91-interface-definition)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [RFC-0004 External Approval Systems Integration](../../rfc/RFC-0004-external-approval.md)

#### Scope Boundary

This stage defines provider-model intent and V1 boundary only.
Detailed provider payload/callback/security design is roadmap content in
[Part 4 §Approval Provider Plugin Architecture (V2+ Roadmap)](#external-approval-v2-roadmap)
and RFC-0004.

---

## Part 2: Resource Management Flow

<a id="stage-4-a"></a>
<a id="stage-4-a-plus"></a>
<a id="stage-4-b"></a>
<a id="stage-4-c"></a>

> **Note**: Before creating VMs, users must create System and Service to organize resources.

### Purpose

Define ownership and hierarchy creation behavior for System/Service resources.

### Actors & Trigger

- Trigger: regular user starts environment setup for VM workloads.
- Actors: resource owner, team members, RBAC evaluator, audit subsystem.

### Interaction Flow

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
│  🔍 Permission inheritance model (pattern aligned with Google Cloud IAM, GitHub Teams):       │
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
│  │  -- 1. Create service (no created_by per ADR-0015 §2; actor recorded in audit_logs) │       │
│  │  INSERT INTO services (id, name, description, system_id, created_at)               │       │
│  │  VALUES ('svc-001', 'redis', 'Cache service', 'sys-001', NOW());                    │       │
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

### Stage 4.C: Service Detail & Update Operations {#stage-4-c-detail}

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                     Stage 4.C: Service Detail & Update Operations                            │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  User navigates to Service detail page:                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  Service: redis  (System: shop)                                                    │       │
│  │                                                                                    │       │
│  │  Description:  Cache service  [✏ Edit]                                            │       │
│  │                                                                                    │       │
│  │  Virtual Machines:                                                                │       │
│  │  ┌────────────────────────────────────────────────────────────────────────────┐   │       │
│  │  │  Name                     Status     Namespace     InstanceSize           │   │       │
│  │  │  ────────────────────────────────────────────────────────────────────────  │   │       │
│  │  │  prod-shop-redis-01       RUNNING    prod-shop     gpu-workstation        │   │       │
│  │  │  prod-shop-redis-02       STOPPED    prod-shop     standard-2c4g          │   │       │
│  │  └────────────────────────────────────────────────────────────────────────────┘   │       │
│  │                                                                                    │       │
│  │  [+ Create VM]  → navigates to Stage 5.A (VM Request)                              │       │
│  │                                                                                    │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  📦 Update description (single transaction):                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐       │
│  │  BEGIN TRANSACTION;                                                               │       │
│  │                                                                                    │       │
│  │  -- 1. Update description only (name is immutable per ADR-0019 RFC 1035)          │       │
│  │  UPDATE services SET description = $1, updated_at = NOW()                         │       │
│  │  WHERE id = $2 AND deleted_at IS NULL;                                            │       │
│  │                                                                                    │       │
│  │  -- 2. 📝 Audit log                                                                │       │
│  │  INSERT INTO audit_logs (action, actor_id, resource_type, resource_id,            │       │
│  │                          parent_type, parent_id, details) VALUES                  │       │
│  │    ('service.update', $actor, 'service', $id, 'system', $sys_id,                  │       │
│  │     '{"field": "description", "old": "...", "new": "..."}');                       │       │
│  │                                                                                    │       │
│  │  COMMIT;                                                                          │       │
│  └──────────────────────────────────────────────────────────────────────────────────┘       │
│                                                                                              │
│  ⚠️ System update follows same pattern (description only, name immutable).                    │
│  ⚠️ Delete operations → see Stage 5.D for cascade constraints and confirmation.               │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### State Transitions (Part 2)

| Entity | Typical Transition |
|------|---------------------|
| System | `none -> ACTIVE` on creation |
| Service | `none -> ACTIVE` after parent-system validation |
| Resource membership | `none -> owner/admin/member/viewer` with inheritance semantics |

### Failure & Edge Cases (Part 2)

- Creating Service without visible/authorized parent System must fail.
- Duplicate logical name under same scope must fail before commit.
- Deletion must respect cascade constraints and confirmation rules.

### Authority Links (Part 2)

- [ADR-0015 §13 Deletion Cascade Constraints](../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0019 §Baseline Controls (Normative)](../../adr/ADR-0019-governance-security-baseline-controls.md#baseline-controls-normative)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [database/schema-catalog.md §Table Domains](../database/schema-catalog.md#table-domains)

### Scope Boundary (Part 2)

This part defines hierarchy and access behavior expectations.
DDL details, index strategies, and SQL implementation belong to database/phase docs.

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
> See [ADR-0017 §Decision](../../adr/ADR-0017-vm-request-flow-clarification.md#decision) for complete rationale.

### Purpose

Capture the end-to-end interaction journey from VM request submission to approval,
execution, and runtime outcomes.

### Actors & Trigger

- Trigger: regular user submits a VM create request in Service scope.
- Actors: requester, platform admin approver, async worker, provider integration.

### Interaction Flow

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
│    constraint and cannot be bypassed.                                                        │
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

### Stage 5.B Constraint Note: Dedicated CPU vs Overcommit {#stage-5b-constraint-note-dedicated-cpu-vs-overcommit}

- Hard constraint: `dedicatedCpuPlacement` requires Guaranteed QoS, so CPU request must equal CPU limit.
- This check is blocking in approval flow (not warning-only).
- Reference:
  [KubeVirt Compute resource requests and limits](https://kubevirt.io/user-guide/compute/resources_requests_and_limits/)

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

### State Transitions (Stage 5.A-5.C)

| Stage | Ticket | Domain Event | VM | Worker Job |
|------|--------|--------------|----|------------|
| 5.A Submit | created as `PENDING_APPROVAL` | created as `PENDING` | none | none |
| 5.B Approve | `PENDING_APPROVAL -> APPROVED` | `PENDING -> PROCESSING` | created as `CREATING` | inserted |
| 5.B Reject | `PENDING_APPROVAL -> REJECTED` | `PENDING -> CANCELLED` | none | none |
| 5.C Execute | unchanged | progresses per execution | `CREATING -> RUNNING|FAILED` | consumed/completed |

### Failure & Edge Cases (Stage 5.A-5.C)

- Duplicate pending submission must be blocked before creating new ticket/event.
- Cluster capability mismatch during approval must block approval before worker scheduling.
- Execution failures must preserve auditable trail and deterministic retry behavior.

### Authority Links (Stage 5.A-5.C)

- [ADR-0017 Decision Boundary](../../adr/ADR-0017-vm-request-flow-clarification.md#decision)
- [ADR-0018 §User Interaction Flow](../../adr/ADR-0018-instance-size-abstraction.md#user-interaction-flow)
- [database/vm-lifecycle-write-model.md §Stage 5.A](../database/vm-lifecycle-write-model.md#stage-5a-vm-request-submission-pending-approval)
- [frontend/FRONTEND.md §API Type Integration](../frontend/FRONTEND.md#api-type-integration-adr-0021)

### Scope Boundary (Stage 5.A-5.C)

This stage group defines interaction sequence and status expectations.
Detailed SQL/DDL/migration and worker internals are documented in database and phase layers.

---

### Stage 5.A: Persistence Summary {#stage-5-a}

#### Purpose

Summarize persistence intent after VM request submission while keeping implementation details in the database layer.

#### Actors & Trigger

- Trigger: user submits VM create request.
- Actors: requester, approval workflow subsystem, notification subsystem.

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Stage 5.A Persistence Intent (Submission Write Set)                                         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Requester submits VM request                                                                │
│        │                                                                                     │
│        ▼                                                                                     │
│  API pre-checks (RBAC + duplicate pending guard)                                             │
│        │                                                                                     │
│        ▼                                                                                     │
│  Single transaction writes:                                                                  │
│    1) approval_tickets: create `PENDING_APPROVAL`                                            │
│    2) domain_events: create `PENDING`                                                        │
│    3) audit_logs: append canonical submission action                                         │
│        │                                                                                     │
│        ▼                                                                                     │
│  Return `202 Accepted` with ticket reference for polling                                     │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| Entity | Before | After |
|------|------|------|
| `approval_tickets` | none | `PENDING_APPROVAL` |
| `domain_events` | none | `PENDING` |
| `vms` | none | none |
| `river_job` | none | none |

#### Failure & Edge Cases

- Duplicate pending request for same operation must return conflict and existing ticket reference.
- If any write in the transaction fails, all writes must rollback.

#### Authority Links

- [database/vm-lifecycle-write-model.md §Stage 5.A](../database/vm-lifecycle-write-model.md#stage-5a-vm-request-submission-pending-approval)
- [ADR-0009 §Constraint 1 DomainEvent Payload Immutability](../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only)
- [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode)

#### Scope Boundary

This stage does not define SQL statements, table indexes, or migration details.

### Stage 5.B: Persistence Summary {#stage-5-b}

#### Purpose

Summarize approval/rejection write outcomes and guarantees for VM creation workflows.

#### Actors & Trigger

- Trigger: platform admin approves or rejects a pending VM request.
- Actors: approver, workflow transaction boundary, River worker scheduler.

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Stage 5.B Persistence Intent (Decision Write Set)                                            │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Approver opens pending ticket                                                               │
│        │                                                                                     │
│        ├── Approve path                                                                      │
│        │      1) ticket: `PENDING_APPROVAL -> APPROVED`                                     │
│        │      2) domain_event: `PENDING -> PROCESSING`                                      │
│        │      3) vms: insert with `CREATING`                                                │
│        │      4) river job: enqueue execution task                                           │
│        │      5) audit_logs: append approval action                                          │
│        │                                                                                     │
│        └── Reject path                                                                       │
│               1) ticket: `PENDING_APPROVAL -> REJECTED`                                     │
│               2) domain_event: `PENDING -> CANCELLED`                                       │
│               3) no VM row / no River job                                                   │
│               4) audit_logs: append rejection action                                         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| Path | Ticket | Domain Event | VM | River Job |
|------|--------|--------------|----|-----------|
| Approve | `PENDING_APPROVAL -> APPROVED` | `PENDING -> PROCESSING` | created with `CREATING` | inserted (`available`) |
| Reject | `PENDING_APPROVAL -> REJECTED` | `PENDING -> CANCELLED` | not created | not inserted |

#### Failure & Edge Cases

- Approval path must preserve claim-check model (River payload carries EventID reference, not full mutable business payload).
- Rejection path must not create VM rows or async jobs.

#### Authority Links

- [database/vm-lifecycle-write-model.md §Stage 5.B](../database/vm-lifecycle-write-model.md#stage-5b-admin-approval-rejection)
- [ADR-0006 §Decision](../../adr/ADR-0006-unified-async-model.md#decision)
- [ADR-0009 §Constraint 1 DomainEvent Payload Immutability](../../adr/ADR-0009-domain-event-pattern.md#constraint-1-domainevent-payload-immutability-append-only)
- [ADR-0012 §Adopt Ent + sqlc Hybrid Mode](../../adr/ADR-0012-hybrid-transaction.md#adopt-ent-sqlc-hybrid-mode)

#### Scope Boundary

This stage defines required status outcomes and transaction guarantees only.

### Stage 5.D: Delete Operations {#stage-5-d}

#### Purpose

Define user-facing delete behavior for VM/Service/System and the corresponding
status expectations.

#### Actors & Trigger

- Trigger: user or admin initiates delete API with required confirmation.
- Actors: requester, approval workflow (VM only), async worker, audit subsystem.

#### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Delete User Journey (Interaction Intent)                                                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  Resource detail page (VM / Service / System)                                                │
│        │                                                                                     │
│        ▼                                                                                     │
│  User clicks Delete -> UI confirmation challenge (`confirm=true` or `confirm_name`)         │
│        │                                                                                     │
│        ▼                                                                                     │
│  API validates RBAC + cascade preconditions + environment policy                             │
│        │                                                                                     │
│        ├── VM path: create delete approval ticket -> approver decision                       │
│        │                                                                                     │
│        └── Service/System path: no delete approval ticket                                    │
│        │                                                                                     │
│        ▼                                                                                     │
│  Execution path sets optional transient `DELETING`, performs cleanup, hard-deletes row      │
│  (audit logs / approval records / domain events remain retained by retention policy)         │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

Entity rule matrix:

| Entity | Preconditions | Approval | Confirmation | Primary Table Behavior |
|------|------|------|------|------|
| VM (test) | none | ✅ required | `confirm=true` | `DELETING` (transient) -> hard delete |
| VM (prod) | none | ✅ required | `confirm_name` | `DELETING` (transient) -> hard delete |
| Service | child VM count must be 0 | ❌ not required | `confirm=true` | `DELETING` (transient) -> hard delete |
| System | child Service count must be 0 | ❌ not required | `confirm_name` | hard delete |

#### State Transitions

| Flow | Ticket | Resource | Final Persistence Outcome |
|------|--------|----------|---------------------------|
| VM delete approved | `PENDING_APPROVAL -> APPROVED` | `RUNNING/STOPPED -> DELETING -> (row removed)` | VM row hard-deleted, records retained separately |
| Service delete | no ticket | `ACTIVE -> DELETING -> (row removed)` | Service row hard-deleted after worker cleanup |
| System delete | no ticket | `ACTIVE -> (row removed)` | System row hard-deleted in validated transaction |

#### Failure & Edge Cases

- Cascade precondition failure must block delete (`Service has VM`, `System has Service`).
- Confirmation mismatch must fail before any write.
- Worker failure after `DELETING` must remain recoverable via retry and auditable history.

#### Authority Links

- [ADR-0015 §13 Deletion Cascade Constraints](../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0015 §13.1 Confirmation Mechanism](../../adr/ADR-0015-governance-model-v2.md#131-delete-confirmation-mechanism)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [database/lifecycle-retention.md §Retention Classes](../database/lifecycle-retention.md#retention-classes-table-centric)
- [database/vm-lifecycle-write-model.md §Stage 5.D](../database/vm-lifecycle-write-model.md#stage-5d-delete-write-model)

#### Scope Boundary

This stage defines delete interaction intent and required outcomes only.
Schema details, purge jobs, and index design are defined in database-layer docs.

> **Naming policy for delete actions**:
> - Canonical V1 actions: `*.delete_submitted`, `*.delete_approved` (when applicable), `*.delete_executed`.
> - Legacy forms such as `*.delete_request` / `*.delete` may appear in historical notes, but new design content MUST use canonical action names above.

---

### Stage 5.E: Batch Operations {#stage-5e-batch-operations}

#### Purpose

Define canonical batch submission/execution behavior with parent-child ticket
model and two-layer throttling.

#### Actors & Trigger

- Trigger: user/admin submits one batch operation containing multiple child items.
- Actors: frontend queue UI, API gateway, governance transaction layer, River workers.

#### Interaction Flow

UI storyboard (parent-child queue):

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ BATCH QUEUE UI STORYBOARD                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  [Batch action page]                                                                             │
│     Select VM rows + choose operation + Submit                                                   │
│                                  │                                                               │
│                                  ▼                                                               │
│  [Queue list page]                                                                                │
│     New parent row appears: `PENDING_APPROVAL`                                                   │
│     Columns: total/success/failed/pending + requester + updated_at                              │
│                                  │                                                               │
│                                  ▼                                                               │
│  [Parent row expanded]                                                                            │
│     Child table shows per-item status + attempt_count + last_error                               │
│                                  │                                                               │
│                                  ▼                                                               │
│  [In progress / terminal handling]                                                                │
│     `IN_PROGRESS`      -> action: Terminate pending children                                     │
│     `PARTIAL_SUCCESS`  -> action: Retry failed children                                           │
│     `FAILED`           -> action: Retry failed children                                           │
│     `COMPLETED`        -> action: Export result                                                   │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ BATCH SUBMISSION FLOW (CANONICAL)                                                               │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. User/Admin selects batch items in UI                                                        │
│                                                                                                  │
│  2. Frontend: POST /api/v1/vms/batch                                                            │
│     └── includes idempotency key + operation payload                                             │
│                                                                                                  │
│  3. Backend pre-checks:                                                                          │
│     • Layer 1 (global): pending parent threshold + API rate                                     │
│     • Layer 2 (user): pending parent/child limits + cooldown                                    │
│                                                                                                  │
│  4. Atomic transaction:                                                                          │
│     • Insert parent batch ticket                                                                 │
│     • Insert all child tickets                                                                   │
│     • If any child insert fails -> rollback all                                                 │
│                                                                                                  │
│  5. Response (202 Accepted):                                                                     │
│     └── {batch_id, status: \"PENDING_APPROVAL\", status_url, retry_after_seconds}               │
│                                                                                                  │
│  6. Frontend tracks: GET /api/v1/vms/batch/{batch_id}                                           │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ BATCH EXECUTION FLOW                                                                             │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  1. Parent enters APPROVED/IN_PROGRESS                                                          │
│                                                                                                  │
│  2. Workers consume child tickets/jobs independently                                             │
│     • Child success/failure updates parent aggregate counters                                    │
│     • Failures are isolated; successful children are not rolled back                             │
│                                                                                                  │
│  3. Parent terminal state calculation:                                                           │
│     • COMPLETED: all children succeeded                                                          │
│     • FAILED: all children failed                                                                │
│     • PARTIAL_SUCCESS: mixed success/failure                                                     │
│     • CANCELLED: pending children terminated by user/admin                                       │
│                                                                                                  │
│  4. Frontend actions during/after execution:                                                     │
│     • Retry failed children: POST /api/v1/vms/batch/{id}/retry                                   │
│     • Terminate pending children: POST /api/v1/vms/batch/{id}/cancel                             │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ COMPATIBILITY ENDPOINTS                                                                          │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  Existing APIs remain supported for compatibility:                                               │
│    • POST /api/v1/approvals/batch                                                                │
│    • POST /api/v1/vms/batch/power                                                                │
│                                                                                                  │
│  Internally, both are normalized into the same parent-child ticket pipeline.                     │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| Scope | Transition Pattern |
|------|---------------------|
| Parent ticket | `PENDING_APPROVAL -> APPROVED/IN_PROGRESS -> COMPLETED|PARTIAL_SUCCESS|FAILED|CANCELLED` |
| Child ticket | `PENDING -> RUNNING -> SUCCESS|FAILED|CANCELLED` |

#### Failure & Edge Cases

- Global or per-user throttling rejection must return actionable retry window.
- Child failure must not rollback successful siblings.
- Retry/cancel must target eligible children only and recompute parent aggregate status.

#### Authority Links

- [ADR-0015 §19 Batch Operations V1](../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)
- [04-governance.md §5.6 Batch Operations](../phases/04-governance.md#56-batch-operations-adr-0015-19)
- [database/vm-lifecycle-write-model.md §Stage 5.E](../database/vm-lifecycle-write-model.md#stage-5e-batch-parent-child-write-model)
- [frontend/features/batch-operations-queue.md §2.0 End-to-End UI Storyboard](../frontend/features/batch-operations-queue.md#20-end-to-end-ui-storyboard)

#### Scope Boundary

This stage defines interactive behavior and state semantics only.
Queue internals, table schema, and worker tuning details are defined in phase and database docs.

---

### Stage 5.F: Notification System {#stage-5f-notification-system}

#### Purpose

Define notification behavior visible to users/admins for request, approval, and VM lifecycle events.

#### Actors & Trigger

- Trigger: approval workflow events and VM state transitions.
- Actors: workflow transaction layer, inbox notification service, frontend polling UI.

#### Interaction Flow

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
│  ⚠️ V1 Constraint: No external channels (email/webhook); V2+ plan is linked below             │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### State Transitions

| Event Type | Delivery Expectation |
|------|------------------------|
| Approval required | notify approvers immediately after ticket submission |
| Approval decision | notify requester after approve/reject |
| Runtime state change | notify resource owner with latest VM state |

#### Failure & Edge Cases

- Notification write must not be dropped silently; failures must be observable.
- V1 uses polling only; clients must tolerate eventual consistency.
- Sensitive details in payload must follow redaction policy before persistence.

#### Authority Links

- [ADR-0015 §20 Notification System](../../adr/ADR-0015-governance-model-v2.md#20-notification-system)
- [04-governance.md §6.3 Notification System](../phases/04-governance.md#63-notification-system-adr-0015-20)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [RFC-0018 §Proposed Solution](../../rfc/RFC-0018-external-notification.md#proposed-solution)

#### Scope Boundary

This stage defines user-visible notification behavior. Channel adapters, delivery retries,
and provider integration internals are defined in governance and RFC documents.

---

## Part 4: State Machines & Data Models

> **Note**: This section defines state machines and DB relationships for core entities.
> It is a critical reference for frontend and backend development.

### Purpose

Provide canonical state semantics and shared data-model intent for cross-team alignment.

### Actors & Trigger

- Trigger: engineers need consistent interpretation of workflow and runtime states.
- Actors: backend engineers, frontend engineers, SRE/operations reviewers.

### Interaction Flow

Part 4 is a reference view rather than a user-operation sequence.
It consolidates entity states, relationship intent, and audit semantics consumed by all flows.

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
│                     (worker hard-deletes DB row; no persisted DELETED state)               │
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

> **Scope boundary**: this section defines audit semantics only.
> Full schema/DDL/index details are authoritative in:
> - [04-governance.md §7](../phases/04-governance.md#7-audit-logging)
> - [database/schema-catalog.md §Table Domains](../database/schema-catalog.md#table-domains)
> - [database/lifecycle-retention.md §Retention Classes](../database/lifecycle-retention.md#retention-classes-table-centric)

#### Mandatory Coverage

- All state-changing operations (CREATE/UPDATE/DELETE)
- Sensitive read operations (for example `vnc.access`)
- Both success and failure paths for submission/approval/execution stages

#### Canonical Action Naming

| Domain | Canonical Actions (V1) | Notes |
|------|------|------|
| Auth | `user.login`, `user.login_failed`, `user.logout` | Authentication events |
| System | `system.create`, `system.update`, `system.delete_submitted`, `system.delete_executed` | No delete approval ticket |
| Service | `service.create`, `service.delete_submitted`, `service.delete_executed` | No delete approval ticket |
| VM | `vm.request`, `vm.create`, `vm.start`, `vm.stop`, `vm.restart`, `vm.delete_submitted`, `vm.delete_approved`, `vm.delete_executed` | Delete requires approval |
| VNC | `vnc.access` | Sensitive read |
| Approval | `approval.approve`, `approval.reject`, `approval.cancel` | Ticket decisions |
| RBAC | `role.create`, `role.update`, `role.delete`, `role.assign`, `role.revoke`, `permission.create`, `permission.delete` | Permission governance |
| Cluster | `cluster.register`, `cluster.update`, `cluster.delete`, `cluster.credential_rotate` | Cluster lifecycle |
| Template | `template.create`, `template.update`, `template.deprecate`, `template.delete` | Template lifecycle |
| InstanceSize | `instance_size.create`, `instance_size.update`, `instance_size.deprecate`, `instance_size.delete` | Sizing lifecycle |
| Namespace | `namespace.create`, `namespace.delete` | Namespace lifecycle |
| Auth Provider | `auth_provider.configure`, `auth_provider.update`, `auth_provider.delete`, `auth_provider.sync`, `auth_provider.mapping_create`, `auth_provider.mapping_update`, `auth_provider.mapping_delete` | ADR-0015 amendment: use `auth_provider.*`, not `idp.*` |
| Config | `config.update` | Platform configuration change |

#### Fields Required in Every Audit Record

- `action`, `actor_id`, `resource_type`, `resource_id`, `created_at`
- Optional but recommended when available: `parent_type`, `parent_id`, `environment`
- `details` payload must be redacted per ADR-0019

#### Operations Commonly Exempt from Audit

| Category | Operation | Reason |
|------|------|------|
| System checks | Cluster health polling, VM status sync polling | High frequency, no direct user intent |
| Read-only | List/detail APIs (`GET`) | No state change |
| Internal | Worker heartbeat, metrics collection | Internal observability traffic |

> **Exception principles**:
> - Write operations are audited by default.
> - Exemptions must be explicit and reviewed.
> - Sensitive reads remain auditable even when not state-changing.

#### Retention Baseline

| Environment | Retention | Notes |
|------|------|------|
| Production | >= 1 year | Compliance baseline |
| Test | >= 90 days | Can be shorter by policy |
| Sensitive operations | >= 3 years | `*.delete*`, `approval.*`, `rbac.*` |

---

### Audit Log JSON Export (v1+)

> **Scenario**: Integrate audit logs into enterprise SIEM (Elasticsearch, Datadog, Splunk, etc.)

> 📦 **API Specification**: See [04-governance.md §7 JSON Export API](../phases/04-governance.md#7-json-export-api) for full API and response format.

**Key Features**:
- Paginated export with time range filtering
- Webhook push integration for real-time streaming
- Structured JSON format compatible with common log aggregators

---

<a id="external-approval-v2-roadmap"></a>

### Approval Provider Plugin Architecture (V2+ Roadmap)

> **Scenario**: integrate with enterprise ITSM (Jira Service Management, ServiceNow, etc.).
>
> **V1 Boundary**: V1 implements one unified approval-provider contract with a single built-in
> provider (`builtin-default`). External systems are plugin adapters in V2+.

#### Design Principles

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                    Approval Provider Plugin Architecture                                    │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  ┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐   │
│  │   Shepherd   │  ──── Webhook ───▶ │ External Sys │  ──── Callback ──▶ │   Shepherd   │   │
│  │   Platform   │                    │ (Jira/SNOW)  │                    │   Platform   │   │
│  └──────────────┘                    └──────────────┘                    └──────────────┘   │
│                                                                                              │
│  Key principles:                                                                             │
│  1. Shepherd owns canonical ticket states and audit trail                                    │
│  2. Providers (built-in/external) share one stable contract                                 │
│  3. Async integration + fail-safe fallback; external failure cannot block built-in path      │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Approval Provider Configuration (External Adapters, Web UI)

> Admin config via **Settings → External Approval Systems → Add**.
> External adapter registry is stored in `external_approval_systems`.

**Webhook security (best practice)**:
- HTTPS only for all webhook URLs.
- Verify webhook signatures with shared secret and constant-time comparison.
- Include a timestamp in the signed payload and reject stale requests to prevent replay.
- Store webhook secrets encrypted at rest; rotate when compromised.

References:
- https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
- https://docs.stripe.com/webhooks/test

Key persisted data (schema authority remains in phase/database docs):

| Object | Representative fields | Purpose |
|----------|------|------|
| `external_approval_systems` | `id`, `name`, `type`, `enabled`, `webhook_url`, `webhook_secret`, `timeout_seconds`, `retry_count` | External adapter registry and delivery guardrails |
| `audit_logs` | `action`, `resource_type`, `resource_id`, `result`, `metadata` | Immutable local trace for external decisions/fallback actions |

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

### State Transitions (Part 4 Reference)

| Domain | Canonical States |
|--------|------------------|
| Approval ticket | `PENDING_APPROVAL`, `APPROVED`, `REJECTED`, `CANCELLED`, `EXECUTING`, `SUCCESS`, `FAILED` |
| VM runtime | `CREATING`, `RUNNING`, `STOPPING`, `STOPPED`, `FAILED`, `DELETING` |
| Audit record lifecycle | append-only write, retained/archived per policy |

### Failure & Edge Cases (Part 4 Reference)

- State machine drift across API/UI/worker implementations is prohibited.
- Any new terminal state must update flow docs, governance docs, and API contracts together.
- Audit redaction policy violations are security incidents, not formatting defects.

### Authority Links (Part 4)

- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [database/schema-catalog.md §Relationship Baseline](../database/schema-catalog.md#relationship-baseline)
- [database/lifecycle-retention.md §Database Guardrails](../database/lifecycle-retention.md#database-guardrails)
- [ADR-0015 §11 Approval Timeout Handling](../../adr/ADR-0015-governance-model-v2.md#11-approval-timeout-handling)

### Scope Boundary (Part 4)

This part defines semantic models and cross-component invariants.
It does not replace schema DDL ownership, API source contracts, or worker implementation playbooks.

---

## Stage 6: VNC Console Access {#stage-6-vnc-console-access}

### Purpose

Define secure browser console access behavior for test and production environments.

### Actors & Trigger

- Trigger: user requests VM console access from VM detail page.
- Actors: requester, RBAC guard, approval workflow (production), token issuer, VNC proxy.

### Interaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Stage 6 Console Access Overview                                                                │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                              │
│  VM detail page -> user clicks Console / Request Console Access                               │
│        │                                                                                     │
│        ▼                                                                                     │
│  Backend guard checks: RBAC (`vnc:access`) + VM state (`RUNNING`)                            │
│        │                                                                                     │
│        ├── Test env: issue token -> open noVNC directly                                      │
│        │                                                                                     │
│        └── Production env: create approval ticket -> admin approve/reject                    │
│                 ├── approved: issue token -> open noVNC                                      │
│                 └── rejected: no console session                                              │
│                                                                                              │
│  Both paths append audit records for request/access outcomes                                  │
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

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
│  │  3. Generate VNC access grant (JWT claim + one-time bootstrap credential):              │
│  │     {                                                                                     │
│  │       "sub": "user-123",           👈 user binding                                        │
│  │       "vm_id": "vm-456",           👈 resource binding                                    │
│  │       "cluster": "cluster-a",                                                            │
│  │       "namespace": "test-ns",                                                            │
│  │       "exp": now + 2h,             👈 TTL                                                 │
│  │       "jti": "vnc-token-789",      👈 unique ID for audit                                 │
│  │       "single_use": true           👈 invalidated after first connection                  │
│  │     }                                                                                     │
│  │                                                                                          │
│  │  4. Open noVNC in new tab/popup using secure bootstrap channel:                          │
│  │     Set-Cookie: vnc_bootstrap=<opaque>; HttpOnly; Secure; SameSite=Strict; Max-Age=60   │
│  │     GET /api/v1/vms/{vm_id}/vnc                                                           │
│  │     (no bearer token in URL query)                                                        │
│  │                                                                                          │
│  │  5. Backend proxies WebSocket to KubeVirt:                                               │
│  │     → subresources.kubevirt.io/v1/namespaces/{ns}/virtualmachineinstances/{name}/vnc     │
│  │                                                                                          │
│  │  6. Audit log created:                                                                   │
│  │     INSERT INTO audit_logs (action, actor_id, resource_type, resource_id, details)       │
│  │     VALUES ('vnc.access', 'user-123', 'vm', 'vm-456',                                     │
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
│  │     a. Generate VNC access grant (same structure as test env)                            │
│  │     b. Notify user with access link                                                       │
│  │     c. User opens noVNC in new tab                                                       │
│  │                                                                                          │
│  │  7. Audit log created (same as test env)                                                 │
│  │                                                                                          │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘
│                                                                                              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

### State Transitions

| Environment | Ticket | Access Outcome |
|-------------|--------|----------------|
| Test | no approval ticket | RBAC pass -> access grant issued -> session started |
| Production | `PENDING_APPROVAL -> APPROVED/REJECTED` | approved -> access grant issued; rejected -> no console access |

### Failure & Edge Cases

- VM not in `RUNNING` state must block access grant issuance.
- Duplicate pending production request must be rejected idempotently.
- Bootstrap credential replay after first successful connection must be denied and audited.

### Authority Links

- [ADR-0015 §18 VNC Console Access](../../adr/ADR-0015-governance-model-v2.md#18-vnc-console-access-permissions)
- [RFC-0011 §V1 Implementation Scope](../../rfc/RFC-0011-vnc-console.md#v1-implementation-scope)
- [database/vm-lifecycle-write-model.md §Stage 6](../database/vm-lifecycle-write-model.md#stage-6-vnc-access-write-model)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)

### Scope Boundary

This stage defines interaction behavior and token policy.
WebSocket proxy internals and storage-specific token tracking implementation are not expanded here.

### VNC Token Security (V1 Simplified)

| Security Feature | V1 Implementation | ADR-0015 Requirement |
|------------------|-------------------|----------------------|
| **Single Use** | Token marked `used_at` on first connection | ✅ Required |
| **Time-Bounded** | JWT `exp` = now + 2h | ✅ 2 hours (configurable) |
| **User-Bound** | JWT `sub` = user_id | ✅ Required |
| **Encrypted** | AES-256-GCM (shared key management) | ✅ Required |
| **Audit Logged** | `vnc.access` event | ✅ Required |

> **V1 Limitation**: No active token revocation. Security relies on short TTL and single-use flag.

### API Endpoints

```
# Request VNC access (creates approval ticket in prod)
POST /api/v1/vms/{vm_id}/console/request
→ Response: { "ticket_id": "...", "status": "PENDING_APPROVAL" }  (prod)
→ Response: { "vnc_url": "/api/v1/vms/{vm_id}/vnc", "bootstrap": "set-cookie" }  (test)

# WebSocket endpoint for noVNC
GET /api/v1/vms/{vm_id}/vnc
Upgrade: websocket
Cookie: vnc_bootstrap=<opaque one-time credential>
→ Proxies to KubeVirt VNC subresource

# Check console access status (for polling)
GET /api/v1/vms/{vm_id}/console/status
→ Response: { "status": "APPROVED", "vnc_url": "..." } | { "status": "PENDING" }
```

Compatibility rule:
- New implementation MUST NOT pass bearer/session token in URI query for VNC access.
- Legacy query-token compatibility, if temporarily retained, must be behind migration flag and removed before GA.

### Database Operations

| Environment | Persistence Behavior |
|-------------|----------------------|
| Test | No approval ticket write; access audit is mandatory. |
| Production | Create `VNC_ACCESS_REQUESTED` approval ticket, then issue token after approval and append audit records. |

Implementation details and write-set ownership are authoritative in:

- [database/vm-lifecycle-write-model.md §Stage 6](../database/vm-lifecycle-write-model.md#stage-6-vnc-access-write-model)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)

---
