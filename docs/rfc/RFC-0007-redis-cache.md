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

### V1.5 Alternative: In-Process Cache (Before Redis)

If performance bottlenecks emerge but do not yet justify Redis complexity, consider **in-process caching** as an intermediate step:

| Library | Best For | GC Impact | Features |
|---------|----------|-----------|----------|
| **ristretto** | High hit-ratio scenarios | Low | Cost-based eviction, TinyLFU admission |
| **bigcache** | Large data volumes | Minimal | Custom memory management, sharding |
| **go-cache** | Simple use cases | Standard | TTL support, easy API |

**Recommended: ristretto** for Shepherd due to:
- Cost-based eviction (larger items can have higher cost)
- High hit ratio with TinyLFU algorithm
- Generics support (v2.x)

**Implementation Pattern:**

```go
// internal/cache/cached_repository.go

type CachedInstanceSizeRepo struct {
    next  repository.InstanceSizeRepository
    cache *ristretto.Cache[string, *ent.InstanceSize]
    ttl   time.Duration
}

func NewCachedInstanceSizeRepo(next repository.InstanceSizeRepository) *CachedInstanceSizeRepo {
    cache, _ := ristretto.NewCache(&ristretto.Config[string, *ent.InstanceSize]{
        NumCounters: 1e4,     // 10,000 keys to track
        MaxCost:     1 << 20, // 1MB max
        BufferItems: 64,
    })
    return &CachedInstanceSizeRepo{next: next, cache: cache, ttl: 5 * time.Minute}
}

func (r *CachedInstanceSizeRepo) GetByName(ctx context.Context, name string) (*ent.InstanceSize, error) {
    if cached, found := r.cache.Get(name); found {
        return cached, nil
    }
    
    is, err := r.next.GetByName(ctx, name)
    if err != nil {
        return nil, err
    }
    
    r.cache.SetWithTTL(name, is, 1, r.ttl)
    return is, nil
}

// Invalidation hook - call on write operations
func (r *CachedInstanceSizeRepo) Invalidate(name string) {
    r.cache.Del(name)
}
```

**When to Use In-Process Cache:**
- Single replica deployment OR cache-miss is acceptable during rolling updates
- Data changes infrequently (InstanceSize list, KubeVirt schema)
- QPS < 1000 per resource (above this, consider Redis)

**When NOT to Use:**
- Multi-replica deployment with strict consistency requirements
- Data changes frequently and must be immediately consistent across pods

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
