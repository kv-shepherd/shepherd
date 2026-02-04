# ADR-0004: Provider Interface Design

> **Status**: Accepted  
> **Date**: 2026-01-14  
> **Decision**: Option C - KubeVirt-Specific Interface

---

## Decision

Phase 1 defines a basic `InfrastructureProvider` interface with CRUD operations, but Phase 2's `KubeVirtProvider` needs to implement many additional features (Snapshot, Clone, Migration, Instancetype, etc.).

Current interface definition:

```go
type InfrastructureProvider interface {
    ProviderID() string
    ProviderName() string
    ValidateSpec(ctx context.Context, spec ResourceSpec) (ValidationResult, error)
    GetResource(ctx context.Context, clusterName, namespace, name string) (*ResourceStatus, error)
    ListResources(ctx context.Context, clusterName string, namespace *string) ([]ResourceStatusLight, error)
    CreateResource(ctx context.Context, spec ResourceSpec) (*ResourceStatus, error)
    UpdateResource(ctx context.Context, spec ResourceSpec) (*ResourceStatus, error)
    DeleteResource(ctx context.Context, clusterName, namespace, name string) error
    PerformAction(ctx context.Context, clusterName, namespace, name, action string) (*ResourceStatus, error)
}
```

---

## Options Considered

### Option A: Extended Single Interface

Add all features to `InfrastructureProvider` interface.

**Pros**:
- Simple and direct
- One interface covers all functionality
- Easy MockProvider implementation

**Cons**:
- Interface too large, violates Interface Segregation Principle
- Providers not supporting certain features must implement empty methods

### Option B: Capability Interface Composition

Define multiple small interfaces, combine through composition.

**Pros**:
- Follows Interface Segregation Principle
- Flexible, Providers implement capabilities as needed
- Easy to extend new features

**Cons**:
- Requires type assertions
- Increases code complexity
- MockProvider needs to implement multiple interfaces

### Option C: KubeVirt-Specific Interface (Recommended)

Since this project is specifically for KubeVirt, define a KubeVirt-specific interface:

```go
// Base interface (for Mock/testing)
type InfrastructureProvider interface {
    ProviderID() string
    ProviderName() string
    ValidateSpec(...) (ValidationResult, error)
    GetResource(...) (*ResourceStatus, error)
    ListResources(...) ([]ResourceStatusLight, error)
    CreateResource(...) (*ResourceStatus, error)
    UpdateResource(...) (*ResourceStatus, error)
    DeleteResource(...) error
    PerformAction(...) (*ResourceStatus, error)
}

// KubeVirt-specific interface (extends base interface)
type KubeVirtProvider interface {
    InfrastructureProvider
    
    // Snapshots
    CreateVMSnapshot(...) (*SnapshotStatus, error)
    GetVMSnapshot(...) (*SnapshotStatus, error)
    ListVMSnapshots(...) ([]SnapshotStatus, error)
    DeleteVMSnapshot(...) error
    RestoreVMFromSnapshot(...) (*RestoreStatus, error)
    
    // Cloning
    CloneVM(...) (*CloneStatus, error)
    GetVMClone(...) (*CloneStatus, error)
    ListVMClones(...) ([]CloneStatus, error)
    
    // Migration
    MigrateVM(...) (*MigrationStatus, error)
    GetVMMigration(...) (*MigrationStatus, error)
    ListVMMigrations(...) ([]MigrationStatus, error)
    CancelVMMigration(...) error
    
    // Instance Types
    ListInstancetypes(...) ([]InstancetypeInfo, error)
    ListPreferences(...) ([]PreferenceInfo, error)
}
```

---

## Decision Rationale

1. **Governance core decoupled from business functions**: Base `InfrastructureProvider` interface remains stable, governance layer (approval, audit) only depends on this interface
2. **KubeVirt functions can evolve independently**: `KubeVirtProvider` interface can freely expand snapshot, clone, migration features
3. **Test-friendly**: Unit tests use simple `InfrastructureProvider` Mock, integration tests use complete `KubeVirtProvider` Mock

---

## MockProvider Impact

| Option | MockProvider Implementation |
|--------|----------------------------|
| A | Implement all methods, unsupported return `ErrNotSupported` |
| B | Implement different capability interfaces based on test needs |
| C | Implement full `KubeVirtProvider` for integration tests; implement `InfrastructureProvider` for unit tests |

---

## Consequences

### Positive

- ✅ Base interface stays concise for general testing
- ✅ KubeVirt-specific interface contains all functionality
- ✅ Service layer can choose which interface to use
- ✅ Future support for other virtualization platforms possible

### Negative

- 🟡 Two sets of interfaces needed
- 🟡 Some code requires type assertions

---

## References

- [01-contracts.md](../design/phases/01-contracts.md) - Interface definitions
- [02-providers.md](../design/phases/02-providers.md) - Provider implementation

---

## Amendments by Subsequent ADRs

> ⚠️ **Notice**: The following sections of this ADR have been amended by subsequent ADRs.
> The original decisions above remain **unchanged for historical reference**.
> When implementing, please refer to the amending ADRs for current design.

### ADR-0024: Provider Interface Capability Composition (2026-01-29)

| Original Section | Status | Amendment Details | See Also |
|------------------|--------|-------------------|----------|
| §Decision: `KubeVirtProvider` interface design | **REFINED** | `KubeVirtProvider` decomposed into capability interfaces (`SnapshotProvider`, `CloneProvider`, `MigrationProvider`, etc.) via embedding for improved testability | [ADR-0024](./ADR-0024-provider-interface-capability-composition.md) |

> **Implementation Guidance**: The base `InfrastructureProvider` interface remains valid. `KubeVirtProvider` now embeds multiple capability interfaces for granular testing. Service layer can depend on narrow interfaces (e.g., `SnapshotProvider` only) instead of the full `KubeVirtProvider`.
