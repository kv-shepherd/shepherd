# ADR-0052 Design Note: Existing VM Mutation Strategy

> **Status**: Accepted (ADR-0052 accepted on 2026-03-26)
> **Related ADR**: [ADR-0052](../../adr/ADR-0052-existing-vm-mutation-strategy.md)

## Purpose

This note captures implementation-facing details for separating:

* full desired-state VM submission
* existing VM mutation workflows

The goal is to keep SSA for create/provisioning while moving modify flows to
exact KubeVirt-native mutation semantics.

## Scope Boundary

### Still under SSA / rendered YAML

The following remain under `ADR-0011`:

* create/provisioning
* template + instance-size rendering into full `VirtualMachine` manifests
* dry-run validation of full rendered specs before first submission

### Moved out of SSA

The following are in scope for ADR-0052:

* CPU/memory modify on existing VMs
* restart-required desired-state resource changes
* `runStrategy` mutations
* other existing-VM mutations where the authoritative source is the current VM

## Client Priority

Implementation must use:

1. KubeVirt official client-go surfaces
2. KubeVirt-native subresources or patch/update methods
3. No generic Kubernetes client fallback for the mutation workflows covered by
   ADR-0052

If a future mutation requirement cannot be expressed through KubeVirt's
official client surfaces, it must be raised as a new ADR decision or explicit
ADR-0052 amendment before implementation proceeds.

## Approval-Time Pre-flight

Modify approval should no longer validate only a partial SSA manifest.

Instead:

1. fetch the current VM from the cluster
2. derive the exact mutation payload from:
   * current VM desired spec
   * requested target limits
   * admin-reviewed request overrides
3. dry-run that exact mutation shape
4. reject approval immediately if the cluster rejects it

This avoids "approval succeeded but execution later failed because the real
update shape differed from the approval-time dry-run."

## Authoritative Source

For existing VM mutation, the authoritative base object should be the live
cluster VM, not a database-only manifest snapshot.

Reasons:

* the cluster object contains the current desired spec that KubeVirt validates
* database-only partial state can drift
* KubeVirt admission may require sibling fields that are only correct when
  derived from the live object

## Error Handling

UI may simplify lifecycle state, but execution failures should preserve the raw
KubeVirt/Kubernetes error text.

The platform should only add context such as:

* ticket id
* event id
* vm identity
* which phase failed (`dry-run`, `execute`, `restart`, etc.)

It should not rewrite the underlying cluster error into platform-specific
wording.

## Suggested Provider Shape

The provider layer will likely need a dedicated mutation planner/executor for
existing VMs, separate from the current `RenderedYAML + UpdateVM` path.

Possible shape:

* `PlanVMMutation(currentVM, intent) -> exact patch payload + apply mode`
* `DryRunVMMutation(...)`
* `ExecuteVMMutation(...)`

This should remain provider-owned. Core should only own:

* approval policy
* canonical request/review data
* job lifecycle
* raw error recording and exposure

## Testing Expectations

Minimum tests after implementation:

* modify approval dry-run and execution use the same mutation payload
* running shrink requests can be approved as restart-required when KubeVirt
  accepts the desired-state mutation
* hotplug-capable increase requests use the KubeVirt-native live mutation path
* raw KubeVirt admission errors remain visible in ticket/request failure views
* create/provisioning still uses rendered YAML + SSA unchanged

## Migration Guidance

The current partial-SSA modify path should be treated as transitional and
replaced after ADR acceptance.

Before and during that replacement:

* avoid widening SSA ownership further just to chase admission-required fields
* do not introduce a persistent database shadow manifest for modify flows
