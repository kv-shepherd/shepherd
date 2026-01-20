# RFC-0009: PgBouncer Dual Connection Pool

> **Status**: Deferred  
> **Priority**: P3  
> **Source**: ADR-0012  
> **Trigger**: Enterprise deployment with high connection requirements

---

## Problem

V1.0 uses direct pgxpool connection to PostgreSQL. In enterprise scenarios with many application instances, connection pooling at the proxy level may be required.

---

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    PgBouncer Architecture                            │
│                                                                      │
│  App Instances (N)                                                  │
│       │                                                             │
│       ▼                                                             │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                       PgBouncer                              │   │
│  │  ├── Transaction Pool (for Ent)                              │   │
│  │  └── Session Pool (for River + Advisory Lock)                │   │
│  └──────────────────────────────────────────────────────────────┘   │
│       │                                                             │
│       ▼                                                             │
│  PostgreSQL                                                         │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Configuration

```ini
# pgbouncer.ini
[databases]
kubevirt_transaction = host=postgres port=5432 dbname=kubevirt pool_mode=transaction
kubevirt_session = host=postgres port=5432 dbname=kubevirt pool_mode=session

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 20
```

---

## Trigger Conditions

- More than 50 application instances
- PostgreSQL max_connections limit reached
- Need to reduce connection overhead

---

## Considerations

- Session pool required for Advisory Lock (River leader election)
- Transaction pool sufficient for regular queries
- Application must be aware of dual-pool configuration

---

## References

- [PgBouncer Documentation](https://www.pgbouncer.org/)
- [ADR-0012: Hybrid Transaction](../adr/ADR-0012-hybrid-transaction.md)
