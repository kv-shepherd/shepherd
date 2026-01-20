# ADR-0010: Transaction Integration Strategy

> **Status**: Superseded  
> **Date**: 2026-01-15  
> **Superseded by**: [ADR-0012](./ADR-0012-hybrid-transaction.md)  
> **Related**: ADR-0006, ADR-0009

---

## Supersession Notice

This ADR adopted the "Eventual Consistency + Orphan Event Scanner" approach due to type incompatibility between Ent's `*ent.Tx` and River's `pgx.Tx`.

**This approach has been deprecated**. ADR-0012 discovered that the **Ent + sqlc hybrid mode** can achieve true ACID atomicity, making the compensation mechanism (OrphanEventScanner) unnecessary.

**Do not reference the two-phase commit code examples in this document.**

---

## Original Problem

| Component | Transaction Type | Issue |
|-----------|-----------------|-------|
| **Ent ORM** | `*ent.Tx` (wraps `*sql.Tx`) | Uses `database/sql` interface |
| **River Queue (riverpgxv5)** | `pgx.Tx` | Uses pgx native interface |

**Core conflict**: `pgx.Tx` and `*sql.Tx` are different types, cannot directly cooperate in the same transaction.

---

## Original Decision (Deprecated)

### Option B: Eventual Consistency (Two-Phase with Compensation)

```
┌─────────────────────────────────────────────────────────────────────┐
│                   Eventual Consistency Model                         │
│                                                                      │
│  Phase 1: Ent Transaction (atomic)                                   │
│  ├── Create DomainEvent (status = PENDING)                          │
│  ├── Business data write (quota, audit log)                         │
│  └── Commit                                                          │
│                                                                      │
│  Phase 2: River Enqueue (independent)                                │
│  ├── Insert River Job                                                │
│  └── If fails → Log warning, don't return error                     │
│                                                                      │
│  Compensation: Orphan Event Scanner                                  │
│  ├── Periodically scan status=PENDING events older than 5min        │
│  ├── Check if corresponding River Job exists                        │
│  └── Re-enqueue if missing                                           │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Why Superseded

ADR-0012 discovered a better approach:

1. **sqlc** can generate type-safe Go code from SQL
2. **sqlc** natively supports `pgx/v5`
3. **sqlc** generated code provides `WithTx` method compatible with `pgx.Tx`
4. **River's** `InsertTx(ctx, tx, args)` can directly use `pgx.Tx`

This enables **single atomic transaction** for DomainEvent INSERT and River Job INSERT, eliminating:
- The 5-minute inconsistency window
- The OrphanEventScanner compensation mechanism
- Additional operational complexity

---

## Consequences of Original Approach

### Positive (Historical)

- ✅ Simplified transaction management code
- ✅ Avoided complex driver adaptation
- ✅ River upgrade safe
- ✅ DomainEvent status provides full audit trail

### Negative (Why Superseded)

- 🔴 Up to 5-minute inconsistency window
- 🔴 Required additional compensation component (OrphanEventScanner)
- 🔴 Additional operational complexity
- 🔴 Not true ACID compliance

---

## References

- [ADR-0006: Unified Async Model](./ADR-0006-unified-async-model.md)
- [ADR-0009: Domain Event Pattern](./ADR-0009-domain-event-pattern.md)
- [ADR-0012: Hybrid Transaction Strategy](./ADR-0012-hybrid-transaction.md) (replacement)
