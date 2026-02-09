# Migrations

> **Purpose**: Define schema evolution workflow for business tables and queue tables.

---

## Tooling Ownership

| Scope | Tool | Source |
|------|------|------|
| Business schema | Atlas (from Ent schema) | `ent/schema/*.go`, Atlas migration files |
| Queue schema | River migration tool | `river migrate-up` managed tables |

---

## Apply Order

1. Apply Atlas migrations (business/domain tables).
2. Apply River migrations (queue runtime tables).
3. Run startup/schema validation checks.

Rationale:

- Business schema must exist before request/worker workflows rely on it.
- River tables are runtime dependencies for async processing.

---

## Migration Rules

- Backward compatibility first: avoid breaking running workers/controllers mid-rollout.
- For destructive schema evolution, use staged rollout (expand -> migrate -> contract).
- Keep DDL and code changes in the same PR/changeset when tightly coupled.
- Regenerate code artifacts after schema changes (Ent/sqlc as applicable).

---

## Rollout Checklist

- Migration scripts reviewed and reproducible in local/dev environments.
- No prohibited manual DDL in app startup path.
- CI checks pass for schema/codegen/governance scripts.
- Rollback/mitigation notes included for non-trivial changes.

---

## Operational Notes

- Monitor queue table bloat/autovacuum (`river_*`) and audit table growth.
- Ensure retention/archival jobs are aligned with lifecycle policy.

See:

- [00-prerequisites.md §6 Database Connection](../phases/00-prerequisites.md#6-database-connection)
- [00-prerequisites.md §Manual Migration](../phases/00-prerequisites.md#manual-migration-developmentci)
- [04-governance.md §1 Database Migration](../phases/04-governance.md#1-database-migration)
- [04-governance.md §2 River Queue](../phases/04-governance.md#2-river-queue-adr-0006)
