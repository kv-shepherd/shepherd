# CI Check Scripts

> This directory contains all CI enforcement scripts referenced by `phases/00-prerequisites.md`.
>
> **📖 Authoritative Reference**: For the complete list of Core ADR Constraints and their enforcement scope, see [CHECKLIST.md §Core ADR Constraints](../CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## Scope Boundary

This document is the authoritative source for **engineering governance and CI gates**.

- `docs/design/interaction-flows/master-flow.md`: expected product interaction outcomes and user-visible behavior.
- `docs/design/ci/README.md` (this file): implementation governance, quality gates, and CI enforcement mechanics.
- `docs/design/ci/MASTER_FLOW_STRICT_TEST_FLOW.md`: mandatory spec-driven test execution order (fail -> rework).
- `docs/design/ci/GATE_HARDENING_CHECKLIST.md`: gate-first remediation queue and verified drift register.
- `docs/design/phases/*.md`: implementation details that must satisfy both interaction outcomes and CI/ADR constraints.

Do not place CI toolchain policy details in `master-flow.md`; keep those details here and in `docs/design/ci/scripts/`.

---

## Script Summary

| Script | Check Content | Level | Blocks CI |
|--------|---------------|-------|-----------|
| `shepherd-arch` (golangci-lint module plugin) | Batch1/2 architecture gates: forbidden imports, no runtime mock wiring, no naked goroutines, River bypass, semaphore pairing, service tx boundary, River job args claim-check, auth-provider core/edge/provider layering | Required | ✅ Yes |
| `shepherd-arch/k8sintransaction` (Analyzer) | No K8s API calls inside transactions | Required | ✅ Yes |
| [check_validate_spec.go](./scripts/check_validate_spec.go) | No ValidateSpec calls inside transactions | Required | ✅ Yes |
| [check_openapi_critical_contract.go](./scripts/check_openapi_critical_contract.go) | Enforce stage-critical OpenAPI contracts (auth/vm/approval/audit/notification + global BearerAuth) | Required | ✅ Yes |
| [check_openapi_critical_fingerprint.go](./scripts/check_openapi_critical_fingerprint.go) | Lock SHA256 fingerprints for critical OpenAPI nodes (intentional-change only) | Required | ✅ Yes |
| [check_no_runtime_placeholders.go](./scripts/check_no_runtime_placeholders.go) | Block TODO/FIXME/placeholder/stub markers in runtime code | Required | ✅ Yes |
| [check_provider_wiring.go](./scripts/check_provider_wiring.go) | Enforce runtime wiring path uses real `NewKubeVirtProvider()` and VM module rejects `mock` provider | Required | ✅ Yes |
| [check_no_sqlite_in_tests.go](./scripts/check_no_sqlite_in_tests.go) | Block SQLite usage in tests and `go.mod` (PostgreSQL-only policy) | Required | ✅ Yes |
| [check_no_redis_import.sh](./scripts/check_no_redis_import.sh) | **Block Redis imports** (removed dependency) | Required | ✅ Yes |
| [check_vm_create_status_progression.go](./scripts/check_vm_create_status_progression.go) | Enforce Stage 5.C VM status persistence (`CREATING -> RUNNING|FAILED`) | Required | ✅ Yes |
| [check_vm_create_spec_completeness.go](./scripts/check_vm_create_spec_completeness.go) | Enforce Stage 5.C carries `spec_overrides` through Worker→Provider rendering path | Required | ✅ Yes |
| [check_critical_test_presence.go](./scripts/check_critical_test_presence.go) | Require paired `_test.go` coverage for critical runtime paths (worker/provider/usecase/gateway/validator) | Required | ✅ Yes |
| [check_stage5c_behavior_tests.go](./scripts/check_stage5c_behavior_tests.go) | Enforce Stage 5.C critical behavior tests/scenarios (spec_overrides mapping + invalid path rejection) | Required | ✅ Yes |
| [check_stage5d_delete_baseline.go](./scripts/check_stage5d_delete_baseline.go) | Enforce Stage 5.D delete confirmation/cascade baseline across OpenAPI + runtime handlers + backend/frontend tests | Required | ✅ Yes |
| [check_duplicate_guard_scope.go](./scripts/check_duplicate_guard_scope.go) | Enforce duplicate guard uses same-resource scope + returns `existing_ticket_id` | Required | ✅ Yes |
| [check_environment_isolation_enforcement.go](./scripts/check_environment_isolation_enforcement.go) | Enforce namespace/cluster environment matching checks in approval + worker runtime paths | Required | ✅ Yes |
| [check_stage5e_batch_baseline.go](./scripts/check_stage5e_batch_baseline.go) | Enforce Stage 5.E batch canonical endpoints (+ `/vms/batch/power` compatibility) + admin rate-limit override endpoints + handler/idempotency/rate-limit baseline + gateway child-dispatch + parent-status sync fragments; remove stale deferred allowlist entries | Required | ✅ Yes |
| [check_stage6_vnc_baseline.go](./scripts/check_stage6_vnc_baseline.go) | Enforce Stage 6 VNC canonical endpoints + handler/token/gateway baseline + behavior tests + stale deferred allowlist cleanup | Required | ✅ Yes |
| [check_live_e2e_no_mock.sh](./scripts/check_live_e2e_no_mock.sh) | Block network route-mocking patterns in strict live e2e spec (`master-flow-live.spec.ts`) | Required | ✅ Yes |
| [check_no_global_platform_admin_gate.go](./scripts/check_no_global_platform_admin_gate.go) | Block route-level global `platform:admin` middleware and legacy rate-limit admin helper; require handler-level granular permissions | Required | ✅ Yes |
| [check_handler_explicit_rbac_guards.go](./scripts/check_handler_explicit_rbac_guards.go) | Enforce explicit fail-closed RBAC guards for high-risk handlers (`member`, `namespace`, `/templates`, `/instance-sizes`) | Required | ✅ Yes |
| [check_auth_provider_plugin_boundary.go](./scripts/check_auth_provider_plugin_boundary.go) | Enforce auth-provider runtime/frontend/OpenAPI stay plugin-standard (no OIDC/LDAP hardcoded branches, no enterprise-private naming on public auth/directory surfaces, narrow contract packages stay split from root re-export files) | Required | ✅ Yes |
| [check_frontend_openapi_usage.go](./scripts/check_frontend_openapi_usage.go) | Enforce each OpenAPI operation is consumed by frontend or explicitly deferred; guard system delete `confirm_name` flow | Required | ✅ Yes |
| [check_frontend_no_non_english_literals.go](./scripts/check_frontend_no_non_english_literals.go) | Block non-English hardcoded literals in frontend source (except `i18n/locales`) | Required | ✅ Yes |
| [check_frontend_no_placeholder_pages.go](./scripts/check_frontend_no_placeholder_pages.go) | Block placeholder/stub markers in frontend route pages (`app/**/page.tsx`) | Required | ✅ Yes |
| [check_frontend_route_shell_architecture.go](./scripts/check_frontend_route_shell_architecture.go) | Enforce route-shell thresholds for `app/**/page.tsx` (page size + write API call count), with explicit legacy allowlist + lock to prevent allowlist expansion | Required | ✅ Yes |
| [check_changed_code_has_tests.sh](./scripts/check_changed_code_has_tests.sh) | Enforce strict test-first delta: runtime code changes must include corresponding test changes | Required | ✅ Yes |
| [check_no_new_run_scripts.sh](./scripts/check_no_new_run_scripts.sh) | Block adding new Go-based CI scripts in `docs/design/ci/scripts/` (must use `shepherd-linter`) | Required | ✅ Yes |
| [check_no_legacy_batch1_invocations.sh](./scripts/check_no_legacy_batch1_invocations.sh) | Block CI entrypoints from invoking migrated Batch1 legacy scripts | Required | ✅ Yes |
| [check_no_chinese_chars.sh](./scripts/check_no_chinese_chars.sh) | Block Chinese characters in repository content except approved i18n paths (`docs/i18n/zh-CN/design/interaction-flows/master-flow.md`, `web/src/i18n/locales/zh-CN/**`) | Required | ✅ Yes |
| [check_module_noop_hooks.go](./scripts/check_module_noop_hooks.go) | Block silent noop `ContributeServerDeps` / `RegisterWorkers` hooks unless allowlisted | Required | ✅ Yes |
| [check_ent_codegen.go](./scripts/check_ent_codegen.go) | Ent code generation sync check | Required | ✅ Yes |
| [check_manual_di.sh](./scripts/check_manual_di.sh) | **Strict Manual DI convention** (replaces Wire check) | Required | ✅ Yes |
| [check_sqlc_usage.sh](./scripts/check_sqlc_usage.sh) | **sqlc usage scope** (ADR-0012 whitelist enforcement) | Required | ✅ Yes |
| [check_repository_tests.go](./scripts/check_repository_tests.go) | Repository methods must have tests | Required | ✅ Yes |
| [check_dead_tests.go](./scripts/check_dead_tests.go) | Orphan/invalid test detection | Warning | ⚠️ No |
| [check_test_assertions.go](./scripts/check_test_assertions.go) | Tests must have assertions | Required | ✅ Yes |
| [check_doc_claims_consistency.go](./scripts/check_doc_claims_consistency.go) | Block checklist \"done\" claims that lack implementation evidence, and block stale provider-boundary doc claims that collapse approval/notification/runtime auth back into `internal/provider/auth.go` | Required | ✅ Yes |
| [check_master_flow_api_alignment.go](./scripts/check_master_flow_api_alignment.go) | Enforce every master-flow API path is either in OpenAPI or explicit deferred allowlist | Required | ✅ Yes |
| [check_master_flow_test_matrix.go](./scripts/check_master_flow_test_matrix.go) | Enforce required master-flow stages have executable tests or explicit deferred entries; when `live_step_markers` are declared, enforce those ASCII flow markers exist in mapped `*-live.spec.ts` files (ADR-0034 strict profile: full stage set) | Required | ✅ Yes |
| [check_master_flow_completion_readiness.go](./scripts/check_master_flow_completion_readiness.go) | Full-completion claim gate: deferred/exemption allowlists must all be empty | Required (for completion claim) | ✅ Yes |
| [check_markdown_links.go](./scripts/check_markdown_links.go) | Validate local markdown links and anchors | Required | ✅ Yes |
| [check_master_flow_traceability.go](./scripts/check_master_flow_traceability.go) | Enforce master-flow traceability manifest (ADR-0032) | Required | ✅ Yes |
| [check_design_doc_governance.sh](./scripts/check_design_doc_governance.sh) | Enforce design doc path/link governance (ADR-0030) | Required | ✅ Yes |

### Exempt Directories

The following directories are exempt from the `nakedgoroutine` analyzer in `shepherd-arch`:

| Directory | Exemption Reason |
|-----------|------------------|
| `internal/pkg/worker/` | Worker Pool infrastructure itself |
| `internal/governance/river/` | River Worker managed by its internal mechanism |

### Relationship with ADR-0006 Unified Async Model

> **Important**: ADR-0006 mandates all write operations go through River Queue asynchronously, with K8s API calls moved to the Worker layer.
> 
> | Check Script | Applicable Scenario in Async Model |
> |--------------|-------------------------------------|
> | `shepherd-arch/k8sintransaction` | Ensures K8s calls in UseCase layer are outside DB transactions |
> | `check_validate_spec.go` | Ensures validation logic completes before transaction starts |
> | `shepherd-arch/txboundary` | Ensures Service layer does not actively manage transaction boundaries |
> | `shepherd-arch/riverbypass` | **Detects direct writes bypassing River Queue in UseCase layer** |
>
> These checks remain valid under the async model as they protect UseCase layer transaction integrity.
>
> **River Bypass Detection (ADR-0006 Enforcement)**:
>
> The `shepherd-arch/riverbypass` analyzer scans `internal/usecase/` for direct database write operations to protected entities (VM, ApprovalTicket, Service, System, Cluster). These operations MUST be submitted as River Jobs, with actual writes performed by Workers after transaction commit.
>
> | Entity Type | River Required? | Rationale |
> |-------------|-----------------|----------|
> | VM, ApprovalTicket, Service, System, Cluster | ✅ Yes | External system coordination (K8s) |
> | Notification, DomainEvent, AuditLog | ❌ Exempt | Pure DB writes, transactional atomicity needed |
>
> Use `//nolint:river-bypass` comment to skip checks for legitimate exemptions.

---

## Usage

### Local Execution

```bash
# Batch1 architecture checks via golangci-lint module plugin
golangci-lint custom
./custom-gcl run ./...

# Spec-driven stage coverage
go run docs/design/ci/scripts/check_master_flow_test_matrix.go

# PostgreSQL-only test policy
go run docs/design/ci/scripts/check_no_sqlite_in_tests.go

# Strict test-first delta (diff against origin/main)
bash docs/design/ci/scripts/check_changed_code_has_tests.sh

# Enforce English-only content policy (except approved i18n paths)
bash docs/design/ci/scripts/check_no_chinese_chars.sh

# Strict live e2e must not contain page.route/route.fulfill mocks
bash docs/design/ci/scripts/check_live_e2e_no_mock.sh

# Full master-flow completion claim (no deferred/exemption debt)
go run docs/design/ci/scripts/check_master_flow_completion_readiness.go

# Workflow-equivalent local PR validation bundle
make pr-ci

# End-to-end strict chain (requires DATABASE_URL)
make master-flow-strict

# Run strict live e2e only (no mock routes; requires backend env)
bash scripts/run_e2e_live.sh --no-db-wrapper
# run_e2e_live.sh includes preflight gates:
#   - check_master_flow_test_matrix.go (incl. live_step_markers)
#   - check_live_e2e_no_mock.sh
# set E2E_SKIP_PREFLIGHT_GATES=1 only for emergency local debugging.

# Isolated Docker PostgreSQL wrapper (auto start/wait/cleanup)
./scripts/run_with_docker_pg.sh -- make master-flow-strict
make master-flow-strict-docker-pg

# Backend PostgreSQL suites with isolated Docker PostgreSQL
./scripts/run_with_docker_pg.sh
make test-backend-docker-pg

# Completion claim gate
make master-flow-completion
```

Docs governance check:

```bash
bash docs/design/ci/scripts/check_design_doc_governance.sh
```

### CI Integration

See the build job in `.github/workflows/ci.yml`.

Current split (2026-03-03 optimization):

- `ci-checks`: canonical static governance and strict script gates (including frontend typecheck/unit).
- `master-flow-strict`: PostgreSQL behavior suites + optional live e2e (`ENABLE_LIVE_E2E=true`).
- `pr-ci`: top-level local mirror for required GitHub Actions jobs (`Lint`, `Build`,
  `Test`, `CI Checks`, `Frontend Tests`, and API contract validation). This target
  intentionally repeats some frontend checks because GitHub runs them in separate jobs.
  Playwright smoke checks now fail the run if any test is only green after retry.

This split avoids duplicate execution of the same static checks across both jobs while keeping gate coverage unchanged.

---

## Directory Structure

```
ci/
├── README.md                      # This file
├── MASTER_FLOW_STRICT_TEST_FLOW.md # Spec-driven strict execution flow
├── GATE_HARDENING_CHECKLIST.md    # Gate-first remediation queue
├── allowlists/
│   ├── master_flow_api_deferred.txt # Explicit deferred API paths from master-flow
│   ├── master_flow_test_deferred.txt # Explicit deferred required stage test coverage (strict profile expects empty)
│   ├── frontend_openapi_unused.txt   # Explicit backend operations intentionally not wired in frontend yet
│   ├── frontend_route_shell_legacy.txt # Temporary legacy route-shell threshold exceptions
│   ├── test_delta_guard_exempt.txt   # Temporary exemptions for strict changed-code-has-tests gate
│   └── module_noop_hooks.txt      # Explicit allowlist for noop module hook methods
├── locks/
│   ├── frontend-route-shell-legacy.lock # Lockfile for allowed legacy route-shell exception paths
│   └── openapi-critical.lock       # Fingerprint lock for critical OpenAPI nodes
└── scripts/
    ├── check_k8s_in_transaction.go    # K8s transaction call check
    ├── check_validate_spec.go         # ValidateSpec transaction check
    ├── check_openapi_critical_contract.go # Critical OpenAPI node regression check
    ├── check_openapi_critical_fingerprint.go # Critical OpenAPI fingerprint lock check
    ├── check_no_runtime_placeholders.go # Runtime TODO/placeholder marker check
    ├── check_provider_wiring.go       # Runtime provider wiring path check
    ├── check_no_sqlite_in_tests.go    # Block SQLite usage in tests/go.mod
    ├── check_no_redis_import.sh       # Block Redis imports
    ├── check_vm_create_status_progression.go # Stage 5.C VM status persistence check
    ├── check_vm_create_spec_completeness.go # Stage 5.C spec_overrides passthrough check
    ├── check_critical_test_presence.go # Critical runtime path test-presence check
    ├── check_stage5c_behavior_tests.go # Stage 5.C behavior-level test scenario check
    ├── check_stage5d_delete_baseline.go # Stage 5.D delete confirmation/cascade baseline check
    ├── check_duplicate_guard_scope.go # Stage 5.A duplicate guard scope check
    ├── check_environment_isolation_enforcement.go # Environment isolation runtime enforcement check
    ├── check_stage5e_batch_baseline.go # Stage 5.E batch runtime+contract baseline check
    ├── check_stage6_vnc_baseline.go # Stage 6 VNC runtime+contract baseline check
    ├── check_live_e2e_no_mock.sh # Strict live e2e must not use route-mocking APIs
    ├── check_no_global_platform_admin_gate.go # Forbid route-level global platform:admin gate + legacy rate-limit helper
    ├── check_handler_explicit_rbac_guards.go # Enforce explicit fail-closed RBAC guards for key handlers
    ├── check_auth_provider_plugin_boundary.go # Auth-provider plugin boundary + anti-hardcode guard
    ├── check_frontend_openapi_usage.go # Frontend/OpenAPI operation usage sync check
    ├── check_frontend_no_non_english_literals.go # Frontend hardcoded non-English literal check
    ├── check_frontend_no_placeholder_pages.go # Frontend placeholder route-page marker check
    ├── check_frontend_route_shell_architecture.go # Frontend route shell threshold gate
    ├── check_changed_code_has_tests.sh # Runtime code diff must include test diff
    ├── check_no_new_run_scripts.sh # Prevent adding new legacy go-run CI scripts
    ├── check_no_legacy_batch1_invocations.sh # Prevent invoking migrated Batch1 legacy scripts in CI entrypoints
    ├── check_no_chinese_chars.sh # Enforce English-only content (except approved i18n paths)
    ├── check_module_noop_hooks.go     # Noop module hook allowlist enforcement
    ├── check_ent_codegen.go           # Ent code generation sync check
    ├── check_manual_di.sh             # Strict Manual DI convention check (replaces Wire)
    ├── check_repository_tests.go      # Repository test coverage check
    ├── check_dead_tests.go            # Dead test detection
    ├── check_test_assertions.go       # Test assertion check
    ├── check_doc_claims_consistency.go # Doc \"done\" claim vs implementation consistency
    ├── check_master_flow_api_alignment.go # master-flow API vs OpenAPI (+ deferred allowlist)
    ├── check_master_flow_test_matrix.go # master-flow stage test coverage/deferred hygiene
    ├── check_master_flow_completion_readiness.go # full-completion claim requires zero deferred/exemption entries
    ├── check_markdown_links.go        # Markdown local link/anchor integrity check
    └── check_design_doc_governance.sh # Design docs governance checks
```

---

## API Contract-First Enforcement (ADR-0021, ADR-0029)

> **Status**: Design Phase - ACTIVE IN DESIGN DOCS
> 
> These files are the design-phase artifacts that define the contract-first
> pipeline. When coding begins, move them to their final locations and wire
> them into the repo root Makefile and CI.

### Toolchain Selection (ADR-0029)

> **Go-Native Backend Tooling**: ADR-0029 mandates Go-native tools for linting and validation.

| Layer | Tool | Replaces | Notes |
|-------|------|----------|-------|
| **Linting** | `vacuum` | spectral | Go-native, 10x faster, Spectral-rule compatible |
| **Runtime Validation** | `libopenapi-validator` | kin-openapi (validation) | StrictMode, undeclared field detection |
| **Overlay Processing** | `libopenapi` | oas-patch | Go-native, same ecosystem |
| **Code Generation** | `oapi-codegen` | (unchanged) | ADR-0021 decision preserved |
| **TypeScript Types** | `openapi-typescript` | (unchanged) | ADR-0021 decision preserved (Node.js) |

### Additional Files for API Contract Enforcement

| File | Purpose | Final Location |
|------|---------|----------------|
| `workflows/api-contract.yaml` | GitHub Actions for spec validation | `.github/workflows/` |
| `.github/workflows/docs-governance.yaml` | GitHub Actions for design-doc governance checks (active) | `.github/workflows/` |
| `workflows/docs-links-advisory.yaml` | GitHub Actions for advisory dead-link checks (lychee + custom) | `.github/workflows/` |
| `scripts/api-check.sh` | Verifies generated code is in sync | `scripts/` |
| `scripts/openapi-compat.sh` | Enforces OpenAPI compat spec presence/freshness | `scripts/` |
| `cmd/openapi-compat-gen/main.go` | Generates OpenAPI 3.0.3 compat artifact from canonical 3.1 spec (Go-native, fail-closed) | `cmd/openapi-compat-gen/` |
| ~~`spectral/.spectral.yaml`~~ | ~~OpenAPI linting rules~~ | ~~Deprecated by ADR-0029~~ |
| `vacuum/.vacuum.yaml` | **Vacuum ruleset** (ADR-0029) | `api/` |
| `api-templates/openapi.yaml` | Starting OpenAPI specification | `api/` |
| `api-templates/oapi-codegen.yaml` | Code generation configuration | `api/` |
| `api-templates/openapi-overlay-3.0.yaml` | OpenAPI 3.1 → 3.0 overlay (libopenapi) | `api/` (or `build/` tooling) |
| `makefile/api.mk` | Make targets for API workflows | `build/` |

### CI Security Best Practices (ADR-0029)

> **Supply Chain Security**: All CI workflows MUST follow these practices.

| Practice | Requirement | Example |
|----------|-------------|----------|
| **Action Pinning** | Pin to commit SHA, not tags | `actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683` |
| **Runner Pinning** | Use specific runner version | `ubuntu-24.04` (not `ubuntu-latest`) |
| **Minimal Permissions** | Use `permissions:` block | `contents: read`, `pull-requests: read` |
| **Timeout** | Set job timeout | `timeout-minutes: 10` |
| **Dependabot** | Auto-update GitHub Actions | Configure `.github/dependabot.yml` |

### Tooling and Compatibility Notes

- **Linting**: **Vacuum** (ADR-0029) is the linter for OpenAPI specs. Vacuum is fully compatible with Spectral rulesets.
- **Breaking changes**: `oasdiff` is used to detect breaking changes between base and PR specs.
- **OpenAPI 3.1**: The canonical spec remains 3.1. Go codegen currently uses a 3.0-compatible derived artifact (`api/openapi.compat.yaml`) while preserving `api/openapi.yaml` as the source of truth.
- **Frontend types**: `openapi-typescript` can consume OpenAPI 3.1 directly.
- **Contract validation**: `libopenapi-validator` (ADR-0029) validates requests/responses against the OpenAPI spec in middleware with **StrictMode**.
- **Compat enforcement**: `openapi-compat.sh` checks `api/openapi.compat.yaml` is present and up to date; set `REQUIRE_OPENAPI_COMPAT=1` in CI to block merges when compat spec is required.
- **Compat generation**: `make api-compat-generate` uses `cmd/openapi-compat-gen/main.go` (Go-native `yaml.Node` traversal) and fails closed on unsupported 3.1 constructs.
- **Critical fingerprint lock**: `check_openapi_critical_fingerprint.go` compares critical OpenAPI nodes against `docs/design/ci/locks/openapi-critical.lock`; after intentional contract change, refresh with `go run docs/design/ci/scripts/check_openapi_critical_fingerprint.go -write-lock` in the same commit.
- **Version pinning**: tool versions must be read from `docs/design/DEPENDENCIES.md` (do not hardcode in other docs).

### Spectral to Vacuum Migration

> **Key Point**: Vacuum is designed for drop-in compatibility with Spectral rulesets.

Existing `.spectral.yaml` files can be used directly with Vacuum:

```bash
# Before (Spectral)
spectral lint api/openapi.yaml --ruleset .spectral.yaml

# After (Vacuum) - same ruleset file works
vacuum lint api/openapi.yaml --ruleset .spectral.yaml
```

For detailed migration guidance, see: [ADR-0029 Implementation Details §8](../notes/ADR-0029-openapi-toolchain-implementation.md#8-spectral-to-vacuum-migration-guide)

### OpenAPI Validator Middleware (ADR-0029)

Runtime request/response validation using `libopenapi-validator` with StrictMode:

| Mode | `gin.Mode()` | Behavior |
|------|--------------|----------|
| Development | `debug` | Full validation errors returned to client |
| Staging | `test` | Full validation errors (for E2E tests) |
| **Production** | `release` | **Generic error only; details logged server-side** |

For implementation code, see: [ADR-0029 Implementation Details §3](../notes/ADR-0029-openapi-toolchain-implementation.md#3-runtime-validation-with-strictmode)

### Activation Checklist

When transitioning from Design Phase to Coding Phase:

1. **Initialize Go module**: `go mod init kv-shepherd.io/shepherd`
2. **Move files** to final locations (see file table above)
3. **Update root Makefile**: `include build/api.mk`
4. **Create vacuum ruleset**: Move `vacuum/.vacuum.yaml` to `api/.vacuum.yaml`
5. **Install vacuum**: use the version pinned in [DEPENDENCIES.md](../DEPENDENCIES.md) (or use the pinned `pb33f/vacuum-action` commit in CI)
6. **Verify**: `make api-lint && make api-generate`
7. **If needed**: add a spec-compat step (3.1 → 3.0) that writes `api/openapi.compat.yaml` for Go codegen/validation until 3.1 support is available.
8. **CI enforcement**: run `REQUIRE_OPENAPI_COMPAT=1 make api-compat` once 3.1-only features are used.
9. **Block merges**: add `make api-check` (and `REQUIRE_OPENAPI_COMPAT=1 make api-compat` when required) as required CI checks before any coding begins.
10. **Compat generation hardening**: keep fail-closed behavior in `cmd/openapi-compat-gen/main.go`; if unsupported 3.1 constructs are introduced, CI must fail until conversion rules are explicitly extended.
11. **Enforce middleware**: Keep `internal/api/middleware/openapi_validator.go` runtime validator implemented and router-wired (`internal/app/router.go`), with environment-aware error handling.
12. **Enable docs governance workflow**: ensure `.github/workflows/docs-governance.yaml` runs `check_design_doc_governance.sh` as a required PR check before coding.
13. **Verify locally**: `bash docs/design/ci/scripts/check_design_doc_governance.sh`
14. **Move advisory link workflow**: copy `workflows/docs-links-advisory.yaml` to `.github/workflows/docs-links-advisory.yaml`.
15. **Enable advisory link report**: keep as non-blocking PR signal (do not mark as required gate).

See [ADR-0021](../../adr/ADR-0021-api-contract-first.md) and [ADR-0029](../../adr/ADR-0029-openapi-toolchain-governance.md) for full design details.

---

## Design Docs Governance Enforcement (ADR-0030)

This directory includes design-phase CI artifacts to prevent frontend/backend documentation drift.

Checks include:

- legacy path usage (`docs/design/FRONTEND.md`)
- canonical frontend path linkage (`docs/design/frontend/FRONTEND.md`)
- required database docs layer (`docs/design/database/*.md`)
- master-flow reference consistency to frontend docs
- master-flow and interaction-flow reference consistency to database docs
- phase/checklist/examples alignment to master-flow for batch/delete/VNC canonical endpoints and status models
- V1 VNC scope traceability anchored by ADR addendum (`ADR-0015 §18.1`)
- canonical Stage 6 VNC endpoint path consistency (`/api/v1/vms/{vm_id}/vnc`) and no legacy `/vnc/{vm_id}` usage
- VNC token tracking docs remain PostgreSQL/shared-store compatible (no Redis dependency requirement)
- checklist authority statements (`CHECKLIST.md` as global standard)
- markdown local path + heading-anchor integrity (`check_markdown_links.go`, blocking)
- master-flow traceability manifest coverage and anchor validity (`check_master_flow_traceability.go`, blocking)

## Link Health Policy

- Local link integrity is **blocking**: `check_markdown_links.go` runs inside `check_design_doc_governance.sh` and fails CI on broken local paths/anchors.
- Traceability drift enforcement is **blocking**: PR workflows must checkout with full history (`fetch-depth: 0`) so diff-based manifest update checks are reliable.
- External link health is **advisory**: `lychee` runs in `workflows/docs-links-advisory.yaml` as non-blocking due network variability.

## `check_markdown_links.go` Scope and Ignore Policy

### Default Scan Scope

- Running without arguments scans `docs/design` recursively.
- Running without arguments scans `docs/i18n/zh-CN/design` recursively.
- Running without arguments scans `docs/adr` recursively.
- Running with arguments scans only the provided files/directories.
- Directory arguments are walked recursively for `*.md`.
- Explicit argument mode is strict: missing roots or empty markdown selection fails immediately.

### What Is Validated

- Local markdown link path existence.
- Local heading/anchor existence.
- Directory targets must resolve to `README.md` (or fail).
- Anchor matching supports GitHub heading slug anchors.
- Anchor matching supports explicit markdown IDs (`{#id}`).
- Anchor matching supports HTML ID anchors (`<a id="..."></a>`).

### Ignore Strategy

- External links are not validated by this script (`http://`, `https://`, `mailto:`, `tel:`, `data:`, `javascript:`).
- Links and anchors inside fenced code blocks are ignored (` ``` ` / `~~~`).
- Template placeholder policy (to avoid false failures): do not use markdown links with fake targets such as `./ADR-XXXX-xxx.md` or `URL`.
- Use inline code placeholders (for example, ``ADR-XXXX-xxx.md#section-anchor``) or a neutral real URL like `https://example.com`.

### Recommended Commands

```bash
# Full default scan
GOCACHE=/tmp/go-build-cache go run docs/design/ci/scripts/check_markdown_links.go

# Scoped scan (changed docs only)
GOCACHE=/tmp/go-build-cache go run docs/design/ci/scripts/check_markdown_links.go \
  docs/design/ci/README.md \
  docs/design/interaction-flows/master-flow.md
```
