---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-04-02
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0053: Pre-Launch RBAC Baseline Cleanup and Environment-Scoped Built-In Roles

> **Status**: Accepted<br>
> **Accepted On**: 2026-04-02<br>
> **Review Window**: 2026-04-02 → 2026-04-02<br>
> **Discussion**: internal product-owner decision for pre-launch RBAC cleanup<br>
> **Amends**: `ADR-0015-governance-model-v2.md#22-permission-model` *(removes wildcard and compatibility-era built-in role assumptions before first launch)*<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md` *(supersedes legacy RBAC seed examples that still mention `SystemAdmin`-style built-ins)*<br>
> **Amends**: `ADR-0019-governance-security-baseline-controls.md#built-in-roles-definition-correction-to-adr-0015-22` *(replaces transitional built-in role examples with the pre-launch baseline)*<br>
> **Extends**: `ADR-0017-vm-request-flow-clarification.md` *(keeps approval-gated production operations separated from platform administration)*<br>
> **Extends**: `ADR-0041-power-operation-approval-requirement-service.md` *(clarifies which non-admin roles may request gated production operations)*

---

## Context and Problem Statement

The repository still carried a pre-launch mixture of legacy compatibility
permissions (`cluster:manage`, `template:manage`, `auth_provider:manage`),
redundant bootstrap-era role ideas, and older built-in roles such as
`SystemAdmin` and `Operator`.

That baseline no longer matches the intended product shape:

* there is no migration burden yet because the platform has not launched
* environment scoping is already modeled through `RoleBinding.allowed_environments`
* approval duties must remain separated from platform administration
* development, test, and operations users need self-service access inside
  approved environments without inheriting platform-wide admin powers

The question is how the public RBAC baseline should be simplified before first
launch so that the permission catalog, seeded roles, runtime guards, and
governance checks all agree.

## Decision Drivers

* **Least privilege**: non-admin engineering roles must not inherit platform admin or approval authority.
* **Separation of duties**: request approvers and platform administrators are distinct responsibilities.
* **Environment scoping**: test/prod access should be granted by bindings, not by multiplying permission variants.
* **No compatibility burden**: legacy permission aliases can be removed before GA.
* **Readable operations model**: seeded roles should reflect real user categories rather than bootstrap-only or transitional roles.
* **Stable runtime guards**: handler checks, tests, and governance scripts must converge on the same canonical permissions.

## Considered Options

* **Option 1**: Keep the compatibility permissions and old built-ins until after GA
* **Option 2**: Remove compatibility permissions now and redefine built-ins around environment-scoped least privilege (chosen)
* **Option 3**: Introduce many environment-specific built-ins such as separate test/prod operator roles

## Decision Outcome

**Chosen option**: **"Remove compatibility permissions now and redefine
built-ins around environment-scoped least privilege"**, because the project has
not launched yet, and the existing role-binding model already provides the
right place to express test/prod scope without carrying transitional RBAC
aliases into GA.

### Scope Boundary

This ADR only defines the **platform/global RBAC baseline**:

* canonical global permission keys
* built-in platform-facing roles
* environment-scoped `RoleBinding.allowed_environments`

It does **not** replace the separate **resource membership / inheritance**
model already defined elsewhere:

* System membership remains the resource-facing access model
* Service and VM visibility/operations continue to inherit from System
* that inherited resource access complements, but does not substitute for,
  the global RBAC layer

In other words, the public product has two related but distinct access layers:

1. **Global/platform RBAC** — what platform capabilities a user may hold
2. **Resource membership inheritance** — which System/Service/VM resources the
   user may actually see or operate within those capabilities

### Normative Decisions

#### 1. Compatibility permissions are not part of the supported permission catalog

The following transitional permissions are removed from the public supported
catalog and must not be accepted when creating or updating roles:

* `cluster:manage`
* `template:manage`
* `auth_provider:manage`

Handlers, tests, and governance scripts must use the canonical granular
permissions instead:

* `cluster:read`, `cluster:write`
* `template:read`, `template:write`
* `auth_provider:{read,configure,update,delete,sync,mapping_*}`

#### 2. The public seeded built-in roles are limited to six canonical roles

The application-owned built-in roles are:

* `PlatformAdmin`
* `ApprovalAdmin`
* `DevelopmentEngineer`
* `TestEngineer`
* `SystemOperator`
* `Viewer`

