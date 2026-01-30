# Phase 1 Checklist: Core Contract Definitions

> **Detailed Document**: [phases/01-contracts.md](../phases/01-contracts.md)

---

## Core Types (Ent Schema)

> **Governance Model Hierarchy** (ADR-0015): `System → Service → VM Instance`
>
> System is a logical grouping decoupled from namespace. Namespace specified at VM creation.

- [ ] `ent/schema/` directory created
- [ ] **Governance Model Core Schema**:
  - [ ] `ent/schema/system.go` - System/Project (e.g., demo, shop)
    - [ ] Contains `id` field (immutable)
    - [ ] Contains `description` field (optional)
    - [ ] Contains `created_by` field
    - [ ] Contains `tenant_id` field (default: "default", reserved for multi-tenancy)
    - [ ] ❌ **No `namespace` field** (ADR-0015 §1)
    - [ ] ❌ **No `environment` field** (ADR-0015 §1)
    - [ ] ❌ **No `maintainers` field** - use RoleBinding table (ADR-0015 §22)
    - [ ] Globally unique name constraint
    - [ ] **User self-service creation, no approval required**
  - [ ] `ent/schema/service.go` - Service (e.g., redis, mysql)
    - [ ] Contains `id` field (immutable)
    - [ ] Contains `name` field (**immutable after creation**, ADR-0015 §2)
    - [ ] Contains `description` field (optional)
    - [ ] ❌ **No `created_by` field** - inherited from System (ADR-0015 §2)
    - [ ] Contains `next_instance_index` field (**permanently incrementing, no reset**)
    - [ ] Globally unique name constraint
    - [ ] **User self-service creation, no approval required**
- [ ] `ent/schema/vm.go` - VM Schema definition
  - [ ] Associates `service_id` **only** (ADR-0015 §3)
  - [ ] ❌ **No `system_id` field** - obtain via service edge (ADR-0015 §3)
  - [ ] `instance` field stores instance number (e.g., "01")
- [ ] `ent/schema/vm_revision.go` - VM version history
- [ ] `ent/schema/audit_log.go` - Audit log Schema
- [ ] `ent/schema/approval_ticket.go` - Approval ticket (Governance Core)
- [ ] `ent/schema/approval_policy.go` - Approval policy (Governance Core)
- [ ] `ent/schema/cluster.go` - Multi-cluster credential management
- [ ] `ent/schema/template.go` - Template definition
- [ ] `ent/schema/resource_spec.go` - Resource spec template
- [ ] `ent/schema/pending_adoption.go` - Pending adoption resources
- [ ] `ent/schema/domain_event.go` - Domain event (ADR-0009)
- [ ] `ent/schema/infra_worker_pod.go` - Worker Pod registry

---

## ResourceSpec Overcommit Design

- [ ] `cpu_request` defaults to `cpu_limit` (no overcommit)
- [ ] `memory_request_mb` defaults to `memory_limit_mb`
- [ ] Admin can set `request < limit` for overcommit
- [ ] User-facing API only returns limit fields

---

## Instance Number Design (Permanently Incrementing)

- [ ] `Service.next_instance_index` only increases
- [ ] VM creation auto-increments
- [ ] ❌ No reset API provided

---

## Multi-cluster Credential Management

- [ ] **Cluster Schema Fields** complete
- [ ] **Encryption Service** (`internal/pkg/crypto/cluster_crypto.go`) implemented
- [ ] **CredentialProvider Interface** (Strategy Pattern) defined
- [ ] **ClusterRepository** methods implemented
- [ ] **Admin API** for dynamic cluster management
- [ ] **File-based Approach Forbidden** (CI detection)

---

## Ent Usage Standards (CI Enforcement)

- [ ] **Schema Definition Standards** followed
- [ ] **Code Generation Sync** (CI detection)
- [ ] **Dynamic Queries Must Be Type-Safe**
- [ ] **Transaction Management** per ADR-0012
- [ ] **Test Infrastructure** (PostgreSQL via testcontainers-go)
- [ ] **Test Coverage** (CI enforcement)

---

## Contract Interfaces

- [ ] `InfrastructureProvider` base interface definition
- [ ] `KubeVirtProvider` specialized interface definition
- [ ] `ResourceSpec` type definition
- [ ] `ResourceStatus` type definition
- [ ] `ValidationResult` type definition
- [ ] KubeVirt-specific types defined

---

## Extension Interfaces

- [ ] **AuthProvider Interface** defined
- [ ] **JWT Implementation** completed
- [ ] **ApprovalProvider Interface** defined
- [ ] **NotificationProvider Interface** defined

---

## Platform RBAC Schema (ADR-0015 §22, ADR-0019)

- [ ] `ent/schema/permission.go` - Atomic permission definitions
- [ ] `ent/schema/role.go` - Role = bundle of permissions
- [ ] `ent/schema/role_binding.go` - User-role assignments with scope
- [ ] `ent/schema/resource_role_binding.go` - Resource-level member management (owner/admin/member/viewer)
- [ ] Built-in roles seeded (per master-flow.md Stage 2.A):
  - [ ] **Bootstrap** - Initial setup only (`*:*`), ⚠️ MUST be disabled after initialization
  - [ ] **PlatformAdmin** - Super admin (explicit permissions, NO wildcards)
  - [ ] **SystemAdmin** - Resource management (`system:*`, `service:*`, `vm:*`)
  - [ ] **Approver** - Can approve requests (`approval:approve`, `approval:view`)
  - [ ] **Operator** - Power operations (`vm:operate`, `vm:read`)
  - [ ] **Viewer** - Read-only access (explicit: `system:read`, `service:read`, `vm:read`, `template:read`, `instance_size:read`) ⚠️ **NO `*:read` wildcard** (ADR-0019)
- [ ] Environment-based permission control (`allowed_environments` field)

---

## Provider Configuration Type Safety

- [ ] `ProviderConfig` uses interface type (not `map[string]interface{}`)
- [ ] `ParseProviderConfig()` implements Discriminated Union logic
- [ ] Validation using `go-playground/validator`

---

## Error System

- [ ] `AppError` struct definition
- [ ] `ErrorCode` constants definition
- [ ] Errors only contain `code` + `params`, no hardcoded messages

---

## Context

- [ ] `AppContext` struct definition
- [ ] Context passing uses `context.Context`
- [ ] Request ID middleware

## Go Module Configuration (ADR-0016)

- [ ] `go.mod` uses vanity import path: `kv-shepherd.io/shepherd`
- [ ] All internal imports use vanity path: `kv-shepherd.io/shepherd/internal/...`
- [ ] Vanity import server configured (for production deployment)

---

## Pre-Phase 2 Verification

- [ ] `go generate ./ent` generates code without errors
- [ ] Ent Schema unit tests 100% pass
- [ ] Provider interface definitions compile without errors
