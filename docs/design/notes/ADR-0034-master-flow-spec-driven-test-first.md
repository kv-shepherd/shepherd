# ADR-0034 Implementation: Master-Flow Spec-Driven Test-First

> **Parent ADR**: [ADR-0034](../../adr/ADR-0034-master-flow-spec-driven-test-first.md)  
> **Status**: Implementation specification draft for ADR-0034 (parent ADR status: **accepted**)  
> **Date**: 2026-02-14

---

## Summary

This note defines the concrete implementation artifacts for enforcing spec-driven, test-first delivery using `master-flow.md` as the canonical behavior source.

Core implementation = **stage test matrix + deferred allowlist + blocking CI gate**.
Strict profile now targets **full master-flow stage coverage**.

---

## Scope

In scope:

* Machine-readable stage-to-test mapping.
* CI blocking checks for required-stage coverage/deferred debt hygiene.
* Full stage mapping across all stage IDs in `master-flow.json`.

Out of scope:

* Auto-generating all E2E/API tests from `master-flow.md` in this change.
* Replacing existing test gates (`check_critical_test_presence`, `check_stage5c_behavior_tests`, etc.).

---

## Delivered Artifacts

### 1. Stage Test Matrix

* `docs/design/traceability/master-flow-tests.json`

Schema (v1):

* `version`
* `master_flow`
* `required_stages[]`
* `stages[]`:
  - `id`
  - `tests[]` (must be executable test files)

### 2. Deferred Stage Allowlist

* `docs/design/ci/allowlists/master_flow_test_deferred.txt`

Rules:

* Only required stages may appear.
* Entry must be removed once stage has executable test mapping.
* Stale entries are CI failures.

### 3. Blocking Gate

* `docs/design/ci/scripts/check_master_flow_test_matrix.go`

Validation behavior:

* Required stage IDs exist in `master-flow.md`.
* Stage mapping IDs are valid and unique.
* Test paths exist and are executable tests:
  - Go: `_test.go` with `func TestXxx(t *testing.T)`
  - TS/JS: contains `test(` / `test.describe(` / `it(`
* Required stage coverage = mapped OR deferred.
* Deferred stale/invalid entries fail CI.

### 4. Coverage Baseline (Strict Profile)

`required_stages` is aligned to all stage IDs currently declared in
`docs/design/traceability/master-flow.json` (24 stage IDs).

### 5. Coverage Depth Reinforcement (2026-02-14)

To reduce "source-fragment only" evidence risk, behavior-level tests were added:

- Stage 4.A / 4.B / 4.C:
  - `internal/api/handlers/server_system_behavior_test.go`
  - Covers visibility filtering and update flow runtime behavior.
- Stage 2.E / Stage 5.B:
  - `internal/governance/approval/gateway_behavior_test.go`
  - Covers approve/reject/cancel state transition behavior and atomic-writer handoff.

---

## Rollout Plan

1. Add matrix + deferred file + gate script.
2. Wire gate into CI required checks.
3. Add/expand tests stage-by-stage and remove deferred entries.
4. Keep checklist progress synchronized with matrix evidence.

---

## Acceptance Criteria

* `go run docs/design/ci/scripts/check_master_flow_test_matrix.go` passes.
* CI workflow includes the new required step.
* All stages in `master-flow.json` have executable test evidence in matrix.
* Deferred allowlist remains empty.
* `docs/design/ci/README.md` documents the new gate.
