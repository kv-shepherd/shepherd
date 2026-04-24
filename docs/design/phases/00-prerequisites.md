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

---

## Deliverables

| Deliverable | File Path | Status | Example |
|-------------|-----------|--------|---------|
| Go module | `go.mod`, `go.sum` | ✅ | - |
| Entry point | `cmd/server/main.go` | ✅ | - |
| Configuration | `internal/config/config.go` | ✅ | [examples/config/config.go](../examples/config/config.go) |
| Logging | `internal/pkg/logger/logger.go` | ✅ | - |
| Health checks | `internal/api/handlers/health.go` | ✅ | [examples/handlers/health.go](../examples/handlers/health.go) |
| Database | `internal/infrastructure/database.go` | ✅ | [examples/infrastructure/database.go](../examples/infrastructure/database.go) |
| Worker pool | `internal/pkg/worker/pool.go` | ✅ | [examples/worker/pool.go](../examples/worker/pool.go) |
| CI config | `.github/workflows/ci.yml` | ✅ | - |
| Docs governance CI | `.github/workflows/docs-governance.yaml` | ✅ | [.github/workflows/docs-governance.yaml](../../../.github/workflows/docs-governance.yaml) |
| Lint config | `.golangci.yml` | ✅ | - |
| Dockerfile | `Dockerfile` | ✅ | - |
| Data seeding | `cmd/seed/main.go` | ✅ | - |
| River migration | `internal/infrastructure/database.go`, `migrations/river/` | ✅ | *Development auto-migration uses `rivermigrate`; production command documented* |
| Error handling | `internal/pkg/errors/errors.go` | ✅ | *Added: structured AppError types* |
| Error middleware | `internal/api/middleware/error_handler.go` | ✅ | *Added: Gin centralized error handling* |
| OpenAPI validator | `internal/api/middleware/openapi_validator.go` | ✅ | *Request/response runtime validation implemented and wired via router middleware* |
| Modular DI | `internal/app/modules/*.go` | ✅ | *Module interface + infrastructure + domain stubs* |
| API contract CI | `.github/workflows/api-contract-validation.yml` | ✅ | - |
| API tooling | `build/api.mk`, `api/.vacuum.yaml` | ✅ | - |
| Config example | `config/config.yaml.example` | ✅ | - |
| Dependabot | `.github/dependabot.yml` | ✅ | - |

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
├── docs/design/ci/scripts/    # CI check scripts (design-phase artifacts)
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
| JWT auth with PostgreSQL-backed bootstrap secrets | Redis removed; signing/encryption secrets are generated and persisted in PostgreSQL when absent |

### Configuration Classification

> **Clarification**: There are two types of configuration with different storage and management patterns.

| Type | Storage | Management | Examples |
|------|---------|------------|----------|
| **Deployment-time (Infrastructure)** | `config.yaml` / env vars | DevOps at deploy time | `DATABASE_URL`, `SERVER_PORT`, `ENCRYPTION_KEY` |
| **Runtime (Business)** | PostgreSQL | WebUI by admins | Clusters, templates, OIDC config, roles, users |

### Deployment-time Configuration (config.yaml / env vars)

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `DATABASE_URL` | ✅ | PostgreSQL connection string | `postgres://user:pass@host:5432/dbname` |
| `SERVER_PORT` | ❌ | HTTP server port (default: 8080) | `8080` |
| `LOG_LEVEL` | ❌ | Logging level (default: info) | `debug`, `info`, `warn`, `error` |
| `ENCRYPTION_KEY` | ❌ | **AES-256-GCM key for sensitive data** (strongly recommended) | 32-byte hex-encoded key |
| `SESSION_SECRET` | ❌ | JWT signing secret (strongly recommended) | Random 256-bit key (32 bytes) |

