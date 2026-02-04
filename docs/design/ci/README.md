# CI Check Scripts

> This directory contains all CI enforcement scripts referenced by `phases/00-prerequisites.md`.

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
| [check_naked_goroutine.go](./scripts/check_naked_goroutine.go) | Block naked `go func()` | Required | ✅ Yes |
| [check_ent_codegen.go](./scripts/check_ent_codegen.go) | Ent code generation sync check | Required | ✅ Yes |
| [check_manual_di.sh](./scripts/check_manual_di.sh) | **Strict Manual DI convention** (replaces Wire check) | Required | ✅ Yes |
| [check_sqlc_usage.sh](./scripts/check_sqlc_usage.sh) | **sqlc usage scope** (ADR-0012 whitelist enforcement) | Required | ✅ Yes |
| [check_semaphore_usage.go](./scripts/check_semaphore_usage.go) | Semaphore Acquire/Release pairing | Required | ✅ Yes |
| [check_repository_tests.go](./scripts/check_repository_tests.go) | Repository methods must have tests | Required | ✅ Yes |
| [check_dead_tests.go](./scripts/check_dead_tests.go) | Orphan/invalid test detection | Warning | ⚠️ No |
| [check_test_assertions.go](./scripts/check_test_assertions.go) | Tests must have assertions | Required | ✅ Yes |

### Exempt Directories

The following directories are exempt from `check_naked_goroutine.go`:

| Directory | Exemption Reason |
|-----------|------------------|
| `internal/pkg/worker/` | Worker Pool infrastructure itself |
| `internal/governance/river/` | River Worker managed by its internal mechanism |
| `cmd/` | Application entry files (e.g., main.go startup logic) |

### Relationship with ADR-0006 Unified Async Model

> **Important**: ADR-0006 mandates all write operations go through River Queue asynchronously, with K8s API calls moved to the Worker layer.
> 
> | Check Script | Applicable Scenario in Async Model |
> |--------------|-------------------------------------|
> | `check_k8s_in_transaction.go` | Ensures K8s calls in UseCase layer are outside DB transactions |
> | `check_validate_spec.go` | Ensures validation logic completes before transaction starts |
> | `check_transaction_boundary.go` | Ensures Service layer does not actively manage transaction boundaries |
>
> These checks remain valid under the async model as they protect UseCase layer transaction integrity.

---

## Usage

### Local Execution

```bash
# Single script
go run scripts/ci/check_transaction_boundary.go

# All checks
make ci-checks
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
    └── check_test_assertions.go       # Test assertion check
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
|----------|-------------|---------|
| **Action Pinning** | Pin to commit SHA, not tags | `actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683` |
| **Runner Pinning** | Use specific runner version | `ubuntu-22.04` (not `ubuntu-latest`) |
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
5. **Install vacuum**: `go install github.com/daveshanley/vacuum@v0.14.0` or use `pb33f/vacuum-action@v2` in CI
6. **Verify**: `make api-lint && make api-generate`
7. **If needed**: add a spec-compat step (3.1 → 3.0) that writes `api/openapi.compat.yaml` for Go codegen/validation until 3.1 support is available.
8. **CI enforcement**: run `REQUIRE_OPENAPI_COMPAT=1 make api-compat` once 3.1-only features are used.
9. **Block merges**: add `make api-check` (and `REQUIRE_OPENAPI_COMPAT=1 make api-compat` when required) as required CI checks before any coding begins.
10. **Compat generation**: implement `make api-compat-generate` using `libopenapi` overlay support and wire it into CI before enabling `REQUIRE_OPENAPI_COMPAT=1`.
11. **Implement middleware**: Create `internal/api/middleware/openapi_validator.go` with StrictMode and environment-aware error handling.

See [ADR-0021](../../adr/ADR-0021-api-contract-first.md) and [ADR-0029](../../adr/ADR-0029-openapi-toolchain-governance.md) for full design details.

