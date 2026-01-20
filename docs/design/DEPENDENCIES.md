# Dependency Version Definitions (Single Source of Truth)

> **Purpose**: Authoritative source for all dependency versions. Other documents reference this file.  
> **Validation**: All versions verified via go.dev / github releases  
> **Key Decision**: ADR-0012 Hybrid Atomic Transaction Strategy (Ent + sqlc)

---

## Document Purpose

**This file is the single source of truth for dependency versions.**

- Other documents must not define versions, only reference this file
- Version changes only happen here
- CI checks verify other documents don't contain hardcoded versions

---

## Go Version

| Item | Version | Notes |
|------|---------|-------|
| **Go** | `1.25.6` | **Recommended latest stable** (released 2026-01-15, includes security patches) |

> **Go Version Strategy**: 
> - **Minimum**: Go 1.24 (required by `kubevirt.io/client-go` v1.7.0)
> - **Recommended**: Go 1.25.6 (latest stable with security patches)
> - Gin v1.11.0 requires Go 1.23+, KubeVirt client-go requires Go 1.24+
> - Unified on **Go 1.25.6**, backward compatible with 1.24 code

---

## Core Dependencies

> **Version Selection Strategy**: 
> - Use exact versions, prefer mature versions with multiple patches
> - All versions verified via `proxy.golang.org` on 2026-01-19

> **Version Verification Method**:
> ```bash
> curl -s "https://proxy.golang.org/<package>/@v/list" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1
> ```

### Web Framework Layer

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `github.com/gin-gonic/gin` | `v1.11.0` | 2025-09 | High-performance web framework |
| `github.com/go-playground/validator/v10` | `v10.25.0` | 2025-12 | Struct validation (Gin dependency) |

### Database Layer (PostgreSQL + Ent)

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| **PostgreSQL** | `18` | 2025-09 | Database (latest stable) |
| `github.com/jackc/pgx/v5` | `v5.8.0` | 2025-12 | PostgreSQL driver (best performance) |
| `entgo.io/ent` | `v0.14.5` | 2025-07 | Entity framework (type-safe ORM, **latest stable**) |
| `ariga.io/atlas` | `v1.0.0` | 2025-12 | Schema migration tool (GA release) |
| `ariga.io/atlas-go-sdk` | `v0.10.0` | 2025-12 | Atlas Go SDK |
| `github.com/riverqueue/river` | `v0.30.0` | 2026-01 | PostgreSQL-native job queue (**latest stable**) |
| `github.com/sqlc-dev/sqlc` | `v1.30.0` | 2025-09 | Type-safe SQL code generation (**ADR-0012 core transaction**) |

### Connection Pool Architecture (ADR-0012 Update)

> **Default Mode**: Ent + River + sqlc share a **single pgxpool**
>
> ADR-0012 uses shared connection pool to avoid doubling connections.

| Mode | Use Case | Pool Count | Notes |
|------|----------|------------|-------|
| **Shared Pool (Default)** | Direct PostgreSQL connection | 1 pgxpool | ADR-0012 recommended |
| Dual Pool (Advanced) | PgBouncer environment | 2 pgxpools | [Backlog reserved](../rfc/RFC-0009-pgbouncer.md) |

#### Default Configuration: Shared Single Pool

| Parameter | Default | Environment Variable | Description |
|-----------|---------|---------------------|-------------|
| `DB_POOL_MAX_CONNS` | `50` | `DB_POOL_MAX_CONNS` | pgxpool max connections |
| `DB_POOL_MIN_CONNS` | `5` | `DB_POOL_MIN_CONNS` | pgxpool min connections |
| `DB_MAX_CONN_LIFETIME` | `1h` | `DB_MAX_CONN_LIFETIME` | Max connection lifetime |

#### Initialization Code Example (ADR-0012 Shared Pool)

