---
status: "accepted"
date: 2026-03-11
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0043: Clarify Bootstrap Composition-Root Quality Gate

> **Review Period**: Until 2026-03-13 (48-hour minimum)<br>
> **Discussion**: [Issue #338](https://github.com/kv-shepherd/shepherd/issues/338)<br>
> **Amends**: `ADR-0022-modular-provider-pattern.md` *(clarifies composition-root confirmation criteria)*

---

## Context and Problem Statement

`ADR-0022` established the modular-provider pattern and the requirement that
`internal/app/bootstrap.go` remain a concise composition root. One confirmation
bullet in the accepted ADR used a file-level heuristic: `bootstrap.go` should
not exceed 100 lines.

The current implementation shows why that heuristic is too mechanical.
`Bootstrap()` itself remains short and orchestration-only, while the file can
exceed 100 total lines because of comments or small local helpers. Treating
total file length as a hard gate would encourage low-value churn such as comment
removal or arbitrary file splitting instead of protecting the actual design
intent.

The question is: how should the project evaluate composition-root quality
without gaming comments, helpers, or harmless file structure?

## Decision Drivers

* Preserve the architectural intent of `ADR-0022`
* Keep the composition root reviewable and free of business logic
* Avoid low-value churn caused by mechanical line-count enforcement
* Allow comments and small helper functions when they improve clarity
* Keep ADR-0013 manual DI discipline explicit and auditable

## Considered Options

* **Option 1**: Continue enforcing a hard `<100 file lines` rule on `bootstrap.go`
* **Option 2**: Evaluate `Bootstrap()` by orchestration-only responsibility and concise function size, not total file lines
* **Option 3**: Remove size guidance entirely and rely only on subjective review

## Decision Outcome

**Chosen option**: "Option 2", because it preserves the real design intent of
`ADR-0022` while preventing metric gaming and unnecessary churn.

### Consequences

* ✅ Good, because comments and helper functions no longer count as artificial violations
* ✅ Good, because reviewers focus on whether `Bootstrap()` contains business logic or excessive dependency wiring complexity
* ✅ Good, because the project keeps a concrete composition-root quality standard instead of a vague "keep it clean" expectation
* 🟡 Neutral, because review now depends on function-level judgment rather than a single file-total number
* ❌ Bad, because there is no longer a one-line mechanical check for total file length; mitigation is to keep explicit checklist guidance and manual DI CI checks

### Confirmation

Implementation conforms to this ADR when all of the following are true:

1. `Bootstrap()` remains orchestration-only: initialize infrastructure, compose
   modules, register workers, and return assembled application state.
2. `Bootstrap()` does not contain domain/business rules, persistence logic, or
   request handling behavior.
3. Small helper functions in the same file are allowed when they keep
   orchestration readable.
4. Review and checklist language refers to `Bootstrap()` responsibility and
   function size, not total file lines.
5. ADR-0013 manual DI constraints and constructor-centralization rules remain
   unchanged.

---

## Pros and Cons of the Options

### Option 1: Continue enforcing a hard `<100 file lines` rule on `bootstrap.go`

Treat total physical lines in the file as the quality gate.

* ✅ Good, because it is easy to measure
* ✅ Good, because it creates a visible ceiling
* ❌ Bad, because comments and helper functions trigger false violations
* ❌ Bad, because it encourages low-value rewrites that improve the metric but not the design

### Option 2: Evaluate `Bootstrap()` by orchestration-only responsibility and concise function size

Keep the architectural rule, but apply it to the composition-root function and
its responsibilities rather than the full file.

* ✅ Good, because it aligns with the actual design goal
* ✅ Good, because it allows explanatory comments and local helpers
* ✅ Good, because it keeps the composition root readable without forcing arbitrary splitting
* ❌ Bad, because code review still requires judgment rather than a single numeric threshold

### Option 3: Remove size guidance entirely and rely only on subjective review

Drop any explicit size/shape expectation.

* ✅ Good, because it avoids metric debates
* ❌ Bad, because drift becomes harder to detect early
* ❌ Bad, because reviewers lose a shared expectation for composition-root shape

---

## More Information

### Related Decisions

* `ADR-0013-manual-di.md` - strict manual dependency injection remains unchanged
* `ADR-0022-modular-provider-pattern.md` - original modular-provider pattern decision

### References

* `docs/adr/TEMPLATE.md`
* `ADR-0022-modular-provider-pattern.md`
* `docs/adr/README.md`

### Implementation Notes

While this ADR remains proposed, implementation-facing guidance should live in
`docs/design/notes/ADR-0043-bootstrap-orchestration-quality-gate.md`. Normative
design specs should only absorb this clarification after the ADR is accepted.

Revisit this ADR if:

* `Bootstrap()` starts accumulating domain logic again
* CI gains a reliable function-level complexity gate that makes the manual
  review language obsolete
* the project adopts a different composition-root structure entirely

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-11 | @jindyzhao | Reworked into template-compliant proposed ADR with explicit 48-hour review window |
