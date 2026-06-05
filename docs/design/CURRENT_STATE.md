# Current Implementation State

> **Last audited**: 2026-05-28
> **Scope**: Code-vs-design alignment snapshot for `public/kubevirt-shepherd`.

This document records the current implementation shape without changing any
accepted ADR. If an accepted decision needs to change, create a new ADR that
amends or supersedes the old decision.

## Audit Sources

| Area | Source |
|------|--------|
| Backend module and dependency state | `go.mod`, `Makefile`, `internal/app/`, `internal/provider/`, `internal/service/`, `internal/jobs/` |
| HTTP contract | `api/openapi.yaml`, `internal/api/generated/server.gen.go` |
| Frontend surface | `web/package.json`, `web/src/app/**/page.tsx`, `web/src/features/` |
| Persistence model | `ent/schema/`, `internal/repository/sqlc/`, `migrations/atlas/` |
| Design governance | `docs/adr/README.md`, `docs/design/CHECKLIST.md`, `docs/design/ci/` |
| Operations runbooks | `docs/operations/`, `deploy/prod/`, `scripts/run_e2e_live.sh` |

## Runtime Snapshot

| Area | Current state |
|------|---------------|
| Go baseline | Go `1.25.11` |
| Backend stack | Gin, Ent, sqlc, pgx, River, zap |
| Database | PostgreSQL 18 baseline; Ent, sqlc, and River share one pgx pool |
| OpenAPI | `api/openapi.yaml` is OpenAPI `3.1.0` with 135 `operationId`s |
| Ent schema count | 33 schema files under `ent/schema/` |
| KubeVirt baseline | `kubevirt.io/api` and `kubevirt.io/client-go` `v1.8.2` |
| Kubernetes baseline | `k8s.io/*` `v0.34.3` |
| Frontend stack | React 19.2, Next.js 16.2, Ant Design 5, TanStack Query 5, Zustand 5 |
| Frontend route files | 29 App Router `page.tsx` files, including root and compatibility/alias routes |
| Background workers | VM create/delete/modify/power/status sync, notification cleanup, domain-event archive, directory sync, directory enrichment scan |
| Observability | Prometheus `/metrics` baseline with Go/process/build collectors, low-cardinality HTTP metrics, PostgreSQL/River bloat metrics, River queue health metrics, OpenAPI validation failure metrics, approval/audit business metrics, bounded HTTP request correlation logs, bounded River worker correlation logs, validated recording rules, rule-test fixtures, Prometheus config validation, Prometheus Operator rule parity validation, alert runbook link validation, Grafana dashboard PromQL validation, optional Prometheus Operator packaging, optional Compose monitoring packaging, a validated starter alert rule pack, a validated starter Grafana dashboard, OpenTelemetry tracing, OpenTelemetry Collector, Tempo, DB spans, River worker spans, and KubeVirt/provider spans |
| Live E2E evidence | ADR-0058 evidence bundle design exists; runner implementation emits per-run result, evidence manifest, and Playwright structured artifacts for readiness/full live E2E runs |

## Implemented Capabilities

| Capability | Current implementation |
|------------|------------------------|
| Local auth | JWT login, forced password change, password change, current-user API |
| External auth | Provider type discovery, login start/submit/callback, JIT user provisioning, external cohort mapping |
| Directory sync | Provider-owned directory descriptor/preview/sync plus scheduled enrichment for existing users |
| RBAC | Global roles, environment-scoped role bindings, resource role bindings, system membership inheritance |
| Governance | Built-in approval provider, external approval webhook registry/admin/runtime wiring, signed external approval callback and polling ingestion, ticket lifecycle, approval requirement service, approval validator |
| VM lifecycle | Request, approve, create, modify, power, delete with tombstone cleanup, manifest, provisioning status with detail-page progress telemetry |
| Batch operations | Parent-child batch model, throttling, status polling, retry failed, cancel pending, compatibility power endpoint |
| Notifications | Inbox APIs, unread count, mark read/all read, triggers, retention cleanup, frontend bell |
| Console | Approval-aware VNC/serial request/status/open flow with encrypted single-use bootstrap credential |
| Catalogs | Admin and user-facing templates and instance sizes with catalog scope and capability hints |
| Cluster policy | Explicit cluster policy controls for clone, image import, host devices, storage classes, and namespace scope |
| Cluster credentials | DB-backed sanitized kubeconfig bytes protected with AES-256-GCM; upload/update rejects local file references, exec/auth-provider plugins, proxy URLs, and unsafe TLS settings |
| VM status convergence | ADR-0038 adaptive polling with ResourceVersion caching and River scheduling |
| Observability | ADR-0054 runtime Prometheus metrics, ADR-0055 starter Prometheus rules and Grafana assets, ADR-0056 optional monitoring deployment packaging, ADR-0057 tracing plus bounded HTTP/River correlation logs, approval/audit business monitoring, OpenTelemetry Collector, Tempo, DB spans, River worker spans, and KubeVirt/provider spans are implemented; broad business SLO metrics, advanced alert routing, advanced dashboards, frontend tracing, and log-search monitoring remain RFC-0010 follow-ups |

