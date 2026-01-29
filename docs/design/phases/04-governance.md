# Phase 4: Governance Capabilities

> **Prerequisites**: Phase 3 complete  
> **Acceptance**: Approval workflow operational, River Queue processing

### Required Deliverables from Phase 3

| Dependency | Location | Verification |
|------------|----------|--------------|
| Composition Root | `internal/app/bootstrap.go` | Application boots successfully |
| VMService | `internal/service/vm_service.go` | Business logic callable |
| CreateVMUseCase | `internal/usecase/create_vm.go` | Atomic transaction works |
| VMHandler | `internal/api/handlers/vm.go` | HTTP endpoints respond |
| Health checks | `/health/live`, `/health/ready` | Both return 200 |
| Manual DI pattern | All `New*()` in bootstrap.go | CI check passes |

---

## Objectives

Implement governance capabilities:

- Database migrations (Atlas)
- River Queue integration (ADR-0006)
- Domain Event pattern (ADR-0009)
- Approval workflow
- Template engine (ADR-0007, ADR-0011)
- Environment isolation

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| Atlas config | `atlas.hcl` | ⬜ | - |
| River Jobs | `internal/jobs/` | ⬜ | - |
| EventDispatcher | `internal/domain/dispatcher.go` | ⬜ | - |
| ApprovalGateway | `internal/governance/approval/` | ⬜ | - |
| AuditLogger | `internal/governance/audit/` | ⬜ | - |
| TemplateService | `internal/service/template.go` | ⬜ | - |
| SSAApplier | `internal/provider/ssa_applier.go` | ⬜ | - |

---

## 1. Database Migration

### Atlas Configuration

```hcl
# atlas.hcl
env "local" {
  src = "ent://ent/schema"
  url = "postgres://user:pass@localhost:5432/kubevirt_shepherd?sslmode=disable"
  dev = "docker://postgres/18/dev"
}
```

### Migration Commands

```bash
# Generate migration
atlas migrate diff --env local

# Apply migration
atlas migrate apply --env local

# Rollback test (CI required)
atlas migrate apply → atlas migrate down → atlas migrate apply
```

---

## 2. River Queue (ADR-0006)

### Job Definition

```go
// internal/jobs/event_job.go

type EventJobArgs struct {
    EventID string `json:"event_id"`
}

func (EventJobArgs) Kind() string { return "event_job" }

// Deprecated: Don't use specific args
// type CreateVMArgs struct { ... }  // ❌ Use EventJobArgs instead
```

### Worker Registration

```go
workers := river.NewWorkers()
river.AddWorker(workers, &EventJobWorker{
    dispatcher: eventDispatcher,
})

riverClient, _ := river.NewClient(driver, &river.Config{
    Queues: map[string]river.QueueConfig{
        river.QueueDefault: {MaxWorkers: 10},
    },
    Workers: workers,
})
```

### Handler Pattern

```go
// POST /api/v1/vms → 202 Accepted + event_id
func (h *VMHandler) Create(c *gin.Context) {
    result, _ := h.createVMUseCase.Execute(ctx, req)
    c.JSON(202, gin.H{
        "event_id":  result.EventID,
        "ticket_id": result.TicketID,
    })
}

// Worker executes actual K8s operation
func (w *EventJobWorker) Work(ctx context.Context, job *river.Job[EventJobArgs]) error {
    event, _ := w.eventRepo.Get(ctx, job.Args.EventID)
    return w.dispatcher.Dispatch(event)
}
```

---

## 3. Domain Event Pattern (ADR-0009)

> **Reference**: [examples/domain/event.go](../examples/domain/event.go)

### Key Constraints

| Constraint | Implementation |
|------------|----------------|
| Payload immutable | Append-only, never update |
| Modifications in ticket | `ApprovalTicket.modified_spec` (full replacement) |
| Get final spec | `GetEffectiveSpec(originalPayload, modifiedSpec)` |
| No merge | **Forbidden** to merge specs |

### Event Status Flow

```
PENDING → PROCESSING → COMPLETED   # Per ADR-0009 L156
                    → FAILED
                    → CANCELLED
```

