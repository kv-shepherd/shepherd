---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-29
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0024: Provider Interface Capability Composition

> **Review Period**: Until 2026-02-01 (48-hour minimum)  
> **Discussion**: [Issue #56](https://github.com/kv-shepherd/shepherd/issues/56)  
> **Amends**: [ADR-0004 §Decision](./ADR-0004-provider-interface.md)

---

## Context and Problem Statement

[ADR-0004](./ADR-0004-provider-interface.md) established the provider interface design with Option C: a base `InfrastructureProvider` interface and an extended `KubeVirtProvider` interface containing all KubeVirt-specific methods.

During implementation, we identified opportunities to improve:

1. **Testability**: Large interfaces require mocking all methods, even when testing a single feature
2. **Go Best Practice Alignment**: Go community consensus (2025) strongly favors small, composable interfaces
3. **Feature Isolation**: Snapshot, Clone, and Migration capabilities have distinct lifecycles and may not all be available on all clusters

**Question**: How should we refine the provider interface design to improve testability and align with Go best practices while maintaining the ADR-0004 principle of KubeVirt-specific functionality?

---

## Decision Drivers

* **Go Interface Segregation**: "Accept interfaces, return concrete types" and prefer small interfaces
* **Testability**: Service layer should depend on narrow interfaces for focused unit tests
* **ADR-0004 Compatibility**: Maintain backward compatibility with existing design intent
* **Capability Detection**: ADR-0014 requires detecting cluster capabilities; interfaces should reflect this

---

## Considered Options

* **Option 1**: Keep ADR-0004 as-is (monolithic `KubeVirtProvider`)
* **Option 2**: Decompose into capability interfaces with embedding

---

## Decision Outcome

**Chosen option**: "Option 2: Decompose into capability interfaces with embedding", because it improves testability, aligns with Go best practices, and maintains ADR-0004 compatibility through interface embedding.

### Implementation

```go
// Capability interfaces (focused, testable)
type SnapshotProvider interface {
    CreateSnapshot(ctx, cluster, namespace, vmName, snapshotName string) (*domain.Snapshot, error)
    GetSnapshot(ctx, cluster, namespace, name string) (*domain.Snapshot, error)
    ListSnapshots(ctx, cluster, namespace, vmName string) ([]*domain.Snapshot, error)
    DeleteSnapshot(ctx, cluster, namespace, name string) error
    RestoreFromSnapshot(ctx, cluster, namespace, snapshotName, targetVMName string) (*domain.VM, error)
}

type CloneProvider interface {
    CloneVM(ctx, cluster, namespace, sourceVM, targetName string) (*domain.VM, error)
    CloneFromSnapshot(ctx, cluster, namespace, snapshotName, targetName string) (*domain.VM, error)
    GetClone(ctx, cluster, namespace, name string) (*domain.Clone, error)
    ListClones(ctx, cluster, namespace string) ([]*domain.Clone, error)
}

type MigrationProvider interface {
    MigrateVM(ctx, cluster, namespace, name string) (*domain.Migration, error)
    GetMigration(ctx, cluster, namespace, name string) (*domain.Migration, error)
    ListMigrations(ctx, cluster, namespace string) ([]*domain.Migration, error)
    CancelMigration(ctx, cluster, namespace, name string) error
}

type InstanceTypeProvider interface {
    ListInstanceTypes(ctx, cluster, namespace string) ([]*domain.InstanceType, error)
    ListClusterInstanceTypes(ctx, cluster string) ([]*domain.InstanceType, error)
    ListPreferences(ctx, cluster, namespace string) ([]*domain.Preference, error)
    ListClusterPreferences(ctx, cluster string) ([]*domain.Preference, error)
}

type ConsoleProvider interface {
    GetVNCConnection(ctx, cluster, namespace, name string) (*domain.ConsoleConnection, error)
    GetSerialConsole(ctx, cluster, namespace, name string) (*domain.ConsoleConnection, error)
}

// KubeVirtProvider composes all capabilities via embedding
// This maintains ADR-0004 compatibility while enabling granular testing
type KubeVirtProvider interface {
    InfrastructureProvider  // Base VM lifecycle (ADR-0004)
    SnapshotProvider        // Snapshot capability
    CloneProvider           // Clone capability
    MigrationProvider       // Migration capability
    InstanceTypeProvider    // Instance type capability
    ConsoleProvider         // Console access capability
}
```

### Consequences

* ✅ Good, because service layer can depend on narrow interfaces (e.g., `SnapshotProvider` only)
* ✅ Good, because MockProvider can implement individual capabilities for focused tests
* ✅ Good, because `KubeVirtProvider` still provides complete interface when needed
* ✅ Good, because aligns with Go interface composition best practice (2025 consensus)
* 🟡 Neutral, because adds more interface definitions
* ❌ Bad, because migration from existing code requires updating type assertions (one-time cost)

### Confirmation

* CI validates that `KubeVirtProvider` embeds all capability interfaces
* Service layer tests demonstrate dependency on narrow interfaces
* Code review checklist includes interface segregation verification

---

## Pros and Cons of the Options

### Option 1: Keep ADR-0004 as-is

* ✅ Good, because no change required
* ❌ Bad, because large `KubeVirtProvider` requires mocking 15+ methods for any test
* ❌ Bad, because violates Go Interface Segregation Principle

### Option 2: Decompose with embedding (Chosen)

* ✅ Good, because testability significantly improves
* ✅ Good, because backward compatible (KubeVirtProvider still exists)
* ✅ Good, because matches ADR-0014 capability detection model
* 🟡 Neutral, because adds ~50 lines of interface definitions

---

## More Information

### Related Decisions

* [ADR-0004](./ADR-0004-provider-interface.md) - Original provider interface design (this ADR amends §Decision)
* [ADR-0014](./ADR-0014-capability-detection.md) - Capability detection pattern
* [ADR-0022](./ADR-0022-modular-provider-pattern.md) - Modular DI pattern (orthogonal concern)

### References

* [Go Interface Best Practices 2025](https://leapcell.io/articles/go-interface-best-practices) - Small interface advocacy
* [Uber Go Style Guide - Interfaces](https://github.com/uber-go/guide/blob/master/style.md) - Consumer-defined interfaces
* [examples/provider/interface.go](../design/examples/provider/interface.go) - Reference implementation

### Implementation Notes

Should be reviewed if:
- New provider types beyond KubeVirt are introduced
- Capability detection model changes significantly

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-29 | @jindyzhao | Initial draft based on implementation review |

---

_End of ADR-0024_
