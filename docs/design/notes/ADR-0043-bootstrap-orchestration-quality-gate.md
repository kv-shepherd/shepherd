# Design Note: ADR-0043 — Bootstrap Composition-Root Quality Gate

> **Status**: Proposed (ADR-0043 under review until 2026-03-13)  
> **Related ADR**: [ADR-0043](../../adr/ADR-0043-bootstrap-orchestration-quality-gate.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-11  
> **Last Updated**: 2026-03-11

## Summary

ADR-0043 proposes a clarification to the `bootstrap.go` quality gate inherited
from ADR-0022: the meaningful constraint is that `Bootstrap()` stays
orchestration-only and concise, not that the entire file must remain under a
mechanical total-line threshold.

This note captures the pending implementation-facing interpretation while the
ADR is still in review.

## Scope

- In scope: composition-root review criteria for `internal/app/bootstrap.go`
- In scope: checklist language for orchestration-only validation
- In scope: allowing comments and small helper functions in the same file
- Out of scope: changing ADR-0013 manual DI rules
- Out of scope: changing module boundaries introduced by ADR-0022
- Out of scope: introducing new automated complexity tooling

## Pending Clarification

Until ADR-0043 is accepted, reviewers should interpret the `bootstrap.go`
quality gate as follows:

1. `Bootstrap()` should only assemble infrastructure, modules, handlers, and
   workers.
2. `Bootstrap()` should not contain domain rules, persistence logic, or request
   behavior.
3. Small local helpers and explanatory comments are allowed when they improve
   readability.
4. Total file line count is a weak signal, not a pass/fail rule by itself.

## Affected Documentation

- `docs/design/CHECKLIST.md`
- `docs/design/checklist/phase-0-checklist.md`
- `docs/design/checklist/phase-5-checklist.md`
- `docs/design/phases/05-auth-api-frontend.md`

## Revisit Conditions

- ADR-0043 is accepted, rejected, or superseded
- the project introduces an automated function-level complexity/shape gate
- `bootstrap.go` begins to accumulate service logic again

