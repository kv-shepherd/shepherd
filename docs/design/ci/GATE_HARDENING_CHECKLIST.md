# CI Gate Hardening Checklist (Gate-First)

> Status: Active (2026-02-14)
> Principle: `Gate > Feature`
> Source of truth for interaction intent: [`docs/design/interaction-flows/master-flow.md`](../interaction-flows/master-flow.md)

## 1. Execution Order (Mandatory)

1. Record and verify drift facts.
2. Add blocking CI gates for verified drift.
3. Only after gates are in place, start feature implementation.

## 2. Verified Drift Register (Audited 2026-02-13)

| ID | Type | Verified Drift | Evidence | Gate Action |
|---|---|---|---|---|
| D-001 | Doc vs Impl | Checklist claimed approval cancel API completed, but OpenAPI/handler route was not implemented | `docs/design/checklist/phase-4-checklist.md`, `api/openapi.yaml`, `internal/api/handlers/server_approval.go` | ✅ Resolved (2026-02-13): cancel endpoint implemented + guarded by doc-claim consistency gate |
| D-002 | Doc vs Impl | Checklist claims `MemberHandler` completed, but `internal/api/handlers/member.go` not present | `docs/design/checklist/phase-5-checklist.md`, `internal/api/handlers/` | ✅ Resolved (2026-02-14): member handler + API routes implemented; doc-claim consistency gate strengthened for phase doc/checklist |
| D-003 | Flow vs Contract | `master-flow` contains API paths not present in OpenAPI (Batch/VNC/OIDC callback/etc.) | `docs/design/interaction-flows/master-flow.md`, `api/openapi.yaml` | ✅ Resolved (2026-02-14): alignment gate + deferred allowlist converged to empty; master-flow/API contract now strict-zero drift |
| D-004 | Runtime Policy | Namespace/cluster environment matching was not enforced in approval+worker execution path | `internal/governance/approval/gateway.go`, `internal/service/approval_validator.go`, `internal/jobs/vm_create.go` | ✅ Resolved (2026-02-13): runtime checks added + CI enforcement script |
| D-005 | Model vs Runtime | `RoleBinding.allowed_environments` exists in schema but user-facing visibility filtering is still not implemented | `ent/schema/role_binding.go`, `internal/api/handlers/environment_visibility.go`, `internal/api/handlers/server_namespace.go`, `internal/api/handlers/server_vm.go` | ✅ Resolved (2026-02-14): visibility filtering implemented and folded into environment-isolation CI gate |
| D-006 | Frontend vs Contract | Backend delete contract requires `confirm_name`, but systems frontend delete flow did not send it, causing runtime failure | `api/openapi.yaml`, `internal/api/handlers/server_system.go`, `web/src/app/(protected)/systems/page.tsx` | ✅ Resolved (2026-02-13): systems UI uses typed confirm_name + frontend/OpenAPI usage gate |
| D-007 | Frontend i18n hygiene | Frontend source contained non-English hardcoded literal outside locale catalogs | `web/src/components/layouts/AppLayout.tsx`, `web/src/i18n/locales/` | ✅ Resolved (2026-02-13): literal removed + CI scanner gate added |
| D-008 | Frontend architecture drift | Route pages mixed workflow orchestration + write API calls + large UI blocks in single `page.tsx` files | `web/src/app/(protected)/vms/page.tsx`, `web/src/app/(protected)/admin/*.tsx`, `docs/design/frontend/architecture/strict-separation.md` | ✅ Resolved (2026-02-13): affected route pages split into feature hook/components; route-shell CI gate enabled with allowlist+lock both empty (strict mode) |
| D-009 | Test backend drift | Behavior tests used SQLite in-memory DB despite PostgreSQL-only project policy | `internal/api/handlers/server_system_behavior_test.go`, `internal/governance/approval/gateway_behavior_test.go`, `go.mod` | ✅ Resolved (2026-02-14): migrated to PostgreSQL schema-isolated helper + SQLite dependency removed + CI gate added |
| D-010 | Test-trigger drift | Frontend tests were not triggered when backend changed, allowing FE/BE drift to bypass validation | `.github/workflows/frontend-tests.yml`, `.github/workflows/ci.yml` | ✅ Resolved (2026-02-14): frontend-tests workflow now runs on all PR/push; main CI adds strict test-first delta guard + frontend unit test gate |
| D-011 | Stage coverage drift | Stage 5.D delete semantics lacked dedicated system/service behavior tests and baseline CI freeze | `internal/api/handlers/server_system.go`, `internal/api/handlers/server_system_behavior_test.go`, `api/openapi.yaml` | ✅ Resolved (2026-02-15): Stage 5.D behavior tests added + `check_stage5d_delete_baseline.go` wired into strict gates |
| D-012 | Plugin-flow drift | Stage 2.B/2.C auth-provider plugin flow lacked live e2e proof (discovery → create → delete) | `web/tests/e2e/master-flow-live.spec.ts`, `docs/design/traceability/master-flow-tests.json` | ✅ Resolved (2026-02-15): live e2e flow added and bound to plugin-boundary gate |
| D-013 | Stage coverage drift | Stage 3 admin catalog (templates/instance sizes) lacked frontend live e2e proof and dedicated baseline freeze; risked regressions to placeholder/no-op UX | `internal/api/handlers/server_admin_catalog.go`, `web/tests/e2e/master-flow-live.spec.ts`, `docs/design/traceability/master-flow-tests.json` | ✅ Resolved (2026-02-15): live e2e CRUD + frontend hook tests added and bound to `check_stage3_admin_catalog_baseline.go` |
| D-014 | Stage coverage drift | Stage 4 hierarchy (system/service/member) lacked dedicated baseline freeze for create/update behavior and frontend live proof; risked behavior drift while delete tests remained green | `internal/api/handlers/server_system.go`, `internal/api/handlers/member.go`, `web/tests/e2e/master-flow-live.spec.ts` | ✅ Resolved (2026-02-15): Stage 4 baseline gate added + live e2e success path (create/update/delete) added and wired |

