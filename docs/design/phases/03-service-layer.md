# Phase 3: Service Layer Integration

> **Prerequisites**: Phase 2 complete  
> **Acceptance**: VMService integrated, API endpoints working

### Required Deliverables from Phase 2

| Dependency | Location | Verification |
|------------|----------|--------------|
| KubeVirtProvider | `internal/provider/kubevirt.go` | All interface methods implemented |
| MockProvider | `internal/provider/mock.go` | Test helper ready |
| KubeVirtMapper | `internal/provider/mapper.go` | K8s ↔ Domain mapping works |
| VM status sync baseline | `internal/jobs/vm_status_sync.go` | ADR-0038 adaptive polling path implemented |
| ClusterHealthChecker | `internal/provider/health_checker.go` | Health checks functional |
| CapabilityDetector | `internal/provider/capability.go` | Feature detection works |

---

## Objectives

Integrate service layer with providers:

- Strict Manual DI (ADR-0013)
- UseCase layer (Clean Architecture)
- Transaction integration (ADR-0012)
- Concurrency control
- Handler layer

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
>
> **ADR Constraints**: For critical ADR enforcement rules (ADR-0006, ADR-0012, ADR-0013, etc.), see [CHECKLIST.md §Core ADR Constraints](../CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| Composition Root | `internal/app/bootstrap.go` | ✅ | Phase 0 |
| VMService | `internal/service/vm_service.go` | ✅ | - |
| CreateVMUseCase | `internal/usecase/create_vm.go` | ✅ | [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) |
| VMHandler | `internal/api/handlers/vm.go` | ✅ | - |
| SystemHandler | `internal/api/handlers/system.go` | ✅ | - |
| **InstanceSizeService** | `internal/service/instance_size_service.go` | ✅ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| **InstanceSize handlers** | `internal/api/handlers/server_admin_catalog.go` | ✅ | Admin and public catalog endpoints |
| CI check | `docs/design/ci/scripts/check_manual_di.sh` | ✅ | Verified locally; registration-only `init()` is allowed |

---

## 1. Dependency Injection (Strict Manual DI)

> **ADR-0013**: Wire removed, use strict manual DI  
> **ADR-0022**: Organize DI via Modular Provider Pattern

### Modular Provider Pattern (ADR-0022)

> **Goal**: Reduce `bootstrap.go` complexity by organizing dependencies into domain-specific modules.

```
internal/app/modules/
├── infrastructure.go   # Database clients, River, shared infra
├── vm.go               # VM domain: services, handlers, workers
├── approval.go         # Approval domain
├── governance.go       # System/Service/Namespace management
└── admin.go            # Admin-only operations (InstanceSize, Cluster)
```

Each module implements:
```go
type Module interface {
    Handlers() []Handler       // HTTP handlers
    Workers() []river.Worker   // River workers
    Shutdown(ctx context.Context) error
}
```

### Module Boundary Rules (Prevent Circular Dependencies)

> **Go Principle**: Go compiler forbids circular imports. Design modules with a DAG (Directed Acyclic Graph) dependency structure.

| Rule | Rationale |
|------|-----------|
| **Intra-layer injection forbidden** | `OrderService` cannot inject `ProductService` (same layer). Use higher-level orchestrator. |
| **Depend on interfaces, not implementations** | Define interfaces in consuming package to break import cycles. |
| **Extract shared types** | Common DTOs/constants go in `internal/domain/` or `internal/pkg/`. |

**Module Dependency Graph**:
```
infrastructure ← [vm, approval, governance, admin]  ✅ All depend on infra
vm ← approval                                        ✅ approval uses VM info
governance ← vm                                      ✅ vm uses System/Service
admin ← (standalone)                                 ✅ No cross-module deps
```

**Anti-patterns to avoid**:
```
vm ↔ approval    ❌ Bidirectional dependency
governance → vm → governance  ❌ Transitive cycle
```

### Composition Root

```go
// internal/app/bootstrap.go

func Bootstrap(cfg *config.Config) (*App, error) {
    // Layer 1: Infrastructure (shared)
    infraModule := modules.NewInfrastructureModule(cfg)
    
    // Layer 2: Domain modules (depend on infrastructure)
    vmModule := modules.NewVMModule(infraModule)
    approvalModule := modules.NewApprovalModule(infraModule)
    governanceModule := modules.NewGovernanceModule(infraModule)
    adminModule := modules.NewAdminModule(infraModule)
    
    // Collect all handlers and workers
    allHandlers := slices.Concat(
        vmModule.Handlers(),
        approvalModule.Handlers(),
        governanceModule.Handlers(),
        adminModule.Handlers(),
    )
    allWorkers := slices.Concat(
        vmModule.Workers(),
        approvalModule.Workers(),
        adminModule.Workers(),
    )
    
    return &App{
        Handlers: allHandlers,
        Workers:  allWorkers,
        Shutdown: func(ctx context.Context) error {
            return errors.Join(
                vmModule.Shutdown(ctx),
                approvalModule.Shutdown(ctx),
                infraModule.Shutdown(ctx),
            )
        },
    }, nil
}
```


### CI Enforcement

| Rule | Enforcement |
|------|-------------|
| All `New*()` calls in `bootstrap.go` | `check_manual_di.sh` |
| No global variables for dependencies | `check_manual_di.sh` |
| No `init()` for initialization | `check_manual_di.sh` |
| No instantiation outside `internal/app/` | `check_manual_di.sh` |

---

## 2. Service Layer Standards

### Layer Responsibilities

| Layer | Responsibility | Can Call |
|-------|----------------|----------|
| Handler | Parse request, call UseCase, return response | UseCase |
| UseCase | Orchestrate flow, manage transactions | Service, Repository |
| Service | Business logic | Repository, Provider |
| Repository | Data access | Ent Client |

### Transaction Rules

| Rule | Enforcement |
|------|-------------|
| Service layer must not manage transactions | `shepherd-arch` (tx boundary analyzer) |
| K8s calls forbidden inside transactions | `shepherd-arch/k8sintransaction` |
| Transaction boundaries at UseCase layer | - |

> ⚠️ **Developer Guidance**: Run these checks locally before committing:
> ```bash
> make lint
> # Optional: run the standalone script when iterating on transaction-safe K8s boundaries
> go run docs/design/ci/scripts/check_k8s_in_transaction.go ./...
> ```
>
> **Anti-Pattern (ADR-0012)**: K8s API calls inside DB transactions cause:
> - Extended lock duration (network latency → deadlocks)
> - False atomicity (K8s changes cannot rollback with DB)
> - Connection pool exhaustion
>
> See Microsoft Saga pattern guidance for distributed transaction patterns:
> https://learn.microsoft.com/en-us/azure/architecture/reference-architectures/saga/saga

---

## 3. Transaction Integration (ADR-0012)

> **Reference**: [examples/usecase/create_vm.go](../examples/usecase/create_vm.go)

### Hybrid Atomic Pattern

```go
// Single pgx transaction for:
// 1. sqlc: Write DomainEvent
// 2. sqlc: Create ApprovalTicket  
// 3. River: InsertTx (after approval)
// 4. Atomic commit

tx, _ := pool.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)

sqlcTx := queries.WithTx(tx)
sqlcTx.CreateDomainEvent(ctx, ...)
sqlcTx.CreateApprovalTicket(ctx, ...)

// After approval:
riverClient.InsertTx(ctx, tx, jobArgs, nil)

tx.Commit(ctx) // Single atomic commit
```

### Shared Connection Pool

```go
DatabaseClients{
    Pool:        *pgxpool.Pool     // Shared by all
    EntClient:   *ent.Client       // Uses stdlib.OpenDBFromPool
    SqlcQueries: *sqlc.Queries     // Uses Pool directly
}
```

### Service Instance Allocation

VM instance numbers are allocated by `sqlc` in the same `pgx.Tx` that approves
the ticket, marks the domain event processing, inserts the VM row, and enqueues
the River job. The canonical query is `AllocateServiceInstance` in
`internal/repository/sqlc/queries/ticket.sql`:

```sql
UPDATE services AS s
SET next_instance_index = s.next_instance_index + 1
WHERE s.id = $1
RETURNING s.next_instance_index - 1 AS allocated_index;
```

This replaces the legacy application-side VM naming helper. PostgreSQL row-level
write locking serializes concurrent updates to the same service row, and
`RETURNING` gives the transaction the allocated value without a separate read.
The behavior is covered by `TestQueries_AllocateServiceInstanceConcurrent`.

---

## 4. Governance Model Operations

### Approval Matrix

| Operation | Approval Required | Notes |
|-----------|-------------------|-------|
| Create System | No | User self-service |
| Create Service | No | User self-service |
| Create VM | **Yes** | Consumes resources |
| Modify VM | **Yes** | Resource change |
| Delete System | No | Must have no Services |
| Delete Service | No | Must have no VMs |

### Delete Restrict Pattern

```go
func (s *SystemService) DeleteSystem(ctx context.Context, id string) error {
    // Check for children
    count, err := s.serviceRepo.CountBySystemID(ctx, id)
    if count > 0 {
        return ErrDeleteRestricted{ChildrenType: "services", Count: count}
    }
    return s.repo.Delete(ctx, id)
}
```

---

## 5. Concurrency Control

### ADR-0006: K8s Ops at Worker Layer

K8s operation concurrency controlled at **River Worker layer**, not HTTP layer:

| Location | Control | Reference |
|----------|---------|-----------|
| River Worker | `RIVER_MAX_WORKERS` (default: 10) | [DEPENDENCIES.md](../DEPENDENCIES.md) |
| HTTP Layer | Only lightweight DB rate limiting | - |

Per-cluster semaphore limits, cluster-specific fairness, and hot-reloadable
quota updates are not part of the V1 runtime. They remain deferred in
[RFC-0015](../../rfc/RFC-0015-per-cluster-concurrency.md).

### HPA Constraints

| Formula | Limit |
|---------|-------|
| `HPA.maxReplicas × River.MaxWorkers` | ≤ 50 |

> **Example**: With `RIVER_MAX_WORKERS=10`, your `HPA.maxReplicas` should be ≤ 5 (5 × 10 = 50).
>
> See [DEPENDENCIES.md](../DEPENDENCIES.md#hpa-concurrency-constraints-required) for detailed calculation examples.

### Manual DI Gate Status

The original legacy scanner `docs/design/ci/scripts/check_manual_di.sh` is no
longer the blocking implementation. The active gate is the syntax-aware
`shepherd-arch/manualdi` golangci-lint analyzer, which enforces centralized
hand-written DI, blocks Wire/Redis runtime drift, and prevents service/repository
wiring outside `internal/app/`. The shell script remains only as historical
reference in `docs/design/ci/README.md`.

### Deferred Per-Cluster Semaphore

```go
type ClusterSemaphoreManager struct {
    semaphores map[string]*ResizableSemaphore
    mu         sync.RWMutex
}

func (m *Manager) Get(clusterName string) *ResizableSemaphore {
    // Lazy create semaphore for cluster
}

func (m *Manager) UpdateGlobalLimit(newLimit int) {
    // Hot-reload support
}
```

---

## 6. Handler Patterns

### Unified 202 Return (ADR-0006)

All write operations return `202 Accepted` with `Location` header:

> **ADR-0006 Compliance**: Response must include `Location` header and `links` for status tracking.

```go
func (h *VMHandler) Create(c *gin.Context) {
    // 1. Parse request
    var req CreateVMRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 2. DryRun validation
    if err := h.validateWithDryRun(ctx, req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 3. Create event + ticket (202 Accepted)
    result, err := h.createVMUseCase.Execute(ctx, req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // 4. Return with Location header (ADR-0006)
    statusURL := fmt.Sprintf("/api/v1/events/%s", result.EventID)
    c.Header("Location", statusURL)
    c.JSON(202, gin.H{
        "event_id":  result.EventID,
        "ticket_id": result.TicketID,
        "status":    "PENDING_APPROVAL",
        "message":   "Request accepted, awaiting approval",
        "links": gin.H{
            "self":   statusURL,
            "ticket": fmt.Sprintf("/api/v1/tickets/%s", result.TicketID),
        },
    })
}
```

> **Note**: For auto-approved operations, return `task_id` instead of `event_id`/`ticket_id`.
> See [ADR-0006 §API Response Standards](../../adr/ADR-0006-unified-async-model.md#api-response-standards).

### Deferred Cache Rebuild UX

The cache-specific `CLUSTER_REBUILDING` handler gate is not part of the V1
runtime. V1 degradation handling is covered by approval preflight checks,
capability detection, and River worker retry behavior. Keep this pattern as
future-scope guidance with the deferred follow-up tracker.

```go
func (h *VMHandler) checkDegradation(c *gin.Context, cluster string) bool {
    if h.cacheService.IsClusterRebuilding(cluster) {
        c.JSON(503, gin.H{
            "code":    "CLUSTER_REBUILDING",
            "message": "Cluster cache is rebuilding, please retry",
        })
        return true
    }
    return false
}
```

---

## 7. VMService Methods

| Method | Transaction | K8s Call |
|--------|-------------|----------|
| `ValidateAndPrepare()` | Outside | Dry run only |
| `CreateVMRecord()` | Inside | No |
| `ExecuteK8sCreate()` | Outside | Yes |

### Idempotency

```go
func (s *VMService) ExecuteK8sCreate(ctx context.Context, spec *domain.VMSpec) error {
    err := s.provider.CreateVM(ctx, cluster, namespace, spec)
    if errors.IsAlreadyExists(err) {
        // Attempt adoption instead of error
        return s.attemptAdoption(ctx, spec)
    }
    return err
}
```

---

## Acceptance Criteria

- [x] Manual DI in `bootstrap.go` and `internal/app/modules/`
- [x] `check_manual_di.sh` passes
- [x] UseCase layer owns transaction orchestration for write paths
- [x] VM service instance allocation uses transaction-local SQL `UPDATE ... RETURNING`
- [x] Handlers return `202 Accepted` for async VM writes and batch/console request paths
- [x] River worker concurrency is configured through queue-specific `MaxWorkers`
- [x] Cluster/runtime degradation handling exists in approval preflight, capability detection, and worker retry paths
- [x] DB-backed VMService create request approval/worker e2e covers UseCase -> approval atomic writer -> River enqueue -> `VMCreateWorker` -> provider create path
- [x] HPA constraints documented in [DEPENDENCIES.md §HPA Concurrency Constraints Required](../DEPENDENCIES.md#hpa-concurrency-constraints-required)

Advanced degradation/circuit-breaker UX remains a non-blocking follow-up in
[DEFERRED_FOLLOWUPS.md](../DEFERRED_FOLLOWUPS.md).

---

## 8. InstanceSize Management (ADR-0018)

> **Added per [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md)**: Admin InstanceSize CRUD operations.

### Admin Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/admin/instance-sizes` | GET | List all InstanceSizes |
| `/api/v1/admin/instance-sizes` | POST | Create InstanceSize |
| `/api/v1/admin/instance-sizes/{instance_size_id}` | PATCH | Update InstanceSize |
| `/api/v1/admin/instance-sizes/{instance_size_id}` | DELETE | Delete InstanceSize |
| `/api/v1/instance-sizes` | GET | User-facing enabled catalog list |

### Write Semantics

InstanceSize CRUD is a pure PostgreSQL catalog operation. It is synchronous and
does not call K8s, so ADR-0006 River enqueue is not required. ADR-0006 applies
to external-system write operations such as VM create/delete/power/modify.

```go
// internal/api/handlers/server_admin_catalog.go
// createAdminInstanceSize -> validate request -> ent.InstanceSize.Create() -> 201
// updateAdminInstanceSize -> validate request -> ent.InstanceSize.UpdateOneID() -> 200
// deleteAdminInstanceSize -> delete guards -> ent.InstanceSize.DeleteOneID() -> 204
```

### Overcommit Warnings (Approval Flow)

| Scenario | Warning Level | Description |
|----------|---------------|-------------|
| Overcommit in Production | ⚠️ Warning | Admin sees explicit warning but can approve |
| Dedicated CPU + Overcommit | ❌ Error | **Blocking** - cannot be approved (incompatible) |

---

## Related Documentation

- [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) - Atomic TX pattern
- [ADR-0006](../../adr/ADR-0006-unified-async-model.md) - Unified Async Model
- [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) - Hybrid Transaction
- [ADR-0013](../../adr/ADR-0013-manual-di.md) - Manual DI
- [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) - Governance Model V2 (Entity Decoupling)
- [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md) - Go Module Vanity Import
- [ADR-0017](../../adr/ADR-0017-vm-request-flow-clarification.md) - VM Request Flow (Cluster selection at approval time)
- [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) - Instance Size Abstraction (Overcommit, Dry-Run, Validation)
- [ADR-0022](../../adr/ADR-0022-modular-provider-pattern.md) - Modular Provider Pattern (Module-based DI organization)
