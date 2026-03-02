# Design Note: ADR-0039 — golangci-lint Custom Analyzer Plugin

> **Status**: Pending ADR-0039 acceptance (Review period: Until 2026-03-04)  
> **ADR**: [ADR-0039](../../adr/ADR-0039-golangci-lint-custom-analyzer.md)  
> **Author**: @jindyzhao  
> **Date**: 2026-03-02

This note captures concrete implementation details for ADR-0039.
It will be merged into `docs/design/ci/README.md` after the ADR is accepted.

---

## Current State Inventory

As of 2026-03-02, `docs/design/ci/scripts/` contains **54 files**:

### `.go` scripts (AST-based, migration candidates)

| Script | ADR enforced | Batch |
|--------|-------------|-------|
| `check_forbidden_imports.go` | ADR-0003 | 1 |
| `check_naked_goroutine.go` | ADR-0031 | 1 |
| `check_river_bypass.go` | ADR-0006 | 1 |
| `check_no_gorm_import.go` | ADR-0003 | 1 |
| `check_no_outbox_import.go` | ADR-0006 | 1 |
| `check_no_runtime_mock.go` | ADR-0034 | 1 |
| `check_handler_explicit_rbac_guards.go` | ADR-0019 | 1 |
| `check_semaphore_usage.go` | ADR-0031 | 1 |
| `check_transaction_boundary.go` | ADR-0010 | 1 |
| `check_river_job_args.go` | ADR-0006 | 1 |
| `check_kubevirt_ssa_compliance.go` | ADR-0011 | 2 |
| `check_k8s_in_transaction.go` | ADR-0012 | 2 |
| `check_auth_provider_plugin_boundary.go` | ADR-0035 | 2 |
| `check_environment_isolation_enforcement.go` | ADR-0034 | 2 |
| `check_no_sqlite_in_tests.go` | ADR-0034 | 2 |
| `check_no_runtime_placeholders.go` | General | 2 |
| `check_duplicate_guard_scope.go` | ADR-0019 | 2 |
| `check_validate_spec.go` | ADR-0021 | 2 |
| `check_module_noop_hooks.go` | ADR-0022 | 2 |
| `check_provider_wiring.go` | ADR-0022 | 2 |
| `check_critical_test_presence.go` | ADR-0034 | 2 |
| `check_dead_tests.go` | General | 2 |
| `check_repository_tests.go` | ADR-0034 | 2 |
| `check_test_assertions.go` | General | 2 |
| `check_vm_create_spec_completeness.go` | ADR-0021 | 2 |
| `check_vm_create_status_progression.go` | ADR-0015 | 2 |
| `check_no_global_platform_admin_gate.go` | ADR-0019 | 2 |
| `check_ent_codegen.go` | ADR-0003 | 2 |
| `check_stage3_admin_catalog_baseline.go` | ADR-0034 | 2 |
| `check_stage4_system_service_baseline.go` | ADR-0034 | 2 |
| `check_stage5c_behavior_tests.go` | ADR-0034 | 2 |
| `check_stage5d_delete_baseline.go` | ADR-0034 | 2 |
| `check_stage5e_batch_baseline.go` | ADR-0034 | 2 |
| `check_stage6_vnc_baseline.go` | ADR-0034 | 2 |
| `check_master_flow_api_alignment.go` | ADR-0032 | 2 |
| `check_master_flow_completion_readiness.go` | ADR-0032 | 2 |
| `check_frontend_no_non_english_literals.go` | ADR-0020 | 2 |
| `check_frontend_no_placeholder_pages.go` | ADR-0020 | 2 |
| `check_frontend_openapi_usage.go` | ADR-0020 | 2 |
| `check_frontend_route_shell_architecture.go` | ADR-0020 | 2 |
| `check_doc_claims_consistency.go` | General | 3 (keep) |
| `check_markdown_links.go` | General | 3 (keep) |
| `check_master_flow_traceability.go` | ADR-0032 | 3 (keep) |
| `check_master_flow_test_matrix.go` | ADR-0032 | 3 (keep) |
| `check_openapi_critical_contract.go` | ADR-0021 | 3 (keep) |
| `check_openapi_critical_fingerprint.go` | ADR-0029 | 3 (keep) |

### `.sh` scripts (permanently retained)

| Script | Purpose |
|--------|---------|
| `api-check.sh` | OpenAPI spec validation |
| `check_changed_code_has_tests.sh` | Test coverage gate |
| `check_design_doc_governance.sh` | Design doc linting |
| `check_live_e2e_no_mock.sh` | E2E isolation enforcement |
| `check_manual_di.sh` | DI pattern enforcement |
| `check_no_redis_import.sh` | Redis import prohibition |
| `check_sqlc_usage.sh` | sqlc scope enforcement |
| `openapi-compat.sh` | OpenAPI compat generation |

