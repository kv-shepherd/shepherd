# Frontend Contract Layer

> **Related ADRs**: [ADR-0021](../../../adr/ADR-0021-api-contract-first.md), [ADR-0029](../../../adr/ADR-0029-openapi-toolchain-governance.md)

## Contract Rules

- OpenAPI spec is the source of truth.
- Frontend request/response types MUST be generated, not manually duplicated.
- Any API behavior relied on by frontend (status values, pagination fields, retry headers) must exist in OpenAPI and be validated in CI.

## Required Artifacts

- `api/openapi.yaml` (canonical)
- `internal/api/generated/` (Go)
- `web/src/types/api.gen.ts` (TypeScript)

## CI Gates

- `make api-lint`
- `make api-generate`
- `make api-check`
- `make api-compat` when 3.1-only features are used

## Flow Alignment

For asynchronous batch flows, frontend must rely on server status endpoints and rate-limit headers instead of assuming local completion.

See: [batch-operations-queue.md](../features/batch-operations-queue.md)
