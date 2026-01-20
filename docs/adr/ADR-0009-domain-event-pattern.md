# ADR-0009: Domain Event Pattern

> **Status**: Accepted (Transaction section superseded by ADR-0012)  
> **Date**: 2026-01-15  
> **Note**: DomainEvent schema design and EventID pattern remain valid

---

## Partial Supersession Notice

> ⚠️ **Important**: Only the **transaction strategy** section of this ADR was superseded by ADR-0012.
>
> | Section | Status |
> |---------|--------|
> | Domain Event schema design | ✅ **Still Valid** - Must read |
> | EventID pattern (Claim Check) | ✅ **Still Valid** - Must read |
> | Payload immutability constraints | ✅ **Still Valid** - Must read |
> | Worker fault tolerance | ✅ **Still Valid** - Must read |
> | Transaction approach (eventual consistency) | ❌ **Obsolete** - Use ADR-0012 instead |

---

## Decision

All write operations are driven by **Domain Events**. River Jobs carry only the Event ID:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Domain Event Pattern (Claim Check)                │
│                                                                      │
│  User request → Create DomainEvent → Insert River Job(EventID) → 202│
│                                                                      │
│  River Worker → Read DomainEvent → Execute business logic → Update  │
│                                                                      │
│  Benefits:                                                           │
│  - Job table only stores EventID (~50 bytes), no bloat              │
│  - Event table managed independently, supports archiving            │
│  - Business changes only modify event structure, Job structure stable│
│  - Event and audit log unified, easy to trace                       │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Key Design Constraints

### Constraint 1: DomainEvent Payload Immutability (Append-Only)

> **Business semantic**: **"What you see is what you sign"** - approval passes the snapshot at submission time.

| Rule | Description |
|------|-------------|
| **Payload immutable** | Once created, `DomainEvent.payload` field **cannot be modified** |
| **Admin changes via ApprovalTicket** | Admin modifications stored in `ApprovalTicket.modified_spec` |
| **Full replacement strategy** | Worker checks `modified_spec`, uses full replacement if present |

```go
// ❌ Forbidden: Directly modifying DomainEvent.payload
event.Payload["cpu"] = 8  // FORBIDDEN

// ✅ Correct: Modifications stored in ApprovalTicket.ModifiedSpec (full config)
ticket.ModifiedSpec = map[string]interface{}{
    "resources": {"cpu": 4, "memory": "4Gi"},  // Full configuration
    "image": "ubuntu:22.04",
}

// ✅ Worker reads with full replacement (no merging)
func GetEffectiveSpec(event *ent.DomainEvent, ticket *ent.ApprovalTicket) map[string]interface{} {
    if ticket != nil && len(ticket.ModifiedSpec) > 0 {
        return ticket.ModifiedSpec  // Full replacement, no merge
    }
    return event.Payload
}
```

> **Enforcement via Ent Hook** (Recommended):
>
> ```go
> // ent/schema/domain_event.go - Add Hook to enforce immutability
> 
> func (DomainEvent) Hooks() []ent.Hook {
>     return []ent.Hook{
>         // Prevent payload modification after creation
>         hook.On(func(next ent.Mutator) ent.Mutator {
>             return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
>                 if dm, ok := m.(*DomainEventMutation); ok {
>                     if _, exists := dm.Payload(); exists {
>                         return nil, errors.New("payload field is immutable after creation")
>                     }
>                 }
>                 return next.Mutate(ctx, m)
>             })
>         }, ent.OpUpdate|ent.OpUpdateOne),
>     }
> }
> ```
>
> This Hook will cause a runtime error if any code attempts to update the `payload` field.

> **Why abandon `mergeSpec`**:
> - Shallow merge: `{"resources": {"cpu": 4}}` overwrites `resources` key, `memory` lost
> - Deep merge: Logic complex, hard to express "delete field" intent
> - Recursion risk: Deep nesting makes code hard to maintain

### Constraint 2: Worker Fault Tolerance

```go
func (w *EventWorker) Work(ctx context.Context, job *river.Job[EventJobArgs]) error {
    event, err := w.client.DomainEvent.Query().
        Where(domainevent.EventIDEQ(job.Args.EventID)).
        Only(ctx)
    
    if err != nil {
        if ent.IsNotFound(err) {
            // Event not found: Cancel job (no retry)
            w.log.Warn("Event not found, cancelling job",
                zap.String("event_id", job.Args.EventID))
            return river.JobCancel(fmt.Errorf("event not found: %s", job.Args.EventID))
        }
        // Other errors (network issues): Return error for River retry
        return err
    }
    
    // Continue execution...
}
```

### Constraint 3: Terminology Clarification (Not Event Sourcing)

> **Important**: This pattern is **Claim Check Pattern**, **not** Event Sourcing.

| This Pattern | Event Sourcing |
|--------------|----------------|
| DomainEvent is Entity table | Event Log is immutable ledger |
| Stores current state | Stores incremental changes |
| ID points to single record | ID points to position in Event Stream |
| State can be updated (PENDING→COMPLETED) | State is append-only |

---

## DomainEvent Schema

```go
// ent/schema/domain_event.go

func (DomainEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("event_id").Unique().NotEmpty(),
        field.String("event_type").NotEmpty(),
        field.String("aggregate_type").NotEmpty(),
        field.String("aggregate_id").Optional(),
        field.JSON("payload", map[string]interface{}{}),
        field.String("requested_by").NotEmpty(),
        field.String("tenant_id").NotEmpty(),
        field.Enum("status").
            Values("PENDING", "PROCESSING", "COMPLETED", "FAILED", "CANCELLED").
            Default("PENDING"),
        field.String("error_message").Optional().Nillable(),
        field.Time("processed_at").Optional().Nillable(),
        field.Time("archived_at").Optional().Nillable(),
    }
}
```

---

## Consequences

### Positive

- ✅ River Job table no longer bloats
- ✅ Business logic decoupled from task scheduling
- ✅ Supports event-driven architecture evolution
- ✅ Clearer audit trail

### Negative

- 🟡 Adds one database read (Worker reads event)
- 🟡 Event table requires independent maintenance (archiving, indexing)

---

## References

- [Domain Events Pattern](https://docs.microsoft.com/en-us/dotnet/architecture/microservices/microservice-ddd-cqrs-patterns/domain-events-design-implementation)
- [Claim Check Pattern](https://www.enterpriseintegrationpatterns.com/patterns/messaging/StoreInLibrary.html)
- [ADR-0012: Hybrid Transaction Strategy](./ADR-0012-hybrid-transaction.md)
