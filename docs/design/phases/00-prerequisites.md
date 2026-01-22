# Phase 0: Project Initialization and Toolchain

> **Prerequisites**: None  
> **Acceptance**: Project compiles, CI pipeline runs, health checks respond

---

## Objectives

Establish Go project infrastructure:

- Go module initialization
- Directory structure
- Configuration management
- Logging system
- CI pipeline
- Health checks
- Worker pool (required coding standard)

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| Go module | `go.mod`, `go.sum` | ⬜ | - |
| Entry point | `cmd/server/main.go` | ⬜ | - |
| Configuration | `internal/config/config.go` | ⬜ | [examples/config/config.go](../examples/config/config.go) |
| Logging | `internal/pkg/logger/logger.go` | ⬜ | - |
| Health checks | `internal/api/handlers/health.go` | ⬜ | [examples/handlers/health.go](../examples/handlers/health.go) |
| Database | `internal/infrastructure/database.go` | ⬜ | [examples/infrastructure/database.go](../examples/infrastructure/database.go) |
| Worker pool | `internal/pkg/worker/pool.go` | ⬜ | [examples/worker/pool.go](../examples/worker/pool.go) |
| CI config | `.github/workflows/ci.yml` | ⬜ | - |
| Lint config | `.golangci.yml` | ⬜ | - |
| Dockerfile | `Dockerfile` | ⬜ | - |
| Data seeding | `cmd/seed/main.go` | ⬜ | - |
| River migration | `migrations/river/` | ⬜ | - |

---

## 1. Project Initialization

### 1.1 Go Module

```bash
mkdir -p shepherd
cd shepherd
go mod init kv-shepherd.io/shepherd
```

### 1.2 Directory Structure

```
kubevirt-shepherd-go/
├── cmd/
│   ├── server/main.go        # Application entry
│   └── seed/main.go          # Data initialization
├── ent/                       # Ent ORM (code generation)
│   └── schema/               # Schema definitions (handwritten)
├── internal/
│   ├── api/
│   │   ├── handlers/         # HTTP handlers
│   │   └── middleware/       # Middleware
│   ├── app/
│   │   └── bootstrap.go      # Manual DI composition root
│   ├── config/               # Configuration
│   ├── domain/               # Domain models
│   ├── governance/           # Approval & audit
│   ├── infrastructure/       # Database, connections
│   ├── pkg/                  # Internal shared packages
│   │   ├── errors/
│   │   ├── logger/
│   │   └── worker/
│   ├── provider/             # K8s provider
│   ├── repository/           # Data access
│   ├── service/              # Business logic
│   └── usecase/              # Clean Architecture use cases
├── migrations/               # Database migrations
├── templates/                # YAML templates
├── scripts/ci/               # CI check scripts
├── .github/workflows/
└── Makefile
```

---

## 2. Configuration Management

> **Reference Implementation**: [examples/config/config.go](../examples/config/config.go)

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Viper for config | Standard Go config library, supports file + env |
| Environment variable prefix | `KUBEVIRT_SHEPHERD_` |
| Shared connection pool | ADR-0012: Ent + River + sqlc share same pgxpool |
| PostgreSQL for sessions | Redis removed, sessions stored in PostgreSQL |

### Configuration Sources (Priority)

1. Environment variables (highest)
2. Config file (`config.yaml`)
3. Default values (lowest)

---

## 3. Logging System

### Design Principles

- Use `zap` for structured logging
- `AtomicLevel` for hot-reload support
- JSON format for production, console for development

### Hot-Reload Support

| Config | Effect | Implementation |
|--------|--------|----------------|
| `log.level` | Immediate | `zap.AtomicLevel` |
| `rate_limit.*` | Immediate | `atomic.Int64` |
| `k8s.per_cluster_limit` | Progressive | New clusters use new value |
| `database.*` | Requires restart | Pool created at startup |

---

## 4. Worker Pool (Coding Standard - Required)

> **Reference Implementation**: [examples/worker/pool.go](../examples/worker/pool.go)

