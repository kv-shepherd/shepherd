# ADR-0008: PostgreSQL Stability Guarantees

> **Status**: Accepted (v2.0)  
> **Date**: 2026-01-15  
> **Related**: ADR-0006 Unified Async Model  
> **Changelog**: 
>   - v1.0: Required pg_partman + pg_cron
>   - v2.0: **Removed pg_partman dependency**, adopted River built-in cleanup + Autovacuum tuning

---

## Context

After adopting River Queue (All-in-PG approach), the `river_job` table experiences INSERT/UPDATE/DELETE operations. PostgreSQL's MVCC mechanism means updates produce "dead tuples" requiring evaluation of stability measures.

---

## Decision

### Adopt: River Built-in Cleanup + Aggressive Autovacuum Tuning

> **Core Principle**: Simple and reliable > Complex and elegant. When load matches, prefer native mechanisms.

---

## 1. Load Assessment

### Platform Positioning

KubeVirt Shepherd is a **governance platform**, not a high-concurrency scheduling platform:

| Metric | Estimate | Description |
|--------|----------|-------------|
| **MaxWorkers** | ≤ 10 | Per-instance concurrency limit |
| **HPA.maxReplicas** | ≤ 5 | Pod count limit |
| **Peak concurrency** | ≤ 50 jobs/min | At 50 concurrent × avg 1 min completion |
| **Daily jobs** | ~thousands | Conservative estimate |

### pg_partman Assessment

**Conclusion**: For governance platform scenarios, pg_partman is **unnecessary and risky**. Removed.

| Factor | Analysis |
|--------|----------|
| **Load match** | Thousands of daily jobs is extremely low load for PostgreSQL |
| **Complexity/benefit ratio** | pg_partman requires extensions, cron config, schema migration |
| **River compatibility risk** | River internals (leader election, job locking) depend on specific index structures |

---

## 2. River Built-in Cleanup (Required Configuration)

```go
// internal/river/config.go

type Config struct {
    // Stability-first: strictly limit worker concurrency
    MaxWorkers int `yaml:"max_workers" default:"10"`
    
    // Poll interval
    PollInterval time.Duration `yaml:"poll_interval" default:"1s"`
    
    // Task retention policy (River built-in cleanup)
    CompletedJobRetentionPeriod time.Duration `yaml:"completed_job_retention" default:"24h"`
    CancelledJobRetentionPeriod time.Duration `yaml:"cancelled_job_retention" default:"24h"`
    DiscardedJobRetentionPeriod time.Duration `yaml:"discarded_job_retention" default:"168h"` // 7 days
}
```

---

## 3. Aggressive Autovacuum Tuning (Required - Deployment Must)

> **Core**: River's DELETE produces dead tuples. Must configure aggressive Autovacuum parameters.
>
> 🚨 **Missing this configuration will cause table bloat**.

### River Table Specific Configuration

```sql
-- Required deployment SQL - River table Autovacuum tuning
ALTER TABLE river_job SET (
    -- 1% dead tuples triggers VACUUM (default 20%)
    autovacuum_vacuum_scale_factor = 0.01,
    
    -- Minimum 100 dead rows triggers
    autovacuum_vacuum_threshold = 100,
    
    -- 1% triggers ANALYZE
    autovacuum_analyze_scale_factor = 0.01,
    
    -- Increase cleanup speed (default 200)
    autovacuum_vacuum_cost_limit = 2000,
    
    -- Reduce delay (default 2ms)
    autovacuum_vacuum_cost_delay = 5
);

ALTER TABLE river_leader SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_threshold = 50
);
```

---

## 4. Monitoring & Alerting (Required)

### Dead Tuple Monitoring View

```sql
CREATE OR REPLACE VIEW river_health AS
SELECT 
    relname,
    n_live_tup,
    n_dead_tup,
    ROUND(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 2) AS dead_ratio_pct,
    last_vacuum,
    last_autovacuum,
    last_analyze
FROM pg_stat_user_tables
WHERE relname LIKE 'river%'
ORDER BY n_dead_tup DESC;
```

### Alert Thresholds

| Metric | Warning | Critical |
|--------|---------|----------|
| Dead tuple ratio | > 10% | > 30% |
| Autovacuum not executed | > 1 hour | > 3 hours |
| PENDING job backlog | > 1000 | > 5000 |

---

## 5. Worker Rate Limiting

```go
const (
    // Per-instance max concurrency
    DefaultMaxWorkers = 10
    
    // HPA constraint: maxReplicas × MaxWorkers ≤ 50
    MaxTotalConcurrency = 50
)
```

| Scenario | MaxWorkers | HPA.maxReplicas | Total Concurrency |
|----------|------------|-----------------|-------------------|
| Small scale (<100 VM) | 5 | 3 | 15 |
| Medium scale (100-500 VM) | 10 | 3 | 30 |
| Large scale (500+ VM) | 10 | 5 | 50 |

> **Better to queue tasks than destabilize the database.**

---

## Future Extension Path

> **Roadmap**: See [RFC-0001 pg_partman Table Partitioning](../rfc/RFC-0001-pg-partman.md)
>
> **Trigger**: When daily job volume exceeds 10 million (10M).

---

## Consequences

| Expected Outcome | Description |
|------------------|-------------|
| ✅ Simplified deployment | No additional PostgreSQL extensions needed |
| ✅ River compatibility | Uses officially supported built-in cleanup |
| ✅ Dead tuples controlled | Aggressive Autovacuum ensures timely cleanup |
| ✅ Observable | Prometheus metrics + dead tuple monitoring |
| ✅ Stability guaranteed | Rate limiting + monitoring dual protection |

---

## References

- [River Queue Documentation - Maintenance](https://riverqueue.com/docs/maintenance)
- [PostgreSQL Autovacuum Tuning](https://www.postgresql.org/docs/current/routine-vacuuming.html)
