# Phase 0 Checklist: Project Initialization and Toolchain

> **Detailed Document**: [phases/00-prerequisites.md](../phases/00-prerequisites.md)
>
> **Local preflight (non-CI)**: `bash scripts/preflight/check_pre_coding_readiness.sh`
>
> **Implementation Status**: ✅ Phase 0 completed (2026-02-09)

---

## Project Structure

- [x] Go module initialized (`go.mod` with `kv-shepherd.io/shepherd`)
- [x] Directory structure follows [README.md](../README.md#project-structure)
- [x] `cmd/server/main.go` created
- [x] Configuration loading (Viper) working correctly
- [x] **Standard environment variables (ADR-0018)**: `DATABASE_URL`, `SERVER_PORT`, `LOG_LEVEL`

---

## CI Pipeline

- [x] `.github/workflows/ci.yml` created
- [x] `golangci-lint` configured (`.golangci.yml`)
- [x] Unit test framework configured
- [x] Code coverage reporting
- [x] **Design Docs Governance CI (ADR-0030)**:
  - [x] Enable `.github/workflows/docs-governance.yaml` as a required PR gate
  - [x] Workflow runs `bash docs/design/ci/scripts/check_design_doc_governance.sh`
  - [x] CI blocks PRs on retired doc path usage and broken canonical design links
- [x] **Master-Flow Traceability Manifest (ADR-0032)**:
  - [x] `docs/design/traceability/master-flow.json` exists and is updated with flow/phase changes
  - [x] CI blocks PRs when traceability manifest check fails (`check_master_flow_traceability.go`) — *Integrated in `.github/workflows/ci.yml` (`ci-checks` job)*
- [x] **Master-Flow Spec-Driven Test-First Gate (ADR-0034)**:
  - [x] `docs/design/traceability/master-flow-tests.json` defines required stage→test mappings
  - [x] Deferred required-stage test debt is explicit in `docs/design/ci/allowlists/master_flow_test_deferred.txt`
  - [x] CI blocks PRs when required-stage coverage/deferred hygiene check fails (`check_master_flow_test_matrix.go`)
- [x] **API Contract-First CI (ADR-0021, ADR-0029)**:
  - [x] Move API contract workflow to `.github/workflows/api-contract.yaml`
  - [x] Move `docs/design/ci/makefile/api.mk` to `build/api.mk`
  - [x] Include `build/api.mk` from root `Makefile`
  - [x] Add strict OpenAPI critical contract gate (`check_openapi_critical_contract.go`) to block accidental deletion of required paths/schemas/global security
  - [x] Add strict OpenAPI fingerprint lock gate (`check_openapi_critical_fingerprint.go`) to force intentional review when critical contract nodes change
  - [x] Add strict runtime placeholder gate (`check_no_runtime_placeholders.go`) to block TODO/FIXME/placeholder/stub markers in runtime code
  - [x] Add strict provider wiring gate (`check_provider_wiring.go`) to enforce runtime real-provider wiring and mock rejection
  - [x] Add strict module noop-hook gate (`check_module_noop_hooks.go`) to block silent noop module wiring unless explicitly allowlisted
  - [x] Add strict VM create spec completeness gate (`check_vm_create_spec_completeness.go`) to enforce `spec_overrides` passthrough in Stage 5.C
  - [x] Add strict critical-test-presence gate (`check_critical_test_presence.go`) to require paired test coverage on critical runtime paths
  - [x] Add strict Stage 5.C behavior-test gate (`check_stage5c_behavior_tests.go`) to enforce key scenario assertions (advanced overrides + invalid-path rejection)
  - [x] Add CI step: `make api-check` — *Defined in `build/api.mk`, runs via api-contract.yaml*
  - [ ] If 3.1-only features are used: add CI step `REQUIRE_OPENAPI_COMPAT=1 make api-compat` — *Deferred: no 3.1-only features yet*
  - [x] Implement `make api-compat-generate` minimal generator (3.1 → 3.0.3 rewrite + 3.1 keyword guard) — *Full overlay transform still recommended before strict compat in 3.1-heavy specs*
- [x] **OpenAPI Toolchain (ADR-0029)**: See [CI README §API Contract-First](../ci/README.md#api-contract-first-enforcement-adr-0021-adr-0029) for details
  - [x] `api/.vacuum.yaml` created (vacuum replaces spectral)
  - [x] CI uses version-pinned GitHub Actions (commit SHA, not tags)
  - [x] `internal/api/middleware/openapi_validator.go` — *Runtime request/response OpenAPI validation is implemented and router-wired (`internal/app/router.go`)*
- [x] **sqlc Usage Scope Check (ADR-0012)**:
  - [x] `check_sqlc_usage.sh` created (see [ci/README.md](../ci/README.md#script-summary))
  - [x] CI blocks: sqlc only allowed in `internal/repository/sqlc/` and `internal/usecase/`
  - [x] Violations cause CI failure (not just warning) — *Integrated in `ci.yml` ci-checks job*
- [x] **Frontend/Backend Documentation Synchronization**:
  - [x] `docs/design/frontend/` directory exists with layered subdirectories
  - [x] `docs/design/README.md` points to `docs/design/frontend/README.md` and `docs/design/frontend/FRONTEND.md`
  - [x] `master-flow.md` frontend references point to `docs/design/frontend/FRONTEND.md`

---

## Infrastructure Code

- [x] **PostgreSQL Connection Pool (ADR-0012)**:
  - [x] Using `pgx/v5` + `pgxpool`
  - [x] **Pool Reuse**: Must use `stdlib.OpenDBFromPool` for Ent to reuse pgxpool — *Implemented in `database.go`*
  - [x] `DatabaseClients` struct created (`internal/infrastructure/database.go`)
  - [x] **Unified Pool**: Ent + River + sqlc share same `pgxpool.Pool` — *Architecture ready, consumers added in Phase 1+*
  - [x] **Forbidden**: Creating separate `sql.Open()` and `pgxpool.New()` (doubles connections)
  - [x] `MaxConns=50`, `MinConns=5`, `MaxConnLifetime=1h`
- [ ] **PostgreSQL Stability Guarantees (ADR-0008)**: — *Partially implemented: River client initialized and completed-job retention configured; remaining vacuum/monitoring tuning deferred*
  - [x] **River Built-in Cleanup**: `CompletedJobRetentionPeriod=24h` — *Configured in `internal/infrastructure/database.go` River client init*
  - [ ] **Aggressive Autovacuum**: `ALTER TABLE river_job SET (autovacuum_vacuum_scale_factor=0.01)`
  - [ ] Dead tuple monitoring view `river_health` created
  - [ ] Prometheus metrics configured (`river_dead_tuple_ratio`)
  - [ ] Alert thresholds configured (>10% warning, >30% critical)
- [ ] Session storage configured (PostgreSQL + alexedwards/scs) — *JWT-based auth implemented instead (Phase 5), scs deferred*
- [x] Logger (zap) configured — *AtomicLevel + HTTPHandler for runtime hot-reload*
- [x] Graceful Shutdown — *Signal handling (SIGINT/SIGTERM) in `cmd/server/main.go`*
- [x] **Worker Pool (Coding Standard - Required)** (ADR-0031):
  - [x] `internal/pkg/worker/pool.go` created
  - [x] Two independent pools: General, K8s
  - [x] Unified panic recovery
  - [x] `Metrics()` method exposes metrics

---

## Health Checks

- [x] `/health/live` returns 200
- [x] `/health/ready` checks:
  - [x] Database connection status
  - [x] **Worker Health**: — *Injection points ready*
    - [x] River Worker heartbeat (Phase 4 injection) — *`SetRiverWorker()` ready, consumer deferred to [Phase 4](../phases/04-governance.md)*
    - [x] ResourceWatcher heartbeat (Phase 2 injection) — *`AddResourceWatcher()` ready, consumer deferred to [Phase 2](../phases/02-providers.md)*
    - [x] Heartbeat timeout: Worker 60s, Watcher 120s

---

## Modular DI Pattern (ADR-0022)

> **Purpose**: Ensure `bootstrap.go` follows modular provider pattern for maintainability.

- [x] `internal/app/modules/` directory exists
- [x] `internal/app/modules/module.go` defines `Module` interface
- [x] `internal/app/modules/infrastructure.go` provides shared dependencies
- [x] Domain modules created (vm.go, approval.go, governance.go, admin.go) — *Placeholder stubs, implementation in respective phases*
- [x] **`bootstrap.go` does not exceed 100 lines** (Code Review enforcement) — *65 lines*
- [x] Each module is independently testable
- [x] No Wire/Dig or reflection-based DI (CI enforcement per ADR-0013) — *`check_manual_di.sh` in CI*

---

## Pre-Phase 1 Verification

Before proceeding to Phase 1, verify:

- [x] Phase 0 CI workflow all passing (green ✅) — *`go vet` + `go build` + `go test -race` all pass*
- [x] `go build ./...` no errors
- [ ] PostgreSQL connection test successful — *Requires running PostgreSQL; River + Ent clients initialized via shared pgxpool*
- [x] Worker Pool initialization test passes — *`pool_test.go` with 68.4% coverage*
- [x] **Auto-initialization (ADR-0018)**: First startup auto-seeds admin/admin with force_password_change — *`cmd/seed/main.go` fully implemented with 6 built-in roles + default admin (Phase 5)*

---

## Deferred Items Summary

The following items are architecturally prepared but depend on packages/features introduced in later phases:

| Item | Blocked By | Target Phase |
|------|-----------|--------------|
| `stdlib.OpenDBFromPool` for Ent | Ent ORM v0.14.5 | [Phase 1: Contracts](../phases/01-contracts.md) |
| River stability (ADR-0008) | River Queue v0.30.2 | [Phase 4: Governance](../phases/04-governance.md) |
| Session storage (scs) | Auth implementation | [Phase 5: Auth/API/Frontend](../phases/05-auth-api-frontend.md) |
| Admin seed (ADR-0018) | User model + auth | [Phase 5: Auth/API/Frontend](../phases/05-auth-api-frontend.md) |
| OpenAPI 3.1 compat check | 3.1-only features usage | When needed |
| Traceability manifest CI | Real flow changes | [Phase 1: Contracts](../phases/01-contracts.md) |