### Rule: Naked Goroutines Are Forbidden

All concurrency must go through Worker Pool:

```go
// ❌ Forbidden
go func() {
    someOperation()
}()

// ✅ Correct
pools.General.Submit(func() {
    someOperation()
})
```

### Why?

| Issue | Naked goroutine | Worker Pool |
|-------|-----------------|-------------|
| Concurrency count | ❌ Uncontrolled | ✅ Configurable limit |
| Panic handling | ❌ Must write each time | ✅ Unified recovery |
| Resource reclamation | ❌ No guarantee | ✅ Pool managed |
| Observability | ❌ No metrics | ✅ Exposable metrics |

### CI Enforcement

See [ci/scripts/check_naked_goroutine.go](../ci/scripts/check_naked_goroutine.go)

---

## 5. Health Checks

> **Reference Implementation**: [examples/handlers/health.go](../examples/handlers/health.go)

### Endpoints

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `/health/live` | Liveness probe | Process responsive |
| `/health/ready` | Readiness probe | DB, River Worker, ResourceWatchers |

### Worker Health Monitoring

| Worker | Heartbeat Timeout | Injected In |
|--------|-------------------|-------------|
| River Worker | 60s | Phase 4 |
| ResourceWatcher | 120s | Phase 2 |

---

## 6. Database Connection

> **Reference Implementation**: [examples/infrastructure/database.go](../examples/infrastructure/database.go)

### ADR-0012: Shared Connection Pool

```go
// Single pgxpool for all components
DatabaseClients{
    Pool:        pgxpool.Pool      // Shared pool
    EntClient:   ent.Client        // Uses stdlib.OpenDBFromPool
    SqlcQueries: sqlc.Queries      // Uses pool directly
}
```

### Why Share Pool?

- Prevents connection count doubling
- Enables atomic transactions across Ent, sqlc, River
- Simplifies connection management

---

## 7. CI Pipeline

### Check Scripts

| Script | Purpose | Blocks CI |
|--------|---------|-----------|
| `check_naked_goroutine.go` | Forbid naked `go func()` | ✅ Yes |
| `check_manual_di.sh` | Strict manual DI | ✅ Yes |
| `check_no_redis_import.sh` | Forbid Redis imports | ✅ Yes |
| `check_ent_codegen.go` | Ent code sync | ✅ Yes |
| `check_transaction_boundary.go` | Service layer no TX | ✅ Yes |
| `check_k8s_in_transaction.go` | No K8s in TX | ✅ Yes |

See [ci/README.md](../ci/README.md) for complete list.

### Phased CI Strategy

| Phase | CI Checks |
|-------|-----------|
| Phase 0 | lint, build, basic standards (no Ent) |
| Phase 1+ | Full checks including Ent sync |

---

## 8. Data Initialization

### Required Seeds

| Data | Purpose |
|------|---------|
| Super admin | Initial admin account |
| System config | Default VM limits |
| Quota template | Default tenant quotas |
| Approval policy | Default approval rules |

### Execution Order

```bash
# 1. Atlas migration (business tables)
atlas migrate apply --dir file://migrations/atlas --url $DATABASE_URL

# 2. River migration (job queue tables)
river migrate-up --database-url $DATABASE_URL

# 3. Data seeding
SEED_ADMIN_PASSWORD=your_secure_password go run ./cmd/seed
```

---

## Acceptance Criteria

- [ ] `go build ./...` no errors
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` no errors
- [ ] Docker image builds successfully
- [ ] `/health/live` returns 200
- [ ] `/health/ready` checks database
- [ ] `make seed` initializes admin account
- [ ] River migration tables created

---

## Related Documentation

- [DEPENDENCIES.md](../DEPENDENCIES.md) - Version definitions
- [CHECKLIST.md](../CHECKLIST.md) - Acceptance checklist
- [examples/](../examples/) - Code examples
- [ci/README.md](../ci/README.md) - CI scripts
- [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) - Hybrid transaction
- [ADR-0013](../../adr/ADR-0013-manual-di.md) - Manual DI
