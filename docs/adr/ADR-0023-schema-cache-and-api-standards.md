---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
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
│  │   └── NO:  Fetch from GitHub, cache locally                             │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│          │                                                                   │
│          ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Periodic Health Check (every 6 hours)                                   │ │
│  │ - Re-detect KubeVirt version from cluster                               │ │
│  │ - If version changed:                                                   │ │
│  │   ├── Fetch new schema if needed                                        │ │
│  │   ├── Emit event: "ClusterKubeVirtVersionChanged"                       │ │
│  │   └── Re-validate affected InstanceSizes (async)                        │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Cache Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `schema.embedded_versions` | `["1.2.x", "1.3.x", "1.4.x", "1.5.x"]` | Versions bundled at compile time |
| `schema.cache_dir` | `/var/cache/shepherd/schemas/` | Local filesystem cache directory |
| `schema.remote_fetch_timeout` | `30s` | Timeout for fetching schema from GitHub |
| `schema.health_check_interval` | `6h` | Interval for checking cluster KubeVirt versions |
| `schema.validation_on_version_change` | `true` | Re-validate InstanceSizes when version changes |

### Schema Expiration Policy

> **Decision**: Schemas are considered valid indefinitely once cached. Expiration is triggered only by cluster version changes, not by time-based TTL.

**Rationale**: KubeVirt schema definitions are immutable per version. A schema for v1.5.x will never change, so time-based expiration adds complexity without benefit.

**Version Change Handling**:

```go
// Pseudo-code for version change handling
func (h *HealthChecker) CheckCluster(ctx context.Context, cluster *Cluster) error {
    currentVersion, err := h.detectKubeVirtVersion(ctx, cluster)
    if err != nil {
        return err
    }
    
    if currentVersion != cluster.KubeVirtVersion {
        // Version changed - this is significant
        h.logger.Warn("KubeVirt version changed",
            "cluster", cluster.Name,
            "from", cluster.KubeVirtVersion,
            "to", currentVersion,
        )
        
        // Update stored version
        cluster.KubeVirtVersion = currentVersion
        
        // Ensure we have the schema for new version
        _, err := h.schemaCache.GetOrFetch(ctx, currentVersion)
        if err != nil {
            return fmt.Errorf("schema for new version unavailable: %w", err)
        }
        
        // Trigger async re-validation of InstanceSizes targeting this cluster
        h.events.Emit(ctx, ClusterKubeVirtVersionChanged{
            ClusterID:   cluster.ID,
            OldVersion:  cluster.KubeVirtVersion,
            NewVersion:  currentVersion,
        })
    }
    
    return nil
}
```

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

1. [ ] Add schema cache configuration to `config.yaml` schema
2. [ ] Implement periodic health check for KubeVirt version detection
3. [ ] Add pagination support to `SuggestClusters` endpoint
4. [ ] Update OpenAPI spec with pagination schemas
5. [ ] Refine namespace error codes per taxonomy
6. [ ] Update `docs/operations/troubleshooting.md` with error code reference
7. [ ] Append "Amendments by Subsequent ADRs" block to ADR-0017 and ADR-0018

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

---

_End of ADR-0023_
