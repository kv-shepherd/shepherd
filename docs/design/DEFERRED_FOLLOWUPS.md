# Deferred Follow-ups

> **Last audited**: 2026-04-24
> **Purpose**: Central parking lot for items that should not keep a completed
> phase marked incomplete.

Phase checklists track phase-blocking acceptance. This document tracks work that
is still useful, environment-dependent, production-operational, or explicitly
future-scoped by RFC/ADR.

## Status Classes

| Status | Meaning |
|--------|---------|
| Production follow-up | Required before a production rollout or by the deployment runbook, but not a code-phase blocker |
| Environment verification | Requires a live database, Docker daemon, DNS, or other external environment |
| RFC / V2 scope | Intentionally deferred behind an RFC or future ADR |
| Replaced | Historical requirement superseded by the current accepted design |

## Phase 0 Follow-ups

| Item | Status | Decision |
|------|--------|----------|
| PostgreSQL/River autovacuum tuning | Production follow-up | ADR-0008 still requires deployment-level tuning. The runtime has River cleanup and shared pool wiring; the actual `ALTER TABLE river_*` settings must be applied by deployment/DB operations. See [database-operations.md](../operations/database-operations.md#critical-configuration-autovacuum-settings). |
| River dead tuple monitoring view | Production follow-up | Keep as an operations task. It depends on the target PostgreSQL deployment and monitoring model. |
| Prometheus metric `river_dead_tuple_ratio` and alert thresholds | Production follow-up / RFC | Keep as observability work. Prometheus endpoint and broader metrics remain RFC-0010 scope. |
| PostgreSQL live connection smoke | Environment verification | `NewDatabaseClients` pings PostgreSQL and CI/test helpers require `TEST_DATABASE_URL` or `DATABASE_URL`; a live smoke still depends on the chosen deployment database. |
| Docker image build | Environment verification | Dockerfile exists, but local verification requires a Docker daemon. Keep as release/CI verification, not Phase 0 completion debt. |
| Vanity import DNS and `go get kv-shepherd.io/shepherd` | Environment verification | Required for external vanity-import distribution, not for monorepo runtime or local development. |
| `golangci-lint run` local execution | Environment verification | CI/tooling owns this gate. Local execution depends on the installed linter binary/plugin setup. |

## RFC / V2 Follow-ups

| Item | Status | Reference |
|------|--------|-----------|
| Active JWT session revocation and session listing | RFC / V2 scope | [RFC-0008](../rfc/RFC-0008-extended-auth.md) |
| Optional K8s watch acceleration / ResourceWatcher | RFC / V2 scope | [RFC-0020](../rfc/RFC-0020-k8s-watch-acceleration.md) |
| VM snapshot workflow | RFC / V2 scope | [RFC-0013](../rfc/RFC-0013-vm-snapshot.md) |
| Full VM clone workflow | RFC / V2 scope | [RFC-0014](../rfc/RFC-0014-vm-clone.md) |
| Live migration workflow | RFC / V2 scope | [RFC-0012](../rfc/RFC-0012-kubevirt-advanced.md) |
| External approval decision ingestion and native provider connectors | RFC / V2 scope | [RFC-0004](../rfc/RFC-0004-external-approval.md) |
| Advanced observability / Prometheus metrics endpoint | RFC / V2 scope | [RFC-0010](../rfc/RFC-0010-observability.md) |
| Full template lifecycle states (`draft -> active -> deprecated -> archived`) | RFC / V2 candidate | V1 uses `enabled`, `catalog_scope`, source validation, and delete guards. Add a contract-first design before introducing lifecycle states. |
| Resource adoption admin workflow | RFC / V2 candidate | V1 keeps the `pending_adoptions` schema as a compensation hook; admin adoption APIs and periodic scan can be designed when adoption becomes a product workflow. |
| Advanced cluster degradation/circuit-breaker UX | RFC / V2 candidate | V1 uses health checks, cluster status, approval preflight degradation handling, and ADR-0038 status sync. |

## Replaced Or Removed Requirements

| Historical item | Resolution |
|-----------------|------------|
| Server-side browser session storage via `scs` | Replaced by Shepherd-issued JWTs plus DB-bootstrapped signing/encryption secrets. Active revocation is RFC-0008 future scope. |
| `controller-runtime` for SSA | Replaced by dynamic-client SSA using `types.ApplyPatchType` in `internal/provider/ssa_applier.go`. |
| Public `/api/v1/vms/validate` dry-run endpoint | Not in the current OpenAPI contract. V1 dry-run is an internal approval preflight gate. |
| Phase 0 OpenAPI 3.1 compat uncertainty | Complete. The canonical spec uses OpenAPI 3.1 nullable union types and `REQUIRE_OPENAPI_COMPAT=1 make api-compat` passes. |