### Worker Fault Tolerance

```go
func (w *EventJobWorker) Work(ctx context.Context, job *river.Job[EventJobArgs]) error {
    event, err := w.eventRepo.Get(ctx, job.Args.EventID)
    if errors.Is(err, ErrNotFound) {
        // Event deleted, cancel job (no retry)
        return river.JobCancel(fmt.Errorf("event not found: %s", job.Args.EventID))
    }
    // Other errors: return error for retry
    return w.dispatcher.Dispatch(event)
}
```

### Soft Archiving

```go
// DomainEvent schema
field.Time("archived_at").Optional().Nillable(),
index.Fields("archived_at"),

// Daily archive job (River Periodic Job)
func archiveOldEvents(ctx context.Context, client *ent.Client) error {
    threshold := time.Now().AddDate(0, 0, -30)
    return client.DomainEvent.Update().
        Where(
            domainevent.StatusIn("COMPLETED", "FAILED", "CANCELLED"), // ADR-0009
            domainevent.CreatedAtLT(threshold),
            domainevent.ArchivedAtIsNil(),
        ).
        SetArchivedAt(time.Now()).
        Exec(ctx)
}
```

---

## 4. Approval Workflow

### Directory Structure

```
internal/governance/
├── approval/         # Approval gateway
│   ├── gateway.go
│   └── handler.go
├── audit/            # Audit logging
│   └── logger.go
└── river/            # River worker config
    └── worker_config.go
```

### Status Flow

> **Ticket Status** (ApprovalTicket table):
>
> ```
>                 ┌─────────► REJECTED (terminal)
>                 │
> PENDING_APPROVAL─┼─────────► CANCELLED (terminal, user cancels)
>                 │
>                 └─────────► APPROVED ──► EXECUTING ──► SUCCESS (terminal)
>                                                    └─► FAILED (terminal)
> ```
>
> Note: APPROVED triggers River Job insertion (ADR-0006/0012).
> EXECUTING state is set when River worker picks up the job.

> **Event Status** (DomainEvent table):
>
> ```
> PENDING ──► PROCESSING ──► COMPLETED   # Per ADR-0009
>                        └─► FAILED
>         └─► CANCELLED                  # If ticket rejected/cancelled
> ```

> ⚠️ **Status Terminology Alignment**:
>
> | Context | Initial Status | Description |
> |---------|---------------|-------------|
> | ApprovalTicket | `PENDING_APPROVAL` | Awaiting admin review |
> | DomainEvent (requires approval) | `PENDING` | Event created, ticket pending |
> | DomainEvent (auto-approved) | `PROCESSING` | Skipped PENDING, directly queued |

### Approval Types

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) §7**: Added power operation types with environment-aware policies.

| Type | test Environment | prod Environment | Notes |
|------|------------------|------------------|-------|
| CREATE_SYSTEM | No | No | Record only |
| CREATE_SERVICE | No | No | Record only |
| CREATE_VM | **Yes** | **Yes** | Resource consumption |
| MODIFY_VM | **Yes** | **Yes** | Config change |
| DELETE_VM | **Yes** | **Yes** | Tiered confirmation (ADR-0015 §13.1) |
| START_VM | ❌ No | **Yes** | Power operation |
| STOP_VM | ❌ No | **Yes** | Power operation |
| RESTART_VM | ❌ No | **Yes** | Power operation |
| VNC_ACCESS | ❌ No | **Yes** (temporary grant) | VNC Console (ADR-0015 §18) |

### Admin Modification

> **Security Constraints (ADR-0017)**:
> - Admin **CAN** modify: `template_version`, `cluster_id`, `storage_class`, resource parameters (CPU, Memory, etc.)
> - Admin **CANNOT** modify: `namespace`, `service_id` (immutable after submission - prevents permission escalation)

