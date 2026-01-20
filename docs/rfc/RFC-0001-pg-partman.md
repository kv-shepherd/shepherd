# RFC-0001: PostgreSQL Table Partitioning (pg_partman)

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: ADR-0008  
> **Trigger**: Daily job volume > 10 million (10M)

---

## Problem

When River Job Cleaner's DELETE operations produce dead tuples faster than Autovacuum can process, table bloat occurs causing performance degradation.

---

## Current State

**Not implementing**

| Factor | Analysis |
|--------|----------|
| **Expected load** | Governance platform ~thousands jobs/day, far below threshold |
| **Complexity** | pg_partman requires extension install, Cron config, Schema migration |
| **River compatibility risk** | River internals depend on specific index structures |
| **Current approach** | River built-in Job Cleaner + aggressive Autovacuum tuning is sufficient |

---

## Trigger Conditions

Evaluate implementation when:

- **Daily job volume exceeds 10 million (10M)**
- River Job Cleaner's DELETE produces dead tuples faster than Autovacuum can handle
- `river_dead_tuple_ratio` metric persistently > 30%

---

## Implementation Path

When trigger conditions are met:

### 1. Install Extensions

```sql
CREATE EXTENSION pg_partman;
CREATE EXTENSION pg_cron;
```

### 2. Configure Table Partitioning

```sql
-- Daily partitions, 7-day retention, auto-cleanup
SELECT partman.create_parent(
    p_parent_table => 'public.river_job',
    p_control => 'created_at',
    p_type => 'native',
    p_interval => 'daily',
    p_premake => 7
);

-- Set retention policy
UPDATE partman.part_config
SET retention = '7 days',
    retention_keep_table = false,
    infinite_time_partitions = true
WHERE parent_table = 'public.river_job';
```

### 3. Configure Scheduled Maintenance

```sql
-- Hourly partition maintenance
SELECT cron.schedule('river_partman_maintenance', '0 * * * *', 
    $$CALL partman.run_maintenance_proc()$$);
```

---

## Why Partitioning > DELETE

| Operation | DELETE + VACUUM | DROP PARTITION |
|-----------|-----------------|----------------|
| Dead tuples | Produces many | **Zero** |
| Locks | May block | Instant |
| WAL logs | Many | Few |
| Execution time | Minutes | **Milliseconds** |

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| River upgrade compatibility | Test partition table behavior before upgrade |
| Schema migration complexity | Execute migration during off-peak hours |
| Operations knowledge requirements | Provide documentation and training |

---

## References

- [pg_partman Documentation](https://github.com/pgpartman/pg_partman)
- [PostgreSQL Table Partitioning](https://www.postgresql.org/docs/current/ddl-partitioning.html)
- [ADR-0008: PostgreSQL Stability](../adr/ADR-0008-postgresql-stability.md)
