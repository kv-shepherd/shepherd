---
status: "accepted"
date: 2026-02-06
deciders: []
consulted: []
informed: []
---

# ADR-0030: Design Documentation Layering and Full-Stack Governance

> **Accepted**: 2026-02-06  
> **Amends**: [ADR-0020 §Frontend Engineering Scope](./ADR-0020-frontend-technology-stack.md), [ADR-0027 §Repository Structure](./ADR-0027-repository-structure-monorepo.md)  
> **Related**: [ADR-0015](./ADR-0015-governance-model-v2.md), [ADR-0021](./ADR-0021-api-contract-first.md), [ADR-0029](./ADR-0029-openapi-toolchain-governance.md)

---

## Context and Problem Statement

Frontend design guidance existed mainly in a single `docs/design/FRONTEND.md` file. As backend design expanded into phased documents, frontend concerns became easy to miss, and cross-doc links drifted. We need a documentation structure and CI governance model that keeps frontend and backend design equally visible and consistently reviewed.

## Decision Drivers

* Prevent frontend/backend documentation drift in a monorepo workflow
* Keep interaction flow (`master-flow.md`) and implementation specs aligned
* Enforce full-stack acceptance through one global checklist standard
* Add CI guardrails for design-document path/link/governance consistency

## Considered Options

* **Option 1**: Keep single-file frontend spec (`docs/design/FRONTEND.md`)
* **Option 2**: Frontend docs under `docs/design/frontend/` with layered subdirectories
* **Option 3**: Split frontend docs into a separate repository

## Decision Outcome

**Chosen option**: "Option 2", because it preserves monorepo atomicity while giving frontend design first-class structure and explicit governance hooks.

### Normative Decisions

1. **Frontend docs location**
   - Move canonical frontend spec to `docs/design/frontend/FRONTEND.md`.
   - `docs/design/FRONTEND.md` is retired.

2. **Frontend documentation layering**
   - `docs/design/frontend/` must contain structured subdirectories by concern:
     - `architecture/`
     - `features/`
     - `contracts/`
     - `testing/`
   - `docs/design/frontend/README.md` is the navigation entry for frontend design docs.

3. **Global acceptance standard**
   - `docs/design/CHECKLIST.md` remains the single global acceptance standard for full-stack delivery.
   - `docs/design/checklist/` phase checklists are execution checklists and must map back to `CHECKLIST.md`.

4. **Master-flow alignment requirement**
   - `docs/design/interaction-flows/master-flow.md` remains the single source of truth for interaction flow.
   - Frontend feature docs must reference master-flow stages for flow semantics and must not redefine contradictory status/transition models.

5. **CI governance for docs**
   - Design-phase CI artifacts must include checks for:
     - Retired path usage (e.g., `docs/design/FRONTEND.md`)
     - Broken canonical cross-links between master-flow, phase docs, frontend docs, and checklists
     - Required governance statements in checklist/readme anchors
   - These checks are mandatory before coding-phase transition.

### Consequences

* ✅ Good, because frontend guidance becomes discoverable by topic instead of one large file
* ✅ Good, because full-stack collaboration is explicitly gated by checklist and CI
* ✅ Good, because master-flow alignment becomes a documented governance rule
* 🟡 Neutral, because contributors must follow new docs paths and update old links
* ❌ Bad, because there is short-term migration overhead for path updates and CI scripts

### Confirmation

* `docs/design/README.md` points to `docs/design/frontend/FRONTEND.md` and `docs/design/frontend/README.md`
* `master-flow.md` and ADR references no longer point to retired `docs/design/FRONTEND.md`
* `docs/design/checklist/README.md` explicitly states global acceptance relationship with `CHECKLIST.md`
* Design CI includes docs governance checks and corresponding phase-0 checklist gates

---

## Pros and Cons of the Options

### Option 1: Keep single-file frontend spec

* ✅ Good, because minimal change effort
* ❌ Bad, because frontend sections become too broad and easy to ignore
* ❌ Bad, because cross-flow conflicts are harder to detect

### Option 2: Layered frontend docs under `docs/design/frontend/`

* ✅ Good, because concerns are separated by architecture/feature/contract/testing
* ✅ Good, because easier to enforce cross-links to master-flow and checklists
* 🟡 Neutral, because requires migration/update of legacy links
* ❌ Bad, because adds initial directory maintenance cost

### Option 3: Separate frontend docs repository

* ✅ Good, because independent frontend doc lifecycle
* ❌ Bad, because violates ADR-0027 monorepo atomic change goal
* ❌ Bad, because API-contract and flow sync would become cross-repo coordination

---

## More Information

### Related Decisions

* [ADR-0020](./ADR-0020-frontend-technology-stack.md) - Frontend stack and baseline spec
* [ADR-0027](./ADR-0027-repository-structure-monorepo.md) - Monorepo requirement (`web/`)
* [ADR-0021](./ADR-0021-api-contract-first.md) - API contract-first workflow
* [ADR-0015](./ADR-0015-governance-model-v2.md) - Batch and governance interaction model

### Implementation Notes

* Migrate existing frontend references to the new path tree in one change set
* Keep frontend feature docs scoped to UX/interaction behavior and reference backend phase docs for server-side internals
* Treat CI docs governance scripts as required artifacts before coding phase

### References

* KubeVirt Shepherd design docs in `docs/design/`

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-06 | @codex | Initial decision |
