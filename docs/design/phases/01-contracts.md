# Phase 1: Core Contract Definitions

> **Prerequisites**: Phase 0 complete  
> **Acceptance**: All core types defined, compiles successfully

### Required Deliverables from Phase 0

| Dependency | Location | Verification |
|------------|----------|--------------|
| Go module initialized | `go.mod` | File exists with correct module path |
| Directory structure | `internal/`, `ent/` | Directories created |
| Configuration management | `internal/config/` | Config struct compiles |
| Database connection | `internal/infrastructure/database.go` | `DatabaseClients` struct defined |
| Logging system | `internal/pkg/logger/` | zap logger configured |
| CI pipeline | `.github/workflows/ci.yml` | `golangci-lint` passes |

---

## Objectives

Define core contracts and types:

- Data models (Ent Schema)
- Provider interfaces
- Error system
- Context propagation
- Domain event types

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| **System Schema** | `ent/schema/system.go` | ⬜ | - |
| **Service Schema** | `ent/schema/service.go` | ⬜ | - |
| VM Schema | `ent/schema/vm.go` | ⬜ | - |
| VM Revision Schema | `ent/schema/vm_revision.go` | ⬜ | - |
| AuditLog Schema | `ent/schema/audit_log.go` | ⬜ | - |
| ApprovalTicket Schema | `ent/schema/approval_ticket.go` | ⬜ | - |
| ApprovalPolicy Schema | `ent/schema/approval_policy.go` | ⬜ | [ADR-0005](../../adr/ADR-0005-workflow-extensibility.md) ¹ |
| Cluster Schema | `ent/schema/cluster.go` | ⬜ | - |
| DomainEvent Schema | `ent/schema/domain_event.go` | ⬜ | - |
| PendingAdoption Schema | `ent/schema/pending_adoption.go` | ⬜ | - |
| **InstanceSize Schema** | `ent/schema/instance_size.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| **Users Schema** | `ent/schema/users.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| **AuthProviders Schema** | `ent/schema/auth_providers.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| **IdPConfigs Schema** | `ent/schema/idp_configs.go` | ⬜ | [master-flow Stage 2.B](../interaction-flows/master-flow.md) - OIDC/LDAP config ³ |
| **IdPSyncedGroups Schema** | `ent/schema/idp_synced_groups.go` | ⬜ | [master-flow Stage 2.C](../interaction-flows/master-flow.md) - Groups from IdP ³ |
| **IdPGroupMappings Schema** | `ent/schema/idp_group_mappings.go` | ⬜ | [master-flow Stage 2.C](../interaction-flows/master-flow.md) - IdP group → role ³ |
| **Roles Schema** | `ent/schema/roles.go` | ⬜ | [ADR-0018 §7](../../adr/ADR-0018-instance-size-abstraction.md), [master-flow Stage 2.A](../interaction-flows/master-flow.md) |
| **RoleBindings Schema** | `ent/schema/role_bindings.go` | ⬜ | [ADR-0018 §7](../../adr/ADR-0018-instance-size-abstraction.md), [master-flow Stage 2.B](../interaction-flows/master-flow.md) |
| **ResourceRoleBindings Schema** | `ent/schema/resource_role_bindings.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md), [master-flow Stage 4.A+](../interaction-flows/master-flow.md) |
| **ExternalApprovalSystems Schema** | `ent/schema/external_approval_systems.go` | ⬜ | [RFC-0004](../../rfc/RFC-0004-external-approval.md) ² |
| Provider interface | `internal/provider/interface.go` | ⬜ | [examples/provider/interface.go](../examples/provider/interface.go) |
| Domain models | `internal/domain/` | ⬜ | [examples/domain/](../examples/domain/) |
| Error system | `internal/pkg/errors/errors.go` | ⬜ | - |

> ³ **V1 Scope**: IdP authentication (OIDC + LDAP) is fully implemented in V1.

---

## API Contract-First Design (ADR-0021)

> **Principle**: OpenAPI 3.1 specification is the **single source of truth** for all HTTP APIs. See [ADR-0021](../../adr/ADR-0021-api-contract-first.md) for complete rationale.

### Spec-First Workflow

```
api/openapi.yaml → Code Generation → Implementation
       ↓
  oapi-codegen (Go types)
  openapi-typescript (TS types)
```

### Directory Structure

```
api/
├── openapi.yaml           # Main spec (single file for simplicity)
├── schemas/               # Reusable schema components
│   ├── common.yaml        # Pagination, Error
│   ├── governance.yaml    # System, Service, VM
│   └── rbac.yaml          # Roles, Permissions
└── paths/                 # API paths (optional split)
```

### Pagination Standard (ADR-0023)

