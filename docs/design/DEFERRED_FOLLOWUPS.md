# Deferred Follow-ups

> **Last audited**: 2026-05-28
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
| Advanced alert routing and dashboards for PostgreSQL/River bloat metrics | Production follow-up / RFC | Runtime metrics are accepted by ADR-0054, starter rules and dashboards are accepted by ADR-0055, and optional deployment packaging is accepted by ADR-0056. Alertmanager routing, escalation, and detailed dashboards remain deployment-policy work. |
| PostgreSQL live connection smoke | Environment verification | `NewDatabaseClients` pings PostgreSQL and CI/test helpers require `TEST_DATABASE_URL` or `DATABASE_URL`; a live smoke still depends on the chosen deployment database. |
| Docker image build | Environment verification | Dockerfile exists, but local verification requires a Docker daemon. Keep as release/CI verification, not Phase 0 completion debt. |
| Production deployment execution evidence | Production follow-up | Deployment and upgrade runbooks are documented in [production-deployment.md](../operations/production-deployment.md), but each rollout still needs operator-captured image, database, monitoring, health, and rollback evidence. |
| Real-cluster live E2E execution evidence | Environment verification | `scripts/run_e2e_live.sh`, ADR-0058, and [live-e2e-validation.md](../operations/live-e2e-validation.md) define the validation path and evidence bundle; completion still requires a real PostgreSQL-backed backend and real K8s/KubeVirt cluster run. |
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
| Advanced observability beyond the accepted baseline | RFC / V2 scope | ADR-0054 covers runtime metrics, ADR-0055 covers starter rules and dashboards, ADR-0056 covers optional deployment packaging, ADR-0057 covers tracing plus bounded HTTP/River correlation logs, and the built-in stack now includes approval/audit business monitoring, OpenTelemetry Collector, Tempo, DB spans, River worker spans, and KubeVirt/provider spans. Frontend tracing, log-search monitoring, service/provider log correlation beyond the River lifecycle boundary, broad business SLO metrics, advanced alert routing, advanced SLOs, and advanced dashboards remain in [RFC-0010](../rfc/RFC-0010-observability.md). |
| Full template lifecycle states (`draft -> active -> deprecated -> archived`) | RFC / V2 candidate | V1 uses `enabled`, `catalog_scope`, source validation, and delete guards. Add a contract-first design before introducing lifecycle states. |
| Template import/export automation | RFC / V2 candidate | V1 keeps catalog administration in the API/UI and stores templates in PostgreSQL. Import/export automation should be designed with the lifecycle-state contract instead of treated as a Phase 4 blocker. |
| Rich VM revision diff/compressed YAML service | RFC / V2 candidate | V1 has `vm_revisions` persistence and audit logs. Diff calculation, compressed YAML storage, and revision-service APIs need a dedicated contract before they become product workflow requirements. |
| Resource adoption admin workflow | RFC / V2 candidate | V1 keeps the `pending_adoptions` schema as a compensation hook; admin adoption APIs and periodic scan can be designed when adoption becomes a product workflow. |
| Full resource reconciler (`dry-run`/`mark`/`delete` + ghost/orphan reports) | RFC / V2 candidate | V1 status convergence is ADR-0038 adaptive polling, and resource adoption is a compensation hook. A full Kubernetes controller-style reconciler needs a contract-first RFC/ADR before it can mutate or mark resources. |
| Advanced cluster degradation/circuit-breaker UX | RFC / V2 candidate | V1 uses health checks, cluster status, approval preflight degradation handling, and ADR-0038 status sync. |

## Replaced Or Removed Requirements

| Historical item | Resolution |
|-----------------|------------|
| Server-side browser session storage via `scs` | Replaced by Shepherd-issued JWTs plus DB-bootstrapped signing/encryption secrets. Active revocation is RFC-0008 future scope. |
| `controller-runtime` for SSA | Replaced by dynamic-client SSA using `types.ApplyPatchType` in `internal/provider/ssa_applier.go`. |
| Public `/api/v1/vms/validate` dry-run endpoint | Not in the current OpenAPI contract. V1 dry-run is an internal approval preflight gate. |
| Phase 0 OpenAPI 3.1 compat uncertainty | Complete. The canonical spec uses OpenAPI 3.1 nullable union types and `REQUIRE_OPENAPI_COMPAT=1 make api-compat` passes. |
