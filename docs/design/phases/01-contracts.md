# Phase 1: Core Contract Definitions

> **Prerequisites**: Phase 0 complete  
> **Acceptance**: All core types defined, compiles successfully

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
| ApprovalPolicy Schema | `ent/schema/approval_policy.go` | ⬜ | - |
| Cluster Schema | `ent/schema/cluster.go` | ⬜ | - |
| DomainEvent Schema | `ent/schema/domain_event.go` | ⬜ | - |
| PendingAdoption Schema | `ent/schema/pending_adoption.go` | ⬜ | - |
| Provider interface | `internal/provider/interface.go` | ⬜ | [examples/provider/interface.go](../examples/provider/interface.go) |
| Domain models | `internal/domain/` | ⬜ | [examples/domain/](../examples/domain/) |
| Error system | `internal/pkg/errors/errors.go` | ⬜ | - |

---

## 1. Governance Model Hierarchy

```
Namespace (K8s) → System → Service → VM Instance
```

| Level | Example | User Self-Service | Approval Required |
|-------|---------|-------------------|-------------------|
| Namespace | `dev`, `prod` | ❌ | Admin only |
| System | `demo`, `shop` | ✅ | No |
| Service | `redis`, `mysql` | ✅ | No |
| VM Instance | `redis-06` | ✅ | **Yes** |

---

## 2. K8s Resource Labels

Platform-managed resources must have these labels:

| Label | Purpose | Example |
|-------|---------|---------|
| `kubevirt-shepherd.io/managed-by` | Platform identifier | `kubevirt-shepherd` |
| `kubevirt-shepherd.io/system` | System name | `demo` |
| `kubevirt-shepherd.io/service` | Service name | `redis` |
| `kubevirt-shepherd.io/instance` | Instance number | `06` |
| `kubevirt-shepherd.io/ticket-id` | Approval ticket | `TKT-123` |

**Unique Identity**: `cluster + namespace + system + service + instance`

---

## 3. Core Ent Schemas

### 3.1 System Schema

```go
// ent/schema/system.go

func (System) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").NotEmpty(),
        field.String("namespace").NotEmpty(),
        field.String("documentation").Optional(),
        field.String("created_by").NotEmpty(),
        field.Enum("environment").Values("test", "prod").Default("test"),
        field.Time("created_at").Default(time.Now),
    }
}

func (System) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("namespace", "name").Unique(),
    }
}

func (System) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("services", Service.Type),
    }
}
```

### 3.2 Service Schema

```go
// ent/schema/service.go

func (Service) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").NotEmpty(),
        field.String("documentation").Optional(),
        field.String("created_by").NotEmpty(),
        field.Int("next_instance_index").Default(1), // Permanent increment, no reset
        field.Time("created_at").Default(time.Now),
    }
}

func (Service) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("system", System.Type).Ref("services").Unique().Required(),
        edge.To("vms", VM.Type),
    }
}
```

### 3.3 DomainEvent Schema (ADR-0009)

> **Reference**: [examples/domain/event.go](../examples/domain/event.go)

Key constraints:
- **Payload is immutable** (append-only)
- Modifications stored in `ApprovalTicket.modified_spec` (full replacement)
- `archived_at` field for soft archiving

### 3.4 Instance Number Design

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
- [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md) - Domain Event Pattern
- [ADR-0014](../../adr/ADR-0014-capability-detection.md) - Capability Detection
