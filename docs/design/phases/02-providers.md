# Phase 2: Provider Implementation

> **Prerequisites**: Phase 1 complete  
> **Acceptance**: KubeVirt Provider implements all interfaces, tests pass

### Required Deliverables from Phase 1

| Dependency | Location | Verification |
|------------|----------|--------------|
| Ent schemas generated | `ent/` | `go generate ./ent` succeeds |
| Provider interfaces defined | `internal/provider/interface.go` | Interfaces compile |
| Domain models | `internal/domain/` | `domain.VM`, `domain.Cluster` exist |
| Error system | `internal/pkg/errors/` | Error codes defined |
| DomainEvent schema | `ent/schema/domain_event.go` | Schema generated |

---

## Objectives

Implement infrastructure providers:

- KubeVirt Provider (production)
- Mock Provider (testing)
- Anti-Corruption Layer (K8s → Domain mapping)
- ResourceWatcher (List-Watch pattern)
- Cluster health checking
- Capability detection (ADR-0014)

> **⚠️ Interface Composition Constraint (ADR-0004, ADR-0024)**:
>
> Provider implementations MUST use **interface composition** pattern defined in [examples/provider/interface.go](../examples/provider/interface.go).
>
> | ADR | Requirement |
> |-----|-------------|
> | ADR-0004 | Provider interfaces must be composable (single responsibility per interface) |
> | ADR-0024 | Capability-based interface selection (providers expose only supported interfaces) |
>
> **Example**: `KubeVirtProvider` embeds `VMOperator + SnapshotOperator + MigrationOperator` based on cluster capabilities.

> **📖 Document Hierarchy (Prevents Content Drift)**:
>
> | Document | Authority | Scope |
> |----------|-----------|-------|
> | **ADRs** | Decisions (immutable after acceptance) | Architecture decisions and rationale |
> | **[master-flow.md](../interaction-flows/master-flow.md)** | Interaction principles (single source of truth) | Data sources, flow rationale, user journeys |
> | **Phase docs (this file)** | Implementation details | Code patterns, schemas, API design |
> | **[CHECKLIST.md](../CHECKLIST.md)** | ADR constraints reference | Centralized ADR enforcement rules |
>
> **Cross-Reference Pattern**: When describing "what data" and "why", link to master-flow. This document defines "how to implement".

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| KubeVirtProvider | `internal/provider/kubevirt.go` | ✅ | - |
| ClientInterface | `internal/provider/client.go` | ✅ | Anti-Corruption Layer |
| MockProvider | `internal/provider/mock.go` | ✅ | - |
| Domain models | `internal/domain/` | ✅ | [examples/domain/vm.go](../examples/domain/vm.go) |
| KubeVirtMapper | `internal/provider/mapper.go` | ✅ | - |
| ResourceWatcher | `internal/provider/watcher.go` | ⬜ | Deferred |
| ClusterHealthChecker | `internal/provider/health_checker.go` | ✅ | - |
| CapabilityDetector | `internal/provider/capability.go` | ✅ | - |

---

## 1. Anti-Corruption Layer

> **Reference**: [examples/domain/vm.go](../examples/domain/vm.go)

### Purpose

Isolate domain logic from K8s API changes:

```
KubeVirt API ──► KubeVirtMapper ──► Domain Model ──► Service Layer
                     ↑
            Defensive programming
            Nil checks
            Error extraction
```

### Mapping Rules

| K8s Type | Domain Type |
|----------|-------------|
| `kubevirtv1.VirtualMachine` | `domain.VM` |
| `kubevirtv1.VirtualMachineInstance` | (merged into domain.VM) |
| `snapshotv1.VirtualMachineSnapshot` | `domain.Snapshot` |

### Defensive Programming

```go
func (m *Mapper) MapVM(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) (*domain.VM, error) {
    // Critical fields must exist
    if vm.Name == "" || vm.Namespace == "" {
        return nil, ErrIncompatibleSchema
    }
    
    // Optional fields: nil checks
    var ip string
    if vmi != nil && len(vmi.Status.Interfaces) > 0 {
        ip = vmi.Status.Interfaces[0].IP
    }
    
    return &domain.VM{
        Name:      vm.Name,
        Namespace: vm.Namespace,
        IP:        ip,
        // ...
    }, nil
}
```

---

## 2. KubeVirt Provider

### Using Official Client

> **ADR-0001**: Use official `kubevirt.io/client-go` client.  
> **Version Tracking**: Client version is specified in [DEPENDENCIES.md](../DEPENDENCIES.md) as single source of truth.

