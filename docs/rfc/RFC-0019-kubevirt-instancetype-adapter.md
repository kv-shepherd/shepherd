# RFC-0019: KubeVirt Instancetype Adapter

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Users require import/export of KubeVirt Instancetype/Preference across clusters, or cross-cluster migration needs higher-fidelity interoperability
> **Related**: [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md), [ADR-0022](../adr/ADR-0022-modular-provider-pattern.md), [RFC-0012](./RFC-0012-kubevirt-advanced.md)

---

## Problem

The platform intentionally uses its own canonical `InstanceSize` model instead of depending directly on KubeVirt Instancetype CRDs as the primary source of truth. That decision fits a multi-cluster control plane: clusters may be on different KubeVirt versions, may expose different CRDs or feature gates, and may not share the same namespaced or cluster-wide catalog objects.

However, users may still want interoperability with upstream KubeVirt objects:

- Import existing `VirtualMachineInstancetype` or `VirtualMachineClusterInstancetype`
- Import matching `VirtualMachinePreference` or `VirtualMachineClusterPreference`
- Export platform-managed sizes into cluster-local KubeVirt resources
- Migrate VM definitions across clusters without manually recreating every instancetype/preference object

## Proposed Solution

Add an optional `KubeVirtInstancetypeAdapter` as a compatibility layer. It is **not** the platform's primary data model and is **not** required for V1.

### Design Principles

| Principle | Description |
|-----------|-------------|
| Canonical source remains platform DB | `InstanceSize` stays authoritative for platform workflows |
| Adapter is optional | Clusters without Instancetype CRDs must continue to work |
| Normalize on import | Imported CRDs are mapped into platform-managed fields and snapshots |
| Preserve raw metadata | Keep enough source metadata to support traceability and later export |
| Prefer portable execution | Where practical, use expanded VM specs rather than assuming target clusters already contain the same CRDs |

### Adapter Responsibilities

#### 1. Import

Read upstream KubeVirt resources from a source cluster:

- `VirtualMachineInstancetype`
- `VirtualMachineClusterInstancetype`
- `VirtualMachinePreference`
- `VirtualMachineClusterPreference`

Normalize them into:

- Platform `InstanceSize` indexed fields
- Platform `spec_overrides` / compatibility metadata
- Optional raw source snapshot for audit and later export

#### 2. Export

Generate target-cluster artifacts from a platform-managed `InstanceSize`:

- Cluster-scoped or namespaced KubeVirt Instancetype/Preference CRDs
- Or a directly expanded VM spec when portability is more important than CRD reuse

#### 3. Compatibility Discovery

Use provider capability detection to determine whether a target cluster supports the required instancetype/preference APIs and features before import/export.

---

## Technical Notes

### Upstream Facts Relevant to the Adapter

KubeVirt provides both cluster-wide and namespaced instancetype resources, and VMs can reference either scope. KubeVirt also supports `referencePolicy` modes such as `expand` and `expandAll`, which expand instancetype/preference data into the VM spec.

This is important for Shepherd because `expand`-style portability is often a better fit for multi-cluster migration than requiring every destination cluster to pre-host identical CRDs.

### Proposed Interface

```go
type KubeVirtInstancetypeAdapter interface {
    ImportInstanceType(ctx context.Context, clusterName, kind, namespace, name string) (*ImportedInstanceSize, error)
    ExportInstanceType(ctx context.Context, clusterName string, sizeID string, mode ExportMode) (*ExportResult, error)
    DetectSupport(ctx context.Context, clusterName string) (*AdapterSupport, error)
}
```

### Export Modes

| Mode | Description | Best Fit |
|------|-------------|----------|
| `crd` | Create or update Instancetype/Preference CRDs on target cluster | Clusters managed as long-lived catalogs |
| `expanded-vm` | Expand settings into VM spec without external CRD dependency | Cross-cluster migration and portability |
| `dry-run` | Validate exportability without writing resources | Pre-flight checks |

---

## Trade-offs

### Pros

- Improves interoperability with upstream KubeVirt ecosystems
- Lowers migration friction for clusters that already use Instancetype/Preference
- Supports future import/export tooling without changing the platform's canonical model
- Keeps V1 simple by making the adapter optional

### Cons

- Adds mapping complexity between platform and upstream concepts
- Not every platform governance rule maps cleanly to upstream CRDs
- Cross-cluster exports may still fail when destination capabilities differ
- Requires careful version and feature compatibility handling

---

## Implementation Notes

### Out of Scope for This RFC

- Replacing platform `InstanceSize` as the primary model
- Guaranteeing perfect round-trip fidelity for every advanced field
- Requiring Instancetype CRDs for basic VM creation flows

### Likely Incremental Rollout

1. Detect instancetype/preference API support per cluster
2. Implement read-only import prototype
3. Add export in `dry-run` mode
4. Add full CRD export and `expanded-vm` export paths
5. Integrate with migration/import UX

### When to Revisit

Promote this RFC when one or more of the following becomes true:

- Users explicitly request upstream KubeVirt catalog import/export
- Cross-cluster migration becomes a roadmap priority
- Multiple managed clusters already maintain native instancetype catalogs

---

## References

- KubeVirt user guide: Instancetypes and Preferences  
  https://kubevirt.io/user-guide/user_workloads/instancetypes/
- KubeVirt user guide: Creating VMs with instancetype/preference references  
  https://kubevirt.io/user-guide/user_workloads/creating_vms/
- [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md)
- [RFC-0012](./RFC-0012-kubevirt-advanced.md)
