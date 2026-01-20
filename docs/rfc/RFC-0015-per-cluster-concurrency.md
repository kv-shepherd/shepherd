# RFC-0015: Per-Cluster Concurrency Control

> **Status**: Deferred  
> **Priority**: P3  
> **Source**: Backlog  
> **Trigger**: Per-cluster rate limiting required for large deployments

---

## Problem

V1.0 uses global worker concurrency limits. Large deployments with many clusters may need:
- Per-cluster concurrency quotas
- Cluster-specific rate limiting
- Fair scheduling across clusters

---

## Current State

**Not implementing**

Global MaxWorkers limit (10 per instance) sufficient for governance platform load.

---

## Proposed Solution

### Semaphore Per Cluster

```go
type ClusterSemaphore struct {
    pool    *pgxpool.Pool
    limits  map[string]int  // cluster -> max concurrent
}

// Acquire acquires permit for cluster
func (s *ClusterSemaphore) Acquire(ctx context.Context, clusterName string) (release func(), err error) {
    // PostgreSQL advisory lock with cluster-specific key
    lockKey := hashClusterKey(clusterName)
    
    // Try to acquire with limit checking
    // Uses pg_advisory_xact_lock + counter table
}
```

### Configuration

```yaml
concurrency:
  global_max: 50
  per_cluster:
    cluster-prod: 10
    cluster-dev: 20
    default: 5
```

---

## Alternative: Redis-based Semaphore

```go
// Using Redis for distributed counting
func (s *RedisSemaphore) Acquire(ctx context.Context, cluster string) error {
    key := fmt.Sprintf("semaphore:%s", cluster)
    current, _ := s.redis.Incr(ctx, key).Result()
    
    if current > s.limits[cluster] {
        s.redis.Decr(ctx, key)
        return ErrLimitExceeded
    }
    return nil
}
```

---

## Trigger Conditions

- Managing 10+ clusters
- Some clusters are resource-constrained
- Need fair scheduling across environments

---

## References

- [PostgreSQL Advisory Locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- [ADR-0008: PostgreSQL Stability](../adr/ADR-0008-postgresql-stability.md)