**Auto-generation rule** (ADR-0025):
- If `ENCRYPTION_KEY` or `SESSION_SECRET` is missing, generate strong random keys on first boot and persist them in PostgreSQL.
- External key or env var overrides DB value.
- Rotation deferred to RFC-0016.

**Key length guidance**:
- For HMAC JWT signing (HS256/HS384/HS512), use key length at least the hash output size (e.g., 256 bits for HS256). See [RFC 7518](https://www.rfc-editor.org/rfc/rfc7518).
- For secrets storage and rotation practices, see [OWASP Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html).

```bash
# Generate ENCRYPTION_KEY (32 bytes, hex-encoded; matches runtime validation)
openssl rand -hex 32

# Generate SESSION_SECRET (32 bytes; for HS256 or similar HMAC JWT signing)
openssl rand -base64 32

# Example config.yaml (DO NOT commit secrets!)
database:
  url: ${DATABASE_URL}
server:
  port: 8080
security:
  encryption_key: ${ENCRYPTION_KEY}
  session_secret: ${SESSION_SECRET}
```

### Password Policy (NIST 800-63B Compliant)

> **Reference**: [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html) - Digital Identity Guidelines

**Default Policy** (NIST-compliant):

| Requirement | Value | NIST Reference |
|-------------|-------|----------------|
| Minimum length | 8 characters | §3.1.1.2 (absolute minimum) |
| Recommended length | 15+ characters | §3.1.1.2 (best practice) |
| Maximum length | 64+ characters | §3.1.1.2 |
| ❌ Composition rules | **Not enforced** | §3.1.1.2 ("shall not impose") |
| ❌ Periodic expiration | **Not enforced** | §3.1.1.2 ("shall not require") |
| ✅ Blocklist check | Required | §3.1.1.3 (common/breached passwords) |
| ✅ Unicode support | Required | §3.1.1.2 (all printable characters) |

**Optional Legacy Policy** (for enterprises with compliance requirements):

Enterprises can enable traditional complexity rules via configuration:

```yaml
# config.yaml - Optional legacy password policy
security:
  password_policy:
    mode: "nist"          # "nist" (default) or "legacy"
    # Legacy mode only:
    require_uppercase: true
    require_lowercase: true
    require_digit: true
    require_special: false
```

> **ADR Note**: If `mode: legacy` is used, document the compliance reason in deployment notes.

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
> **Decision**: [ADR-0031](../../adr/ADR-0031-concurrency-and-worker-pool-standard.md)

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

See [ci/README.md §Script Summary](../ci/README.md#script-summary) and
[tools/shepherd-linter/README.md](../../../tools/shepherd-linter/README.md) for current architecture-enforcement entry points.

---

## 5. Health Checks

> **Reference Implementation**: [examples/handlers/health.go](../examples/handlers/health.go)

### Endpoints

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `/health/live` | Liveness probe | Process responsive |
| `/health/ready` | Readiness probe | DB, River Worker |

### Worker Health Monitoring

| Worker | Heartbeat Timeout | Injected In |
|--------|-------------------|-------------|
| River Worker | 60s | Phase 4 |

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
| `check_master_flow_traceability.go` | Enforce master-flow traceability manifest (ADR-0032) | ✅ Yes |
| `check_master_flow_test_matrix.go` | Enforce required stage test matrix coverage/deferred debt (ADR-0034) | ✅ Yes |
| `check_design_doc_governance.sh` | Enforce docs layering/link consistency (ADR-0030) | ✅ Yes |

See [ci/README.md §Script Summary](../ci/README.md#script-summary) for complete list.

> **ADR-0030 Requirement**: Docs governance checks are mandatory before coding starts.
> Frontend docs must remain under `docs/design/frontend/` with canonical links from `README.md` and `master-flow.md`.

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

> **Design**: Application auto-initializes on first startup. See [ADR-0018 §Configuration Storage Strategy](../../adr/ADR-0018-instance-size-abstraction.md#configuration-storage-strategy-added-2026-01-26) and [master-flow.md Stage 1.5](../interaction-flows/master-flow.md#stage-1-5).

### Auto-Initialization Flow

Application performs these steps on startup (idempotent, `ON CONFLICT DO NOTHING`):

1. **Run Atlas migrations** - Schema changes
2. **Run River migrations** - Job queue tables
3. **Seed built-in roles** - Complete role set (see below)
4. **Seed default admin** - `admin/admin` with `force_password_change=true`

### First Login Experience

- User logs in with `admin/admin`
- System forces password change before any other action
- After password change, `force_password_change` flag cleared

### Built-in Roles (master-flow Stage 2.A)

> **ADR-0019 / ADR-0053**: Wildcard permissions (`*:*`, `*:read`) and
> compatibility aliases such as `cluster:manage` are **PROHIBITED**. Use
> explicit canonical permissions and scope access through role bindings.

| Role | Permissions | Notes |
|------|-------------|-------|
| **PlatformAdmin** | `platform:admin` | Super-admin, explicit single permission |
| **ApprovalAdmin** | `builtin_approval:approve`, `builtin_approval:view`, `ticket:view`, `vm:read`, `service:read`, `system:read` | Owns approval queue, not platform administration |
| **DevelopmentEngineer** | `system:read`, `system:write`, `service:create`, `service:read`, `vm:create`, `vm:read`, `vm:operate`, `vm:delete`, `vnc:access` | Usually bound to `test`; no platform or approval administration |
| **TestEngineer** | `system:read`, `system:write`, `service:create`, `service:read`, `vm:create`, `vm:read`, `vm:operate`, `vm:delete`, `vnc:access` | Same capability envelope, different assignment policy |
| **SystemOperator** | `system:read`, `system:write`, `service:create`, `service:read`, `vm:create`, `vm:read`, `vm:operate`, `vm:delete`, `vnc:access` | May additionally be bound to `prod`; approvals still gate governed production actions |
| **Viewer** | `system:read`, `service:read`, `vm:read` | Read-only access (explicit, no `*:read`) |

> **Note**: Environment scope is expressed through `allowed_environments`
> bindings such as `test` and `prod`, not by cloning separate permission
> variants.

### Required Seeds

| Data | Purpose | Idempotent |
|------|---------|------------|
| Super admin | Initial admin account (`admin/admin`) | ✅ `ON CONFLICT DO NOTHING` |
| Built-in roles | PlatformAdmin, ApprovalAdmin, DevelopmentEngineer, TestEngineer, SystemOperator, Viewer | ✅ reconciled at seed time |
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

- [x] `go build ./...` no errors
- [x] Go test framework configured — *full suite execution is owned by CI and release verification*
- [x] Lint configuration exists — *local `golangci-lint run` is an environment verification item because it requires the linter/plugin binary setup*
- [x] Dockerfile exists — *image build verification requires a Docker daemon and is tracked as release/environment verification*
- [x] `/health/live` returns 200 — *Verified via unit test*
- [x] `/health/ready` checks database — *Handler ready with DB pool ping*
- [x] First startup auto-seeds admin account — *implemented through seed/bootstrap path and forced password change*
- [x] River migration tables created in development auto-migration path — *`DatabaseClients.AutoMigrate` runs `rivermigrate.DirectionUp`; live DB verification is environment-specific*

Environment-dependent follow-ups are tracked in
[DEFERRED_FOLLOWUPS.md §Phase 0 Follow-ups](../DEFERRED_FOLLOWUPS.md#phase-0-follow-ups).

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

Vanity import deployment is external infrastructure work, not a Phase 0 runtime
blocker. Track DNS, vanity server deployment, and `go get
kv-shepherd.io/shepherd` verification in
[DEFERRED_FOLLOWUPS.md §Phase 0 Follow-ups](../DEFERRED_FOLLOWUPS.md#phase-0-follow-ups).
