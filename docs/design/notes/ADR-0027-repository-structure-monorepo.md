# ADR-0027 Design Notes: Monorepo Repository Structure with web/

> **Status**: Pending ADR-0027 acceptance

## Summary

- Keep frontend in this repository under `web/`.
- Generate TypeScript types into `web/src/types/api.gen.ts` via `make api-generate`.
- Maintain atomic changes for `api/openapi.yaml`, Go server code, and frontend types.

## Rationale

- `docs/design/ci/makefile/api.mk` already targets `web/` for TypeScript generation.
- Solo maintenance favors a single CI and review workflow.
- Avoid cross-repo version drift for API contracts.

## Follow-ups (post-acceptance)

- Ensure root README documents the `web/` directory.
- If a frontend-only release becomes necessary, revisit ADR-0027.
