# RFC-0021: Preset Catalog Marketplace for Templates and Instance Sizes

> **Status**: Proposed  
> **Priority**: P2  
> **Trigger**: Users need durable import/export, shared catalog governance, or community-distributed template and instance-size presets beyond built-in frontend defaults  
> **Related**: [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md), [ADR-0021](../adr/ADR-0021-api-contract-first.md), [ADR-0036](../adr/ADR-0036-template-instancesize-boundary-enforcement.md), [ADR-0040](../adr/ADR-0040-catalog-scope-for-template-and-instancesize.md), [ADR-0044](../adr/ADR-0044-frontend-built-in-preset-catalog-boundary.md), [RFC-0019](./RFC-0019-kubevirt-instancetype-adapter.md)

---

## Problem

The current admin experience needs higher-level presets so operators do not have
to understand or manually fill every KubeVirt-adjacent field.

That requirement has two distinct layers:

1. **V1 preset guidance**
   - The UI should offer curated choices instead of blank inputs wherever
     possible.
   - Curated presets may reflect validated production-friendly conventions, but
     they are **not** universal defaults for all users.
   - The product should therefore expose multiple built-in catalog sources, such
     as `curated` and `official`, rather than only one opinionated internal set.

2. **V2 import/export and marketplace**
   - Once users can import, publish, export, and share presets, the platform
     needs persistence, provenance, validation, RBAC, and auditability.
   - A browser-session-only import/export flow is not a real product capability.
   - Durable catalog exchange implies backend-managed storage and APIs.

The key clarification is that **frontend-only presets are valid for V1**, but
**import/export is not**. Import/export changes the system boundary from local UI
convenience into a managed catalog lifecycle. ADR-0044 defines the V1 boundary;
this RFC begins where that ADR stops.

---

## Current V1 Direction

V1 should remain intentionally simple:

- Built-in preset catalogs are authored in frontend code and shipped with the UI
- Catalog sources are limited to:
  - `curated`: validated combinations for common production-friendly scenarios
  - `official`: upstream or open-source starter presets that help users bootstrap common scenarios
- Admin forms consume these catalogs as one-click helpers
- The backend remains unaware of built-in preset catalogs
- No durable user import/export, publish, sync, or marketplace flows are exposed

This keeps V1 aligned with the existing "dumb backend" constraint for preset
authoring while still improving usability.

---

## Proposed Solution

Introduce a **backend-backed Preset Catalog Marketplace** as a V2 capability.

### Design Principles

| Principle | Description |
|-----------|-------------|
| V1 stays frontend-only | Built-in presets remain UI assets, not database records |
| Import/export requires persistence | Once a preset enters user workflow as shareable content, it must be stored, validated, and traceable |
| Template and InstanceSize are separate kinds | Preserve ADR-0036 boundary; do not blur image/source concerns with hardware sizing |
| Source and trust are distinct | A preset may be `official`, `curated`, or `imported`; verification state is independent |
| Marketplace is catalog-first, not provider-first | The platform manages metadata, governance, and UX even if some payloads map to upstream KubeVirt objects |
| Exchange format is external, authoring format is internal | Internal built-ins can stay in TypeScript; external exchange should use a portable bundle format validated by backend APIs |

### V2 Capability Set

The backend should own a catalog registry with at least these concerns:

- Store catalog items for:
  - templates
  - instance sizes
- Track catalog metadata:
  - `source_type`: `official | curated | imported`
  - `verification_level`: `verified | experimental | unverified`
  - maintainer / owner
  - visibility / sharing scope
  - tags
  - created / updated timestamps
  - optional upstream references
- Validate imported bundles against a versioned schema
- Support RBAC-controlled publish/unpublish flows
- Preserve audit trails for creation, import, promotion, update, and deprecation

### V2 API Surface

Representative API shape:

```text
GET    /api/v1/catalog/items
POST   /api/v1/catalog/items
GET    /api/v1/catalog/items/{id}
PATCH  /api/v1/catalog/items/{id}
POST   /api/v1/catalog/import
GET    /api/v1/catalog/export/{id}
POST   /api/v1/catalog/bundles
GET    /api/v1/catalog/bundles/{id}
```

The exact OpenAPI contract is deferred, but the key point is that catalog
exchange becomes a backend concern, not a browser-only convenience function.

### Catalog Data Model

Suggested normalized model:

```text
CatalogBundle
  - id
  - name
  - source_type
  - visibility
  - owner_id
  - checksum
  - imported_from
  - created_at

CatalogItem
  - id
  - bundle_id
  - kind (template | instance_size)
  - key
  - label
  - description
  - source_type
  - verification_level
  - catalog_scope
  - os_family (template only)
  - os_version (template only)
  - values_json
  - tags_json
  - maintainer
  - created_at
  - updated_at
```

This preserves product-level metadata separately from the underlying template or
instance-size business tables.

### Frontend UX in V2

The frontend should evolve from "preset helper buttons" into a dedicated catalog
experience:

- `Official Catalog`
- `Curated Catalog`
- `My Imports`
- `Community`

Each item should expose:

- source badge
- verification badge
- tags
- requirements / caveats
- apply-to-form action
- import/export or publish actions when permitted

The key UX rule remains: prefer guided choices and grouped presets over large
unstructured forms.

---

## Phased Rollout

### Phase 1: Built-in Frontend Catalogs

Shipped in the current direction:

- Frontend-only TypeScript catalogs
- Curated presets for common production-friendly combinations
- Official open-source starter presets
- One-click application into template and instance-size forms
- No user import/export UI

### Phase 2: Backend Catalog Persistence

Promote this RFC when durable exchange becomes necessary:

1. Add catalog storage tables and APIs
2. Add import validation and persistence
3. Add export endpoints
4. Add RBAC and audit coverage
5. Add dedicated catalog listing pages

### Phase 3: Marketplace Workflows

Optional later extensions:

- moderation / approval for community submissions
- signed bundles or publisher trust
- upstream sync jobs
- organization-scoped shared catalogs
- compatibility checks against cluster/provider capabilities

---

## Trade-offs

### Pros

- Keeps V1 small and practical
- Avoids pretending browser-session import/export is a complete solution
- Creates a clear path from local preset UX to real catalog governance
- Preserves existing template vs instance-size boundaries
- Allows internal conventions and upstream starters to coexist without claiming either is universal

### Cons

- V2 introduces new backend tables, APIs, and RBAC rules
- Marketplace semantics add moderation and provenance complexity
- Imported presets may not map cleanly to every provider or cluster capability
- Users cannot share custom presets in V1 without waiting for backend support

---

## Implementation Notes

### Internal Authoring Format

For built-in catalogs, TypeScript remains the preferred authoring format because
it supports:

- type safety
- composition and reuse
- field-level helpers
- shared examples and transformations

### External Exchange Format

For V2 import/export, a portable bundle format is still useful, but it should be
treated as **API payload**, not as frontend-only state.

YAML is acceptable for external exchange because it matches Kubernetes operator
expectations and is easy to review, but the backend must parse, validate, and
normalize it before persistence.

### Non-Goals

This RFC does not propose:

- replacing the existing `Template` or `InstanceSize` business models
- storing V1 built-in presets in the database immediately
- requiring KubeVirt native template/instancetype CRDs as the platform source of truth
- shipping a community marketplace in V1

---

## References

- KubeVirt user guide: Templates  
  https://kubevirt.io/user-guide/user_workloads/templates/
- KubeVirt common templates  
  https://github.com/kubevirt/common-templates
- KubeVirt common instancetypes  
  https://github.com/kubevirt/common-instancetypes
