# ADR-0025 Design Notes: Bootstrap Secrets Auto-Generation (V1)

> **Status**: Pending ADR-0025 acceptance

## Summary

- V1: If `ENCRYPTION_KEY` or `SESSION_SECRET` is missing at startup, generate strong random keys.
- Persist generated keys in PostgreSQL.
- Load keys into memory on startup; no automatic rotation in V1.
- External secret manager or env vars override DB values.

## Schema Proposal (DB Storage)

**Table**: `system_secrets`

| Column | Type | Notes |
|--------|------|------|
| `id` | string | Primary key (single row or named keys) |
| `key_name` | string | `ENCRYPTION_KEY` / `SESSION_SECRET` |
| `key_value` | string | Base64-encoded secret; encrypted at rest at DB level | 
| `source` | string | `db_generated` / `env` / `external` |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update |

**Usage**:
- On startup: check external/env → else DB → else generate+persist in DB.
- Only one active value per `key_name`.

## Access Control (Minimum Privilege)

- Only the application service DB role can `SELECT/INSERT/UPDATE` this table.
- No admin UI exposure; no API returns key values.
- Audit any changes to `system_secrets` metadata (not values).
- Logs must never include `key_value`.

## References

- ADR-0025 (proposed)
- RFC-0016 Key Rotation
