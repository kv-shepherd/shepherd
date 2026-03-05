# Design Note: ADR-0039 — golangci-lint Custom Analyzer Plugin

> **Status**: Implementation Complete. ADR accepted on 2026-03-05.  
> **ADR**: [ADR-0039](../../adr/ADR-0039-golangci-lint-custom-analyzer.md)  
> **Author**: @jindyzhao  
> **Date**: 2026-03-02  
> **Last Updated**: 2026-03-05

This note captures concrete implementation details for ADR-0039.
It complements `docs/design/ci/README.md` with implementation-level migration notes.

---

## Implementation Update (2026-03-03)

- Full local lint gate now passes: `make lint` returns `0 issues`.
- Strict custom analyzer proofread passes:
  `./custom-gcl-proofread run ./...` returns `0 issues`.
- Added no-cap lint audit to detect hidden debt beyond cap-limited runs:
  `./custom-gcl run --max-issues-per-linter=0 --max-same-issues=0 ./...`.
- Tuned `gocritic` by disabling `hugeParam` (`.golangci.yml`) to reduce
  low-signal micro-optimization noise while keeping correctness-focused checks strict.

---

## Implementation Findings (Batch 1 Prototype — 2026-03-02)

A proof-of-concept implementation of Batch 1 was executed prior to ADR acceptance to validate feasibility.

### Finding 1: golangci-lint upgraded to v2.10.1 ✔️

**Original concern**: The project had golangci-lint v1.64.8 installed, which does not support the v2 module plugin system.  
**Resolution**: golangci-lint upgraded to **v2.10.1** on 2026-03-02 using the official install script.
`.golangci.yml` migrated to v2 format using `golangci-lint migrate`.

The ADR Confirmation criterion `golangci-lint run --enable shepherd-arch passes on main branch` will be
validated during the Batch 1 PR by running `golangci-lint custom` to build the custom binary.

### Finding 2: Batch 1 completion — 9 of 10 scripts implemented ✔️

All AST-analyzable scripts from Batch 1 have been implemented as `go/analysis` Analyzers.

| Script | Analyzer | Status |
|--------|----------|--------|
| `check_forbidden_imports.go` | `forbiddenimports.Analyzer` | ✅ Done |
| `check_no_gorm_import.go` | merged into `forbiddenimports.Analyzer` | ✅ Done |
| `check_no_outbox_import.go` | merged into `forbiddenimports.Analyzer` | ✅ Done |
| `check_naked_goroutine.go` | `nakedgoroutine.Analyzer` | ✅ Done |
| `check_river_bypass.go` | `riverbypass.Analyzer` | ✅ Done |
| `check_no_runtime_mock.go` | `runtimemock.Analyzer` | ✅ Done |
| `check_semaphore_usage.go` | `semaphoreusage.Analyzer` | ✅ Done |
| `check_transaction_boundary.go` | `txboundary.Analyzer` | ✅ Done |
| `check_river_job_args.go` | `riverjobargs.Analyzer` | ✅ Done |
| `check_handler_explicit_rbac_guards.go` | **retained as `go run`** — file-content string matching; not AST-analyzable | 📌 Batch 3 |

> **Note**: `forbiddenimports.Analyzer` consolidates three original scripts (`check_forbidden_imports.go`,
> `check_no_gorm_import.go`, `check_no_outbox_import.go`) into a single import-path denylist.

All 7 Analyzers build successfully and pass `go test ./...` with zero violations on the main project code.
All tests include both positive (violation) and negative (clean) fixtures per the ADR Confirmation requirement.

### Finding 3: Test coverage ✔️

All 7 Analyzers include `// want` annotated violation fixtures and clean fixtures,
fulfilling the ADR Confirmation requirement: "both positive and negative test cases".

### Finding 4: Strict proofread before cleanup ✔️

Before cleaning legacy CI invocations, a strict verification pass was executed:

```bash
# analyzer unit/integration
cd tools/shepherd-linter && go test ./...

# standalone binary path
make build-shepherd-lint
make lint-arch

# module plugin path
golangci-lint custom --name custom-gcl-proofread
./custom-gcl-proofread run ./...
```

Result:

- `shepherd-arch` path reports **0 issues**
- no analyzer panic in golangci-lint plugin execution
- full lint baseline remains non-zero (expected debt tracked separately in `ai-code/go_lint/`)

This strict proofread is now a precondition for CI cleanup changes.

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

> The structure below reflects the **actual implementation** (canonical per ADR-0039 §1).

