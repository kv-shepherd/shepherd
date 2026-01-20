# RFC-0007: Redis Cache Support

> **Status**: Deferred  
> **Priority**: P3  
> **Source**: ADR-0006, ADR-0008  
> **Trigger**: Cache miss causing performance bottleneck

---

## Problem

The platform may need a distributed cache layer for high-frequency read scenarios that PostgreSQL alone cannot efficiently handle.

---

## Current State

**Not implementing (Deferred)**

In V1.0 architecture simplification, Redis dependency was completely removed:

| Scenario | V1.0 Alternative | Notes |
|----------|------------------|-------|
| Session storage | PostgreSQL + scs | Supports immediate revocation |
| Distributed lock | PostgreSQL Advisory Lock | Transaction-safe |
| Task queue | River Queue | PostgreSQL native |
| Cluster state cache | Ent local query + sync.Map | Low-frequency access |

---

## Trigger Conditions

Re-introduce Redis when these scenarios occur:

1. **High-frequency reads**: Single resource > 100 queries/second
2. **Cross-Pod cache consistency**: Multi-replica deployment needs shared cache
3. **Config hot-reload broadcast**: Redis Pub/Sub for change notifications
4. **Rate limiting counter**: High-precision sliding window rate limiting

---

## Proposed Solution

```go
// internal/service/cache_service.go

type CacheService struct {
    redis  *redis.Client       // Primary cache
    local  sync.Map            // Local L1 cache
    client *ent.Client         // Database fallback
}

// Get two-level cache read
func (c *CacheService) Get(ctx context.Context, key string) (interface{}, error) {
    // L1: Local cache
    if val, ok := c.local.Load(key); ok {
        return val, nil
    }
    
    // L2: Redis cache
    val, err := c.redis.Get(ctx, key).Result()
    if err == nil {
        c.local.Store(key, val) // Backfill L1
        return val, nil
    }
    
    // L3: Database fallback
    // ...
}
```

---

## Evaluation Criteria

Before introducing Redis, evaluate:

| Metric | Threshold | Description |
|--------|-----------|-------------|
| QPS | > 1000 | Single resource read frequency |
| Latency requirement | < 10ms | P99 response time |
| Consistency requirement | Weak | Can accept brief inconsistency |
| Operations complexity | Acceptable | Team has Redis operations experience |

---

## Configuration

```yaml
# config.yaml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  pool_size: 10
  min_idle_conns: 5
```

---

## References

- [ADR-0006: Unified Async Model](../adr/ADR-0006-unified-async-model.md) - River replaces Redis queue
- [ADR-0008: PostgreSQL Stability](../adr/ADR-0008-postgresql-stability.md) - Advisory Lock replaces Redis lock
