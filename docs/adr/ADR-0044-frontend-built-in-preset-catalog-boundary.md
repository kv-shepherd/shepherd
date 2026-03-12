---
status: "proposed"
date: 2026-03-12
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0044: Frontend Built-In Preset Catalog Boundary

> **Review Period**: Until 2026-03-14 (48-hour minimum)<br>
> **Discussion**: [Issue #357](https://github.com/kv-shepherd/shepherd/issues/357)<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md#configuration-storage-strategy-added-2026-01-26`<br>
> **Relates To**: `ADR-0036-template-instancesize-boundary-enforcement.md`, `ADR-0040-catalog-scope-for-template-and-instancesize.md`, `RFC-0021-preset-catalog-marketplace.md`

---

## Context and Problem Statement

The admin catalog UX needs scenario-based presets so operators do not have to
manually understand or fill every low-level KubeVirt-related field. The current
work introduces built-in template and instance-size presets with both curated
and upstream-friendly starter entries.

Existing architecture already establishes two important boundaries:

1. `Template` and `InstanceSize` are persisted business records and remain the
   authoritative catalog objects once created or updated.
2. ADR-0018 states that runtime configuration is primarily stored in
   PostgreSQL, with only limited code-embedded assets such as schemas, masks,
   and built-in definitions living in source control.

Without an explicit decision, built-in preset catalogs are ambiguous:

- Are they authoritative runtime catalog records that must be seeded into the
  backend database?
- Or are they frontend authoring aids that help fill forms but do not exist as
  server-managed resources until an administrator actually saves a Template or
  InstanceSize?

This ADR resolves that V1 boundary.

## Decision Drivers

* Improve admin UX with guided choices instead of raw blank forms
* Preserve PostgreSQL-backed `Template` and `InstanceSize` as authoritative runtime data
* Keep V1 backend intentionally simple and unaware of built-in preset catalogs
* Avoid presenting browser-session import/export as a durable product capability
* Preserve the Template vs InstanceSize separation from ADR-0018 and ADR-0036
* Allow curated conventions without claiming they are universal defaults

## Considered Options

* **Option 1**: Frontend built-in preset catalogs as non-authoritative UI assets
* **Option 2**: Backend-seeded and persisted preset catalogs from V1
* **Option 3**: Frontend-only import/export and community sharing without backend persistence

## Decision Outcome

**Chosen option**: "Option 1: Frontend built-in preset catalogs as
non-authoritative UI assets", because it improves V1 usability without
violating the existing source-of-truth model or introducing fake durability.

### Consequences

* ✅ Good, because built-in preset catalogs can ship as version-controlled frontend assets without new backend schema or APIs
* ✅ Good, because `Template` and `InstanceSize` remain authoritative only after an admin saves them through existing APIs
* ✅ Good, because V1 can ship multiple built-in sources such as `curated` and `official` instead of implying a single opinionated default set
* ✅ Good, because V2 import/export and marketplace work can be deferred to a backend-backed design instead of being simulated in browser state
* 🟡 Neutral, because updating built-in presets requires a frontend release rather than an admin-side runtime edit path
* ❌ Bad, because users cannot durably publish or exchange custom presets in V1; mitigation: track that as backend-backed follow-up work in RFC-0021

### Confirmation

This decision is correctly implemented when all of the following are true:

1. Built-in preset catalogs live in frontend source code under `web/src/presets/`
2. Applying a preset only fills admin form values; it does not create backend records by itself
3. The only persisted objects remain the resulting `Template` and `InstanceSize` rows created through existing catalog APIs
4. V1 exposes only built-in catalog sources such as `curated` and `official`
5. V1 does not expose browser-only import/export or community sharing flows as durable platform features
6. Template and InstanceSize validators continue to enforce ADR-0036 boundaries after preset application

---

## Pros and Cons of the Options

### Option 1: Frontend built-in preset catalogs as non-authoritative UI assets

Treat presets as frontend authoring helpers that fill form values and examples,
but do not exist as backend-managed catalog resources until a save action
creates or updates a real `Template` or `InstanceSize`.

* ✅ Good, because it matches the current V1 "dumb backend" requirement
* ✅ Good, because it keeps authoritative runtime records in PostgreSQL
* ✅ Good, because it supports richer UX guidance such as grouped presets, examples, and linked defaults
* ❌ Bad, because presets are not reusable outside the frontend until a later backend catalog service exists

### Option 2: Backend-seeded and persisted preset catalogs from V1

Store built-in preset catalogs as database records and expose them via backend
APIs from day one.

* ✅ Good, because presets become auditable and reusable across clients immediately
* ✅ Good, because future import/export can build directly on persistent records
* ❌ Bad, because it expands V1 scope into new tables, APIs, RBAC, audit behavior, and bootstrap logic
* ❌ Bad, because it forces an authoritative backend model before the preset UX has stabilized

### Option 3: Frontend-only import/export and community sharing without backend persistence

Expose YAML import/export or "community" flows purely in browser state.

* ✅ Good, because it is quick to demo
* ❌ Bad, because it has no durability, auditability, or RBAC semantics
* ❌ Bad, because it misleads users into treating session-local state as a real platform catalog
* ❌ Bad, because it blurs the boundary between convenience helpers and managed assets

---

## More Information

### Related Decisions

* `ADR-0007-template-storage.md` - template records remain PostgreSQL-managed
* `ADR-0018-instance-size-abstraction.md` - Template and InstanceSize remain separate canonical models
* `ADR-0036-template-instancesize-boundary-enforcement.md` - preset application must still respect Template vs InstanceSize boundaries
* `ADR-0040-catalog-scope-for-template-and-instancesize.md` - visibility classification remains separate from preset source classification

### References

* `web/src/presets/catalog/`
* `web/src/features/admin-templates/templatePresets.ts`
* `web/src/features/admin-instance-sizes/instanceSizePresets.ts`
* `docs/rfc/RFC-0021-preset-catalog-marketplace.md`

### Implementation Notes

While this ADR remains proposed:

* Built-in preset authoring should remain in TypeScript, not JSON or YAML, so
  the frontend can enforce type safety, shared helpers, and linked defaults.
* Product language should prefer `Curated Catalog` over `Enterprise Catalog`.
  These presets may be production-informed, but they are not universal or
  organization-binding defaults.
* V1 UI should prefer guided selections and examples over raw blank inputs.
* Durable import/export, marketplace, and community distribution remain V2 work
  and require backend persistence. See RFC-0021.

Revisit this ADR when one or more of the following becomes true:

* users need durable preset import/export
* multiple clients beyond the web UI need shared preset catalogs
* moderation, RBAC, provenance, or bundle validation become product requirements

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-12 | @jindyzhao | Initial draft |
