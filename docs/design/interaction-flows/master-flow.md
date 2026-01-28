# Master Interaction Flow

> **Status**: Stable (ADR-0017, ADR-0018 Accepted)  
> **Version**: 1.0  
> **Date**: 2026-01-28  
> **Language**: English (Canonical Version)  
> **Source**: Extracted from ADR-0018 Appendix

---

## Document Purpose

This document is the **canonical reference** for all Shepherd platform interaction flows, serving as the single source of truth for frontend, backend, and database development.

**Related Documents**:
- [ADR-0018: Instance Size Abstraction](../../adr/ADR-0018-instance-size-abstraction.md)
- [ADR-0015: Governance Model V2](../../adr/ADR-0015-governance-model-v2.md)
- [ADR-0017: VM Request Flow](../../adr/ADR-0017-vm-request-flow-clarification.md)

---

## Document Structure

| Part | Content | Involved Roles |
|------|---------|----------------|
| **Part 1** | Platform Initialization (Schema/Mask, **First Deployment Bootstrap**, RBAC/Permissions, OIDC/LDAP Authentication, IdP Group Mapping, **External Approval Systems**, Cluster/InstanceSize/Template Configuration) | Developer, Platform Admin |
| **Part 2** | Resource Management (System/Service CRUD with database operations, **including audit logs**) | Regular User |
| **Part 3** | VM Lifecycle (Create Request → Approval → Execution → Delete with database operations, **including audit logs**) | Regular User, Platform Admin |
| **Part 4** | State Machines & Data Models (State transition diagrams, Table relationships, **Audit log design**) | All Developers |

---

## Core Design Principles

| Principle | Description |
|-----------|-------------|
| **Schema as Single Source of Truth** | KubeVirt official JSON Schema defines all field types, constraints, enum options. We do NOT duplicate this in code. |
| **Mask Only Selects Paths** | Mask only specifies which Schema paths to expose. It does NOT define field options. |
| **Dumb Backend** | Backend stores `map[string]interface{}` and does NOT interpret field semantics. |
| **Schema-Driven Frontend** | Frontend reads JSON Schema + Mask to render appropriate UI components. |

---

## Role Definitions

| Role | Responsibilities | Layer |
|------|------------------|-------|
| **Developer** | Obtain KubeVirt Schema, define Mask (select exposed paths) | Code/Config Layer |
| **Platform Admin** | Create InstanceSize (fill values via Schema-driven forms), configure RBAC | Admin Backend Layer |
| **Regular User** | Select InstanceSize, submit VM creation requests | Business Usage Layer |

---

## Part 1: Platform Initialization Flow

