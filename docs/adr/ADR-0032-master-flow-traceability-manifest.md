---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-08
deciders: []
consulted: []
informed: []
---

# ADR-0032: Master-Flow Traceability Manifest and Drift Enforcement

> **Review Period**: Until 2026-02-10 (48-hour minimum)  
> **Discussion**: [Issue #152](https://github.com/kv-shepherd/shepherd/issues/152)  
> **Related**: [ADR-0030](./ADR-0030-design-documentation-layering-and-fullstack-governance.md)

---

## Context and Problem Statement

`docs/design/interaction-flows/master-flow.md` is the canonical source of truth for product interaction behavior.

However, interaction documentation alone does not ensure:

* every master-flow stage is mapped to an implementation phase contract (`docs/design/phases/`)
* every stage has an explicit verification surface (CI gate and/or checklist)
* changes to master-flow and implementation docs remain synchronized over time

We need a machine-readable manifest that provides strict, auditable traceability from
master-flow stages to the engineering delivery system (phases, checklists, examples, CI gates, and ADRs),
and we need CI to enforce it to prevent drift.

## Decision Drivers

* Keep `master-flow.md` as the human-readable canonical truth.
* Add a machine-readable traceability index without duplicating content.
* Make drift detection blocking in CI to enforce continuous maintenance.
* Minimize additional tooling and dependencies for the design-phase repo state.

## Considered Options

* **Option 1**: No manifest, rely on ad-hoc links and code review.
* **Option 2**: Markdown traceability table maintained by humans.
* **Option 3**: Machine-readable manifest + CI validation (blocking).

## Decision Outcome

**Chosen option**: "Option 3", because it makes traceability enforceable and keeps the index low-maintenance.

### Normative Decisions

1. **Traceability manifest**
   - Add `docs/design/traceability/master-flow.json` as the single machine-readable manifest for master-flow traceability.
   - The manifest MUST NOT restate flows, states, or implementation details. It stores links only.

2. **Minimum coverage requirements (CI-blocking)**
   - Every master-flow stage identifier MUST exist in the manifest.
   - Each stage entry MUST link to at least one Phase document (`docs/design/phases/`), proving an implementation contract exists.
   - Each stage entry MUST link to at least one verification surface:
     - a Phase checklist (`docs/design/checklist/`), and/or
     - a CI gate (`docs/design/ci/`).

3. **Drift enforcement**
   - A CI script validates:
     - stage coverage completeness (no missing stage IDs),
     - no unknown stage IDs (removed stages must be removed from the manifest),
     - referenced files exist,
     - referenced Markdown anchors exist.
   - A CI policy additionally enforces change linkage:
     - if master-flow/phases/checklists/examples/ADRs change, the manifest must be updated in the same PR.

## Consequences

* ✅ Good, because traceability gaps become explicit and block merges.
* ✅ Good, because the manifest is stable (IDs + links) and avoids duplicating content.
* ✅ Good, because removing or renaming anchors becomes a controlled change (manifest breaks loudly).
* ❌ Bad, because there is a maintenance burden to update the manifest for any affected changes (mitigated by keeping it link-only and CI-supported).

## Confirmation

* `docs/design/ci/scripts/check_master_flow_traceability.go` blocks on missing/invalid mappings.
* `docs/design/ci/scripts/check_design_doc_governance.sh` runs the traceability check as a required gate.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-08 | @jindyzhao | Initial draft |