---

## New Module Structure

```
tools/shepherd-linter/
├── go.mod
├── go.sum
├── plugin/
│   └── main.go                          # golangci-lint plugin entrypoint
├── analyzers/
│   ├── forbidden_imports/
│   │   ├── analyzer.go
│   │   └── analyzer_test.go
│   ├── naked_goroutine/
│   │   ├── analyzer.go
│   │   └── analyzer_test.go
│   ├── river_bypass/
│   │   ├── analyzer.go
│   │   └── analyzer_test.go
│   ├── kubevirt_ssa_compliance/
│   │   ├── analyzer.go                  # Migrated from check_kubevirt_ssa_compliance.go
│   │   └── analyzer_test.go
│   └── ... (one directory per analyzer)
└── testdata/
    └── src/
        ├── forbidden_imports/           # analysistest fixtures
        ├── naked_goroutine/
        └── ...
```

### `go.mod` version constraint

```bash
# Verify versions before writing go.mod
go version -m $(which golangci-lint)
```

The `go.mod` MUST NOT use `replace` directives that conflict with golangci-lint's own `replace` directives.

---

## Analyzer Migration Pattern

The existing `check_kubevirt_ssa_compliance.go` provides the reference pattern.
Its `ast.Inspect` + `*ast.CompositeLit` logic maps directly:

**Before (`go run` script)**:
```go
ast.Inspect(node, func(n ast.Node) bool {
    cl, ok := n.(*ast.CompositeLit)
    // ... report violation via fmt.Println + os.Exit(1)
    return true
})
```

**After (Analyzer)**:
```go
var Analyzer = &analysis.Analyzer{
    Name: "kubevirt_ssa_compliance",
    Doc:  "Enforces ADR-0011: VM write paths must use SSA, not typed struct construction.",
    Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    for _, file := range pass.Files {
        ast.Inspect(file, func(n ast.Node) bool {
            cl, ok := n.(*ast.CompositeLit)
            if !ok { return true }
            // ... same logic, but report via:
            pass.Reportf(cl.Pos(), "ADR-0011: VM write paths must not construct KubeVirt typed structs: %s", typeName)
            return true
        })
    }
    return nil, nil
}
```

The `pass.Reportf()` call is what enables IDE integration and golangci-lint integration.

---

## `.golangci.yml` Changes (after Batch 1 acceptance)

```yaml
# .golangci.yml additions
version: "2"

linters:
  enable:
    - shepherd-arch  # Custom architecture enforcement
  settings:
    custom:
      shepherd-arch:
        type: module
        path: github.com/kv-shepherd/shepherd/tools/shepherd-linter
        description: Architecture enforcement (ADR compliance, import boundaries, concurrency rules)
```

---

## CI Workflow Changes (after each batch)

For each migrated script in `Makefile` (example):
```makefile
# Before:
check-naked-goroutine:
    go run docs/design/ci/scripts/check_naked_goroutine.go

# After (Batch 1 complete):  
check-naked-goroutine:
    golangci-lint run --enable shepherd-arch --disable-all
    # Note: shepherd-arch includes naked_goroutine.Analyzer
```

After all Batch 1+2 scripts are migrated, the Makefile target becomes:
```makefile
check-arch:
    golangci-lint run --enable shepherd-arch
```

---

## `CONTRIBUTING.md` Updates (after Batch 1 acceptance)

Add to `§CI Checks` table:

```markdown
| `shepherd-arch` (golangci-lint) | Architecture enforcement: import boundaries, ADR compliance, concurrency rules |
```

Add new rule to `§Architecture Decisions`:

```markdown
> ⚠️ **Rule (ADR-0039)**: New CI gates for Go code MUST be written as  
> `go/analysis.Analyzer` entries in `tools/shepherd-linter/`.  
> Do NOT add new `.go` scripts to `docs/design/ci/scripts/`.
```

---

## Pending Changes Block (to be added to `docs/design/ci/README.md` after ADR acceptance)

```markdown
<!-- PENDING: ADR-0039 (under review until 2026-03-04) -->
> ⚠️ **Pending Change**: Architecture CI gates are being migrated to  
> `tools/shepherd-linter` golangci-lint custom Analyzer plugin.  
> See docs/design/notes/ADR-0039-golangci-lint-custom-analyzer.md for migration plan.
```
