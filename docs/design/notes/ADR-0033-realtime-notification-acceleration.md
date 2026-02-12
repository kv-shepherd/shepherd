# Design Note: Realtime Notification Acceleration (LISTEN/NOTIFY + SSE Hint)

> Status: Proposed  
> Related ADR: [ADR-0033](../../adr/ADR-0033-realtime-notification-acceleration.md)  
> Owner: @jindyzhao  
> Date: 2026-02-12

## Summary

This note captures implementation impacts for the proposed ADR-0033 decision: add a non-authoritative realtime push acceleration path (PostgreSQL LISTEN/NOTIFY + SSE) while keeping existing polling as the canonical consistency mechanism.

## Scope

- In scope:
  - Add infrastructure-level event listener lifecycle for PostgreSQL notifications.
  - Add notification refresh hint stream endpoint (SSE-first).
  - Define fallback behavior: push lost/disconnected -> polling catches up.
  - Add governance checks to prevent UseCase/business layer from directly invoking push transport.
- Out of scope:
  - Replacing polling as source of truth.
  - Durable event replay, broker introduction (Redis/Kafka), and exactly-once semantics.
  - Token blacklist management APIs (V2 topic).

## Pending Changes (Not Yet Normative)

- Affected docs:
  - `docs/adr/ADR-0033-realtime-notification-acceleration.md` (decision and guardrails).
  - `docs/adr/README.md` (ADR index update).
  - `docs/design/traceability/master-flow.json` (Stage 5.F ADR mapping).
  - `docs/design/phases/04-governance.md` (notification section: add push-acceleration rollout notes).
  - `docs/design/checklist/phase-4-checklist.md` (new checklist item for listener lifecycle/reconnect and fallback validation).
  - `docs/design/CHECKLIST.md` (Stage 5.F progress notes update after implementation).
  - `docs/design/interaction-flows/master-flow.md` and `docs/i18n/zh-CN/design/interaction-flows/master-flow.md` (update only after ADR acceptance; proposed phase keeps normative flow text unchanged).
- Affected components:
  - `internal/infrastructure` (listener + reconnect loop + observability).
  - `internal/app/modules` (composition and lifecycle wiring).
  - `internal/api/handlers` (SSE endpoint).
  - `web/src` (subscribe to SSE hints + trigger canonical refetch).
- Behavior changes:
  - Online clients can receive lower-latency refresh hints.
  - Polling remains enabled as baseline and recovery path.

## Technical Integration Points

### 1. Database-Driven Emission (Coverage Guarantee)

- Source of truth for push hint emission: PostgreSQL trigger(s) on notification persistence tables (insert/read-state updates).
- Suggested migration location:
  - `internal/infrastructure/migrations/*_notification_realtime_notify.sql`
- Trigger payload principle:
  - Include only compact identifiers (`recipient_id`, `notification_id`, `event`, `version`).
  - Keep payload strictly bounded (application-side guard <= 1024 bytes; PostgreSQL default payload must be < 8000 bytes).
- Rationale:
  - Eliminates Ent/sqlc path drift for notification events by observing committed row changes directly at DB layer.

### 2. Listener Lifecycle and Bootstrap Integration

- Suggested listener component:
  - `internal/infrastructure/notification_listener.go`
- Suggested composition point:
  - initialize from module bootstrap path (`internal/app/modules/infrastructure.go`), wire start/stop from app lifecycle (`internal/app/bootstrap.go`).
- Runtime model:
  - pgx dedicated connection acquired from pool for `LISTEN`.
  - reconnect loop with bounded backoff and observability metrics.
- Concurrency constraint:
  - use worker pool (`SubmitDetached`) for loop orchestration; no naked goroutine in `internal/` (ADR-0031 + CI gate).

### 3. SSE Endpoint and Routing Model

- Suggested endpoint:
  - `GET /api/v1/notifications/stream`
- Handler location:
  - `internal/api/handlers/server_notification.go` (or dedicated stream handler file in same package).
- Routing/fan-out:
  - Maintain user-scoped connection registry keyed by authenticated `user_id`.
  - On incoming hint event, dispatch only to matching user bucket; multi-tab sessions fan-out within same user only.
  - Cross-user broadcast is forbidden for user-scoped events.

### 4. SSE Authentication Strategy (EventSource Constraints)

- Browser `EventSource` cannot set custom `Authorization` headers in standard mode.
- Recommended strategy:
  1. Authenticated REST call mints short-lived stream token (JWT-derived, TTL 30-60s, bound to `user_id` and session/token ID).
  2. Client opens `GET /api/v1/notifications/stream?st=<short_lived_token>`.
  3. Server validates `st`, binds stream context to resolved identity, and applies same authorization scope as REST.
- Explicitly forbidden:
  - trusting raw `user_id` from query string.
  - passing long-lived primary JWT directly in URL.

## Migration / Rollout

- One migration is required to add DB trigger/function for notification hint emission; no historical backfill is required.
- Feature-gated rollout:
  1. Backend listener + metrics only.
  2. SSE endpoint (internal testing).
  3. Frontend subscription with poll fallback.
  4. Gradual enablement in non-prod, then prod.

## Acceptance Gate Resolutions

1. **Emission Strategy**: Use DB trigger-based emission for notification hint channel by default; app-layer manual notify is not authoritative for this path.
2. **SSE Authentication**: Use short-lived stream token derived from authenticated session; do not use raw `user_id` as trust input.
3. **SLO Baseline (initial target)**:
   - p95 notify-to-dispatch latency <= 1s (same region, normal load)
   - reconnect recovery p95 <= 5s
   - functional correctness relies on polling fallback for missed hints (no exactly-once guarantee)