All list APIs use standardized pagination parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number (1-indexed) |
| `per_page` | int | Items per page (default: 20, max: 100) |
| `sort_by` | string | Field to sort by |
| `sort_order` | string | `asc` or `desc` |

### Error Code Standard (ADR-0023)

Granular error codes for frontend handling:

| Code | HTTP Status | Description | V1 Status |
|------|-------------|-------------|-----------|
| `NAMESPACE_PERMISSION_DENIED` | 403 | No JIT namespace creation permission | ✅ Active |
| `NAMESPACE_QUOTA_EXCEEDED` | 403 | Cluster namespace quota reached (K8s ResourceQuota) | ✅ Active ¹ |
| `QUOTA_EXCEEDED` | 422 | Tenant resource quota exceeded | ⏳ V2+ Reserved ² |
| `CLUSTER_UNHEALTHY` | 503 | Target cluster unavailable | ✅ Active |
| `APPROVAL_REQUIRED` | 202 | Request pending approval | ✅ Active |

> **¹ NAMESPACE_QUOTA_EXCEEDED**: This error is returned when K8s rejects namespace creation due to ResourceQuota limits. The platform does NOT manage K8s quotas — it only reports K8s errors. See [master-flow.md Stage 3 JIT Namespace](../interaction-flows/master-flow.md) for error handling flow.
>
> **² QUOTA_EXCEEDED**: Reserved for future tenant-level resource quota system (CPU/Memory/VM count limits). V1 does not implement tenant quotas — this error code is a placeholder for V2+ expansion.

---

## 1. Governance Model Hierarchy

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md)**: System is decoupled from namespace/environment. See ADR for complete rationale.

```
System → Service → VM Instance
         ↑
    (Namespace specified at VM creation, not at System level)
```

| Level | Example | Uniqueness | User Self-Service | Approval Required |
|-------|---------|------------|-------------------|-------------------|
| System | `demo`, `shop` | **Global** | ✅ | No |
| Service | `redis`, `mysql` | **Global** | ✅ | No |
| VM Instance | `dev-shop-redis-01` | Per Namespace | ✅ | **Yes** |

**Key Decisions (ADR-0015)**:
- System is a **logical business grouping**, not bound to namespace or cluster
- Namespace is specified at **VM creation time**, not at System creation time
- Permissions managed via **Platform RBAC tables**, not entity fields

### 1.1 Naming Constraints (ADR-0019)

> **Security Baseline**: All platform-managed logical names MUST follow RFC 1035-based rules.

| Rule | Constraint |
|------|------------|
| **Character Set** | Lowercase letters, digits, and hyphen only (`a-z`, `0-9`, `-`) |
| **Start Character** | MUST start with a letter (`a-z`) |
| **End Character** | MUST end with a letter or digit |
| **Consecutive Hyphens** | Prohibited (`--`) — Reserved for Punycode |
| **Length Limit** | System/Service/Namespace: max 15 characters each (ADR-0015 §16) |

**Scope**: System names, Service names, Namespace names, VM name components.

**Reserved Names**: The following names are reserved and SHOULD be avoided:
- `default`, `system`, `admin`, `root`, `internal`
- Prefixes: `kube-`, `kubevirt-shepherd-`

**Validation Regex**:
```go
// RFC 1035 + no consecutive hyphens (ADR-0019)
var validNameRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)

func ValidateName(name string) error {
    if len(name) > 15 {
        return errors.New("name exceeds 15 characters")
    }
    if !validNameRegex.MatchString(name) {
        return errors.New("name must follow RFC 1035 rules")
    }
    if strings.Contains(name, "--") {
        return errors.New("consecutive hyphens are not allowed")
    }
    return nil
}
```

> 📋 **Decision reference**: [ADR-0019 §1 Naming Policy](../../adr/ADR-0019-governance-security-baseline-controls.md#1-naming-policy-most-conservative)

---

## 2. K8s Resource Labels

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) §4**: Added hostname, created-by labels.

Platform-managed resources must have these labels:

| Label | Purpose | Example |
|-------|---------|---------|
| `kubevirt-shepherd.io/managed-by` | Platform identifier | `kubevirt-shepherd` |
| `kubevirt-shepherd.io/system` | System name | `shop` |
| `kubevirt-shepherd.io/service` | Service name | `redis` |
| `kubevirt-shepherd.io/instance` | Instance number | `01` |
| `kubevirt-shepherd.io/ticket-id` | Approval ticket | `TKT-12345` |
| `kubevirt-shepherd.io/created-by` | Request creator | `alice` |
| `kubevirt-shepherd.io/hostname` | VM hostname | `dev-shop-redis-01` |

**Unique Identity**: `namespace + system + service + instance` (within a cluster)

> ⚠️ **User-Forbidden Labels**: Users cannot set labels directly. All labels are platform-managed for governance integrity.

---

## 3. Core Ent Schemas

