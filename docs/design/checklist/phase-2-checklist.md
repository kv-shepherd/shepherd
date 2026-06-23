# Phase 2 Checklist: Provider Implementation

> **Detailed Document**: [phases/02-providers.md](../phases/02-providers.md)
>
> **Implementation Status**: 🔄 Partial (~73%) — Basic VM CRUD + SSAApplier + VMRenderer + AuthProvider Admin + KubeVirt instance type/preference catalog reads + label-based pending adoption discovery/periodic scan + admin adoption management, bounded provider K8s operation timeouts, provider unit test lane, V1 River queue concurrency, i18n, and Atlas baselines done; Snapshot/Clone/Migration deferred. V1 status sync baseline is ADR-0038 adaptive polling; optional ResourceWatcher acceleration remains deferred

---

## Anti-Corruption Layer

- [x] **Domain Model Definition** (`internal/domain/`):
  - [x] `vm.go` - VM domain model (decoupled from K8s VirtualMachine)
  - [x] `snapshot.go` - Snapshot domain model (in vm.go)
  - [x] `VMStatus` internal enum (CREATING, STARTING, RUNNING, STOPPING, STOPPED, DELETING, FAILED, PENDING as K8s scheduler wait, MIGRATING, PAUSED, UNKNOWN, NOT_FOUND)
- [x] **KubeVirtMapper** (`internal/provider/mapper.go`):
  - [x] `MapVM()` - Maps VirtualMachine + VMI to `domain.VM`
  - [x] `MapSnapshot()` - Maps VirtualMachineSnapshot to `domain.VMSnapshot`
  - [x] `MapVMList()` - Batch mapping with VMI lookup optimization
  - [x] **Defensive Programming**: All pointer fields must check nil
  - [x] **Error Extraction**: Extract from Status.PrintableStatus and Conditions
- [x] **Provider Integration**: All methods return `domain.*` types

> ✅ **Master-Flow Alignment Resolved (audited 2026-06-19)**:
> `domain/vm.go` now uses `FAILED` instead of `ERROR` and includes `STOPPING`,
> `STARTING`, and `NOT_FOUND`. `PENDING` remains in the VM status enum only as a
> K8s/KubeVirt scheduler-wait state; it is not used to model pre-approval VM
> requests, because Stage 5.A creates a pending Ticket/DomainEvent without a VM row.

---

## VM Basic Operations

- [x] Using `kubevirt.io/api` types + custom client interface (kubecli bound at composition root)
- [x] `GetVM`, `ListVMs`, `CreateVM`, `UpdateVM`, `DeleteVM` implemented
- [x] `StartVM`, `StopVM`, `RestartVM`, `PauseVM`, `UnpauseVM` implemented
- [x] VMI queries (via `VirtualMachineInstanceClient` interface)

---

## VM Snapshot Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (scheduled backup, retention policies) are defined in [RFC-0013](../../rfc/RFC-0013-vm-snapshot.md).

- [ ] `CreateVMSnapshot` create snapshot (RFC-backed future scope; not V1 runtime)
- [ ] `GetVMSnapshot`, `ListVMSnapshots` query snapshots (RFC-backed future scope; not V1 runtime)
- [ ] `DeleteVMSnapshot` delete snapshot (RFC-backed future scope; not V1 runtime)
- [ ] `RestoreVMFromSnapshot` restore from snapshot (RFC-backed future scope; not V1 runtime)

---

## VM Clone Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (data masking, cross-cluster clone) are defined in [RFC-0014](../../rfc/RFC-0014-vm-clone.md).

- [ ] `CloneVM` clone from VM (RFC-backed future scope; not V1 runtime)
- [ ] Support cloning from snapshot (RFC-backed future scope; not V1 runtime)
- [ ] `GetVMClone`, `ListVMClones` status query (RFC-backed future scope; not V1 runtime)

---

## VM Migration Operations (Provider-Level)

> **Scope**: Basic Provider CRUD methods only. Advanced features (automated migration policies, maintenance mode) are defined in [RFC-0012](../../rfc/RFC-0012-kubevirt-advanced.md).

- [ ] `MigrateVM` initiate migration (RFC-backed future scope; not V1 runtime)
- [ ] `GetVMMigration`, `ListVMMigrations` status query (RFC-backed future scope; not V1 runtime)
- [ ] `CancelVMMigration` cancel migration (RFC-backed future scope; not V1 runtime)

