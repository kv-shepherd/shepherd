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

> **Updated per ADR-0018**: Templates are stored in PostgreSQL, not as YAML files.

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
├── config/                   # Configuration files
│   ├── seed/                 # Seed data (templates, instance_sizes) - loaded to PostgreSQL
│   └── mask.yaml             # Field visibility configuration
├── scripts/ci/               # CI check scripts
├── .github/workflows/
└── Makefile
```

> **Note**: `templates/` directory removed per ADR-0018. All templates stored in PostgreSQL database.

---

## 2. Configuration Management

> **Reference Implementation**: [examples/config/config.go](../examples/config/config.go)

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Viper for config | Standard Go config library, supports file + env |
| Standard env vars | ADR-0018: `DATABASE_URL`, `SERVER_PORT`, `LOG_LEVEL` (no prefix) |
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

## 7.5 PostgreSQL Stability (ADR-0008) ⚠️ CRITICAL

> **Risk**: River job queue tables experience high-frequency inserts/updates/deletes.
> Without aggressive autovacuum, tables will bloat and severely degrade performance.

### Required Deployment SQL

```sql
-- River job table: aggressive autovacuum (vacuum earlier, at 1% dead tuples instead of 20%)
ALTER TABLE river_job SET (
    autovacuum_vacuum_scale_factor = 0.01,  -- 1% threshold (default: 0.2 = 20%)
    autovacuum_vacuum_threshold = 1000,     -- minimum dead tuples before vacuum
    autovacuum_analyze_scale_factor = 0.01, -- frequent statistics update
    autovacuum_analyze_threshold = 500
);

-- If using audit_logs with high write volume, apply similar settings
ALTER TABLE audit_logs SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 5000
);
```

### River Built-in Cleanup

```go
// River client configuration
riverClient, _ := river.NewClient(riverpgxv5.New(pool), &river.Config{
    // Automatically delete completed jobs after 24 hours
    CompletedJobRetentionPeriod: 24 * time.Hour,
})
```

### Monitoring

| Metric | Warning | Critical |
|--------|---------|----------|
| `river_dead_tuple_ratio` | > 10% | > 30% |
| `pg_stat_user_tables.n_dead_tup` | Review | Vacuum immediately |

### Verification Query

```sql
SELECT relname, n_dead_tup, n_live_tup,
       round(100.0 * n_dead_tup / nullif(n_live_tup + n_dead_tup, 0), 2) as dead_ratio
FROM pg_stat_user_tables
WHERE relname LIKE 'river%' OR relname = 'audit_logs'
ORDER BY dead_ratio DESC;
```

---

## 8. Data Initialization (ADR-0018)

> **Design**: Application auto-initializes on first startup. See [ADR-0018 §First Deployment](../../adr/ADR-0018-instance-size-abstraction.md) and [master-flow.md Stage 1](../interaction-flows/master-flow.md).

### Auto-Initialization Flow

Application performs these steps on startup (idempotent, `ON CONFLICT DO NOTHING`):

1. **Run Atlas migrations** - Schema changes
2. **Run River migrations** - Job queue tables
3. **Seed built-in roles** - PlatformAdmin, Approver, Viewer (do not overwrite existing)
4. **Seed default admin** - `admin/admin` with `force_password_change=true`

### First Login Experience

- User logs in with `admin/admin`
- System forces password change before any other action
- After password change, `force_password_change` flag cleared

### Required Seeds

| Data | Purpose | Idempotent |
|------|---------|------------|
| Super admin | Initial admin account (`admin/admin`) | ✅ `ON CONFLICT DO NOTHING` |
| Built-in roles | PlatformAdmin, Approver, Viewer | ✅ `ON CONFLICT DO NOTHING` |
| Default quota | Tenant quota template | ✅ `ON CONFLICT DO NOTHING` |

### Manual Migration (Development/CI)

For explicit control outside auto-init:

```bash
# 1. Atlas migration (business tables)
atlas migrate apply --dir file://migrations/atlas --url $DATABASE_URL

# 2. River migration (job queue tables)
river migrate-up --database-url $DATABASE_URL

# 3. Application auto-seeds on first startup
go run cmd/server/main.go
```

---

## Acceptance Criteria

- [ ] `go build ./...` no errors
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` no errors
- [ ] Docker image builds successfully
- [ ] `/health/live` returns 200
- [ ] `/health/ready` checks database
- [ ] First startup auto-seeds admin account
- [ ] River migration tables created

---

## Related Documentation

- [DEPENDENCIES.md](../DEPENDENCIES.md) - Version definitions
- [CHECKLIST.md](../CHECKLIST.md) - Acceptance checklist
- [examples/](../examples/) - Code examples
- [ci/README.md](../ci/README.md) - CI scripts
- [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) - Hybrid transaction
- [ADR-0013](../../adr/ADR-0013-manual-di.md) - Manual DI
- [ADR-0016](../../adr/ADR-0016-go-module-vanity-import.md) - Vanity Import

---

## ADR-0016: Vanity Import Deployment

> **Required for `go get kv-shepherd.io/shepherd` to work**

The vanity import server must be deployed before external users can import the module.

### Deployment Options (per ADR-0016)

| Option | Complexity | Recommended For |
|--------|-----------|-----------------|
| **Cloudflare Pages** (Recommended) | Low | Projects using Cloudflare DNS |
| Static HTML | Low | Any web host |
| [govanityurls](https://github.com/GoogleCloudPlatform/govanity) | Medium | Programmatic management |

### Quick Setup (Cloudflare Pages)

1. Create Cloudflare Pages project for `kv-shepherd.io`
2. Deploy static HTML with `go-import` meta tag:

```html
<!-- public/shepherd/index.html -->
<!DOCTYPE html>
<html>
<head>
    <meta name="go-import" content="kv-shepherd.io/shepherd git https://github.com/kv-shepherd/shepherd">
    <meta name="go-source" content="kv-shepherd.io/shepherd https://github.com/kv-shepherd/shepherd https://github.com/kv-shepherd/shepherd/tree/main{/dir} https://github.com/kv-shepherd/shepherd/blob/main{/dir}/{file}#L{line}">
    <meta http-equiv="refresh" content="0; url=https://github.com/kv-shepherd/shepherd">
</head>
<body>Redirecting...</body>
</html>
```

3. Verify: `go get kv-shepherd.io/shepherd@latest`

### Status

- [ ] Domain DNS configured
- [ ] Vanity import server deployed
- [ ] `go get` verification passed