```go
import "kubevirt.io/client-go/kubecli"

// Create typed client
virtClient, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)

// Use Informer for List-Watch
vmInformer := virtClient.VirtualMachine().Informer()
```

### VM Operations

| Operation | Method | Notes |
|-----------|--------|-------|
| Get VM | `GetVM(cluster, namespace, name)` | Returns domain.VM |
| List VMs | `ListVMs(cluster, namespace, opts)` | With pagination |
| Create VM | `CreateVM(cluster, namespace, spec)` | SSA Apply (ADR-0011) |
| Start/Stop | `StartVM`, `StopVM` | Power operations |
| Migrate | `MigrateVM` | Live migration |

---

## 3. ResourceWatcher

### List-Watch Pattern

```
Initial List → resourceVersion → Watch Events → Update Cache
                                       ↓
                              410 Gone? → Re-list
```

### 410 Gone Handling (Critical)

| Step | Action |
|------|--------|
| 1 | Clear `resourceVersion` (force full re-list) |
| 2 | Notify CacheService to mark cluster rebuilding |
| 3 | **Do not** count toward circuit breaker (410 is normal) |
| 4 | Read requests return stale data with `cache_status: STALE` |
| 5 | Write requests return 503 (strong consistency) |

### Circuit Breaker

| Parameter | Value |
|-----------|-------|
| Failure threshold | 5 consecutive |
| Breaker duration | 60 seconds |
| Recovery | Auto-attempt after duration |

---

## 4. Cluster Health Check

### Health Check Components

| Check | Frequency | Action on Failure |
|-------|-----------|-------------------|
| API Server connectivity | 60s | Mark UNREACHABLE |
| KubeVirt CRD exists | 60s | Mark UNHEALTHY |
| KubeVirt version | 60s | Log warning |

### Status Enum

| Status | Description |
|--------|-------------|
| UNKNOWN | Initial state |
| HEALTHY | Connection OK, KubeVirt installed |
| UNHEALTHY | Connection OK, KubeVirt issue |
| UNREACHABLE | Cannot connect |

---

## 5. Capability Detection (ADR-0014)

### Detection Sources

| Source | Data |
|--------|------|
| `ServerVersion().Get()` | KubeVirt version (e.g., `1.7.0`) |
| KubeVirt CR `featureGates` | Enabled feature gates |
| Static GA table | Features that became GA by version |

### Cluster Schema Extensions

```go
field.String("kubevirt_version"),
field.Strings("enabled_features"),
field.Time("capabilities_detected_at"),
field.JSON("hardware_capabilities", map[string]bool{}), // Auto-detected during health check (ADR-0014)
```

### Dry Run Fallback (ADR-0014)

> **Compatibility Validation**: When static capability detection is insufficient,
> use `DryRunAll` to validate resource creation without actual execution.
>
> | Strategy | Use Case | Implementation |
> |----------|----------|----------------|
> | **Static Detection** | Known feature gates (e.g., GPU, Hugepages) | Query `ServerVersion()` + `featureGates` |
> | **Dry Run Fallback** | Unknown/edge-case capabilities | `client.Create(ctx, vm, client.DryRunAll)` |
>
> **Note**: Dry run requires `controller-runtime v0.22.4+` with `DryRunAll` support.
> See ADR-0014 §Runtime Validation for implementation details.

### Template Matching

> **Updated per ADR-0018**: Capability requirements are now stored in InstanceSize, not Template.

```go
// FilterCompatibleClusters returns clusters that support the given InstanceSize requirements
// Note: RequiredCapabilities moved from Template to InstanceSize per ADR-0018
func FilterCompatibleClusters(clusters []Cluster, instanceSize InstanceSize) []Cluster {
    var result []Cluster
    for _, c := range clusters {
        if hasAllCapabilities(c.Capabilities, instanceSize.RequiredCapabilities) {
            result = append(result, c)
        }
    }
    return result
}

// hasAllCapabilities checks if cluster has all required capabilities from InstanceSize
func hasAllCapabilities(clusterCaps map[string]bool, required []string) bool {
    for _, cap := range required {
        if !clusterCaps[cap] {
            return false
        }
    }
    return true
}
```

