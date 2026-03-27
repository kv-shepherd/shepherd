# ADR-0037 Implementation Note: OpenAPI Validation Architecture and Enforcement Policy

Status: Implementation note for ADR-0037 (`accepted`)
Linked ADR: `docs/adr/ADR-0037-openapi-validation-architecture-and-enforcement-policy.md`

> **rev.3 Change Summary**:
> 1. Added frontend toolchain impact overview (completely missing from original design note)
> 2. Refined compat bridge layer exit criteria with automated detection triggers
> 3. Added compat script robustness improvements (3.1-only keywords + multi-element union detection)
> 4. Added Go codegen alternative evaluation record (ogen vs oapi-codegen conclusion)
> 5. Validated best practices against context7 docs for libopenapi/oapi-codegen/validator
> 6. Added monitoring and alerting strategy (internet search best practices)
> 7. Added spec loading security considerations
> 8. Enhanced CI governance coverage (contract testing + spec drift prevention)
>
> **rev.4 Change Summary (post-review fixes)**:
> 1. **BLOCKING FIX**: Runtime validator must use `//go:embed` canonical spec, NOT `generated.GetSwagger()` (resolves compat scope contradiction)
> 2. **BLOCKING FIX**: CI ordering corrected: `api-compat-generate` → `api-generate` → `api-compat` → sync diff; `api-check.sh` added to PR-6 changelist
> 3. PR-1 required checks now reference specific job/check names, not workflow step names
> 4. Compat fail-closed upgraded from regex to YAML-aware AST/object traversal; block-style union coverage added
> 5. Added libopenapi-validator concurrency/lifecycle policy and `-race` test requirement for PR-3
>
> **rev.5 Change Summary (second review fixes)**:
> 1. **BLOCKING FIX**: `//go:embed ../../api/openapi.yaml` is illegal (Go embed forbids `..`); replaced with `internal/api/specembed/` derived-file approach
> 2. Cleaned ALL stale `generated.GetSwagger()` references from best-practice alignment section
> 3. Clarified strict mode API: `validator.NewValidator(doc, config.WithStrictMode())` per pb33f official docs (not a separate `strict` sub-package)
> 4. Added `-race` flag to PR-3 acceptance commands for consistency
> 5. Changed `validator/v10` error handling from bare type assertion to checked assertion (`errors.As`) + `InvalidValidationError` handling
> 6. Added dependency management plan for compat YAML AST parsing upgrade
> 7. Synced version metadata
>
> **rev.6 Change Summary (best-practice cleanup)**:
> 1. Corrected `PyYAML` statement: it is third-party (not Python stdlib), so CI must install from pinned requirements
> 2. Clarified strict API wording: prefer top-level `validator.NewValidator(..., config.WithStrictMode())`; `strict` package is internal/advanced path and not needed here
> 3. Hardened dependency reproducibility guidance for YAML AST conversion in blocking CI gates
>
> **rev.7 Change Summary (best-practice cross-audit)**:
> 1. All dependency versions confirmed via pkg.go.dev version listing pages (libopenapi v0.33.11, libopenapi-validator v0.12.1, validator/v10 v10.30.1 all verified)
> 2. Refined PR-3 concurrency validation strategy: default conservative (sync.Pool/per-request) with benchmark-driven conditional switch
> 3. Added post-PR-7 monitoring enhancement plan with concrete Prometheus metric definitions and implementation timeline
> 4. Added version verification record section with full audit trail
>
> **rev.8 Change Summary (implementation-feasibility corrections)**:
> 1. **BLOCKING FIX**: PR-3 concurrency policy updated to per-validation instance creation only (no shared/sync.Pool reuse), aligned with `libopenapi-validator/strict` docs
> 2. **BLOCKING FIX**: compat exit steps corrected to require disabling/removing compat before canonical codegen verification (matches current `build/api.mk` behavior)
> 3. Fixed typo in rollout command examples and synchronized source-evidence wording
>
> **rev.9 Change Summary (Go-native tooling + final audit refinements)**:
> 1. **PR-2 major upgrade**: Replaced Python/PyYAML compat script with Go-native tool (`cmd/openapi-compat-gen/main.go` using `gopkg.in/yaml.v3` AST traversal). Evaluated and excluded `dense-analysis/openapi-spec-converter` (depends on `kin-openapi` + requires Node.js for tests).
> 2. **PR-3 API selection**: Clarified that `NewDocument([]byte)` is the correct and optimal choice for self-contained embedded spec; `NewDocumentWithConfiguration` is only relevant when external `$ref` file references need resolution — not applicable here.
> 3. **PR-4 defensive addition**: Confirmed `gin.Recovery()` middleware should be registered at outermost router layer as last-resort panic defense for `InvalidValidationError` edge cases.
> 4. Added `gopkg.in/yaml.v3` (already an indirect dependency) to version baseline table.
>
> **rev.10 Change Summary (PR-2 execution backfill)**:
> 1. Marked PR-2 as completed in progress tracking.
> 2. Backfilled concrete execution evidence: Go compat generator implementation, Makefile wiring, legacy script removal, and test/verification command results.
> 3. Kept remaining PR-3~PR-7 items unchanged for sequential execution.
>
> **rev.11 Change Summary (PR-3 execution backfill)**:
> 1. Marked PR-3 as completed in progress tracking.
> 2. Backfilled runtime validator migration evidence: canonical spec embedding (`internal/api/specembed`), middleware migration to `libopenapi-validator` strict mode, and Makefile sync integration.
> 3. Backfilled verification evidence including middleware `-race` tests and frontend E2E smoke regression (`35 passed`).
>
> **rev.12 Change Summary (scope cleanup)**:
> 1. Simplified PR-2/PR-3 sections in this ADR note to retain only progress and test records.
> 2. Moved submission-scoped details (file lists/splitting guidance) to `REMEDIATION-PLAN-BEST-STATE.md`.
>
> **rev.13 Change Summary (revalidation backfill)**:
> 1. Backfilled final local revalidation results for PR-2/PR-3 scope.
> 2. Added `make api-check` and API middleware race-test rerun records.
> 3. Confirmed frontend E2E re-run remains green (`35 passed`).
>
> **rev.14 Change Summary (validator instance lifecycle optimization)**:
> 1. **Source-confirmed optimization**: Read `libopenapi-validator@v0.12.1/validator.go` directly — `NewValidator()` calls `BuildV3Model()` and `warmSchemaCaches()` (pre-compiles all schemas). Original middleware created **two** validator instances per request (one for request, one for response validation), incurring double `warmSchemaCaches` cost.
> 2. **Interim optimization (later superseded in rev.16)**: Middleware briefly reused one validator instance within a single request goroutine for request+response validation to avoid duplicate schema warm-up.
> 3. **Supersession**: rev.16 reverted to strict per-validation creation to align exactly with `libopenapi-validator/strict` guidance and ADR-0037 lifecycle wording.
> 4. **Error semantic fix**: Validator creation failure changed from 400 (incorrect: route error semantics) to 500 (correct: server initialization failure), error code `OPENAPI_VALIDATOR_UNAVAILABLE`.
> 5. Verified: `go test -race -v ./internal/api/middleware/...` ✅ (7 tests pass, no data races).
>
> **rev.15 Change Summary (PR-4~PR-7 execution backfill)**:
> 1. Marked PR-4/PR-5/PR-6/PR-7 as completed in execution tracking.
> 2. Backfilled PR-4: unified `internal/api/validator` package, `WithRequiredStructEnabled`, field-level error mapping, and request validation helper wiring.
> 3. Backfilled PR-5: OpenAPI request schema `x-oapi-codegen-extra-tags.validate` rollout + generated tag visibility + handler bind/validate consolidation.
> 4. Backfilled PR-6: CI ordering fixed to `api-compat-generate -> api-generate -> api-compat -> sync diff`, compat enforced by default, changelog PR comment publishing, contract-test gate enabled, and frontend type sync check added.
> 5. Backfilled PR-7: ADR amendment block + CONTRIBUTING/API contract guidance + rollout comment template alignment.
>
> **rev.16 Change Summary (consistency remediation)**:
> 1. Fixed CI required-check reliability: removed workflow path filters so required checks do not stay pending when files outside filter change.
> 2. Re-aligned middleware with strict lifecycle policy: validator instance is now created per validation call (request and response each create their own instance), no reuse.
> 3. Added release-mode regression test to ensure request validation errors return generic messages without detailed leakage.
> 4. Cleaned stale design-template/tooling references to removed `openapi-compat-generate.sh` script and synchronized with Go-native compat generator.