### 3.1 System Schema

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) §1**: Removed `namespace`, `environment` fields. System is now a logical grouping decoupled from infrastructure.

```go
// ent/schema/system.go

func (System) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),
        field.String("description").Optional(),
        field.String("created_by").NotEmpty(),
        // NOTE: No maintainers field - permissions managed via RoleBinding table (ADR-0015 §22)
        field.String("tenant_id").Default("default").Immutable(),  // Multi-tenancy reserved
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (System) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("name").Unique(),  // Globally unique (ADR-0015 §16)
    }
}

func (System) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("services", Service.Type),
    }
}
```

**Removed Fields** (per ADR-0015 §1):

| Field | Reason for Removal |
|-------|--------------------|
| `namespace` | Namespace is specified at VM creation, not System level |
| `environment` | Environment is determined by namespace, not System |
| `maintainers` ❌ **Not added** | Permissions managed via RoleBinding table |

### 3.2 Service Schema

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) §2**: Removed `created_by`. Service inherits permissions from parent System via RoleBinding. Name is immutable after creation.

```go
// ent/schema/service.go

func (Service) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty().Immutable(),           // Cannot change after creation (ADR-0015 §2)
        field.String("description").Optional(),
        field.Int("next_instance_index").Default(1),
        field.Time("created_at").Default(time.Now).Immutable(),
        // NOTE: No created_by, no maintainers - fully inherited from System (ADR-0015 §2)
    }
}

func (Service) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("system", System.Type).Ref("services").Unique().Required(),
        edge.To("vms", VM.Type),
    }
}
```

**Removed Fields** (per ADR-0015 §2):

| Field | Reason for Removal |
|-------|--------------------|
| `created_by` | Inherited from System |
| `maintainers` | Inherited from System via RoleBinding |

### 3.3 DomainEvent Schema (ADR-0009)

> **Reference**: [examples/domain/event.go](../examples/domain/event.go)

Key constraints:
- **Payload is immutable** (append-only)
- Modifications stored in `ApprovalTicket.modified_spec` (full replacement)
- `archived_at` field for soft archiving

### 3.4 ApprovalTicket Admin Fields (ADR-0017)

> **Added by [ADR-0017](../../adr/ADR-0017-vm-request-flow-clarification.md)**: Admin-determined fields during approval workflow.

| Field | Type | Description |
|-------|------|-------------|
| `selected_cluster_id` | string | Admin selects target cluster during approval |
| `selected_template_version` | int | Admin confirms template version |
| `selected_storage_class` | string | From cluster's available storage classes |
| `template_snapshot` | JSONB | Full template configuration at approval time (immutable) |
| `instance_size_snapshot` | JSONB | InstanceSize configuration at approval time (ADR-0018) |

> **Security Note**: User-provided `namespace` is **immutable after submission**. Admin can only approve/reject, never modify the namespace. This prevents permission escalation attacks.

### 3.5 Instance Number Design

**Rule**: Instance numbers permanently increment, **no reset API**.

**Reason**: Prevents "ghost instance" resurrection conflicts when clusters recover after failures.

---

## 4. Provider Interfaces

> **Reference**: [examples/provider/interface.go](../examples/provider/interface.go)

### Interface Hierarchy

```
InfrastructureProvider (base)        ← Phase 2: Full implementation
├── SnapshotProvider                 ← Phase 2: Interface only (RFC-0013)
├── CloneProvider                    ← Phase 2: Interface only (RFC-0014)
├── MigrationProvider                ← Phase 2: Basic methods only
├── InstanceTypeProvider             ← Phase 2: Full implementation
└── ConsoleProvider                  ← Phase 2: Interface only (RFC-0011)
         ↓
   KubeVirtProvider (combined)
```

> **Interface vs Implementation Scope**:
> 
> | Provider | Phase 2 Delivers | Full Implementation |
> |----------|------------------|---------------------|
> | InfrastructureProvider | Full | Phase 2 |
> | MigrationProvider | `MigrateVM()`, `GetVMMigration()` | Phase 2 (basic) |
> | SnapshotProvider | Interface definition only | [RFC-0013](../../rfc/RFC-0013-vm-snapshot.md) |
> | CloneProvider | Interface definition only | [RFC-0014](../../rfc/RFC-0014-vm-clone.md) |
> | ConsoleProvider | Interface definition only | [RFC-0011](../../rfc/RFC-0011-vnc-console.md) |
>
> **Why define interfaces early?** Pre-defining interfaces ensures Service layer code can be written against stable contracts, enabling parallel RFC development without refactoring core code.

### Anti-Corruption Layer

All Provider methods return domain types, **not** K8s types:

```go
// ✅ Correct
func (p *KubeVirtProvider) GetVM(...) (*domain.VM, error)

// ❌ Forbidden
func (p *KubeVirtProvider) GetVM(...) (*kubevirtv1.VirtualMachine, error)
```

