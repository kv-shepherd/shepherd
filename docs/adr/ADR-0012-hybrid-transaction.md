# ADR-0012: Hybrid Transaction Strategy (Ent + sqlc)

> **Status**: Accepted  
> **Date**: 2026-01-16  
> **Supersedes**: [ADR-0010](./ADR-0010-transaction-strategy.md) (Eventual Consistency)  
> **Related**: ADR-0006, ADR-0008

---

## Context

### ADR-0010 Limitations

ADR-0010 adopted "Eventual Consistency + Orphan Event Scanner" because:
- Ent ORM uses `*ent.Tx` (wraps `*sql.Tx`)
- River Queue uses `pgx.Tx`
- Types incompatible, cannot cooperate in same transaction

This resulted in:
1. Compensation mechanism required (OrphanEventScanner)
2. Up to 5-minute inconsistency window
3. Additional operational complexity

### Discovery of Better Approach

**Ent + sqlc hybrid mode** is widely recognized as Go ecosystem best practice:

- **sqlc** generates type-safe Go code from SQL
- **sqlc** natively supports `pgx/v5`
- **sqlc** generated code provides `WithTx` method, seamlessly works with `pgx.Tx`
- **River's** `InsertTx(ctx, tx, args)` directly uses `pgx.Tx`

---

## Decision

### Adopt: Ent + sqlc Hybrid Mode

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   Technology Stack Separation (T0 Architecture)          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Ent ORM (99%)                     sqlc (1%)                            │
│  ├── Schema definition & migration ├── Core transactional writes        │
│  ├── All read operations           ├── DomainEvent INSERT               │
│  ├── Non-core write operations     └── Atomic commit with River InsertTx│
│  └── Hooks, Validation                                                   │
│                                                                          │
│  ──────────────────────────────────────────────────────────────────────  │
│                                                                          │
│                              pgxpool.Pool                                │
│                                   ↓                                      │
│                               pgx.Tx                                     │
│                                   ↓                                      │
│                            Atomic Transaction Commit                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### 0. Connection Pool Reuse (Required)

> ⚠️ **Critical**: Must use `stdlib.OpenDBFromPool` to reuse the same `pgxpool`, otherwise connection count doubles.

```go
// internal/infrastructure/database.go

type DatabaseClients struct {
    PgxPool   *pgxpool.Pool   // Native pgx pool (for River, sqlc)
    EntClient *ent.Client     // Ent ORM (via stdlib wrapper)
}

func NewDatabaseClients(ctx context.Context, connString string) (*DatabaseClients, error) {
    // 1. Create single pgxpool (all clients share)
    config, _ := pgxpool.ParseConfig(connString)
    config.MaxConns = 50
    config.MinConns = 5
    
    pgxPool, _ := pgxpool.NewWithConfig(ctx, config)
    
    // 2. Critical: Wrap pgxpool as *sql.DB (for Ent)
    sqlDB := stdlib.OpenDBFromPool(pgxPool)
    
    // 3. Create Ent Client using wrapped *sql.DB
    drv := entsql.OpenDB(dialect.Postgres, sqlDB)
    entClient := ent.NewClient(ent.Driver(drv))
    
    return &DatabaseClients{
        PgxPool:   pgxPool,      // River, sqlc use directly
        EntClient: entClient,    // Ent uses wrapped connection
    }, nil
}
```

### Atomic Transaction Example

