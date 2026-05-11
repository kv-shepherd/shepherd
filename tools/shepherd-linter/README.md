# shepherd-linter

Custom architecture enforcement linters for the KubeVirt Shepherd project,
built on the [`golang.org/x/tools/go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) framework.

## Background

The project previously enforced architectural invariants via 54 individual scripts
in `docs/design/ci/scripts/`, each invoked with `go run` in CI — requiring a full
compilation per script, with no IDE feedback during development.

This module replaces Go-based scripts with standard `go/analysis.Analyzer` implementations,
integrated into the project's existing `golangci-lint` workflow as a **module plugin** (v2).

See [ADR-0039](../../docs/adr/ADR-0039-golangci-lint-custom-analyzer.md) for the full decision record.

## Directory Structure

```
tools/shepherd-linter/
├── go.mod                              # module kv-shepherd.io/shepherd-linter
├── plugin.go                          # golangci-lint v2 module plugin (register.LinterPlugin)
├── cmd/
│   └── shepherd-lint/
│       └── main.go                    # standalone multichecker binary
├── analyzer/                          # one package per logical check
│   ├── nakedgoroutine/
│   │   ├── analyzer.go                # Enforces ADR-0031: no naked go statements
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   ├── forbiddenimports/
│   │   ├── analyzer.go                # Detects forbidden imports and hardcoded paths
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   ├── openapirbaccontract/
│   │   ├── analyzer.go                # Enforces explicit OpenAPI x-rbac semantics
│   │   └── analyzer_test.go
│   ├── riverbypass/
│   │   ├── analyzer.go                # Enforces ADR-0006: UseCase writes must use River Queue
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   ├── runtimemock/
│   │   ├── analyzer.go                # Detects runtime wiring of MockProvider
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   ├── semaphoreusage/
│   │   ├── analyzer.go                # Enforces Acquire/defer Release pairing
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   ├── txboundary/
│   │   ├── analyzer.go                # Forbids Tx/Commit/Rollback in service layer
│   │   ├── analyzer_test.go
│   │   └── testdata/src/
│   └── riverjobargs/
│       ├── analyzer.go                # Enforces ADR-0009 Claim Check pattern
│       ├── analyzer_test.go
│       └── testdata/src/
└── README.md
```

## Migration Status

| Batch | Priority | Scripts | Status |
|-------|----------|---------|--------|
| Batch 1 | P0 — pure AST, no external deps | **12 scripts → 9 analyzers** | ✅ Complete |
| Batch 2 | P1 — provider-layer + transaction safety (AST) | 3 scripts → 3 Analyzers | ✅ Complete (extended 2026-03-23) |
| Additional | Cross-cutting contract validation | 1 analyzer-only gate | ✅ Active |
| Batch 3 | P2 — doc/manifest consistency checks | Permanently retained as `go run` | 📌 Documented |

### Batch 1 — Implemented Analyzers

| Analyzer | Enforces | Original script |
|----------|----------|-----------------|
| `nakedgoroutine` | ADR-0031 | `check_naked_goroutine.go` |
| `forbiddenimports` | ADR-0003 + ADR-0006 hygiene | `check_forbidden_imports.go` + `check_no_gorm_import.go` + `check_no_outbox_import.go` |
| `manualdi` | Centralized hand-written DI + no Wire/Redis drift | `check_manual_di.sh` |
| `riverbypass` | ADR-0006 | `check_river_bypass.go` |
| `rbacguards` | Explicit fail-closed high-risk RBAC guards | `check_no_global_platform_admin_gate.go` + `check_handler_explicit_rbac_guards.go` |
| `runtimemock` | ADR-0034 | `check_no_runtime_mock.go` |
| `semaphoreusage` | ADR-0031 | `check_semaphore_usage.go` |
| `txboundary` | ADR-0010 | `check_transaction_boundary.go` |
| `riverjobargs` | ADR-0009 | `check_river_job_args.go` |

### Batch 2 — Implemented Analyzers (2026-03-03)

Context7 best practices applied: `pass.Files` for comment scanning (not `os.ReadFile` in `Run()`),
`insp.Preorder` with typed node filters, per-file suppression marker index built once per `Run()`.

| Analyzer | Enforces | Original script |
|----------|----------|-----------------|
| `authproviderlayering` | ADR-0035/ADR-0048/ADR-0049/ADR-0050/ADR-0051: core/edge/provider auth layering | New analyzer-only enforcement |
| `ssacompliance` | ADR-0011: provider write paths must use SSA+Unstructured | `check_kubevirt_ssa_compliance.go` |
| `k8sintransaction` | ADR-0006/ADR-0012: K8s calls inside DB transaction callbacks (advisory) | `check_k8s_in_transaction.go` |

### Additional Analyzer-Only Gates

| Analyzer | Enforces | Original script |
|----------|----------|-----------------|
| `openapirbaccontract` | Explicit `x-rbac` semantics, auth scheme alignment, and `401/403` response coverage on every OpenAPI operation | New analyzer-only enforcement |
| `entquerysafety` | Raw `ent/dialect/sql` / `sqljson` usage is limited to reviewed `*_ent_predicates.go` helpers plus database/test integration | New analyzer-only enforcement |

**`ssacompliance` rules:**
- Forbidden struct literals: `kubevirtv1.VirtualMachine{...}`, `kubevirtv1.VirtualMachineSpec{...}`, etc.
- Forbidden method calls: `.Create()`, `.Update()` on typed K8s clients
- Scope: `internal/provider/` packages, non-test, non-exempt files
- Per-line suppression: `// ssa-compliance:ignore`
- Exempt files: `mapper.go`, `kubecli_adapter.go`, `mock.go`, `client.go`, `capability.go`, `health_checker.go`

**`k8sintransaction` rules:**
- Detects K8s provider methods (`CreateVM`, `DeleteVM`, `UpdateVM`…) inside `WithTx(func(){})` closures
- Scope: `internal/api/handlers/` and `internal/service/` packages, non-test files
- Advisory: reports as diagnostics (manual review required — consistent with original script semantics)


## Usage

### Build the standalone binary

```bash
cd tools/shepherd-linter
go build -o ../../bin/shepherd-lint ./cmd/shepherd-lint/
```

Or via Makefile:

```bash
make build-shepherd-lint   # build only
make shepherd-lint         # build + run on the main project
make lint-arch             # build (if needed) + run on the main project
```

### Run as standalone binary

```bash
./bin/shepherd-lint ./internal/... ./cmd/... ./pkg/...
```

### Run via golangci-lint v2 module plugin

The analyzers are integrated into golangci-lint as a module plugin via the
`register.LinterPlugin` interface. The project's `.custom-gcl.yml` and `.golangci.yml`
configure the plugin.

```bash
golangci-lint custom          # build custom binary with shepherd-arch plugin
./custom-gcl run ./...        # run with all linters including shepherd-arch
```

### Run tests

```bash
cd tools/shepherd-linter
go test ./...
```

## golangci-lint v2 Integration

The module implements the golangci-lint v2 **Module Plugin System** via the
[`plugin-module-register`](https://github.com/golangci/plugin-module-register) library.

**`plugin.go`** (root package) exports the required `register.LinterPlugin` interface:

```go
func init() {
    register.Plugin("shepherd-arch", New)
}

func New(settings any) (register.LinterPlugin, error) {
    return &shepherdArchPlugin{}, nil
}

func (p *shepherdArchPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
    return AllAnalyzers, nil
}

func (p *shepherdArchPlugin) GetLoadMode() string {
    return register.LoadModeSyntax  // Batch 1: AST-only
}
```

**`.custom-gcl.yml`** (project root):
```yaml
version: v2.10.1
plugins:
  - module: 'kv-shepherd.io/shepherd-linter'
    path: ./tools/shepherd-linter
```

**`.golangci.yml`** additions:
```yaml
version: "2"
linters:
  enable:
    - shepherd-arch
  settings:
    custom:
      shepherd-arch:
        type: module
        description: >
          Architecture enforcement linters for kubevirt-shepherd.
          Enforces ADR compliance, import boundaries, concurrency rules,
          and coding conventions defined in docs/adr/.
```
