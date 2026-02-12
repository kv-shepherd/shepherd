---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-12
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0033: Realtime Notification Acceleration with PostgreSQL LISTEN/NOTIFY

> **Review Period**: Until 2026-02-14 (48-hour minimum)  
> **Discussion**: [Issue #204](https://github.com/kv-shepherd/shepherd/issues/204)  
> **Related**: [ADR-0006](./ADR-0006-unified-async-model.md), [ADR-0012](./ADR-0012-hybrid-transaction.md), [ADR-0015 §20](./ADR-0015-governance-model-v2.md#20-notification-system), [ADR-0031](./ADR-0031-concurrency-and-worker-pool-standard.md)

---

## Context and Problem Statement

Current V1 notification UX is poll-based by design: frontend reads unread count and list via periodic API calls, and this behavior is part of the canonical interaction truth.

The team wants lower-latency UI feedback without introducing a mandatory external broker or violating existing transaction and governance constraints.

We need a solution that improves perceived realtime behavior while preserving:

* database-as-truth consistency
* non-invasive business layering (UseCase purity)
* existing V1 poll fallback semantics

## Decision Drivers

* Preserve `master-flow` V1 contract: poll-based notification remains valid baseline.
* Avoid dual-write inconsistency between business state and push events.
* Keep PostgreSQL-only operations (no mandatory Redis/Kafka for V1/V1.5).
* Respect ADR-0012 transaction boundaries and ADR-0031 concurrency rules.
* Ensure changes remain auditable through ADR/design governance.

## Feasibility Assessment (Current Codebase)

### Confirmed Fit

* Notification persistence is already DB-backed and decoupled via `NotificationSender`.
* Frontend already tolerates eventual consistency through polling.
* PostgreSQL is a required system dependency; LISTEN/NOTIFY introduces no new infra class.

### Confirmed Risks

* LISTEN/NOTIFY is best-effort delivery for online listeners, not durable replay.
* App-layer-only emission points can drift when write paths differ (Ent/sqlc), causing silent coverage gaps.
* Browser SSE clients cannot set arbitrary auth headers natively; auth inheritance must be designed explicitly.
* Any direct goroutine-based listener implementation in application code must comply with ADR-0031 and existing CI gates.

### Practical Conclusion

The proposal is feasible only as a **non-authoritative acceleration layer** over existing polling, not as a replacement event bus.

## Considered Options

* **Option 1**: Keep V1 polling only (no push path).
* **Option 2**: Ent Hooks + LISTEN/NOTIFY as the sole realtime event bus.
* **Option 3**: Hybrid acceleration path: database-driven push hint + polling fallback (non-authoritative push).

## Decision Outcome

**Chosen option**: "Option 3", because it improves UX latency while preserving the current truth model and governance constraints.

### Normative Decisions

1. **Authoritative source is unchanged**
   - Business truth remains persisted PostgreSQL state and existing REST query APIs.
   - Realtime push payload is a hint for UI refresh, not a state authority.

2. **Push channel is additive**
   - Keep V1 poll loop as mandatory fallback and consistency recovery path.
   - Clients receiving push events MUST still re-fetch canonical REST data before rendering final state.

3. **Emission mechanism rule (notification domain)**
   - For notification acceleration, emission MUST be database-driven via PostgreSQL trigger(s) on notification state tables.
   - Application code MUST NOT rely on scattered manual `NOTIFY` calls for core notification fan-out correctness.
   - Rationale: DB-trigger emission observes committed row changes independently of Ent/sqlc call path and avoids coverage drift.

4. **Transaction safety rule**
   - Realtime emission must occur after commit visibility and must not break primary transaction success path.
   - Emission failures must be observable but must not roll back committed business operations.

5. **Layering rule**
   - UseCase layer MUST NOT call WebSocket/SSE push APIs directly.
   - Realtime transport belongs to infrastructure/module composition boundaries.

6. **Protocol rollout preference**
   - Prefer SSE first for one-way server-to-client updates in notification scenarios.
   - WebSocket remains optional for later bidirectional scenarios.

7. **Recipient-scoped fan-out/routing rule**
   - Listener payload MUST include a routable recipient identity key (for example, `recipient_id`) and event identifiers.
   - Stream hub routing key is authenticated user identity; server dispatches only to matching user channels/sessions.
   - Broadcast of user-scoped notification hints to all connected clients is forbidden.

8. **Payload governance rule**
   - `NOTIFY` payload MUST remain ID-only and compact (event type + identifiers + version), and MUST NOT embed full message body or sensitive fields.
   - Enforce a defensive payload ceiling in application code (for example, reject or truncate at 1024 bytes) to stay well below PostgreSQL's <8000-byte default limit.
   - If additional data is required, persist it in DB and send only lookup keys in notify payload.

9. **SSE authentication inheritance rule**
   - SSE subscription MUST inherit authenticated identity equivalently to existing JWT-protected REST APIs.
   - Passing raw `user_id` as a trust signal in query parameters is forbidden.
   - For browser `EventSource` constraints, prefer a short-lived signed stream token minted from an authenticated API call.

10. **ADR-0031 execution rule**
   - Listener loops, reconnect loops, and stream fan-out tasks under `internal/` MUST use project worker pool APIs.
   - Naked `go` statements remain forbidden and enforced by CI (`check_naked_goroutine.go`).

### Consequences

* ✅ Good, because UX latency can be reduced without changing domain consistency model.
* ✅ Good, because polling fallback keeps behavior stable under disconnections and dropped push events.
* ✅ Good, because the approach avoids mandatory new infra dependencies.
* ✅ Good, because recipient-scoped routing avoids cross-user leakage risks in push delivery.
* 🟡 Neutral, because implementation still needs operational handling (listener lifecycle/reconnect/monitoring).
* ❌ Bad, because push is not exactly-once and not replayable by itself (mitigated by canonical polling fallback).

### Confirmation

* Architecture review confirms `master-flow` poll truth is preserved.
* Architecture review confirms recipient-scoped fan-out routing model is implemented (no global broadcast for user-scoped events).
* Code review/CI confirms no UseCase-level manual push calls.
* Code review confirms DB-trigger-based emission exists for notification acceleration path and is covered by migration/test artifacts.
* Integration tests cover:
  - push received -> client refetch -> UI update
  - push dropped/disconnected -> polling catches up
  - listener reconnect behavior
  - cross-user isolation (user A must not receive user B hint events)
* Load tests validate that listener path does not affect write-path SLO.

---

## Pros and Cons of the Options

### Option 1: Polling only

* ✅ Good, because simplest and already implemented.
* ✅ Good, because no additional moving parts.
* ❌ Bad, because UX freshness is bounded by polling interval.

### Option 2: Ent Hooks as sole event source

* ✅ Good, because appears low-intrusion at first glance.
* ❌ Bad, because coverage can drift when writes bypass Ent (sqlc paths exist in project).
* ❌ Bad, because easy to overestimate reliability of LISTEN/NOTIFY and under-spec fallback behavior.

### Option 3: Push acceleration + polling fallback

* ✅ Good, because balances UX and reliability with current architecture.
* ✅ Good, because aligns with existing poll-based product truth.
* 🟡 Neutral, because requires explicit coverage map for emission points.
* ❌ Bad, because adds infrastructure complexity compared with polling-only baseline.

---

## More Information

### Scope Boundary

This ADR defines architectural boundary and rollout principles only.

It does not finalize:

* full versioned channel payload schema evolution policy
* final SSE/WebSocket endpoint contracts
* implementation milestone and SLA targets

These details should be captured in phase docs after ADR acceptance.

### Related Decisions

* [ADR-0006](./ADR-0006-unified-async-model.md) - async model scope and local DB-write clarification
* [ADR-0012](./ADR-0012-hybrid-transaction.md) - transaction boundary constraints
* [ADR-0015 §20](./ADR-0015-governance-model-v2.md#20-notification-system) - notification system baseline
* [ADR-0031](./ADR-0031-concurrency-and-worker-pool-standard.md) - concurrency enforcement

### References

* PostgreSQL LISTEN/NOTIFY (transaction and queue semantics): https://www.postgresql.org/docs/current/sql-notify.html
* PostgreSQL LISTEN semantics: https://www.postgresql.org/docs/current/sql-listen.html
* pgx LISTEN/NOTIFY usage: https://github.com/jackc/pgx
* Ent hooks and transaction hooks: https://entgo.io/docs/hooks
* Design Note (pending implementation details): [docs/design/notes/ADR-0033-realtime-notification-acceleration.md](../design/notes/ADR-0033-realtime-notification-acceleration.md)

### Implementation Notes

* Before acceptance, keep current V1 docs unchanged as canonical runtime behavior.
* After acceptance, update `master-flow` / phase docs and traceability manifest in one atomic docs change.
* CI guard proposal (post-acceptance): add a static check that non-infrastructure packages do not import realtime transport packages directly.
* CI guard proposal (post-acceptance): add focused checks for SSE auth inheritance and recipient-isolated routing tests.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-12 | @jindyzhao | Initial draft |
| 2026-02-12 | @jindyzhao | Added routing/fan-out rules, DB-trigger emission requirement, payload governance, and ADR-0031 execution constraints |
