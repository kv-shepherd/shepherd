# Design Note: ADR-0015 Batch Operations Implementation

> Status: Applied to normative docs (2026-02-06)  
> Related ADR: [ADR-0015 §19](../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)  
> Owner: @codex  
> Date: 2026-02-06

---

## Summary

This note defines a concrete implementation for ADR-0015 §19:

- Parent-child ticket model for batch operations
- Two-layer rate limiting (global + user-level)
- Atomic ticket creation with independent child execution

This note captured the design review baseline. The resulting changes were merged into `04-governance.md`, `master-flow.md`, and phase checklists on 2026-02-06.

---

## Scope

In scope:

- Batch create/delete/approve/power operation orchestration
- Parent-child ticket schema and status aggregation
- PostgreSQL-based two-layer rate limiting without Redis
- API request/response shape and error model
- Worker concurrency, retry, and observability requirements

Out of scope:

- UI pixel-level changes
- Multi-tenant custom quota policies
- Distributed transaction across clusters

---

## Baseline Requirements (ADR-0015 §19)

ADR-0015 requires:

1. Parent-child ticket model
2. Two-layer rate limits
3. Atomic creation, independent execution

This note was created while docs were still in the simplified "frontend batch -> per-item jobs" model and did not yet define full parent-child persistence plus two-layer limiting.

---

## Reviewed Implementation Plan (Historical)

## 1. API Contract

Canonical endpoints from ADR-0015:

- `POST /api/v1/vms/batch` (batch create/delete request submission)
- `GET /api/v1/vms/batch/{id}` (parent ticket status)
- `POST /api/v1/vms/batch/{id}/retry` (retry failed child items)
- `POST /api/v1/admin/rate-limits/exemptions`
- `DELETE /api/v1/admin/rate-limits/exemptions/{user_id}`
- `PUT /api/v1/admin/rate-limits/users/{user_id}`
- `GET /api/v1/admin/rate-limits/status`

Extension for existing simplified endpoints:

- Keep `POST /api/v1/vms/batch` and `POST /api/v1/vms/batch/power`
- Internally normalize them into the same parent-child ticket pipeline

Request requirements:

- Add optional opaque `request_id` for idempotency. UUIDs are accepted but are
  not required, preserving existing client-generated keys. The persistence
  column remains PostgreSQL `text`; the contract does not introduce a character
  limit, `varchar` narrowing, length-based truncation, or length-based clearing
  of historical keys.
- The attempt migration does not alter the `request_id` column or rewrite any
  historical value, including empty, whitespace-only, duplicate, or long keys.
- Enforce the currently published API limit while tracking the accepted-ADR
  limit mismatch as explicit contract debt; see §11 below

Response model:

- Submit returns `202 Accepted` with `batch_id` and `status_url`
- Status returns counts and per-child states

---

## 2. Data Model

Current physical mapping uses the workflow tables plus one API projection:

- `batch_tickets` (parent API projection)
  - `id`, `batch_type`, `child_count`, `success_count`, `failed_count`,
    `pending_count`, `status`, `request_id`, `reason`, `created_by`,
    `created_at`, `updated_at`
- `tickets` (workflow parent and child rows)
  - `id`, `event_id`, `parent_ticket_id`, `operation_type`, `status`,
    `requester`, `approver`, `reject_reason`, `attempt_count`,
    `last_attempt_at`, `created_at`, `updated_at`
- `domain_events`
  - durable parent/child operation payload and raw event state

Earlier draft-only logical aliases are retired. Runtime SQL examples must use
the physical names above.

Rate-limit policy tables (ADR-0015 §19):

- `rate_limit_exemptions`
  - `id` (the subject user ID), `exempted_by`, `reason`, `expires_at`
- `rate_limit_user_overrides`
  - `id` (the subject user ID), `max_pending_parents`,
    `max_pending_children`, `cooldown_seconds`, `reason`, `updated_by`

There is no mutable rate-limit counter table in the current implementation. The
submission transaction reads pending parents/children, recent batch Events, and
the actor's latest submission from durable workflow rows under the canonical
submission locks.

Current baseline indexes:

- `batch_tickets(status)`, `batch_tickets(created_at)`,
  `batch_tickets(created_by)`, and `batch_tickets(batch_type, created_by)`
- non-unique batch replay lookup on
  `(created_by, batch_type, sha256(normalized request_id))`; every hit is
  collision-checked against the exact normalized key and durable payload
- `tickets(status)`, `tickets(requester)`, `tickets(event_id)`, and
  `tickets(parent_ticket_id)`
- `rate_limit_exemptions(expires_at)`,
  `rate_limit_exemptions(created_at)`,
  `rate_limit_user_overrides(updated_by)`, and
  `rate_limit_user_overrides(updated_at)`

The current hardening deliberately does not add a unique index or digest column
for the idempotency tuple. Its SHA-256 expression index is only a non-unique
lookup accelerator and does not authorize replay. In particular,
`(created_by, batch_type, request_id)`
would incorrectly collapse power `START`, `STOP`, and `RESTART` into one scope.
New-version writers use transaction-scoped advisory locks and an exact
post-lock `(created_by, requested operation, request_id)` replay lookup against
the persisted parent payload. A database uniqueness mechanism requires a later
two-phase change after every old writer has exited and historical duplicates
have been audited; that follow-up must preserve `request_id` as `text`, retain
operation-scoped behavior, and choose its index representation through its own
review.

---

## 3. Atomicity Boundary

Creation phase (`single SQL transaction` for new-version writers):

1. Validate immutable request shape and every child before opening the write
   transaction.
2. Acquire global, actor, and non-empty request-key advisory locks in canonical
   order.
3. Resolve exact operation-scoped replay before mutable throttling.
4. Read current global/user limits and cooldown under those guards.
5. Insert the workflow parent/Event, API projection, and all child
   Ticket/Event pairs.
6. Commit once.

If any child insert/validation fails, rollback entire submission.
The handler attempts supplemental audit only after commit; that best-effort call
is not part of this atomic write set and its failure does not undo submission.

Execution phase (`independent`):

- Initial approval claims the parent/event, persists the normalized execution
  snapshot, refreshes the projection, and inserts one parent-keyed dispatcher
  in the same transaction. Any failure rolls back the entire approval decision.
- The dispatcher runs on its dedicated `batch_approval_dispatch` queue and
  idempotently schedules only `PENDING` children; each child then executes in
  its own River job.
- Parent status is aggregated from child counters
- No rollback of successful children when siblings fail

---

## 4. Worker Concurrency and Queue Safety

The current design does not let independent consumers scan pending child rows
directly. Initial approval and generic retry atomically insert one parent-keyed
`batch_approval_dispatch` River job. River uniqueness covers only runnable
states on that dedicated queue, so a completed/cancelled/discarded job does not
block an explicit later retry.

The dispatcher reloads the durable parent snapshot and schedules only children
whose current Ticket/Event state is safe. Each child transition and River
`InsertTx` is idempotent. Parent aggregation obtains `FOR UPDATE` on the parent
workflow ticket before re-reading children, then uses expected-state writes for
the parent Ticket and Event. A terminal mismatch cancels the dispatcher rather
than silently reopening children.

---

## 5. Parent Status Aggregation

Status derivation:

- `pending_count > 0` -> `IN_PROGRESS`
- `pending_count = 0 && failed_count = 0` -> `COMPLETED`
- all children cancelled -> `CANCELLED`
- at least one success plus any failed/cancelled child -> `PARTIAL_SUCCESS`
- other terminal combinations with no success -> `FAILED`

The public projection statuses above are distinct from the raw workflow parent
Ticket (`PENDING|EXECUTING|SUCCESS|FAILED|CANCELLED`) and Event
(`PENDING|PROCESSING|COMPLETED|FAILED|CANCELLED`). Persist aggregate fields on
`batch_tickets`; do not expose a public parent `APPROVED` state.

---

## 6. Two-Layer Rate Limiting

Layer 1 (Global protection):

- max global pending batch parents: `100`
- max global API requests: `1000/min`

Layer 2 (User fairness):

- max pending batch requests per user: `3`
- cooldown: `2 minutes`
- max pending child tickets per user: `30`

Implementation rules:

- Resolve exemption/override policy within the guarded submission transaction
- Derive the one-minute request window and current pending parent/child counts
  from durable workflow rows; there is no separately incremented counter row
- Use `429` with the integer-seconds `Retry-After` header and runtime error
  parameters. Every rejection includes `retry_after_seconds`, `contact_admin`,
  and `user_exempted`. Pending-parent rejection additionally includes
  `global_pending`, `user_pending`, and `max_user_pending`; the global-rate,
  pending-child, and cooldown branches include `reason`,
  `global_recent_submits`, `user_pending_children`, `user_cooldown_seconds`,
  `requested_child_count`, `max_global_per_minute`, and
  `max_user_pending_children`. Clients must not depend on the retired draft
  names `limit_type`, `current_value`, `max_value`, or `retry_after`.

---

## 7. Retry and Idempotency

Retry:

- `attempt_count` tracks logical dispatches, including the initial dispatch;
  River backoff/redelivery inside one dispatch does not increment it
- Retry endpoint conditionally requeues only execution-`FAILED` children and
  caps each child at three total logical attempts. `REJECTED` is an approval
  outcome and cannot be converted into execution by this endpoint.
- Generic retry inserts the dedicated dispatcher, resets the exact
  execution-`FAILED` ticket and a paired event in
  `PENDING|FAILED|CANCELLED`, reopens an already-approved parent/event,
  persists the normalized execution snapshot, and refreshes the projection in
  one transaction. An active parent dispatcher aborts every mutation.
- Retry is accepted only while the parent ticket/event remains
  `EXECUTING|FAILED` / `PROCESSING|FAILED`. Generic batches must retain a prior
  approver; ordinary VM operators cannot manufacture a new approval decision.
- Power ticket/event reset and replacement River insertion commit atomically;
  stale state, an active event, or an equivalent runnable job aborts the retry.
  This direct retry preserves the already accepted decision; it does not
  re-enter the approval path or manufacture a second approval decision.
- VM restart claims a durable `PENDING -> PROCESSING` dispatch fence and fails
  closed when a duplicate worker cannot prove whether the provider accepted the
  operation. Timeout, response loss, or post-dispatch persistence failure keeps
  the event/ticket `PROCESSING/EXECUTING`; the ordinary retry endpoint cannot
  reopen it. Power-operation conflicts set `operator_action_required=true` and
  return the read-only `operator-runbook:ambiguous-vm-restart` identifier. River
  terminal state cannot prove the provider request is no longer in flight, so
  no API can release the fence. Operators must drain the originating worker,
  preserve evidence, and inspect provider state without editing workflow rows
  or redispatching. Safe recovery requires a future provider receipt,
  idempotency, or provable-cancellation protocol.

Idempotency:

- `request_id` deduplicates repeated submit calls
- Duplicate request returns original `batch_id` and current parent status
- The key is scoped by requester and exact requested operation. Power `START`,
  `STOP`, and `RESTART` are distinct scopes, so the same opaque key may be used
  independently for those three intentional operations.
- The global/actor/request advisory locks, exact replay check, mutable quota
  reads, and batch persistence share one transaction for new-version writers.