```go
// internal/infrastructure/database.go

package infrastructure

import (
    "context"
    "database/sql"
    "fmt"
    
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5/stdlib"
    
    "entgo.io/ent/dialect"
    entsql "entgo.io/ent/dialect/sql"
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/riverdriver/riverpgxv5"
    
    "github.com/CloudPasture/kubevirt-shepherd/ent"
    "github.com/CloudPasture/kubevirt-shepherd/internal/repository/sqlc"
)

// DatabaseClients - Database client container (ADR-0012 shared pool)
// Coding Standard: Use this struct to manage connection pools
type DatabaseClients struct {
    // Pool - Shared connection pool (Ent + River + sqlc reuse)
    Pool *pgxpool.Pool
    
    // EntClient - Ent ORM client
    EntClient *ent.Client
    
    // SqlcQueries - sqlc query client (for core transactions)
    SqlcQueries *sqlc.Queries
    
    // Optional: Separate WorkerPool for PgBouncer scenarios
    WorkerPool *pgxpool.Pool
}

// NewDatabaseClients creates database clients (default shared pool)
func NewDatabaseClients(ctx context.Context, dsn string, cfg PoolConfig) (*DatabaseClients, error) {
    // Create shared connection pool
    poolConfig, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("parse pool config: %w", err)
    }
    poolConfig.MaxConns = cfg.MaxConns
    poolConfig.MinConns = cfg.MinConns
    poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
    
    pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }
    
    // Ent Client (reuse pgxpool via stdlib.OpenDBFromPool)
    entDB := stdlib.OpenDBFromPool(pool)
    entDriver := entsql.OpenDB(dialect.Postgres, entDB)
    entClient := ent.NewClient(ent.Driver(entDriver))
    
    // sqlc Queries (use pgxpool directly)
    sqlcQueries := sqlc.New(pool)
    
    return &DatabaseClients{
        Pool:        pool,
        EntClient:   entClient,
        SqlcQueries: sqlcQueries,
    }, nil
}

// Close closes all connection pools
func (c *DatabaseClients) Close() {
    if c.EntClient != nil {
        c.EntClient.Close()
    }
    if c.WorkerPool != nil {
        c.WorkerPool.Close()
    }
    if c.Pool != nil {
        c.Pool.Close()
    }
}
```

> **PostgreSQL Stability Guarantees**
>
> See [ADR-0008-postgresql-stability.md](../../adr/ADR-0008-postgresql-stability.md)
>
> **Adopted Approach**: River built-in cleanup + Autovacuum aggressive tuning
>
> | Measure | Description |
> |---------|-------------|
> | **River Job Cleaner** | Built-in cleanup, configurable `CompletedJobRetentionPeriod` |
> | **Autovacuum Tuning** | Aggressive settings for `river_job` table (`scale_factor=0.01`) |
> | **Dead Tuple Monitoring** | Prometheus metrics + alert thresholds |
>
> **Roadmap**: See [RFC-0001 pg_partman Table Partitioning](../../rfc/RFC-0001-pg-partman.md)

> **Decision Record**: [ADR-0003-database-orm.md](../../adr/ADR-0003-database-orm.md)

### Kubernetes Layer

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `k8s.io/client-go` | `v0.34.0` | 2025-12 | K8s official client (aligned with K8s 1.34) |
| `k8s.io/apimachinery` | `v0.34.0` | 2025-12 | K8s API machinery |
| `k8s.io/api` | `v0.34.0` | 2025-12 | K8s API types |
| `kubevirt.io/client-go` | `v1.7.0` | 2025-11-27 | **KubeVirt official client** (Informer usage) |
| `kubevirt.io/api` | `v1.7.0` | 2025-11-27 | KubeVirt API type definitions |
| `sigs.k8s.io/controller-runtime` | `v0.22.4` | 2025-11-03 | **SSA Apply core dependency** (ADR-0011), compatible with k8s.io v0.34.0 |

> **ADR-0011 SSA Apply Strategy**:
> 
> `controller-runtime` provides `client.Apply` (Server-Side Apply) capability.
> 
> | Use Case | Package | Description |
> |----------|---------|-------------|
> | **SSA Resource Submission** | `sigs.k8s.io/controller-runtime/pkg/client` | `client.Patch(..., client.Apply)` |
> | **Unstructured Operations** | `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` | Dynamic object decoding |
> | **Informer Event Parsing** | `kubevirt.io/api` | Type-safe event handling |
>
> **Decision Record**: [ADR-0011-ssa-apply-strategy.md](../../adr/ADR-0011-ssa-apply-strategy.md)

> **Important**: Use KubeVirt official client-go for type-safe VM/VMI operations
>
> **Decision Record**: [ADR-0001-kubevirt-client.md](../../adr/ADR-0001-kubevirt-client.md)

