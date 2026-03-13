# Design Note: ADR-0046 — Schema Mask Field Visibility Tiers

> **Status**: Proposed (ADR-0046 under review until 2026-03-15)  
> **Related ADR**: [ADR-0046](../../adr/ADR-0046-schema-mask-field-visibility-tiers.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-13  
> **Last Updated**: 2026-03-13

## Summary

ADR-0046 proposes formalizing `professional_fields` as a third SchemaMask tier
alongside `quick_fields` and `advanced_fields`. The intent is to keep the
default admin UX safe and uncluttered while still exposing rare/expert-only
configuration paths behind an explicit opt-in.

This note captures the implementation-facing boundary while ADR-0046 is still
in review.

## Scope

- In scope: `SchemaMask` tiers (`quick_fields`, `advanced_fields`, `professional_fields`)
- In scope: UI gating and presentation conventions for `professional_fields`
- In scope: mask authoring guidelines for promoting a path to `professional_fields`
- Out of scope: authorization/RBAC semantics for schema paths
- Out of scope: server-side validation differences based on tier
- Out of scope: introducing more than three tiers or a risk taxonomy

## Pending Clarification

Until ADR-0046 is accepted, implementation should follow these constraints:

1. `professional_fields` must not be shown by default; require an explicit opt-in.
2. Visibility tiers are UI-only; they do not change backend validation or permission rules.
3. `mask.quick_fields` must remain present as an array in API responses (OpenAPI required).
4. Mask validation, audits, and tests must treat `professional_fields` paths the same as other mask paths.

## Affected Components

- `api/openapi.yaml` and `internal/api/specembed/openapi.yaml` (`SchemaMask`)
- `internal/api/handlers/server_schema.go` (response shape; `quick_fields` non-nil)
- `internal/pkg/schema/*.mask.json` (mask authoring)
- `internal/pkg/schema/validate.go` (mask path validation)
- `web/src/features/admin-templates/components/DynamicSchemaForm.tsx` (tier gating in UI)
- `web/scripts/audit-schema-help.mjs` (coverage audit)

## Rollout Notes

- Keep the default UI focused on `quick_fields`.
- Keep `advanced_fields` for commonly adjusted tuning.
- Use `professional_fields` only for rare/expert-only paths and require reviewers to ask "why is this not advanced?".

## Revisit Conditions

- ADR-0046 is accepted, rejected, or superseded
- the platform needs additional field categories beyond three tiers
- the UI requires a richer per-field metadata model (for example risk grades)