```
tools/shepherd-linter/
├── go.mod                                   # module kv-shepherd.io/shepherd-linter
├── go.sum
├── plugin.go                                # register.LinterPlugin + register.Plugin() init
├── cmd/
│   └── shepherd-lint/
│       └── main.go                          # multichecker.Main() for standalone binary
├── analyzer/                                # singular, one package per logical check
│   ├── forbiddenimports/
│   │   ├── analyzer.go
│   │   ├── analyzer_test.go
│   │   └── testdata/src/                   # per-analyzer testdata (analysistest.TestData())
│   ├── nakedgoroutine/
│   │   ├── analyzer.go
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   └── riverbypass/
│       ├── analyzer.go
│       ├── analyzer_test.go
│       └── testdata/src/
└── README.md
```

### `go.mod` version constraint

Run before upgrading `golang.org/x/tools` in `go.mod`:

```bash
go version -m $(which golangci-lint)
```

The `go.mod` MUST NOT use `replace` directives that conflict with golangci-lint's own `replace` directives.

---

## Analyzer Migration Pattern

### Batch 1 Pattern — AST-Only (pure syntax, no type info)

Applicable for: `nakedgoroutine`, `riverbypass`, `forbiddenimports`, `runtimemock`, `semaphoreusage`, `txboundary`, `riverjobargs`

These analyzers work on syntactic structure only (package import paths, AST node types, method names, struct field names). They do not need runtime type information because the entities they check have project-wide unique names enforced by ADR convention.

**Before (`go run` script)**:
```go
ast.Inspect(node, func(n ast.Node) bool {
    cl, ok := n.(*ast.CompositeLit)
    // ... report violation via fmt.Println + os.Exit(1)
    return true
})
```

**After (Batch 1 Analyzer — AST-only)**:
```go
var Analyzer = &analysis.Analyzer{
    Name:     "kubevirt_ssa_compliance",
    Doc:      "Enforces ADR-0011: VM write paths must use SSA, not typed struct construction.",
    Requires: []*analysis.Analyzer{inspect.Analyzer},
    Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
    insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
        cl := n.(*ast.CompositeLit)
        // ... same logic, but report via:
        pass.Reportf(cl.Pos(), "ADR-0011: VM write paths must not construct KubeVirt typed structs: %s", typeName)
    })
    return nil, nil
}
```

### Batch 2 Pattern — Type-Aware (REQUIRED for Batch 2+)

> ⚠️ **Batch 2 MUST use `pass.TypesInfo`** for analyzers that operate on method calls or struct construction
> where the same method/struct name may appear in unrelated packages. AST string matching alone
> (e.g., `sel.Sel.Name == "Create"`) produces false positives on any type with that method name.

Applicable for: `kubevirt_ssa_compliance`, `k8s_in_transaction`, `auth_provider_boundary`

**Batch 2 Analyzer — type-aware (preferred)**:
```go
var Analyzer = &analysis.Analyzer{
    Name:     "kubevirt_ssa_compliance",
    Doc:      "Enforces ADR-0011: VM write paths must use SSA, not typed struct construction.",
    Requires: []*analysis.Analyzer{inspect.Analyzer},
    Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
    insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
        cl := n.(*ast.CompositeLit)

        // ✅ Type-aware: use TypesInfo to verify the actual Go type, not just the name.
        // This eliminates false positives from identically-named structs in other packages.
        tv, ok := pass.TypesInfo.Types[cl]
        if !ok {
            return
        }
        typeName := tv.Type.String()
        // Only flag types from the actual kubevirt API package.
        if strings.HasPrefix(typeName, "kubevirt.io/api/core/v1.") {
            pass.Reportf(cl.Pos(),
                "ADR-0011: direct struct construction of KubeVirt type %q: use SSA unstructured.Unstructured instead",
                typeName)
        }
    })
    return nil, nil
}
```

**Decision Rule for Batch 2**:
- If the analyzer checks a method/function call where name conflicts are possible → **use `pass.TypesInfo`**
- If the analyzer checks an import path → AST `file.Imports` is sufficient (import paths are globally unique)
- If the analyzer checks an AST statement type (e.g., `*ast.GoStmt`) → AST-only is correct

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
        description: >
          Architecture enforcement linters for kubevirt-shepherd.
          Enforces ADR compliance, import boundaries, concurrency rules,
          and coding conventions defined in docs/adr/.
```

---

## CI Workflow Changes (after each batch)

For each migrated script in `Makefile` (example):
```makefile
# Before:
check-naked-goroutine:
    go run <legacy-batch1-script>.go

# After (Batch 1 complete):  
check-naked-goroutine:
    make lint-arch
    # Note: lint-arch runs shepherd-lint (shepherd-arch analyzers)
```

After all Batch 1+2 scripts are migrated, the Makefile target becomes:
```makefile
check-arch:
    ./custom-gcl run ./...
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
<!-- PENDING: ADR-0039 (accepted on 2026-03-05) -->
> ⚠️ **Pending Change**: Architecture CI gates are being migrated to  
> `tools/shepherd-linter` golangci-lint custom Analyzer plugin.  
> See docs/design/notes/ADR-0039-golangci-lint-custom-analyzer.md for migration plan.
```
