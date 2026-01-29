# Phase 3: Service Layer Integration

> **Prerequisites**: Phase 2 complete  
> **Acceptance**: VMService integrated, API endpoints working

### Required Deliverables from Phase 2

| Dependency | Location | Verification |
|------------|----------|--------------|
| KubeVirtProvider | `internal/provider/kubevirt.go` | All interface methods implemented |
| MockProvider | `internal/provider/mock.go` | Test helper ready |
| KubeVirtMapper | `internal/provider/mapper.go` | K8s ↔ Domain mapping works |
| ResourceWatcher | `internal/provider/watcher.go` | List-Watch pattern implemented |
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

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| Composition Root | `internal/app/bootstrap.go` | ⬜ | - |
| VMService | `internal/service/vm_service.go` | ⬜ | - |
| CreateVMUseCase | `internal/usecase/create_vm.go` | ⬜ | [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) |
| VMHandler | `internal/api/handlers/vm.go` | ⬜ | - |
| **InstanceSizeService** | `internal/service/instance_size.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| **InstanceSizeHandler** | `internal/api/handlers/instance_size.go` | ⬜ | [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md) |
| CI check | `scripts/ci/check_manual_di.sh` | ⬜ | - |

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
| Service layer must not manage transactions | `check_transaction_boundary.go` |
| K8s calls forbidden inside transactions | `check_k8s_in_transaction.go` |
| Transaction boundaries at UseCase layer | - |

> ⚠️ **Developer Guidance**: Run these checks locally before committing:
> ```bash
> go run scripts/ci/check_transaction_boundary.go ./...
> go run scripts/ci/check_k8s_in_transaction.go ./...
> ```
>
> **Anti-Pattern (ADR-0012)**: K8s API calls inside DB transactions cause:
> - Extended lock duration (network latency → deadlocks)
> - False atomicity (K8s changes cannot rollback with DB)
> - Connection pool exhaustion
>
> See [Best Practice Search Results] for distributed transaction patterns.

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
| Per-Cluster | `K8S_CLUSTER_CONCURRENCY` (default: 20) | [DEPENDENCIES.md](../DEPENDENCIES.md) |
| HTTP Layer | Only lightweight DB rate limiting | - |

### HPA Constraints

| Formula | Limit |
|---------|-------|
| `HPA.maxReplicas × River.MaxWorkers` | ≤ 50 |
| `HPA.maxReplicas × K8S_CLUSTER_CONCURRENCY` | ≤ 60 |

> **Example**: With `RIVER_MAX_WORKERS=10`, your `HPA.maxReplicas` should be ≤ 5 (5 × 10 = 50).
>
> See [DEPENDENCIES.md](../DEPENDENCIES.md#hpa-concurrency-constraints-required) for detailed calculation examples.

### ResizableSemaphore

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

### Degradation Protection

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

- [ ] Manual DI in `bootstrap.go`
- [ ] `check_manual_di.sh` passes
- [ ] UseCase manages transactions
- [ ] Handler returns 202 for writes
- [ ] Degradation check works
- [ ] HPA constraints documented

---

## 8. InstanceSize Management (ADR-0018)

> **Added per [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md)**: Admin InstanceSize CRUD operations.

### Admin Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/admin/instance-sizes` | GET | List all InstanceSizes |
| `/api/v1/admin/instance-sizes` | POST | Create InstanceSize |
| `/api/v1/admin/instance-sizes/{name}` | GET | Get InstanceSize by name |
| `/api/v1/admin/instance-sizes/{name}` | PUT | Update InstanceSize |
| `/api/v1/admin/instance-sizes/{name}` | DELETE | Delete InstanceSize |
| `/api/v1/admin/instance-sizes?dryRun=All` | POST | Dry-run validation only |

### River Queue Integration (ADR-0006 Compliance)

> **Mandatory**: All InstanceSize write operations MUST go through River Queue per [ADR-0006](../../adr/ADR-0006-unified-async-model.md).

```go
// All admin writes create River Job
func (h *InstanceSizeHandler) Create(c *gin.Context) {
    var req CreateInstanceSizeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Insert via River Queue
    job, err := h.riverClient.Insert(ctx, InstanceSizeCRUDJobArgs{
        Operation: "CREATE",
        Payload:   req,
    }, nil)
    
    c.JSON(202, gin.H{
        "job_id": job.ID,
        "status": "PENDING",
    })
}
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