---

## Purpose

This note captures implementation impacts for ADR-0037. It is intentionally operational and file-oriented. It augments — but does not replace — the remediation plan (`ai-code/rewire-codex/REMEDIATION-PLAN-BEST-STATE.md`).

---

## Execution Tracking (WIP on `wip/implementation`)

### Placement Decision

Use `docs/design/notes/` for ADR-0037 execution tracking because it is non-normative and ADR-oriented. The normative decision is recorded in accepted ADR-0037.

### Serial Execution Order

1. PR-1 Governance and compatibility baseline
2. PR-2 OpenAPI 3.1 spec fixes and lint green (incl. frontend type regen)
3. PR-3 Runtime middleware migration to `libopenapi-validator` (incl. frontend E2E regression)
4. PR-4 Unified business validator package
5. PR-5 Spec-driven validate tags + handler migration (incl. frontend typecheck)
6. PR-6 CI completion (PR comment + contract-test gate + frontend type sync gate)
7. PR-7 ADR/docs/contributing alignment (post-acceptance items only where normative)

### Progress

- [x] PR-1 local compatibility alias (`api-diff`) added in `build/api.mk`
- [x] PR-1 workflow permission prepared in `.github/workflows/api-contract-validation.yml` (`pull-requests: write`)
- [ ] PR-1 branch protection applied in GitHub UI (deferred until ADR path is approved for rollout)
- [x] PR-2 OpenAPI 3.1 nullable fixes + Vacuum lint green + frontend type regen
- [x] PR-3 runtime validator migration + frontend E2E regression
- [x] PR-4 unified business validator package
- [x] PR-5 spec-driven validate tags and handler migration + frontend typecheck
- [x] PR-6 CI completion and PR comment/blocking gates + frontend type sync gate
- [x] PR-7 ADR/docs/contributing alignment (acceptance-dependent items)

### PR-2 Progress and Test Record

> Submission-scoped file lists are maintained in `ai-code/rewire-codex/REMEDIATION-PLAN-BEST-STATE.md` only.

Verification executed (local):

1. `go test ./cmd/openapi-compat-gen/...`
2. `make api-compat-generate`
3. `make api-compat`
4. `make api-generate`
5. `make api-lint`
6. `cd web && npm run typecheck`
7. `cd web && npm run test:run`
8. `go build ./...`

### PR-3 Progress and Test Record

> Submission-scoped file lists are maintained in `ai-code/rewire-codex/REMEDIATION-PLAN-BEST-STATE.md` only.

Verification executed (local):

1. `go test ./internal/api/specembed ./internal/api/middleware`
2. `go test -race ./internal/api/middleware/...`
3. `go test ./internal/api/...`
4. `go test ./internal/app/...`
5. `make api-check`
6. `go build ./...`
7. `cd web && npm run test:e2e` (`35 passed`)

### PR-4 Progress and Test Record

Verification executed (local):

