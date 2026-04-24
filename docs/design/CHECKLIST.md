# Acceptance Checklist

> **Purpose**: This document is the single acceptance standard  
> **Key Decision**: ADR-0012 Hybrid Transaction Strategy (Ent + sqlc) + CI Blocking Checks
>
> **Note**: Detailed per-phase checklists are now in the [checklist/](./checklist/) directory.
> Non-blocking production, environment, and V2/RFC work is tracked in
> [DEFERRED_FOLLOWUPS.md](./DEFERRED_FOLLOWUPS.md).

---

## Usage Instructions

1. Verify each phase (frontend and backend scope) using the detailed phase checklists.
2. All ✅ required before proceeding to the next phase.
3. ❌ items must be fixed and re-verified.
4. API/flow changes are not complete until OpenAPI, frontend docs, master-flow, and phase docs are all synchronized.
5. Gate-first rule: complete [ci/GATE_HARDENING_CHECKLIST.md](./ci/GATE_HARDENING_CHECKLIST.md) before new feature delivery.

---

## Phase Checklists

| Phase | Checklist | Specification | Status |
|-------|-----------|---------------|--------|
| Phase 0 | [checklist/phase-0-checklist.md](./checklist/phase-0-checklist.md) | [phases/00-prerequisites.md](./phases/00-prerequisites.md) | ✅ Complete (2026-02-09) |
| Phase 1 | [checklist/phase-1-checklist.md](./checklist/phase-1-checklist.md) | [phases/01-contracts.md](./phases/01-contracts.md) | 🔄 Partial (~94%) — 32 Ent schemas + TS/Go API types + frontend testing toolchain ✅, remaining work is contract hardening and deferred V2 schemas |
| Phase 2 | [checklist/phase-2-checklist.md](./checklist/phase-2-checklist.md) | [phases/02-providers.md](./phases/02-providers.md) | 🔄 Partial (~65%) — Basic CRUD + SSAApplier + VMRenderer + AuthProvider Admin ✅, Snapshot/Clone/Migration deferred; V1 status sync uses ADR-0038 polling baseline |
| Phase 3 | [checklist/phase-3-checklist.md](./checklist/phase-3-checklist.md) | [phases/03-service-layer.md](./phases/03-service-layer.md) | 🔄 Partial (~80%) — Core DI/UseCase + ADR-0012 atomic path + sqlc + InstanceSize handler ✅, concurrency semaphore/degradation ❌ |
| Phase 4 | [checklist/phase-4-checklist.md](./checklist/phase-4-checklist.md) | [phases/04-governance.md](./phases/04-governance.md) | 🔄 Partial (~96%) — Approval/Audit/Atomic enqueue/Delete/Namespace CRUD/Notification system ✅, environment scheduling + visibility filtering ✅, Stage 5.E batch + queue UX ✅, Stage 6 VNC token hardening ✅, Catalog Scope + Cluster Policy + VM Status Sync (ADR-0038) + Template/InstanceSize validators ✅; Reconciler ❌ |
| Phase 5 | [checklist/phase-5-checklist.md](./checklist/phase-5-checklist.md) | [phases/05-auth-api-frontend.md](./phases/05-auth-api-frontend.md) | 🔄 In Progress (~97%) — Backend Auth ✅ (JWT hardening + bcrypt cost 12 + log redaction), 127 endpoints (ADR-0028 omitzero) ✅, 28 App Router page files (including compatibility/alias routes), E2E pending |

---

## Cross-Phase Verification

### Master-Flow VM Lifecycle Alignment (audited 2026-02-10)