## 3. Gate Hardening Queue (Do First)

| Gate ID | Priority | Description | Deliverable | Status |
|---|---|---|---|---|
| GH-001 | P0 | Block "doc claims done but implementation missing" regressions | `check_doc_claims_consistency.go` + CI step | ✅ Done (2026-02-13) |
| GH-002 | P0 | Block unmanaged `master-flow` API drift from OpenAPI | `check_master_flow_api_alignment.go` + deferred allowlist | ✅ Done (2026-02-13) |
| GH-003 | P0 | Enable design-doc governance script in main CI path | CI step for `check_design_doc_governance.sh` | ✅ Done (2026-02-13) |
| GH-004 | P1 | Expand critical OpenAPI contract gate beyond notification-only scope to stage-critical endpoints | update `check_openapi_critical_contract.go` | ✅ Done (2026-02-13) |
| GH-005 | P1 | Add environment-isolation runtime enforcement gate after policy scope freeze | `check_environment_isolation_enforcement.go` + CI step + tests | ✅ Done (2026-02-13), expanded (2026-02-14) to include `allowed_environments` visibility enforcement fragments |
| GH-006 | P1 | Enforce backend operation changes are synced to frontend usage or explicit defer list | `check_frontend_openapi_usage.go` + allowlist + CI step + frontend typecheck | ✅ Done (2026-02-13), converged (2026-02-14): `frontend_openapi_unused` cleared to zero |
| GH-007 | P1 | Block frontend non-English hardcoded literals outside i18n locales | `check_frontend_no_non_english_literals.go` + CI step | ✅ Done (2026-02-13) |
| GH-008 | P1 | Enforce route-shell architecture thresholds (page size + write API aggregation) with explicit migration debt tracking and anti-regression lock | `check_frontend_route_shell_architecture.go` + legacy allowlist + lock + CI step | ✅ Done (2026-02-13) |
| GH-009 | P1 | Block placeholder-only frontend route pages from merging | `check_frontend_no_placeholder_pages.go` + CI step | ✅ Done (2026-02-14) |
| GH-010 | P1 | Freeze Stage 5.E batch baseline once implemented (OpenAPI + handler + allowlist hygiene) | `check_stage5e_batch_baseline.go` + CI step | ✅ Done (2026-02-14) |
| GH-011 | P1 | Enforce spec-driven stage test coverage (strict profile: all master-flow stages must map to executable tests, deferred list stays empty) | `check_master_flow_test_matrix.go` + `master-flow-tests.json` + deferred allowlist + CI step | ✅ Done (2026-02-14), upgraded to full-stage strict profile (2026-02-14) |
| GH-012 | P1 | Block SQLite usage in tests and module dependencies (PostgreSQL-only) | `check_no_sqlite_in_tests.go` + CI step | ✅ Done (2026-02-14) |
| GH-013 | P0 | Enforce strict test-first by diff: runtime code changes must include corresponding test changes | `check_changed_code_has_tests.sh` + CI step + allowlist | ✅ Done (2026-02-14) |
| GH-014 | P0 | Add end-to-end strict master-flow gate chain in main CI (traceability + backend PG behavior suites + frontend unit/e2e) | `.github/workflows/ci.yml` job `master-flow-strict` | ✅ Done (2026-02-14) |
| GH-015 | P1 | Add explicit "full completion claim" gate: fail if any deferred/exemption allowlist remains non-empty | `check_master_flow_completion_readiness.go` + strict-flow docs | ✅ Done (2026-02-14) |
| GH-016 | P1 | Freeze Stage 5.D delete baseline once implemented (OpenAPI + runtime handlers + backend/frontend tests) | `check_stage5d_delete_baseline.go` + CI step | ✅ Done (2026-02-15) |
| GH-017 | P1 | Bind Stage 2.B/2.C auth-provider plugin live e2e flow into plugin-boundary gate | `check_auth_provider_plugin_boundary.go` + live e2e spec fragments | ✅ Done (2026-02-15) |
| GH-018 | P1 | Freeze Stage 3 admin catalog baseline (templates + instance sizes: OpenAPI/runtime/RBAC/frontend tests/live e2e) | `check_stage3_admin_catalog_baseline.go` + `make master-flow-strict` integration | ✅ Done (2026-02-15) |
| GH-019 | P1 | Freeze Stage 4 system/service/member baseline (OpenAPI/runtime/RBAC/frontend tests/live e2e) | `check_stage4_system_service_baseline.go` + `make master-flow-strict` integration | ✅ Done (2026-02-15) |
| GH-020 | P1 | Upgrade Stage 5.E baseline to include frontend queue UX hard requirements (`status_url`, 429 cooldown, affected-child feedback, aria-live) | `check_stage5e_batch_baseline.go` frontend fragment assertions + controller tests | ✅ Done (2026-02-15) |
| GH-021 | P1 | Freeze Stage 6 replay-store baseline to prevent regression to process-local token replay semantics | `check_stage6_vnc_baseline.go` + PG replay store + cross-instance token tests | ✅ Done (2026-02-15) |

