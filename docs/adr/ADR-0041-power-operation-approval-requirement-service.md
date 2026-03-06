---
status: "proposed"
date: 2026-03-06
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0041: Power Operation Approval Requirement Service

> **Review Period**: Until 2026-03-08 (48-hour minimum)<br>
> **Amends**: `ADR-0005-workflow-extensibility.md`, `ADR-0015-governance-model-v2.md#7-environment-aware-approval-policies`<br>
> **Relates To**: `ADR-0015-governance-model-v2.md#15-namespace-cluster-binding-and-environment-type-clarification`, `RFC-0004-external-approval.md`

---

## Context and Problem Statement

ADR-0015 already defines the intended rule matrix:

| Operation | test | prod |
|-----------|------|------|
| `START_VM` | no approval | requires approval |
| `STOP_VM` | no approval | requires approval |
| `RESTART_VM` | no approval | requires approval |
| `VNC_ACCESS` | no approval | requires approval |

The current implementation does not apply that matrix to VM power operations. `StartVM`, `StopVM`, and `RestartVM` directly create `DomainEvent` records and enqueue River jobs without first deciding whether an approval ticket is required.

There is also a naming and responsibility problem in the original draft: the codebase already contains an `ApprovalProviderRouter`, and that router is responsible for routing tickets to approval backends. Introducing a second router for policy evaluation would blur responsibilities and make the design harder to reason about.

**Core question**: How should the platform decide whether a VM power operation requires approval, in a way that is environment-aware, data-driven, and cleanly separated from approval backend routing?

## Decision Drivers

* **ADR-0015 compliance**: power operations must respect the accepted test/prod matrix
* **Single source of environment truth**: policy decisions must use Namespace environment type, not namespace name patterns
* **Separation of concerns**: "does this need approval" is different from "which approval backend handles it"
* **Operational clarity**: specific power action belongs in DomainEvent/payload, not in a coarse ticket type explosion
* **Extensibility**: future admin-configurable policy should remain possible without introducing regex-driven ambiguity

## Considered Options

* **Option 1**: `ApprovalRequirementService` backed by `ApprovalPolicy` on `environment_type + operation` (chosen)
* **Option 2**: Hardcode `if env == prod` inside handlers
* **Option 3**: `namespace_pattern`-based `PolicyRouter`
* **Option 4**: Encode process rules in RBAC permissions

## Decision Outcome

**Chosen option**: "Use an `ApprovalRequirementService` backed by `ApprovalPolicy` keyed on environment type and operation", because it matches the accepted ADR-0015 model, avoids a second router abstraction, and keeps policy evaluation separate from provider routing.

### Consequences

* ✅ Good, because policy is evaluated against Namespace environment type (`test`/`prod`), not naming heuristics
* ✅ Good, because the existing `ApprovalProviderRouter` keeps its current, narrower responsibility
* ✅ Good, because power-operation handlers can share a single approval requirement path with VNC
* ✅ Good, because `ApprovalTicket.operation_type` can stay coarse (`POWER`) while DomainEvent keeps the exact action (`START/STOP/RESTART`)
* 🟡 Neutral, because this changes the current experimental `ApprovalPolicy` schema direction
* ❌ Bad, because power operation handlers must be refactored from direct enqueue to a split path; mitigation: keep the current direct-execute path for operations that do not require approval

### Confirmation

1. `StartVM` on a VM in a `test` namespace creates a `DomainEvent` and River job directly, with no approval ticket
2. `StartVM` on a VM in a `prod` namespace creates `DomainEvent + ApprovalTicket(operation_type=POWER)` and does not enqueue the River job until approval
3. `StopVM` and `RestartVM` follow the same environment-type policy path
4. The same service can be reused by VNC request policy so that test/prod approval rules live in one place
5. No code path introduces a second "policy router" next to `ApprovalProviderRouter`

---

## Pros and Cons of the Options

### Option 1: `ApprovalRequirementService` backed by `ApprovalPolicy`

Use a dedicated service to answer the question: does operation `X` in namespace environment type `Y` require approval?

Proposed policy shape:

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

Service sketch:

```go
type ApprovalRequirementService struct {
    client *ent.Client
}

func (s *ApprovalRequirementService) RequiresApproval(
    ctx context.Context,
    operation approvalpolicy.Operation,
    env namespaceregistry.Environment,
) (bool, error) {
    rule, err := s.client.ApprovalPolicy.Query().
        Where(
            approvalpolicy.EnabledEQ(true),
            approvalpolicy.OperationEQ(operation),
            approvalpolicy.EnvironmentTypeIn(
                approvalpolicy.EnvironmentType(env),
                approvalpolicy.EnvironmentTypeAll,
            ),
        ).
        Order(ent.Asc(approvalpolicy.FieldPriority)).
        First(ctx)
    if err != nil {
        return false, err
    }
    return rule.RequiresApproval, nil
}
```

Handler split:

```go
needsApproval, err := s.approvalRequirementService.RequiresApproval(ctx, approvalpolicy.OperationSTART_VM, ns.Environment)
if needsApproval {
    return s.createPowerApprovalTicket(...)
}
return s.enqueueVMPowerOp(...)
```

Ticket design:

```go
field.Enum("operation_type").
    Values("CREATE", "DELETE", "POWER", "VNC_ACCESS").
    Default("CREATE")
```

* ✅ Good, because it aligns with ADR-0015's accepted `test/prod` environment model
* ✅ Good, because it removes namespace-name regexes from approval policy
* ✅ Good, because it avoids adding a second router abstraction next to `ApprovalProviderRouter`
* ✅ Good, because specific power action remains where it already belongs: DomainEvent and payload
* ❌ Bad, because the existing experimental `ApprovalPolicy` schema must be reshaped

### Option 2: Hardcode environment checks in handlers

```go
if ns.Environment == namespaceregistry.EnvironmentProd {
    return s.createPowerApprovalTicket(...)
}
return s.enqueueVMPowerOp(...)
```

* ✅ Good, because it is simple
* ❌ Bad, because it duplicates policy logic across handlers and features
* ❌ Bad, because it does not provide a migration path to admin-configurable rules

### Option 3: `namespace_pattern`-based `PolicyRouter`

Evaluate policies against regexes such as `^prod[-_]`.

* ✅ Good, because it looks flexible
* ❌ Bad, because namespace names are free-form and are not the policy truth source
* ❌ Bad, because it conflicts with ADR-0015, which explicitly defines environment type on Namespace/Cluster
* ❌ Bad, because the codebase already has `ApprovalProviderRouter` with a different role

### Option 4: Encode process rules in RBAC permissions

Use permissions like `vm:start:direct` and `vm:start:approved`.

* ✅ Good, because permissions already exist in the system
* ❌ Bad, because authorization and approval process are orthogonal concerns
* ❌ Bad, because it doubles policy complexity without improving clarity

---

## More Information

### Related Decisions

* `ADR-0005-workflow-extensibility.md` - Approval flow remains `PENDING -> APPROVED/REJECTED`
* `ADR-0015-governance-model-v2.md` - Defines the test/prod approval matrix and explicit namespace environment type
* `RFC-0004-external-approval.md` - External backends remain a later concern handled by provider routing, not requirement evaluation

### References

* `internal/governance/approval/provider_router.go` - Existing provider router; this ADR intentionally does not add a second router concept
* `internal/service/vnc_policy.go` - Existing environment-aware decision precedent
* `internal/api/handlers/server_vm.go` - Current direct-enqueue power-operation path

### Implementation Notes

**Schema changes**:

| Entity | Change |
|--------|--------|
| `ApprovalPolicy` | Replace experimental `namespace_pattern` matching with `environment_type + operation + requires_approval + priority + enabled` |
| `ApprovalTicket` | Add `POWER` to `operation_type` |

**Execution model**:

| Stage | No approval required | Approval required |
|------|-----------------------|-------------------|
| Handler | Create `DomainEvent` and enqueue River job | Create `DomainEvent` and `ApprovalTicket` only |
| Approval | N/A | On approval, enqueue River power job |
| Worker | Executes K8s start/stop/restart | Executes only after approval |

**Naming rule**:

Do **not** introduce `PolicyRouter`. Keep these responsibilities separate:

| Component | Responsibility |
|-----------|----------------|
| `ApprovalRequirementService` | Decide whether approval is required |
| `ApprovalProviderRouter` | Route a ticket to builtin/external approval backend |

**Revisit criteria**:

If future policy needs selectors richer than environment type, add explicit typed selectors later. Do not fall back to namespace-name regexes as the primary policy key.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-06 | @jindyzhao | Initial draft |
| 2026-03-06 | @jindyzhao | Reworked around `ApprovalRequirementService`, environment-type policy keys, and existing provider-router boundaries |