> **Purpose**: This section is the **coding checkpoint** for VM lifecycle implementation.
> Start here instead of re-reading all backend code.
>
> **Source of truth**: [master-flow.md](interaction-flows/master-flow.md) Part 3 (Stages 5.A–5.F) + Stage 6 + Part 4
>
> **Detailed gaps**: See [phase-4-checklist.md §Master-Flow Alignment Status](checklist/phase-4-checklist.md#master-flow-alignment-status-vm-lifecycle-audited-2026-02-10)

| Stage | Description | Alignment | Checklist Section | Design Doc Reference | Code Fix Priority |
|-------|-------------|-----------|-------------------|----------------------|-------------------|
| **Part 4** | State Machines & Data Models | ✅ 90% | [phase-2 §Anti-Corruption](checklist/phase-2-checklist.md#anti-corruption-layer) | [master-flow §Part 4](interaction-flows/master-flow.md#part-4-state-machines-data-models) | ~~P0~~ ✅ Done (2026-02-10) |
| **5.A** | VM Request Submission | ✅ 90% | [phase-3 §UseCase](checklist/phase-3-checklist.md#usecase-layer-standards-clean-architecture) | [master-flow §5.A](interaction-flows/master-flow.md#stage-5-a) | P2 — remove domain `PENDING` |
| **5.B** | Admin Approval | ✅ 90% | [phase-4 §Approval](checklist/phase-4-checklist.md#approval-flow-governance-core) | [master-flow §5.B](interaction-flows/master-flow.md#stage-5-b) | P3 — prod overcommit warning UX |
| **5.C** | VM Creation Execution | ✅ 95% | [phase-4 §River Queue](checklist/phase-4-checklist.md#river-queue-task-system-adr-0006) | [master-flow §5.C](interaction-flows/master-flow.md#state-transitions-stage-5a-5c) | P3 — provider hard idempotency |
| **5.D** | Delete Operations | ✅ 90% | [phase-4 §Delete](checklist/phase-4-checklist.md#delete-confirmation-mechanism-adr-0015-131) | [master-flow §5.D](interaction-flows/master-flow.md#stage-5d-delete-operations) | P2 — tombstone cleanup policy |
| **5.E** | Batch Operations | ✅ 96% | [phase-4 §Batch](checklist/phase-4-checklist.md#batch-operations-adr-0015-19) | [master-flow §5.E](interaction-flows/master-flow.md#stage-5e-batch-operations) | P2 — API + child execution dispatch + two-layer throttling + parent projection + admin override APIs + `/vms/batch/power` + frontend queue UX (`status_url` tracking/429 cooldown/affected-child feedback/aria-live) complete; export-result UX pending |
| **5.F** | Notification System | ✅ 95% | [phase-4 §Notification](checklist/phase-4-checklist.md#notification-system-adr-0015-20) | [master-flow §5.F](interaction-flows/master-flow.md#stage-5f-notification-system) | P3 — V1 inbox flow complete (API+triggers+sender+frontend bell+retention cleanup); external channels deferred to V2+ |
| **6** | VNC Console Access | ✅ 90% | [phase-4 §VNC](checklist/phase-4-checklist.md#vnc-console-permissions-adr-0015-18-181-addendum) | [master-flow §6](interaction-flows/master-flow.md#stage-6-vnc-console-access) | P2 — noVNC proxy internals and transport hardening rollout validation |

#### Coding Priority Queue (work in this order)

1. ~~**P0** — Fix VM status enum (`domain/vm.go`): `ERROR`→`FAILED`, add `STOPPING`, remove `PENDING`~~ ✅ **Done** (2026-02-10)
   - `ERROR`→`FAILED`, added `STOPPING`/`MIGRATING`/`PAUSED`/`UNKNOWN`
   - Ent schema, mapper, OpenAPI spec, all handlers aligned
   - `PENDING` retained as K8s-extended state (not removed, used by mapper)
2. ~~**P1** — Ent codegen + Atlas migration for VM status enum changes~~ ✅ **Done** (2026-02-10)
3. ~~**P1** — Frontend VM status labels/colors sync (`web/src/types/api.gen.ts`)~~ ✅ **Done** (2026-02-10)
   - `api.gen.ts` regenerated from OpenAPI spec
   - `VM_STATUS_MAP` in `vms/page.tsx` updated (APPROVED/ERROR/DELETED → STOPPING/FAILED/MIGRATING/PAUSED/UNKNOWN)
   - i18n keys updated (en + zh-CN)
4. ~~**P1** — Enhance `ApprovalValidator`: dedicated CPU + overcommit check, resource capability matching~~ ✅ **Done** (2026-02-10)
5. ~~**P1** — Fix delete governance: cascade checks, approval ticket for VM delete, `DELETING` state usage~~ ✅ **Done** (2026-02-10)
6. ~~**P2** — Ticket lifecycle: worker updates ticket `EXECUTING`→`SUCCESS/FAILED`~~ ✅ **Done** (2026-02-10)
7. ~~**P1** — Align approval commit path to ADR-0012 (`sqlc + InsertTx`)~~ ✅ **Done** (2026-02-10)
8. ~~**P2/P3** — Batch API + child execution dispatch + two-layer throttling + parent projection + admin override APIs + `/vms/batch/power` + frontend queue UX~~ ✅ **Done** (Stage 5.E baseline complete); ~~Stage 6 VNC baseline with shared PG replay marker~~ ✅ **Done**; ~~VNC credential AES-256-GCM hardening~~ ✅ **Done** (`ENCRYPTION_KEY` + AES-256-GCM encrypted JWT envelope); Notification system ✅ (V1 inbox flow complete); VM Status Sync Worker (ADR-0038) ✅; Catalog Scope separation (ADR-0040) ✅

### CI Checks

- [ ] `golangci-lint` passes
- [ ] Unit test coverage ≥ 60%
- [ ] No data races (`go test -race`)
- [ ] OpenAPI spec and generated Go/TS types are in sync (`make api-check`)
- [ ] If 3.1-only features are used, `REQUIRE_OPENAPI_COMPAT=1 make api-compat` passes
- [ ] Design docs governance checks pass (frontend path, master-flow links, checklist authority references)
- [ ] Master-flow traceability manifest check passes (`check_master_flow_traceability.go`)
- [ ] Master-flow required-stage test matrix gate passes (`check_master_flow_test_matrix.go`)

### Architecture Constraints

- [ ] Context correctly passed in all async operations ([ADR-0031](../adr/ADR-0031-concurrency-and-worker-pool-standard.md) Rule 2)
- [ ] All K8s calls have timeout set
- [ ] Service layer has no transaction control code
- [ ] Batch operations use ADR-0015 parent-child model with two-layer rate limiting
- [ ] Frontend batch UI exposes parent/child status, retry failed children, and terminate pending children

### Code-Level Architecture (Code Review Enforcement)

> **Note**: These constraints are enforced during code review, not CI.

| Constraint | ADR | Verification Method |
|------------|-----|---------------------|
| `Bootstrap()` stays orchestration-only and concise | ADR-0022 + pending ADR-0043 note | Manual review |
| Provider interfaces use embedding | ADR-0024 | Verify `KubeVirtProvider` embeds capability interfaces |
| Optional fields use `omitzero` | ADR-0028 | Verify generated types (when ADR accepted) |
| Service layer uses narrow interfaces | ADR-0024 | No dependency on full `KubeVirtProvider` when subset suffices |

### Documentation Sync

- [ ] `DEPENDENCIES.md` is only source for versions
- [ ] Other documents don't hardcode versions
- [ ] `docs/design/frontend/` is used for frontend specs (no legacy `docs/design/FRONTEND.md` links)
- [ ] `master-flow.md` and phase/frontend docs share consistent status models and endpoint names

---

## Prohibited Patterns

| Pattern | Reason | CI Check Script |
|---------|--------|-----------------|
| GORM import | Use Ent only | `shepherd-arch/forbiddenimports` |
| Redis import | PostgreSQL only in V1 | `check_no_redis_import.sh` |
| SQLite usage in tests | PostgreSQL-only policy across runtime and tests | `check_no_sqlite_in_tests.go` |
| Naked goroutines | Use worker pool (ADR-0031) | `shepherd-arch/nakedgoroutine` |
| Wire import | Manual DI only | `check_manual_di.sh` |
| Outbox pattern | Use River directly | `shepherd-arch/forbiddenimports` |
| sqlc outside whitelist | Limited to specific dirs | `check_sqlc_usage.sh` |
| Handler manages transactions | UseCase layer only | `shepherd-arch/txboundary` |
| K8s calls in transactions | Two-phase pattern only | `check_k8s_in_transaction.go` |
| Unsafe semaphore usage | Always `defer Release()` (ADR-0031) | `shepherd-arch/semaphoreusage` |
| Runtime code change without corresponding test delta | Enforce test-first delivery | `check_changed_code_has_tests.sh` |

---

## Core ADR Constraints (Single Reference Point)

> **Purpose**: This section is the **authoritative reference** for critical ADR constraints.
> Other documents (phases, master-flow, notes) SHOULD link here instead of repeating these rules.
> This prevents "content drift" during ADR updates.

| ADR | Constraint | Scope | Enforcement |
|-----|------------|-------|-------------|
| [ADR-0003](../adr/ADR-0003-database-orm.md) | Ent ORM only, no GORM | All data access | CI: `shepherd-arch/forbiddenimports` |
| [ADR-0008](../adr/ADR-0008-postgresql-stability.md) | PostgreSQL-only baseline; no SQLite fallback in test path | Runtime + tests | CI: `check_no_sqlite_in_tests.go` |
| [ADR-0006](../adr/ADR-0006-unified-async-model.md) | All K8s operations via River Queue | External API calls¹ | CI: `shepherd-arch/riverbypass` |
| [ADR-0009](../adr/ADR-0009-domain-event-pattern.md) | Payload is immutable (append-only) | DomainEvent table | Code Review |
| [ADR-0012](../adr/ADR-0012-hybrid-transaction.md) | K8s calls outside DB transactions | UseCase layer | CI: `check_k8s_in_transaction.go` |
| [ADR-0013](../adr/ADR-0013-manual-di.md) | Manual DI, no Wire/fx | All DI | CI: `check_manual_di.sh` |
| [ADR-0015](../adr/ADR-0015-governance-model-v2.md) | Entity decoupling (VM→Service only) + VNC credential transport hardening (no URI query token) | Schema design + Stage 6 VNC flow | CI: `check_stage6_vnc_baseline.go` + Code Review |
| [ADR-0016](../adr/ADR-0016-go-module-vanity-import.md) | Vanity import: `kv-shepherd.io/shepherd` | All Go imports | Code Review |
| [ADR-0017](../adr/ADR-0017-vm-request-flow-clarification.md) | User does NOT provide ClusterID; Namespace immutable after submission | VM Request Flow | Code Review |
| [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md) | InstanceSize hybrid model (indexed columns + JSONB); snapshot at approval | Schema design | Code Review |
| [ADR-0019](../adr/ADR-0019-governance-security-baseline-controls.md) | RFC 1035 naming, least privilege RBAC, audit log redaction | All platform-managed names | Code Review |
| [ADR-0021](../adr/ADR-0021-api-contract-first.md) | OpenAPI spec is single source of truth | All HTTP APIs | CI: `make api-check` |
| [ADR-0025](../adr/ADR-0025-secret-bootstrap.md) | Auto-generate secrets on first boot; priority: env vars > DB-generated | Bootstrap flow | Code Review |
| [ADR-0028](../adr/ADR-0028-oapi-codegen-optional-field-strategy.md) | oapi-codegen with `omitzero`; Go 1.25+ required | API code generation | CI: `make generate` |
| [ADR-0029](../adr/ADR-0029-openapi-toolchain-governance.md) | Vacuum for linting, libopenapi-validator | API toolchain | CI: `make api-lint` |
| [ADR-0031](../adr/ADR-0031-concurrency-and-worker-pool-standard.md) | No naked `go` statements; worker pool with context propagation; semaphore Acquire/Release leak-safe | In-process concurrency | CI: `shepherd-arch/nakedgoroutine`, `shepherd-arch/semaphoreusage` |
| [ADR-0034](../adr/ADR-0034-master-flow-spec-driven-test-first.md) | Required master-flow stages must have executable tests or explicit deferred debt; runtime code diffs must carry test diffs | Cross-layer interaction delivery | CI: `check_master_flow_test_matrix.go`, `check_changed_code_has_tests.sh` |

> ¹ **ADR-0006 Scope Clarification**: "All writes via River Queue" applies to operations requiring external system calls (K8s API).
> Pure PostgreSQL writes (e.g., Notification, AuditLog, DomainEvent insert) are **synchronous** for transactional atomicity.
> See [Phase 4 §6.3](phases/04-governance.md#63-notification-system-adr-0015-20) for detailed rationale.

---

## Explicitly Not Doing

The following items are moved to [RFC directory](../rfc/):

| Item | Status | Notes |
|------|--------|-------|
| Complex Admission Rules | 📋 RFC | Phase 2 only basic validation |
| Config Hot-Reload (Basic) | ✅ Done | Log level, rate limit params support hot-reload |
| Config Admin API | 📋 RFC | API dynamic modification, see [RFC-0006](../rfc/RFC-0006-hot-reload.md) |
| Notification/Approval Plugin System | 📋 RFC | Implement as Service first |
| Frontend Refactor | 📋 RFC | Consider after backend stable |

---

## Progress Tracking

| Phase | Status | Completion Date | Verified By |
|-------|--------|-----------------|-------------|
| Phase 0 | ✅ Complete | 2026-02-09 | CI green (go vet/build/test) |
| Phase 1 | 🔄 Partial (~94%) | - | 32 Ent schemas + Go/TS API types + frontend testing toolchain done, contract CI hardening gaps and deferred V2 schemas |
| Phase 2 | 🔄 Partial (~65%) | - | Basic VM CRUD + SSAApplier + VMRenderer + AuthProvider Admin done; Snapshot/Clone/Migration deferred; ResourceWatcher docs superseded in V1 by ADR-0038 polling baseline |
| Phase 3 | 🔄 Partial (~80%) | - | Core DI/UseCase + ADR-0012 atomic approval + sqlc + InstanceSize handler done; concurrency semaphore/degradation deferred |
| Phase 4 | 🔄 Partial (~96%) | - | Approval/Audit/Delete/Notification/Batch/VNC hardening/Catalog Scope/Cluster Policy/VM Status Sync (ADR-0038)/Template+InstanceSize validators done; Reconciler deferred |
| Phase 5 | 🔄 In Progress (~97%) | - | Backend auth ✅, API gen 127 endpoints (ADR-0028 omitzero) ✅, 28 App Router page files (incl. AuthProvider/RateLimit/Batch/Profile/VNC routes), E2E pending |

---

## Quick Links

- [DEPENDENCIES.md](./DEPENDENCIES.md) - Version pinning (single source of truth)
- [ci/README.md](./ci/README.md) - CI scripts documentation
- [examples/](./examples/) - Reference implementations
