# ADR-0037 Implementation Note: OpenAPI Validation Architecture and Enforcement Policy

Status: Draft implementation note for ADR-0037 (`proposed`)  
Linked ADR: `docs/adr/ADR-0037-openapi-validation-architecture-and-enforcement-policy.md`

## Purpose

This note captures implementation impacts while ADR-0037 is in review. It is intentionally operational and file-oriented.

## Scope

1. Runtime validator migration to `libopenapi-validator` strict mode.
2. Business validator unification (`validator/v10`).
3. Spec-driven validation tags (`x-oapi-codegen-extra-tags`).
4. CI enforcement and branch protection hardening.

## Planned Change Set

### 1. API contract and lint baseline

- `api/openapi.yaml`
  - replace OpenAPI 3.0 `nullable: true` style with OpenAPI 3.1-compatible union types.
  - add/normalize validation tags on request schemas via `x-oapi-codegen-extra-tags`.
- `api/.vacuum.yaml`
  - keep Vacuum ruleset.
  - enforce project naming policy (snake_case) without opening naming drift.

### 2. Runtime validation

- `internal/api/middleware/openapi_validator.go`
  - migrate runtime request validation to `libopenapi-validator` strict mode.
  - use embedded spec source (`generated.GetSwagger()`) instead of runtime file path dependency.
- `internal/api/middleware/openapi_validator_test.go`
  - add strict coverage for undeclared fields/params and production-safe error behavior.

### 3. Business validation

- `internal/api/validator/validator.go` (new)
  - centralize `validator/v10` initialization.
  - standardize field-level error mapping for API responses.
- `internal/api/validator/validator_test.go` (new)
  - cover required/format/cross-field scenarios.
- `internal/api/handlers/*.go`
  - gradually replace duplicated request checks with centralized validator path.

### 4. CI and governance

- `build/api.mk`
  - ensure compatibility alias: `api-diff -> api-breaking`.
- `.github/workflows/api-contract.yaml`
  - enforce lint, sync, and breaking gates.
  - add PR visibility for API changelog output.
- GitHub branch protection (repository settings)
  - require status checks on `main`.

## Verification Plan

1. `make api-lint` -> `0 errors`
2. `make api-generate && make api-check`
3. `go test ./internal/api/middleware/...`
4. `go test ./internal/api/validator/...`
5. PR-level breaking-check blocking behavior verified

## Rollout Strategy

Use sequential atomic PRs, one concern per PR, with baseline refresh between each merge.

## Deferred Until ADR Acceptance

1. Amendment block append in ADR-0029.
2. Normative documentation rewrite beyond minimal references.