- Because an older writer does not take those locks, deployment must drain old
  batch mutation paths and power workers before relying on the new guarantee.
  Any short overlap retains the prior duplicate-submit race but the attempt-only
  migration does not lose or rewrite request-ID data. See
  [database/migrations.md §Batch Retry and Idempotency Rollout](../database/migrations.md#batch-retry-and-idempotency-rollout).
- After successful explicit retry/cancel commit, the handler attempts a
  best-effort `vm.batch.retry` or `vm.batch.cancel` supplemental audit record.
  Audit failure is ignored and cannot roll back the committed mutation, so it
  is not durable actor attribution. Approval attribution is preserved
  separately: an ordinary retry keeps the original approver, while a
  reviewer-supplied replacement plan records that reviewer as approver.
- Cancel validates the durable parent/event identity and allowed state before
  entering the mutation. Its parent-keyed transaction commits exact `PENDING`
  child Ticket/Event cancellation and parent Ticket/Event/projection
  aggregation together. Parent aggregation uses a parent-row lock and
  expected-state writes; terminal parent/child outcome mismatches fail closed
  instead of allowing a finalizer to rewrite pending children under a terminal
  parent.

---

## 8. Error Model

Submission-time errors (request rejected, no ticket created):

- `400 INVALID_BATCH_OPERATION`
- `400 INVALID_BATCH_SIZE`
- `400 INVALID_BATCH_ITEM`
- `403 PERMISSION_DENIED`
- `429 BATCH_RATE_LIMITED`

Execution-time errors (child failed, parent may be partial success):

- child-specific error exposed as optional `last_error`
- parent summary remains queryable

---

## 9. Observability and Audit

Metrics:

- `batch_submit_total{batch_type,result}`
- `batch_child_execution_total{batch_type,status}`
- `batch_parent_duration_seconds`
- `rate_limit_rejections_total{limit_type}`

Audit posture:

- submit/retry/cancel handlers currently make best-effort supplemental audit
  calls after their durable write commits
- those calls cannot be used as a durable attribution guarantee; workflow
  requester/approver fields remain the authoritative persisted actors
- per-child execution completion/failure and admin exemption changes continue
  to follow the platform audit baseline

Sensitive data must follow ADR-0019 redaction rules.

---

## 10. Rollout Plan

Use the active
[Batch Retry and Idempotency Rollout](../database/migrations.md#batch-retry-and-idempotency-rollout)
runbook. It includes old-writer/worker drain order, historical operation-payload
integrity checks, dedicated dispatcher-state reconciliation, restart fences,
and rollback/forward-fix criteria. Do not substitute the historical five-step
plan for that runbook.

---

## 11. Accepted-ADR Batch-Limit Contract Debt

Accepted ADR-0015 specifies `10` items for create/delete and `50` for power.
The current public API and runtime accept up to `100` items and also implement
modify. That mismatch is unresolved contract debt, not ADR compliance. A
dedicated issue and a new accepted ADR/amendment must choose the intended
limits and align API, runtime, UI, tests, and documentation. This implementation
note does not amend the accepted ADR.

---

## Applied Changes (2026-02-06)

- `docs/design/phases/04-governance.md` updated to ADR-complete §5.6 model
- `docs/design/interaction-flows/master-flow.md` updated with parent-child + two-layer limit flows
- `docs/design/checklist/phase-4-checklist.md` updated with parent-child/rate-limit/frontend acceptance items
- `docs/design/examples/usecase/batch_approval.go` updated to parent-child atomic submission example

---

## Resolved Decisions

1. Batch submit and power compatibility endpoints (`POST /api/v1/vms/batch`, `POST /api/v1/vms/batch/power`) remain public and are normalized to the same parent-child pipeline.
2. Submission remains strictly atomic for parent/child ticket creation; invalid child inputs fail the whole submission transaction.
3. User-level throttling has admin exemption/override APIs. Per-request item
   limits remain the unresolved accepted-ADR contract debt described in §11.

---

## References (Best Practices)

- ADR baseline:
  - [ADR-0015 §19 Batch Operations](../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)
- Batch API behavior:
  - Google AIP-233 Batch Create: https://google.aip.dev/233
  - Google AIP-235 Batch Delete: https://google.aip.dev/235
  - Google AIP-151 Long-running Operations: https://google.aip.dev/151
  - Google AIP-155 Request ID/Idempotency: https://google.aip.dev/155
- HTTP rate limit signaling:
  - RFC 6585 (`429 Too Many Requests`): https://www.rfc-editor.org/rfc/rfc6585.html
  - RFC 9110 (`Retry-After`): https://www.rfc-editor.org/rfc/rfc9110
- Transaction coordination in PostgreSQL:
  - PostgreSQL advisory locks: https://www.postgresql.org/docs/18/explicit-locking.html#ADVISORY-LOCKS
  - PostgreSQL row locking (`SELECT ... FOR UPDATE`): https://www.postgresql.org/docs/18/sql-select.html
- Independent per-item failure handling analogy:
  - Kubernetes Jobs (`backoffLimitPerIndex`): https://kubernetes.io/docs/concepts/workloads/controllers/job/
