# Design Note: ADR-0018 — Schema Help Localization Boundary

> **Status**: Implementation note for accepted ADR-0018  
> **Related ADR**: [ADR-0018](../../adr/ADR-0018-instance-size-abstraction.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-12  
> **Last Updated**: 2026-03-12

## Summary

ADR-0018 is accepted and therefore immutable. This note records the current
implementation boundary for localized field help in schema-driven forms without
editing the accepted ADR text.

The governing interpretation remains:

- KubeVirt official schema is the only semantic source of truth
- mask remains a projection layer for path exposure and UX grouping
- localization may improve comprehension, but must not redefine field meaning,
  valid values, or constraints

## Resolution Order

Structured form help should resolve in the following order:

1. explicit UI help override (`help_key` / `help_text`)
2. schema-specific i18n override keyed by stable schema path
3. official schema `description`

Runtime machine translation is out of scope.

## Coverage Rules

1. `display_name_key`, `help_key`, and `placeholder_key` are UX metadata only.
2. Missing localized help must degrade to official schema description when
   available.
3. If official schema has no `description`, the gap should be tracked by an
   audit mechanism and filled with curated bilingual help text.
4. Bilingual help supplements official schema semantics; it does not replace
   them.

## Implementation Pointers

- `web/src/i18n/schemaHelp.ts`
- `web/src/i18n/locales/en/schema.json`
- `web/src/i18n/locales/zh-CN/schema.json`
- `web/scripts/audit-schema-help.mjs`
- `web/src/features/admin-templates/components/DynamicSchemaForm.tsx`

## Revisit Conditions

- schema-driven rendering expands beyond `InstanceSize`
- the project introduces a centralized localization workflow for schema-derived help
- a later ADR supersedes ADR-0018's frontend interpretation boundary
