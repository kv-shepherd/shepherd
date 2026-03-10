# Design Note: ADR-0041 — Power Operation Approval Requirement Service

> **Status**: Active (ADR-0041 accepted 2026-03-10)  
> **Related ADR**: [ADR-0041](../../adr/ADR-0041-power-operation-approval-requirement-service.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-06  
> **Last Updated**: 2026-03-10

## Summary

ADR-0041 establishes a dedicated `ApprovalRequirementService` so VM
start/stop/restart requests can be evaluated against the accepted `test` / `prod`
approval matrix before any power job is enqueued. This note captures the
accepted schema, handler flow, and current implementation shape.

## Scope

- In scope: `ApprovalPolicy` schema reshape around environment type and
  operation
- In scope: `ApprovalTicket.operation_type` addition for `POWER`
- In scope: handler flow changes for VM start, stop, and restart
- In scope: reuse of the same policy evaluation path for VNC approval rules
- Out of scope: approval backend provider selection
- Out of scope: external approval provider implementation details
- Out of scope: namespace-name regex routing

## Accepted Scope

- Affected docs:
  - `docs/adr/ADR-0041-power-operation-approval-requirement-service.md`
  - governance design docs that describe approval-rule evaluation
- Affected components:
  - `ent/schema/approval_policy.go`
  - `ent/schema/approval_ticket.go`
  - `internal/api/handlers/server_vm.go`
  - `internal/service/vnc_policy.go`
  - approval services and jobs that transition power requests from approval to
    execution
- Behavior changes:
  - `START_VM`, `STOP_VM`, and `RESTART_VM` stop bypassing approval in `prod`
  - policy evaluation keys are `operation + namespace environment_type`, not
    namespace name patterns
  - the platform keeps one router concept for approval backends:
    `ApprovalProviderRouter`

## Proposed Policy Shape

```go
field.Enum("environment_type").
    Values("test", "prod", "all").
    Default("all")

field.Enum("operation").
    Values(
        "CREATE_VM", "MODIFY_VM", "DELETE_VM",
        "START_VM", "STOP_VM", "RESTART_VM",
        "VNC_ACCESS",
    )

field.Bool("requires_approval").Default(true)
field.Int("priority").Default(100)
field.Bool("enabled").Default(true)
```

Evaluation contract:

- choose the first enabled rule by ascending `priority`
- allow exact `environment_type` match or `all`
- return a boolean requirement only
- keep approval-provider routing out of this service

## Proposed Execution Path

### No Approval Required

1. resolve VM and namespace environment type
2. evaluate `RequiresApproval`
3. create `DomainEvent`
4. enqueue the River power-operation job immediately

### Approval Required

1. resolve VM and namespace environment type
2. evaluate `RequiresApproval`
3. create `DomainEvent`
4. create `ApprovalTicket(operation_type=POWER)`
5. defer River enqueue until approval succeeds

Specific power actions remain in the event payload or request payload, not in a
growing approval-ticket enum.

## Rollout Notes

- This project is still pre-launch, so no legacy compatibility layer is
  retained for older `ApprovalPolicy` shapes.
- Canonical behavior is:
  1. `ApprovalPolicy` is keyed by `environment_type + operation`
  2. `ApprovalTicket.operation_type` includes `POWER`
  3. single and batch power operations use the shared approval requirement path
  4. VNC approval decisions reuse the same requirement service

## Open Questions

- Should the first rollout move VNC onto `ApprovalRequirementService`, or keep
  VNC on its current path until power operations are stable?
- Does the existing approval-event model need a distinct "approved power job
  dispatch" helper, or can it reuse the current approval callback path?
