---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-01-27
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-0022: Modular Provider Pattern for Dependency Injection

> **Review Period**: Until 2026-01-30 (48-hour minimum)  
> **Discussion**: [Issue #32](https://github.com/kv-shepherd/shepherd/issues/32)  
> **Extends**: [ADR-0013](./ADR-0013-manual-di.md) (Manual Dependency Injection)

---

## Context and Problem Statement

[ADR-0013](./ADR-0013-manual-di.md) established **strict manual dependency injection** as the project standard, with all dependency assembly centralized in `internal/app/bootstrap.go`.

As the application grows, `bootstrap.go` becomes increasingly complex:

```go
// Current state - 200+ lines of initialization
func Bootstrap(ctx context.Context, cfg *config.Config) (*Application, error) {
    // Infrastructure layer (~20 lines)
    pool, _ := infrastructure.NewPgxPool(...)
    entClient, _ := infrastructure.NewEntClient(pool)
    riverClient, _ := infrastructure.NewRiverClient(pool)
    logger := slog.Default()
    
    // Repository layer (~30 lines)
    vmRepo := repository.NewVMRepository(entClient)
    serviceRepo := repository.NewServiceRepository(entClient)
    systemRepo := repository.NewSystemRepository(entClient)
    clusterRepo := repository.NewClusterRepository(entClient)
    templateRepo := repository.NewTemplateRepository(entClient)
    instanceSizeRepo := repository.NewInstanceSizeRepository(entClient)
    approvalRepo := repository.NewApprovalRepository(entClient)
    // ... 10+ more repositories
    
    // Service layer (~40 lines)
    vmService := service.NewVMService(vmRepo, riverClient)
    approvalService := service.NewApprovalService(approvalRepo, riverClient)
    // ... 10+ more services
    
    // UseCase layer (~30 lines)
    // Handler layer (~30 lines)
    // ...
}
```

This violates the **Single Responsibility Principle** - `bootstrap.go` knows about every domain's dependencies.

We need a pattern that:

1. Preserves strict manual DI (no Wire/Dig - per ADR-0013)
2. Reduces `bootstrap.go` cognitive load
3. Enables domain-focused dependency grouping
4. Maintains explicit, reviewable dependency graph

---

## Decision Drivers

* **ADR-0013 Compliance**: Must remain strictly manual DI, no code generation
* **Cognitive load reduction**: Developers should understand one domain at a time
* **Testability**: Modules should be independently testable
* **Explicit dependencies**: Dependency graph must remain visible and reviewable
* **Scalability**: Pattern must support 10+ domains without degradation

---

## Considered Options

* **Option 1**: Modular Provider Pattern (Domain Modules)
* **Option 2**: Functional Options Pattern
* **Option 3**: Builder Pattern
* **Option 4**: Keep current flat structure (do nothing)

---

## Decision Outcome

**Recommended option**: "Option 1: Modular Provider Pattern", because it maintains strict manual DI while organizing dependencies by domain, reducing `bootstrap.go` complexity from 200+ lines to ~30 lines.

### Consequences

* ✅ Good, because `bootstrap.go` becomes concise top-level orchestration
* ✅ Good, because each domain module is self-contained and testable
* ✅ Good, because new developers only need to understand relevant domain modules
* ✅ Good, because fully compliant with ADR-0013 (no code generation)
* 🟡 Neutral, because adds one level of indirection
* ❌ Bad, because requires refactoring existing code (one-time cost)

### Confirmation

* All modules can be instantiated independently in tests
* `bootstrap.go` does not exceed 100 lines
* No Wire/Dig or reflection-based DI is used
* CI validates manual DI compliance

---

## Implementation

### Directory Structure

```
internal/
├── app/
│   ├── bootstrap.go          # Top-level orchestration (< 100 lines)
│   └── modules/              # Domain modules
│       ├── infrastructure.go # Cross-cutting infrastructure
│       ├── vm.go             # VM domain module
│       ├── approval.go       # Approval domain module
│       ├── governance.go     # System/Service/RBAC module
│       └── admin.go          # Admin (Cluster/Template/InstanceSize) module
```

### Module Interface

```go
// internal/app/modules/module.go

// Module represents a domain-specific dependency container.
// Each module owns its repositories, services, usecases, and handlers.
type Module interface {
    // Handlers returns the HTTP handlers for this module
    Handlers() []Handler
    
    // Workers returns the River workers for this module
    Workers() []river.Worker
    
    // Shutdown performs graceful shutdown of module resources
    Shutdown(ctx context.Context) error
}

// Handler represents an HTTP handler that can register routes
type Handler interface {
    RegisterRoutes(rg *gin.RouterGroup)
}
```

### Infrastructure Module

```go
// internal/app/modules/infrastructure.go

// Infrastructure provides cross-cutting dependencies used by all domain modules.
// This is NOT a Module - it's a dependency provider.
type Infrastructure struct {
    EntClient   *ent.Client
    RiverClient *river.Client[pgx.Tx]
    Logger      *slog.Logger
    Config      *config.Config
}

func NewInfrastructure(ctx context.Context, cfg *config.Config) (*Infrastructure, error) {
    pool, err := pgxpool.New(ctx, cfg.Database.DSN)
    if err != nil {
        return nil, fmt.Errorf("create pgx pool: %w", err)
    }
    
    entClient := ent.NewClient(ent.Driver(entsql.OpenDB(cfg.Database.DriverName, pool)))
    
    riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
        Queues: map[string]river.QueueConfig{
            river.QueueDefault: {MaxWorkers: 10},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("create river client: %w", err)
    }
    
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: cfg.Log.Level,
    }))
    
    return &Infrastructure{
        EntClient:   entClient,
        RiverClient: riverClient,
        Logger:      logger,
        Config:      cfg,
    }, nil
}

func (i *Infrastructure) Shutdown(ctx context.Context) error {
    var errs []error
    
    if err := i.RiverClient.Stop(ctx); err != nil {
        errs = append(errs, fmt.Errorf("stop river: %w", err))
    }
    if err := i.EntClient.Close(); err != nil {
        errs = append(errs, fmt.Errorf("close ent: %w", err))
    }
    
    return errors.Join(errs...)
}
```

### Domain Module Example (VM)

```go
// internal/app/modules/vm.go

// VMModule encapsulates all VM-related dependencies.
type VMModule struct {
    // Public: exposed to other modules if needed
    VMService *service.VMService
    
    // Private: internal to this module
    repo    *repository.VMRepository
    usecase *usecases.VMUseCase
    handler *handlers.VMHandler
    workers []river.Worker
}

func NewVMModule(infra *Infrastructure) *VMModule {
    // Layer 1: Repository
    repo := repository.NewVMRepository(infra.EntClient)
    
    // Layer 2: Service
    vmService := service.NewVMService(repo, infra.RiverClient)
    
    // Layer 3: UseCase
    usecase := usecases.NewVMUseCase(vmService)
    
    // Layer 4: Handler
    handler := handlers.NewVMHandler(usecase)
    
    // Workers
    workers := []river.Worker{
        workers.NewVMProvisionWorker(vmService, infra.Logger),
        workers.NewVMDeleteWorker(vmService, infra.Logger),
    }
    
    return &VMModule{
        VMService: vmService,
        repo:      repo,
        usecase:   usecase,
        handler:   handler,
        workers:   workers,
    }
}

func (m *VMModule) Handlers() []Handler {
    return []Handler{m.handler}
}

func (m *VMModule) Workers() []river.Worker {
    return m.workers
}

func (m *VMModule) Shutdown(ctx context.Context) error {
    return nil // No module-specific shutdown needed
}
```

### Simplified Bootstrap

```go
// internal/app/bootstrap.go

func Bootstrap(ctx context.Context, cfg *config.Config) (*Application, error) {
    // Layer 0: Infrastructure (cross-cutting)
    infra, err := modules.NewInfrastructure(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("create infrastructure: %w", err)
    }
    
    // Domain Modules (each is self-contained)
    vmModule := modules.NewVMModule(infra)
    approvalModule := modules.NewApprovalModule(infra, vmModule.VMService)
    governanceModule := modules.NewGovernanceModule(infra)
    adminModule := modules.NewAdminModule(infra)
    
    // Collect all modules
    allModules := []modules.Module{
        vmModule,
        approvalModule,
        governanceModule,
        adminModule,
    }
    
    // Setup HTTP Router
    router := gin.New()
    api := router.Group("/api/v1")
    for _, mod := range allModules {
        for _, h := range mod.Handlers() {
            h.RegisterRoutes(api)
        }
    }
    
    // Setup River Workers
    var allWorkers []river.Worker
    for _, mod := range allModules {
        allWorkers = append(allWorkers, mod.Workers()...)
    }
    // Register workers with riverClient...
    
    return &Application{
        Router:     router,
        Infra:      infra,
        Modules:    allModules,
    }, nil
}
```

### Inter-Module Dependencies

When modules need dependencies from other modules, pass them explicitly:

```go
// Approval module needs VM service for status lookups
approvalModule := modules.NewApprovalModule(infra, vmModule.VMService)
```

This keeps dependencies explicit and visible in `bootstrap.go`.

---

## Integration with Testing Strategy

This pattern enables better integration testing with `testcontainers-go` and `envtest`:

### Per-Module Testing

```go
// internal/app/modules/vm_test.go

func TestVMModule_Integration(t *testing.T) {
    ctx := context.Background()
    
    // Start test containers
    pgContainer, err := postgres.Run(ctx, "postgres:16")
    require.NoError(t, err)
    defer pgContainer.Terminate(ctx)
    
    // Create test infrastructure
    cfg := &config.Config{
        Database: config.Database{DSN: pgContainer.MustConnectionString(ctx)},
    }
    infra, err := modules.NewInfrastructure(ctx, cfg)
    require.NoError(t, err)
    defer infra.Shutdown(ctx)
    
    // Run migrations
    require.NoError(t, infra.EntClient.Schema.Create(ctx))
    
    // Create module under test
    vmModule := modules.NewVMModule(infra)
    
    // Test use cases...
    t.Run("CreateVM", func(t *testing.T) {
        // ...
    })
}
```

### K8s Integration with envtest

```go
// internal/app/modules/vm_k8s_test.go

func TestVMModule_KubeVirt(t *testing.T) {
    // Setup envtest
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "testdata", "crds"),
        },
    }
    
    cfg, err := testEnv.Start()
    require.NoError(t, err)
    defer testEnv.Stop()
    
    // Create KubeVirt client
    scheme := runtime.NewScheme()
    kubevirtv1.AddToScheme(scheme)
    k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
    require.NoError(t, err)
    
    // Test K8s operations...
}
```

---

## Pros and Cons of the Options

### Option 1: Modular Provider Pattern (Recommended)

* ✅ Good, because maintains strict manual DI
* ✅ Good, because reduces `bootstrap.go` to orchestration-only
* ✅ Good, because enables per-domain testing
* ✅ Good, because new developers focus on one domain at a time
* 🟡 Neutral, because adds one level of indirection
* ❌ Bad, because requires one-time refactoring effort

### Option 2: Functional Options Pattern

* ✅ Good, because flexible configuration
* ❌ Bad, because obscures dependency graph
* ❌ Bad, because harder to understand for new developers

### Option 3: Builder Pattern

* ✅ Good, because fluent API
* ❌ Bad, because adds unnecessary complexity for this use case
* ❌ Bad, because doesn't naturally group by domain

### Option 4: Do Nothing

* ✅ Good, because no change required
* ❌ Bad, because `bootstrap.go` continues to grow linearly
* ❌ Bad, because cognitive load increases with each new feature

---

## Acceptance Checklist (Execution Tasks)

Upon acceptance, perform the following:

1. [ ] Create `internal/app/modules/` directory
2. [ ] Create `module.go` interface definition
3. [ ] Create `infrastructure.go` for shared dependencies
4. [ ] Migrate VM domain to `vm.go` module
5. [ ] Migrate Approval domain to `approval.go` module
6. [ ] Migrate Governance domain to `governance.go` module
7. [ ] Migrate Admin domain to `admin.go` module
8. [ ] Refactor `bootstrap.go` to use modules
9. [ ] Add integration test examples with testcontainers
10. [ ] Update developer documentation

---

## Related Decisions

* [ADR-0013: Manual Dependency Injection](./ADR-0013-manual-di.md) - This ADR extends ADR-0013's pattern
* [ADR-0006: Unified Async Model](./ADR-0006-unified-async-model.md) - River workers integrate with modules

---

## References

* [Uber Go Style Guide - Dependency Injection](https://github.com/uber-go/guide/blob/master/style.md)
* [testcontainers-go Documentation](https://golang.testcontainers.org/)
* [controller-runtime envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest)
* [Clean Architecture in Go](https://threedots.tech/post/introducing-clean-architecture/)

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-27 | @jindyzhao | Initial draft based on architecture improvement suggestions |

---

_End of ADR-0022_
