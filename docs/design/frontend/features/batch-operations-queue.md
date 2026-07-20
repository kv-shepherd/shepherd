# Batch Operations Queue UI (Parent-Child Model)

> **Flow Source**: [master-flow.md Stage 5.E](../../interaction-flows/master-flow.md#stage-5e-batch-operations)  
> **Backend Spec**: [04-governance.md §5.6](../../phases/04-governance.md#56-batch-operations-adr-0015-19)  
> **ADR Source**: [ADR-0015 §19](../../../adr/ADR-0015-governance-model-v2.md#19-batch-operations)

---

## 1. Objective

Define a frontend model that correctly visualizes and controls batch operations implemented as:

- one parent batch ticket
- many child operation tickets/jobs
- independent child execution
- parent aggregated status

This document is mandatory for approvals batch, VM batch create/delete, and batch power operations.

## 2. Parent/Child UI Model

### 2.0 End-to-End UI Storyboard

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Batch Queue UI Storyboard                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  Screen A: Resource List                                                                         │
│    - user selects targets                                                                        │
│    - chooses batch action                                                                        │
│    - clicks [Submit Batch]                                                                       │
│                                  │                                                               │
│                                  ▼                                                               │
│  Screen B: Queue List                                                                             │
│    - new parent row appears with `PENDING_APPROVAL`                                              │
│    - polling starts by `status_url`                                                              │
│    - parent counters update (total/success/failed/pending)                                       │
│                                  │                                                               │
│                                  ▼                                                               │
│  Screen C: Parent Row Expanded                                                                    │
│    - child detail table visible                                                                   │
│    - each child shows status + attempt_count + last_error                                        │
│                                  │                                                               │
│                                  ▼                                                               │
│  Screen D: Action States                                                                          │
│    - `IN_PROGRESS`: allow [Retry failed] when eligible and [Terminate pending]                  │
│    - `PARTIAL_SUCCESS` / `FAILED`: allow [Retry failed]                                         │
│    - `COMPLETED` / `CANCELLED`: disable mutating actions, keep [Export result] / [View details] │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Parent Row

The list endpoint exposes these parent-row fields:

- `id`
- `operation`
- `status` (`PENDING_APPROVAL`, `IN_PROGRESS`, `COMPLETED`,
  `PARTIAL_SUCCESS`, `FAILED`, `CANCELLED`)
- `child_count`, `success_count`, `failed_count`, `pending_count`
- `created_at`

The detail endpoint uses `batch_id` for the same parent and additionally exposes
`children`, `created_by`, and `updated_at`. The UI must not invent a public
`APPROVED` parent status or require list-only `batch_type`, `requester`, or
`updated_at` fields that are absent from the list schema.

### 2.2 Child Detail Panel

Child tasks are rendered via expandable panel/table, showing:

- `ticket_id`
- `event_id`
- optional `resource_id` and `resource_name`
- `status` (`PENDING`, `APPROVED`, `REJECTED`, `CANCELLED`, `EXECUTING`,
  `SUCCESS`, `FAILED`)
- `attempt_count`
- optional `last_error`
- optional `provisioning` detail

### 2.3 Actions

By parent status:

| Parent Status | Allowed Actions |
|---------------|-----------------|
| `PENDING_APPROVAL` | Refresh / Terminate pending children; approval decisions remain in the approvals UI |
| `IN_PROGRESS` | Refresh / Retry eligible failed children / Terminate remaining pending children |
| `PARTIAL_SUCCESS` | Retry failed children / Export result |
| `FAILED` | Retry failed children / Export result |
| `COMPLETED` | Export result |
| `CANCELLED` | View details only |

## 3. API Interaction Contract

### 3.1 Submit and Track

Submission endpoints return `202 Accepted` and include a status resource URL:

```json
{
  "batch_id": "BAT-20260206-001",
  "status": "PENDING_APPROVAL",
  "status_url": "/api/v1/vms/batch/BAT-20260206-001",
  "retry_after_seconds": 2
}
```

Frontend MUST treat `202` as "accepted for processing" and transition UI into tracking mode.

### 3.2 Polling Strategy

Use TanStack Query polling for parent and child status endpoints:

- Initial polling interval: use submission `retry_after_seconds` (`2s` in the
  current contract); fall back to `2s` if tracking starts from a copied URL
- Backoff on consecutive transient failures: exponential (max `30s`)
- Stop polling when parent reaches terminal status
- Resume polling immediately after user-triggered retry/terminate

### 3.3 Idempotency, Retry, and Fence Handling

- Generate one stable `request_id` for one intentional submission and reuse it
  across transport timeout, reconnect, and `5xx` recovery. Generate a new value
  only after the user intentionally starts a distinct batch.
- Treat `request_id` as opaque text. The client must not truncate it or infer a
  `varchar`/512-character server limit.
- Do not automatically repeat a batch mutation with a new key. First replay the
  original key or reload the known `status_url` so a response lost after commit
  resolves to the existing parent.
- Display `attempt_count` as the logical dispatch count. The initial dispatch is
  attempt 1 and the server permits at most three logical attempts per child;
  River's internal job redelivery does not increase this number.
- Enable `Retry failed` only for execution-`FAILED` children below the cap.
  `REJECTED` is an approval outcome and must not expose this execution action.
  The server remains authoritative because another actor or worker may win
  after the view was rendered.
- On `BATCH_RETRY_IN_PROGRESS`, `BATCH_NOTHING_TO_RETRY`,
  `BATCH_NOTHING_TO_CANCEL`, or `BATCH_RETRY_ATTEMPTS_EXHAUSTED`, keep the parent
  selection, refresh status, and explain the conflict instead of resubmitting.
- Never automatically retry an ambiguous VM restart mutation. Continue polling
  the child/parent state; when the conflict reports
  `operator_action_required=true`, surface its server-provided
  `reconciliation_path` runbook identifier to an authorized platform
  administrator. The UI must keep this guidance read-only, must never expose a
  fence-clear action, and must explain that a provider receipt/idempotency or
  provable-cancellation protocol is required before safe recovery. It must not
  turn operator guidance into a duplicate restart action.

### 3.4 Rate Limit Handling

When backend returns `429 Too Many Requests`:

- Read `Retry-After` header if present
- Disable resubmit/retry buttons until countdown ends
- Show clear user message and next allowed retry time
- Keep form/selection state intact

## 4. UX Requirements

### 4.1 Table Interaction

Use Ant Design `Table` with `rowSelection` for batch creation and action triggers.

- Preserve selected keys across pagination/filter changes
- Show selected count and current limit usage
- Block submit if selection exceeds operation limit

### 4.2 Status Presentation

- Parent status: Tag + progress summary (success/failed/pending counts)
- Child status: per-row icon/tag + last error tooltip
- Partial success: explicit warning banner, never silent

### 4.3 Retry and Terminate

- `Retry failed` only targets eligible execution-`FAILED` children with
  `attempt_count < 3`; approval-`REJECTED` children do not expose this action
- `Terminate` cancels only not-yet-started or pending children
- UI must always indicate which items were actually affected
- Concurrency conflicts must trigger a status refresh; optimistic UI must not
  leave a child in a fabricated pending/executing state

### 4.4 Result Export

- `Export result` downloads a JSON snapshot derived from `GET /api/v1/vms/batch/{batch_id}`
- Export is enabled for `COMPLETED`, `PARTIAL_SUCCESS`, and `FAILED`
- `CANCELLED` remains view-only because it does not represent an execution result set

## 5. Accessibility and Observability

### 5.1 Accessibility

- Dynamic status summary must be announced via `aria-live="polite"`
- Progress widgets should use proper `progressbar` semantics
- Buttons must expose disabled reasons in accessible text

### 5.2 Client Metrics

Track at least:

- batch submit latency
- status polling failures
- retry attempts by batch type
- terminate success ratio

## 6. Suggested React Query Pattern

```ts
function useBatchStatus(batchId: string, retryAfterSeconds = 2) {
  const pollIntervalMs = Math.max(1, retryAfterSeconds) * 1000;

  return useQuery({
    queryKey: ['batch-status', batchId],
    queryFn: ({ signal }) => api.getBatchStatus(batchId, { signal }),
    refetchInterval: (q) => {
      const status = q.state.data?.status;
      if (status && ['COMPLETED', 'FAILED', 'PARTIAL_SUCCESS', 'CANCELLED'].includes(status)) {
        return false;
      }
      return pollIntervalMs;
    },
    retry: 3,
    retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 30000),
  });
}
```

## 7. Cross-Document Consistency Rules

- Status enum definitions must stay synchronized with `master-flow.md` and OpenAPI.
- Parent/child counters shown in UI must come from backend aggregate fields, not local recomputation.
- Frontend should not infer completion from HTTP request success alone; only terminal parent status ends flow.
- Field-level three-state updates for administrator rate-limit overrides are
  outside this batch-queue contract. Omitted/`null`/numeric semantics remain
  deferred to a separate accepted ADR and must not be inferred here.

## 8. External Best-Practice References

- RFC 9110 `202 Accepted`: https://datatracker.ietf.org/doc/html/rfc9110#section-15.3.3
- RFC 6585 `429 Too Many Requests` + `Retry-After`: https://datatracker.ietf.org/doc/rfc6585/
- MDN `202 Accepted`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/202
- MDN `429 Too Many Requests`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/429
- TanStack Query retries and backoff: https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults
- TanStack Query cancellation via AbortSignal: https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation
- Ant Design Table selection patterns: https://ant.design/components/table/
- ARIA live regions (`status`): https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Guides/Live_regions
