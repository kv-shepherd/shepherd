# VM Lifecycle Write Model

> **Purpose**: Authoritative persistence model for VM lifecycle stages referenced by
> `interaction-flows/master-flow.md`.
>
> **Scope**: Write sets, transaction boundaries, and status transitions.
> Detailed DDL/index definitions remain in schema and migration documents.

---

## Authority Links

- [ADR-0006 Unified Async Model](../../adr/ADR-0006-unified-async-model.md)
- [ADR-0009 Domain Event Pattern](../../adr/ADR-0009-domain-event-pattern.md)
- [ADR-0012 Hybrid Transaction](../../adr/ADR-0012-hybrid-transaction.md)
- [ADR-0015 §13 Deletion Cascade Constraints](../../adr/ADR-0015-governance-model-v2.md#13-deletion-cascade-constraints)
- [ADR-0015 §19 Batch Operations V1](../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)
- [04-governance.md §5.6 Batch Operations](../phases/04-governance.md#56-batch-operations-adr-0015-19)
- [04-governance.md §6.1 Delete Cascade and Confirmation](../phases/04-governance.md#61-delete-cascade-and-confirmation-mechanism-adr-0015-13-131)
- [04-governance.md §7 Audit Logging](../phases/04-governance.md#7-audit-logging)

---

## Stage 5.A: VM Request Submission (Pending Approval)

### Transaction Boundary

- Pre-check (outside transaction): duplicate pending request detection.
- Main write transaction (single commit):
  - Insert `domain_events` (`VM_CREATE_REQUESTED`, `PENDING`)
  - Insert `approval_tickets` (`VM_CREATE`, `PENDING_APPROVAL`)
  - Insert `audit_logs` (`vm.request_submitted`)
  - Insert `notifications` (admin inbox)

### Write-Set Diagram

```
User Submit
  -> duplicate check (pending ticket exists?)
  -> TX begin
       -> domain_events: PENDING
       -> approval_tickets: PENDING_APPROVAL
       -> audit_logs: vm.request_submitted
       -> notifications: APPROVAL_REQUIRED
     TX commit
  -> response 202 + ticket_id
```

### State Conclusions

| Entity | Before | After |
|------|------|------|
| `approval_tickets` | none | `PENDING_APPROVAL` |
| `domain_events` | none | `PENDING` |
| `vms` | none | none |
| `river_job` | none | none |

---

## Stage 5.B: Admin Approval / Rejection

### Approve Path Transaction

- Update `approval_tickets` to `APPROVED` and persist approval snapshot fields.
- Update `domain_events` to `PROCESSING`.
- Insert `vms` row with transient status `CREATING`.
- Insert `river_job` with EventID claim-check payload.
- Insert `audit_logs` (`vm.request_approved`) and notification for requester.

### Reject Path Transaction

- Update `approval_tickets` to `REJECTED`.
- Update `domain_events` to `CANCELLED`.
- Insert `audit_logs` (`vm.request_rejected`) and requester notification.
- No `vms` insert, no `river_job` insert.

### State Conclusions

| Path | Ticket | Domain Event | VM Row | River Job |
|------|--------|--------------|--------|-----------|
| Approve | `PENDING_APPROVAL -> APPROVED` | `PENDING -> PROCESSING` | created (`CREATING`) | created (`available`) |
| Reject | `PENDING_APPROVAL -> REJECTED` | `PENDING -> CANCELLED` | not created | not created |

---

## Stage 5.D: Delete Write Model

### Canonical Policy

- Primary resource tables (`vms`, `services`, `systems`) use hard delete.
- `audit_logs`, `approval_tickets`, `domain_events` are retained independently and
  archived by retention policy.

### Delete Write Patterns

| Entity | Approval | Primary Write Pattern |
|------|----------|-----------------------|
| VM | Required | Ticket + event + transient `DELETING` -> provider delete -> hard delete |
| Service | Not required | Cascade validation + transient `DELETING` -> worker cleanup -> hard delete |
| System | Not required | Cascade validation -> hard delete in transaction |

### Audit Naming Baseline

- Canonical actions: `*.delete_submitted`, `*.delete_approved`, `*.delete_executed`.
- New content must not introduce `*.delete_request` as canonical naming.

---

## Stage 5.E: Batch Parent-Child Write Model

### Submission Transaction

- Layer 1/L2 throttling pre-check.
- One atomic transaction:
  - Insert parent batch ticket.
  - Insert all child tickets.
  - Insert initial audit record(s).
  - If any child insert fails, rollback all.

### Execution Model

- Child jobs execute independently.
- Parent row is aggregate projection (`IN_PROGRESS`, `COMPLETED`,
  `PARTIAL_SUCCESS`, `FAILED`, `CANCELLED`).
- Retry/cancel writes operate on child scope first, then recompute parent aggregate.

---

## Stage 6: VNC Access Write Model

### Test Environment

- No approval ticket.
- Permission check + runtime state check.
- Token issue and access audit write (`vnc.session_started`).

### Production Environment

- Create approval ticket (`VNC_ACCESS_REQUESTED`, `PENDING_APPROVAL`).
- On approval: issue token and append audit + notification.
- Token usage tracking is runtime-state oriented; storage implementation is
  governed by security/ops policy and must still satisfy auditability.

---

## Related Docs

- [master-flow.md §Part 3 VM Lifecycle Flow](../interaction-flows/master-flow.md#part-3-vm-lifecycle-flow)
- [master-flow.md §Stage 6 VNC Console Access](../interaction-flows/master-flow.md#stage-6-vnc-console-access)
- [lifecycle-retention.md §Retention Classes](./lifecycle-retention.md#retention-classes-table-centric)
- [transactions-consistency.md §Canonical Write Pattern](./transactions-consistency.md#canonical-write-pattern)
