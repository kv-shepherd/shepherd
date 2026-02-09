# Schema Catalog

> **Purpose**: Define canonical table domains and ownership boundaries.
> Detailed field definitions remain in Ent schema and phase documents.

---

## Source of Truth

- Ent schema definitions: `ent/schema/*.go`
- Governance and audit schema details: [04-governance.md §6.1 and §7](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- Contract and entity schema details: [01-contracts.md §3 Core Ent Schemas](../phases/01-contracts.md#3-core-ent-schemas)

---

## Table Domains

| Domain | Core Tables | Notes |
|------|------|------|
| Primary resources | `systems`, `services`, `vms`, `templates`, `instance_sizes`, `clusters`, `namespace_registry` | Runtime-owned entities |
| Governance and approvals | `approval_tickets`, `batch_approval_tickets`, `approval_policies` | Approval workflow and batch orchestration (`approval_tickets.parent_ticket_id` models child linkage) |
| Platform RBAC | `users`, `roles`, `permissions`, `role_permissions`, `role_bindings`, `resource_role_bindings` | Platform-level access control |
| Event and async | `domain_events`, River tables (`river_job`, `river_*`) | Claim-check and async execution |
| Audit and notifications | `audit_logs`, `notifications` | Compliance and user feedback |
| Auth provider integration | `auth_providers`, `idp_synced_groups`, `idp_group_mappings`, `external_approval_systems` | Enterprise auth and external approvals |
| Security/bootstrap | `system_secrets`, session/auth tables | Secret bootstrap and auth runtime |
| Recovery and compensation | `pending_adoptions` | Resource adoption compensation capability |
| Rate limiting | `rate_limit_counter`, `rate_limit_exemption` | ADR-0015 two-layer limits support |

---

## Relationship Baseline

- `systems` 1:N `services`
- `services` 1:N `vms`
- `approval_tickets` link request lifecycle to resources and events
- `domain_events` are immutable execution intents consumed by River workers
- `audit_logs` are append-only compliance records

---

## Hard Delete Boundary

- Primary resource tables (`systems`, `services`, `vms`) use hard delete in final state.
- `DELETING` is a transient operational state, not a long-term tombstone strategy.
- Audit/event/ticket records are retained independently by policy.

See:

- [lifecycle-retention.md §Retention Classes](./lifecycle-retention.md#retention-classes-table-centric)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