---

## Instance Types and Preferences

- [x] `ListInstancetypes` list namespace-scoped instance types
- [x] `ListClusterInstancetypes` list cluster-level instance types
- [x] `ListPreferences` list namespace-scoped and cluster-level preferences

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

## ResourceWatcher (V2 Optional Acceleration)

> **Best-practice rule**:
> V1 keeps **one authoritative status sync path**: ADR-0038 adaptive polling with
> `ResourceVersion` caching. A future watch/informer path, if added, must be an
> optional acceleration layer with polling/reconcile fallback, not a second
> authoritative pipeline.
>
> **Tracking**: Future watch acceleration is documented in
> [RFC-0020](../../rfc/RFC-0020-k8s-watch-acceleration.md).

- [ ] List-Watch pattern implemented (RFC-backed future scope; deferred V2 acceleration)
- [ ] **410 Gone Complete Handling** (RFC-backed future scope; deferred V2 acceleration):
  - [ ] Clear `resourceVersion` (force full Re-list; RFC-backed future scope)
  - [ ] Notify `CacheService` to invalidate cache (RFC-backed future scope)
  - [ ] Don't count toward circuit breaker (RFC-backed future scope)
  - [ ] **Read Request Degradation Strategy** implemented (RFC-backed future scope)
- [ ] Exponential backoff reconnect (with jitter; RFC-backed future scope)
- [ ] Circuit breaker configured (RFC-backed future scope)

---

## Cluster Health Check

- [x] **ClusterHealthChecker** implemented (`internal/provider/health_checker.go`)
- [x] **Health Check Logic** complete (periodic + on-demand)
- [x] **Status Enum** defined (UNKNOWN, HEALTHY, UNHEALTHY, UNREACHABLE)
- [x] **App Lifecycle Wiring** complete (`internal/app/lifecycle.go` starts loop and persists `clusters.status`)

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

- [x] **Discovery Mechanism** (Label-based only) implemented
- [x] **PendingAdoption Table** schema complete
- [x] **Admin API** for adoption management
  - [x] List pending adoption resources with typed/validated V1 `VirtualMachine` resource-type filtering
  - [x] Reject pending adoption resources
  - [x] Adopt pending resources into VM DB records
- [x] **Periodic Scan** configured
- [x] **Audit Log** for adoption operations
  - [x] Rejection audit events
  - [x] Adoption execution audit events

---

## General

- [x] **Concurrency Control** baseline — V1 uses River queue `MaxWorkers`; per-cluster queue-wait/semaphore remains deferred to [RFC-0015](../../rfc/RFC-0015-per-cluster-concurrency.md)
- [x] Context timeout handling — provider-owned K8s operations and cluster health probes use bounded `k8s.operation_timeout` contexts, enforced by `shepherd-arch/k8stimeout`
- [ ] Cache service (Ent local query, no Redis) — deferred; CacheService-based `CLUSTER_REBUILDING` UX is tracked in [DEFERRED_FOLLOWUPS.md](../DEFERRED_FOLLOWUPS.md)
- [x] i18n Standards verified — frontend non-English literal and repository Chinese-character allowlist checks pass, with en/zh-CN locale catalogs as the approved i18n boundary

---

## Approval Protocol Skeleton

- [x] Approval ticket data model (Ent schema)
- [x] Approval policy data model (Ent schema)
- [x] State machine definition (PENDING → APPROVED/REJECTED/CANCELLED)
- [x] Interface definitions (`ApprovalProvider` in `internal/provider/approvalcontract/contract.go`, thin re-export in `internal/provider/approval.go`)
- [x] Database migration scripts (Atlas — Phase 4) — `migrations/atlas/atlas.hcl`, checked-in Atlas SQL, and startup migration tests are present

---

## Pre-Phase 3 Verification

- [x] KubeVirtProvider unit tests pass (using fake/mock client interfaces) — verified 2026-06-19 with `go test -count=1 ./internal/provider`
- [ ] ResourceWatcher `410 Gone` handling test passes — only required if optional watch accelerator is introduced later
- [ ] Mapper defensive code test coverage > 80% — deferred
- [x] `go vet ./...` passes
- [x] `go build ./...` passes
- [x] `go test -race ./...` passes