> **Version Compatibility Constraints**:
> - `kubevirt.io/client-go` v1.7.0 is built for **Kubernetes v1.34**
> - Also supports K8s v1.32 ~ v1.34
> - **Must** use `k8s.io/client-go` **v0.34.x** series
> - **Do not** upgrade to `k8s.io/client-go` v0.35.x+ (API type incompatibility)
> - All three k8s.io packages must use **exactly the same version**

> **Compatibility Verification Record** (2026-01-19):
>
> | Package Pair | Status | Verification Method |
> |--------------|--------|---------------------|
> | `controller-runtime v0.22.4` + `client-go v0.34.0` | ✅ Compatible | Official compatibility matrix |
> | `kubevirt.io/client-go v1.7.0` + `client-go v0.34.0` | ✅ Compatible | KubeVirt v1.7.0 built for K8s 1.34 |
> | `controller-runtime v0.22.4` + `kubevirt.io/client-go v1.7.0` | ✅ Compatible | Both use `client-go v0.34.x` |
>
> **Note**: Actual compatibility must be verified via `go mod tidy && go build` during Phase 0.

### K8s Dependency Hell Prevention

> **Dependency Hell Risk**:
> 
> `kubevirt.io/client-go` depends on specific versions of `k8s.io/*` packages. Introducing other K8s ecosystem libraries
> (like `controller-runtime`, Operator SDK) can easily cause version conflicts.

#### go.mod Replace Directive Strategy

> **Core Principle: Minimize replace, only use for these scenarios**
>
> | Scenario | Allow replace | Reason |
> |----------|--------------|--------|
> | **K8s core package version locking** | ✅ Yes | Kubernetes ecosystem specificity |
> | **Permanent fork redirect** | ✅ Yes | e.g., `goforj/wire` replacing `google/wire` |
> | **Other dependency conflicts** | ❌ No | Prefer removing conflicting features/libraries |
> | **Local development debugging** | ❌ No | Use `go.work` instead |

```go
// go.mod

module github.com/CloudPasture/kubevirt-shepherd

go 1.25

require (
    kubevirt.io/client-go v1.7.0
    kubevirt.io/api v1.7.0
    k8s.io/client-go v0.34.0
    k8s.io/apimachinery v0.34.0
    k8s.io/api v0.34.0
)

// Force lock K8s dependency versions
replace (
    k8s.io/api => k8s.io/api v0.34.0
    k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.0
    k8s.io/apimachinery => k8s.io/apimachinery v0.34.0
    k8s.io/apiserver => k8s.io/apiserver v0.34.0
    k8s.io/client-go => k8s.io/client-go v0.34.0
    k8s.io/component-base => k8s.io/component-base v0.34.0
)
```

### Session Storage (PostgreSQL Native)

> **Architecture Simplification**:
> 
> **Removed Redis**, Session storage uses PostgreSQL:
> - ✅ Unified tech stack: Only depends on PostgreSQL
> - ✅ Instant revocation: Session invalidated immediately (delete record)
> - ✅ Simplified supply chain: Reduced operational complexity

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `github.com/alexedwards/scs/v2` | `v2.8.0` | 2025-10 | HTTP Session management (OWASP security spec) |
| `github.com/alexedwards/scs/postgresstore` | `v2.8.0` | 2025-10 | PostgreSQL Session Store |
| `github.com/sony/gobreaker` | `v1.0.0` | Stable | Circuit breaker pattern (ResourceWatcher usage) |

> **Distributed Lock Best Practices**:
> 
> **Use PostgreSQL Advisory Lock instead of Redis Lock**:
> - ✅ Strong consistency: Lock tied to database transaction
> - ✅ Auto-release: Use `pg_advisory_xact_lock`, auto-releases on transaction end
> - ✅ Reduced components: No need for additional Redis lock library

### Logging and Observability

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `go.uber.org/zap` | `v1.27.1` | 2025-11-19 | High-performance structured logging |
| `go.opentelemetry.io/otel` | `v1.39.0` | 2025-12-08 | OpenTelemetry API |
| `go.opentelemetry.io/otel/sdk` | `v1.39.0` | 2025-12-08 | OpenTelemetry SDK |
| `go.opentelemetry.io/otel/exporters/prometheus` | `v0.61.0` | 2025-12 | Prometheus exporter |
| `github.com/prometheus/client_golang` | `v1.21.0` | 2025-12 | Prometheus client |

