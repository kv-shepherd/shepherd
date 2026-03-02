---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-03-02
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0039: Architecture Enforcement via golangci-lint Custom Analyzer Plugin

> **Review Period**: Until 2026-03-04 (48-hour minimum)<br>
> **Discussion**: [Issue #TBD](https://github.com/kv-shepherd/shepherd/issues/)<br>
> **Related**: `CONTRIBUTING.md`, `docs/design/ci/README.md`, all `check_*.go` scripts

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
├── go.mod               # Separate go.mod; versions MUST match golangci-lint v2
├── plugin/
│   └── main.go          # Required: package main, func New(conf any) ([]*analysis.Analyzer, error)
└── analyzers/
    ├── forbidden_imports/
    │   ├── analyzer.go
    │   └── analyzer_test.go
    ├── naked_goroutine/
    │   ├── analyzer.go
    │   └── analyzer_test.go
    ├── river_bypass/
    │   ├── analyzer.go
    │   └── analyzer_test.go
    └── ...               # One package per logical check
```

> ⚠️ **Critical constraint**: All Go module dependencies in `tools/shepherd-linter/go.mod` that overlap with `golangci-lint`'s own dependencies **MUST use the exact same versions** as the `golangci-lint` binary in use. Run `go version -m golangci-lint` to verify.

#### 2. Plugin entrypoint

```go
// tools/shepherd-linter/plugin/main.go
package main

import "golang.org/x/tools/go/analysis"

// New is the required golangci-lint module plugin entrypoint.
func New(conf any) ([]*analysis.Analyzer, error) {
    return []*analysis.Analyzer{
        forbidden_imports.Analyzer,
        naked_goroutine.Analyzer,
        river_bypass.Analyzer,
        kubevirt_ssa_compliance.Analyzer,
        k8s_in_transaction.Analyzer,
        // ... all migrated analyzers
    }, nil
}
```

#### 3. golangci-lint configuration

```yaml
# .golangci.yml
version: "2"

linters:
  settings:
    custom:
      shepherd-arch:
        type: module
        path: github.com/kv-shepherd/shepherd/tools/shepherd-linter
        description: >
          Architecture enforcement linters for kubevirt-shepherd.
          Enforces ADR compliance, import boundaries, concurrency rules,
          and coding conventions defined in docs/adr/.
```

#### 4. Migration strategy (three batches)

Migration proceeds in three prioritized batches to reduce risk and allow incremental validation:

**Batch 1 — P0 (no external dependencies)**

Scripts with pure AST analysis and no file system or external data dependencies:

| Original script | Target analyzer |
|-----------------|-----------------|
| `check_forbidden_imports.go` | `forbidden_imports.Analyzer` |
| `check_naked_goroutine.go` | `naked_goroutine.Analyzer` |
| `check_river_bypass.go` | `river_bypass.Analyzer` |
| `check_no_gorm_import.go` | `no_gorm_import.Analyzer` |
| `check_no_outbox_import.go` | `no_outbox_import.Analyzer` |
| `check_no_runtime_mock.go` | `no_runtime_mock.Analyzer` |
| `check_handler_explicit_rbac_guards.go` | `handler_rbac_guards.Analyzer` |
| `check_semaphore_usage.go` | `semaphore_usage.Analyzer` |
| `check_transaction_boundary.go` | `transaction_boundary.Analyzer` |
| `check_river_job_args.go` | `river_job_args.Analyzer` |

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

Test fixtures are placed in `tools/shepherd-linter/testdata/src/<analyzer-name>/`.

### Consequences

* ✅ Good, because golangci-lint runs all Analyzers in parallel with caching; CI time for architecture checks reduces from `N × go-compile-time` to approximately `1 × golangci-lint-time`.
* ✅ Good, because IDE plugins (GoLand, VSCode with `gopls`) surface Analyzer diagnostics in real-time as the developer codes, eliminating the "wait for CI" feedback loop.
* ✅ Good, because existing AST logic in `check_kubevirt_ssa_compliance.go` and others uses `ast.Inspect` / `*ast.CompositeLit` patterns that map directly to `go/analysis.Pass.ResultOf` and `analysis.Analyzer.Run`, minimizing rewrite effort.
* ✅ Good, because each Analyzer is independently testable with `analysistest`, improving confidence in constraint enforcement.
* 🟡 Neutral, because the `tools/shepherd-linter/go.mod` version pinning constraint requires careful maintenance when upgrading `golangci-lint`.
* ❌ Bad, because Batch 3 scripts (document consistency checks) cannot be migrated and remain as `go run`, creating a two-tier CI system for architectural checks (mitigated by clear categorization and documented rationale).

### Confirmation

* PR merging Batch 1 must demonstrate: `golangci-lint run --enable shepherd-arch` passes on main branch; IDE shows diagnostic on a deliberately injected violation.
* All migrated Analyzers have `analysistest`-based unit tests with both positive and negative test cases.
* `check_no_new_run_scripts.sh` CI gate is added and passing before Batch 1 PR merges.
* `CONTRIBUTING.md` §CI Checks section is updated to reference `shepherd-arch` as the canonical architecture enforcement linter.
* Existing `go run` invocations for migrated scripts are **removed from `Makefile` and CI workflows** after each batch is validated.

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
* ✅ Good, because IDE integration via LSP/gopls surfaces violations in real-time.
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

* **Step 1**: Create `tools/shepherd-linter/` module skeleton with `plugin/main.go` and `go.mod`.
* **Step 2**: Migrate Batch 1 scripts. Validate with `analysistest`. Remove from `Makefile`/CI.
* **Step 3**: Add `check_no_new_run_scripts.sh` gate. Update `CONTRIBUTING.md`.
* **Step 4**: Migrate Batch 2 scripts. Validate with `analysistest`. Remove from `Makefile`/CI.
* **Step 5**: Categorize Batch 3 scripts as permanently retained; document rationale in `docs/design/ci/README.md`.
* **Revisit trigger**: If golangci-lint introduces breaking changes to its module plugin API, evaluate migration path at that time.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-02 | @jindyzhao | Initial draft |
