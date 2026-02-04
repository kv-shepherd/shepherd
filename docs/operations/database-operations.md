# PostgreSQL Database Operations Guide

> **Reference ADRs**: 
> - [ADR-0003](../adr/ADR-0003-database-orm.md) - Database ORM (Ent)
> - [ADR-0008](../adr/ADR-0008-postgres-only.md) - PostgreSQL Only
> - [ADR-0012](../adr/ADR-0012-hybrid-transaction.md) - Hybrid Transaction

---

## Critical Configuration: Autovacuum Settings

> ⚠️ **PRODUCTION REQUIREMENT (ADR-0008)**: River job queue tables experience high-frequency inserts/updates/deletes.
> Without aggressive autovacuum, tables will bloat and **severely degrade performance**.

### Required SQL Commands

Execute these commands on first deployment and verify after PostgreSQL upgrades:

```sql
-- River job table: aggressive autovacuum (vacuum earlier, at 1% dead tuples instead of 20%)
ALTER TABLE river_job SET (
    autovacuum_vacuum_scale_factor = 0.01,  -- 1% threshold (default: 0.2 = 20%)
    autovacuum_vacuum_threshold = 1000,     -- minimum dead tuples before vacuum
    autovacuum_analyze_scale_factor = 0.01, -- frequent statistics update
    autovacuum_analyze_threshold = 500
);

-- Audit_logs table (high write volume)
ALTER TABLE audit_logs SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 5000
);

-- Domain_events table (append-only but may have soft deletes)
ALTER TABLE domain_events SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 2000
);
```

### Verification Query

Run this query periodically to check table health:

```sql
SELECT 
    relname AS table_name,
    n_dead_tup,
    n_live_tup,
    round(100.0 * n_dead_tup / nullif(n_live_tup + n_dead_tup, 0), 2) as dead_ratio_percent
FROM pg_stat_user_tables
WHERE relname IN ('river_job', 'audit_logs', 'domain_events')
ORDER BY dead_ratio_percent DESC;
```

### Monitoring Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| `dead_ratio_percent` | > 10% | > 30% | Manual VACUUM immediately |
| `n_dead_tup` (river_job) | > 100,000 | > 500,000 | Check autovacuum daemon |

---

## River Job Queue Cleanup

River client is configured with automatic cleanup:

```go
riverClient, _ := river.NewClient(riverpgxv5.New(pool), &river.Config{
    // Automatically delete completed jobs after 24 hours
    CompletedJobRetentionPeriod: 24 * time.Hour,
})
```

### Manual Cleanup (Emergency)

If River automatic cleanup fails:

```sql
-- Delete completed jobs older than 7 days (emergency cleanup)
DELETE FROM river_job 
WHERE state = 'completed' 
AND finalized_at < NOW() - INTERVAL '7 days';

-- Run VACUUM after large deletes
VACUUM ANALYZE river_job;
```

---

## Connection Pool Settings

> **Reference**: [DEPENDENCIES.md](../design/DEPENDENCIES.md) §PostgreSQL

| Parameter | Recommended Value | Rationale |
|-----------|-------------------|--------------|
| `max_connections` | 100-200 | Shared pool for Ent + River + sqlc |
| `shared_buffers` | 25% of RAM | Standard for dedicated DB server |
| `effective_cache_size` | 75% of RAM | Query planner hint |
| `work_mem` | 4MB-16MB | Per-operation sort/hash memory |

---

## Backup and Recovery

### Backup Strategy

| Backup Type | Frequency | Retention |
|-------------|-----------|-----------|
| Full (pg_dump) | Daily | 30 days |
| WAL archiving | Continuous | 7 days |
| Logical (table-level) | Weekly | 90 days |

### Restore Testing

**Quarterly requirement**: Restore a backup to a test environment and verify:
1. Application startup succeeds
2. Health checks pass (`/health/ready`)
3. Recent data is present

---

## Related Documents

- [00-prerequisites.md §7.5](../design/phases/00-prerequisites.md) - PostgreSQL Stability Configuration
- [ADR-0008](../adr/ADR-0008-postgres-only.md) - PostgreSQL Only Decision
- [ADR-0012](../adr/ADR-0012-hybrid-transaction.md) - Hybrid Transaction Strategy
- [DEPENDENCIES.md](../design/DEPENDENCIES.md) - Version Requirements
