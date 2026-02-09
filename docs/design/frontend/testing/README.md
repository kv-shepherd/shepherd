# Frontend Testing Layer

> **Primary Guide**: [ADR-0020 testing note](../../notes/ADR-0020-frontend-testing-toolchain.md)

## Required Test Levels

- Unit/component tests for queue widgets, state badges, and action guards.
- Integration tests for status polling, retry submission, and cancellation pathways.
- E2E tests for parent-child batch task lifecycle with partial-success scenarios.

## Mandatory Scenarios for Batch UI

- Parent ticket transitions: `PENDING_APPROVAL -> IN_PROGRESS -> COMPLETED|PARTIAL_SUCCESS|FAILED`.
- Child task transitions with retries capped by backend policy.
- `429` responses display retry countdown and preserve unsent form state.
- Accessibility checks for live status updates (`aria-live`) and progress indicators.

## CI Expectations

- Coverage threshold gates remain mandatory (lines/functions/statements >= 80%, branches >= 75%).
- Frontend tests and API contract checks must run together before merge for queue-related changes.
