# Design Notes

Design Notes document **proposed changes** that are not yet accepted as ADRs.
They allow teams to share implementation impact without changing normative specs.

## When to Use

Create a Design Note when:
- An ADR is **Proposed** and not yet accepted
- You need to communicate concrete changes to design or implementation
- You must avoid updating normative design docs until a decision is accepted

## Naming

Use one of:
- `ADR-XXXX-title.md` (recommended when tied to an ADR)
- `NOTE-YYYYMMDD-short-title.md` (for exploratory notes)

## Template (Minimal)

```
# Design Note: <Title>

> Status: Proposed
> Related ADR: ADR-XXXX
> Owner: @name
> Date: YYYY-MM-DD

## Summary
One paragraph summary of what is changing and why.

## Scope
- In scope: ...
- Out of scope: ...

## Pending Changes (Not Yet Normative)
- Affected docs: ...
- Affected components: ...
- Behavior changes: ...

## Migration / Rollout
- Data migration (if any)
- Compatibility notes

## Open Questions
- ...
```

## Lifecycle

1. **Proposed**: Design Note exists; no normative docs changed.
2. **Accepted**: ADR accepted; incorporate changes into design docs.
3. **Archived**: If ADR rejected or superseded.

## Pending Changes Blocks

If you must surface proposed changes inside a design doc, add a short block:

```
> ⚠ Pending Changes (Proposed, not yet accepted)
> - See docs/design/notes/ADR-XXXX-title.md
```

Keep it short and do not alter normative sections until acceptance.

