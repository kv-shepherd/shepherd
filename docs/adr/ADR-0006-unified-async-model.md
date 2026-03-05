# ADR-0006: Unified Async Model

> **Status**: Accepted  
> **Date**: 2026-01-14  
> **Updates**:
>   - 2026-01-15: Use River Queue instead of Asynq, prohibit self-built Outbox Worker
>   - 2026-01-15: Unified adoption of **EventID pattern**
>   - 2026-01-15: Adopt **external state management** for approval mechanism
>   - 2026-01-16: **Transaction strategy switched to ADR-0012** (Ent + sqlc atomic transactions)

---

## Key Decisions (Required)

> **River Queue is the only task queue implementation**
>
> | ✅ Use | ❌ Prohibited |
> |--------|--------------|
> | `github.com/riverqueue/river` | Self-built Outbox Worker + FOR UPDATE SKIP LOCKED |
> | River Job definitions | Self-built TaskHandler interface |
> | River Client.Insert() | Self-built Repository.ClaimTask() |
> | River Worker consumption | Self-built Worker.Run() loop |
>
> **Reason**:
> - River is a production-grade PostgreSQL task queue with distributed lock, retry, dead letter handling
> - Self-built Outbox Worker reimplements existing River features, increasing maintenance burden
> - River uses the same FOR UPDATE SKIP LOCKED mechanism, equivalent performance

---

## Context

Architecture review found the system has two change paths:

1. **Direct flow**: `CreateVMUseCase` → `ExecuteK8sCreate` (synchronous blocking HTTP)
2. **Approval flow**: `ApprovalTicket` → `River Job` → `Worker` (asynchronous)

### Problem Analysis

| Issue | Direct Flow | Approval Flow |
|-------|-------------|---------------|
| Pod restart | ❌ In-memory semaphore lost, queued requests lost | ✅ Tasks persist in DB |
| HTTP timeout | ❌ Blocks waiting for K8s response (30s+) | ✅ Returns immediately |
| Traceability | ❌ No unified task ID | ✅ Outbox task_id |
| Peak shaving | ❌ Depends on in-memory semaphore | ✅ Worker rate-limited consumption |

**Conclusion**: Direct flow violates the "governance platform is not high-concurrency scheduling" positioning and has single point of failure risk.

---

## Decision

### Adopt: All change operations use PostgreSQL task queue (River)

> **Use River Queue instead of Redis/Asynq**:
> - ✅ **Transaction consistency**: Task insertion and business data write in same DB transaction
> - ✅ **Fewer components**: No Redis dependency, only PostgreSQL
> - ✅ **SKIP LOCKED**: River leverages PostgreSQL's `FOR UPDATE SKIP LOCKED` for high-performance queue
> - ✅ **Native observability**: Task state stored directly in PostgreSQL, queryable via SQL

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Unified Async Model (River Queue)                 │
│                                                                      │
│  All change APIs (POST/PUT/DELETE) → River Job → Return 202 Accepted│
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  Operations requiring approval → River Job (after approval)  │    │
│  │  Operations not requiring approval → River Job (immediate)   │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  River Worker unified consumption:                                   │
│  - available → Execute K8s operation directly                        │
│  - approved → Execute K8s operation after approval                   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## API Response Standards

**Change operations** (POST/PUT/DELETE):

```json
HTTP/1.1 202 Accepted
Location: /api/v1/tasks/task_abc123

{
  "task_id": "task_abc123",
  "status": "PENDING",
  "message": "Task accepted, processing asynchronously",
  "links": {
    "self": "/api/v1/tasks/task_abc123",
    "cancel": "/api/v1/tasks/task_abc123/cancel"
  }
}
```

**Task status query**:

```json
GET /api/v1/tasks/task_abc123

{
  "task_id": "task_abc123",
  "status": "COMPLETED",
  "result": { ... },
  "error": null,
  "created_at": "2026-01-14T10:00:00Z",
  "completed_at": "2026-01-14T10:00:05Z"
}
```

---

## River Job State Machine

River built-in states mapping to business semantics:

| River State | Business Meaning | Description |
|-------------|------------------|-------------|
| `available` | Ready to execute | River Worker can consume |
| `pending` | Queued | Not yet scheduled |
| `running` | Executing | Worker processing |
| `completed` | Success | Terminal state |
| `retryable` | Failed, retry pending | Under max_retries |
| `discarded` | Dead letter | Exceeded retries, terminal |
| `cancelled` | Cancelled | User cancelled, terminal |
| `scheduled` | Scheduled task | Waiting for execution time |

**Approval state implementation**:
- River does **not** have built-in `PENDING_APPROVAL` / `APPROVED` / `REJECTED` states
- Approval logic implemented via **DomainEvent table** + **ApprovalTicket table**
- 🚨 **Do not insert River Job before approval**, insert `EventJobArgs` only after approval passes

---

## Dry Run Exception

> **Dry run is not subject to async restriction**
>
> K8s dry run is a **read operation** (validating Admission Controllers), can be called synchronously in Handler layer:
>
> | Operation Type | Handling | Description |
> |----------------|----------|-------------|
> | `POST /vms` | Async (River Job) | Actually creates resource |
> | `POST /vms/dry-run` | Sync | Validates only, no resource created |
> | `DELETE /vms/:id` | Async (River Job) | Actually deletes resource |

---

## Notification Exception

