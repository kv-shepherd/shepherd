# RFC-0013: VM Snapshot

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Backup and restore capabilities required

> **Implementation Boundary**:
> - **Provider-level methods** (CreateVMSnapshot, GetVMSnapshot, etc.) are implemented in **Phase 2**
> - **This RFC covers Service-level orchestration**: scheduled backups, retention policies, cross-cluster restore

---

## Problem

Users may need to create point-in-time snapshots of VMs for backup, recovery, or cloning purposes.

---

## Proposed Solution

### Snapshot Operations

```go
type SnapshotService struct {
    provider provider.KubeVirtProvider
}

// CreateSnapshot creates a VM snapshot
func (s *SnapshotService) CreateSnapshot(ctx context.Context, input CreateSnapshotInput) (*Snapshot, error) {
    return s.provider.CreateVMSnapshot(ctx, SnapshotSpec{
        VMName:      input.VMName,
        Namespace:   input.Namespace,
        ClusterName: input.ClusterName,
        Name:        input.SnapshotName,
    })
}

// RestoreSnapshot restores VM to snapshot state
func (s *SnapshotService) RestoreSnapshot(ctx context.Context, snapshotID string) error {
    snapshot, _ := s.repo.Get(ctx, snapshotID)
    return s.provider.RestoreVMFromSnapshot(ctx, snapshot)
}
```

### API Endpoints

```
POST   /api/v1/vms/{id}/snapshots
GET    /api/v1/vms/{id}/snapshots
DELETE /api/v1/snapshots/{id}
POST   /api/v1/snapshots/{id}/restore
```

---

## Prerequisites

- KubeVirt Snapshot API enabled
- CSI snapshot support in storage class
- VolumeSnapshot CRD installed

---

## Trigger Conditions

- Backup/recovery requirements
- Pre-update snapshots needed
- Compliance requires point-in-time recovery

---

## Snapshot Lifecycle Management (Added 2026-01-26)

