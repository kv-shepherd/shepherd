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

## Startup Migration Modes

Production startup owns the default migration flow. Released Docker images and
Go runtime archives default `DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS=true`, so
the server inspects database state before accepting traffic:

| Database state | Startup behavior |
|------|------|
| Fresh database, no core tables, no `atlas_schema_revisions` | Bootstrap the current Ent schema, record the latest Atlas migration version as the baseline, then apply River migrations. |
| Existing schema without `atlas_schema_revisions` | Adopt the schema with Atlas versioned migrations using the dirty-database allowance, then apply River migrations. |
| Atlas-managed schema with `atlas_schema_revisions` | Apply pending Atlas migrations normally, then apply River migrations. |

Do not run raw `atlas migrate apply` against a fresh database as the first
operation. The Atlas directory contains reviewed incremental migrations, not a
full base-schema dump. Fresh deployments should use the server startup path so
the base schema and Atlas revision baseline are established together.
Release artifacts bundle the Atlas executable and migration directory needed by
this path.

---

## Migration Rules

- Backward compatibility first: avoid breaking running workers/controllers mid-rollout.
- For destructive schema evolution, use staged rollout (expand -> migrate -> contract).
- Keep DDL and code changes in the same PR/changeset when tightly coupled.
- Regenerate code artifacts after schema changes (Ent/sqlc as applicable).
- Manual Atlas apply is reserved for development diagnostics or emergency
  operator-controlled repair where the database already has the base business
  schema or an Atlas revision table. Routine production upgrades must use the
  released server artifact startup path.

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
- [00-prerequisites.md §Migration Verification](../phases/00-prerequisites.md#migration-verification-developmentci)
- [04-governance.md §1 Database Migration](../phases/04-governance.md#1-database-migration)
- [04-governance.md §2 River Queue](../phases/04-governance.md#2-river-queue-adr-0006)