```go
// internal/usecase/create_vm_atomic.go

func (uc *CreateVMAtomicUseCase) Execute(ctx context.Context, input CreateVMInput) (*CreateVMOutput, error) {
    eventID := uuid.New().String()

    // ==========================================
    // Single Atomic Transaction (True ACID)
    // ==========================================
    tx, err := uc.pool.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("begin transaction: %w", err)
    }
    defer tx.Rollback(ctx)

    // 1. Use sqlc to create DomainEvent within transaction
    qtx := uc.queries.WithTx(tx)
    _, err = qtx.CreateDomainEvent(ctx, db.CreateDomainEventParams{
        EventID:       eventID,
        EventType:     domain.EventVMCreationRequested,
        AggregateType: domain.AggregateVM,
        Payload:       payloadJSON,
        Status:        "PENDING",
        RequestedBy:   input.RequestedBy,
        TenantID:      input.TenantID,
    })
    if err != nil {
        return nil, fmt.Errorf("create domain event: %w", err)
    }

    // 2. River InsertTx (same transaction!)
    _, err = uc.riverClient.InsertTx(ctx, tx, jobs.EventJobArgs{
        EventID: eventID,
    }, nil)
    if err != nil {
        return nil, fmt.Errorf("insert river job: %w", err)
    }

    // 3. Atomic commit: all succeed or all fail
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit transaction: %w", err)
    }

    return &CreateVMOutput{EventID: eventID, Status: "PENDING"}, nil
}
```

---

## sqlc Usage Specification (Required)

### Allowed Scope (Whitelist)

| Directory | Allowed Operations |
|-----------|-------------------|
| `internal/repository/sqlc/` | sqlc query definitions |
| `internal/usecase/` | Core atomic transactions (DomainEvent + River) |

### Prohibited Usage

| Scenario | Reason |
|----------|--------|
| Using sqlc for ordinary CRUD | Ent handles this |
| Using sqlc in Service layer | Only UseCase layer controls transactions |
| Using `sql.Open()` to create separate connection pool | Must use shared pgxpool |
| Using `INSERT INTO river_job` directly | Must use River's official API |
| **Mixing Ent and sqlc in the same transaction** | Connection borrowing conflict (see below) |

> ⚠️ **Critical Rule: No Ent+sqlc Mixing Within Same Transaction**
>
> Although Ent and sqlc share the same `pgxpool`, they use different connection borrowing mechanisms:
> - **sqlc**: Uses `pgx.Tx` directly via `pgxpool.BeginTx()`
> - **Ent**: Uses `*sql.Tx` via `stdlib.OpenDBFromPool()` wrapper
>
> **Within a single atomic transaction, use ONLY ONE approach:**
>
> | Pattern | Allowed | Example |
> |---------|---------|---------|
> | ✅ sqlc-only transaction | Yes | `tx := pool.Begin(); sqlcTx := queries.WithTx(tx); sqlcTx.CreateEvent(); river.InsertTx(tx)` |
> | ✅ Ent-only transaction | Yes | `entTx := entClient.Tx(ctx); entTx.VM.Create().Save(ctx)` |
> | ❌ Mixed transaction | **NO** | `tx := pool.Begin(); sqlcTx.Create(); entClient.VM.Create()` ← Different TX contexts |
>
> **Rationale**: The `stdlib.OpenDBFromPool` wrapper borrows connections from the pool in a way
> that may not align with an externally-managed `pgx.Tx`, potentially causing:
> - Operations executing on different connections
> - Transaction isolation violations
> - Deadlocks under load

### CI Enforcement

```bash
# scripts/check-sqlc-usage.sh
# Only allow sqlc usage in whitelisted directories
```

---

## Consequences

### Positive

- ✅ **True ACID**: DomainEvent and River Job in single atomic transaction
- ✅ **Eliminated OrphanEventScanner**: No compensation mechanism needed
- ✅ **Zero inconsistency window**: Either both succeed or both fail
- ✅ **Simplified architecture**: Less operational complexity

### Negative

- 🟡 **Two tools to maintain**: Ent for 99%, sqlc for 1%
- 🟡 **Stricter discipline required**: Must follow sqlc usage specifications
- 🟡 **Learning curve**: Developers need to understand when to use which

### Mitigation

- CI enforcement of sqlc usage scope
- Clear documentation of which layer uses which tool
- Code review for UseCase layer atomic transactions

---

## References

- [sqlc Documentation](https://sqlc.dev/)
- [River Queue InsertTx](https://riverqueue.com/docs/transactions)
- [Ent ORM + pgx Integration](https://entgo.io/docs/sql-integration/)