## Design Drift Decisions

| Older design expectation | Current code | Decision |
|--------------------------|--------------|----------|
| `KubeVirtProvider` implements all snapshot, clone, migration, instance-type, and console interfaces in V1 | Runtime wiring depends on the narrower `InfrastructureProvider` plus optional provisioning, mutation, VNC, and serial contracts. Snapshot/full clone/live migration remain RFC-backed future scope. | Keep the current narrower wiring. It aligns better with ADR-0024 capability composition and avoids pretending unsupported KubeVirt operations are production-ready. No new ADR required. |
| `ResourceWatcher` is the canonical VM status path | `internal/jobs/vm_status_sync.go` is the authoritative ResourceVersion-aware polling path. | Keep polling as canonical per ADR-0038. Watch/informer work remains optional acceleration under RFC-0020. |
| SSA requires `controller-runtime` | SSA is implemented with `dynamic.Interface`, `types.ApplyPatchType`, and `FieldManager` in `internal/provider/ssa_applier.go`. | Keep the current dynamic-client implementation. It satisfies ADR-0011 without adding `controller-runtime` as a runtime dependency. |
| Server-side browser sessions via `scs` | The product uses Shepherd-issued JWTs, DB-bootstrapped signing/encryption secrets, and PostgreSQL replay markers for console bootstrap credentials. | Keep the current JWT model. If active token revocation or server-side user session state becomes a requirement, that should be introduced by a new ADR/RFC. |
| API and frontend counts from March docs | Current OpenAPI exposes 135 operationIds; frontend has 29 App Router page files. | Update summary docs and checklists. Counts are implementation facts, not ADR decisions. |
| Phase 1 cluster credentials expected a future `ClusterRepository`/file-backed credential provider shape | Current runtime uses Ent-backed admin handlers, `ClusterPolicyService`, `ClusterKubeconfigCodec`, and byte-based client-go loading from persisted kubeconfig bytes. | Keep the implemented DB-backed credential boundary. It better matches artifact-owned runtime operation and avoids production dependence on local kubeconfig paths. |

## Remaining Gaps

Non-blocking production, environment, and V2 follow-ups are tracked centrally in
[DEFERRED_FOLLOWUPS.md](./DEFERRED_FOLLOWUPS.md).

| Gap | Status |
|-----|--------|
| Live E2E validation across a real K8s/KubeVirt cluster | Runner, preflight gates, SOP, and ADR-0058 evidence manifest exist; real-cluster execution evidence remains pending |
| Batch result export UX | Implemented via frontend JSON export from the canonical batch detail response |
| VNC proxy internals, active revocation, and transport hardening validation | V2+ / hardening |
| Reconciler beyond the ADR-0038 status sync worker | Deferred |
| Tenant-level quota enforcement | V2+ |
| External approval native connectors and provider metadata enrichment | RFC-backed future scope |
| VM snapshot, full VM clone, and live migration workflows | RFC-backed future scope |
| PostgreSQL partitioning / pg_partman | RFC-backed future scope |
| Deep OpenTelemetry instrumentation, service/provider log correlation beyond the River lifecycle boundary, business metrics, advanced alert routing, and advanced dashboards | RFC-backed future scope |

## ADR Assessment

No ADR changes are required for this sync:

- The provider narrowing is covered by ADR-0024 and the RFC split for future
  snapshot/clone/migration features.
- The status convergence path is covered by ADR-0038.
- Existing VM mutation behavior is covered by ADR-0052.
- The minimal Prometheus metrics baseline is covered by ADR-0054.
- PostgreSQL/River bloat metrics are covered by ADR-0054.
- OpenAPI validation failure metrics are covered by ADR-0054.
- Minimal Prometheus alert rules are covered by ADR-0055.
- Starter Grafana dashboard assets are covered by ADR-0055.
- HTTP ingress tracing is covered by ADR-0057; deeper DB/River/KubeVirt/provider spans are covered by the observability design baseline.
- Prometheus recording rules are covered by ADR-0055.
- Prometheus rule unit tests are covered by ADR-0055.
- Prometheus Operator packaging is covered by ADR-0056.
- Docker Compose monitoring packaging is covered by ADR-0056.
- Prometheus config validation is covered by ADR-0056.
- Prometheus Operator rule-content parity validation is covered by ADR-0056.
- Prometheus alert runbook link validation is covered by ADR-0055.
- Grafana dashboard PromQL validation is covered by ADR-0055.
- River queue observability metrics are covered by ADR-0054.
- River worker correlation logs are covered by ADR-0057.
- Live E2E evidence bundling is covered by ADR-0058.
- The frontend and contract-first behavior remain covered by ADR-0020,
  ADR-0021, ADR-0028, ADR-0029, and ADR-0030.

If the project decides to require server-side user sessions, active JWT
revocation, or a watch/informer path as an authoritative sync mechanism, create
a new ADR instead of editing accepted ADRs.
