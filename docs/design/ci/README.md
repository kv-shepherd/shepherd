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

## API Contract-First Enforcement (ADR-0021)

> **Status**: Design Phase - NOT ACTIVE YET
> 
> These files will be moved to their final locations when coding begins.

### Additional Files for API Contract Enforcement

| File | Purpose | Final Location |
|------|---------|----------------|
| `workflows/api-contract.yaml` | GitHub Actions for spec validation | `.github/workflows/` |
| `scripts/api-check.sh` | Verifies generated code is in sync | `scripts/` |
| `spectral/.spectral.yaml` | OpenAPI linting rules | `api/` |
| `api-templates/openapi.yaml` | Starting OpenAPI specification | `api/` |
| `api-templates/oapi-codegen.yaml` | Code generation configuration | `api/` |
| `makefile/api.mk` | Make targets for API workflows | `build/` |

### Activation Checklist

When transitioning from Design Phase to Coding Phase:

1. **Initialize Go module**: `go mod init kv-shepherd.io/shepherd`
2. **Move files** to final locations (see file table above)
3. **Update root Makefile**: `include build/api.mk`
4. **Verify**: `make api-lint && make api-generate`

See [ADR-0021](../../adr/ADR-0021-api-contract-first.md) for full design details.

