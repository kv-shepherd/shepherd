---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-14
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0034: Master-Flow Spec-Driven Test-First Execution Standard

> **Review Period**: Until 2026-02-17 (48-hour minimum)  
> **Discussion**: TBD (Issue link required before acceptance)  
> **Related**: [ADR-0015](./ADR-0015-governance-model-v2.md), [ADR-0021](./ADR-0021-api-contract-first.md), [ADR-0030](./ADR-0030-design-documentation-layering-and-fullstack-governance.md), [ADR-0032](./ADR-0032-master-flow-traceability-manifest.md)

---

## Context and Problem Statement

`docs/design/interaction-flows/master-flow.md` is already the canonical interaction truth, and the repository has strict API/CI governance.

However, for complex cross-layer workflows (RBAC inheritance, async queue writes, approval chains, batch parent-child execution), "code-first then test" still allows spec drift to accumulate before discovery.

We need a mandatory execution model where stage-level tests are treated as first-class spec artifacts and are enforced by CI before feature completion claims.

## Decision Drivers

* Keep `master-flow.md` as single interaction truth.
* Convert stage intent into executable test evidence as early as possible.
* Prevent "implemented but unverified" claims in high-risk lifecycle stages.
* Fit existing CI gate architecture and traceability pattern (ADR-0032).

## Considered Options

* **Option 1**: Continue implementation-first, tests as follow-up.
* **Option 2**: Team convention only ("please write tests first"), no enforcement.
* **Option 3**: Spec-driven test-first with machine-readable stage matrix + CI blocking gate.

## Decision Outcome

**Chosen option**: "Option 3", because only enforceable gates can consistently prevent spec drift in this codebase.

### Normative Decisions

1. **Stage test matrix is mandatory**
   - Add `docs/design/traceability/master-flow-tests.json` as canonical mapping from master-flow stages to executable test artifacts.
   - Matrix entries map `stage id -> test files`, not prose-only descriptions.

2. **Risk-based required stage set**
   - A required stage set is defined in the matrix for critical flows (resource ownership, VM lifecycle, batch, notification, VNC).
   - Required stages MUST have either:
     - at least one executable test artifact, or
     - an explicit deferred entry in allowlist with reason.

3. **Deferred test debt must be explicit**
   - Deferred stages are tracked in `docs/design/ci/allowlists/master_flow_test_deferred.txt`.
   - Deferred entries are temporary and CI-enforced (stale entries must be removed once tests exist).

4. **CI gate is blocking**
   - Add a required CI script that validates:
     - stage IDs exist in `master-flow.md`,
     - matrix references point to existing executable test files,
     - required stages are covered or explicitly deferred,
     - no stale deferred entries remain.

5. **Execution rule for feature work**
   - For changes affecting required stages, PRs MUST update matrix/deferred entries and executable tests in the same change set.
   - "Checklist done" claims for required stages are invalid without matrix-backed test evidence.

### Consequences

* ✅ Good, because stage-level intent becomes executable and reviewable.
* ✅ Good, because hidden drift is surfaced early by CI instead of late integration failures.
* ✅ Good, because the mechanism reuses current gate architecture and avoids new platform dependencies.
* ❌ Bad, because teams must maintain matrix/deferred metadata in addition to tests (mitigated by automation and strict stale-entry checks).

### Confirmation

* `docs/design/ci/scripts/check_master_flow_test_matrix.go` blocks merge when:
  - required stage coverage is missing without deferral,
  - mapped test files are missing/non-executable,
  - deferred entries are stale or invalid.
* CI workflow runs this script as required gate.

---

## Pros and Cons of the Options

### Option 1: Implementation-first

* ✅ Good, because initial coding speed appears high.
* ❌ Bad, because drift is discovered late and fixes are costlier.

### Option 2: Convention-only test-first

* ✅ Good, because no tooling changes are needed.
* ❌ Bad, because compliance depends on memory and review strictness.

### Option 3: Enforced spec-driven test-first

* ✅ Good, because stage coverage is objective and machine-checkable.
* ✅ Good, because documentation, tests, and runtime evolve together.
* ❌ Bad, because introduces additional governance artifacts.

---

## More Information

### Related Decisions

* [ADR-0021](./ADR-0021-api-contract-first.md) - contract-first API baseline
* [ADR-0030](./ADR-0030-design-documentation-layering-and-fullstack-governance.md) - documentation authority boundaries
* [ADR-0032](./ADR-0032-master-flow-traceability-manifest.md) - stage traceability baseline

### References

* Playwright best practices (stable locators, web-first assertions): https://playwright.dev/docs/best-practices
* `docs/design/interaction-flows/master-flow.md`

### Implementation Notes

Detailed rollout and file-level impacts are tracked in:

* `docs/design/notes/ADR-0034-master-flow-spec-driven-test-first.md`

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-14 | @jindyzhao | Initial draft |
| 2026-02-15 | @jindyzhao | Status set to proposed pending public review/issue linkage |