> **V1 In-app notifications are synchronous writes**
>
> Platform-internal notifications (inserted into `notifications` table) are **synchronous** writes within the same DB transaction as business operations. This is an intentional design decision:
>
> | Channel | V1 Handling | Rationale |
> |---------|-------------|-----------|
> | **In-app inbox** | Sync (same transaction) | Ensures atomicity with business operation; no external I/O overhead |
> | **V2+ External channels** (Email/Webhook/Slack) | Async (River Job) | External I/O requires fault tolerance and retry semantics |
>
> **Why synchronous notifications don't violate ADR-0006**:
> - ADR-0006's async mandate addresses **K8s API calls** and **external I/O** that may timeout or fail
> - In-app notifications are **internal DB writes** with negligible latency
> - Same-transaction semantics ensure "business success = notification visible" atomicity
>
> See [04-governance.md §6.3](../design/phases/04-governance.md#63-notification-system-adr-0015-20) for implementation details.

## Stability Prerequisites

All-in-PG strategy **must** be combined with the following measures for stability:

- **River built-in cleanup**: `CompletedJobRetentionPeriod=24h` auto-cleans expired tasks
- **Aggressive Autovacuum tuning**: 1% dead tuples triggers cleanup (`scale_factor=0.01`)
- **Worker concurrency limiting**: MaxWorkers ≤ 10

See [ADR-0008](./ADR-0008-postgresql-stability.md) for details.

---

## Transaction Consistency Strategy

> **Important Update**: Adopt **Ent + sqlc hybrid atomic transaction** strategy for true ACID atomicity.
>
> | Component | Purpose |
> |-----------|---------|
> | Ent ORM | 99% of read/write operations |
> | sqlc | Core write transactions (DomainEvent + River InsertTx atomic commit) |
> | pgxpool | Shared connection pool, reused by Ent/River/sqlc |

See [ADR-0012](./ADR-0012-hybrid-transaction.md) for details.

---

## Consequences

### Positive

- ✅ No request loss on Pod restart
- ✅ HTTP response time < 200ms
- ✅ Unified task tracking
- ✅ Native PostgreSQL observability

### Negative

- 🟡 All writes become async, UX needs adjustment
- 🟡 Frontend needs polling/WebSocket for status updates

### Mitigation

- Provide real-time status via WebSocket
- Dry run for synchronous pre-validation

---

## References

- [River Queue Documentation](https://riverqueue.com/docs)
- [ADR-0008: PostgreSQL Stability](./ADR-0008-postgresql-stability.md)
- [ADR-0012: Hybrid Transaction Strategy](./ADR-0012-hybrid-transaction.md)

---

## Scope Clarification Addendum (2026-02-14) {#adr-0006-scope-clarification-2026-02-14}

> **Type**: Clarification addendum (no change to original decision intent).

To avoid ambiguity from shorthand phrases like "all writes async", this ADR scope is clarified as:

- River is mandatory for writes that coordinate **external side effects** (for example Kubernetes/provider calls, external channels).
- Pure PostgreSQL writes that require transaction-level atomicity (for example audit/event/notification persistence in the same business transaction) may remain synchronous.
- This clarification aligns with this ADR's existing notification exception and with [ADR-0012](./ADR-0012-hybrid-transaction.md).

---

## DryRun Pre-flight Gate Addendum (2026-03-03) {#adr-0006-dryrun-preflight-gate-2026-03-03}

> **Type**: Implementation scope addendum (extends the §Dry Run Exception section above).  
> **Status**: Implemented in `internal/governance/approval/gateway.go`.

### Problem

The §Dry Run Exception section defines `POST /vms/dry-run` as a synchronous endpoint.
However, it does not address when DryRun validation should occur in the **approval path**
(`POST /vms` → River Job → `approveCreate()`).

Before this addendum, K8s Apply errors in the approved path were only discovered inside the River Worker,
after the approval ticket had already been accepted. This produced a confusing UX:
"Approved successfully" → cloud job silently fails minutes later.

### Decision

> All change operations that enter the River queue via the **approval gateway** must execute a
> `DryRunApply()` Pre-flight Gate **before enqueuing the River Job**.
>
> Gate failure (the K8s server rejects the rendered spec) must return a synchronous error (4xx),
> not enter the queue. Gate success is required before the ticket transitions to `APPROVED`.

**Implementation location**: `gateway.go` → method `approveCreate()`.

The gate is optional at construction time: if `VMService` is not injected (`nil`), the gate is silently skipped
(backward-compatible for unit-test installs that do not have a K8s cluster).

### Why This Does Not Require a New ADR

- The architectural principle (`DryRunApply()` as synchronous pre-validation) is already established in
  ADR-0011 §DryRunApply and the §Dry Run Exception section of this ADR.
- The change is a **scope boundary clarification**: extending where the existing mechanism is applied,
  not introducing a new mechanism.
- Gate failure semantics follow the existing 4xx convention for synchronous validation errors.

### Implementation Evidence

| File | Change |
|------|--------|
| `internal/governance/approval/gateway.go` | `vmService` field, `SetVMService()` setter, DryRun Gate logic, `buildDryRunSpec()` |
| `internal/app/modules/approval.go` | Updated `NewApprovalModule(infra, vmSvc)` signature |
| `internal/app/modules/vm.go` | Added `VMService()` accessor |
| `internal/app/bootstrap.go` | Wired `SetVMService()` in application startup |
| `internal/governance/approval/gateway_behavior_test.go` | 13 DryRun Gate-specific test cases |
