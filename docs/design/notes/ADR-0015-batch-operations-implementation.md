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

- Keep `POST /api/v1/approvals/batch` and `POST /api/v1/vms/batch/power`
- Internally normalize them into the same parent-child ticket pipeline

Request requirements:

- Add optional `request_id` (UUID) for idempotency
- Enforce max batch size by operation type (ADR default values)

Response model:

- Submit returns `202 Accepted` with `batch_id` and `status_url`
- Status returns counts and per-child states

---

## 2. Data Model

Use explicit parent table plus child rows:

- `batch_approval_tickets`
  - `ticket_id`, `batch_type`, `child_count`, `success_count`, `failed_count`, `pending_count`, `status`, `reason`, `created_by`, `created_at`, `updated_at`
- `approval_tickets` (child)
  - add `parent_ticket_id`, `sequence_no`, `status`, `error_message`, `attempt_count`, `last_attempt_at`

Rate limit tables (ADR-0015 §19):

- `rate_limit_counter`
  - `scope` (`global` or `user`)
  - `subject_id` (`global` or user id)
  - `limit_type`
  - `window_start`, `current_value`
- `rate_limit_exemption`
  - `user_id`, `exempted_by`, `reason`, `expires_at`

Recommended indexes:

- `batch_approval_tickets(status, created_at)`
- `approval_tickets(parent_ticket_id, status, sequence_no)`
- unique idempotency key: `(created_by, batch_type, request_id)` where `request_id` is not null
- `rate_limit_counter(scope, subject_id, limit_type, window_start)` unique

---

## 3. Atomicity Boundary

Creation phase (`single SQL transaction`):

1. Validate limits (global and user)
2. Validate all child requests
3. Insert parent ticket
4. Insert all child tickets
5. Insert audit log
6. Commit once

If any child insert/validation fails, rollback entire submission.

Execution phase (`independent`):

- Each child ticket processed by its own River job
- Parent status is aggregated from child counters
- No rollback of successful children when siblings fail

---

## 4. Worker Concurrency and Queue Safety

For workers pulling child tasks, use row-level locking with skip semantics:

```sql
SELECT id
FROM approval_tickets
WHERE parent_ticket_id = $1
  AND status = 'PENDING'
ORDER BY sequence_no
FOR UPDATE SKIP LOCKED
LIMIT $2;
```

Notes:

- `SKIP LOCKED` is appropriate for queue-like multi-consumer processing.
- Child completion updates parent counters with atomic increment/decrement SQL.

---

## 5. Parent Status Aggregation

Status derivation:

- `pending_count > 0` -> `IN_PROGRESS`
- `pending_count = 0 && failed_count = 0` -> `COMPLETED`
- `pending_count = 0 && success_count = 0` -> `FAILED`
- otherwise -> `PARTIAL_SUCCESS`

Persist aggregate fields on parent row to avoid expensive full scans.

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

- Exemption check first
- All counters updated in SQL transaction with deterministic window key
- Use `429` with `Retry-After` and error body fields:
  - `limit_type`, `current_value`, `max_value`, `retry_after`, `contact_admin`

---

## 7. Retry and Idempotency

Retry:

- Child retries follow River backoff policy
- Retry endpoint only requeues `FAILED` children
- Track `attempt_count` and cap retries per child

Idempotency:

- `request_id` deduplicates repeated submit calls
- Duplicate request returns original `batch_id` and current parent status

---

## 8. Error Model

Submission-time errors (request rejected, no ticket created):

- `400 INVALID_BATCH_REQUEST`
- `400 BATCH_SIZE_EXCEEDED`
- `403 PERMISSION_DENIED`
- `429 RATE_LIMIT_EXCEEDED`

Execution-time errors (child failed, parent may be partial success):

- child-specific app error in `error_message`
- parent summary remains queryable

---

## 9. Observability and Audit

Metrics:

- `batch_submit_total{batch_type,result}`
- `batch_child_execution_total{batch_type,status}`
- `batch_parent_duration_seconds`
- `rate_limit_rejections_total{limit_type}`

Audit requirements:

- submission event
- per-child execution completion/failure
- admin exemption changes

Sensitive data must follow ADR-0019 redaction rules.

---

## 10. Rollout Plan

1. Add schema and migrations for parent-child + rate limit tables
2. Implement submit/status/retry service layer with atomic creation
3. Migrate existing batch endpoints to unified parent-child internals
4. Add CI checks and integration tests:
   - atomic creation rollback test
   - partial success aggregation test
   - limit enforcement and `Retry-After` behavior
5. Apply normative doc updates and re-run design governance checks

---

## Applied Changes (2026-02-06)

- `docs/design/phases/04-governance.md` updated to ADR-complete §5.6 model
- `docs/design/interaction-flows/master-flow.md` updated with parent-child + two-layer limit flows
- `docs/design/checklist/phase-4-checklist.md` updated with parent-child/rate-limit/frontend acceptance items
- `docs/design/examples/usecase/batch_approval.go` updated to parent-child atomic submission example

---

## Resolved Decisions

1. Compatibility endpoints (`POST /api/v1/approvals/batch`, `POST /api/v1/vms/batch/power`) remain public but are normalized to the same parent-child pipeline.
2. Submission remains strictly atomic for parent/child ticket creation; invalid child inputs fail the whole submission transaction.
3. User-level limits use ADR defaults, with admin exemption/override APIs for exceptional cases.

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
- Queue concurrency in PostgreSQL:
  - PostgreSQL `SELECT ... FOR UPDATE ... SKIP LOCKED`: https://www.postgresql.org/docs/18/sql-select.html
- Independent per-item failure handling analogy:
  - Kubernetes Jobs (`backoffLimitPerIndex`): https://kubernetes.io/docs/concepts/workloads/controllers/job/
