# ADR-0017: VM Request and Approval Flow Clarification

> **Status**: Proposed  
> **Date**: 2026-01-22  
> **Amends**: ADR-0015 §4 (User-Forbidden Fields and Platform-Controlled Attributes)  
> **Review Period**: Until 2026-01-28 (48-hour public comment period)

---

## Context

### Problem Statement

ADR-0015 §4 defined `VMCreateRequest` with `ClusterID` as a required field:

```go
type VMCreateRequest struct {
    ServiceID   string `json:"service_id" binding:"required"`
    TemplateID  string `json:"template_id" binding:"required"`
    ClusterID   string `json:"cluster_id" binding:"required"`  // ❌ INCORRECT
    Namespace   string `json:"namespace" binding:"required"`
    // ...
}
```

This is **incorrect** because it contradicts the governance model's core principle:

1. **Users should not select infrastructure details** (cluster is an infrastructure concern)
2. **Administrators determine deployment targets** during the approval workflow
3. **Separation of concerns**: Business request (user) vs. Infrastructure decision (admin)

### Discovered During

During document synchronization review on 2026-01-22, the following clarification was provided:

1. System and Service have **no direct association** with clusters or namespaces
2. At VM creation request time, only namespace/system/service/template are associated, **not cluster**
3. During approval, the administrator determines the **final template version** and **target cluster**

---

## Decision

### Correct VM Request Flow

**Phase 1: User Submits Request**

User provides:
- `ServiceID` - Which service this VM belongs to (inherits System governance)
- `TemplateID` - Requested template (admin may override during approval)
- `Namespace` - Target K8s namespace
- Resource parameters (CPU, Memory, Disk) - Subject to template constraints
- `Reason` - Business justification

User does **NOT** provide:
- `ClusterID` - Determined by admin during approval
- `Name` - Platform-generated
- `Labels` - Platform-managed
- `CloudInit` - Template-defined

**Phase 2: Admin Approves Request**

Admin determines/confirms:
- Final template version (may differ from user's request)
- Target cluster (based on namespace environment, cluster capacity, policy)
- Storage class (from cluster's available options)
- Any parameter overrides

### Corrected VMCreateRequest

```go
// VMCreateRequest is what users submit when requesting a new VM
type VMCreateRequest struct {
    ServiceID   string `json:"service_id" binding:"required"`
    TemplateID  string `json:"template_id" binding:"required"`
    Namespace   string `json:"namespace" binding:"required"`
    
    // NOTE: ClusterID is NOT here - admin selects during approval
    
    // Quick mode adjustable fields (controlled by template mask)
    CPU       *int `json:"cpu,omitempty"`
    MemoryMB  *int `json:"memory_mb,omitempty"`
    DiskGB    *int `json:"disk_gb,omitempty"`
    
    // Advanced mode fields (visible only if template enables)
    GPU       *int    `json:"gpu,omitempty"`
    Hugepages *string `json:"hugepages,omitempty"`
    NUMA      *string `json:"numa,omitempty"`
    
    Reason string `json:"reason" binding:"required"`
}
```

### ApprovalTicket Admin Fields

```go
// ApprovalTicket stores admin decisions made during approval
type ApprovalTicket struct {
    // ... existing fields ...
    
    // Admin-determined fields (set during approval)
    SelectedClusterID     string `json:"selected_cluster_id"`      // Admin selects target cluster
    SelectedTemplateVersion int   `json:"selected_template_version"` // Admin confirms template version
    SelectedStorageClass  string `json:"selected_storage_class"`   // From cluster's available SCs
    
    // Template snapshot at approval time (immutable record)
    TemplateSnapshot string `json:"template_snapshot"`
}
```

### Cluster Selection Logic

During approval, the platform suggests clusters based on:

```go
func SuggestClusters(namespace string) ([]ClusterSuggestion, error) {
    // 1. Get namespace's environment type (test/prod)
    nsEnv := getNamespaceEnvironment(namespace)
    
    // 2. Filter clusters matching environment
    clusters := filterClustersByEnvironment(nsEnv)
    
    // 3. Sort by scheduling weight
    sortByWeight(clusters)
    
    // 4. Return suggestions (admin makes final decision)
    return clusters, nil
}
```

Admin can:
- Accept suggested cluster
- Override with different cluster (same environment)
- Override with different environment cluster (with explicit confirmation)

---

## Consequences

### Positive

- ✅ **Clear separation of concerns**: User focuses on business needs, admin handles infrastructure
- ✅ **Governance compliance**: Infrastructure decisions require admin approval
- ✅ **Flexibility**: Admin can make informed decisions based on cluster capacity/health
- ✅ **Auditability**: Cluster selection is recorded in ApprovalTicket

### Negative

- 🟡 **Requires admin action**: Users cannot self-deploy to specific clusters
- 🟡 **Approval latency**: Cluster selection adds to approval decision time

### Mitigation

- For test environments with auto-approval policies, system can auto-select cluster based on weight
- Admin workbench shows cluster suggestions to speed up decision

---

## Implementation Impact

### Documents Requiring Updates

| Document | Section | Change |
|----------|---------|--------|
| `ADR-0015` | §4 VMCreateRequest | Add amendment notice pointing to this ADR |
| `01-contracts.md` | VM creation flow | Document correct request structure |
| `04-governance.md` | Approval workflow | Document admin cluster selection |

### API Changes

| Endpoint | Change |
|----------|--------|
| `POST /api/v1/vms` | Remove `cluster_id` from request body |
| `POST /api/v1/approvals/{id}/approve` | Add `cluster_id`, `template_version`, `storage_class` to approval body |
| `GET /api/v1/approvals/{id}/suggested-clusters` | New endpoint for cluster suggestions |

---

## References

- [ADR-0015: Governance Model V2](./ADR-0015-governance-model-v2.md) - Original governance model
- [Issue #5: Governance Model Alignment](https://github.com/kv-shepherd/shepherd/issues/5) - Design review discussion
- [Issue #15: ADR-0017 Proposal](https://github.com/kv-shepherd/shepherd/issues/15) - This proposal's discussion thread

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-22 | Initial draft, amending ADR-0015 §4 |
| 2026-01-26 | Submitted as formal proposal with 48-hour review period (Issue #15) |