### Stage 1: Developer Operations (Schema/Mask)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 1: Platform Initialization (Developer)              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Developer:                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. Obtain KubeVirt Official JSON Schema                                │ │
│  │    - Source: KubeVirt CRD OpenAPI Schema or official docs              │ │
│  │    - Contains: All field types, constraints, enum options              │ │
│  │                                                                        │ │
│  │ 2. Define Mask Configuration (select paths only, not options)          │ │
│  │    - mask.yaml specifies which Schema paths to expose                  │ │
│  │                                                                        │ │
│  │ 3. Integrate Schema + Mask into platform codebase                      │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stage 1.5: First Deployment Bootstrap {#stage-1-5}

> **Added 2026-01-26**: Configuration storage strategy - First deployment bootstrap

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 1.5: First Deployment Bootstrap                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Deployment Configuration (Choose One):                                      │
│                                                                              │
│  📁 Option A: config.yaml (Local Development / Traditional Deploy)          │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  # config.yaml                                                         │ │
│  │  database:                                                             │ │
│  │    url: "postgresql://user:pass@localhost:5432/shepherd"               │ │
│  │                                                                        │ │
│  │  server:                                                               │ │
│  │    port: 8080                                                          │ │
│  │    log_level: "info"               # Optional, default: info           │ │
│  │                                                                        │ │
│  │  worker:                                                               │ │
│  │    max_workers: 10                 # Optional, default: 10             │ │
│  │                                                                        │ │
│  │  security:                                                             │ │
│  │    encryption_key: "32-byte-hex"   # Optional, for encrypting secrets  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  🐳 Option B: Environment Variables (Containerized Deployment)              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  DATABASE_URL=postgresql://user:pass@host:5432/shepherd   # Required   │ │
│  │  SERVER_PORT=8080                  # Optional, default: 8080           │ │
│  │  LOG_LEVEL=info                    # Optional, default: info           │ │
│  │  RIVER_MAX_WORKERS=10              # Optional, default: 10             │ │
│  │  ENCRYPTION_KEY=<32-byte-hex>      # Optional, encrypt secrets         │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ⚡ Priority: Environment Variables > config.yaml > defaults                │
│  💡 Env vars always override config.yaml (12-factor app principle)          │
│                                                                              │
│  Application Auto-initialization:                                            │
│  1. Run database migrations                                                  │
│  2. Seed built-in roles (ON CONFLICT DO NOTHING)                            │
│  3. Seed default admin: admin/admin (force_password_change=true)            │
│                                                                              │
│  First Login:                                                                │
│  - Login with admin/admin                                                    │
│  - Force password change dialog                                              │
│  - Enter admin console                                                       │
│                                                                              │
│  Configuration Storage Strategy:                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  config.yaml / Env Vars (Infrastructure Only)                         │   │
│  │  - DATABASE_URL, SERVER_PORT, LOG_LEVEL, ENCRYPTION_KEY               │   │
│  │                                                                        │   │
│  │  PostgreSQL (All Runtime Configuration)                                │   │
│  │  - users, auth_providers, external_approval_systems                   │   │
│  │  - roles, role_bindings, resource_role_bindings                       │   │
│  │  - clusters, instance_sizes, templates                                 │   │
│  │  - All business data + audit_logs                                     │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stage 2: Security Configuration (RBAC/Authentication)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 2: Security Configuration (Platform Admin)          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  2.A: Built-in Roles (Pre-configured)                                        │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  PlatformAdmin  │ *:*                    │ Full platform control       │ │
│  │  SystemAdmin    │ system:*, service:*    │ Manage systems/services     │ │
│  │  Approver       │ approval:*, vm:*       │ Approve VM requests         │ │
│  │  Operator       │ vm:operate             │ VM power operations         │ │
│  │  Viewer         │ *:read                 │ Read-only access            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  2.A+: Custom Role Management (Optional)                                     │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Admin can create custom roles with specific permission combinations   │ │
│  │  e.g., DevLead = system:*, service:*, vm:read                         │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  2.B: Authentication Configuration (OIDC/LDAP) - Via Web UI                  │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Configure OIDC provider or LDAP connection                            │ │
│  │  All config stored in PostgreSQL (auth_providers table)               │ │
│  │  Sensitive data (client_secret) encrypted with AES-256-GCM            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  2.C: IdP Group Mapping                                                      │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Map IdP groups to platform roles                                      │ │
│  │  Configure allowed environments per group                              │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  2.E: External Approval Systems (Optional) - Via Web UI                      │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Configure external approval integrations (Webhook, ServiceNow, Jira)  │ │
│  │  All config stored in PostgreSQL (external_approval_systems table)    │ │
│  │  Sensitive data (webhook_secret) encrypted with AES-256-GCM           │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stage 3: Admin Configuration (Cluster/InstanceSize/Template)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 3: Admin Configuration                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  3.A: Cluster Registration                                                   │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  - Add cluster with kubeconfig                                         │ │
│  │  - Set environment type (test/prod)                                    │ │
│  │  - Configure scheduling weight                                         │ │
│  │  - Auto-detect capabilities (GPU, Hugepages, SR-IOV)                  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  3.B: InstanceSize Creation                                                  │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  - Schema-driven form for CPU/Memory/Disk                             │ │
│  │  - Optional: Overcommit ratios, capability requirements               │ │
│  │  - Stored as spec_overrides JSON                                      │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  3.C: Template Creation                                                      │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  - Select OS image source                                              │ │
│  │  - Configure cloud-init (one-time password)                           │ │
│  │  - Define field visibility (quick/advanced modes)                     │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 2: Resource Management Flow

### Stage 4.A: Create System

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 4.A: Create System (User)                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User operations:                                                            │
│  - Enter system name (globally unique, max 15 chars)                        │
│  - Enter description (optional, markdown supported)                         │
│                                                                              │
│  Database operations:                                                        │
│  - INSERT INTO systems (id, name, description, created_by, ...)             │
│  - INSERT INTO resource_role_bindings (user_id, role='owner', ...)          │
│  - INSERT INTO audit_logs (action='system.create', ...)                     │
│                                                                              │
│  Result:                                                                     │
│  - Creator automatically becomes System Owner                               │
│  - Other users cannot see this System by default                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stage 4.A+: Resource-level Member Management

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 4.A+: Member Management (Owner/Admin)               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Core Design: System Owner can add other users to their System              │
│  - No platform admin involvement required (self-service)                    │
│  - Service/VM permissions fully inherited from System                       │
│                                                                              │
│  Resource-level Roles:                                                       │
│  ┌────────────┬────────┬────────┬────────┬────────┐                        │
│  │ Operation  │ Owner  │ Admin  │ Member │ Viewer │                        │
│  ├────────────┼────────┼────────┼────────┼────────┤                        │
│  │ View       │   ✅   │   ✅   │   ✅   │   ✅   │                        │
│  │ Create     │   ✅   │   ✅   │   ✅   │   ❌   │                        │
│  │ Modify     │   ✅   │   ✅   │   ❌   │   ❌   │                        │
│  │ Delete     │   ✅   │   ✅   │   ❌   │   ❌   │                        │
│  │ Manage     │   ✅   │   ✅   │   ❌   │   ❌   │ ← System level only    │
│  │ Transfer   │   ✅   │   ❌   │   ❌   │   ❌   │                        │
│  └────────────┴────────┴────────┴────────┴────────┘                        │
│                                                                              │
│  Permission Boundary:                                                        │
│  - Shepherd controls: Resource visibility, lifecycle management, VNC       │
│  - Shepherd does NOT control: SSH/RDP login (managed by bastion)           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 3: VM Lifecycle Flow

### Stage 5.A: VM Creation Request

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 5.A: VM Creation Request (User)                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User provides:                                                              │
│  - ServiceID (which Service this VM belongs to)                             │
│  - TemplateID (requested template)                                          │
│  - Namespace (target K8s namespace)                                         │
│  - Resource parameters (CPU, Memory, Disk)                                  │
│  - Reason (business justification)                                          │
│                                                                              │
│  User does NOT provide (admin determines during approval):                  │
│  - ClusterID                                                                │
│  - VM Name (platform-generated)                                             │
│  - Labels (platform-managed)                                                │
│  - CloudInit (template-defined)                                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stage 5.B: Approval Workflow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Stage 5.B: Approval Workflow (Admin)                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Admin determines:                                                           │
│  - Target cluster (based on namespace environment, capacity, policy)        │
│  - Final template version                                                   │
│  - Storage class                                                            │
│  - Any parameter overrides (CPU, Memory, etc.)                              │
│                                                                              │
│  Admin CANNOT modify (Security - prevents permission escalation):           │
│  - Namespace (user-provided and immutable after submission) [ADR-0017]      │
│  - ServiceID (determines ownership and inheritance)                         │
│                                                                              │
│  Approval decisions:                                                         │
│  - APPROVE → VM creation proceeds                                           │
│  - REJECT → Request closed with reason                                      │
│  - REQUEST_CHANGES → User revises and resubmits                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 4: State Machines & Data Models

### Dual-layer Permission Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Permission Check Flow                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User Request → Resource R                                                   │
│        │                                                                     │
│        ▼                                                                     │
│  ┌─ Step 1: Global RBAC Check ─────────────────────────────────────────────┐│
│  │  Query role_bindings → Aggregate Permissions                            ││
│  │  - PlatformAdmin (*:*) → Allow all                                      ││
│  │  - Has required permission → Continue to Step 2                         ││
│  │  - Otherwise → Deny                                                     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│        │                                                                     │
│        ▼                                                                     │
│  ┌─ Step 2: Resource-level RBAC Check ─────────────────────────────────────┐│
│  │  Query resource_role_bindings (resource_id, user_id)                    ││
│  │  - Found binding → Return role permissions                              ││
│  │  - Not found → Traverse inheritance chain (VM→Service→System)          ││
│  │  - Still not found → Deny (resource invisible to user)                 ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

Permission Inheritance:
  System (shop)                    ← Members configured here
    ├─ Admin: Li Si
    ├─ Member: Wang Wu, Zhao Liu
    │
    ├── Service (redis)            ← Auto-inherit from System
    │     ├── VM (redis-01)        ← Auto-inherit
    │     └── VM (redis-02)        ← Auto-inherit
    │
    └── Service (mysql)            ← Auto-inherit
          └── VM (mysql-01)        ← Auto-inherit

Benefits:
  - Add/remove members only at System level, Service/VM auto-update
  - Consistent with Google Cloud IAM and GitHub Teams models
```

---

## Database Tables Summary

### Global RBAC Tables

```sql
-- Roles (built-in + custom)
CREATE TABLE roles (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    permissions JSON NOT NULL,  -- ['system:*', 'vm:create', ...]
    is_builtin BOOLEAN DEFAULT FALSE
);

-- Role bindings (OIDC/LDAP mapping)
CREATE TABLE role_bindings (
    id VARCHAR PRIMARY KEY,
    user_id VARCHAR NOT NULL,
    role_id VARCHAR NOT NULL REFERENCES roles(id),
    scope_type VARCHAR NOT NULL,  -- global, system, service
    allowed_environments TEXT[],   -- ['test', 'prod']
    source VARCHAR NOT NULL        -- 'idp_mapping', 'manual'
);
```

### Resource-level RBAC Tables

```sql
-- Resource role bindings (user self-service)
CREATE TABLE resource_role_bindings (
    id VARCHAR PRIMARY KEY,
    user_id VARCHAR NOT NULL,
    role VARCHAR NOT NULL,          -- owner, admin, member, viewer
    resource_type VARCHAR NOT NULL, -- system, service, vm
    resource_id VARCHAR NOT NULL,
    granted_by VARCHAR NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
```

---

## Changelog

| Date | Version | Change |
|------|---------|--------|
| 2026-01-28 | 1.0 | **STABLE**: ADR-0017 and ADR-0018 accepted |
| 2026-01-28 | 1.0 | Added: Stage 5.B Namespace immutability constraint (Admin CANNOT modify Namespace) |
| 2026-01-28 | 1.0 | Verified: roles, role_bindings, resource_role_bindings tables match ADR-0018 §7 |
| 2026-01-26 | 0.1-draft | Updated: Support both config.yaml and env vars for infrastructure config |
| 2026-01-26 | 0.1-draft | Added: Configuration Storage Strategy - PostgreSQL-first design |
| 2026-01-26 | 0.1-draft | Added: First Deployment Bootstrap flow (admin/admin + force password change) |
| 2026-01-26 | 0.1-draft | Added: External Approval Systems configuration (Stage 2.E) |
| 2026-01-26 | 0.1-draft | Initial extraction from ADR-0018 Appendix |

---

> **Translations**: For the Chinese version with complete detailed flows, please refer to [i18n/zh-CN/design/interaction-flows/master-flow.md](../../i18n/zh-CN/design/interaction-flows/master-flow.md)
