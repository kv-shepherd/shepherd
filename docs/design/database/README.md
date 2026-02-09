# Database Design Index

> **Authority**: This directory is the database-focused reference layer for `docs/design/`.
> It defines persistent data boundaries, lifecycle, and transactional consistency rules.

---

## Scope

This layer is authoritative for:

- Database object ownership and table catalog
- Resource lifecycle and retention policy at persistence level
- Transaction/consistency boundaries (ADR-0006, ADR-0009, ADR-0012)
- Migration and schema evolution rules

This layer is not for:

- End-user interaction storyboards (see [master-flow.md §Stage 5.D](../interaction-flows/master-flow.md#stage-5-d))
- API request/response contract details (see [01-contracts.md §3 Core Ent Schemas](../phases/01-contracts.md#3-core-ent-schemas))
- CI gate policy details (see [ci/README.md §Scope Boundary](../ci/README.md#scope-boundary))

---

## Document Map

| Document | Purpose |
|------|------|
| [schema-catalog.md §Table Domains](./schema-catalog.md#table-domains) | Canonical table groups, ownership, and key relationships |
| [lifecycle-retention.md §Retention Classes](./lifecycle-retention.md#retention-classes-table-centric) | Hard-delete policy, retained records, and archival baseline |
| [transactions-consistency.md §Canonical Write Pattern](./transactions-consistency.md#canonical-write-pattern) | Transaction boundaries and async consistency model |
| [vm-lifecycle-write-model.md §Stage 5.A](./vm-lifecycle-write-model.md#stage-5a-vm-request-submission-pending-approval) | VM request/approval/delete/batch/VNC write sets and state transitions |
| [migrations.md §Apply Order](./migrations.md#apply-order) | Atlas/River migration flow and rollout rules |

---

## Relationship with Other Layers

- [master-flow.md §Stage 5.D](../interaction-flows/master-flow.md#stage-5-d) describes interaction intent and links here for database principles.
- [phase docs](../phases/04-governance.md#1-database-migration) provide implementation-level details that must comply with this layer.
- [examples/README.md §Example Index](../examples/README.md#example-index) indexes code patterns implementing these rules.

When conflicts are found:

1. Accepted ADR
2. This database layer + [CHECKLIST.md §Core ADR Constraints](../CHECKLIST.md#core-adr-constraints-single-reference-point)
3. Phase and example docs
4. Interaction flow representation

---

## Change Governance

- Structural or behavioral DB changes (delete semantics, retention semantics, transaction model) must trace to ADR decisions.
- Editorial and linkage updates can be applied directly with CI checks and review.