> **Scope**: This section covers the lifecycle management of **InstanceSize snapshots** stored in `ApprovalTicket.instance_size_snapshot`, as defined in [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md#instancesize-immutability-snapshot-pattern).
>
> VM snapshots (via KubeVirt Snapshot API) follow separate lifecycle policies managed by storage layer.

### Problem

`instance_size_snapshot` provides immutability guarantees for approved VMs, but raises questions:
- How long should snapshots be retained after VM deletion?
- How to balance compliance requirements with storage costs?
- How to handle GDPR "right to be forgotten" while maintaining audit trails?

### Lifecycle States

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Snapshot Lifecycle State Machine                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   [ACTIVE]         [ARCHIVED]         [TOMBSTONE]        [PURGED]           │
│      │                  │                  │                  │              │
│      │   VM Deleted     │   1 Year After   │   2 Years After  │              │
│      │   ─────────►     │   ─────────►     │   ─────────►     │              │
│      │                  │   Archive        │   Tombstone      │              │
│                                                                              │
│   Full snapshot     Summarized data    Metadata only      Hard deleted       │
│   (Complete JSON)   (Key fields)       (ID, hash, date)   (No trace)        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Retention Policy (Based on Industry Standards)

| Stage | Retention Duration | Data Retained | Compliance Basis |
|-------|-------------------|---------------|------------------|
| **ACTIVE** | Until VM deleted | Full `instance_size_snapshot` JSON | Operational requirement |
| **ARCHIVED** | 1 year after VM deletion | Summarized snapshot (key fields only) | PCI-DSS (1 year audit logs) |
| **TOMBSTONE** | 2 additional years (3 years total) | Metadata only: ID, hash, timestamp, operator | SOC2/SOX (audit trail) |
| **PURGED** | After tombstone period | Physically deleted | GDPR data minimization |

> **Note**: Default retention values are configurable. Financial/healthcare sectors may require 7+ years.

### Database Schema Extension

```sql
-- Extend approval_tickets table OR create dedicated snapshot table
CREATE TABLE instance_size_snapshots (
    id UUID PRIMARY KEY,
    approval_ticket_id UUID NOT NULL REFERENCES approval_tickets(id),
    
    -- Snapshot content
    instance_size_snapshot JSONB,                 -- Full snapshot (cleared on archive)
    snapshot_hash VARCHAR(64) NOT NULL,           -- SHA256 for integrity verification
    
    -- Lifecycle management
    lifecycle_state VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    vm_deleted_at TIMESTAMPTZ,                    -- When associated VM was deleted
    archived_at TIMESTAMPTZ,                      -- When transitioned to ARCHIVED
    tombstoned_at TIMESTAMPTZ,                    -- When transitioned to TOMBSTONE
    purge_after TIMESTAMPTZ,                      -- Scheduled deletion time
    
    -- Archived summary (populated during archive transition)
    archived_summary JSONB,
    
    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID NOT NULL REFERENCES users(id),
    
    CONSTRAINT valid_lifecycle_state CHECK (
        lifecycle_state IN ('ACTIVE', 'ARCHIVED', 'TOMBSTONE')
    )
);

-- Indexes for lifecycle job
CREATE INDEX idx_snapshots_lifecycle 
    ON instance_size_snapshots(lifecycle_state, vm_deleted_at);
CREATE INDEX idx_snapshots_purge 
    ON instance_size_snapshots(purge_after) 
    WHERE purge_after IS NOT NULL;
```

### Archived Summary Structure

When transitioning from ACTIVE to ARCHIVED, the full snapshot is replaced with a summary:

```go
// ArchivedSummary contains essential fields for audit without full config
type ArchivedSummary struct {
    SizeName        string    `json:"size_name"`         // "gpu-workstation"
    DisplayName     string    `json:"display_name"`      // "GPU Workstation (8 vCPU)"
    CPUCores        int       `json:"cpu_cores"`
    MemoryMB        int       `json:"memory_mb"`
    StorageGB       int       `json:"storage_gb"`
    RequiresGPU     bool      `json:"requires_gpu"`
    CreatedAt       time.Time `json:"created_at"`
    ApprovedBy      string    `json:"approved_by"`       // Operator username
    SnapshotHash    string    `json:"snapshot_hash"`     // SHA256 of original
    OriginalVersion string    `json:"original_version"`  // InstanceSize version
}
```

### Lifecycle Management Job

```go
// SnapshotLifecycleJob runs daily to transition snapshot states
func SnapshotLifecycleJob(ctx context.Context, db *sql.DB, config LifecycleConfig) error {
    now := time.Now()
    
    // 1. ACTIVE → ARCHIVED: VM deleted > archive_retention_days ago
    archiveThreshold := now.AddDate(0, 0, -config.ActiveRetentionDays)
    _, err := db.ExecContext(ctx, `
        UPDATE instance_size_snapshots
        SET lifecycle_state = 'ARCHIVED',
            archived_at = NOW(),
            archived_summary = build_archived_summary(instance_size_snapshot),
            instance_size_snapshot = NULL  -- Clear full snapshot
        WHERE lifecycle_state = 'ACTIVE'
          AND vm_deleted_at IS NOT NULL
          AND vm_deleted_at < $1
    `, archiveThreshold)
    if err != nil {
        return fmt.Errorf("archive transition failed: %w", err)
    }
    
    // 2. ARCHIVED → TOMBSTONE: archived > archive_retention_days ago
    tombstoneThreshold := now.AddDate(0, 0, -config.ArchiveRetentionDays)
    _, err = db.ExecContext(ctx, `
        UPDATE instance_size_snapshots
        SET lifecycle_state = 'TOMBSTONE',
            tombstoned_at = NOW(),
            purge_after = NOW() + INTERVAL '$1 days',
            archived_summary = jsonb_build_object(
                'size_name', archived_summary->>'size_name',
                'snapshot_hash', archived_summary->>'snapshot_hash',
                'created_at', archived_summary->>'created_at',
                'approved_by', archived_summary->>'approved_by'
            )  -- Further reduce to metadata only
        WHERE lifecycle_state = 'ARCHIVED'
          AND archived_at < $2
    `, config.TombstoneRetentionDays, tombstoneThreshold)
    if err != nil {
        return fmt.Errorf("tombstone transition failed: %w", err)
    }
    
    // 3. TOMBSTONE → PURGED: purge_after has passed
    _, err = db.ExecContext(ctx, `
        DELETE FROM instance_size_snapshots
        WHERE lifecycle_state = 'TOMBSTONE'
          AND purge_after < NOW()
    `)
    if err != nil {
        return fmt.Errorf("purge failed: %w", err)
    }
    
    return nil
}
```

### Configuration

```yaml
# config/lifecycle_policy.yaml (or stored in database)
snapshot_retention:
  # Days to keep in ACTIVE state after VM deletion
  active_retention_days: 365        # 1 year (PCI-DSS minimum)
  
  # Days to keep in ARCHIVED state
  archive_retention_days: 730       # 2 years
  
  # Days to keep in TOMBSTONE state before purge
  tombstone_retention_days: 730     # 2 years (total 5 years)
  
  # Tenant-specific overrides (optional)
  tenant_overrides:
    finance-department:
      active_retention_days: 365
      archive_retention_days: 1095   # 3 years
      tombstone_retention_days: 1095 # 3 years (total 7 years for SOX)
    
    gdpr-sensitive:
      active_retention_days: 90      # Minimize retention
      archive_retention_days: 275    # ~1 year total
      tombstone_retention_days: 0    # No tombstone, direct purge
```

### GDPR Considerations

| Requirement | Implementation |
|-------------|----------------|
| **Right to Erasure** | If user requests deletion, mark for immediate purge (skip archive/tombstone) |
| **Data Minimization** | Archive reduces data to essential audit fields only |
| **Purpose Limitation** | Snapshots only used for audit, not re-processing |
| **Storage Limitation** | Configurable retention with automatic cleanup |

```go
// Handle GDPR erasure request
func HandleErasureRequest(ctx context.Context, userID uuid.UUID) error {
    // Mark all user's snapshots for immediate purge
    _, err := db.ExecContext(ctx, `
        UPDATE instance_size_snapshots
        SET lifecycle_state = 'TOMBSTONE',
            purge_after = NOW(),  -- Immediate purge
            archived_summary = jsonb_build_object(
                'erased_at', NOW(),
                'reason', 'GDPR_ERASURE_REQUEST'
            ),
            instance_size_snapshot = NULL
        WHERE created_by = $1
          AND lifecycle_state IN ('ACTIVE', 'ARCHIVED')
    `, userID)
    return err
}
```

---

## API Extensions for Snapshot Lifecycle

```
GET    /api/v1/admin/snapshots/stats        # Lifecycle statistics
POST   /api/v1/admin/snapshots/cleanup      # Manual cleanup trigger
GET    /api/v1/admin/snapshots/{id}/history # View lifecycle transitions
DELETE /api/v1/admin/users/{id}/snapshots   # GDPR erasure
```

---

## References

- [KubeVirt Snapshot](https://kubevirt.io/user-guide/virtual_machines/snapshot_restore_api/)
- [ADR-0004: Provider Interface](../adr/ADR-0004-provider-interface.md)
- [ADR-0018: Instance Size Abstraction](../adr/ADR-0018-instance-size-abstraction.md)
- [PCI-DSS Requirement 10.7](https://www.pcisecuritystandards.org/) - Audit log retention
- [GDPR Article 17](https://gdpr.eu/article-17-right-to-be-forgotten/) - Right to erasure
