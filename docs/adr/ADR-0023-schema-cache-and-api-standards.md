---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-28
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0023: Schema Cache Management and API Standardization

> **Review Period**: Until 2026-01-31 (48-hour minimum)  
> **Discussion**: [Issue #TBD](https://github.com/kv-shepherd/shepherd/issues/TBD)  
> **Amends**: [ADR-0017](./ADR-0017-vm-request-flow-clarification.md), [ADR-0018](./ADR-0018-instance-size-abstraction.md)

---

## Context and Problem Statement

ADR-0017 and ADR-0018 have been accepted, establishing the VM request flow and Instance Size abstraction patterns. However, review feedback identified several operational concerns that were not fully addressed:

1. **Schema Cache Expiration**: ADR-0018 defines versioned Schema caching but lacks explicit TTL/expiration policies, risking validation against stale schemas
2. **API Pagination Standards**: ADR-0017's `SuggestClusters` endpoint lacks pagination, which may cause issues with large cluster deployments
3. **Error Code Granularity**: Namespace creation errors use a single error code, but operational clarity requires finer distinction

This ADR defines supplementary standards to address these gaps without modifying the accepted ADRs.

---

## Decision Drivers

* **Operational reliability**: Prevent silent failures from stale cached data
* **Scalability**: Support deployments with 50+ managed clusters
* **Debuggability**: Enable precise error diagnosis in production
* **ADR immutability**: Supplement rather than modify accepted decisions

---

## Decision Outcome

Adopt the following supplementary standards that extend ADR-0017 and ADR-0018.

---

## 1. Schema Cache Management Policy

### Design Motivation

Schema caching addresses **four core operational pain points**:

| Pain Point | Problem Without Cache | Solution With Cache |
|------------|----------------------|---------------------|
| **1. Frontend Performance** (Schema-Driven UI) | OpenAPI Spec is 3-5MB. Each form open requires download and browser parsing, causing 1-2s lag. | Backend returns simplified, pre-parsed JSON (~50KB). Instant page load. |
| **2. Multi-Version Compatibility** (Matrix Problem) | With 10+ clusters at different KubeVirt versions: validation impossible before cluster selection (ADR-0017 specifies cluster selection at approval time). | Pre-load all managed version schemas. Enable pre-validation: "This config requires v1.5+ clusters". |
| **3. Offline Validation** | When target cluster API is unreachable (network flap, control plane upgrade), requests fail immediately. | Validate against cached schema, create ticket, execute when cluster recovers. Business flow uninterrupted. |
| **4. Schema Immutability Leverage** | N/A | KubeVirt schemas are immutable per version (v1.5.0 schema never changes). Enables indefinite caching with version-triggered updates only. |

### Cache Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  KubeVirt Schema Cache Lifecycle                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────┐                                                          │
│  │ Application    │                                                          │
│  │ Startup        │                                                          │
│  └───────┬────────┘                                                          │
│          │                                                                   │
│          ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Load Embedded Schemas (bundled at compile time)                         │ │
│  │ - kubevirt-v1.2.x.json, v1.3.x.json, v1.4.x.json, v1.5.x.json           │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│          │                                                                   │
│          ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Register Cluster                                                        │ │
│  │ - Detect KubeVirt version (e.g., v1.5.2)                                │ │
│  │ - Extract minor version (1.5.x)                                         │ │
│  │ - Check if schema exists in cache                                       │ │
│  │   ├── YES: Use cached schema                                            │ │
│  │   └── NO:  Queue SchemaFetchJob (River), use embedded fallback          │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│          │                                                                   │
│          ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ ClusterHealthChecker Integration (Piggyback Model)                      │ │
│  │                                                                          │ │
│  │ ┌─────────────────────────────────────────────────────────────────────┐ │ │
│  │ │ Health Check Loop (every 60s)                                       │ │ │
│  │ │ - Primary: Check cluster Running status                             │ │ │
│  │ │ - Piggyback: Read KubeVirt Operator Status (version field)          │ │ │
│  │ │   └── Cost: 1 additional field read, negligible overhead            │ │ │
│  │ └─────────────────────────────────────────────────────────────────────┘ │ │
│  │                          │                                               │ │
│  │                          ▼                                               │ │
│  │ ┌─────────────────────────────────────────────────────────────────────┐ │ │
│  │ │ Version Diff Check                                                  │ │ │
│  │ │ - Compare: clusters.kubevirt_version vs. detected version           │ │ │
│  │ │ - If SAME:  No action (most common case)                            │ │ │
│  │ │ - If DIFFERENT:                                                     │ │ │
│  │ │   1. Queue SchemaUpdateJob (River Job)                              │ │ │
│  │ │   2. Emit "ClusterKubeVirtVersionChanged" event                     │ │ │
│  │ │   3. Update clusters.kubevirt_version                               │ │ │
│  │ │   4. Async: Re-validate affected InstanceSizes                      │ │ │
│  │ └─────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                          │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### ClusterHealthChecker Integration

> **Design Decision**: Version detection is **piggybacked** onto existing health checks, NOT a separate polling loop.

**Rationale**:
- Health check already connects to cluster every 60s
- Reading KubeVirt Operator status adds negligible overhead (1 field)
- KubeVirt upgrades are **infrequent** (monthly/quarterly), so most checks return "no change"
- Avoids separate 6h polling interval complexity

```go
// ClusterHealthChecker with piggyback version detection
type ClusterHealthChecker struct {
    healthCheckInterval time.Duration  // 60s - checks cluster reachability
    schemaCache         *SchemaCache
    riverClient         *river.Client
    db                  *ent.Client
}

// CheckCluster is called every 60s per cluster
func (h *ClusterHealthChecker) CheckCluster(ctx context.Context, cluster *ent.Cluster) error {
    // Primary responsibility: health check
    status, err := h.checkClusterHealth(ctx, cluster)
    if err != nil {
        return h.handleUnhealthyCluster(ctx, cluster, err)
    }
    
    // Piggyback: version detection (low overhead)
    detectedVersion := status.KubeVirtOperatorVersion  // e.g., "v1.5.2"
    if detectedVersion == "" {
        // KubeVirt not installed or status unavailable
        return nil
    }
    
    storedVersion := cluster.KubevirtVersion
    if detectedVersion != storedVersion {
        return h.handleVersionChange(ctx, cluster, storedVersion, detectedVersion)
    }
    
    return nil
}

func (h *ClusterHealthChecker) handleVersionChange(
    ctx context.Context,
    cluster *ent.Cluster,
    oldVersion, newVersion string,
) error {
    h.logger.Info("KubeVirt version change detected",
        "cluster", cluster.Name,
        "from", oldVersion,
        "to", newVersion,
    )
    
    // 1. Queue async schema fetch job (non-blocking)
    _, err := h.riverClient.Insert(ctx, &SchemaUpdateJob{
        ClusterID:  cluster.ID,
        NewVersion: newVersion,
    }, nil)
    if err != nil {
        h.logger.Error("failed to queue schema update job", "error", err)
        // Continue - don't block health check for schema issues
    }
    
    // 2. Update cluster record
    err = cluster.Update().
        SetKubevirtVersion(newVersion).
        Exec(ctx)
    if err != nil {
        return fmt.Errorf("update cluster version: %w", err)
    }
    
    // 3. Emit event for downstream consumers
    h.events.Emit(ctx, ClusterKubeVirtVersionChanged{
        ClusterID:  cluster.ID,
        OldVersion: oldVersion,
        NewVersion: newVersion,
    })
    
    return nil
}
```

### Cache Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `schema.embedded_versions` | `["1.2.x", "1.3.x", "1.4.x", "1.5.x"]` | Versions bundled at compile time |
| `schema.cache_dir` | `/var/cache/shepherd/schemas/` | Local filesystem cache directory |
| `schema.remote_fetch_timeout` | `30s` | Timeout for fetching schema from GitHub |
| `schema.remote_fetch_retries` | `3` | Retry count for remote fetch with exponential backoff |
| `schema.validation_on_version_change` | `true` | Re-validate InstanceSizes when version changes |

> **Note**: `health_check_interval` is configured at the ClusterHealthChecker level (default 60s), not in schema config. Version detection piggybacks on this existing loop.

### Schema Expiration Policy

> **Decision**: Schemas are considered valid indefinitely once cached. Expiration is triggered only by cluster version changes, not by time-based TTL.

**Rationale**: KubeVirt schema definitions are immutable per version. A schema for v1.5.x will never change, so time-based expiration adds complexity without benefit.

### Graceful Degradation Strategy

When schema fetch fails, the system MUST NOT block operations:

| Scenario | Behavior | User Impact |
|----------|----------|-------------|
| **Schema fetch timeout** | Log warning, continue with closest embedded version | Minor: advanced features may not validate correctly |
| **GitHub API rate limited** | Retry with exponential backoff (30s, 60s, 120s), use embedded fallback | None if embedded version exists |
| **No matching embedded schema** | Log error, alert admin, allow cluster registration | Admin must manually provide schema or wait for retry |
| **Cluster unreachable during version check** | Keep existing version, mark cluster unhealthy | Existing validation continues working |

```go
// SchemaUpdateJob - River job for async schema fetching
type SchemaUpdateJob struct {
    ClusterID  uuid.UUID `json:"cluster_id"`
    NewVersion string    `json:"new_version"`
}

func (j *SchemaUpdateJob) Work(ctx context.Context) error {
    minorVersion := extractMinorVersion(j.NewVersion)  // "v1.5.2" -> "1.5.x"
    
    // Try fetching from remote
    schema, err := j.schemaFetcher.FetchFromGitHub(ctx, minorVersion)
    if err != nil {
        // Fallback: check embedded schemas
        if embedded, ok := j.schemaCache.GetEmbedded(minorVersion); ok {
            j.logger.Warn("using embedded schema as fallback",
                "version", minorVersion,
                "fetch_error", err,
            )
            return j.schemaCache.Store(minorVersion, embedded)
        }
        
        // No fallback available - retry later
        return fmt.Errorf("schema fetch failed, no fallback: %w", err)
    }
    
    // Store fetched schema
    return j.schemaCache.Store(minorVersion, schema)
}

---

## 2. API Pagination Standards

### Standard Pagination Parameters

All list endpoints MUST support pagination using the following query parameters:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number (1-indexed) |
| `per_page` | integer | 20 | Items per page (max: 100) |
| `sort_by` | string | varies | Field to sort by |
| `sort_order` | string | `asc` | Sort direction: `asc` or `desc` |

### Standard Pagination Response

```yaml
# OpenAPI schema for paginated responses
PaginatedResponse:
  type: object
  required: [items, pagination]
  properties:
    items:
      type: array
      description: Array of items for current page
    pagination:
      type: object
      required: [page, per_page, total_items, total_pages]
      properties:
        page:
          type: integer
          minimum: 1
        per_page:
          type: integer
          minimum: 1
          maximum: 100
        total_items:
          type: integer
          minimum: 0
        total_pages:
          type: integer
          minimum: 0
        has_next:
          type: boolean
        has_prev:
          type: boolean
```

### Endpoints Requiring Pagination

Per this ADR, the following endpoints from ADR-0017 MUST implement pagination:

| Endpoint | Default Sort | Notes |
|----------|--------------|-------|
| `GET /api/v1/approvals/{id}/suggested-clusters` | `name asc` | Critical for multi-cluster deployments |
| `GET /api/v1/vms` | `created_at desc` | Large services may have 100+ VMs |
| `GET /api/v1/systems` | `name asc` | |
| `GET /api/v1/services` | `name asc` | |
| `GET /api/v1/admin/clusters` | `name asc` | |

---

## 3. Error Code Taxonomy

### Namespace Operation Error Codes

Extend ADR-0017's namespace error handling with granular codes:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `NAMESPACE_PERMISSION_DENIED` | 403 | Shepherd lacks permission to create namespace |
| `NAMESPACE_ALREADY_EXISTS` | 200 | Namespace exists, no action needed (success) |
| `NAMESPACE_QUOTA_EXCEEDED` | 403 | Cluster namespace quota reached |
| `NAMESPACE_NAME_INVALID` | 400 | Name violates RFC 1035/ADR-0019 rules |
| `NAMESPACE_CREATION_FAILED` | 500 | Unknown error during creation |

### Implementation

```go
// Refine namespace creation error handling
func EnsureNamespaceExists(ctx context.Context, cluster *Cluster, nsName string, env string) error {
    exists, err := checkNamespaceExists(ctx, cluster, nsName)
    if err != nil {
        return fmt.Errorf("failed to check namespace: %w", err)
    }
    if exists {
        // Log but return success - this is expected in many cases
        slog.Debug("namespace already exists", "namespace", nsName, "cluster", cluster.Name)
        return nil
    }
    
    err = createNamespace(ctx, cluster, nsName, labels)
    if err != nil {
        return classifyNamespaceError(err, cluster.Name, nsName)
    }
    
    return nil
}

func classifyNamespaceError(err error, clusterName, nsName string) error {
    if isPermissionDenied(err) {
        return &apperror.AppError{
            Code:    "NAMESPACE_PERMISSION_DENIED",
            Message: fmt.Sprintf("Cannot create namespace '%s' on cluster '%s': insufficient permissions", nsName, clusterName),
            Details: map[string]string{
                "hint": "kubectl create namespace " + nsName,
                "cluster": clusterName,
            },
        }
    }
    
    if isQuotaExceeded(err) {
        return &apperror.AppError{
            Code:    "NAMESPACE_QUOTA_EXCEEDED",
            Message: fmt.Sprintf("Cluster '%s' has reached namespace quota limit", clusterName),
        }
    }
    
    // Unknown error
    return &apperror.AppError{
        Code:    "NAMESPACE_CREATION_FAILED",
        Message: fmt.Sprintf("Failed to create namespace '%s': %v", nsName, err),
    }
}
```

---

## Consequences

### Positive

* ✅ Schema cache behavior is now fully defined and predictable
* ✅ API pagination prevents performance issues at scale
* ✅ Granular error codes improve operational debugging
* ✅ Accepted ADRs remain unmodified

### Negative

* 🟡 Additional configuration parameters to manage
* 🟡 Pagination adds slight complexity to API clients

### Neutral

* Existing ADR-0017/0018 implementations may need minor updates to comply

---

## Confirmation

* Schema cache behavior validated through integration tests
* Pagination tested with 100+ items
* Error codes documented in API specification

---

## Acceptance Checklist (Execution Tasks)

Upon acceptance, perform the following:

### Schema Cache Implementation

1. [ ] Add schema cache configuration to `config.yaml` schema
2. [ ] Integrate KubeVirt version detection into `ClusterHealthChecker` (piggyback model)
3. [ ] Implement `SchemaUpdateJob` River job for async schema fetching
4. [ ] Implement graceful degradation with embedded schema fallback
5. [ ] Bundle embedded schemas for KubeVirt v1.2.x - v1.5.x at compile time

### API Pagination

6. [ ] Add pagination support to `SuggestClusters` endpoint
7. [ ] Update OpenAPI spec with `PaginatedResponse` schema
8. [ ] Add pagination to list endpoints: `/vms`, `/systems`, `/services`, `/admin/clusters`

### Error Handling

9. [ ] Refine namespace error codes per taxonomy
10. [ ] Update `docs/operations/troubleshooting.md` with error code reference

### Documentation

11. [ ] Append "Amendments by Subsequent ADRs" block to ADR-0017 and ADR-0018

---

## References

* [ADR-0017: VM Request and Approval Flow Clarification](./ADR-0017-vm-request-flow-clarification.md)
* [ADR-0018: Instance Size Abstraction Layer](./ADR-0018-instance-size-abstraction.md)
* [KubeVirt API Versions](https://kubevirt.io/api-reference/)
* [JSON Schema for KubeVirt](https://github.com/kubevirt/kubevirt/tree/main/api/openapi-spec)

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-28 | @jindyzhao | Initial draft based on ADR-0017/0018 review feedback |
| 2026-01-28 | @jindyzhao | Added: Design motivation (4 core pain points), ClusterHealthChecker piggyback integration, graceful degradation strategy |

---

_End of ADR-0023_
