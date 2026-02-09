# CI Check Scripts

> This directory contains all CI enforcement scripts referenced by `phases/00-prerequisites.md`.
>
> **📖 Authoritative Reference**: For the complete list of Core ADR Constraints and their enforcement scope, see [CHECKLIST.md §Core ADR Constraints](../CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## Scope Boundary

This document is the authoritative source for **engineering governance and CI gates**.

- `docs/design/interaction-flows/master-flow.md`: expected product interaction outcomes and user-visible behavior.
- `docs/design/ci/README.md` (this file): implementation governance, quality gates, and CI enforcement mechanics.
- `docs/design/phases/*.md`: implementation details that must satisfy both interaction outcomes and CI/ADR constraints.

Do not place CI toolchain policy details in `master-flow.md`; keep those details here and in `docs/design/ci/scripts/`.

---

## Script Summary

| Script | Check Content | Level | Blocks CI |
|--------|---------------|-------|-----------|
| [check_transaction_boundary.go](./scripts/check_transaction_boundary.go) | Service layer must not manage transactions | Required | ✅ Yes |
| [check_k8s_in_transaction.go](./scripts/check_k8s_in_transaction.go) | No K8s API calls inside transactions | Required | ✅ Yes |
| [check_validate_spec.go](./scripts/check_validate_spec.go) | No ValidateSpec calls inside transactions | Required | ✅ Yes |
| [check_forbidden_imports.go](./scripts/check_forbidden_imports.go) | Block fake client, hardcoded paths | Required | ✅ Yes |
| [check_no_gorm_import.go](./scripts/check_no_gorm_import.go) | **Block GORM imports** (migrated to Ent) | Required | ✅ Yes |
| [check_no_outbox_import.go](./scripts/check_no_outbox_import.go) | **Block Outbox imports** (use River Queue, ADR-0006) | Required | ✅ Yes |
| [check_no_redis_import.sh](./scripts/check_no_redis_import.sh) | **Block Redis imports** (removed dependency) | Required | ✅ Yes |
| [check_river_bypass.go](./scripts/check_river_bypass.go) | **Block direct writes bypassing River Queue** (ADR-0006) | Required | ✅ Yes |
| [check_naked_goroutine.go](./scripts/check_naked_goroutine.go) | Block naked `go func()` (ADR-0031) | Required | ✅ Yes |
| [check_ent_codegen.go](./scripts/check_ent_codegen.go) | Ent code generation sync check | Required | ✅ Yes |
| [check_manual_di.sh](./scripts/check_manual_di.sh) | **Strict Manual DI convention** (replaces Wire check) | Required | ✅ Yes |
| [check_sqlc_usage.sh](./scripts/check_sqlc_usage.sh) | **sqlc usage scope** (ADR-0012 whitelist enforcement) | Required | ✅ Yes |
| [check_semaphore_usage.go](./scripts/check_semaphore_usage.go) | Semaphore Acquire/Release pairing (ADR-0031) | Required | ✅ Yes |
| [check_repository_tests.go](./scripts/check_repository_tests.go) | Repository methods must have tests | Required | ✅ Yes |
| [check_dead_tests.go](./scripts/check_dead_tests.go) | Orphan/invalid test detection | Warning | ⚠️ No |
| [check_test_assertions.go](./scripts/check_test_assertions.go) | Tests must have assertions | Required | ✅ Yes |
| [check_markdown_links.go](./scripts/check_markdown_links.go) | Validate local markdown links and anchors | Required | ✅ Yes |
| [check_master_flow_traceability.go](./scripts/check_master_flow_traceability.go) | Enforce master-flow traceability manifest (ADR-0032) | Required | ✅ Yes |
| [check_design_doc_governance.sh](./scripts/check_design_doc_governance.sh) | Enforce design doc path/link governance (ADR-0030) | Required | ✅ Yes |

### Exempt Directories

The following directories are exempt from `check_naked_goroutine.go`:

| Directory | Exemption Reason |
|-----------|------------------|
| `internal/pkg/worker/` | Worker Pool infrastructure itself |
| `internal/governance/river/` | River Worker managed by its internal mechanism |

### Relationship with ADR-0006 Unified Async Model

> **Important**: ADR-0006 mandates all write operations go through River Queue asynchronously, with K8s API calls moved to the Worker layer.
> 
> | Check Script | Applicable Scenario in Async Model |
> |--------------|-------------------------------------|
> | `check_k8s_in_transaction.go` | Ensures K8s calls in UseCase layer are outside DB transactions |
> | `check_validate_spec.go` | Ensures validation logic completes before transaction starts |
> | `check_transaction_boundary.go` | Ensures Service layer does not actively manage transaction boundaries |
> | `check_river_bypass.go` | **Detects direct writes bypassing River Queue in UseCase layer** |
>
> These checks remain valid under the async model as they protect UseCase layer transaction integrity.
>
> **River Bypass Detection (ADR-0006 Enforcement)**:
>
> The `check_river_bypass.go` script scans `internal/usecase/` for direct database write operations to protected entities (VM, ApprovalTicket, Service, System, Cluster). These operations MUST be submitted as River Jobs, with actual writes performed by Workers after transaction commit.
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
# Single script
go run docs/design/ci/scripts/check_transaction_boundary.go
```

Docs governance check:

```bash
bash docs/design/ci/scripts/check_design_doc_governance.sh
```

### CI Integration

See the build job in `.github/workflows/ci.yml`.

---

## Directory Structure

```
ci/
├── README.md                      # This file
└── scripts/
    ├── check_transaction_boundary.go  # Transaction boundary check
    ├── check_k8s_in_transaction.go    # K8s transaction call check
    ├── check_validate_spec.go         # ValidateSpec transaction check
    ├── check_forbidden_imports.go     # Forbidden import check
    ├── check_no_gorm_import.go        # Block GORM imports (migrated to Ent)
    ├── check_no_outbox_import.go      # Block Outbox imports
    ├── check_no_redis_import.sh       # Block Redis imports
    ├── check_naked_goroutine.go       # Naked goroutine check
    ├── check_ent_codegen.go           # Ent code generation sync check
    ├── check_manual_di.sh             # Strict Manual DI convention check (replaces Wire)
    ├── check_semaphore_usage.go       # Semaphore usage check
    ├── check_repository_tests.go      # Repository test coverage check
    ├── check_dead_tests.go            # Dead test detection
    ├── check_test_assertions.go       # Test assertion check
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
| `scripts/openapi-compat-generate.sh` | Generates OpenAPI 3.0-compatible spec (placeholder) | `scripts/` |
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
- **OpenAPI 3.1**: The canonical spec remains 3.1, but Go tooling (`oapi-codegen`, `kin-openapi`) targets 3.0.x. If 3.1-only features are used, generate `api/openapi.compat.yaml` (3.0-compatible) for Go codegen and validation while preserving `api/openapi.yaml` as the source of truth.
- **Frontend types**: `openapi-typescript` can consume OpenAPI 3.1 directly.
- **Contract validation**: `libopenapi-validator` (ADR-0029) validates requests/responses against the OpenAPI spec in middleware with **StrictMode**.
- **Compat enforcement**: `openapi-compat.sh` checks `api/openapi.compat.yaml` is present and up to date; set `REQUIRE_OPENAPI_COMPAT=1` in CI to block merges when compat spec is required.
- **Compat generation**: Use `libopenapi` overlay support (Go-native, replaces oas-patch) to produce a 3.0-compatible spec.
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
10. **Compat generation**: implement `make api-compat-generate` using `libopenapi` overlay support and wire it into CI before enabling `REQUIRE_OPENAPI_COMPAT=1`.
11. **Implement middleware**: Create `internal/api/middleware/openapi_validator.go` with StrictMode and environment-aware error handling.
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
