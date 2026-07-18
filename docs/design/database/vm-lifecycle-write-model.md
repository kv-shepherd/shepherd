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
  - Insert `tickets` (`CREATE`, `PENDING`)
- After commit, the use case attempts the audit write and the handler invokes
  approval routing and admin notification triggers. These supplemental writes
  are best-effort and are not part of the atomic Event/Ticket write set.

### Write-Set Diagram

```
User Submit
  -> duplicate check (pending ticket exists?)
  -> TX begin
       -> domain_events: PENDING
       -> tickets: CREATE / PENDING
     TX commit
  -> best-effort audit + approval routing + admin notification
  -> response 202 + ticket_id
```

### State Conclusions

| Entity | Before | After |
|------|------|------|
| `tickets` | none | `PENDING` |
| `domain_events` | none | `PENDING` |
| `vms` | none | none |
| `river_job` | none | none |

---

## Stage 5.B: Admin Approval / Rejection

### Approve Path Transaction

- Update `tickets` to `APPROVED` and persist approval snapshot fields.
- Update `domain_events` to `PROCESSING`.
- Insert `vms` row with transient status `CREATING`.
- Insert `river_job` with EventID claim-check payload.
- After commit, attempt the approval audit and requester notification as
  best-effort supplemental writes.

### Reject Path Transaction

- Update `tickets` to `REJECTED`.
- Update `domain_events` to `CANCELLED`.
- No `vms` insert, no `river_job` insert.
- After commit, attempt the rejection audit and requester notification as
  best-effort supplemental writes.

### State Conclusions

| Path | Ticket | Domain Event | VM Row | River Job |
|------|--------|--------------|--------|-----------|
| Approve | `PENDING -> APPROVED` | `PENDING -> PROCESSING` | created (`CREATING`) | created (`available`) |
| Reject | `PENDING -> REJECTED` | `PENDING -> CANCELLED` | not created | not created |

---

## Stage 5.D: Delete Write Model

### Canonical Policy

- Primary resource tables (`vms`, `services`, `systems`) use hard delete.
- `audit_logs`, `tickets`, `domain_events` are retained independently and
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

- Validate immutable request shape and every item before the write transaction.
- For new-version writers, one `READ COMMITTED` business transaction:
  - Acquire global, actor, and non-empty operation-scoped request advisory
    locks in canonical order.
  - Resolve exact replay before reading mutable quotas/cooldown.
  - Evaluate current global/user throttles under those guards.
  - Insert the parent Ticket/Event, `batch_tickets` projection, and every child
    Ticket/Event pair.
  - If any check or insert fails, roll back all.
- The handler attempts a supplemental audit write only after commit. That call
  is best-effort, is not part of this atomic write set, and cannot provide
  durable actor attribution.

### Execution Model

- Initial approval atomically claims the raw parent Ticket/Event to
  `EXECUTING/PROCESSING`, persists the normalized execution snapshot, refreshes
  the projection to `IN_PROGRESS`, and inserts one parent-keyed
  `batch_approval_dispatch` River job on its dedicated queue.
- The dispatcher reloads durable state and schedules safe `PENDING` children;
  child jobs then execute independently. Dispatcher consistency mismatches fail
  closed instead of rewriting children under a terminal parent.
- Explicit retry targets only an execution-`FAILED` child below three logical
  attempts. Generic retry conditionally resets the child Ticket and its
  accepted-state Event, reopens the approved parent, refreshes the projection,
  and inserts a dispatcher in one transaction. Power retry conditionally resets
  Ticket/Event and inserts replacement River work in one transaction.
  Approval-`REJECTED` is terminal and cannot enter execution retry.
- Cancel performs a handler-side parent identity/state check, then uses one
  parent-keyed transaction for exact `PENDING` child Ticket/Event cancellation
  and parent Ticket/Event/projection aggregation. The parent row is locked and
  updated by expected state; a concurrent mismatch rolls back the whole cancel.
- Raw parent Ticket states are
  `PENDING -> EXECUTING -> SUCCESS|FAILED|CANCELLED`; raw parent Event states
  are `PENDING -> PROCESSING -> COMPLETED|FAILED|CANCELLED`. The distinct API
  projection is
  `PENDING_APPROVAL -> IN_PROGRESS -> COMPLETED|PARTIAL_SUCCESS|FAILED|CANCELLED`
  and has no `APPROVED` state.
- Parent aggregation runs under a parent-row lock. All cancelled children map
  to `CANCELLED`; success plus failed/cancelled siblings maps to
  `PARTIAL_SUCCESS`; other terminal outcomes with no success map to `FAILED`.
- After a successful retry/cancel commit the handler makes a best-effort
  supplemental audit call. The workflow requester and original/replacement
  approver remain the durable attribution fields.
- An ambiguous restart remains Event/Ticket `PROCESSING/EXECUTING` and cannot
  use ordinary retry. No API can release the fence: a River job may become
  terminal while its provider request is still in flight, so job state and an
  operator observation do not prove that a late restart is impossible. The
  conflict response exposes `operator-runbook:ambiguous-vm-restart`; operators
  must drain the originating worker, preserve evidence, inspect provider state,
  and escalate without editing workflow rows or redispatching. A safe release
  requires a future provider receipt/idempotency or provable-cancellation
  protocol.

### Ambiguous Restart Fence Runbook

1. Stop new power mutations for the VM and drain or terminate the worker that
   claimed the event. Do not infer cancellation from River state alone.
2. Preserve the Event, Ticket, River job, provider, and worker evidence and
   independently inspect the VM/VMI state.
3. Do not clear database state, call a reconciliation endpoint, or redispatch
   restart. No public or administrative API releases this fence.
4. Escalate for a reviewed forward recovery only after the provider can return
   a durable operation receipt or the original request is provably cancelled.

---

## Stage 6: VNC Access Write Model

### Test Environment

- No approval ticket.
- Permission check + runtime state check.
- Issue the access grant, then attempt the `vnc.session_started` audit as a
  best-effort supplemental write.

### Production Environment

- Create a `tickets` row (`VNC_ACCESS`, `PENDING`) paired with its DomainEvent.
- On approval, issue the access grant; audit and notification calls run after
  the workflow commit as best-effort supplemental writes.
- Token usage tracking is runtime-state oriented; storage implementation is
  governed by security/ops policy and must still satisfy auditability.

---

## Related Docs

- [master-flow.md §Part 3 VM Lifecycle Flow](../interaction-flows/master-flow.md#part-3-vm-lifecycle-flow)
- [master-flow.md §Stage 6 VNC Console Access](../interaction-flows/master-flow.md#stage-6-vnc-console-access)
- [lifecycle-retention.md §Retention Classes](./lifecycle-retention.md#retention-classes-table-centric)
- [transactions-consistency.md §Canonical Write Pattern](./transactions-consistency.md#canonical-write-pattern)