Legacy built-ins such as `Bootstrap`, `SystemAdmin`, and `Operator` are not
part of the pre-launch baseline and must not be seeded as public built-ins.

#### 3. `PlatformAdmin` remains the only built-in super-admin role

`PlatformAdmin` keeps the explicit `platform:admin` permission.

No wildcard permissions such as `*:*`, `system:*`, or `vm:*` are allowed in the
public baseline.

#### 4. Approval authority is isolated into `ApprovalAdmin`

`ApprovalAdmin` is the built-in role for built-in approval queue ownership.

It may read the request context needed to review work, but it must not inherit
general platform administration.

#### 5. Engineering/operator roles share a capability envelope; bindings provide environment scope

`DevelopmentEngineer`, `TestEngineer`, and `SystemOperator` all use the same
core capability envelope:

* create/read services
* read/write systems
* create/read/delete/operate VMs
* request interactive VM console access (`vnc:access`, covering both VNC and serial)

These roles do **not** receive:

* `platform:admin`
* `builtin_approval:approve`
* `builtin_approval:view`

Environment scope is expressed through role bindings, especially
`allowed_environments`, not by introducing duplicate test/prod role variants.

Typical usage:

* development/test roles are usually bound to `test`
* system-operator may additionally be bound to `prod`
* production power or mutation actions still go through the approval policies
  defined elsewhere; holding `vm:operate` means "may request/perform the
  workflow", not "bypass approval"

#### 6. Bootstrap is a startup workflow, not a durable built-in role

The default admin bootstrap flow may create or reconcile a starting
`PlatformAdmin` assignment, but there is no dedicated long-lived `Bootstrap`
built-in role.

If the default admin already exists, seeding must reconcile the canonical
`PlatformAdmin` binding rather than skip RBAC bootstrap altogether.

### Consequences

* ✅ Good, because the permission catalog is smaller, explicit, and easier to audit.
* ✅ Good, because handler guards and UI permission pages no longer need to carry compatibility-era aliases.
* ✅ Good, because development/test/operations users can be scoped by environment without receiving platform or approval authority.
* ✅ Good, because production operations stay approval-gated without making operators platform administrators.
* 🟡 Neutral, because `DevelopmentEngineer`, `TestEngineer`, and `SystemOperator` now differ mainly by assignment policy rather than permission shape.
* ❌ Bad, because any stored roles still using removed compatibility permissions must be updated manually or rejected on write.

### Confirmation

* Seed tests verify that only the six canonical built-ins are created.
* Handler/API tests verify unsupported compatibility permissions are rejected.
* Governance checks verify admin catalog and explicit handler guards reference only canonical permissions.
* UI permission catalog renders only the supported public permissions.

---

## Pros and Cons of the Options

### Option 1: Keep the compatibility permissions and old built-ins until after GA

* ✅ Good, because it delays cleanup work.
* ❌ Bad, because it bakes transitional RBAC semantics into the public baseline.
* ❌ Bad, because runtime guards, tests, and UI continue to carry unused alias permissions.

### Option 2: Remove compatibility permissions now and redefine built-ins around environment-scoped least privilege (Chosen)

* ✅ Good, because it aligns the seeded roles with the intended first-launch product.
* ✅ Good, because environment scoping stays in bindings where the model already supports it.
* ✅ Good, because approval and platform-administration duties remain separated.
* 🟡 Neutral, because admins must think in terms of role + environment assignment rather than many special-case built-ins.

### Option 3: Introduce many environment-specific built-ins

* ✅ Good, because test/prod intent is explicit in role names.
* ❌ Bad, because it duplicates permission sets and pushes environment policy into the wrong layer.
* ❌ Bad, because every new workflow would need more built-in role variants.

---

## More Information

### Related Decisions

* `ADR-0015-governance-model-v2.md` — original governance model, amended here for pre-launch RBAC cleanup
* `ADR-0017-vm-request-flow-clarification.md` — approval and responsibility boundaries
* `ADR-0019-governance-security-baseline-controls.md` — explicit permission philosophy extended here to built-in role cleanup
* `ADR-0041-power-operation-approval-requirement-service.md` — approval-gated production operations remain in force

### Implementation Notes

* Public permission pages must render the canonical permission catalog only.
* Seeding reconciles built-in roles and removes obsolete built-in records.
* Governance/baseline scripts must assert the canonical read/write permissions rather than legacy compatibility aliases.
