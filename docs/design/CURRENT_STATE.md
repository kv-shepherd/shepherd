# Current Implementation State

> **Last audited**: 2026-06-19
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
| Go baseline | See [DEPENDENCIES.md §Go Version](./DEPENDENCIES.md#go-version) |
| Backend stack | Gin, Ent, sqlc, pgx, River, zap |
| Database | PostgreSQL baseline from [DEPENDENCIES.md §Database](./DEPENDENCIES.md#database); Ent, sqlc, and River share one pgx pool |
| OpenAPI | `api/openapi.yaml` is the canonical OpenAPI spec with 140 `operationId`s; spec/tool versions are in [DEPENDENCIES.md §API Contract Tooling](./DEPENDENCIES.md#api-contract-tooling) |
| Ent schema count | 33 schema files under `ent/schema/` |
| KubeVirt baseline | KubeVirt API/client versions from [DEPENDENCIES.md §Kubernetes and KubeVirt](./DEPENDENCIES.md#kubernetes-and-kubevirt) |
| Kubernetes baseline | Kubernetes client/API versions from [DEPENDENCIES.md §Kubernetes and KubeVirt](./DEPENDENCIES.md#kubernetes-and-kubevirt) |
| Frontend stack | React, Next.js, Ant Design, TanStack Query, and Zustand versions from [DEPENDENCIES.md §Frontend Runtime](./DEPENDENCIES.md#frontend-runtime) |
| Frontend route files | 32 App Router `page.tsx` files, including root and compatibility/alias routes |
| Background workers | VM create/delete/modify/power/status sync, VM adoption discovery scan, notification cleanup, domain-event archive, directory sync, directory enrichment scan |
| Observability | Prometheus `/metrics` baseline with Go/process/build collectors, low-cardinality HTTP metrics, PostgreSQL/River bloat metrics, River queue health metrics, OpenAPI validation failure metrics, approval/audit business metrics, bounded HTTP request correlation logs, bounded River worker correlation logs, validated recording rules, rule-test fixtures, Prometheus config validation, Prometheus Operator rule parity validation, alert runbook link validation, Grafana dashboard PromQL validation, optional Prometheus Operator packaging, optional Compose monitoring packaging, a validated starter alert rule pack, a validated starter Grafana dashboard, OpenTelemetry tracing, OpenTelemetry Collector, Tempo, DB spans, River worker spans, and KubeVirt/provider spans |
| Live E2E evidence | ADR-0058 evidence bundle design exists; runner implementation emits per-run result, evidence manifest, and Playwright structured artifacts for readiness/full live E2E runs |

## Implemented Capabilities

| Capability | Current implementation |
|------------|------------------------|
| Local auth | JWT login, forced password change, password change, current-user API, session-version JWT revocation |
| External auth | Provider type discovery, login start/submit/callback, JIT user provisioning, external cohort mapping |
| Directory sync | Provider-owned directory descriptor/preview/sync plus scheduled enrichment for existing users |
| RBAC | Global roles, environment-scoped role bindings, resource role bindings, system membership inheritance |
| Governance | Built-in approval provider, external approval webhook registry/admin/runtime wiring, signed external approval callback and polling ingestion, ticket lifecycle, approval requirement service, approval validator |
| VM lifecycle | Request, approve, create, modify, power, delete with tombstone cleanup, manifest, provisioning status with detail-page progress telemetry |
| Resource adoption | Label-based live VM discovery persists `pending_adoptions`, scheduled adoption scans refresh candidates, platform-admin API endpoints list/reject/adopt candidates, and `/admin/pending-adoptions` provides the review UI |
| Batch operations | Parent-child batch model, throttling, status polling, retry failed, cancel pending, compatibility power endpoint |
| Notifications | Inbox APIs, unread count, mark read/all read, triggers, retention cleanup, frontend bell |
| Console | Approval-aware VNC/serial request/status/open flow with encrypted single-use bootstrap credential and same-origin approved-target validation |
| Catalogs | Admin and user-facing templates and instance sizes with catalog scope and capability hints |
| Cluster policy | Explicit cluster policy controls for clone, image import, host devices, storage classes, and namespace scope |
| Cluster credentials | DB-backed sanitized kubeconfig bytes protected with AES-256-GCM; upload/update rejects local file references, exec/auth-provider plugins, proxy URLs, and unsafe TLS settings |
| VM status convergence | ADR-0038 adaptive polling with ResourceVersion caching and River scheduling |
| Observability | ADR-0054 runtime Prometheus metrics, ADR-0055 starter Prometheus rules and Grafana assets, ADR-0056 optional monitoring deployment packaging, ADR-0057 tracing plus bounded HTTP/River correlation logs, approval/audit business monitoring with fixed low-cardinality action aggregation, OpenTelemetry Collector, Tempo, DB spans, River worker spans, and KubeVirt/provider spans are implemented; broad business SLO metrics, advanced alert routing, advanced dashboards, frontend tracing, and log-search monitoring remain RFC-0010 follow-ups |

## Design Drift Decisions

| Older design expectation | Current code | Decision |
|--------------------------|--------------|----------|
| `KubeVirtProvider` implements all snapshot, clone, migration, instance-type, and console interfaces in V1 | Runtime wiring depends on the narrower `InfrastructureProvider` plus optional provisioning, mutation, VNC, and serial contracts. Snapshot/full clone/live migration remain RFC-backed future scope. | Keep the current narrower wiring. It aligns better with ADR-0024 capability composition and avoids pretending unsupported KubeVirt operations are production-ready. No new ADR required. |
| `ResourceWatcher` is the canonical VM status path | `internal/jobs/vm_status_sync.go` is the authoritative ResourceVersion-aware polling path. | Keep polling as canonical per ADR-0038. Watch/informer work remains optional acceleration under RFC-0020. |
| SSA requires `controller-runtime` | SSA is implemented with `dynamic.Interface`, `types.ApplyPatchType`, and `FieldManager` in `internal/provider/ssa_applier.go`. | Keep the current dynamic-client implementation. It satisfies ADR-0011 without adding `controller-runtime` as a runtime dependency. |
| Server-side browser sessions via `scs` | The product uses Shepherd-issued JWTs, DB-bootstrapped signing/encryption secrets, and PostgreSQL replay markers for console bootstrap credentials. | Keep the current JWT model. If active token revocation or server-side user session state becomes a requirement, that should be introduced by a new ADR/RFC. |
| API and frontend counts from March docs | Current OpenAPI exposes 140 operationIds; frontend has 32 App Router page files. | Update summary docs and checklists. Counts are implementation facts, not ADR decisions. |
| Phase 1 cluster credentials expected a future `ClusterRepository`/file-backed credential provider shape | Current runtime uses Ent-backed admin handlers, `ClusterPolicyService`, `ClusterKubeconfigCodec`, and byte-based client-go loading from persisted kubeconfig bytes. | Keep the implemented DB-backed credential boundary. It better matches artifact-owned runtime operation and avoids production dependence on local kubeconfig paths. |

## Remaining Gaps

Non-blocking production, environment, and V2 follow-ups are tracked centrally in
[DEFERRED_FOLLOWUPS.md](./DEFERRED_FOLLOWUPS.md).

| Gap | Status |
|-----|--------|
| Live E2E validation across a real K8s/KubeVirt cluster | Runner, preflight gates, SOP, and ADR-0058 evidence manifest exist; real-cluster execution evidence remains pending |
| VNC proxy internals, VNC active-revocation validation, and transport hardening validation | V2+ / hardening |
| Reconciler beyond the ADR-0038 status sync worker | Deferred |
| Tenant-level quota enforcement | V2+ |
| External approval native connectors and provider metadata enrichment | RFC-backed future scope |
| VM snapshot, full VM clone, and live migration workflows | RFC-backed future scope |
| PostgreSQL partitioning / pg_partman | RFC-backed future scope |
| Frontend tracing, log-search monitoring, service/provider log correlation beyond the existing HTTP/River lifecycle boundary, broad business SLO metrics, advanced alert routing, and advanced dashboards | RFC-0010 future scope |

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

If the project decides to require server-side browser sessions, productized
session listing/admin session-management APIs, or a watch/informer path as an
authoritative sync mechanism, create a new ADR instead of editing accepted ADRs.
