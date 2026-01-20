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

### Architecture Constraints

- [ ] Context correctly passed in all async operations
- [ ] All K8s calls have timeout set
- [ ] Service layer has no transaction control code

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
