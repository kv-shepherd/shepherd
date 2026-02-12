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

> **📖 Document Hierarchy (Prevents Content Drift)**:
>
> | Document | Authority | Scope |
> |----------|-----------|-------|
> | **ADRs** | Decisions (immutable after acceptance) | Architecture decisions and rationale |
> | **[master-flow.md](../interaction-flows/master-flow.md)** | Interaction principles (single source of truth) | Data sources, flow rationale, user journeys |
> | **[database/README.md](../database/README.md)** | Database reference layer | Schema domains, lifecycle/retention, transaction boundaries |
> | **Phase docs (this file)** | Implementation details | Code patterns, schemas, API design |
> | **[CHECKLIST.md](../CHECKLIST.md)** | ADR constraints reference | Centralized ADR enforcement rules |
>
> **Cross-Reference Pattern**: When describing "what data" and "why", link to master-flow. This document defines "how to implement".
>
> **ADR Constraints**: For critical ADR enforcement rules (ADR-0006, ADR-0009, ADR-0012, etc.), see [CHECKLIST.md §Core ADR Constraints](../CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## V1 Scope Boundaries (ADR-0015 §21)

> **Reference**: [ADR-0015 §21 Scope Exclusions](../../adr/ADR-0015-governance-model-v2.md)

The following features are **explicitly out of scope** for V1:

| Feature | V1 Status | Future Path |
|---------|-----------|-------------|
| Resource Quota Management | ❌ Not in V1 | May add in future RFC |
| User-defined Business Tags | ❌ Not in V1 | If added, stored in DB not K8s |
| Full Multi-tenancy | ❌ Not in V1 | Schema reserved (`tenant_id = "default"`) |
| Complex Approval Workflows | ❌ Not in V1 | See RFC-0002 for Temporal integration |
| Approval Timeout Auto-processing | ❌ Not in V1 | UI prioritization used instead |
| Automatic Page Refresh via WebSocket | ❌ Not in V1 | Manual refresh in V1 |
| External Approval System Integration | ⚠️ Interface Only | Standard interface; adapters in V2+ |

**Implementation Guidance**:
- If a feature request touches any item above, redirect to future RFC
- Do not implement features beyond this document's scope
- `tenant_id` is always `"default"` in V1 code

> ⚠️ **Approval Engine V1 Constraint (ADR-0005)**:
>
> V1 approval **decision outcomes** are limited to: `PENDING_APPROVAL → APPROVED` or `PENDING_APPROVAL → REJECTED`.
> Ticket lifecycle may still include out-of-band `CANCELLED` (user action) and execution tracking states.
> DO NOT design for:
> - Multi-level approval chains (L1 → L2 → L3)
> - Withdraw/Countersign/Transfer operations
> - Timeout auto-processing (use UI prioritization instead)
>
> This is intentional to keep the approval engine simple and maintainable. See [01-contracts.md §Footnote 1](01-contracts.md#footnotes).

## Deliverables

> **Last Updated**: 2026-02-11

| Deliverable | File Path | Status | Notes |
|-------------|-----------|--------|-------|
| Atlas config | `atlas.hcl` | ⬜ | Deferred (requires running DB) |
| River Jobs | `internal/jobs/vm_create.go`, `vm_delete.go`, `vm_power.go` | ✅ | VMCreate/Delete/Power workers with retry + idempotency guard + audit |
| EventDispatcher | `internal/domain/dispatcher.go` | ✅ | - |
| Domain Event Payloads | `internal/domain/event.go` | ✅ | VMCreationPayload, VMDeletePayload, VMPowerPayload |
| ApprovalGateway | `internal/governance/approval/gateway.go` | ✅ | Approve/Reject/Cancel/ListPending + ADR-0012 atomic writer integration |
| ApprovalValidator | `internal/service/approval_validator.go` | ✅ | Cluster health + overcommit + dedicated CPU conflict + capability matching (GPU/SR-IOV/Hugepages) |
| AuditLogger | `internal/governance/audit/logger.go` | ✅ | LogAction + LogVMOperation |
| TemplateService | `internal/service/template_service.go` | ✅ | - |
| VM Handlers | `internal/api/handlers/server_vm.go` | ✅ | CRUD + Delete + Power ops + tiered confirm params (test/prod) |
| System Handlers | `internal/api/handlers/server_system.go` | ✅ | CRUD + GetService/DeleteService + confirm_name query param via generated params |
| Approval Handlers | `internal/api/handlers/server_approval.go` | ✅ | ListPending/Approve/Reject/Cancel + DELETE ticket target VM enrichment |
| Namespace Handlers | `internal/api/handlers/server_namespace.go` | ✅ | CRUD (List/Create/Get/Update/Delete) with environment filter + confirm_name delete gate |
| Notification Handlers | `internal/api/handlers/server_notification.go` | ✅ | List/UnreadCount/MarkRead/MarkAllRead; triggers/sender integrated; retention cleanup scheduled |
| Admin Handlers | `internal/api/handlers/server_admin.go` | ✅ | Clusters/Templates/InstanceSizes CRUD + UpdateClusterEnvironment (omitzero adapted) |
| SSAApplier | `internal/provider/ssa_applier.go` | ⬜ | Deferred |
| OpenAPI Spec | `api/openapi.yaml` | ✅ | 38 endpoints total; Namespace CRUD + Notification + omitzero value types (ADR-0028) |

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

> **ADR-0005 Phase Extension**: ADR-0005 defines the **approval decision flow** (`PENDING → APPROVED/REJECTED`).
> This section extends it with **execution tracking phases** (`APPROVED → EXECUTING → SUCCESS/FAILED`) to support River Queue integration and provide complete ticket lifecycle visibility.
>
> ⚠️ **V1 Scope Clarification (ADR-0005)**:
> - The **approval engine** in V1 supports only `PENDING → APPROVED/REJECTED` transitions
> - User-initiated `CANCELLED` is an **out-of-band** action (user cancels their own request)
> - `CANCELLED` is NOT part of the approval workflow logic; it bypasses the approval engine
> - Multi-level approvals, countersign, and timeout auto-processing are **out of V1 scope**

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

### Approval List UI Prioritization (ADR-0015 §11)

> **V1 Strategy**: No automatic timeout or auto-cancellation. UI-based visual prioritization guides admin attention to aging requests.

| Days Pending | Visual Treatment | Sort Priority | User Action |
|--------------|------------------|---------------|-------------|
| 0-3 days | Normal | Standard | Wait or cancel |
| 4-7 days | 🟡 Yellow highlight | Higher | Consider follow-up |
| 7+ days | 🔴 Red highlight | Highest (top of list) | User may cancel and resubmit |

**Frontend Implementation**:

```typescript
// Approval list sorting: oldest first within each priority tier
const sortApprovals = (tickets: ApprovalTicket[]) => {
  return tickets.sort((a, b) => {
    const tierA = getPriorityTier(a.created_at);
    const tierB = getPriorityTier(b.created_at);
    if (tierA !== tierB) return tierB - tierA; // Higher tier first
    return a.created_at - b.created_at;        // Older first within tier
  });
};

const getPriorityTier = (createdAt: Date): number => {
  const days = daysSince(createdAt);
  if (days > 7) return 3;  // Red - highest priority
  if (days > 3) return 2;  // Yellow - higher priority
  return 1;                // Normal - standard
};
```

**API Response**:

```json
{
  "tickets": [
    {
      "id": "ticket-001",
      "status": "PENDING_APPROVAL",
      "created_at": "2026-01-25T10:00:00Z",
      "days_pending": 9,
      "priority_tier": "urgent"
    }
  ]
}
```

> **User Self-Cancellation**: Users can cancel their own pending requests at any time via `POST /api/v1/approvals/{id}/cancel`. This is independent of timeout - users may cancel to resubmit with different parameters.

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

> **Storage Decision (ADR-0007)**: All templates and system templates are stored in **PostgreSQL database**.
> **No Git dependency** is required for template management. The Git library approach (original ADR-0002) has been **superseded** and fully removed.
>
> | Aspect | Decision | ADR Reference |
> |--------|----------|---------------|
> | **Storage** | PostgreSQL only | ADR-0007 |
> | **Version control** | Database-level versioning (draft → active → deprecated → archived) | ADR-0007 |
> | **Git library** | ❌ **Not used** - original ADR-0002 superseded | ADR-0002 → ADR-0007 |

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

> **Version Requirement**: `controller-runtime v0.22.4+` required for `client.DryRunAll` support.
> See [DEPENDENCIES.md §Core Dependencies](../DEPENDENCIES.md#core-dependencies) for version matrix.

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

### Dry-Run Validation Flow (ADR-0018)

> **Purpose**: Validate VM creation request against target cluster BEFORE approval, ensuring request is valid and can be executed.

#### When Dry-Run is Performed

| Stage | Trigger | Target Cluster |
|-------|---------|----------------|
| VM Request Submission | User submits VM creation | Preview cluster (admin-configured) |
| Template Save | Admin saves template | Test cluster |
| Approval Phase | Admin assigns target cluster | Actual target cluster |

#### API Endpoint

```
POST /api/v1/vms/validate
Content-Type: application/json

{
  "instance_size": "medium-gpu",
  "template_name": "centos7-docker",
  "namespace": "prod-shop",
  "cluster_id": "cluster-01"  // Optional: specific cluster, otherwise uses preview cluster
}

Response (200 OK):
{
  "valid": true,
  "warnings": ["GPU quota is at 80%"],
  "estimated_resources": {
    "cpu": "4",
    "memory": "8Gi",
    "gpu": "1"
  }
}

Response (422 Unprocessable Entity):
{
  "valid": false,
  "code": "VALIDATION_FAILED",
  "errors": [
    {
      "field": "spec.template.spec.domain.devices.gpus",
      "message": "GPU allocation failed: insufficient GPU resources",
      "k8s_reason": "Forbidden"
    }
  ]
}
```

#### Implementation

```go
// internal/provider/validator.go

type VMValidator struct {
    applier  *SSAApplier
    clusters ClusterProvider
}

// ValidateVMSpec performs dry-run validation against target K8s cluster
func (v *VMValidator) ValidateVMSpec(ctx context.Context, req *ValidateVMRequest) (*ValidationResult, error) {
    // 1. Resolve target cluster (preview or specified)
    cluster, err := v.resolveTargetCluster(ctx, req.ClusterID)
    if err != nil {
        return nil, err
    }
    
    // 2. Generate VM manifest from InstanceSize + Template
    manifest, err := v.generateVMManifest(ctx, req)
    if err != nil {
        return &ValidationResult{
            Valid:  false,
            Errors: []ValidationError{{Message: err.Error()}},
        }, nil
    }
    
    // 3. Perform K8s Dry-Run Apply
    err = v.applier.DryRunApply(ctx, manifest)
    if err != nil {
        return v.parseK8sError(err), nil
    }
    
    // 4. Check resource availability (optional quota check)
    warnings := v.checkResourceWarnings(ctx, cluster, manifest)
    
    return &ValidationResult{
        Valid:    true,
        Warnings: warnings,
    }, nil
}

// DryRunApply performs SSA with DryRunAll option
func (a *SSAApplier) DryRunApply(ctx context.Context, yaml []byte) error {
    obj := &unstructured.Unstructured{}
    if err := yamlutil.Unmarshal(yaml, obj); err != nil {
        return fmt.Errorf("invalid YAML: %w", err)
    }
    
    return a.client.Patch(ctx, obj, client.Apply,
        client.FieldOwner("kubevirt-shepherd"),
        client.ForceOwnership,
        client.DryRunAll,  // Key: DryRunAll option
    )
}
```

#### Graceful Degradation

If dry-run fails due to cluster unreachable:

| Scenario | Behavior |
|----------|----------|
| Preview cluster unreachable | Allow submission with warning, re-validate at approval |
| Target cluster unreachable at approval | Block approval, require cluster recovery |
| Dry-run timeout (>10s) | Allow submission with warning |

---

## 5.5 Cluster StorageClass Management (ADR-0015 §8)

> **Reference**: [ADR-0015 §8](../../adr/ADR-0015-governance-model-v2.md)

### Design Overview

StorageClass management ensures VMs use appropriate storage for their workload. The platform auto-detects available StorageClasses during cluster health checks and allows admin override during approval.

### Schema Extensions

```go
// ent/schema/cluster.go - additional fields
field.Strings("storage_classes").Optional().
    Comment("Auto-detected StorageClass list from cluster"),
field.String("default_storage_class").Optional().
    Comment("Admin-specified default StorageClass"),
field.Time("storage_classes_updated_at").Optional().
    Comment("Last StorageClass detection timestamp"),
```

### Detection Flow (Health Check Integration)

```
Health Check (60s interval)
    ├── API Server connectivity check
    ├── KubeVirt CRD check
    ├── Capability detection (ADR-0014)
    └── StorageClass detection
        ├── List StorageClasses from cluster
        ├── Update clusters.storage_classes
        └── Set storage_classes_updated_at = now()
```

### Implementation

> **Code Example**: See [`examples/provider/storage_detector.go`](../examples/provider/storage_detector.go)

### Admin API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `GET /api/v1/admin/clusters/{id}/storage-classes` | GET | List cluster's available storage classes |
| `PUT /api/v1/admin/clusters/{id}/storage-classes/default` | PUT | Set default storage class for cluster |

### Approval Workflow Integration

During approval, admin can select a specific StorageClass (or use cluster default):

```go
// ApprovalTicket additional field for storage class selection
field.String("selected_storage_class").Optional().
    Comment("Admin-selected StorageClass during approval, empty uses cluster default"),
```

**Approval Flow**:

```
Admin approves request
    ├── Select target cluster
    ├── [Optional] Select StorageClass from cluster.storage_classes
    │   └── If not specified → use cluster.default_storage_class
    ├── Validate StorageClass exists on cluster
    └── Proceed with VM creation
```

### Validation Rules

| Check | Enforcement |
|-------|------------|
| Selected SC must exist | Validate against `cluster.storage_classes` before approval |
| Default SC must be set | Warn if cluster has no `default_storage_class` |
| SC detection staleness | Warn if `storage_classes_updated_at` > 24 hours |

---

## 5.6 Batch Operations (ADR-0015 §19)

> **Reference**: [ADR-0015 §19](../../adr/ADR-0015-governance-model-v2.md)

### Design Goals

Batch operations MUST follow ADR-0015 §19 as the normative model:

- Parent-child ticket persistence
- Atomic parent + child ticket creation
- Independent child execution via River Jobs
- Two-layer rate limiting (global + user-level)
- Frontend-visible aggregate and per-child status

### Supported Operations and API Surface

| Operation | Canonical Endpoint | Notes |
|-----------|--------------------|-------|
| Batch VM create/delete | `POST /api/v1/vms/batch` | Creates parent + child tickets atomically |
| Batch status query | `GET /api/v1/vms/batch/{id}` | Parent summary + child states |
| Batch retry failed | `POST /api/v1/vms/batch/{id}/retry` | Requeue failed child items only |
| Batch terminate pending | `POST /api/v1/vms/batch/{id}/cancel` | Cancel not-yet-started children |
| Batch approval compatibility | `POST /api/v1/approvals/batch` | Supported; normalized into canonical parent-child pipeline |
| Batch power compatibility | `POST /api/v1/vms/batch/power` | Supported; normalized into canonical parent-child pipeline |

### Parent-Child Data Model

| Entity | Key Fields |
|--------|------------|
| `batch_approval_tickets` (parent) | `ticket_id`, `batch_type`, `child_count`, `success_count`, `failed_count`, `pending_count`, `status`, `created_by` |
| `approval_tickets` (child) | `ticket_id`, `parent_ticket_id`, `sequence_no`, `status`, `attempt_count`, `error_message`, `last_attempt_at` |

### Atomicity and Execution Boundary

| Phase | Guarantee | Implementation |
|------|-----------|----------------|
| Submission (parent + all children) | ✅ Atomic | Single DB transaction, rollback on any insert failure |
| Execution (child jobs) | ❌ Non-atomic by design | Each child runs independently in River |
| Parent status | ✅ Deterministic aggregation | Computed from aggregate counters (`success/failed/pending`) |

### Two-Layer Rate Limiting (ADR-0015 §19)

| Layer | Limit | Default |
|------|-------|---------|
| Global | Max pending parent tickets | `100` |
| Global | Max batch API requests | `1000 req/min` |
| User | Max pending parent batch requests per user | `3` |
| User | Cooldown between submissions | `2 minutes` |
| User | Max pending child tickets per user | `30` |

Admin override APIs (ADR-0015):

- `POST /api/v1/admin/rate-limits/exemptions`
- `DELETE /api/v1/admin/rate-limits/exemptions/{user_id}`
- `PUT /api/v1/admin/rate-limits/users/{user_id}`
- `GET /api/v1/admin/rate-limits/status`

### Response Contract (Submission)

`POST /api/v1/vms/batch` returns `202 Accepted` with tracking metadata:

```json
{
  "batch_id": "BAT-20260206-001",
  "status": "PENDING_APPROVAL",
  "status_url": "/api/v1/vms/batch/BAT-20260206-001",
  "retry_after_seconds": 2
}
```

### Frontend Contract (Mandatory)

Frontend implementation MUST follow:

- [frontend/features/batch-operations-queue.md](../frontend/features/batch-operations-queue.md)
- [master-flow.md Stage 5.E](../interaction-flows/master-flow.md#stage-5e-batch-operations)

Required frontend behavior:

- Parent row + child detail visualization
- Polling by `status_url` until terminal parent status
- `Retry failed` and `Terminate pending` actions with explicit affected-item feedback
- `429` handling with `Retry-After` countdown

### Constraints

- Max batch size per operation type follows ADR-0015 defaults
- Item-level validation is required before child insertion
- Duplicate submit with same idempotency key returns existing parent ticket
- Partial success is a first-class outcome (`PARTIAL_SUCCESS`)

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

> ⚠️ **CRITICAL IMPLEMENTATION WARNING**: 
> - **DO NOT** add `cluster_id` to `namespace_registry` schema
> - Namespace ↔ Cluster binding occurs at **VM approval time**, not schema level
> - Failure to follow this pattern will break multi-cluster namespace sharing
> - See [ADR-0017 §Namespace Just-In-Time Creation](../../adr/ADR-0017-vm-request-flow-clarification.md#namespace-just-in-time-creation-added-2026-01-27) for complete rationale

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

## 6.1 Delete Cascade and Confirmation Mechanism (ADR-0015 §13, §13.1)

> **Primary resources use hard delete** (System/Service/VM) after cascade checks pass.
> `audit_logs` and `domain_events` are retained for traceability for all delete flows.
> `approval_tickets` are retained only for operations that require approval (for example, VM create/delete and production VNC requests), and archived per retention policy.

### Cascade Constraints (Hard Delete)

| Entity | Deletion Constraint | Data Retention |
|--------|---------------------|----------------|
| System | Must have zero Services | Hard delete system row; keep audit/event records (no delete approval ticket) |
| Service | Must have zero VMs | Hard delete service row; keep audit/event records (no delete approval ticket) |
| VM | Direct delete allowed | Hard delete VM row; keep audit/event records and VM-related approval tickets |

### Confirmation Rules (Tiered)

> **Tiered confirmation prevents accidental irreversible deletion.**

| Entity | Environment | Confirmation Method |
|--------|-------------|---------------------|
| VM | test | `confirm=true` query parameter |
| VM | prod | Type VM name in request body |
| Service | all | `confirm=true` query parameter |
| System | all | Type system name in request body |

```bash
# Test VM Delete - simple confirm parameter
DELETE /api/v1/vms/{id}?confirm=true

# Prod VM Delete - requires typing VM name (query param per ADR-0015 §13 addendum)
DELETE /api/v1/vms/{id}?confirm_name=prod-shop-redis-01

# Service Delete - confirm=true required
DELETE /api/v1/systems/{sys_id}/services/{svc_id}?confirm=true

# System Delete - confirm_name required
DELETE /api/v1/systems/{sys_id}?confirm_name=my-system
```

### ✅ Implementation Progress (audited 2026-02-10T23:59)

| Item | Status | Details |
|------|--------|---------|
| DeleteVM handler | ✅ Done | State guard + DomainEvent + River job + audit |
| DeleteVM confirm mechanism | ✅ Done | Tiered: `confirm=true` (test) or `confirm_name` matching VM name (prod) |
| DeleteSystem handler | ✅ Done | Cascade check (child Service count == 0) + hard delete + audit |
| DeleteSystem confirm param | ✅ Done | Uses `confirm_name` query param via generated params (ADR-0015 §13 addendum) |
| DeleteService handler | ✅ Done | Cascade check (child VM count == 0) + `confirm=true` gate + hard delete + audit |
| GetService handler | ✅ Done | Verifies service belongs to system + returns service detail |
| OpenAPI: DeleteVM params | ✅ Done | Added `confirm` and `confirm_name` query params |
| OpenAPI: DeleteService | ✅ Done | Endpoint defined with `confirm` query param |
| OpenAPI: DeleteSystem params | ✅ Done | Added `confirm_name` query param |
| ApprovalTicket.operation_type | ✅ Done | Enum field (`CREATE`/`DELETE`) with `CREATE` default |
| VM delete approval ticket flow | ✅ Done | DeleteVM use case creates `operation_type=DELETE` ticket and routes through approval gateway |

> **Remaining**: Batch/VNC are still out of current implementation scope.

---

## 6.2 VNC Console Permissions (ADR-0015 §18, RFC-0011)

> **V1 Status**: Simplified implementation (see RFC-0011 for details).
>
> **Full Reference**: [Master Flow Stage 6](../interaction-flows/master-flow.md#stage-6-vnc-console-access)

| Environment | VNC Access | Approval Required | Token TTL |
|-------------|------------|-------------------|-----------|
| test | ✅ Allowed | ❌ No (RBAC only) | 2 hours |
| prod | ✅ Allowed | ✅ Yes | 2 hours |

### V1 Implementation Scope

| Feature | V1 (Simplified) | Full (V2+) |
|---------|-----------------|------------|
| Token tracking | Signed JWT + shared replay marker (`jti`, `used_at`) | Full token lifecycle table + policy controls |
| Token revocation | No active revoke API (TTL + single-use only) | Active revocation API |
| Session recording | ❌ Not supported | ✅ Optional |

> **Traceability note**: V1 boundary is now formalized by
> [ADR-0015 §18.1 Addendum](../../adr/ADR-0015-governance-model-v2.md#adr-0015-vnc-v1-addendum).
> The full revocation-capable model in ADR-0015 §18 remains the V2+ target architecture.

### Production VNC Flow

1. User requests VNC access to prod VM
2. Request creates approval ticket (`VNC_ACCESS_REQUESTED`)
3. Admin approves with time limit (default: 2 hours)
4. User gets temporary VNC token (single-use, user-bound)
5. Token expires after time limit
6. All VNC sessions are audit logged

### VNC Token Security (ADR-0015 §18)

| Security Feature | Requirement | V1 Implementation |
|------------------|-------------|-------------------|
| **Single Use** | Token invalidated after first connection | JWT `jti` replay marker (`used_at`) in shared store (PostgreSQL recommended) |
| **Time-Bounded** | Max TTL: 2 hours (configurable) | JWT `exp` claim |
| **User Binding** | Token includes user ID | JWT `sub` claim |
| **Encryption** | AES-256-GCM | Shared key management |
| **Audit Logged** | All sessions recorded | `vnc.access` event (see [master-flow.md §Canonical Action Naming](../interaction-flows/master-flow.md#canonical-action-naming)) |

> **V1 Note**: No active revoke endpoint. Security relies on short TTL + single-use replay protection.
> Do not introduce Redis dependency for token tracking.

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `POST /api/v1/vms/{vm_id}/console/request` | POST | Request VNC access (creates approval ticket in prod) |
| `GET /api/v1/vms/{vm_id}/console/status` | GET | Check access status (for polling) |
| `GET /api/v1/vms/{vm_id}/vnc?token={jwt}` | WS | WebSocket VNC connection (noVNC) |

---

## 6.3 Notification System (ADR-0015 §20)

> **Reference**: [ADR-0015 §20](../../adr/ADR-0015-governance-model-v2.md)

### V1 Design: Platform Inbox

V1 implements a minimal internal notification system. External push channels (email, webhook) are deferred to V2+.

> **Write Strategy Clarification (ADR-0006 Compliance)**:
>
> Notification writes are **synchronous** (within the same database transaction as business operations), NOT via River Queue.
>
> | Aspect | Notification | VM/Approval Operations |
> |--------|--------------|------------------------|
> | **Write mode** | Synchronous (same TX) | Async (River Queue) |
> | **Why?** | Pure DB write, no external API | Requires K8s API calls |
> | **ADR-0006 scope** | ❌ Not in scope | ✅ In scope |
> | **Failure handling** | Rolls back with business TX | River retry mechanism |
>
> **Rationale**: ADR-0006's "all writes via River Queue" applies to operations requiring external system calls (K8s API). Notification inserts are local PostgreSQL writes with predictable latency, benefiting from transactional atomicity with business data.
>
> **V2+ External Channels**: When email/webhook/Slack adapters are added, those external pushes will use River Queue for retry resilience. See [RFC-0018 §River Queue Integration](../../rfc/RFC-0018-external-notification.md#river-queue-integration) for planned architecture.

### Data Model

> **Code Example**: See [`examples/notification/sender.go`](../examples/notification/sender.go) for full schema and interface definitions

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `GET /api/v1/notifications` | GET | List user's notifications (paginated) |
| `GET /api/v1/notifications/unread-count` | GET | Get unread count for badge display |
| `PATCH /api/v1/notifications/{notification_id}/read` | PATCH | Mark notification as read |
| `POST /api/v1/notifications/mark-all-read` | POST | Mark all notifications as read |

### Decoupled Interface (V2+ Ready)

The notification system uses a decoupled `NotificationSender` interface:

- **V1**: `InboxSender` (stores to PostgreSQL) — ✅ Implemented (`internal/notification/sender.go`)
- **V2+**: Add `EmailSender`, `WebhookSender`, `SlackSender` via plugin

### Trigger Points

| Event | Notification Type | Recipients | Status |
|-------|-------------------|------------|--------|
| VM request submitted | `APPROVAL_PENDING` | Approvers (users with `approval:approve` permission) | ✅ Implemented |
| Request approved | `APPROVAL_COMPLETED` | Requester | ✅ Implemented |
| Request rejected | `APPROVAL_REJECTED` | Requester | ✅ Implemented |
| VM power state changed | `VM_STATUS_CHANGE` | VM owner | ✅ Implemented |

> **Implementation Details** (2026-02-11):
>
> - **Triggers**: `internal/notification/triggers.go` — `OnTicketSubmitted`, `OnTicketApproved`, `OnTicketRejected`, `OnVMStatusChanged`
> - **Integration**: `ApprovalGateway.SetNotifier()` calls triggers on approve/reject; `CreateVMRequest`/`DeleteVM` handlers call `OnTicketSubmitted`
> - **DI Wiring**: `ApprovalModule` wires `InboxSender → Triggers → Gateway.SetNotifier`
> - **Frontend**: `NotificationBell` component in `web/src/components/ui/NotificationBell.tsx` (badge + popover + mark-read)

### V1 Constraints

- **No external push**: Email/webhook adapters in V2+
- **Poll-based**: Frontend polls unread count every 30s (via TanStack Query `refetchInterval`)
- **Retention**: Auto-cleanup after 90 days (via River periodic job) — ✅ implemented (`internal/jobs/notification_cleanup.go`)

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

> **Canonical Naming**: Use dot-notation `{domain}.{action}` per [master-flow.md §Canonical Action Naming](../interaction-flows/master-flow.md#canonical-action-naming).

| Domain | Canonical Actions (V1) | Notes |
|--------|------------------------|-------|
| Auth | `user.login`, `user.login_failed`, `user.logout` | Authentication events |
| System | `system.create`, `system.update`, `system.delete_submitted`, `system.delete_executed` | No delete approval ticket |
| Service | `service.create`, `service.delete_submitted`, `service.delete_executed` | No delete approval ticket |
| VM | `vm.request`, `vm.create`, `vm.start`, `vm.stop`, `vm.restart`, `vm.delete_submitted`, `vm.delete_approved`, `vm.delete_executed` | Delete requires approval |
| VNC | `vnc.access` | Sensitive read |
| Approval | `approval.approve`, `approval.reject`, `approval.cancel` | Ticket decisions |
| RBAC | `role.create`, `role.update`, `role.delete`, `role.assign`, `role.revoke` | Permission governance |
| Cluster | `cluster.register`, `cluster.update`, `cluster.delete`, `cluster.credential_rotate` | Cluster lifecycle |
| Template | `template.create`, `template.update`, `template.deprecate`, `template.delete` | Template lifecycle |
| InstanceSize | `instance_size.create`, `instance_size.update`, `instance_size.deprecate`, `instance_size.delete` | Sizing lifecycle |

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

## 8. IdP Authentication (V1 Scope)

> **Reference**: [master-flow.md Stage 2.B/2.C/2.D](../interaction-flows/master-flow.md#stage-2-b)

### 8.1 Supported Authentication Methods

| Method | V1 Status | Use Case |
|--------|-----------|----------|
| **OIDC** | ✅ Implemented | Modern SSO (Azure AD, Okta, Keycloak) |
| **LDAP** | ✅ Implemented | Legacy Active Directory |
| **Built-in Users** | ✅ Implemented | Development/testing, bootstrap admin |

### 8.2 OIDC Token Validation Checklist

> **Security Requirement**: All ID Tokens MUST be validated per [OIDC Core Spec](https://openid.net/specs/openid-connect-core-1_0.html).

| Validation Step | Required | Implementation |
|-----------------|----------|----------------|
| **Signature verification** | ✅ Mandatory | Verify against IdP JWKS endpoint public keys |
| **`alg` algorithm whitelist** | ✅ Mandatory | Only accept RS256, ES256; reject "none" |
| **`iss` (issuer) match** | ✅ Mandatory | Must exactly match configured IdP issuer URL |
| **`aud` (audience) match** | ✅ Mandatory | Must contain application's `client_id` |
| **`exp` (expiration) check** | ✅ Mandatory | Current time < exp (allow 30s clock skew) |
| **`nonce` validation** | ✅ Mandatory | Must match nonce sent in auth request |
| **`iat` (issued at) freshness** | ⚠️ Recommended | Reject tokens older than 1 hour |

```go
// internal/auth/oidc/validator.go
type TokenValidator struct {
    jwksCache    *jwk.Cache
    issuer       string
    clientID     string
    allowedAlgs  []string // ["RS256", "ES256"]
    clockSkew    time.Duration
}

func (v *TokenValidator) Validate(ctx context.Context, rawToken string) (*Claims, error) {
    // 1. Parse and verify signature
    token, err := jwt.ParseSigned(rawToken)
    if err != nil {
        return nil, ErrInvalidToken
    }
    
    // 2. Get public key from JWKS cache
    keySet, err := v.jwksCache.Get(ctx, v.issuer+"/.well-known/jwks.json")
    if err != nil {
        return nil, ErrJWKSFetchFailed
    }
    
    // 3. Verify signature and extract claims
    var claims Claims
    if err := token.Claims(keySet, &claims); err != nil {
        return nil, ErrSignatureInvalid
    }
    
    // 4. Validate required claims
    if claims.Issuer != v.issuer {
        return nil, ErrIssuerMismatch
    }
    if !claims.Audience.Contains(v.clientID) {
        return nil, ErrAudienceMismatch
    }
    if time.Now().After(claims.Expiry.Time().Add(v.clockSkew)) {
        return nil, ErrTokenExpired
    }
    
    return &claims, nil
}
```

### 8.3 IdP Data Model

> **Reference**: [01-contracts.md §3 Core Ent Schemas](./01-contracts.md#3-core-ent-schemas) for full schema.

| Table | Purpose |
|-------|---------|
| `auth_providers` | OIDC/LDAP provider configuration |
| `idp_synced_groups` | Groups discovered from IdP |
| `idp_group_mappings` | IdP group → Shepherd role mapping |

### 8.4 User Login Flow

See [master-flow.md Stage 2.D](../interaction-flows/master-flow.md#stage-2-d) for complete flow diagram.

Key operations:
1. Validate OIDC/LDAP credentials
2. Extract user groups from token/LDAP
3. Delete old IdP-assigned RoleBindings (`source = 'idp_mapping'`)
4. Recreate RoleBindings based on current group mappings
5. Return session JWT

---

## 9. External Approval Systems (V1 Interface Only)

> **V1 Scope**: Interface and schema defined. Full implementation in V2.

### 9.1 Interface Definition

```go
// internal/governance/approval/external.go

// ExternalApprovalProvider defines the contract for external approval systems
type ExternalApprovalProvider interface {
    // SubmitForApproval sends a request to external system
    SubmitForApproval(ctx context.Context, ticket *ApprovalTicket) (externalID string, err error)
    
    // CheckStatus polls external system for decision
    CheckStatus(ctx context.Context, externalID string) (ExternalDecision, error)
    
    // CancelRequest cancels pending external request
    CancelRequest(ctx context.Context, externalID string) error
}

type ExternalDecision struct {
    Status    string    // "pending", "approved", "rejected"
    Approver  string    // External approver ID
    Comment   string    // Approval/rejection reason
    Timestamp time.Time
}
```

### 9.2 Schema (V1 - Defined but not fully implemented)

```go
// ent/schema/external_approval_system.go
field.String("id"),
field.String("name"),
field.Enum("type").Values("webhook", "servicenow", "jira"),
field.Bool("enabled"),
field.String("webhook_url").Optional(),
field.String("webhook_secret").Optional().Sensitive(), // Encrypted
field.JSON("webhook_headers", map[string]string{}),
field.Int("timeout_seconds").Default(30),
field.Int("retry_count").Default(3),
```

### 9.3 V2 Roadmap

| Feature | V2 Target |
|---------|-----------|
| Webhook integration | Full bidirectional webhook |
| ServiceNow connector | Native ServiceNow API |
| JIRA connector | JIRA issue-based approval |
| Callback handling | Async approval notification |

---

## 10. Resource-Level RBAC

> **Reference**: [master-flow.md Stage 4.A+](../interaction-flows/master-flow.md#stage-4-a-plus)

### 10.1 Resource Role Binding

| Role | Permissions |
|------|-------------|
| **owner** | Full control, can transfer ownership |
| **admin** | Manage members, create/delete child resources |
| **member** | Create child resources, view all |
| **viewer** | Read-only access |

### 10.2 Inheritance Model

```
System (shop)           ← Members configured here
  ├── Service (redis)   ← Inherits from System
  │     ├── VM-01       ← Inherits from Service → System
  │     └── VM-02       ← Inherits from Service → System
  └── Service (mysql)   ← Inherits from System
        └── VM-03       ← Inherits from Service → System
```

### 10.3 Permission Check Algorithm

```go
func (s *AuthzService) CheckResourceAccess(ctx context.Context, userID, resourceType, resourceID string) (Role, error) {
    // 1. Check global admin
    if s.hasGlobalPermission(ctx, userID, "platform:admin") {
        return RoleOwner, nil // Super admin sees everything
    }
    
    // 2. Traverse inheritance chain
    resource := s.getResource(resourceType, resourceID)
    for resource != nil {
        binding, err := s.repo.GetResourceRoleBinding(ctx, userID, resource.Type, resource.ID)
        if err == nil && binding != nil {
            return binding.Role, nil
        }
        resource = resource.Parent() // VM → Service → System → nil
    }
    
    return RoleNone, ErrAccessDenied // Resource not visible to user
}
```

### 10.4 Member Management API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `GET /api/v1/systems/{id}/members` | GET | List system members |
| `POST /api/v1/systems/{id}/members` | POST | Add member |
| `PATCH /api/v1/systems/{id}/members/{userId}` | PATCH | Update member role |
| `DELETE /api/v1/systems/{id}/members/{userId}` | DELETE | Remove member |

---

## 11. VM Deletion Workflow

> **Reference**: [master-flow.md Stage 5.D](../interaction-flows/master-flow.md#stage-5-d)

### 11.1 Deletion Confirmation (Tiered)

| Entity | Environment | Confirmation Required |
|--------|-------------|----------------------|
| VM | test | `?confirm=true` query param |
| VM | prod | `?confirm_name=<vm-name>` query param |
| Service | all | `?confirm=true` query param |
| System | all | `?confirm_name=<system-name>` query param |

### 11.2 Deletion API

```
DELETE /api/v1/vms/{id}?confirm=true           # Test environment
DELETE /api/v1/vms/{id}?confirm_name=prod-shop-redis-01   # Prod environment
```

### 11.3 Deletion Flow

1. **Validate confirmation** - Tier-appropriate confirmation
2. **Check permissions** - User must have `vm:delete` + resource access
3. **Create approval ticket** (default policy: both test/prod for `DELETE_VM`, ADR-0015 §7)
4. **On approval**:
   - Mark VM as `DELETING` in database
   - Enqueue River job for K8s deletion
   - River worker deletes VirtualMachine CR
   - Keep VM in `DELETING` tombstone state after K8s deletion (cleanup can be handled by periodic maintenance)
5. **Audit log** - Record deletion with actor, reason, timestamp

---

## 12. Reconciler

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
- [ ] **IdP Authentication** (V1):
  - [ ] OIDC login flow works (token validation per checklist)
  - [ ] LDAP login flow works
  - [ ] IdP group → role mapping synchronized on login
- [ ] **Resource-level RBAC**:
  - [ ] Member management API functional
  - [ ] Permission inheritance chain correct
- [ ] **VM Deletion**:
  - [ ] Tiered confirmation enforced
  - [ ] Audit log recorded

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
- [ADR-0020](../../adr/ADR-0020-frontend-technology-stack.md) - Frontend Technology Stack
- [ADR-0027](../../adr/ADR-0027-repository-structure-monorepo.md) - Repository Structure (monorepo with `web/`)
- [ADR-0030](../../adr/ADR-0030-design-documentation-layering-and-fullstack-governance.md) - Frontend doc layering and full-stack governance
- [frontend/features/batch-operations-queue.md](../frontend/features/batch-operations-queue.md) - Parent-child queue UI spec

---

## ADR-0015 Section Coverage Index

> The following table provides a **complete mapping** of all ADR-0015 decisions to their implementation locations.

| ADR-0015 Section | Status | Covered In | Notes |
|------------------|--------|------------|-------|
| §1 System Entity Decoupling | ✅ Done | [01-contracts.md §3.1](01-contracts.md#31-system-schema) | No namespace/environment/cluster bindings |
| §2 Service Entity & Permission Inheritance | ✅ Done | [01-contracts.md §3.2](01-contracts.md#32-service-schema) | Runtime inheritance from System |
| §3 VM Entity Association | ✅ Done | [01-contracts.md §3](01-contracts.md#3-core-ent-schemas) | VM → Service only (no direct system_id) |
| §4 VM Field Control | ✅ Done | [01-contracts.md §3.4](01-contracts.md#34-approvalticket-admin-fields-adr-0017) | User-forbidden fields; amended by ADR-0017 |
| §5 Template Layered Design | ✅ Done | [master-flow.md Stage 1](../interaction-flows/master-flow.md#stage-1) | Amended by ADR-0018 (capability → InstanceSize) |
| §6 Audit Trail | ✅ Done | Section 7 (this doc) | DomainEvent pattern; redaction per ADR-0019 |
| §7 Approval Policies | ✅ Done | Section 4 (this doc) | Environment-aware policy matrix |
| §8 Storage Class | ✅ Done | Section 5.5 (this doc) | Auto-detection, admin default, approval override |
| §9 Namespace Responsibility | ✅ Done | Section 6 (this doc) + [01-contracts.md §1](01-contracts.md#1-governance-model-hierarchy) | Platform does NOT manage K8s RBAC/Quota |
| §10 Cancellation | ✅ Done | Section 4 (this doc) | User can cancel pending requests |
| §11 Approval Timeout | ✅ V1 UI | Section 4 (this doc) | Days pending sort + color warning; no auto-cancel |
| §12 Resource Adoption | ✅ V1 Minimal | [02-providers.md §7](02-providers.md#7-resource-adoption-v1-minimal-compensation) | Compensation capability only; no complex reconciliation |
| §13 Delete Cascade | ✅ Done | Section 6.1 (this doc) | Hierarchical hard delete with constraints; audit/events retained, approval tickets retained where applicable |
| §13.1 Delete Confirmation | ✅ Done | [master-flow.md Stage 5.D](../interaction-flows/master-flow.md#stage-5-d) | Tiered confirmation (test vs prod) |
| §14 Platform RBAC | ✅ Done | Section 3 (this doc) | Dual-layer RBAC; ADR-0019 amendments |
| §15 Cluster Visibility | ✅ Done | Section 5.5 (this doc) | Environment matching; scheduling weight |
| §16 Global Naming | ✅ Done | [01-contracts.md §1.1](01-contracts.md#11-naming-constraints-adr-0019) | RFC 1035 + ADR-0019 extension |
| §17 Template Snapshot | ✅ Done | [master-flow.md Stage 5.B](../interaction-flows/master-flow.md#stage-5-b) | ApprovalTicket stores immutable snapshot |
| §18 VNC Permissions | ✅ Done | Section 6.2 (this doc) | Token-based access |
| §19 Batch Operations | ✅ Done | Section 5.6 (this doc) | Parent-child ticket model + two-layer rate limiting + frontend queue contract |
| §20 Notification System | ✅ V1 Inbox | Section 6.3 (this doc) | Sync writes; external adapters V2+ |
| §21 Scope Exclusions | 📋 Reference | ADR-0015 | Lists deferred items |
| §22 Authentication | ✅ V1 Scope | Section 8 (this doc) | OIDC + LDAP; group mapping |
| External Approval Systems | ⚠️ V1 Interface | - | Standard data interface; plugin layer |

> **Legend**: ✅ Done = Implemented in V1 | ⚠️ Partial = Implemented subset, ADR gap remains | ⚠️ V1 Interface = Only data interface defined

> **Interface-First Design**: Notification and Approval systems use **standard data interfaces** (ADR-0015 §20, §9).
> V1 implements simple built-in solutions. External integrations (Slack, ServiceNow, Jira) are handled by plugin adapters without core interface changes.

---

## ADR-0012 CI Enforcement

> **sqlc Usage Whitelist** (per [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md)):

| Directory | Allowed | Reason |
|-----------|---------|--------|
| `internal/repository/sqlc/` | ✅ Yes | sqlc query definitions |
| `internal/usecase/` | ✅ Yes | Core atomic transactions |
| All other directories | ❌ No | Must use Ent ORM |

```bash
# CI validation: check_sqlc_usage.sh
# Fails build if sqlc imported outside whitelist
```
