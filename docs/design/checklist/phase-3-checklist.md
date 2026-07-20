# Phase 3 Checklist: Service Layer Integration

> **Detailed Document**: [phases/03-service-layer.md](../phases/03-service-layer.md)
>
> **Implementation Status**: 🔄 Partial (~89%) — Core DI/UseCase + ADR-0012 atomic path + sqlc + InstanceSize handler + analyzer-backed DI/sqlc CI gates done; River worker concurrency, V1 degradation baseline, VMService create `AlreadyExists` idempotency, DB-backed VM API CRUD handler success paths, and DB-backed VMService create approval/worker e2e done; per-cluster semaphore/degradation UX deferred

---

## Dependency Injection (Strict Manual DI)

- [x] **Composition Root Created**:
  - [x] `internal/app/bootstrap.go` created (Phase 0)
  - [x] All dependency assembly centralized in this file
  - [x] Layered construction: Infrastructure → Repository → Service → UseCase → Handler
- [x] **CI Check**:
  - [x] `shepherd-arch/manualdi` analyzer active in the golangci-lint gate
  - [x] `docs/design/ci/scripts/check_manual_di.sh` retained as legacy reference only
  - [x] Forbidden to wire Service/Repository structs outside `internal/app/`
  - [x] Forbidden to call constructor-style Service/Repository/UseCase/Gateway/Sender wiring outside the composition root
  - [x] Wire and Redis runtime imports blocked by analyzer-backed architecture lint
- [x] **Blocking Standards**:
  - [x] Constructor-style dependency wiring centralized in `internal/app/` module/bootstrap wiring
  - [x] Dependencies explicitly injected via constructors
  - [x] Global dependency containers remain prohibited by review and analyzer-backed wiring constraints
  - [x] Dependency initialization in `init()` remains prohibited; registration-only `init()` is allowed where documented by package purpose
- [x] Provider factory functions centralized in infrastructure/module wiring
- [x] Repository factory functions centralized in infrastructure/module wiring
- [x] Service dependencies injected via constructors

---

## Service Layer Standards

- [x] ❌ Service layer must not directly manage transactions
- [x] Service receives `*ent.Client` parameter (in-transaction or not)
- [x] ❌ K8s API calls forbidden inside transactions
- [x] ✅ Uses DB/K8s two-phase execution pattern (ADR-0012, not ADR-0010 deprecated approach)

---

## Governance Model Operation Standards

- [x] **Operation Approval Matrix**:
  - [x] Create System: **No approval required** (user self-service)
  - [x] Create Service: **No approval required** (user self-service)
  - [x] Create VM: **Approval required** (consumes resources)
  - [x] Delete System: No approval, but must have no child Services
  - [x] Delete Service: No approval, but must have no child VMs
- [x] **VM Request Flow Implementation** complete
- [x] **Hierarchical Delete Constraint (Delete Restrict)** implemented (SystemHandler checks child services)

---

## UseCase Layer Standards (Clean Architecture)

- [x] `internal/usecase/` directory created
- [x] `CreateVMUseCase` implementation complete (`internal/usecase/create_vm.go`)
- [x] **UseCase Reusability** verified (HTTP, CLI, gRPC, Cron)
- [x] **Handler Simplification** enforced (handlers delegate to usecases)

---

## Transaction Integration (ADR-0012 Hybrid Atomic Transaction)

- [x] **sqlc Configuration and Code Generation** complete (`internal/repository/sqlc/`)
- [x] **DatabaseClients Shared Pool** implemented
- [x] **CreateVMAtomicUseCase Implementation** complete (`internal/usecase/approval_atomic.go`)
- [x] **CI Block: sqlc Usage Scope Check** active (`check_sqlc_usage.sh`)
- [x] **Instance allocation concurrency** implemented — VM instance numbers are allocated through sqlc `AllocateServiceInstance` (`UPDATE services ... RETURNING`) inside the approval transaction, avoiding an application-side lock-key helper.

---

## Concurrency Control

- [x] **River Worker Concurrency Control** configured (`river.max_workers` + queue-specific `MaxWorkers`)
- [x] `river.max_workers` validation rejects invalid queue worker counts before River client startup
- [x] **HPA Constraint Verification** documented in [DEPENDENCIES.md](../DEPENDENCIES.md#hpa-concurrency-constraints-required)
- [ ] **ResizableSemaphore Implementation** deferred ([RFC-0015](../../rfc/RFC-0015-per-cluster-concurrency.md))
- [ ] **ClusterSemaphoreManager** deferred ([RFC-0015](../../rfc/RFC-0015-per-cluster-concurrency.md))
- [ ] **Hot-Reload Integration** deferred with per-cluster concurrency quotas
- [x] HTTP middleware-level K8s concurrency gate is not a V1 runtime control; K8s writes are bounded at the River worker layer

---

## VMService Refactoring

- [x] `ValidateAndPrepare()` method (outside transaction)
- [x] `CreateVMRecord()` — via CreateVMUseCase atomic transaction (DomainEvent + Ticket)
- [x] `ExecuteK8sCreate()` method (outside transaction)
  - [x] **Idempotency**: Kubernetes `AlreadyExists` is handled by reading the existing VM and accepting it only when `shepherd.io/event-id` matches the requested spec
  - [ ] **Adoption Logic**: generalized K8s resource exists handling remains deferred; V1 only reuses same-event resources for retry safety

---

## Handler Layer Degradation Protection

- [x] Approval preflight returns clear placement/degradation errors before enqueue
- [x] Capability detection degrades gracefully when optional cluster feature probes fail
- [x] Worker retry paths preserve transient K8s/runtime failures for River retry
- [ ] CacheService-based `CLUSTER_REBUILDING` UX deferred to [DEFERRED_FOLLOWUPS.md](../DEFERRED_FOLLOWUPS.md)

---

## Unit Tests

- [x] VMService unit tests cover namespace provisioning, create `AlreadyExists` idempotency, read/list/manifest, update, mutation, power/delete, console streams, and unconfigured-provider error behavior through provider stubs
- [x] Can directly pass in MockProvider
- [x] No HTTP Server dependency

---

## Pre-Phase 4 Verification

- [x] Manual DI `bootstrap.go` verified
- [x] VMService end-to-end test passes (requires DB)
  - [x] DB-backed create request -> approval atomic writer -> River enqueue -> `VMCreateWorker` -> VMService provider create path is covered
- [x] API `/api/v1/vms` CRUD handler success paths pass (requires DB)
  - [x] DB-backed list/get/filter handler coverage exists
  - [x] DB-backed create request success path persists pending `CREATE` Ticket + `VM_CREATION_REQUESTED` DomainEvent and does not create a VM before approval
  - [x] DB-backed modify request success path persists pending `MODIFY` Ticket + DomainEvent
  - [x] DB-backed delete request success path persists pending `DELETE` Ticket + `VM_DELETION_REQUESTED` DomainEvent and leaves the VM unchanged before approval
- [x] `go vet ./...` passes
- [x] `go build ./...` passes
- [x] `go test -race ./...` passes
