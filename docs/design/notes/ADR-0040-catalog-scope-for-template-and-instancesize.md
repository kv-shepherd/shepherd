# Design Note: ADR-0040 — Catalog Scope for Template and InstanceSize

> **Status**: Proposed  
> **Related ADR**: [ADR-0040](../../adr/ADR-0040-catalog-scope-for-template-and-instancesize.md)  
> **Owner**: @jindyzhao  
> **Date**: 2026-03-06

## Summary

ADR-0040 proposes a dedicated `catalog_scope` classification on `Template` and
`InstanceSize` so user-facing request flows can filter catalog entries by the
selected namespace environment type without overloading the platform meaning of
`environment`. This note captures the proposed schema, API, validation, and
migration impacts while the ADR remains under review.

## Scope

- In scope: Ent schema changes for `Template` and `InstanceSize`
- In scope: VM request-context filtering and stale-selection validation
- In scope: Admin catalog behavior for `unclassified` entries
- In scope: OpenAPI and web contract updates for catalog-scope exposure
- Out of scope: cluster capability matching
- Out of scope: cluster policy governance for overcommit and hardware controls
- Out of scope: per-cluster catalog allowlists

## Pending Changes (Not Yet Normative)

- Affected docs:
  - `docs/adr/ADR-0040-catalog-scope-for-template-and-instancesize.md`
  - `docs/design/interaction-flows/master-flow.md`
  - `docs/design/admin-catalog/` documents that mention template or instance
    size visibility
- Affected components:
  - `ent/schema/template.go`
  - `ent/schema/instance_size.go`
  - `internal/api/handlers/server_vm.go`
  - `internal/service/approval_validator.go`
  - OpenAPI contract and generated types for template / instance-size payloads
  - Web VM-request context loaders and admin catalog forms
- Behavior changes:
  - request-context APIs return only `matching environment_type` and `all`
  - `unclassified` catalog entries are hidden from regular user request flows
  - create-request validation rejects template or size selections whose
    `catalog_scope` no longer matches the selected namespace environment type

## Proposed Data Shape

Both catalog entities gain the same field:

```go
field.Enum("catalog_scope").
    Values("unclassified", "test", "prod", "all").
    Default("unclassified")
```

Proposed semantics:

| Value | Meaning |
|------|---------|
| `unclassified` | Hidden from regular user request flows until reviewed |
| `test` | Visible only when the selected namespace environment type is `test` |
| `prod` | Visible only when the selected namespace environment type is `prod` |
| `all` | Visible in both `test` and `prod` request flows |

This field is for catalog visibility only. It must not be reused as a
scheduling, approval, or capability signal.

## API and Validation Touchpoints

### Request Context

`GetVMRequestContext` should stop returning all enabled templates and instance
sizes. The proposed filtering rules are:

- allowed scopes for `test`: `test`, `all`
- allowed scopes for `prod`: `prod`, `all`
- excluded from user context: `unclassified`

### Create Request Validation

The write path should re-check the selected template and instance size against
the namespace environment type to prevent stale UI selections or forged
requests.

### Admin Catalog APIs

Admin-facing list and detail APIs should continue to expose all scopes,
including `unclassified`, so catalog maintainers can review and classify items.

## Migration / Rollout

- Data migration:
  - add `catalog_scope` to `templates` and `instance_sizes`
  - backfill existing rows to `unclassified`
  - do not use `NULL = all`; zero-trust default is intentional
- Compatibility notes:
  - user-facing APIs should treat missing field values from pre-migration data
    as non-visible until backfill is complete
  - admin APIs may need a temporary filter or badge for `unclassified` cleanup
- Rollout order:
  1. schema migration
  2. admin API + UI exposure
  3. request-context filtering
  4. create-request enforcement

## Open Questions

- Should admin create/update APIs require explicit `catalog_scope` from day one,
  or allow temporary omission with server default `unclassified`?
- Which design doc under `docs/design/` should become the canonical catalog
  visibility reference after ADR-0040 is accepted?
