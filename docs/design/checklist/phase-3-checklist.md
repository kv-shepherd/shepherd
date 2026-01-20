# Phase 3 Checklist: Service Layer Integration

> **Detailed Document**: [phases/03-service-layer.md](../phases/03-service-layer.md)

---

## Dependency Injection (Strict Manual DI)

- [ ] **Composition Root Created**:
  - [ ] `internal/app/bootstrap.go` created
  - [ ] All dependency assembly centralized in this file
  - [ ] Layered construction: Infrastructure → Repository → Service → UseCase → Handler
- [ ] **CI Check**:
  - [ ] `scripts/ci/check_manual_di.sh` created
  - [ ] Forbidden to instantiate Service/Repository outside `internal/app/`
  - [ ] Forbidden to initialize dependencies in `init()` functions
- [ ] **Standards**:
  - [ ] ✅ All `New*()` constructor calls centralized in `bootstrap.go`
  - [ ] ✅ Dependencies explicitly injected via constructors
  - [ ] ❌ Forbidden to use global variables for dependencies
  - [ ] ❌ Forbidden to use `init()` functions for dependency initialization
- [ ] Provider factory functions
- [ ] Repository factory functions
- [ ] Service dependencies injected via constructors

---

## Service Layer Standards

- [ ] ❌ Service layer must not directly manage transactions
- [ ] Service receives `*ent.Client` parameter (in-transaction or not)
- [ ] ❌ K8s API calls forbidden inside transactions
- [ ] ✅ Uses two-phase pattern

---

## Governance Model Operation Standards

- [ ] **Operation Approval Matrix**:
  - [ ] Create System: **No approval required** (user self-service)
  - [ ] Create Service: **No approval required** (user self-service)
  - [ ] Create VM: **Approval required** (consumes resources)
  - [ ] Delete System: No approval, but must have no child Services
  - [ ] Delete Service: No approval, but must have no child VMs
- [ ] **VM Request Flow Implementation** complete
- [ ] **Hierarchical Delete Constraint (Delete Restrict)** implemented

---

## UseCase Layer Standards (Clean Architecture)

- [ ] `internal/usecase/` directory created
- [ ] `CreateVMUseCase` implementation complete
- [ ] **UseCase Reusability** verified (HTTP, CLI, gRPC, Cron)
- [ ] **Handler Simplification** enforced

---

## Transaction Integration (ADR-0012 Hybrid Atomic Transaction)

- [ ] **sqlc Configuration and Code Generation** complete
- [ ] **DatabaseClients Shared Pool** implemented
- [ ] **CreateVMAtomicUseCase Implementation** complete
- [ ] **CI Block: sqlc Usage Scope Check** active
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

- [ ] `ValidateAndPrepare()` method (outside transaction)
- [ ] `CreateVMRecord()` method (inside transaction, only writes PENDING)
- [ ] `ExecuteK8sCreate()` method (outside transaction)
  - [ ] **Idempotency**: Handle AlreadyExists error
  - [ ] **Adoption Logic**: K8s resource exists handling

---

## Handler Layer Degradation Protection

- [ ] VMHandler injects CacheService
- [ ] `checkClusterDegradation()` method implemented
- [ ] **Strong Consistency Operations Block** implemented
- [ ] Degradation returns clear error code: `CLUSTER_REBUILDING`

---

## Unit Tests

- [ ] VMService unit tests
- [ ] Can directly pass in MockProvider
- [ ] No HTTP Server dependency

---

## Pre-Phase 4 Verification

- [ ] Manual DI `bootstrap.go` verified
- [ ] VMService end-to-end test passes
- [ ] API `/api/v1/vms` CRUD test passes
