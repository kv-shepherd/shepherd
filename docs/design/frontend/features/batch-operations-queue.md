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
│    - `IN_PROGRESS`: allow [Terminate pending]                                                    │
│    - `PARTIAL_SUCCESS` / `FAILED`: allow [Retry failed]                                         │
│    - `COMPLETED` / `CANCELLED`: disable mutating actions, keep [Export result] / [View details] │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Parent Row

Each parent row must display at least:

- `batch_id`
- `batch_type`
- `status` (`PENDING_APPROVAL`, `APPROVED`, `IN_PROGRESS`, `COMPLETED`, `PARTIAL_SUCCESS`, `FAILED`, `CANCELLED`)
- `child_count`, `success_count`, `failed_count`, `pending_count`
- `created_at`, `updated_at`
- `requester`

### 2.2 Child Detail Panel

Child tasks are rendered via expandable panel/table, showing:

- `ticket_id`
- target object (`vm_id`/`approval_ticket_id`)
- `status` (`PENDING`, `RUNNING`, `SUCCESS`, `FAILED`, `CANCELLED`)
- `attempt_count`
- `last_error_code`, `last_error_message`
- `last_attempt_at`

### 2.3 Actions

By parent status:

| Parent Status | Allowed Actions |
|---------------|-----------------|
| `PENDING_APPROVAL` | Approve / Reject |
| `APPROVED` | Refresh / View details (awaiting execution scheduling) |
| `IN_PROGRESS` | Refresh / Terminate remaining pending children |
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

- Initial polling interval: `2s`
- Backoff on consecutive transient failures: exponential (max `30s`)
- Stop polling when parent reaches terminal status
- Resume polling immediately after user-triggered retry/terminate

### 3.3 Rate Limit Handling

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

- `Retry failed` only targets failed children
- `Terminate` cancels only not-yet-started or pending children
- UI must always indicate which items were actually affected

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
const batchStatusQuery = useQuery({
  queryKey: ['batch-status', batchId],
  queryFn: ({ signal }) => api.getBatchStatus(batchId, { signal }),
  refetchInterval: (q) => {
    const status = q.state.data?.status;
    if (!status || ['COMPLETED', 'FAILED', 'PARTIAL_SUCCESS', 'CANCELLED'].includes(status)) {
      return false;
    }
    return 2000;
  },
  retry: 3,
  retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 30000),
});
```

## 7. Cross-Document Consistency Rules

- Status enum definitions must stay synchronized with `master-flow.md` and OpenAPI.
- Parent/child counters shown in UI must come from backend aggregate fields, not local recomputation.
- Frontend should not infer completion from HTTP request success alone; only terminal parent status ends flow.

## 8. External Best-Practice References

- RFC 9110 `202 Accepted`: https://datatracker.ietf.org/doc/html/rfc9110#section-15.3.3
- RFC 6585 `429 Too Many Requests` + `Retry-After`: https://datatracker.ietf.org/doc/rfc6585/
- MDN `202 Accepted`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/202
- MDN `429 Too Many Requests`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/429
- TanStack Query retries and backoff: https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults
- TanStack Query cancellation via AbortSignal: https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation
- Ant Design Table selection patterns: https://ant.design/components/table/
- ARIA live regions (`status`): https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Guides/Live_regions
