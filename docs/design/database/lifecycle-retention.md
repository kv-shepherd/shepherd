# Data Lifecycle and Retention

> **Purpose**: Define persistence-level lifecycle, archival, and purge policy.
> This document is database-focused and does not define user interaction flow.

---

## Scope Boundary

This document defines:

- Table-level retention classes
- Archival strategy (`archived_at`, export, purge windows)
- Database-side protection for immutable compliance data

This document does not define:

- Approval decisions or UI confirmation flow
- API behavior and endpoint contracts
- Action naming conventions for business workflow

For those, see:

- [master-flow.md §Stage 5.D Delete Operations](../interaction-flows/master-flow.md#stage-5-d)
- [master-flow.md §Audit Log Design](../interaction-flows/master-flow.md#audit-log-design)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)

---

## Retention Classes (Table-Centric)

| Class | Tables (Representative) | Lifecycle | Purge Policy |
|------|------|------|------|
| Primary runtime state | `systems`, `services`, `vms` | Final state uses hard delete for removed resources | No long-term tombstone in primary tables |
| Immutable compliance/event | `audit_logs`, `domain_events` | Append-only records; may be marked archived | Purge only by scheduled admin-controlled job with separate audit trail |
| Workflow history | `approval_tickets`, `batch_approval_tickets` | Retained for traceability; eligible for archival (`approval_tickets.parent_ticket_id` carries child lineage) | Purge by retention window and policy class |
| Operational transient | `notifications`, selected queue/runtime records | Time-window retention | Periodic cleanup job |

---

## Archival Model

### In-Database Archive Marking

- Use nullable `archived_at` where table semantics require "retained but cold" data.
- Keep index support for archive filtering (for example, index on `archived_at`).
- Archived records remain queryable for compliance/reporting paths.

### Export and Cold Storage

- For large immutable datasets (especially `audit_logs`), support periodic export to external storage/SIEM.
- Export does not bypass in-database minimum retention requirements.

### Physical Purge

- Physical deletion is done only by scheduled privileged job.
- Purge job execution must itself be auditable.
- Purge criteria must be time-window + class-based (for example, environment/sensitivity).

---

## Database Guardrails

| Guardrail | Requirement |
|------|------|
| Append-only protection | Application roles must not have `UPDATE`/`DELETE` on immutable compliance tables |
| Privilege separation | Purge capability restricted to dedicated admin role/job |
| Archival safety | Archive queries rely on indexed time fields (`created_at`, `archived_at`) |
| Change control | Retention window changes require documented review and traceability |

---

## Baseline Windows (Policy Defaults)

| Policy Scope | Minimum Retention |
|------|------|
| Production audit data | >= 1 year |
| Test audit data | >= 90 days |
| Sensitive governance operations | >= 3 years |

These are baseline defaults. Stricter organizational compliance rules take precedence.

---

## Operational Responsibilities

| Job Type | Responsibility |
|------|------|
| Archive marker job | Mark eligible records with `archived_at` according to policy |
| Export job | Deliver immutable records to external compliance/analytics sink |
| Purge job | Permanently delete only records past policy threshold and legal hold checks |

---

## References

- [schema-catalog.md §Table Domains](./schema-catalog.md#table-domains)
- [migrations.md §Migration Rules](./migrations.md#migration-rules)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)
- [ADR-0015 §13 Deletion Cascade Constraints](../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0019 §3 Audit Logging and Sensitive Data Controls](../../adr/ADR-0019-governance-security-baseline-controls.md#3-audit-logging-and-sensitive-data-controls)
