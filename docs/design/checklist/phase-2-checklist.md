# Phase 2 Checklist: Provider Implementation

> **Detailed Document**: [phases/02-providers.md](../phases/02-providers.md)

---

## Anti-Corruption Layer

- [ ] **Domain Model Definition** (`internal/domain/`):
  - [ ] `vm.go` - VM domain model (decoupled from K8s VirtualMachine)
  - [ ] `snapshot.go` - Snapshot domain model
  - [ ] `VMStatus` internal enum (PENDING, RUNNING, STOPPED, FAILED, MIGRATING)
- [ ] **KubeVirtMapper** (`internal/provider/mapper.go`):
  - [ ] `MapVM()` - Maps VirtualMachine + VMI to `domain.VM`
  - [ ] `MapSnapshot()` - Maps VirtualMachineSnapshot to `domain.VMSnapshot`
  - [ ] `MapVMList()` - Batch mapping with VMI lookup optimization
  - [ ] **Defensive Programming**: All pointer fields must check nil
  - [ ] **Error Extraction**: Extract from Status.PrintableStatus and Conditions
- [ ] **Provider Integration**: All methods return `domain.*` types

---

## VM Basic Operations

- [ ] Using `kubevirt.io/client-go` official client
- [ ] `GetVM`, `ListVMs`, `CreateVM`, `UpdateVM`, `DeleteVM` implemented
- [ ] `StartVM`, `StopVM`, `RestartVM`, `PauseVM`, `UnpauseVM` implemented
- [ ] VMI queries (`GetVMI`, `ListVMIs`)

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

## MockProvider

- [ ] Interface identical to `KubeVirtProvider`
- [ ] In-memory storage implementation
- [ ] Supports `Seed()` and `Reset()` test methods

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

- [ ] **ClusterHealthChecker** implemented
- [ ] **Health Check Logic** complete
- [ ] **Status Enum** defined (UNKNOWN, HEALTHY, UNHEALTHY, UNREACHABLE)

---

## Cluster Capability Detection (ADR-0014)

- [ ] **CapabilityDetector Implementation** complete
- [ ] **Cluster Schema Extensions** added
- [ ] **Template Schema Extensions** added
- [ ] **ClusterCompatibilityService** implemented
- [ ] **Health Check Integration** working
- [ ] **Dry run fallback** implemented

---

## Resource Adoption Security

- [ ] **Discovery Mechanism** (Label-based only) implemented
- [ ] **PendingAdoption Table** schema complete
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

- [ ] Approval ticket data model
- [ ] Approval policy data model
- [ ] State machine definition
- [ ] Interface definitions
- [ ] Database migration scripts

---

## Pre-Phase 3 Verification

- [ ] KubeVirtProvider unit tests pass (using Mock Client)
- [ ] ResourceWatcher `410 Gone` handling test passes
- [ ] Mapper defensive code test coverage > 80%
