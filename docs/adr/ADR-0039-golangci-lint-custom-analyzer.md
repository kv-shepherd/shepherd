---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-03-02
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0039: Architecture Enforcement via golangci-lint Custom Analyzer Plugin

> **Review Period**: Until 2026-03-04 (48-hour minimum)<br>
> **Accepted On**: 2026-03-05<br>
> **Discussion**: [Issue #293](https://github.com/kv-shepherd/shepherd/issues/293)<br>
> **Related**: `CONTRIBUTING.md`, `docs/design/ci/README.md`, all `check_*.go` scripts

---

## Implementation Update (2026-03-03)

- Full local lint gate now passes: `make lint` reports `0 issues`.
- Strict analyzer proofread passes: `./custom-gcl-proofread run ./...` reports `0 issues`.
- No-cap audit was added to avoid hidden debt behind issue caps:
  `./custom-gcl run --max-issues-per-linter=0 --max-same-issues=0 ./...`.
- `gocritic` rule tuning applied: disabled `hugeParam` in `.golangci.yml` due high-noise/low-signal
  micro-optimization findings on immutable DTO/config paths.

---

## Context and Problem Statement

The project enforces architectural constraints (ADR compliance, import boundaries, concurrency rules, etc.) through a collection of custom Go scripts and shell scripts located in `docs/design/ci/scripts/`. As of 2026-03-02, this directory contains **54 files** (.go and .sh mixed). Each `.go` script is invoked via `go run <script>.go` in CI, which requires a **full Go compilation per script** and produces no feedback to developers during local coding.

This creates two distinct problems:

1. **CI performance**: 54 independent `go run` invocations serialized in CI are a major contributor to total pipeline time. Each compilation is cold (no caching between scripts).
2. **Developer experience gap**: Architectural violations are only discovered after `git push` when CI runs. There is no IDE-level feedback (red squiggly lines, hover explanations) during development. This "CI police" model creates friction and frustration.

**The core question**: How can the existing AST-based architectural constraints be surfaced as real-time developer feedback in IDEs while simultaneously reducing CI pipeline time?

## Decision Drivers

* Developers must receive architectural violation feedback in their IDE during coding, not only after CI runs.
* CI pipeline time for architecture checks must be reduced significantly (target: eliminate per-script compilation overhead).
* Existing AST-based check logic must not be discarded; it represents significant accumulated architectural knowledge.
* The solution must integrate with `golangci-lint`, which is already the project's standard linter runner (referenced in `CONTRIBUTING.md`).
* No new major toolchain dependencies beyond what `golangci-lint` already requires.
* Shell scripts (`.sh`) that check non-Go artifacts must not be forced into this migration.

## Considered Options

* **Option 1**: Keep the current `go run` script model (status quo)
* **Option 2**: Migrate `.go` scripts to a golangci-lint custom Analyzer plugin (`type: module`)
* **Option 3**: Migrate to a standalone binary with a custom CI invocation (not golangci-lint integrated)

## Decision Outcome

**Chosen option**: **"Option 2: golangci-lint custom Analyzer plugin (module type)"**, because it directly integrates with the existing `golangci-lint` toolchain, enables IDE plugin support via the Language Server Protocol, eliminates per-script compilation overhead through golangci-lint's parallel cached execution, and preserves all existing AST check logic with minimal rewrite effort.

### Normative Decisions

#### 1. New repository: `shepherd-linter`

A new Go module **`shepherd-linter`** is created as a package within the monorepo:

```
tools/shepherd-linter/
├── go.mod               # module kv-shepherd.io/shepherd-linter
├── go.sum
├── cmd/
│   └── shepherd-lint/
│       └── main.go      # package main: multichecker.Main() + New() plugin entrypoint
└── analyzer/            # singular; one package per logical check
    ├── nakedgoroutine/
    │   ├── analyzer.go
    │   ├── analyzer_test.go
    │   └── testdata/src/    # per-analyzer testdata/ (required by analysistest)
    ├── forbiddenimports/
    │   ├── analyzer.go
    │   ├── analyzer_test.go
    │   └── testdata/src/
    ├── riverbypass/
    │   ├── analyzer.go
    │   ├── analyzer_test.go
    │   └── testdata/src/
    └── ...               # one package per logical check
```

> **Design rationale for structure choices**:
> - `analyzer/` (singular) — Go convention for package directories; matches `go/analysis` package naming.
> - `cmd/shepherd-lint/main.go` — Go standard layout for executables under `cmd/`; `plugin/` would imply a build plugin artifact.
> - Per-analyzer `testdata/src/` — required by `golang.org/x/tools/go/analysis/analysistest`: `analysistest.TestData()` resolves relative to the test package directory. A global `testdata/` at module root does not work.

> ⚠️ **Critical constraint**: All Go module dependencies in `tools/shepherd-linter/go.mod` that overlap with `golangci-lint`'s own dependencies **MUST use the exact same versions** as the `golangci-lint` binary in use. Run `go version -m $(which golangci-lint)` to verify.

#### 2. Plugin entrypoint

The root package (`tools/shepherd-linter/plugin.go`) implements the `register.LinterPlugin` interface
required by golangci-lint v2 Module Plugin System:

```go
// tools/shepherd-linter/plugin.go
package shepherdlinter

import (
    "github.com/golangci/plugin-module-register/register"
    "golang.org/x/tools/go/analysis"
)

func init() {
    register.Plugin("shepherd-arch", New)
}

// New is the golangci-lint v2 Module Plugin entrypoint.
func New(settings any) (register.LinterPlugin, error) {
    return &shepherdArchPlugin{}, nil
}

type shepherdArchPlugin struct{}

func (p *shepherdArchPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
    return AllAnalyzers, nil
}

func (p *shepherdArchPlugin) GetLoadMode() string {
    // Batch 1: AST-only. Change to register.LoadModeTypesInfo for Batch 2.
    return register.LoadModeSyntax
}
```

The standalone binary (`cmd/shepherd-lint/main.go`) imports the root package and uses `multichecker.Main()`:

#### 3. golangci-lint v2 configuration

The project uses **golangci-lint v2.10.1** (upgraded from v1.64.8 on 2026-03-02).

**`.custom-gcl.yml`** (new file, project root — created at Batch 1 acceptance):

```yaml
# .custom-gcl.yml
version: v2.10.1
plugins:
  # Local module plugin — no remote proxy needed for monorepo
  - module: 'kv-shepherd.io/shepherd-linter'
    path: ./tools/shepherd-linter
```

**`.golangci.yml`** additions (after running `golangci-lint custom`):

```yaml
version: "2"

linters:
  enable:
    - shepherd-arch  # custom architecture enforcement
  settings:
    custom:
      shepherd-arch:
        type: module
        description: >
          Architecture enforcement linters for kubevirt-shepherd.
          Enforces ADR compliance, import boundaries, concurrency rules,
          and coding conventions defined in docs/adr/.
```

The workflow for a developer to enable the plugin:

```bash
# 1. Build the custom golangci-lint binary with shepherd-arch embedded
golangci-lint custom

# 2. Run with all linters including shepherd-arch
./custom-gcl run ./...
```

#### 4. Migration strategy (three batches)

Migration proceeds in three prioritized batches to reduce risk and allow incremental validation:

**Batch 1 — P0 (no external dependencies)**

Scripts with pure AST analysis and no file system or external data dependencies:

| Original script | Target analyzer |
|-----------------|-----------------|
| `check_forbidden_imports.go` | `forbiddenimports.Analyzer` |
| `check_naked_goroutine.go` | `nakedgoroutine.Analyzer` |
| `check_river_bypass.go` | `riverbypass.Analyzer` |
| `check_no_gorm_import.go` | merged into `forbiddenimports.Analyzer` |
| `check_no_outbox_import.go` | merged into `forbiddenimports.Analyzer` |
| `check_no_runtime_mock.go` | `runtimemock.Analyzer` |
| `check_semaphore_usage.go` | `semaphoreusage.Analyzer` |
| `check_transaction_boundary.go` | `txboundary.Analyzer` |
| `check_river_job_args.go` | `riverjobargs.Analyzer` |

> **Note**: `check_handler_explicit_rbac_guards.go` uses file-content string matching
> and is NOT AST-analyzable. It is retained as `go run` (Batch 3).

**Batch 2 — P1 (AST + file path conventions)**

Scripts requiring awareness of directory structure or file naming conventions:

| Original script | Target analyzer |
|-----------------|-----------------|
| `check_kubevirt_ssa_compliance.go` | `kubevirt_ssa_compliance.Analyzer` |
| `check_k8s_in_transaction.go` | `k8s_in_transaction.Analyzer` |
| `check_auth_provider_plugin_boundary.go` | `auth_provider_boundary.Analyzer` |
| `check_environment_isolation_enforcement.go` | `env_isolation.Analyzer` |
| `check_no_sqlite_in_tests.go` | `no_sqlite_tests.Analyzer` |
| `check_no_runtime_placeholders.go` | `no_runtime_placeholders.Analyzer` |
| `check_duplicate_guard_scope.go` | `duplicate_guard_scope.Analyzer` |
| `check_validate_spec.go` | `validate_spec.Analyzer` |
| `check_module_noop_hooks.go` | `module_noop_hooks.Analyzer` |

**Batch 3 — P2 (document/manifest consistency — deferred)**

Scripts that check consistency between Go code and external documents (Markdown, JSON manifests):
These scripts operate on non-Go data and **MUST NOT** be migrated to Analyzer if they require reading external files at analysis time. They remain as `go run` scripts.

| Script | Migration Decision |
|--------|-------------------|
| `check_markdown_links.go` | ❌ Keep as `go run` (reads Markdown files) |
| `check_doc_claims_consistency.go` | ❌ Keep as `go run` (reads design docs) |
| `check_master_flow_traceability.go` | ❌ Keep as `go run` (reads traceability manifest) |
| `check_openapi_critical_contract.go` | ❌ Keep as `go run` (reads OpenAPI spec) |

#### 5. Shell script policy

All `.sh` scripts in `docs/design/ci/scripts/` are **permanently retained as-is**. Shell scripts check non-Go artifacts (SQL, YAML, directory structure) and cannot be expressed as Go Analyzers.

#### 6. New CI gate authoring rule

> From this ADR's acceptance date forward, **all new architectural CI gates that check Go source code MUST be written as `go/analysis.Analyzer` entries in `shepherd-linter`**. New `go run` scripts for Go-code checks are prohibited.

This rule is enforced by adding a CI check (`check_no_new_run_scripts.sh`) that fails if new `.go` scripts are added to `docs/design/ci/scripts/` after this ADR's acceptance date.

#### 7. Analyzer unit test requirement

Every migrated Analyzer **MUST** include unit tests using `golang.org/x/tools/go/analysis/analysistest`:

```go
// analyzers/naked_goroutine/analyzer_test.go
func TestNakedGoroutineAnalyzer(t *testing.T) {
    analysistest.Run(t, testdata.Dir(), naked_goroutine.Analyzer, "naked_goroutine")
}
```

Test fixtures are placed in `<analyzer-package>/testdata/src/<analyzer-name>/` (per-analyzer, co-located).

### Consequences

* ✅ Good, because golangci-lint runs all Analyzers in parallel with caching; CI time for architecture checks reduces from `N × go-compile-time` to approximately `1 × golangci-lint-time`.
* ✅ Good, because IDE integration (GoLand built-in, VSCode via `golangci-lint` extension or `golangci-lint-langserver`) surfaces custom linter diagnostics in real-time, eliminating the "wait for CI" feedback loop.
* ✅ Good, because existing AST logic in `check_kubevirt_ssa_compliance.go` and others uses `ast.Inspect` / `*ast.CompositeLit` patterns that map directly to `go/analysis.Pass.ResultOf` and `analysis.Analyzer.Run`, minimizing rewrite effort.
* ✅ Good, because each Analyzer is independently testable with `analysistest`, improving confidence in constraint enforcement.
* 🟡 Neutral, because the `tools/shepherd-linter/go.mod` version pinning constraint requires careful maintenance when upgrading `golangci-lint`.
* ❌ Bad, because Batch 3 scripts (document consistency checks) cannot be migrated and remain as `go run`, creating a two-tier CI system for architectural checks (mitigated by clear categorization and documented rationale).

### Confirmation

* PR merging Batch 1 must demonstrate: `golangci-lint run --enable shepherd-arch` passes on main branch; IDE shows diagnostic on a deliberately injected violation.
* All migrated Analyzers have `analysistest`-based unit tests with both positive and negative test cases.
* Before removing legacy Batch1 CI invocations, run a strict proofread checklist: `go test ./tools/shepherd-linter/...`, `make lint-arch`, and `./custom-gcl run ./...`.
* `check_no_new_run_scripts.sh` CI gate is added and passing before Batch 1 PR merges.
* `CONTRIBUTING.md` §CI Checks section is updated to reference `shepherd-arch` as the canonical architecture enforcement linter.
* Existing `go run` invocations for migrated scripts are **removed from `Makefile` and CI workflows** after each batch is validated, and duplicate architecture lint execution in multiple jobs should be avoided.

---

## Pros and Cons of the Options

### Option 1: Keep the current `go run` script model (status quo)

* ✅ Good, because zero migration cost.
* ❌ Bad, because CI pipeline time grows linearly with new script additions (54 scripts and counting).
* ❌ Bad, because zero IDE integration; developers only learn of violations post-push.
* ❌ Bad, because inconsistent quality: some scripts use AST properly, others use fragile string scanning.
* ❌ Bad, because maintainability degrades as the number of scripts grows.

### Option 2: golangci-lint custom Analyzer plugin (module type)

A new `shepherd-linter` module in `tools/` exposes all Architecture Analyzers as a single golangci-lint plugin.

* ✅ Good, because single `golangci-lint run` replaces all individual `go run` invocations.
* ✅ Good, because IDE integration via golangci-lint (GoLand, golangci-lint-langserver) surfaces violations in real-time.
* ✅ Good, because existing `go/ast` logic translates directly to `go/analysis` with minimal rewrite.
* ✅ Good, because each Analyzer is independently testable.
* 🟡 Neutral, because version pinning discipline is required for `go.mod`.
* ❌ Bad, because document-consistency scripts (Batch 3) cannot be migrated.

### Option 3: Standalone binary with custom CI invocation

Compile all checks into a single binary (`shepherd-check`) invoked as a single CI step.

* ✅ Good, because eliminates per-script compilation overhead.
* ❌ Bad, because no IDE integration (not a `go/analysis` compatible plugin).
* ❌ Bad, because requires building and distributing a custom binary, adding CI complexity.
* ❌ Bad, because does not leverage golangci-lint's existing caching and parallel execution.

---

## More Information

### Related Decisions

* `CONTRIBUTING.md §CI Checks` — Current list of required CI checks; will be updated upon acceptance.
* `docs/design/ci/README.md` — CI gate documentation; will reference shepherd-linter.
* All ADRs whose constraints are currently enforced by `check_*.go` scripts — no change to their content; enforcement mechanism changes from `go run` to Analyzer.

### References

* [golangci-lint: Module Plugin documentation](https://golangci-lint.run/plugins/module-plugins/)
* [golang.org/x/tools/go/analysis: Analyzer API](https://pkg.go.dev/golang.org/x/tools/go/analysis)
* [analysistest package for Analyzer unit testing](https://pkg.go.dev/golang.org/x/tools/go/analysis/analysistest)
* [golangci-lint example plugin repository](https://github.com/golangci/example-plugin-linter)

### Implementation Notes

* **Step 1**: Create `tools/shepherd-linter/` module skeleton with `plugin.go` (register.LinterPlugin) and `cmd/shepherd-lint/main.go` (multichecker).
* **Step 2**: Migrate Batch 1 scripts. Validate with `analysistest`. Remove from `Makefile`/CI.
* **Step 3**: Add `check_no_new_run_scripts.sh` gate. Update `CONTRIBUTING.md`.
* **Step 4**: Migrate Batch 2 scripts. Upgrade `GetLoadMode()` to `register.LoadModeTypesInfo`. Validate. Remove from CI.
* **Step 5**: Categorize Batch 3 scripts as permanently retained; document rationale in `docs/design/ci/README.md`.
* **Revisit trigger**: If golangci-lint introduces breaking changes to its module plugin API, evaluate migration path at that time.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-02 | @jindyzhao | Initial draft |
| 2026-03-02 | @jindyzhao | Corrected directory structure to reflect Go/analysistest best practices; updated golangci-lint version to v2.10.1; updated `.custom-gcl.yml` to use local path plugin |
| 2026-03-02 | @jindyzhao | Fixed `New()` signature: Go Plugin `[]*analysis.Analyzer` → Module Plugin `register.LinterPlugin`; added `register.Plugin()` init; removed `handler_rbac_guards` from Batch 1 (not AST-analyzable); updated implementation notes |
| 2026-03-05 | @jindyzhao | Status changed to `accepted`; proofread command updated to `./custom-gcl run ./...` |