```go
// ApprovalTicket fields
field.JSON("modified_spec", &ModifiedSpec{}),
field.String("modification_reason"),

// GetEffectiveSpec returns final config
func GetEffectiveSpec(ticket *ApprovalTicket) (*VMSpec, error) {
    if ticket.ModifiedSpec != nil {
        // Full replacement, not merge
        // NOTE: Namespace is NOT included in ModifiedSpec (immutable)
        return applyModifications(ticket.Payload, ticket.ModifiedSpec)
    }
    return parsePayload(ticket.Payload)
}
```

### Safety Protection

| Check | Action |
|-------|--------|
| ≥5 top-level fields deleted | Log warning |
| Required field deleted | Reject with error |
| **Namespace modification attempted** | **Reject with error (ADR-0017)** |
| Preview before save | `POST /api/v1/admin/approvals/:id/preview` |

---

## 5. Template Engine (ADR-0007, ADR-0011, ADR-0018)

> **Simplified per ADR-0018**: Template no longer contains Go Template variables or YAML template files. Templates define only OS image source and cloud-init configuration.

### Template Scope (After ADR-0018)

| In Scope | Description |
|----------|-------------|
| OS image source | DataVolume, ContainerDisk, PVC reference |
| Cloud-init YAML | SSH keys, one-time password, network config |
| Field visibility | `quick_fields`, `advanced_fields` for UI |
| ❌ ~~Go Template variables~~ | **REMOVED** - Too complex, error-prone |
| ❌ ~~RequiredFeatures/Hardware~~ | **MOVED** to InstanceSize per ADR-0018 |

### Template Lifecycle

```
draft → active → deprecated → archived
```

| Status | Meaning |
|--------|---------|
| draft | Under development |
| active | Available for VM creation |
| deprecated | No new VMs, existing VMs OK |
| archived | Hidden from all UIs |

> ⚠️ **ADR-0007 Constraint**: Only **one active template per name** is allowed.
> Creating a new version automatically deprecates the previous active version.

### Template Validation (Before Save)

> **Updated per ADR-0018**: Removed Go Template syntax check.

1. ~~Go Template syntax check~~ → **REMOVED**
2. Cloud-init YAML syntax validation
3. K8s Server-Side Dry-Run validation

### SSA Apply (ADR-0011)

```go
type SSAApplier struct {
    client client.Client
}

func (a *SSAApplier) ApplyYAML(ctx context.Context, yaml []byte) error {
    obj := &unstructured.Unstructured{}
    _ = yamlutil.Unmarshal(yaml, obj)
    
    return a.client.Patch(ctx, obj, client.Apply, 
        client.FieldOwner("kubevirt-shepherd"),
        client.ForceOwnership,
    )
}

func (a *SSAApplier) DryRunApply(ctx context.Context, yaml []byte) error {
    // Same but with DryRunAll option
}
```

---

## 6. Environment Isolation

> **Updated by [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) §1, §15**: System is decoupled from environment. Environment is determined by Cluster and Namespace.

### Environment Source (ADR-0015 §15 Clarification)

| Entity | Environment Field | Set By | Example Names |
|--------|-------------------|--------|---------------|
| **Cluster** | `environment` (test/prod) | Admin | cluster-01, cluster-02 |
| **Namespace** | `environment` (test/prod) | Admin at creation | dev, test, uat, stg, prod01, shop-prod |
| **System** | ❌ **Removed** | - | System is a logical grouping, not infrastructure-bound |

> **Key Point**: Namespace name can be anything (dev, test, uat, shop-prod, etc.), but its environment **type** is one of: `test` or `prod`.

```go
// ent/schema/cluster.go
field.Enum("environment").Values("test", "prod"),

// ent/schema/namespace_registry.go (Platform maintains namespace registry)
// Updated by ADR-0017: Removed cluster_id - Namespace is a global logical entity
field.String("name").NotEmpty().Unique(),      // Globally unique in Shepherd
field.Enum("environment").Values("test", "prod"),  // Explicit, set by admin
field.String("description").Optional(),
// ❌ NO cluster_id - Namespace can be deployed to multiple clusters of matching environment
// Cluster selection happens at VM approval time (ADR-0017)
```

