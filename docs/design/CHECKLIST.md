# Acceptance Checklist

> **Purpose**: This document is the single acceptance standard  
> **Key Decision**: ADR-0012 Hybrid Transaction Strategy (Ent + sqlc) + CI Blocking Checks
>
> **Note**: Detailed per-phase checklists are now in the [checklist/](./checklist/) directory.

---

## Usage Instructions

1. Verify each Phase upon completion using the detailed phase checklists
2. All ✅ required before proceeding to next phase
3. ❌ items must be fixed and re-verified

---

## Phase Checklists

| Phase | Checklist | Specification | Status |
|-------|-----------|---------------|--------|
| Phase 0 | [checklist/phase-0-checklist.md](./checklist/phase-0-checklist.md) | [phases/00-prerequisites.md](./phases/00-prerequisites.md) | ⬜ Not Started |
| Phase 1 | [checklist/phase-1-checklist.md](./checklist/phase-1-checklist.md) | [phases/01-contracts.md](./phases/01-contracts.md) | ⬜ Not Started |
| Phase 2 | [checklist/phase-2-checklist.md](./checklist/phase-2-checklist.md) | [phases/02-providers.md](./phases/02-providers.md) | ⬜ Not Started |
| Phase 3 | [checklist/phase-3-checklist.md](./checklist/phase-3-checklist.md) | [phases/03-service-layer.md](./phases/03-service-layer.md) | ⬜ Not Started |
| Phase 4 | [checklist/phase-4-checklist.md](./checklist/phase-4-checklist.md) | [phases/04-governance.md](./phases/04-governance.md) | ⬜ Not Started |

---

## Cross-Phase Verification

### CI Checks

- [ ] `golangci-lint` passes
- [ ] Unit test coverage ≥ 60%
- [ ] No data races (`go test -race`)
- [ ] OpenAPI spec and generated Go/TS types are in sync (`make api-check`)
- [ ] If 3.1-only features are used, `REQUIRE_OPENAPI_COMPAT=1 make api-compat` passes

### Architecture Constraints

- [ ] Context correctly passed in all async operations
- [ ] All K8s calls have timeout set
- [ ] Service layer has no transaction control code

### Code-Level Architecture (Code Review Enforcement)

> **Note**: These constraints are enforced during code review, not CI.

| Constraint | ADR | Verification Method |
|------------|-----|---------------------|
| `bootstrap.go` < 100 lines | ADR-0022 | Manual review |
| Provider interfaces use embedding | ADR-0024 | Verify `KubeVirtProvider` embeds capability interfaces |
| Optional fields use `omitzero` | ADR-0028 | Verify generated types (when ADR accepted) |
| Service layer uses narrow interfaces | ADR-0024 | No dependency on full `KubeVirtProvider` when subset suffices |

### Documentation Sync

- [ ] `DEPENDENCIES.md` is only source for versions
- [ ] Other documents don't hardcode versions

---

## Prohibited Patterns

| Pattern | Reason | CI Check Script |
|---------|--------|-----------------|
| GORM import | Use Ent only | `check_forbidden_imports.go` |
| Redis import | PostgreSQL only in V1 | `check_no_redis_import.sh` |
| Naked goroutines | Use worker pool | `check_naked_goroutine.go` |
| Wire import | Manual DI only | `check_manual_di.sh` |
| Outbox pattern | Use River directly | `check_no_outbox_import.go` |
| sqlc outside whitelist | Limited to specific dirs | `check_sqlc_usage.sh` |
| Handler manages transactions | UseCase layer only | `check_transaction_boundary.go` |
| K8s calls in transactions | Two-phase pattern only | `check_k8s_in_transaction.go` |

---

## Core ADR Constraints (Single Reference Point)

> **Purpose**: This section is the **authoritative reference** for critical ADR constraints.
> Other documents (phases, master-flow, notes) SHOULD link here instead of repeating these rules.
> This prevents "content drift" during ADR updates.

| ADR | Constraint | Scope | Enforcement |
|-----|------------|-------|-------------|
| [ADR-0003](../adr/ADR-0003-database-orm.md) | Ent ORM only, no GORM | All data access | CI: `check_no_gorm_import.go` |
| [ADR-0006](../adr/ADR-0006-unified-async-model.md) | All K8s operations via River Queue | External API calls¹ | CI: `check_river_bypass.go` |
| [ADR-0009](../adr/ADR-0009-domain-event-pattern.md) | Payload is immutable (append-only) | DomainEvent table | Code Review |
| [ADR-0012](../adr/ADR-0012-hybrid-transaction.md) | K8s calls outside DB transactions | UseCase layer | CI: `check_k8s_in_transaction.go` |
| [ADR-0013](../adr/ADR-0013-dependency-injection.md) | Manual DI, no Wire/fx | All DI | CI: `check_manual_di.sh` |
| [ADR-0015](../adr/ADR-0015-governance-model-v2.md) | Entity decoupling (VM→Service only) | Schema design | Code Review |
| [ADR-0016](../adr/ADR-0016-go-module-vanity-import.md) | Vanity import: `kv-shepherd.io/shepherd` | All Go imports | Code Review |
| [ADR-0019](../adr/ADR-0019-governance-security-baseline-controls.md) | RFC 1035 naming, least privilege RBAC | All platform-managed names | Code Review |
| [ADR-0021](../adr/ADR-0021-api-contract-first.md) | OpenAPI spec is single source of truth | All HTTP APIs | CI: `make api-check` |
| [ADR-0029](../adr/ADR-0029-openapi-toolchain-governance.md) | Vacuum for linting, libopenapi-validator | API toolchain | CI: `make api-lint` |

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
| Phase 0 | ⬜ Not Started | - | - |
| Phase 1 | ⬜ Not Started | - | - |
| Phase 2 | ⬜ Not Started | - | - |
| Phase 3 | ⬜ Not Started | - | - |
| Phase 4 | ⬜ Not Started | - | - |

---

## Quick Links

- [DEPENDENCIES.md](./DEPENDENCIES.md) - Version pinning (single source of truth)
- [ci/README.md](./ci/README.md) - CI scripts documentation
- [examples/](./examples/) - Reference implementations