## 4. Deferred API Scope (Explicitly Tracked)

Deferred or roadmap APIs must be listed in:

- `docs/design/ci/allowlists/master_flow_api_deferred.txt`

Any API path present in `master-flow` but missing from OpenAPI MUST be either:

1. implemented, or
2. listed in the deferred allowlist with reason.

No implicit drift is allowed.

## 5. Documentation Synchronization Rules

1. Checklist cannot mark an item as done without code + contract evidence.
2. If implementation is missing, reset status immediately (`[ ]` / progress downgrade).
3. If behavior deviates from design, keep it in this checklist until a blocking gate exists.

## 6. Local Verification Commands

```bash
go run docs/design/ci/scripts/check_doc_claims_consistency.go
go run docs/design/ci/scripts/check_master_flow_api_alignment.go
go run docs/design/ci/scripts/check_environment_isolation_enforcement.go
go run docs/design/ci/scripts/check_stage3_admin_catalog_baseline.go
go run docs/design/ci/scripts/check_stage4_system_service_baseline.go
go run docs/design/ci/scripts/check_stage5d_delete_baseline.go
go run docs/design/ci/scripts/check_stage5e_batch_baseline.go
go run docs/design/ci/scripts/check_stage6_vnc_baseline.go
go run docs/design/ci/scripts/check_master_flow_test_matrix.go
go run docs/design/ci/scripts/check_frontend_openapi_usage.go
go run docs/design/ci/scripts/check_frontend_no_non_english_literals.go
go run docs/design/ci/scripts/check_frontend_no_placeholder_pages.go
go run docs/design/ci/scripts/check_frontend_route_shell_architecture.go
bash docs/design/ci/scripts/check_changed_code_has_tests.sh
go run docs/design/ci/scripts/check_no_sqlite_in_tests.go
go run docs/design/ci/scripts/check_master_flow_completion_readiness.go
bash docs/design/ci/scripts/check_design_doc_governance.sh

# Isolated Docker PostgreSQL execution
./scripts/run_with_docker_pg.sh -- make master-flow-strict
```
