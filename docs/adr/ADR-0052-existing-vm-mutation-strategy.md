---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-03-24
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0052: Existing VM Mutation Strategy — KubeVirt Client First, Exact Patch + Dry-Run

> **Status**: 🔍 Public Review — 48-hour minimum comment window<br>
> **Review Open**: 2026-03-24<br>
> **Review Closes**: 2026-03-26 (earliest merge date)<br>
> **Discussion**: [Issue #412](https://github.com/kv-shepherd/shepherd/issues/412)<br>
> **Amends**: `ADR-0011-ssa-apply-strategy.md` *(narrows SSA to full desired-state provisioning/create flows and removes existing-VM mutation flows from the SSA rule)*<br>
> **Extends**: `ADR-0001-kubevirt-client.md` *(requires KubeVirt official client/subresources for existing VM mutation workflows)*<br>
> **Extends**: `ADR-0017-vm-request-flow-clarification.md` *(clarifies how modify approvals validate and execute restart-required changes)*
>
> 📝 **Design Note**: implementation-facing details are in `docs/design/notes/ADR-0052-existing-vm-mutation-strategy.md`.

---

## Context and Problem Statement

`ADR-0011` established a strong rule for VM resource submission: Shepherd is a
"YAML porter" and should submit rendered `VirtualMachine` manifests through
Server-Side Apply (SSA).

That rule works well for create/provisioning flows where Shepherd owns the
whole desired manifest. It breaks down for **existing VM mutation** flows such
as resource modify, because KubeVirt upstream examples and admission behavior
expect narrow patch semantics against the live `VirtualMachine`, not partial
SSA re-application of `spec.template.spec.domain`.

The open question is how Shepherd should validate and execute existing VM
mutations without losing KubeVirt-native semantics or drifting into
platform-specific shadow models.

## Decision Drivers

* **Upstream compatibility**: KubeVirt user-guide examples for CPU/memory hotplug and runStrategy changes use patch semantics, not SSA apply.
* **Client commitment**: existing VM mutation should stay on KubeVirt official client surfaces rather than generic Kubernetes client paths.
* **Exact pre-flight validation**: approval-time validation should check the same mutation shape that execution will later submit.
* **Operational transparency**: KubeVirt/Kubernetes admission and validation errors should remain visible in their native form for debugging.
* **Boundary clarity**: create/provisioning and existing-VM mutation are different categories and should not be forced into the same submission model.
* **Minimal field ownership**: narrow mutations should avoid unnecessarily re-owning broad VM spec fields through SSA field ownership.

## Considered Options

* **Option 1**: Keep SSA for all VM writes, enrich partial manifests until admission succeeds
* **Option 2**: Keep SSA for create/provisioning, use exact patch semantics for existing VM mutation (chosen)
* **Option 3**: Replace all VM writes with typed Get-Modify-Update

## Decision Outcome

**Chosen option**: **"Keep SSA for create/provisioning, use exact patch
semantics for existing VM mutation"**, because it preserves the value of
`ADR-0011` for full desired-state submission while aligning modify flows with
KubeVirt upstream mutation patterns and exact dry-run validation.

### Normative Decisions

#### 1. `ADR-0011` remains authoritative for full desired-state VM submission only

The SSA + rendered YAML strategy continues to apply to:

* create/provisioning flows
* template-rendered full VM desired state
* validation of full rendered specs before first submission

It no longer normatively applies to **existing VM mutation workflows**.

#### 2. Existing VM mutation flows must use exact mutation semantics, not partial SSA replay

"Existing VM mutation" includes any workflow whose intent is to mutate an
already-existing `VirtualMachine`, for example:

* CPU/memory modify
* restart-required offline resource changes
* runStrategy changes
* other narrow spec mutations on a live VM

These flows must construct the **exact patch/update shape** that will later be
executed, instead of building a partial `VirtualMachine` manifest and submitting
it through SSA.

#### 3. KubeVirt official client is the required mutation client

For existing VM mutation workflows, Shepherd must prefer:

* KubeVirt client-go surfaces
* KubeVirt subresources
* KubeVirt-native patch/update paths

For the workflows covered by this ADR, Shepherd must not fall back to the
generic Kubernetes client. If a future mutation requirement cannot be expressed
through KubeVirt's official client surfaces, that gap must be handled by a new
ADR or an explicit amendment to this ADR before implementation proceeds.

#### 4. Approval-time pre-flight must use the same mutation shape as execution

Approval of a modify request must validate the exact mutation that execution
will later submit.

This means:

* build the exact patch/update payload from the live VM + approved overrides
* run dry-run against that mutation shape
* reject approval synchronously if the cluster or admission chain rejects it

Approval must not green-light a request based on a different validation model
than the one used at execution time.

#### 5. Restart-required changes remain allowed as desired-state mutations

Some changes may be valid desired-state updates but not hotpluggable on a
running VM, for example:

* CPU shrink
* memory shrink
* topology changes that require restart

These changes may still be approved and persisted as desired-state changes, as
long as:

* the exact mutation dry-run succeeds
* the platform clearly marks them as `restart_required`
* execution does not pretend the change is live-effective immediately

#### 6. KubeVirt and Kubernetes errors should remain raw at the execution boundary

When the cluster rejects a mutation, Shepherd may wrap the error with platform
context (ticket/event/vm identifiers), but must preserve the original
KubeVirt/Kubernetes error message.

User-facing UI may simplify the workflow state, but the platform/operator view
must still expose the raw backend error.

#### 7. Shepherd must not introduce a database-only shadow manifest for existing VM mutation

For existing VM mutation, the authoritative starting point is the **current
cluster VM object**, not a stale database snapshot.

If a mutation needs current spec context, Shepherd must derive it from the live
VM object fetched from the cluster, then build the exact mutation payload from
that state.

### Consequences

* ✅ Good, because existing VM mutation now follows KubeVirt-native semantics instead of forcing SSA into workflows it does not fit well.
* ✅ Good, because approval-time dry-run can become identical to execution-time mutation, reducing "approval passed, execution failed" drift.
* ✅ Good, because the project still keeps SSA where it provides the most value: create/provisioning and full desired-state submission.
* ✅ Good, because field ownership stays narrower; modify flows no longer re-own large parts of the VM spec just to change a few fields.
* ✅ Good, because provider behavior stays tightly aligned with KubeVirt-native mutation semantics.
* 🟡 Neutral, because the provider layer now owns two submission styles: full-manifest SSA and exact mutation patch/update.
* 🟡 Neutral, because some modify flows may need cluster-state reads before both dry-run and execution.
* ❌ Bad, because this narrows the original simplicity of "all VM writes use SSA", and implementation must now classify which path a workflow belongs to.

### Confirmation

* Code review confirms create/provisioning still uses rendered YAML + SSA.
* Code review confirms modify flows no longer submit partial `VirtualMachine` manifests via SSA.
* Approval tests confirm modify approval dry-run uses the same mutation shape as execution.
* Provider tests confirm existing VM mutation paths use KubeVirt client-go patch/update surfaces, not generic Kubernetes client fallbacks.
* Failure-path tests confirm raw KubeVirt/Kubernetes error messages remain visible in ticket/request execution surfaces.

---

## Pros and Cons of the Options

### Option 1: Keep SSA for all VM writes, enrich partial manifests until admission succeeds

Continue treating every VM write as SSA, patching over missing required fields
case-by-case.

* ✅ Good, because it preserves the simplest reading of `ADR-0011`.
* ✅ Good, because create and modify would still share one submission mechanism.
* ❌ Bad, because KubeVirt mutation examples and admission behavior are not centered on partial SSA replay of existing VM specs.
* ❌ Bad, because each new admission failure risks another round of "copy more fields from the live object" instead of using the native mutation shape.
* ❌ Bad, because field ownership becomes wider than the intended change set.

### Option 2: Keep SSA for create/provisioning, use exact patch semantics for existing VM mutation (Chosen)

Use the best submission model for each category: SSA for full desired-state
create, exact patch/update for existing VM mutation.

* ✅ Good, because it matches KubeVirt upstream guidance for VM mutation.
* ✅ Good, because dry-run and execution can be made identical for modify approval.
* ✅ Good, because it preserves `ADR-0011` where it is strongest instead of discarding it.
* 🟡 Neutral, because the provider layer must clearly separate create vs mutate flows.

### Option 3: Replace all VM writes with typed Get-Modify-Update

Abandon the YAML porter + SSA model and move everything to typed structs.

* ✅ Good, because update semantics can be explicit and type-safe.
* ❌ Bad, because it throws away the version-decoupling and full-manifest flexibility that motivated `ADR-0011`.
* ❌ Bad, because create/provisioning would become more tightly coupled to KubeVirt API versions.

---

## More Information

### Related Decisions

* `ADR-0001-kubevirt-client.md` — official KubeVirt client selection
* `ADR-0011-ssa-apply-strategy.md` — SSA strategy for full desired-state submission; amended here for existing VM mutation scope
* `ADR-0017-vm-request-flow-clarification.md` — approval flow boundaries and user/admin responsibility split
* `ADR-0041-power-operation-approval-requirement-service.md` — approval-gated operational actions already treat execution as a distinct workflow

### References

* KubeVirt user-guide `/compute/cpu_hotplug.md` — uses `kubectl patch` for CPU socket hotplug
* KubeVirt user-guide `/compute/memory_hotplug.md` — uses `kubectl patch` for guest memory hotplug
* KubeVirt user-guide `/architecture.md` — uses patch semantics for `runStrategy`
* Related Issue: [#412](https://github.com/kv-shepherd/shepherd/issues/412)

### Implementation Notes

If this ADR is accepted, the implementation should:

1. keep create/provisioning on `RenderVMSpecToYAML + SSA`
2. introduce an explicit provider-owned mutation planner for existing VM changes
3. dry-run the exact mutation during modify approval
4. execute the same mutation shape after approval
5. preserve raw KubeVirt/Kubernetes failure text in execution surfaces

Revisit this ADR if KubeVirt later introduces a first-class official mutation
client abstraction that changes the recommended client surface for CPU/memory
modify workflows.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-24 | @jindyzhao | Initial draft — published for 48-hour public review |
