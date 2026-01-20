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

## References

- [KubeVirt Snapshot](https://kubevirt.io/user-guide/virtual_machines/snapshot_restore_api/)
- [ADR-0004: Provider Interface](../adr/ADR-0004-provider-interface.md)
