# Phase 2 Checklist: Provider Implementation

> **Detailed Document**: [phases/02-providers.md](../phases/02-providers.md)
>
> **Implementation Status**: 🔄 Partial (~50%) — Basic VM CRUD + mapper done, Snapshot/Clone/Migration/ResourceWatcher deferred

---

## Anti-Corruption Layer

- [x] **Domain Model Definition** (`internal/domain/`):
  - [x] `vm.go` - VM domain model (decoupled from K8s VirtualMachine)
  - [x] `snapshot.go` - Snapshot domain model (in vm.go)
  - [x] `VMStatus` internal enum (PENDING, RUNNING, STOPPED, FAILED, MIGRATING)
- [x] **KubeVirtMapper** (`internal/provider/mapper.go`):
  - [x] `MapVM()` - Maps VirtualMachine + VMI to `domain.VM`
  - [x] `MapSnapshot()` - Maps VirtualMachineSnapshot to `domain.VMSnapshot`
  - [x] `MapVMList()` - Batch mapping with VMI lookup optimization
  - [x] **Defensive Programming**: All pointer fields must check nil
  - [x] **Error Extraction**: Extract from Status.PrintableStatus and Conditions
- [x] **Provider Integration**: All methods return `domain.*` types

> ⚠️ **Master-Flow Alignment Issue (P0, audited 2026-02-10)**:
> `domain/vm.go` declares `VMStatusError = "ERROR"` but master-flow state diagram uses `FAILED`.
> Also missing `STOPPING` transitional state. `PENDING` status should not exist at VM domain level
> (VM row is not created until approval per master-flow Stage 5.A).
> **Fix**: Rename `ERROR` → `FAILED`, add `STOPPING`, remove domain-level `PENDING`.

---

## VM Basic Operations

- [x] Using `kubevirt.io/api` types + custom client interface (kubecli bound at composition root)
- [x] `GetVM`, `ListVMs`, `CreateVM`, `UpdateVM`, `DeleteVM` implemented
- [x] `StartVM`, `StopVM`, `RestartVM`, `PauseVM`, `UnpauseVM` implemented
- [x] VMI queries (via `VirtualMachineInstanceClient` interface)

---

## VM Snapshot Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (scheduled backup, retention policies) are defined in [RFC-0013](../../rfc/RFC-0013-vm-snapshot.md).

- [ ] `CreateVMSnapshot` create snapshot
- [ ] `GetVMSnapshot`, `ListVMSnapshots` query snapshots
- [ ] `DeleteVMSnapshot` delete snapshot
- [ ] `RestoreVMFromSnapshot` restore from snapshot

---

## VM Clone Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (data masking, cross-cluster clone) are defined in [RFC-0014](../../rfc/RFC-0014-vm-clone.md).

- [ ] `CloneVM` clone from VM
- [ ] Support cloning from snapshot
- [ ] `GetVMClone`, `ListVMClones` status query

---

## VM Migration Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (automated migration policies, maintenance mode) are defined in [RFC-0012](../../rfc/RFC-0012-kubevirt-advanced.md).

- [ ] `MigrateVM` initiate migration
- [ ] `GetVMMigration`, `ListVMMigrations` status query
- [ ] `CancelVMMigration` cancel migration

---

## Instance Types and Preferences

- [ ] `ListInstancetypes` list instance types
- [ ] `ListClusterInstancetypes` list cluster-level instance types
- [ ] `ListPreferences` list preferences

---

## Provider Interface Capability Composition (ADR-0024)

> **Purpose**: Ensure provider interfaces follow capability interface segregation for testability.
> **Reference**: [examples/provider/interface.go](../examples/provider/interface.go)

- [x] **Capability interfaces defined** (`internal/provider/interface.go`):
  - [x] `InfrastructureProvider` - Base VM lifecycle
  - [x] `SnapshotProvider` - Snapshot operations
  - [x] `CloneProvider` - Clone operations
  - [x] `MigrationProvider` - Migration operations
  - [x] `InstanceTypeProvider` - Instance type queries
  - [x] `ConsoleProvider` - Console access
- [x] **`KubeVirtProvider` embeds all capability interfaces** (Code Review enforcement)
- [x] **Service layer depends on narrow interfaces** (e.g., `SnapshotProvider` only when only snapshot is needed)
- [x] ❌ **No monolithic interface dependencies** - avoid depending on full `KubeVirtProvider` when a narrow interface suffices

---

## MockProvider

- [x] Interface identical to `KubeVirtProvider`
- [x] In-memory storage implementation
- [x] Supports `Seed()` and `Reset()` test methods

---

## ResourceWatcher

- [ ] List-Watch pattern implemented
- [ ] **410 Gone Complete Handling**:
  - [ ] Clear `resourceVersion` (force full Re-list)
  - [ ] Notify `CacheService` to invalidate cache
  - [ ] Don't count toward circuit breaker
  - [ ] **Read Request Degradation Strategy** implemented
- [ ] Exponential backoff reconnect (with jitter)
- [ ] Circuit breaker configured

---

## Cluster Health Check

- [x] **ClusterHealthChecker** implemented (`internal/provider/health_checker.go`)
- [x] **Health Check Logic** complete (periodic + on-demand)
- [x] **Status Enum** defined (UNKNOWN, HEALTHY, UNHEALTHY, UNREACHABLE)

---

## Cluster Capability Detection (ADR-0014)

- [x] **CapabilityDetector Implementation** complete (`internal/provider/capability.go`)
- [x] **Cluster Schema Extensions** added
- [x] **InstanceSize Capability Requirements** verified (capabilities moved from Template to InstanceSize per ADR-0018)
- [x] **HasAllCapabilities** for cluster-instancesize matching implemented
- [x] **Health Check Integration** working (piggybacks on health check cycle)
- [x] **Dry run fallback** implemented (ValidateSpec with DryRunAll)

---

## Resource Adoption Security

- [ ] **Discovery Mechanism** (Label-based only) implemented
- [x] **PendingAdoption Table** schema complete
- [ ] **Admin API** for adoption management
- [ ] **Periodic Scan** configured
- [ ] **Audit Log** for adoption operations

---

## General

- [ ] **Concurrency Control** with queue-wait mechanism
- [ ] Context timeout handling
- [ ] Cache service (Ent local query, no Redis)
- [ ] i18n Standards verified

---

## Approval Protocol Skeleton

- [x] Approval ticket data model (Ent schema)
- [x] Approval policy data model (Ent schema)
- [x] State machine definition (PENDING → APPROVED/REJECTED/CANCELLED)
- [x] Interface definitions (`ApprovalProvider` in `internal/provider/auth.go`)
- [ ] Database migration scripts (Atlas — Phase 4)

---

## Pre-Phase 3 Verification

- [ ] KubeVirtProvider unit tests pass (using Mock Client) — requires testcontainers
- [ ] ResourceWatcher `410 Gone` handling test passes — deferred
- [ ] Mapper defensive code test coverage > 80% — deferred
- [x] `go vet ./...` passes
- [x] `go build ./...` passes
- [x] `go test -race ./...` passes
