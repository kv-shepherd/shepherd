---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-01-31
deciders: []
consulted: []
informed: []
---

# ADR-0027: Monorepo Repository Structure with web/

> **Review Period**: Until 2026-02-02 (48-hour minimum)
> **Discussion**: [Issue #81](https://github.com/kv-shepherd/shepherd/issues/81)
> **Amends**: [ADR-0020 §Repository Structure](./ADR-0020-frontend-technology-stack.md)
> **Related**: [ADR-0021](./ADR-0021-api-contract-first.md) (API Contract-First Design)

---

## Context and Problem Statement

ADR-0020 selected a separate repository for the frontend, but current API
contract-first tooling assumes the frontend lives under `web/` in this
repository (see `docs/design/ci/makefile/api.mk`). We need a repository
structure that keeps API changes, generated Go code, and generated TypeScript
types consistent with minimal operational overhead, especially for a solo
maintainer.

## Decision Drivers

* Contract-first API workflow requires synchronized Go and TypeScript artifacts
* Minimize operational overhead for a solo maintainer (single CI and review flow)
* Atomic changes for API schema, server code, and frontend types
* Alignment with existing build tooling and docs in this repository
* Avoid cross-repo version drift for API contracts

## Considered Options

* **Option 1**: Monorepo with `web/` frontend directory (single repo)
* **Option 2**: Separate frontend repository (`shepherd-ui` / `shepherd-web`)
* **Option 3**: Hybrid (submodule or published package for types)

## Decision Outcome

**Chosen option**: "Monorepo with `web/` frontend directory", because it keeps
the contract-first pipeline atomic and matches existing tooling while reducing
operational cost.

### Consequences

* ✅ Good, because `make api-generate` updates Go and TypeScript artifacts in one commit
* ✅ Good, because CI, code review, and DCO checks remain unified
* 🟡 Neutral, because frontend tooling now lives alongside Go tooling in one repo
* ❌ Bad, because an eventual standalone frontend release would require a later split

### Confirmation

* `docs/design/ci/makefile/api.mk` generates `web/src/types/api.gen.ts`
* CI `api-check` passes with no uncommitted generated changes
* Repository layout includes `web/` and documents it in root README (future)

---

## Pros and Cons of the Options

### Option 1: Monorepo with `web/`

* ✅ Good, because API changes and generated types are committed atomically
* ✅ Good, because release and governance processes stay centralized
* 🟡 Neutral, because frontend dependencies are present in the same repo
* ❌ Bad, because frontend-only contributors may prefer a separate repo

### Option 2: Separate frontend repository

* ✅ Good, because frontend can release independently
* ❌ Bad, because API schema changes require cross-repo coordination and versioning

### Option 3: Hybrid (submodule or published package)

* ✅ Good, because it reduces some duplication
* ❌ Bad, because it still adds coordination overhead and tooling complexity

---

## More Information

### Related Decisions

* [ADR-0020](./ADR-0020-frontend-technology-stack.md) - Frontend stack (amended for repo structure)
* [ADR-0021](./ADR-0021-api-contract-first.md) - Contract-first API workflow

### References

* `docs/design/ci/makefile/api.mk` (TypeScript generation path)

### Implementation Notes

* Use `web/` as the frontend root in this repository
* Revisit if the frontend becomes an independently released product

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-31 | @jindyzhao | Initial draft |