> **ADR-0017 Clarification**: Namespace is a Shepherd-managed logical entity, NOT bound to any single K8s cluster. When a VM is approved, the admin selects the target cluster. If the namespace doesn't exist on that cluster, Shepherd creates it JIT (Just-In-Time).

### Visibility Rules (via Platform RBAC)

Environment access is controlled by `RoleBinding.allowed_environments` (ADR-0015 §22):

| User RoleBinding | Allowed Environments | Can See |
|------------------|---------------------|--------|
| `allowed_environments: ["test"]` | test only | test namespaces |
| `allowed_environments: ["test", "prod"]` | test + prod | all namespaces |
| PlatformAdmin | all | all |

### Scheduling Strategy

```
User with test permission → sees test namespaces → VMs scheduled to test clusters
User with prod permission → sees test+prod namespaces → VMs scheduled to matching cluster type
```

```go
func (s *ApprovalService) Approve(ctx context.Context, ticketID string) error {
    ticket := s.getTicket(ticketID)
    namespace := ticket.Namespace  // From VM creation request
    cluster := s.getSelectedCluster(ticket)
    
    // Environment is determined by namespace/cluster, not by System
    if GetNamespaceEnvironment(namespace) != cluster.Environment {
        return ErrEnvironmentMismatch{
            NamespaceEnv: GetNamespaceEnvironment(namespace),
            ClusterEnv:   cluster.Environment,
        }
    }
    // Continue approval...
}
```

---

## 6.1 Delete Confirmation Mechanism (ADR-0015 §13.1)

> **Tiered confirmation to prevent accidental deletions.**

| Entity | Environment | Confirmation Method |
|--------|-------------|---------------------|
| VM | test | `confirm=true` query parameter |
| VM | prod | Type VM name in request body |
| Service | all | `confirm=true` query parameter |
| System | all | Type system name in request body |

```bash
# Test VM Delete - simple confirm parameter
DELETE /api/v1/vms/{id}?confirm=true

# Prod VM Delete - requires typing VM name
DELETE /api/v1/vms/{id}
Content-Type: application/json
{
  "confirm_name": "prod-shop-redis-01"  // Must match VM name exactly
}
```

---

## 6.2 VNC Console Permissions (ADR-0015 §18)

> **Low priority for V1**. VNC is a convenience feature; enterprises should use bastion hosts for production.

| Environment | VNC Access | Approval Required |
|-------------|------------|-------------------|
| test | ✅ Allowed | ❌ No |
| prod | ✅ Allowed | ✅ Yes (temporary grant) |

**Production VNC Flow**:
1. User requests VNC access to prod VM
2. Request creates approval ticket (`VNC_ACCESS_REQUESTED`)
3. Admin approves with time limit (e.g., 2 hours)
4. User gets temporary VNC token (single-use, user-bound)
5. Token expires after time limit
6. All VNC sessions are audit logged

**VNC Token Security** (ADR-0015 §18):
- **Single Use**: Token invalidated after first connection
- **Time-Bounded**: Max TTL: 2 hours
- **User Binding**: Token includes hashed user ID
- **Encryption**: AES-256-GCM (shared key management with cluster credentials)

---

## 7. Audit Logging

