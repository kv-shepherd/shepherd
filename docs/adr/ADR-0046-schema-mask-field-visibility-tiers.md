---
status: "proposed"
date: 2026-03-13
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0046: Schema Mask Field Visibility Tiers

> **Review Period**: Until 2026-03-15 (48-hour minimum)<br>
> **Discussion**: [Issue #363](https://github.com/kv-shepherd/shepherd/issues/363)<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md` *(formalizes a third field-visibility tier in SchemaMask)*<br>
> **Relates To**: `ADR-0023-schema-cache-and-api-standards.md`, `ADR-0030-design-documentation-layering-and-fullstack-governance.md`, `ADR-0037-openapi-validation-architecture-and-enforcement-policy.md`

---

## Context and Problem Statement

The project uses a schema-driven admin UX: the backend serves an embedded JSON
Schema and a developer-authored UI mask for a given entity via
`GET /schemas/{entity_type}`. The mask dictates which schema paths are exposed
and how they are presented, while the JSON Schema remains the source of truth
for types and constraints.

Today, the mask supports `quick_fields` (always visible) and `advanced_fields`
(optional tuning). As the schema surface grows, `advanced_fields` becomes a
single overloaded bucket: either it contains too much and overwhelms the UI, or
it hides expert-only paths behind the same toggle used for commonly adjusted
settings.

The question is: how should the platform represent and gate "rare/expert"
configuration paths without turning `advanced_fields` into an unsafe dumping
ground?

## Decision Drivers

* Keep the default admin UX safe and approachable for common operations
* Provide a clear place for expert-only knobs without hiding them entirely
* Keep schema mask semantics stable and reviewable (contract-first)
* Avoid conflating UI visibility tiers with authorization or runtime behavior
* Preserve backward compatibility for existing mask consumers
* Keep documentation and governance aligned with the schema-driven UI approach

## Considered Options

* **Option 1**: Keep only `quick_fields` and `advanced_fields`
* **Option 2**: Add `professional_fields` as a third visibility tier
* **Option 3**: Replace tiers with per-field risk metadata (labels/severity)

## Decision Outcome

**Chosen option**: "Option 2: Add `professional_fields` as a third visibility
tier", because it provides a simple, contract-visible split between common
advanced tuning and rare/expert settings without introducing a new taxonomy or
runtime semantics.

### Consequences

* ✅ Good, because the admin UX can keep defaults clean while still exposing expert knobs behind an explicit opt-in
* ✅ Good, because reviewers gain a clear, auditable place to discuss what qualifies as "professional" vs merely "advanced"
* ✅ Good, because the OpenAPI contract can express the tier explicitly via `SchemaMask.professional_fields`
* 🟡 Neutral, because UI code must implement one additional toggle and presentation convention
* ❌ Bad, because teams may disagree on where a path belongs; mitigation: require a short rationale in mask PR reviews and keep mask diffs small and intentional

### Confirmation

This decision is correctly implemented when all of the following are true:

1. `SchemaMask` contains `quick_fields` (required), `advanced_fields` (optional), and `professional_fields` (optional).
2. `professional_fields` is not shown by default in the admin UI; it requires an explicit "Professional/Expert" opt-in.
3. Visibility tiers do not change server-side authorization, validation, or runtime behavior; they only affect UI exposure.
4. Backend responses keep `mask.quick_fields` present as an array (may be empty, but never nil).
5. Static audits and tests cover `professional_fields` mask paths the same way they cover `advanced_fields` paths.

---

## Pros and Cons of the Options

### Option 1: Keep only `quick_fields` and `advanced_fields`

Continue using a two-tier mask and place all non-default paths into
`advanced_fields`.

* ✅ Good, because it is simpler (no new tier)
* ❌ Bad, because expert-only knobs share the same toggle as commonly adjusted settings, increasing risk and UI clutter over time
* ❌ Bad, because "advanced" becomes a dumping ground with no reviewable boundary

### Option 2: Add `professional_fields` as a third visibility tier

Introduce a third tier for rare or expert-only settings and gate it explicitly
in the UI.

* ✅ Good, because it keeps the "advanced" tier focused on commonly adjusted tuning
* ✅ Good, because it creates an explicit review boundary for high-risk or low-frequency knobs
* ❌ Bad, because it adds a small amount of UI complexity (an extra toggle and copy)

### Option 3: Replace tiers with per-field risk metadata (labels/severity)

Attach risk metadata to each mask field and let the UI sort and filter by
severity.

* ✅ Good, because it can express nuanced risk and multiple categories
* ❌ Bad, because it introduces a new taxonomy and more complex UI logic without clear V1 need
* ❌ Bad, because it increases authoring and review overhead for every mask entry

---

## More Information

### Related Decisions

* `ADR-0018-instance-size-abstraction.md` - schema-driven admin UI and mask-based field exposure
* `ADR-0023-schema-cache-and-api-standards.md` - schema caching and public schema endpoint behavior
* `ADR-0030-design-documentation-layering-and-fullstack-governance.md` - governance for design documentation and frontend/backed layering
* `ADR-0037-openapi-validation-architecture-and-enforcement-policy.md` - OpenAPI contract and validation enforcement policy

### References

* Related Issue: [#363](https://github.com/kv-shepherd/shepherd/issues/363)

### Implementation Notes

While this ADR remains proposed:

* Treat `professional_fields` as a UI-only visibility tier; it must not change runtime behavior or permissions.
* Keep mask PRs small and require explicit review rationale when promoting a path into `professional_fields`.
* If additional tiers or per-field risk metadata are needed later, introduce a separate ADR rather than extending `SchemaMask` ad hoc.

Revisit this ADR if:

* the platform needs more than three visibility tiers, or
* the UI requires richer metadata than a tiered list (for example category tags or risk grades).

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-13 | @jindyzhao | Initial draft |

