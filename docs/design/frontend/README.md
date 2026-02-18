# Frontend Design Index

> **Authority**: Frontend implementation specification under [ADR-0030](../../adr/ADR-0030-design-documentation-layering-and-fullstack-governance.md)
> **Flow Source of Truth**: [master-flow.md](../interaction-flows/master-flow.md)

---

## Purpose

This directory is the canonical entry for frontend design documents.

Frontend docs are layered by concern to prevent drift and to keep alignment with backend phases and interaction flow.

## Reading Order

1. [FRONTEND.md](./FRONTEND.md) - baseline frontend engineering standard
2. [architecture/README.md](./architecture/README.md) - app-level architecture and boundaries
3. [architecture/strict-separation.md](./architecture/strict-separation.md) - strict route-shell split rules and migration policy
4. [local-dev-docker.md](./local-dev-docker.md) - integrated local Docker workflow and ingress model
5. [features/batch-operations-queue.md](./features/batch-operations-queue.md) - parent-child batch queue UX and state model
6. [contracts/README.md](./contracts/README.md) - API contract and generated type integration
7. [testing/README.md](./testing/README.md) - frontend test and CI gates

## Alignment Rules

- UI state names and transitions MUST match [master-flow.md](../interaction-flows/master-flow.md).
- API contracts MUST come from OpenAPI artifacts (ADR-0021/ADR-0029), not hand-written TS types.
- Full-stack acceptance MUST be tracked in [CHECKLIST.md](../CHECKLIST.md) and phase checklists under [checklist/](../checklist/).
- Route pages (`app/**/page.tsx`) MUST remain route shells; workflow logic belongs in `web/src/features/**`.

## Frontend CI Hard Gates

Frontend architecture and contract alignment are blocking checks in CI:

1. `check_frontend_route_shell_architecture.go`
2. `check_frontend_openapi_usage.go`
3. `check_frontend_no_non_english_literals.go`
4. `check_frontend_no_placeholder_pages.go`
5. `npm run typecheck --prefix web`

Current policy:

1. `docs/design/ci/allowlists/frontend_route_shell_legacy.txt` is kept empty (strict mode).
2. `docs/design/ci/locks/frontend-route-shell-legacy.lock` is kept empty (strict mode, no allowlist expansion).

## Scope Boundary

- This directory defines frontend behavior and UX contracts.
- Backend execution details remain in phase documents (`../phases/`).
