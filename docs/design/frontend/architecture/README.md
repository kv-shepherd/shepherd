# Frontend Architecture Layer

> **Related ADRs**: [ADR-0020](../../../adr/ADR-0020-frontend-technology-stack.md), [ADR-0027](../../../adr/ADR-0027-repository-structure-monorepo.md), [ADR-0030](../../../adr/ADR-0030-design-documentation-layering-and-fullstack-governance.md)

## Goals

- Keep UI architecture consistent with monorepo contract-first development.
- Make backend flow changes traceable to frontend behavior updates.
- Avoid ad-hoc page-level logic that bypasses shared state/query/error patterns.

## Mandatory Boundaries

- `web/src/types/api.gen.ts` is the only backend contract type source.
- Domain-level UI state is managed through `zustand` stores and `tanstack-query` queries/mutations.
- Queue-style async operations (approval, batch, power ops) must expose explicit status, retry, and cancellation actions in UI.

## Cross-References

- Batch queue model: [features/batch-operations-queue.md](../features/batch-operations-queue.md)
- OpenAPI contract workflow: [../contracts/README.md](../contracts/README.md)
- Testing and CI gates: [../testing/README.md](../testing/README.md)
