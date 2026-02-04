# Phase 0 Checklist: Project Initialization and Toolchain

> **Detailed Document**: [phases/00-prerequisites.md](../phases/00-prerequisites.md)

---

## Project Structure

- [ ] Go module initialized (`go.mod` with `kv-shepherd.io/shepherd`)
- [ ] Directory structure follows [README.md](../README.md#project-structure)
- [ ] `cmd/server/main.go` created
- [ ] Configuration loading (Viper) working correctly
- [ ] **Standard environment variables (ADR-0018)**: `DATABASE_URL`, `SERVER_PORT`, `LOG_LEVEL`

---

## CI Pipeline

- [ ] `.github/workflows/ci.yml` created
- [ ] `golangci-lint` configured (`.golangci.yml`)
- [ ] Unit test framework configured
- [ ] Code coverage reporting
- [ ] **API Contract-First CI (ADR-0021, ADR-0029)**:
  - [ ] Move API contract workflow to `.github/workflows/api-contract.yaml`
  - [ ] Move `docs/design/ci/makefile/api.mk` to `build/api.mk`
  - [ ] Include `build/api.mk` from root `Makefile`
  - [ ] Add CI step: `make api-check`
  - [ ] If 3.1-only features are used: add CI step `REQUIRE_OPENAPI_COMPAT=1 make api-compat`
  - [ ] Implement `make api-compat-generate` before enabling compat enforcement
- [ ] **OpenAPI Toolchain (ADR-0029)**: See [CI README §API Contract-First](../ci/README.md#api-contract-first-enforcement-adr-0021-adr-0029) for details
  - [ ] `api/.vacuum.yaml` created (vacuum replaces spectral)
  - [ ] CI uses version-pinned GitHub Actions (commit SHA, not tags)
  - [ ] `internal/api/middleware/openapi_validator.go` uses `libopenapi-validator` with StrictMode
- [ ] **sqlc Usage Scope Check (ADR-0012)**:
  - [ ] `scripts/check-sqlc-usage.sh` created
  - [ ] CI blocks: sqlc only allowed in `internal/repository/sqlc/` and `internal/usecase/`
  - [ ] Violations cause CI failure (not just warning)

---

## Infrastructure Code

- [ ] **PostgreSQL Connection Pool (ADR-0012)**:
  - [ ] Using `pgx/v5` + `pgxpool`
  - [ ] **Pool Reuse**: Must use `stdlib.OpenDBFromPool` for Ent to reuse pgxpool
  - [ ] `DatabaseClients` struct created (`internal/infrastructure/database.go`)
  - [ ] **Unified Pool**: Ent + River + sqlc share same `pgxpool.Pool`
  - [ ] **Forbidden**: Creating separate `sql.Open()` and `pgxpool.New()` (doubles connections)
  - [ ] `MaxConns=50`, `MinConns=5`, `MaxConnLifetime=1h`
- [ ] **PostgreSQL Stability Guarantees (ADR-0008)**:
  - [ ] **River Built-in Cleanup**: `CompletedJobRetentionPeriod=24h`
  - [ ] **Aggressive Autovacuum**: `ALTER TABLE river_job SET (autovacuum_vacuum_scale_factor=0.01)`
  - [ ] Dead tuple monitoring view `river_health` created
  - [ ] Prometheus metrics configured (`river_dead_tuple_ratio`)
  - [ ] Alert thresholds configured (>10% warning, >30% critical)
- [ ] Session storage configured (PostgreSQL + alexedwards/scs)
- [ ] Logger (zap) configured
- [ ] Graceful Shutdown
- [ ] **Worker Pool (Coding Standard - Required)**:
  - [ ] `internal/pkg/worker/pool.go` created
  - [ ] Two independent pools: General, K8s
  - [ ] Unified panic recovery
  - [ ] `Metrics()` method exposes metrics

---

## Health Checks

- [ ] `/health/live` returns 200
- [ ] `/health/ready` checks:
  - [ ] Database connection status
  - [ ] **Worker Health**:
    - [ ] River Worker heartbeat (Phase 4 injection)
    - [ ] ResourceWatcher heartbeat (Phase 2 injection)
    - [ ] Heartbeat timeout: Worker 60s, Watcher 120s

---

## Modular DI Pattern (ADR-0022)

> **Purpose**: Ensure `bootstrap.go` follows modular provider pattern for maintainability.

- [ ] `internal/app/modules/` directory exists
- [ ] `internal/app/modules/module.go` defines `Module` interface
- [ ] `internal/app/modules/infrastructure.go` provides shared dependencies
- [ ] Domain modules created (vm.go, approval.go, governance.go, admin.go)
- [ ] **`bootstrap.go` does not exceed 100 lines** (Code Review enforcement)
- [ ] Each module is independently testable
- [ ] No Wire/Dig or reflection-based DI (CI enforcement per ADR-0013)

---

## Pre-Phase 1 Verification

Before proceeding to Phase 1, verify:

- [ ] Phase 0 CI workflow all passing (green ✅)
- [ ] `go build ./...` no errors
- [ ] PostgreSQL connection test successful
- [ ] Worker Pool initialization test passes
- [ ] **Auto-initialization (ADR-0018)**: First startup auto-seeds admin/admin with force_password_change