1. `go get github.com/go-playground/validator/v10@v10.30.1`
2. `go test ./internal/api/validator/...`
3. `go test ./internal/api/handlers/...`
4. `go test ./...`

### PR-5 Progress and Test Record

Verification executed (local):

1. `make api-compat-generate`
2. `make api-generate`
3. `rg -n 'validate:\"' internal/api/generated/server.gen.go`
4. `go test ./internal/api/...`
5. `cd web && npm run typecheck`
6. `cd web && npm run test:run`

### PR-6 Progress and Test Record

Verification executed (local):

1. `REQUIRE_OPENAPI_COMPAT=1 make api-check`
2. `make api-compat`
3. `make api-lint`
4. `make api-contract-test`
5. `go test -race ./internal/api/middleware/...`

---

## Empirical Toolchain Checkpoint

1. Canonical spec should still move to pure OpenAPI 3.1 syntax (`type: ["<type>", "null"]`) for correctness.
2. However, current Go codegen is not yet fully compatible on this repository's schemas:
   - local repro with `oapi-codegen v2.5.1` still fails on `type: ["object", "null"]`
   - failing field: `ApprovalTicket.ticket_payload`
   - **Internet search confirmed**: `oapi-codegen` still does not support OpenAPI 3.1 `type: [T, "null"]` syntax. Their roadmap lists this as "as needs arise" priority with no confirmed timeline.