---

## 5. Multi-Cluster Credential Management

### Design Principles

- Unified Kubeconfig format (uploaded via API)
- Encrypted storage in database (AES-256-GCM)
- No file-based configuration
- Dynamic hot-loading (no restart required)

### Cluster Schema Fields

| Field | Type | Purpose |
|-------|------|---------|
| `encrypted_kubeconfig` | bytes | AES-256-GCM encrypted |
| `encryption_key_id` | string | Key rotation support |
| `api_server_url` | string | Parsed from kubeconfig |
| `status` | enum | UNKNOWN, HEALTHY, UNHEALTHY, UNREACHABLE |
| `kubevirt_version` | string | Detected version |
| `enabled_features` | []string | Detected feature gates |

### CredentialProvider Interface

```go
type CredentialProvider interface {
    GetRESTConfig(ctx context.Context, clusterName string) (*rest.Config, error)
    Type() string
}

// Phase 1: KubeconfigProvider (from database)
// Future: VaultProvider, ExternalSecretProvider
```

---

## 6. Error System

### Design Principles

- Errors contain `code` + `params` only, no hardcoded messages
- Frontend handles i18n translation
- Backend logs always in English

```go
type AppError struct {
    Code   string                 `json:"code"`
    Params map[string]interface{} `json:"params,omitempty"`
}

const (
    ErrVMNotFound       = "VM_NOT_FOUND"
    ErrClusterDegraded  = "CLUSTER_DEGRADED"
    ErrApprovalRequired = "APPROVAL_REQUIRED"
)
```

---

## 7. Extension Interfaces

| Interface | Purpose | Phase 1 Implementation |
|-----------|---------|------------------------|
| `AuthProvider` | Authentication | JWT |
| `ApprovalProvider` | Approval workflow | Internal |
| `NotificationProvider` | Notifications | Log (noop) |
| `CredentialProvider` | Cluster credentials | Kubeconfig |

---

## Ent Usage Standards (CI Enforcement)

| Rule | CI Script |
|------|-----------|
| Run `go generate ./ent` after schema changes | `check_ent_codegen.go` |
| No handwritten SQL strings | `check_forbidden_imports.go` |
| Transaction boundaries at UseCase layer | `check_transaction_boundary.go` |

---

## Acceptance Criteria

- [ ] All Ent schemas compile (`go generate ./ent`)
- [ ] Provider interfaces compile
- [ ] Domain types defined
- [ ] Error codes defined
- [ ] CI checks pass

---

## Related Documentation

- [CHECKLIST.md](../CHECKLIST.md) - Phase 1 acceptance items
- [examples/provider/interface.go](../examples/provider/interface.go)
- [examples/domain/](../examples/domain/)
- [ADR-0005](../../adr/ADR-0005-workflow-extensibility.md) - Workflow Extensibility (Simplified Approval)
- [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md) - Domain Event Pattern
- [ADR-0014](../../adr/ADR-0014-capability-detection.md) - Capability Detection
- [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) - Governance Model V2 (Entity Decoupling, RBAC)
- [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md) - Go Module Vanity Import
- [ADR-0017](../../adr/ADR-0017-vm-request-flow-clarification.md) - VM Request Flow (Cluster selection at approval time)
- [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) - Instance Size Abstraction (InstanceSize, Users, AuthProviders schemas)
- [RFC-0004](../../rfc/RFC-0004-external-approval.md) - External Approval Systems (Deferred → V1+)

---

## Footnotes

> **¹ ApprovalPolicy Scope (ADR-0005)**: 
> 
> ApprovalPolicy defines **environment-level policies** (e.g., "prod environment requires approval for VM creation").
> This is **NOT** multi-level approval. Per ADR-0005, the following are explicitly **out of scope for V1**:
> 
> | Feature | V1 Status | Roadmap |
> |---------|-----------|---------|
> | Multi-level approval (L1 → L2 → L3) | ❌ Not implementing | P2 Future |
> | Withdraw/Countersign/Transfer | ❌ Not implementing | P3 Never |
> | Timeout auto-processing | ❌ Not implementing | P2 Future |
> 
> ApprovalPolicy supports only: `PENDING → APPROVED` or `PENDING → REJECTED` (two paths, no intermediate states).

> **² ExternalApprovalSystems (RFC-0004)**:
> 
> RFC-0004 status is `Proposed` (P1, V1+ optional). Design defined in [Master Flow Stage 2.E](../interaction-flows/master-flow.md#stage-2-e).
> Review Period: Until 2026-02-01. This is an **optional feature** - if external approval is not configured, the built-in approval engine is used.
> Security: TLS mandatory, HMAC signature verification, fallback to built-in on failure.