### Authentication and Security

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `github.com/golang-jwt/jwt/v5` | `v5.3.0` | 2025-07-30 | JWT handling |
| `golang.org/x/crypto` | `v0.37.0` | 2026-01 | Crypto utilities (argon2, bcrypt) |
| `github.com/go-ldap/ldap/v3` | `v3.4.10` | 2025 | LDAP authentication |

### Configuration and Tools

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `github.com/spf13/viper` | `v1.21.0` | 2025-09-08 | Configuration management |
| `github.com/spf13/cobra` | `v1.9.1` | 2025 | CLI framework |
| `gopkg.in/yaml.v3` | `v3.0.1` | Stable | YAML parsing |
| `github.com/robfig/cron/v3` | `v3.0.1` | Stable | Cron expression parsing |
| `github.com/google/uuid` | `v1.6.0` | 2025 | UUID generation |

### Dependency Injection (Strict Manual DI)

> **Architecture Simplification**:
> 
> **Removed Wire, adopted strict Manual DI pattern**:
> - ✅ Zero supply chain risk: Only depends on Go compiler
> - ✅ Developer-friendly: Explicit code more transparent than code generators
> - ✅ Shortest feedback loop: `go build` checks directly, no need for `wire gen`
> - ✅ WYSIWYG: No generated code to trace during debugging

> **Why Abandon Wire**:
> 
> | Dimension | Wire (goforj) | Strict Manual DI |
> |-----------|---------------|------------------|
> | Boilerplate code | Less | More (acceptable trade-off) |
> | Compile check | Requires running wire | `go build` checks directly |
> | Debug difficulty | Medium (need to view generated code) | Low (WYSIWYG) |
> | Supply chain | 🔴 High risk (Fork library) | 🟢 Zero risk (stdlib) |
> | Potential misuse | Can misuse Wire DSL | Compiler intercepts directly |
>
> **Conclusion**: Wire's core value (reducing boilerplate) is outweighed by explicit code transparency.

### Worker Pool (Coding Standard - Required)

> **Naked goroutines are forbidden**: All concurrency must go through Worker Pool

| Package | Version | Description |
|---------|---------|-------------|
| `github.com/panjf2000/ants/v2` | `v2.11.4` | High-performance goroutine pool |
| `golang.org/x/sync` | `v0.12.0` | Semaphore and errgroup |

> **Why Forbid Naked `go func()`**:
> 
> | Issue | Naked goroutine | Worker Pool |
> |-------|-----------------|-------------|
> | Concurrency count | ❌ Uncontrolled | ✅ Configurable limit |
> | Panic handling | ❌ Must write each time | ✅ Unified recovery |
> | Resource reclamation | ❌ No guarantee | ✅ Pool managed |
> | Observability | ❌ No metrics | ✅ Can expose metrics |
> | Code consistency | ❌ Variable patterns | ✅ Unified pattern |

> **ants vs River Workers - Clear Separation of Responsibilities**:
>
> | Component | Purpose | Usage Scope |
> |-----------|---------|-------------|
> | **River Workers** | Job queue consumption | All async write operations (VM create, delete, etc.) |
> | **ants Pool** | General concurrency | Non-River async tasks (ResourceWatcher, batch reads, etc.) |
>
> ⚠️ **Anti-Pattern**: Do NOT use ants Pool inside River Worker handlers. River has built-in concurrency control.
>
> ```go
> // ❌ Forbidden: Using ants inside River Worker
> func (w *EventWorker) Work(ctx context.Context, job *river.Job[EventJobArgs]) error {
>     pools.General.Submit(func() { ... }) // DON'T DO THIS
> }
>
> // ✅ Correct: River Worker executes synchronously, River controls concurrency
> func (w *EventWorker) Work(ctx context.Context, job *river.Job[EventJobArgs]) error {
>     return w.processEvent(ctx, job.Args.EventID) // Direct execution
> }
> ```

### Template Engine

> **Helm Basic Syntax Compatible**: Uses same template engine as Helm (Go text/template + Sprig)

| Package | Version | Release Date | Description |
|---------|---------|--------------|-------------|
| `text/template` | stdlib | - | Go standard template engine |
| `github.com/Masterminds/sprig/v3` | `v3.3.0` | 2024-08-29 | Template function extension (same as Helm) |
| `sigs.k8s.io/yaml` | `v1.4.0` | Stable | YAML parsing to K8s objects |

### Test Dependencies

