# Phase 3 Checklist: Service Layer Integration

> **Detailed Document**: [phases/03-service-layer.md](../phases/03-service-layer.md)
>
> **Implementation Status**: 🔄 Partial (~84%) — Core DI/UseCase + ADR-0012 atomic path + sqlc + InstanceSize handler + analyzer-backed DI/sqlc CI gates done; concurrency/degradation deferred

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
- [ ] **Lock Key Standardization** implemented

---

## Concurrency Control

- [ ] **River Worker Concurrency Control** configured
- [ ] **ResizableSemaphore Implementation** complete
- [ ] **ClusterSemaphoreManager** implemented
- [ ] **Hot-Reload Integration** working
- [ ] **HPA Constraint Verification** passed
- [ ] Middleware correctly registered to routes

---

## VMService Refactoring

- [x] `ValidateAndPrepare()` method (outside transaction)
- [x] `CreateVMRecord()` — via CreateVMUseCase atomic transaction (DomainEvent + ApprovalTicket)
- [x] `ExecuteK8sCreate()` method (outside transaction)
  - [ ] **Idempotency**: Handle AlreadyExists error (deferred)
  - [ ] **Adoption Logic**: K8s resource exists handling (deferred)

---

## Handler Layer Degradation Protection

- [ ] VMHandler injects CacheService
- [ ] `checkClusterDegradation()` method implemented
- [ ] **Strong Consistency Operations Block** implemented
- [ ] Degradation returns clear error code: `CLUSTER_REBUILDING`

---

## Unit Tests

- [ ] VMService unit tests (deferred — requires testcontainers)
- [x] Can directly pass in MockProvider
- [x] No HTTP Server dependency

---

## Pre-Phase 4 Verification

- [x] Manual DI `bootstrap.go` verified
- [ ] VMService end-to-end test passes (requires DB)
- [ ] API `/api/v1/vms` CRUD test passes (requires DB)
- [x] `go vet ./...` passes
- [x] `go build ./...` passes
- [x] `go test -race ./...` passes
