# Phase 4: Governance Capabilities

> **Prerequisites**: Phase 3 complete  
> **Acceptance**: Approval workflow operational, River Queue processing

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
PENDING → PROCESSING → COMPLETED
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
            domainevent.StatusIn("COMPLETED", "FAILED", "CANCELLED"),
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

```
SUBMITTED → PENDING_APPROVAL → APPROVED → EXECUTING → COMPLETED
                            → REJECTED           → FAILED
```

### Approval Types

| Type | Approval Required | Notes |
|------|-------------------|-------|
| CREATE_SYSTEM | No | Record only |
| CREATE_SERVICE | No | Record only |
| CREATE_VM | **Yes** | Resource consumption |
| MODIFY_VM | **Yes** | Config change |
| DELETE_VM | Configurable | Policy-based |

### Admin Modification

```go
// ApprovalTicket fields
field.JSON("modified_spec", &ModifiedSpec{}),
field.String("modification_reason"),

// GetEffectiveSpec returns final config
func GetEffectiveSpec(ticket *ApprovalTicket) (*VMSpec, error) {
    if ticket.ModifiedSpec != nil {
        // Full replacement, not merge
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
| Preview before save | `POST /api/v1/admin/approvals/:id/preview` |

---

## 5. Template Engine (ADR-0007, ADR-0011)

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

### Template Validation (Before Save)

1. Go Template syntax check
2. Mock data render test
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

### Schema Fields

```go
// Cluster
field.Enum("environment").Values("test", "prod"),

// System
field.Enum("environment").Values("test", "prod"),
```

### Visibility Rules

| User Role | Can See |
|-----------|---------|
| Regular user | test only |
| prod user | test + prod |
| Admin | all |

### Scheduling Constraint

```go
func (s *ApprovalService) Approve(ctx context.Context, ticketID string) error {
    ticket := s.getTicket(ticketID)
    system := s.getSystem(ticket.SystemID)
    cluster := s.getSelectedCluster(ticket)
    
    if system.Environment != cluster.Environment {
        return ErrEnvironmentMismatch{
            SystemEnv:  system.Environment,
            ClusterEnv: cluster.Environment,
        }
    }
    // Continue approval...
}
```

---

## 7. Audit Logging

### Design Principles

- **Append-only**: No modify, no delete
- **Complete**: Record all operations (success and failure)
- **Traceable**: Link to TicketID

### ActionCodes

| Category | Examples |
|----------|----------|
| Submission | REQUEST_SUBMITTED, REQUEST_CANCELLED |
| Approval | APPROVAL_APPROVED, APPROVAL_REJECTED |
| Execution | EXECUTION_STARTED, EXECUTION_COMPLETED, EXECUTION_FAILED |

### Storage

```go
// ent/schema/audit_log.go
field.String("action_code").NotEmpty(),
field.JSON("params", map[string]interface{}{}),
field.String("ticket_id").Optional(),
field.String("performed_by").NotEmpty(),
field.Time("performed_at").Default(time.Now),
```

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
- [ ] Approval workflow functional
- [ ] Event status updates correctly
- [ ] Template lifecycle works
- [ ] Audit logs complete
- [ ] Environment isolation enforced

---

## Related Documentation

- [examples/domain/event.go](../examples/domain/event.go) - Event pattern
- [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) - Atomic TX
- [ADR-0006](../../adr/ADR-0006-unified-async-model.md) - Unified Async
- [ADR-0007](../../adr/ADR-0007-template-storage.md) - Template Storage
- [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md) - Domain Event
- [ADR-0011](../../adr/ADR-0011-ssa-apply-strategy.md) - SSA Apply