| Package | Version | Description |
|---------|---------|-------------|
| `github.com/stretchr/testify` | `v1.10.0` | Test assertion library |
| `go.uber.org/mock` | `v0.5.2` | Mock generation (uber maintained) |
| `github.com/testcontainers/testcontainers-go` | `v0.40.0` | Docker container testing (replaces SQLite) |
| `github.com/testcontainers/testcontainers-go/modules/postgres` | `v0.40.0` | PostgreSQL module |
| `sigs.k8s.io/controller-runtime` | `v0.22.4` | Test environment (envtest) |

> **Test Database Strategy**:
> 
> **Completely removed SQLite, unified on PostgreSQL**
> 
> | Scenario | Solution | Description |
> |----------|----------|-------------|
> | **Local Development** | testcontainers-go | Auto-starts Docker PostgreSQL container |
> | **CI (GitHub Actions)** | Service Container | postgres:18 container |

---

## Middleware Versions

| Middleware | Version | Support Cycle |
|------------|---------|---------------|
| **PostgreSQL** | `18.x` | Latest stable |
| **Kubernetes** | `1.32+` | Test baseline 1.34 (aligned with kubevirt.io/client-go v1.7.0) |
| **KubeVirt** | `1.6+` | Recommended 1.7+ |

> **Database Selection**: PostgreSQL, supports JSONB indexes, transactional DDL, SKIP LOCKED
>
> **Redis Removed**: See [ADR-0013](../../adr/ADR-0013-manual-di.md), Session storage uses PostgreSQL + alexedwards/scs

---

## Configuration Parameters

> **Single Source**: All configuration parameter defaults defined here

### Database Connection Pool (pgxpool terminology)

| Parameter | Default | Environment Variable | Constraint |
|-----------|---------|---------------------|------------|
| `DB_POOL_MAX_CONNS` | `50` | `DB_POOL_MAX_CONNS` | Max connections |
| `DB_POOL_MIN_CONNS` | `5` | `DB_POOL_MIN_CONNS` | Min connections |
| `DB_CONN_MAX_LIFETIME` | `1h` | `DB_CONN_MAX_LIFETIME` | Max connection lifetime |
| `DB_CONN_MAX_IDLE_TIME` | `10m` | `DB_CONN_MAX_IDLE_TIME` | Idle connection timeout |

### Concurrency Control

> **Stability First Principle**: This platform is a governance platform, not a high-concurrency scheduling platform.
> In high-concurrency scenarios, use **batching and queuing**. Stability and consistency are top priority.

| Parameter | Default | Environment Variable | Constraint |
|-----------|---------|---------------------|------------|
| `K8S_CLUSTER_CONCURRENCY` | `20` | `K8S_CLUSTER_CONCURRENCY` | Single cluster K8s operation concurrency limit |
| `HEAVY_WRITE_LIMIT` | `30` | `HEAVY_WRITE_LIMIT` | Heavy write operations (K8s API, external systems) |
| `LIGHT_WRITE_LIMIT` | `80` | `LIGHT_WRITE_LIMIT` | Light write operations (pure DB) |
| `RIVER_MAX_WORKERS` | `10` | `KUBEVIRT_SHEPHERD_RIVER_MAX_WORKERS` | River Worker max concurrency |

### HPA Concurrency Constraints (Required)

> **Key Constraint**: With multiple replicas, total concurrency = Pod count × per-instance config. Must follow these formulas.

| Constraint Formula | Upper Limit | Description |
|-------------------|-------------|-------------|
| `HPA.maxReplicas × RIVER_MAX_WORKERS` | **≤ 50** | Global River Worker total concurrency |
| `HPA.maxReplicas × K8S_CLUSTER_CONCURRENCY` | **≤ 60** | Single cluster K8s operation total concurrency |

> **Calculation Examples**:
>
> | Scenario | maxReplicas | RIVER_MAX_WORKERS | Total Workers | Status |
> |----------|-------------|-------------------|---------------|--------|
> | ✅ Recommended | 3 | 10 | 30 | Safe |
> | ✅ Conservative | 5 | 10 | 50 | At limit |
> | ❌ Over limit | 6 | 10 | 60 | Exceeds 50 |
>
> **Why these limits?**
> - PostgreSQL connection pool typically sized at 50-100 connections
> - Each River Worker holds a connection during job execution
> - Excessive workers can exhaust connections, causing job failures
> - K8s API server has rate limiting; too many concurrent requests cause throttling

### Distributed Lock and Timeout (PostgreSQL Advisory Lock)

