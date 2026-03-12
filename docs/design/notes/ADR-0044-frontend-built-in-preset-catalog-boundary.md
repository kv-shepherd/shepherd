# Design Note: ADR-0044 — Frontend Built-In Preset Catalog Boundary

> **Status**: Proposed (ADR-0044 under review until 2026-03-14)  
> **Related ADR**: [ADR-0044](../../adr/ADR-0044-frontend-built-in-preset-catalog-boundary.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-12  
> **Last Updated**: 2026-03-12

## Summary

ADR-0044 proposes that built-in preset catalogs for Templates and InstanceSizes
remain frontend-authored UI assets in V1. They are intended to improve form UX
through one-click grouped presets, linked defaults, and examples, but they do
not become backend-managed resources until an administrator saves an actual
Template or InstanceSize.

This note captures the implementation-facing boundary while the ADR is still in
review.

## Scope

- In scope: built-in preset catalogs under `web/src/presets/`
- In scope: `curated` and `official` source groups in admin catalog forms
- In scope: using presets as form-fill helpers only
- In scope: keeping Template and InstanceSize persistence unchanged in V1
- Out of scope: backend catalog tables or catalog import/export APIs
- Out of scope: marketplace moderation, provenance, or community sharing

## Pending Clarification

Until ADR-0044 is accepted, implementation should follow these rules:

1. Built-in presets are not seeded backend records.
2. Applying a preset only changes current form values and help text.
3. `Template` and `InstanceSize` become real catalog entries only after the
   existing save flow succeeds.
4. V1 should expose built-in sources such as `curated` and `official`, but not
   browser-only import/export or community-market semantics.
5. Internal authoring format for built-in presets stays in TypeScript.

## Affected Components

- `web/src/presets/catalog/`
- `web/src/features/admin-templates/templatePresets.ts`
- `web/src/features/admin-instance-sizes/instanceSizePresets.ts`
- `web/src/features/admin-templates/components/AdminTemplatesContent.tsx`
- `web/src/features/admin-instance-sizes/components/AdminInstanceSizesContent.tsx`

## Rollout Notes

- Keep the current V1 preset picker focused on guided application only.
- Treat `RFC-0021` as the backend-backed V2 follow-up for durable catalog
  exchange and marketplace workflows.
- If later work requires import/export, design the backend API and persistence
  first; do not reintroduce browser-session-only flows.

## Revisit Conditions

- ADR-0044 is accepted, rejected, or superseded
- the project introduces backend catalog persistence
- durable import/export becomes part of the committed roadmap
