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
3. [local-dev-docker.md](./local-dev-docker.md) - integrated local Docker workflow and ingress model
4. [features/batch-operations-queue.md](./features/batch-operations-queue.md) - parent-child batch queue UX and state model
5. [contracts/README.md](./contracts/README.md) - API contract and generated type integration
6. [testing/README.md](./testing/README.md) - frontend test and CI gates

## Alignment Rules

- UI state names and transitions MUST match [master-flow.md](../interaction-flows/master-flow.md).
- API contracts MUST come from OpenAPI artifacts (ADR-0021/ADR-0029), not hand-written TS types.
- Full-stack acceptance MUST be tracked in [CHECKLIST.md](../CHECKLIST.md) and phase checklists under [checklist/](../checklist/).

## Scope Boundary

- This directory defines frontend behavior and UX contracts.
- Backend execution details remain in phase documents (`../phases/`).