> **Use PostgreSQL Advisory Lock instead of Redis Lock**:
> - Lock tied to database transaction, PostgreSQL is a hard dependency
> - Auto-releases on transaction end, no watchdog mechanism needed

| Parameter | Default | Description |
|-----------|---------|-------------|
| `K8S_OPERATION_TIMEOUT` | `5m` | K8s operation hard timeout |
| `DB_LOCK_TIMEOUT` | `10s` | Advisory Lock acquisition timeout |

### Cache TTL

| Cache Type | TTL | Description |
|------------|-----|-------------|
| `list_resources` | `5s` | List cache |
| `get_resource` | `3s` | Single resource cache |

---

## go.mod Template

```go
module github.com/CloudPasture/kubevirt-shepherd

go 1.25

require (
    // Web framework
    github.com/gin-gonic/gin v1.11.0
    
    // Database (PostgreSQL + Ent + River)
    entgo.io/ent v0.14.5
    ariga.io/atlas v1.0.0
    github.com/jackc/pgx/v5 v5.8.0
    github.com/riverqueue/river v0.30.0
    
    // Kubernetes (must match kubevirt.io/client-go v1.7.0)
    k8s.io/client-go v0.34.0
    k8s.io/apimachinery v0.34.0
    k8s.io/api v0.34.0
    kubevirt.io/client-go v1.7.0
    kubevirt.io/api v1.7.0
    
    // Session storage (replaces Redis)
    github.com/alexedwards/scs/v2 v2.8.0
    github.com/alexedwards/scs/postgresstore v2.8.0
    
    // Logging and observability
    go.uber.org/zap v1.27.1
    go.opentelemetry.io/otel v1.39.0
    
    // Worker Pool (Coding Standard - Required)
    github.com/panjf2000/ants/v2 v2.11.4
    golang.org/x/sync v0.12.0
    
    // Testing (unified PostgreSQL, removed SQLite)
    github.com/stretchr/testify v1.10.0
    github.com/testcontainers/testcontainers-go v0.40.0
)

// Force lock K8s dependency versions
replace (
    k8s.io/api => k8s.io/api v0.34.0
    k8s.io/apimachinery => k8s.io/apimachinery v0.34.0
    k8s.io/client-go => k8s.io/client-go v0.34.0
)
```

---

## Toolchain

| Tool | Version | Purpose |
|------|---------|---------|
| `golangci-lint` | `v1.63.0` | Static code analysis |
| `goimports` | Latest | Import formatting |
| `mockgen` | `v0.5.2` | Mock generation (uber-go/mock) |
| `swag` | `v2.0.1` | Swagger documentation generation |

---

## Version Upgrade Guide

### Upgrading KubeVirt client-go

1. Check [KubeVirt Release Notes](https://github.com/kubevirt/kubevirt/releases)
2. Confirm compatible `k8s.io/client-go` version
3. **Simultaneously update** all three k8s.io packages to same version
4. Update versions in `go.mod`
5. Run `go mod tidy`
6. Run full test suite
7. Verify CI passes

### Upgrading K8s Dependencies

⚠️ **Warning**: K8s dependency versions **must be consistent**

```bash
# Upgrade all k8s.io packages simultaneously
go get k8s.io/client-go@v0.34.0
go get k8s.io/apimachinery@v0.34.0
go get k8s.io/api@v0.34.0
go mod tidy
```

### Verifying Dependency Compatibility

```bash
# Check dependency conflicts
go mod tidy
go mod verify

# Run tests
go test -race ./...

# Check K8s version consistency
go list -m k8s.io/client-go k8s.io/apimachinery k8s.io/api
```

---

## References

- [ADR-0001: KubeVirt Client Selection](../../adr/ADR-0001-kubevirt-client.md)
- [ADR-0003: Database ORM Selection](../../adr/ADR-0003-database-orm.md)
- [ADR-0006: Unified Async Model](../../adr/ADR-0006-unified-async-model.md)
- [ADR-0008: PostgreSQL Stability](../../adr/ADR-0008-postgresql-stability.md)
- [ADR-0011: SSA Apply Strategy](../../adr/ADR-0011-ssa-apply-strategy.md)
- [ADR-0012: Hybrid Transaction](../../adr/ADR-0012-hybrid-transaction.md)
- [ADR-0013: Manual DI](../../adr/ADR-0013-manual-di.md)
