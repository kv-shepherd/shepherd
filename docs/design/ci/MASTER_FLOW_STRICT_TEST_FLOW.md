# Master-Flow Strict Test Flow (Spec-Driven)

> Source of truth: `docs/design/interaction-flows/master-flow.md`  
> Related: `docs/design/traceability/master-flow-tests.json`, ADR-0034 strict profile

## 1. Goal

Use executable tests to constrain both backend and frontend delivery.
Rule: **if tests fail against master-flow intent, implementation must be reworked**.

## 2. Entry Preconditions

1. `master-flow.md` stage/branch behavior is explicit.
2. Stage is mapped in `docs/design/traceability/master-flow-tests.json`.
3. No hidden bypass:
   - no runtime mock wiring
   - no runtime placeholders
   - PostgreSQL-only test backend
   - no network route mocking in strict live e2e (`check_live_e2e_no_mock.sh`)
4. Runtime code diff includes corresponding test diff (strict delta guard).

## 3. Execution Order (Mandatory)

1. Contract and traceability gates
   - `check_master_flow_api_alignment.go`
   - `check_master_flow_test_matrix.go`
   - `check_master_flow_traceability.go`
2. Test-first delta gate
   - `check_changed_code_has_tests.sh`
3. Backend behavior suites (PostgreSQL)
   - handler/usecase/gateway/job behavior tests for changed stages
4. Frontend suites
   - `typecheck`
   - unit/integration tests (`vitest`)
   - strict live e2e (`playwright`) against real backend, no `page.route` mocks
5. Full CI gates
   - run all strict scripts in `docs/design/ci/scripts/`

Any failure in steps 1-5 is a hard stop for merge.

## 3.1 Completion Claim Rule (No Deferred Debt)

Passing strict tests means implementation is valid for current scope.
Claiming "**master-flow fully completed**" additionally requires:

1. No deferred API entries.
2. No deferred stage-test entries.
3. No frontend/OpenAPI unused allowlist debt.
4. No strict-gate exemption debt.

Run:

```bash
go run docs/design/ci/scripts/check_master_flow_completion_readiness.go
```

If this check fails, the project is still in "partially implemented / deferred" state.

## 4. Red/Green Rule

1. Write or update tests from `master-flow.md` behavior.
2. Run tests and observe red.
3. Implement until green.
4. Do not weaken assertions to force green unless `master-flow.md` is updated in same change set.

## 5. Frontend/Backend Sync Rule

When backend contract/behavior changes:

1. OpenAPI + generated types update.
2. Frontend usage update.
3. Frontend tests update (unit/e2e).
4. CI must pass in the same delivery set.

No “backend first, frontend later” merge in strict mode.

## 6. Local Strict Run

```bash
go run docs/design/ci/scripts/check_master_flow_api_alignment.go
go run docs/design/ci/scripts/check_master_flow_test_matrix.go
go run docs/design/ci/scripts/check_master_flow_traceability.go
bash docs/design/ci/scripts/check_changed_code_has_tests.sh
go run docs/design/ci/scripts/check_no_sqlite_in_tests.go
go run docs/design/ci/scripts/check_stage3_admin_catalog_baseline.go
go run docs/design/ci/scripts/check_stage4_system_service_baseline.go
go run docs/design/ci/scripts/check_stage5d_delete_baseline.go
bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
go run docs/design/ci/scripts/check_frontend_openapi_usage.go
go run docs/design/ci/scripts/check_frontend_no_placeholder_pages.go
go run docs/design/ci/scripts/check_doc_claims_consistency.go
go run docs/design/ci/scripts/check_master_flow_completion_readiness.go # for full-completion claim

# then execute stage-relevant backend and frontend test suites
```

CI reference: `.github/workflows/ci.yml` job `master-flow-strict`.

Convenience targets:

```bash
make master-flow-strict      # requires DATABASE_URL
make master-flow-completion  # full-completion claim check
make master-flow-strict-docker-pg # auto-provision isolated Docker PostgreSQL
make test-backend-docker-pg       # backend PostgreSQL suites with isolated Docker PostgreSQL
bash scripts/run_e2e_live.sh --no-db-wrapper
./scripts/run_with_docker_pg.sh -- make master-flow-strict
```

`scripts/run_e2e_live.sh` defaults `NEXT_PUBLIC_API_URL` to same-origin `/api/v1`
and uses Next.js rewrite (`INTERNAL_API_URL`) to avoid live-e2e CORS false negatives
caused by random Playwright web ports.

## 7. Ambiguity Handling

If master-flow wording and implementation/test expectations conflict:

1. Stop implementation.
2. Record conflict in design notes.
3. Clarify and update source-of-truth docs first.
4. Resume test/implementation after clarification.
