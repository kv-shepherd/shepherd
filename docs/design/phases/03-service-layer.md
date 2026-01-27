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
| CI check | `scripts/ci/check_manual_di.sh` | ⬜ | - |

---

## 1. Dependency Injection (Strict Manual DI)

> **ADR-0013**: Wire removed, use strict manual DI

### Composition Root

```go
// internal/app/bootstrap.go

func Bootstrap(cfg *config.Config) (*App, error) {
    // Layer 1: Infrastructure
    dbClients, err := infrastructure.NewDatabaseClients(ctx, cfg.Database)
    
    // Layer 2: Repositories
    vmRepo := repository.NewVMRepository(dbClients.EntClient)
    clusterRepo := repository.NewClusterRepository(dbClients.EntClient)
    
    // Layer 3: Providers
    credProvider := provider.NewKubeconfigProvider(clusterRepo, crypto)
    kubeProvider := provider.NewKubeVirtProvider(credProvider)
    
    // Layer 4: Services
    vmService := service.NewVMService(vmRepo, kubeProvider)
    
    // Layer 5: UseCases
    createVMUseCase := usecase.NewCreateVMAtomicUseCase(
        dbClients.Pool, 
        dbClients.SqlcQueries, 
        riverClient,
    )
    
    // Layer 6: Handlers
    vmHandler := handlers.NewVMHandler(vmService, createVMUseCase)
    
    return &App{...}, nil
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

All write operations return `202 Accepted`:

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
    
    c.JSON(202, gin.H{
        "event_id":  result.EventID,
        "ticket_id": result.TicketID,
        "status":    "PENDING_APPROVAL",
    })
}
```

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

## Related Documentation

- [examples/usecase/create_vm.go](../examples/usecase/create_vm.go) - Atomic TX pattern
- [ADR-0006](../../adr/ADR-0006-unified-async-model.md) - Unified Async Model
- [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) - Hybrid Transaction
- [ADR-0013](../../adr/ADR-0013-manual-di.md) - Manual DI
- [ADR-0015](../../adr/ADR-0015-governance-model-v2.md) - Governance Model V2 (Entity Decoupling)
- [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md) - Go Module Vanity Import
