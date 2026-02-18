# ADR-0034 Strict Check Profile (Master-Flow Full Coverage)

> Status: Active implementation profile (under ADR-0034 accepted scope)
> Related ADR: [ADR-0034](../../adr/ADR-0034-master-flow-spec-driven-test-first.md)
> Date: 2026-02-14

## Summary

This profile upgrades ADR-0034 execution from "critical-stage coverage" to
"full master-flow stage coverage" and sets stricter anti-shortcut rules.

Goal: prevent implementation drift where runtime code uses mock/empty behavior
while docs claim completion.

## Scope

In scope:

- All stage IDs in `docs/design/traceability/master-flow.json`.
- Backend interaction behavior tests and supporting CI gates.
- Stage-to-test traceability evidence quality.

Out of scope:

- UI visual regression details.
- Non-master-flow exploratory features.

## Strict Rules

1. **Full Stage Coverage**
   - `docs/design/traceability/master-flow-tests.json.required_stages` MUST
     include every stage ID in `master-flow.json`.
   - No "critical-only subset" mode on protected branches.

2. **No Deferred Debt for Required Stages**
   - `docs/design/ci/allowlists/master_flow_test_deferred.txt` stays empty.
   - Any deferred entry is treated as release blocker unless explicitly approved
     by a new ADR.

3. **Behavior Test First**
   - For each required stage, mapped tests MUST include executable behavior
     assertions (status code, state transition, payload/field constraints, or
     decision function outcomes).
   - Source-fragment checks may exist, but cannot be the only evidence.

4. **No Test Semantics Downgrade**
   - It is forbidden to "make tests pass" by weakening expectations when
     `master-flow.md` behavior is unchanged.
   - If expected behavior changes, update `master-flow.md` and traceability
     artifacts in the same change set.

5. **Runtime Anti-Shortcut Baseline**
   - Runtime must not wire `MockProvider`.
   - Runtime must not rely on placeholder/stub behavior.
   - Test suites must use PostgreSQL-backed storage only (no SQLite fallback).
   - Runtime code changes must carry corresponding test changes in the same
     delivery set (strict delta guard).
   - Existing checks in `docs/design/ci/scripts/` remain mandatory.

## Enforcement Mapping

- Stage matrix gate:
  - `docs/design/ci/scripts/check_master_flow_test_matrix.go`
- Runtime anti-shortcut gates:
  - `docs/design/ci/scripts/check_no_runtime_mock.go`
  - `docs/design/ci/scripts/check_provider_wiring.go`
  - `docs/design/ci/scripts/check_no_runtime_placeholders.go`
  - `docs/design/ci/scripts/check_no_sqlite_in_tests.go`
  - `docs/design/ci/scripts/check_changed_code_has_tests.sh`
- Full-completion claim gate (no deferred/exemption debt):
  - `docs/design/ci/scripts/check_master_flow_completion_readiness.go`
- Stage-specific behavior gates:
  - Stage 5.C/5.E and other explicit scripts in `docs/design/ci/scripts/`

## Implementation Plan

1. Expand required stages to full list.
2. Add missing stage behavior tests.
3. Fill implementation gaps exposed by tests.
4. Keep deferred allowlist empty.
5. Gate all changes through CI scripts + `go test` suites.