3. Therefore, do **not** revert canonical spec back to 3.0 syntax.
4. Keep/use a generated 3.0-compatible artifact (`api/openapi.compat.yaml`) as a temporary bridge **for Go codegen only** until an empirically verified `oapi-codegen` version passes on this spec.
5. PR-2 execution should include compat artifact generation in the validation path when 3.1 unions are present.
6. **Frontend is unaffected**: `openapi-typescript v7.x` fully supports OpenAPI 3.1 natively (`type: ["string", "null"]` maps to `string | null`). Frontend reads the canonical spec directly and does not require the compat artifact.
7. **(rev.4 CRITICAL)** **Runtime validation must NOT use the compat artifact.** The current `generated.GetSwagger()` returns an embedded spec that comes from the compat-fed codegen pipeline (`build/api.mk:50` auto-selects `openapi.compat.yaml` when present → `oapi-codegen` embeds it via `embedded-spec: true` in `api/oapi-codegen.yaml:8`). This means `GetSwagger()` indirectly returns the 3.0-downgraded spec, contradicting the principle that compat is only for codegen, not for runtime validation.
8. **(rev.5 BLOCKING FIX)** Go `//go:embed` forbids `..` path elements (see [embed package docs](https://pkg.go.dev/embed)). Therefore, `//go:embed ../../api/openapi.yaml` is **uncompilable**. The correct approach is:
   - Create `internal/api/specembed/` package with a `spec.go` file containing `//go:embed openapi.yaml`.
   - A build-time copy step (Makefile target or CI step) copies `api/openapi.yaml` → `internal/api/specembed/openapi.yaml`.
   - The middleware imports `specembed.CanonicalSpec` (the embedded `[]byte`) and passes it to `libopenapi.NewDocument(specembed.CanonicalSpec)`.
   - CI sync check (`make api-check`) verifies the copy is fresh.
   - This is the same derived-file pattern used by `api/openapi.compat.yaml`, providing a precedent.

---

## Go Codegen Alternative Evaluation (Assessment)

### Core Question

The compat bridge layer exists solely because `oapi-codegen` does not support OpenAPI 3.1 `type: [T, "null"]`. Can switching codegen tools eliminate the compat layer?

### Candidate Comparison

| Dimension | `oapi-codegen` v2.5.x | `ogen-go/ogen` | OpenAPI Generator |
|-----------|----------------------|----------------|-------------------|
| OpenAPI 3.1 support | ❌ No `type: [T, "null"]` | ⚠️ Partial (Issue #1617 Open) | ⚠️ Experimental |
| Gin integration | ✅ Native `gin-server` output | ❌ Built-in router only | ✅ Configurable |
| Go 1.25 `omitzero` | ✅ v2.5+ supported | ❌ Uses `Opt[T]` wrapper | ❌ Not supported |
| `x-oapi-codegen-extra-tags` | ✅ Native support | ❌ Not supported | ❌ Not supported |
| Community maturity | ✅ High | ⚠️ Medium | ✅ High (Go quality varies) |

### Conclusion

**No Go codegen tool currently exists that can directly replace `oapi-codegen` while satisfying all project constraints.**

> **Key finding** (context7 + internet search verified): `ogen` also cannot process `type: [T, "null"]` syntax (Issue #1617 Open), uses its own router incompatible with Gin, and does not support `x-oapi-codegen-extra-tags`. Migration cost is prohibitively high and would not eliminate the compat layer.

**Recommendation: Maintain current approach** — retain `oapi-codegen` + compat bridge layer with strict exit strategy enforcement.

---

## Compat Bridge Governance (Temporary, Mandatory)

### Fundamental Principles

1. `api/openapi.yaml` remains the only canonical contract source and must stay pure OpenAPI 3.1 syntax.
2. `api/openapi.compat.yaml` is a derived artifact used **exclusively** for Go codegen (`oapi-codegen`). It is NOT used for:
   - Runtime validation (which must use canonical spec via `//go:embed`)
   - Lint (`vacuum` always uses canonical)
   - Frontend type generation (`openapi-typescript` always uses canonical)
   - Architecture truth or documentation
3. **Frontend does not use the compat artifact** — `openapi-typescript v7.x` reads the canonical 3.1 spec directly.
4. Compat conversion must fail closed:
   - unsupported / unsafe 3.1 constructs must cause generation failure
   - no silent best-effort downgrade
5. CI must enforce compat hygiene:
   - compat generation executed in CI
   - compat freshness checked as blocking gate
   - compat missing should be blocking once PR-6 enables `REQUIRE_OPENAPI_COMPAT=1`

### Compat Corrosion Risks (What We Are Preventing)

1. **Ghost artifact drift**: canonical spec changes but compat artifact is stale or missing.
2. **Converter coverage drift**: new 3.1 patterns added that compat script cannot safely transform.
3. **Semantic drift**: compat output compiles, but generated Go semantics diverge subtly from canonical 3.1 intent.
4. **Frontend type drift** (rev.3): canonical spec modified but frontend types not regenerated.

### Compat Script Robustness Improvements (Must be completed in PR-2)

#### (rev.9) Go-native AST Processing for `type` Field

> **Review finding**: line-level regex cannot catch block-style YAML union syntax:
> ```yaml
> type:
>   - string
>   - 'null'
> ```
> This can silently bypass regex logic and violates fail-closed requirements.

PR-2 standardizes on a Go-native compat generator (`cmd/openapi-compat-gen/main.go`) using `gopkg.in/yaml.v3` `yaml.Node` traversal.

Execution requirements:

1. Detect and fail-closed on all listed OpenAPI 3.1-only keywords (`jsonSchemaDialect`, `unevaluatedProperties`, `dependentSchemas`, `prefixItems`, `minContains`, `maxContains`, `contentEncoding`, `contentMediaType`, `$dynamicRef`, `$dynamicAnchor`, `if`, `then`, `else`, `const`).
2. Convert only safe two-element nullable unions (`["T","null"]` / `["null","T"]`) into OpenAPI 3.0-compatible `type: T` + `nullable: true`.
3. Fail-closed on any 3+ element union (e.g. `["string","integer","null"]`).
4. Handle both inline and block-style union syntax via AST traversal, not line regex.
5. Provide table-driven tests in `cmd/openapi-compat-gen/main_test.go` for success and fail-closed paths.
6. Keep CI/toolchain homogeneous: `api-compat-generate` calls `go run ./cmd/openapi-compat-gen/main.go ...` and no Python/pip requirements file is needed.

### Compat Layer Exit Strategy (Hard Criteria — Must Be Written Into ADR-0037 Decision)

#### Automated Detection Triggers

1. **`canonical codegen probe` CI job** (PR-6 optional enhancement): periodically run `oapi-codegen` directly against the canonical 3.1 spec.
   - If successful = compat exit signal; automatically create an Issue for notification.
   - Recommended as a scheduled workflow (weekly or on each `oapi-codegen` new version release).

#### Hard Exit Conditions (Any one sufficient to initiate exit)

1. `oapi-codegen` releases a version that declares support for OpenAPI 3.1 `type: [T, "null"]` syntax.
2. This repository's `canonical codegen probe` CI job passes on 2 consecutive versions.
3. A mature alternative codegen tool emerges that satisfies all constraints: Gin support + 3.1 support + extra-tags support + omitzero support.

#### Soft Exit Triggers (Require retrospective but do not force exit)

1. Compat layer persists across 2 `oapi-codegen` release versions.
2. Compat bridge script maintenance cost exceeds expectations (e.g. frequent script modifications due to new 3.1 syntax).
3. More than 180 days since compat layer establishment.

#### Exit Execution Steps (Pre-documented)

1. Disable/remove compat artifact first (at minimum ensure `api/openapi.compat.yaml` is absent), so canonical codegen path is actually exercised.
2. Confirm canonical direct codegen succeeds (e.g., `make api-generate` passes with no compat file, or `oapi-codegen ... api/openapi.yaml` passes directly).
3. Remove `api/openapi.compat.yaml`, compat generation script, and CI compat-related steps.
4. Update `build/api.mk` to remove "prefer compat when file exists" behavior and keep canonical as default input.
5. Update ADR-0037 status to "compat layer removed".
6. `go build ./... && go test ./...` all pass.

---

## Scope

1. Runtime validator migration to `libopenapi-validator` strict mode.
2. Business validator unification (`validator/v10`).
3. Spec-driven validation tags (`x-oapi-codegen-extra-tags`).
4. CI enforcement and branch protection hardening.
5. **(rev.3)** Frontend toolchain synchronization and CI gates.
6. **(rev.3)** Compat bridge layer governance and exit strategy.

---

## Planned Change Set

### 1. API contract and lint baseline

- `api/openapi.yaml`
  - replace OpenAPI 3.0 `nullable: true` style with OpenAPI 3.1-compatible union types.
  - add/normalize validation tags on request schemas via `x-oapi-codegen-extra-tags`.
- `api/.vacuum.yaml`
  - keep Vacuum ruleset.
  - enforce project naming policy (snake_case) without opening naming drift.
- **(rev.9) `cmd/openapi-compat-gen/main.go`** (NEW — replaces Python script `docs/design/ci/scripts/openapi-compat-generate.sh`)
  - **Rationale**: Python/PyYAML requires CI to manage Python version + pip install + pinned requirements file, introducing a heterogeneous dependency chain. A Go tool can use `go run` or compile to binary, integrating cleanly into the existing Go CI toolchain.
  - **Evaluated and excluded**: `dense-analysis/openapi-spec-converter` (Go) internally depends on `kin-openapi` (the library being replaced) and its tests require Node.js — not compatible with project requirements.
  - **Implementation**: Uses `gopkg.in/yaml.v3` (already an indirect project dependency) with `yaml.Node`-based AST traversal for structured YAML processing:
    - Two-element union `["T", "null"]`/`["null", "T"]` → convert to 3.0 `type: T, nullable: true`
    - 3+ element union → fail-closed with error exit
    - Handles both inline (`["string", "null"]`) and block-style union syntax (`yaml.SequenceNode`)
    - Detects all 3.1-only keywords → fail-closed with error exit
  - **Invocation** (in `build/api.mk`): `go run ./cmd/openapi-compat-gen/main.go api/openapi.yaml api/openapi.compat.yaml`
  - **Tests**: `cmd/openapi-compat-gen/main_test.go` covering inline union, block-style union, 3.1-only keywords, multi-element union (success and fail-closed paths).
  - **Cleanup**: Delete `openapi-compat-generate.sh` and `requirements-openapi-compat.txt` (if created) after Go tool regression tests pass.
- compat generator tests (new, Go)
  - `cmd/openapi-compat-gen/main_test.go` with table-driven tests covering all conversion and fail-closed paths.

- **(rev.3) Frontend type regeneration**
  - Run `cd web && npm run api:generate` to regenerate `web/src/types/api.gen.ts`.
  - Review diff to confirm only nullable representation changes.
  - Run `cd web && npm run typecheck` for zero TypeScript compilation errors.
  - Run `cd web && npm run test:run` for passing frontend unit tests.

### 2. Runtime validation

- `internal/api/middleware/openapi_validator.go`
  - first characterize current middleware behavior with tests (actual behavior baseline, not assumptions).
  - migrate runtime request validation to `libopenapi-validator` strict mode.
  - **(rev.4 CRITICAL — replaces rev.3 guidance)** Do **NOT** use `generated.GetSwagger()` as the spec source for runtime validation. `GetSwagger()` returns the embedded spec from `oapi-codegen`, which is fed from `api/openapi.compat.yaml` (the 3.0 downgrade) when the compat file exists (see `build/api.mk:50–53`, `api/oapi-codegen.yaml:8`). This would mean the runtime validator validates against the 3.0-downgraded spec, contradicting the principle that compat is only for codegen.
  - **(rev.5 BLOCKING FIX — replaces rev.4 `//go:embed ../../` guidance)** Go `embed` forbids `..` in patterns. Use a derived-file approach instead:
    - New package: `internal/api/specembed/spec.go` with `//go:embed openapi.yaml` and `var CanonicalSpec []byte`.
    - Build step: `cp api/openapi.yaml internal/api/specembed/openapi.yaml` (add to `Makefile` / `api-generate` target).
    - Middleware: `doc, _ := libopenapi.NewDocument(specembed.CanonicalSpec)` then `validator.NewValidator(doc, config.WithStrictMode())`.
    - CI: `make api-check` verifies the copy is fresh (add `internal/api/specembed/openapi.yaml` to sync-check paths).
  - **(rev.9 — API selection decision)** Use `libopenapi.NewDocument([]byte)`, NOT `libopenapi.NewDocumentWithConfiguration`. The `//go:embed` spec is a self-contained single-file byte slice with no external `$ref` file or remote references to resolve. `NewDocumentWithConfiguration`'s `AllowFileReferences`/`AllowRemoteReferences`/`BaseURL`/`BasePath` settings are all irrelevant in this scenario. `NewDocument` is the correct minimal-complexity choice. Only upgrade to `NewDocumentWithConfiguration` if the spec architecture changes to introduce external `$ref` files.
  - **(rev.5 — strict mode API clarification, pb33f official docs confirmed)** The correct strict mode invocation is:

    ```go
    import (
        validator "github.com/pb33f/libopenapi-validator"
        "github.com/pb33f/libopenapi-validator/config"
    )
    strictValidator, _ := validator.NewValidator(doc, config.WithStrictMode())
    ```
    Preferred project path is the top-level API above. The `strict` package exists for lower-level/internal use, but is unnecessary for this middleware migration.
  - **(rev.3 best practice)** Use `ValidateHttpRequestResponse(request, response)` for combined request+response validation to reduce duplicate route lookup overhead.
  - **(rev.3 best practice)** Enable response validation only in non-production mode (`gin.Mode() != gin.ReleaseMode`).
  - **(rev.8 corrected) Validator instance lifecycle and concurrency policy**: `libopenapi-validator/strict` docs explicitly require creating a new validator per validation and warn against reusing instances. PR-3 must follow these steps:
    1. **Fixed strategy**: Per-validation instance creation only (no shared instance and no `sync.Pool` reuse).
    2. **Run `-race` tests**: `go test -race ./internal/api/middleware/...` to verify no data races.
    3. **Benchmark assessment**: Measure overhead of per-validation creation versus current middleware baseline for capacity planning.
    4. **Optimization boundary**: If performance is insufficient, optimize validation scope/path (for example keep response validation non-production only), not by sharing validator instances.
    5. **Documentation requirement**: Record test data, `-race` results, and final decision rationale in the PR description.
- `internal/api/middleware/openapi_validator_test.go`
  - add strict coverage for undeclared fields/params and production-safe error behavior.
  - **(rev.4)** must include `go test -race` in CI to guard against concurrent validator instance misuse.
- **(rev.3) Frontend E2E regression**
  - StrictMode will reject undeclared fields sent by the frontend (400). Frontend code must be audited.
  - Run `cd web && npm run test:e2e` to verify all frontend requests are accepted by the backend.

### 3. Business validation

- `internal/api/validator/validator.go` (new)
  - centralize `validator/v10` initialization.
  - **(rev.3 — context7 confirmed)** Initialize with `validator.New(validator.WithRequiredStructEnabled())`.
  - **(rev.3 — context7 confirmed)** Configure `RegisterTagNameFunc` to use `json` tag as field name.
  - standardize field-level error mapping for API responses.
  - **(rev.5 — corrected from rev.3)** Error handling must use a **checked type assertion** (not bare assertion) to prevent panics on unexpected error types:
    ```go
    var ve validator.ValidationErrors
    if errors.As(err, &ve) {
        // map ve to API field errors
    } else if _, ok := err.(*validator.InvalidValidationError); ok {
        // input was not a struct — programming error, log and return 500
    } else {
        // unexpected error type
    }
    ```
- `internal/api/validator/validator_test.go` (new)
  - cover required/format/cross-field scenarios.
- `internal/api/handlers/*.go`
  - gradually replace duplicated request checks with centralized validator path.
- **(rev.9 — defensive configuration check)** Confirm `gin.Recovery()` middleware is registered at the outermost router layer (typically in `internal/api/server.go` or router initialization). `ValidateStruct(nil)` or non-struct input causing `InvalidValidationError` is a programming error handled by the `errors.As` branch, but `gin.Recovery()` provides the final safety net against any unexpected panic propagation through the HTTP layer. This is a **configuration verification only**, no new code needed — check during PR-4 acceptance.
- **(rev.3 — context7 confirmed) Spec-driven validate tags pattern**
  - Declare `validate:"required,min=1,max=256"` etc. via `x-oapi-codegen-extra-tags` at the OpenAPI schema property level.
  - `oapi-codegen` generated Go structs automatically include these tags, achieving the full spec → code → validation pipeline.
  - Example (from context7 oapi-codegen docs):
    ```yaml
    properties:
      id:
        type: number
        x-oapi-codegen-extra-tags:
          validate: "required,min=1,max=256"
    ```
    Generated Go struct:
    ```go
    type Client struct {
        Id float32 `json:"id" validate:"required,min=1,max=256"`
    }
    ```

### 4. CI and governance

- `build/api.mk`
  - ensure compatibility alias: `api-diff -> api-breaking`.
- `.github/workflows/api-contract-validation.yml`
  - enforce lint, sync, and breaking gates.
  - add PR visibility for API changelog output.
  - enable compat bridge as blocking path (`REQUIRE_OPENAPI_COMPAT=1` in CI).
  - **(rev.4 CRITICAL — corrected ordering)** The CI Stage 3 (`generated-code-sync`) job steps must execute in this exact order:
    1. `make api-compat-generate` (generate compat artifact from canonical spec)
    2. `make api-generate` (Go codegen reads compat; TS typegen reads canonical)
    3. `make api-compat` (freshness check — blocking gate)
    4. `git diff --exit-code` (sync check)
  - **Current defect**: The workflow (`api-contract-validation.yml:150–153`) runs `make api-generate` BEFORE `make api-compat-generate`, meaning Go codegen may use a stale or missing compat file. This must be fixed in PR-6.
  - preserve compat freshness checks as blocking gate.
  - optionally add a non-blocking canonical-codegen probe to monitor compat removal readiness.
  - **(rev.3) Frontend type sync gate**:
    ```yaml
    - name: Frontend type sync check
      run: |
        cd web && npm ci && npm run api:generate
        git diff --exit-code web/src/types/api.gen.ts
    ```
- **(rev.4) `docs/design/ci/scripts/api-check.sh`** (must be added to PR-6 changelist)
  - Current `api-check.sh:50` calls `make api-generate` directly without first generating the compat artifact.
  - Must be updated to call `make api-compat-generate` before `make api-generate` when `REQUIRE_OPENAPI_COMPAT=1`.
  - This ensures local `make api-check` and CI `api-check` behave identically.
- GitHub branch protection (repository settings)
  - require status checks on `main`.
  - **(rev.4 — corrected from rev.3)** Required checks must reference **job names** (which map to GitHub status check names), not workflow step names. GitHub branch protection "Require status checks" operates on job-level check names. The specific check names to configure are:
    - `Lint OpenAPI Spec` (from `api-contract-validation.yml` job `lint-spec`, line 36)
    - `Detect Breaking Changes` (from job `breaking-changes`, line 66)
    - `Verify Generated Code Sync` (from job `generated-code-sync`, line 115)
  - These job names must remain stable and unique across all workflows to prevent check name conflicts.
  - **(rev.3 — internet search best practices confirmed)** Additional recommended configuration:
    - Require pull request reviews (≥1 approval)
    - Require status checks to pass before merging
    - Require branches to be up to date before merging
    - Dismiss stale pull request approvals when new commits are pushed
    - Restrict direct push to `main`

### 5. Monitoring and alerting (rev.3 — internet search best practices)

> Source: Internet search "OpenAPI validation architecture best practices" — monitoring validation failures is a key component of production-grade API governance.

- **Validation failure monitoring**: Collect runtime validation failure metrics (endpoint, error type, frequency) and integrate with existing observability infrastructure.
- **Alerting strategy**: Configure alerts for high rates of invalid requests to specific endpoints, which may indicate client implementation defects or spec documentation discrepancies.
- **Contract drift detection** (already covered in CI): `api-check` and `api-compat` gates intercept spec drift at CI stage; no additional runtime detection needed.

---

## Frontend Toolchain Impact Overview (rev.3)

> **Original design note gap**: All 7 PRs only covered backend (Go) toolchain, with zero mention of frontend.
> In practice, the frontend directly consumes the canonical `api/openapi.yaml` to generate TypeScript types.

### Frontend Current State

| Component | Version | Description |
|-----------|---------|-------------|
| `openapi-typescript` | `^7.12.0` | Generates `web/src/types/api.gen.ts` (4642 lines) from `api/openapi.yaml` |
| `openapi-fetch` | `^0.16.0` | Type-safe HTTP client based on generated types |
| Generation script | `npm run api:generate` | `openapi-typescript ../api/openapi.yaml -o src/types/api.gen.ts` |

### Key Conclusions

1. **`openapi-typescript` v7.x fully supports OpenAPI 3.1 natively** (including `type: ["string", "null"]` → `string | null`).
2. **Frontend does not need the compat artifact** — it reads the canonical 3.1 spec directly.
3. However, every modification to `openapi.yaml` requires frontend type regeneration and verification.
4. StrictMode activation (PR-3) will cause the backend to reject undeclared fields sent by the frontend (400), requiring audit.

### Per-PR Frontend Impact

| PR | Impact | Required Action |
|----|--------|-----------------|
| PR-2 | Nullable representation change → TS types may change | Regenerate + typecheck + unit tests |
| PR-3 | StrictMode rejects undeclared fields → frontend requests may get 400 | Audit frontend code + E2E regression tests |
| PR-5 | `x-oapi-codegen-extra-tags` does not affect frontend generation | Regenerate + typecheck (confirm no side effects) |
| PR-6 | CI should include frontend type sync check | Add CI step |

---

## Verification Plan

1. `make api-lint` → `0 errors`
2. `make api-compat-generate` → `make api-generate` → `make api-compat` (correct ordering per rev.4)
3. `make api-check` (must internally respect compat-generate ordering per rev.4)
4. compat generator tests pass (success + fail-closed cases, including block-style YAML unions per rev.4)
5. `go test -race ./internal/api/middleware/...` (includes `-race` flag per rev.4)
6. `go test ./internal/api/validator/...`
7. PR-level breaking-check blocking behavior verified
8. CI blocks on compat missing/stale once PR-6 enables strict compat enforcement
9. **(rev.3)** `cd web && npm run api:generate && npm run typecheck` — zero frontend type errors
10. **(rev.3)** `cd web && npm run test:run` — frontend unit tests pass
11. **(rev.3)** `cd web && npm run test:e2e` — frontend E2E regression passes (no StrictMode false-reject 400s)
12. **(rev.3)** `git diff --exit-code web/src/types/api.gen.ts` — frontend generated artifact matches commit
13. **(rev.4)** Runtime validator spec source verification: confirm embedded spec bytes match canonical `api/openapi.yaml`, NOT the compat artifact
14. **(rev.5)** `git diff --exit-code internal/api/specembed/openapi.yaml` — specembed copy is fresh

---

## Rollout Strategy

Use sequential atomic PRs, one concern per PR, with baseline refresh between each merge.

> **(rev.3 — internet search best practices confirmed)** This aligns with the industry-recommended "design-first + policy-as-code" strategy: each PR is independently rollback-safe and includes complete verification commands, ensuring CI gates are enforceable at every step.

---

## Deferred Until ADR Acceptance

1. Amendment block append in ADR-0029.
2. Normative documentation rewrite beyond minimal references.
3. Final compat sunset policy wording in accepted ADR-0037 (review cadence, timeout trigger, removal gates).

---

## Planned Sunset Criteria (To Be Copied Into ADR-0037 At Acceptance)

1. Re-evaluate compat removal at least once per `oapi-codegen` release bump or every 90 days (whichever comes first).
2. If compat remains after two `oapi-codegen` release bumps, trigger explicit architecture review and decision log update.
3. Remove compat only when all gates pass without `api/openapi.compat.yaml`:
   - `make api-generate`
   - `go build ./...`
   - `go test ./...`
   - CI generated-code-sync
4. **(rev.3)** Post-exit, also verify frontend is unaffected: `cd web && npm run api:generate && npm run typecheck`.
5. **(rev.3)** `canonical codegen probe` scheduled CI job provides automated exit signal detection.

---

## Best Practice Alignment Record (rev.3)

> This section documents alignment with internet search and context7 query results, serving as evidence for best-practice verification of the plan.

### 1. Schema-First Development (✅ Aligned)

- **Best practice**: OpenAPI spec as single source of truth, driving code generation and validation.
- **This plan**: `api/openapi.yaml` is the canonical spec, driving Go codegen (`oapi-codegen`) + TS typegen (`openapi-typescript`) + runtime validation (`libopenapi-validator`) + lint (`vacuum`).

### 2. libopenapi-validator Usage Pattern (✅ Aligned — context7 confirmed)

- **Best practice**: Use `libopenapi.NewDocument(specBytes)` + `validator.NewValidator(doc, config.WithStrictMode())` to create the strict validator; supports independent or combined request/response validation.
- **This plan**: PR-3 follows this pattern, using an independently embedded canonical spec (via `internal/api/specembed/` package) → `libopenapi.NewDocument()` → `validator.NewValidator(doc, config.WithStrictMode())`. The `generated.GetSwagger()` path is explicitly prohibited because it embeds the compat (3.0-downgraded) spec.

### 3. oapi-codegen x-oapi-codegen-extra-tags (✅ Aligned — context7 confirmed)

- **Best practice**: Declare `x-oapi-codegen-extra-tags.validate` at the OpenAPI schema property level; codegen automatically generates struct tags.
- **This plan**: PR-5 follows this pattern, adding validate tags to 37 requestBody schemas in batch.

### 4. validator/v10 Initialization and Error Handling (✅ Aligned — context7 confirmed)

- **Best practice**: `validator.New(validator.WithRequiredStructEnabled())`, `RegisterTagNameFunc` with json tag, checked assertion via `errors.As(err, &ve)` + `InvalidValidationError` handling.
- **This plan**: PR-4 fully follows this pattern.

### 5. CI Enforcement (✅ Aligned — internet search best practices confirmed)

- **Best practice**: All checks as blocking gates, branch protection required checks, contract testing in CI pipeline, spec drift prevention via automated validation.
- **This plan**: PR-1 establishes branch protection; PR-6 completes CI gates (lint + sync + breaking + compat + frontend type sync + contract test).

### 6. Monitoring Validation Failures (🟡 Partially aligned — concrete plan added in rev.7)

- **Best practice**: Collect validation failure metrics, configure alerts for high-frequency invalid requests.
- **This plan**: PR-3 middleware can instrument metrics, but specific implementation is deferred to post-PR-7 as it depends on observability infrastructure selection.
- **(rev.7) Concrete post-PR-7 plan** (see `REMEDIATION-PLAN-BEST-STATE.md` §13 for full details):
  - Metrics defined: `openapi_validation_failures_total` (Counter), `openapi_validation_latency_seconds` (Histogram), `openapi_strict_reject_total` (Counter)
  - Timeline: PR-7 merge + 1 sprint to embed metric collection points (default disabled); enable after observability infra selection
  - Tracking: Dedicated Issue with `enhancement` + `observability` labels, referenced in ADR-0037 Decision section

### 7. Asynchronous Response Validation (🟡 Noted for reference)

- **Best practice**: In production, response validation can be executed asynchronously to avoid impacting request latency.
- **This plan**: Currently uses synchronous response validation in non-production mode, disabled in production. If future performance requirements change, async approach can be evaluated.

---

## Reference Documents

1. `ai-code/rewire-codex/REMEDIATION-PLAN-BEST-STATE.md` (normative: complete PR plan and acceptance criteria)
2. `ai-code/rewire/BEST-PRACTICE-VALIDATION.md` (official API validation correction points)
3. `ai-code/rewire/REMEDIATION-PLAN.md` (precise line numbers + code examples)
4. `ai-code/rewire/CONSOLIDATED-DECISION.md` (three-way consolidated decision)
5. `ai-code/rewire/openapi-validation-strategy.md` (independent research input)
6. `ai-code/rewire-codex/ISSUE85_IMPLEMENTATION_ADR_BESTPRACTICE_ANALYSIS.md` (gap audit original record)

### rev.3 External Validation Sources

| Source | Information Type | Conclusion |
|--------|-----------------|------------|
| Internet search: OpenAPI validation best practices | middleware pattern, monitoring, schema-first | Plan is aligned |
| Internet search: oapi-codegen 3.1 support status | `type: [T, "null"]` not supported, no timeline | Maintain compat strategy |
| Internet search: CI enforcement governance | branch protection, contract testing, spec drift | Plan is aligned |
| context7: pb33f/libopenapi docs | `NewDocument()` + schema iteration + extensions | PR-3 implementation path validated |
| context7: oapi-codegen/oapi-codegen docs | `x-oapi-codegen-extra-tags` + nullable + `x-go-type` | PR-5 implementation path validated |
| context7: go-playground/validator docs | `WithRequiredStructEnabled()` + `ValidationErrors` | PR-4 initialization pattern validated |

### rev.7 Version Verification Record (pkg.go.dev confirmed)

| Component | Plan Version | pkg.go.dev Status | Result |
|-----------|-------------|-------------------|--------|
| `github.com/pb33f/libopenapi` | v0.33.11 | Exists (top of version list) | ✅ Correct |
| `github.com/pb33f/libopenapi-validator` | v0.12.1 | Exists (v0.12.0 and v0.12.1 both published) | ✅ Correct |
| `github.com/go-playground/validator/v10` | v10.30.1 | Exists (released) | ✅ Correct |
| `github.com/oapi-codegen/oapi-codegen/v2` | v2.5.1 | Internet search + context7 confirmed | ✅ Correct |
| `openapi-typescript` | ^7.12.0 | Internet search confirmed v7.x supports 3.1 | ✅ Correct |
| `gopkg.in/yaml.v3` | v3 (already indirect dep) | Project already has it; direct use in PR-2 Go tool | ✅ Directly available |

> **Note**: Initial search engine results showed `libopenapi-validator` latest as v0.11.4 (stale cache), but direct pkg.go.dev version listing page confirmed v0.12.0 and v0.12.1 both exist. All plan version numbers are correct.

### rev.7/rev.8 External Validation Sources

| Source | Information Type | Conclusion |
|--------|-----------------|------------|
| pkg.go.dev: libopenapi versions | v0.33.11 is latest | Version correct |
| pkg.go.dev: libopenapi-validator versions | v0.12.1 published | Version correct |
| pkg.go.dev: validator/v10 versions | v10.30.1  | Version correct |
| [pkg.go.dev: libopenapi-validator/strict docs](https://pkg.go.dev/github.com/pb33f/libopenapi-validator/strict) | per-validation creation required; validator reuse discouraged | PR-3 uses fixed per-validation strategy |
| Internet search: OpenAPI validation monitoring | validation failure metrics is best practice | Added §13 monitoring plan |
| Internet search: Go embed derived-file pattern | `..` forbidden, read-only, concurrent-safe | specembed approach correct |
| context7: libopenapi docs | `NewDocument()` + `BuildV3Model()` + schema iteration | PR-3 path validated |
| context7: oapi-codegen docs | `x-oapi-codegen-extra-tags` usage + 3.1 FAQ | PR-5 path validated |
| context7: validator/v10 docs | `WithRequiredStructEnabled()` + `ValidationErrors` | PR-4 pattern validated |

### rev.9 External Validation Sources

| Source | Information Type | Conclusion |
|--------|-----------------|------------|
| Internet search: libopenapi NewDocument vs NewDocumentWithConfiguration | Self-contained single-file embed needs no external reference configuration | PR-3 keeps `NewDocument([]byte)`; `NewDocumentWithConfiguration` not applicable |
| pkg.go.dev: dense-analysis/openapi-spec-converter | Go OpenAPI version converter; internally uses `kin-openapi`; tests require Node.js | Evaluated and excluded: dependency chain incompatible with project requirements |
| Internet search: gopkg.in/yaml.v3 yaml.Node AST traversal | `yaml.Node` supports structured traversal handling both inline and block-style | Technical basis for PR-2 Go tool implementation |
| Internet search: validator/v10 WithRequiredStructEnabled v11 | Will become default in v11; enabling in v10 is forward-compatible | PR-4 initialization pattern is correct and future-proof |