> **See Also**: [ADR-0018 §Cluster Capability Matching](../../adr/ADR-0018-instance-size-abstraction.md#cluster-capability-matching)

## 6. Schema Cache Lifecycle (ADR-0023)

> **Purpose**: KubeVirt Schema caching enables offline validation, multi-version compatibility, and frontend type generation.

### Cache Lifecycle

| Stage | Trigger | Action |
|-------|---------|--------|
| **1. Startup** | Application boot | Load embedded schemas (bundled at compile time) |
| **2. Cluster Registration** | New cluster added | Detect KubeVirt version → check cache → queue fetch if missing |
| **3. Version Detection** | Health check loop (60s) | Piggyback: compare `clusters.kubevirt_version` with detected version |
| **4. Schema Update** | Version change detected | Queue `SchemaUpdateJob` (River) → async fetch → cache update |

### Implementation Integration

- **ClusterHealthChecker**: Detects version during health check, triggers schema update if mismatch
- **SchemaUpdateJob**: River job that fetches and caches OpenAPI schema from cluster
- **Embedded Fallback**: Bundled schemas for common KubeVirt versions (compile-time)

### Expiration Policy

Schemas are **immutable per version** (v1.5.0 never changes). Cache indefinitely; update only on version change.

### Graceful Degradation

If schema fetch fails → use embedded fallback → retry on next health check cycle.

> **Frontend Fallback Strategy**: When schema cache fails or version drifts, the Schema-Driven UI provides a Fallback UI Mode with basic fields only. See [master-flow.md §Frontend Schema Fallback Strategy](../interaction-flows/master-flow.md#schema-cache-lifecycle-adr-0023) for detailed UI behavior, alert integration, and implementation notes.

> **See Also**: [ADR-0023 §1 Schema Cache](../../adr/ADR-0023-schema-cache-and-api-standards.md#1-schema-cache-management-policy), [master-flow.md §Schema Cache Lifecycle](../interaction-flows/master-flow.md#schema-cache-lifecycle-adr-0023)

## 7. Resource Adoption (V1 Minimal Compensation)

> **Decision Reference**: [ADR-0015 §12](../../adr/ADR-0015-governance-model-v2.md#12-resource-adoption-rules)
>
> Adoption is a **recovery capability** for rare inconsistencies (e.g., K8s create succeeded but DB write failed).
> It is not a full reconciliation framework in V1.

### V1 Flow

```
Periodic Scan
  → Find K8s VMs with Shepherd labels but missing DB record
  → Check adoption criteria (valid Service association)
  → Write adoptable items to pending_adoptions

Admin Review
  → Adopt (create DB record) OR Ignore
```

### Adoption Criteria (ADR-0015 §12)

| Condition | Adoptable | Action |
|-----------|-----------|--------|
| Has `kubevirt-shepherd.io/service` label and Service exists in DB | ✅ Yes | Add to pending list |
| Shepherd labels exist but Service missing | ❌ No | Ignore as orphan; manual kubectl cleanup if needed |
| No Shepherd labels | ❌ No | Not platform-managed |

### PendingAdoption Fields

| Field | Type | Purpose |
|-------|------|---------|
| `cluster_name` | string | Resource location |
| `namespace` | string | K8s namespace |
| `system`, `service`, `instance` | string | Governance identifiers from labels |
| `k8s_uid` | string | K8s resource UID |
| `resource_spec` | JSON | Snapshot for admin review |
| `status` | enum | PENDING, ADOPTED, IGNORED |

### Admin APIs

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/admin/pending-adoptions` | List pending adoptable resources |
| `POST .../adopt` | Confirm adoption into DB |
| `POST .../ignore` | Ignore item (no DB record created) |

### V1 Constraints (Intentional)

- No automatic adoption.
- No bulk conflict resolution.
- No automatic deletion of non-adoptable resources.
- No cross-cluster deduplication logic beyond label + Service existence checks.

---

## 8. MockProvider

For testing without K8s cluster:

```go
type MockProvider struct {
    vms      map[string]*domain.VM
    mu       sync.RWMutex
}

func (p *MockProvider) Seed(vms []*domain.VM) { ... }
func (p *MockProvider) Reset() { ... }
```

---

## Acceptance Criteria

- [ ] KubeVirtProvider implements all interfaces
- [ ] MockProvider matches KubeVirtProvider interface
- [ ] MapVM handles nil fields correctly
- [ ] ResourceWatcher 410 handling tested
- [x] Health check updates cluster status
- [x] Capability detector runs on health check
- [ ] Adoption discovery works

---

## Related Documentation

- [examples/domain/vm.go](../examples/domain/vm.go) - Domain models
- [examples/provider/interface.go](../examples/provider/interface.go) - Interfaces (ADR-0024: capability interface composition)
- [ADR-0001](../../adr/ADR-0001-kubevirt-client.md) - KubeVirt Client
- [ADR-0011](../../adr/ADR-0011-ssa-apply-strategy.md) - SSA Apply
- [ADR-0014](../../adr/ADR-0014-capability-detection.md) - Capability Detection
- [ADR-0024](../../adr/ADR-0024-provider-interface-capability-composition.md) - Provider Capability Interface Composition
