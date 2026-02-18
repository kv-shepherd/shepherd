# Frontend Strict Separation Standard

> **Scope**: `web/src/**`
> **Authority**: `docs/design/interaction-flows/master-flow.md`
> **Related**: ADR-0020, ADR-0021, ADR-0027, ADR-0030

## Why strict separation

Backend already splits by action and lifecycle (`create/delete/power`, `usecase/job/handler`).
Frontend must enforce equivalent boundaries; otherwise page files become orchestration + workflow + rendering + API wiring mixed together, which causes drift and hidden regressions.

## Mandatory boundaries

1. `app/**/page.tsx` is a route shell, not a workflow container.
2. Workflow orchestration belongs to feature hooks (`web/src/features/**/hooks`).
3. Large UI blocks (table, wizard, forms, dialogs) belong to feature components (`web/src/features/**/components`).
4. `page.tsx` must not directly own many mutation endpoints (`POST/PUT/PATCH/DELETE`); write actions are delegated to feature hooks.
5. OpenAPI-generated types remain the only contract type source (`web/src/types/api.gen.ts`).
6. Query keys must include all variables used by query functions.

## Rule source mapping

1. Next.js App Router guidance: page as server/client boundary and composition entry, with data logic moved to dedicated components/modules.
2. React guidance: extract reusable logic into custom hooks, keep component intent-focused, avoid side effects in render.
3. TanStack Query guidance: query keys must include function dependencies; enable eslint plugin rules for query safety.
4. Local rule pack: `ai-code/.agent/skills/vercel-react-best-practices/rules/` reinforces rerender/data-fetch/bundle separation patterns.

## CI-enforced subset

1. Route shell architecture gate:
   - max route page line count threshold.
   - mutation endpoint count threshold per route page.
   - optional route-level allowlist only for staged migration.
2. Frontend/OpenAPI usage sync gate:
   - every OpenAPI operation must be consumed or explicitly deferred.
3. Non-English literal gate:
   - hardcoded non-English strings forbidden outside locale resources.
4. Type safety gate:
   - frontend `typecheck` must pass in CI.

## Non-automatable checks (review required)

1. UI state transitions fully aligned with `master-flow.md`.
2. UX-level error handling semantics (retry/cancel/backoff) are consistent with backend behavior.
3. Component cohesion and naming quality.

## Migration policy

1. New/changed routes must satisfy strict separation immediately.
2. Existing legacy routes may use temporary allowlist entries with explicit cleanup ticket linkage.
3. Allowlist entries are removed as routes are refactored; no permanent exemptions.

## Current status (2026-02-13)

1. `frontend_route_shell_legacy.txt` is empty (strict mode).
2. `frontend-route-shell-legacy.lock` is empty and enabled to block allowlist expansion by default.