> 📋 **Decision reference**: [ADR-0015 §6](../../adr/ADR-0015-governance-model-v2.md#6-comprehensive-operation-audit-trail), [ADR-0019 §3](../../adr/ADR-0019-governance-security-baseline-controls.md#3-audit-logging-and-sensitive-data-controls)

### Design Principles

- **Append-only**: No modify, no delete
- **Complete**: Record all operations (success and failure)
- **Traceable**: Link to TicketID
- **Secure**: Sensitive data MUST be redacted (ADR-0019)

### Sensitive Data Redaction (ADR-0019)

> **Security Baseline**: Audit logs MUST NOT contain plaintext sensitive data.

| Data Category | Redaction Rule | Example |
|---------------|----------------|---------|
| **Passwords** | Replace with `[REDACTED]` | `password: [REDACTED]` |
| **Tokens/Secrets** | Replace with `[REDACTED]` | `api_key: [REDACTED]` |
| **Personal Identifiers** | Hash or partial mask | `ssn: ***-**-1234` |
| **Kubernetes Credentials** | Never log | `kubeconfig: [NOT_LOGGED]` |

```go
// internal/governance/audit/redactor.go
var sensitiveFields = []string{
    "password", "secret", "token", "credential", 
    "kubeconfig", "private_key", "api_key",
}

func RedactSensitiveData(params map[string]interface{}) map[string]interface{} {
    redacted := make(map[string]interface{})
    for k, v := range params {
        if containsSensitiveField(k) {
            redacted[k] = "[REDACTED]"
        } else {
            redacted[k] = v
        }
    }
    return redacted
}
```

### ActionCodes

| Category | Examples |
|----------|----------|
| Submission | REQUEST_SUBMITTED, REQUEST_CANCELLED |
| Approval | APPROVAL_APPROVED, APPROVAL_REJECTED |
| Execution | EXECUTION_STARTED, EXECUTION_COMPLETED, EXECUTION_FAILED |

### Storage Schema

```sql
-- Full DDL for audit_logs table (migrated from master-flow.md)
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Operation info
    action          VARCHAR(50) NOT NULL,    -- action type
    actor_id        VARCHAR(50) NOT NULL,    -- actor user ID
    actor_name      VARCHAR(100),            -- display name (redundant for query)

    -- Resource info
    resource_type   VARCHAR(50) NOT NULL,    -- system, service, vm, approval, template, etc.
    resource_id     VARCHAR(50) NOT NULL,    -- resource ID
    resource_name   VARCHAR(100),            -- resource name (redundant for query)

    -- Context
    parent_type     VARCHAR(50),             -- parent resource type
    parent_id       VARCHAR(50),             -- parent resource ID
    environment     VARCHAR(20),             -- test, prod

    -- Details (MUST be redacted before storage per ADR-0019)
    details         JSONB,                   -- details (before/after, reason, etc.)
    ip_address      INET,                    -- actor IP
    user_agent      TEXT,                    -- client info

    -- Time
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common query patterns
CREATE INDEX idx_audit_actor ON audit_logs (actor_id, created_at DESC);
CREATE INDEX idx_audit_resource ON audit_logs (resource_type, resource_id, created_at DESC);
CREATE INDEX idx_audit_action ON audit_logs (action, created_at DESC);
CREATE INDEX idx_audit_time ON audit_logs (created_at DESC);
```

### Retention Policy

| Environment | Min Retention | Reason |
|-------------|---------------|--------|
| **Production** | ≥ 1 year | Compliance |
| **Test** | ≥ 90 days | Configurable shorter |
| **Sensitive ops** | ≥ 3 years | `*.delete`, `approval.*`, `rbac.*` |

### JSON Export API {#7-json-export-api}

> **Scenario**: Integrate audit logs into enterprise SIEM (Elasticsearch, Datadog, Splunk)

```
GET /api/v1/admin/audit-logs/export
Content-Type: application/json

Query Parameters:
  - start_time: ISO 8601 start time
  - end_time: ISO 8601 end time
  - action: action filter (optional)
  - actor_id: actor filter (optional)
  - page: page number
  - per_page: page size (max 1000)
```

**Response Format**:

```json
{
  "logs": [
    {
      "@timestamp": "2026-01-26T10:14:16Z",
      "event_id": "log-001",
      "action": "vm.create",
      "level": "INFO",
      "actor": {
        "id": "user-001",
        "name": "Zhang San",
        "ip_address": "192.168.1.100"
      },
      "resource": {
        "type": "vm",
        "id": "vm-001",
        "name": "prod-shop-redis-01"
      },
      "context": {
        "environment": "prod",
        "cluster": "prod-cluster-01",
        "correlation_id": "req-xxx-yyy"
      },
      "details": {
        "instance_size": "medium-gpu",
        "template": "centos7-docker"
      }
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 100,
    "total": 1500
  }
}
```

### Webhook Push Integration

```json
POST /api/v1/admin/audit-logs/webhook
{
  "name": "datadog-integration",
  "url": "https://http-intake.logs.datadoghq.com/v1/input/API_KEY",
  "method": "POST",
  "headers": {
    "DD-API-KEY": "${DATADOG_API_KEY}"
  },
  "filters": {
    "actions": ["*.delete", "approval.*"],
    "environments": ["prod"]
  },
  "batch_size": 100,
  "flush_interval_seconds": 60
}
```

### Best Practices

| Practice | Description |
|----------|-------------|
| **Structured logs** | Always JSON for search/analysis |
| **Consistent field names** | Unified naming (snake_case) |
| **Correlation ID** | Include `correlation_id` for tracing |
| **Redaction** | Redact PII and sensitive data (ADR-0019) |
| **Shallow nesting** | 2-3 levels max for query performance |

---

## 8. Reconciler

| Mode | Behavior |
|------|----------|
| dry-run | Report only, no changes |
| mark | Mark ghost/orphan resources |
| delete | Actually delete (not implemented) |

### Circuit Breaker

If >50% of resources detected as ghosts, halt and alert.

---

## Acceptance Criteria

- [ ] Atlas migrations work
- [ ] River Jobs process correctly
- [ ] Approval workflow functional (including power ops)
- [ ] Event status updates correctly
- [ ] Template lifecycle works
- [ ] Audit logs complete
- [ ] Environment isolation enforced (via Cluster + RoleBinding.allowed_environments)
- [ ] Delete confirmation mechanism works (tiered by entity/environment)
- [ ] VNC token security enforced (single-use, time-bounded)

---

## Related Documentation

- [examples/domain/event.go](../examples/domain/event.go) - Event pattern
- [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) - Atomic TX
- [ADR-0006](../../adr/ADR-0006-unified-async-model.md) - Unified Async
- [ADR-0007](../../adr/ADR-0007-template-storage.md) - Template Storage
- [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md) - Domain Event
- [ADR-0011](../../adr/ADR-0011-ssa-apply-strategy.md) - SSA Apply
- [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) - Hybrid Transaction (Ent + sqlc) with CI enforcement
- [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) - Governance Model V2
- [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md) - Go Module Vanity Import
- [ADR-0017](../../adr/ADR-0017-vm-request-flow-clarification.md) - VM Request Flow
- [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) - Instance Size Abstraction
- [ADR-0019](../../adr/ADR-0019-governance-security-baseline-controls.md) - Governance Security Baseline
- [ADR-0020](../../adr/ADR-0020-frontend-technology-stack.md) - Frontend Technology Stack (separate repo)

---

## ADR-0015 Section Coverage Index

> The following ADR-0015 decisions are implemented in this phase:

| ADR-0015 Section | Covered In | Notes |
|------------------|------------|-------|
| §7 Approval Policies | Section 4 | Environment-aware policy matrix |
| §8 Storage Class | Section 6.0.1 | Per-cluster default SC |
| §10 Cancellation | Section 6.1 | Delete confirmation |
| §11 Approval Timeout | ⚠️ **Pending** | Worker-side timeout or cron |
| §13 Delete Cascade | Section 6.1 | Hierarchical delete |
| §18 VNC Permissions | Section 6.2 | Token-based access |
| §19 Batch Operations | ⚠️ **Pending** | Bulk approval/power ops |
| §20 Notification System | ⚠️ **Pending** | In-app + email alerts |
| §22 Authentication (IdP) | ⚠️ **Out of Scope V1** | See ADR-0015 §21 |

> **Pending items** will be addressed in future iterations. See ADR-0015 for full specification.

---

## ADR-0012 CI Enforcement

> **sqlc Usage Whitelist** (per [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md)):

| Directory | Allowed | Reason |
|-----------|---------|--------|
| `internal/repository/sqlc/` | ✅ Yes | sqlc query definitions |
| `internal/usecase/` | ✅ Yes | Core atomic transactions |
| All other directories | ❌ No | Must use Ent ORM |

```bash
# CI validation: scripts/check-sqlc-usage.sh
# Fails build if sqlc imported outside whitelist
```

