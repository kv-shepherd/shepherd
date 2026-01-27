# Code Examples

> This directory contains reference implementations for the KubeVirt Shepherd Go refactor.
>
> These examples serve as blueprints for implementation. Actual code may vary based on specific requirements.

> ⚠️ **DESIGN DOCUMENTS ONLY - NOT COMPILABLE CODE**
>
> These `.go` files are **pseudo-code examples** for documentation purposes. They demonstrate patterns and architecture but **will not compile** because:
>
> 1. Referenced packages (`internal/domain`, `internal/jobs`, etc.) do not exist yet
> 2. Import paths use vanity import (`kv-shepherd.io/shepherd/...`) per [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md)
> 3. Some helper functions are omitted for brevity
>
> **To use these examples**: Copy the patterns into your actual implementation during Phase 0-1.

---

## Directory Structure

```
examples/
├── README.md                   # This index
├── config/
│   └── config.go              # Viper-based config loading
├── infrastructure/
│   └── database.go            # ADR-0012 shared pool setup
├── worker/
│   └── pool.go                # ants-based goroutine pool
├── handlers/
│   └── health.go              # Liveness and readiness probes
├── domain/
│   ├── vm.go                  # VM domain model (Anti-Corruption Layer)
│   └── event.go               # Domain event pattern (ADR-0009)
├── provider/
│   └── interface.go           # Provider interface definitions
└── usecase/
    └── create_vm.go           # ADR-0012 atomic transaction example
```

---

## Example Index

| File | Description | Related ADR |
|------|-------------|-------------|
| [config/config.go](./config/config.go) | Configuration loading with Viper, hot-reload support | - |
| [infrastructure/database.go](./infrastructure/database.go) | Shared pgxpool for Ent + sqlc + River | ADR-0012 |
| [worker/pool.go](./worker/pool.go) | Worker pool with panic recovery | - |
| [handlers/health.go](./handlers/health.go) | Health check endpoints | - |
| [domain/vm.go](./domain/vm.go) | VM domain model (Anti-Corruption Layer) | ADR-0015 §3-4 |
| [domain/event.go](./domain/event.go) | Domain event types (Power Ops, VNC, Batch) | ADR-0009, ADR-0015 §6 |
| [provider/interface.go](./provider/interface.go) | KubeVirt provider interfaces | ADR-0004 |
| [usecase/create_vm.go](./usecase/create_vm.go) | Atomic transaction with pgx + sqlc + River | ADR-0012, ADR-0015 §3 |

---

## Key Patterns

### ADR-0012: Hybrid Atomic Transaction

See [usecase/create_vm.go](./usecase/create_vm.go) - Demonstrates Ent + sqlc + River in single pgx transaction.

**Method Selection Decision Tree:**

```
Does operation require approval?
│
├─ YES → Call Execute()
│        → Creates Event + Ticket atomically
│        → Returns PENDING_APPROVAL
│        │
│        └─ When admin approves → Call ApproveAndEnqueue()
│                                  → Updates status + Inserts River Job atomically
│
└─ NO (auto-approval policy matches) → Call AutoApproveAndEnqueue()
                                        → Creates Event + Ticket + Job atomically
                                        → Returns PROCESSING
```

**Method Summary:**

| Method | Use Case | Atomicity Scope |
|--------|----------|-----------------|
| `Execute()` | Approval-required operations | Event + Ticket (Job inserted after approval) |
| `ApproveAndEnqueue()` | After admin approval | Status update + River Job |
| `AutoApproveAndEnqueue()` | Auto-approval operations | Event + Ticket + Job (all in one) |

```go
// True ACID atomicity (auto-approval or post-approval)
tx, _ := pool.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)

sqlcTx := queries.WithTx(tx)
sqlcTx.CreateDomainEvent(ctx, ...)
riverClient.InsertTx(ctx, tx, jobArgs, nil)

tx.Commit(ctx) // Single atomic commit - all succeed or all fail
```

### ADR-0009: Domain Event Pattern

See [domain/event.go](./domain/event.go) - Claim Check pattern with immutable payloads.

- Payload is **immutable** (append-only)
- Modifications stored in `ApprovalTicket.modified_spec` (full replacement)
- `GetEffectiveSpec()` returns the final config

### ADR-0006: Unified Async Model

All write operations return `202 Accepted` with event ID. Workers execute actual K8s operations.

### ADR-0015: Governance Model V2

**Entity Decoupling**:
- System is decoupled from namespace/environment (§1)
- Service inherits permissions from System (§2)
- VM does not store SystemID directly - resolve via ServiceID → Service.Edges.System (§3)
- User-forbidden fields: Name, Labels, CloudInit are platform-controlled (§4)

**Extended Event Types** (see [domain/event.go](./domain/event.go)):
- Power operations: `VM_START_REQUESTED`, `VM_STOP_REQUESTED`, `VM_RESTART_REQUESTED`
- VNC console: `VNC_ACCESS_REQUESTED`, `VNC_ACCESS_GRANTED`
- Batch operations: `BATCH_CREATE_REQUESTED`, `BATCH_DELETE_REQUESTED`
- Notifications: `NOTIFICATION_SENT`

### Worker Pool (Coding Standard)

Naked `go func()` is **forbidden**. Use [worker/pool.go](./worker/pool.go) pattern.

---

## Usage

These examples are for reference only. To use in your project:

1. Copy relevant files to your `internal/` directory
2. Modify package names and imports
3. Adjust configurations as needed

---

## Related Documentation

- [Project README](../README.md) - Project overview
- [Phase 00: Prerequisites](../phases/00-prerequisites.md)
- [Phase 01: Contracts](../phases/01-contracts.md)
- [Phase 02: Providers](../phases/02-providers.md)
- [Phase 03: Service Layer](../phases/03-service-layer.md)
- [Phase 04: Governance](../phases/04-governance.md)
- [DEPENDENCIES.md](../DEPENDENCIES.md) - Dependency versions
- [ADR Directory](../../adr/) - Architecture decisions
