---
status: "proposed"
date: 2026-03-12
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0045: Cluster-Resolved Root Volume Provisioning Intent

> **Review Period**: Until 2026-03-14 (48-hour minimum)<br>
> **Discussion**: [Issue #359](https://github.com/kv-shepherd/shepherd/issues/359)<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md#core-design-principles` *(clarifies lifecycle boundary for storage provisioning intent)*<br>
> **Relates To**: `ADR-0011-ssa-apply-strategy.md`, `ADR-0017-vm-request-flow-clarification.md`, `ADR-0018-instance-size-abstraction.md`, `ADR-0042-cluster-policy-governance-model.md`

---

## Context and Problem Statement

The platform now needs to expose root-volume provisioning controls derived from
CDI and Kubernetes PVC semantics, especially `dv_access_modes` and
`dv_volume_mode`. These settings are cluster-sensitive: the same
`StorageClass` may expose one or more supported claim-property combinations via
CDI `StorageProfile`, while catalog authors often do not know the eventual
target cluster at authoring time.

The question is: where may the platform keep ambiguous storage intent such as
`auto`, and at what point must it be resolved into explicit runtime values?

## Decision Drivers

* Preserve `InstanceSize` portability across clusters with different storage capabilities
* Prevent approval and runtime from operating on ambiguous storage intent
* Keep SSA declarative and explicit by the time VM YAML is rendered
* Avoid silent drift when cluster storage capabilities change after VM creation
* Make ambiguous multi-option `StorageProfile` cases auditable and reviewable
* Align root-volume provisioning behavior with target-cluster governance

## Considered Options

* **Option 1**: Keep `auto` through approval and runtime, re-resolving on every operation
* **Option 2**: Allow `auto` only in catalog/spec intent; require approval-time resolution and persist explicit runtime values
* **Option 3**: Forbid `auto` entirely and require catalog authors to always choose explicit storage settings

## Decision Outcome

**Chosen option**: "Option 2", because it preserves reusable catalog intent
without allowing ambiguous values to leak into approval or runtime behavior.

### Consequences

* ✅ Good, because `InstanceSize` can stay portable when catalog authors do not yet know target-cluster storage capabilities
* ✅ Good, because approval decisions and rendered VM YAML always operate on explicit `storageClass`, `dv_access_modes`, and `dv_volume_mode`
* ✅ Good, because later CPU, memory, or disk changes can reuse the previously resolved storage values instead of silently re-resolving `auto`
* ✅ Good, because ambiguous `StorageProfile` cases become reviewable approval choices instead of hidden backend guesses
* 🟡 Neutral, because approval UX becomes more stateful: it may need to request additional storage choices when cluster capabilities are ambiguous
* ❌ Bad, because implementation must persist a per-VM resolved storage profile in addition to catalog intent; mitigation: keep `auto` as authoring-time intent only and treat resolved values as runtime state

### Confirmation

This decision is correctly implemented when all of the following are true:

1. `InstanceSize` may represent storage provisioning intent as either explicit values or `auto`
2. Approval cannot complete while root-volume provisioning remains unresolved
3. If the target cluster and storage class expose exactly one valid provisioning mode, approval may resolve it automatically
4. If multiple valid provisioning modes remain, approval must require an explicit operator choice before completion
5. The approved VM stores explicit resolved values; runtime records and rendered YAML do not contain `auto`
6. Subsequent VM updates reuse persisted resolved values instead of re-running `auto` resolution
7. Storage mode changes after creation are treated as explicit migration/reprovisioning decisions, not ordinary CPU/memory edits

---

## Pros and Cons of the Options

### Option 1: Keep `auto` through approval and runtime

Treat `auto` as a durable runtime value and re-resolve it whenever approval,
creation, or later updates occur.

* ✅ Good, because catalog authors need not think about cluster-specific storage details
* ❌ Bad, because approval outcomes become dependent on hidden backend re-resolution
* ❌ Bad, because later cluster `StorageProfile` changes can silently alter behavior for existing VMs
* ❌ Bad, because SSA input would be derived from ambiguous runtime intent rather than explicit desired state

### Option 2: Resolve `auto` at approval and persist explicit runtime values

Keep `auto` only as catalog/spec intent, then resolve it against the target
cluster during approval and store the explicit result for runtime use.

* ✅ Good, because approval is the first point where target-cluster capabilities are known
* ✅ Good, because rendered VM YAML stays explicit and deterministic
* ✅ Good, because later updates can remain stable even if cluster storage defaults drift
* ❌ Bad, because approval must sometimes collect more information before it can proceed

### Option 3: Forbid `auto` and require explicit catalog values only

Force authors to always choose `storageClass`, `dv_access_modes`, and
`dv_volume_mode` in the catalog itself.

* ✅ Good, because there is no ambiguous value anywhere in the workflow
* ❌ Bad, because catalog authors often do not know the eventual target cluster or storage class
* ❌ Bad, because it reduces reuse of shared `InstanceSize` definitions across clusters

---

## More Information

### Related Decisions

* `ADR-0011-ssa-apply-strategy.md` - SSA remains the write mechanism for explicit desired state
* `ADR-0017-vm-request-flow-clarification.md` - approval is the governance point where target placement is known
* `ADR-0018-instance-size-abstraction.md` - `InstanceSize` remains the authoring layer for hardware/provisioning intent
* `ADR-0042-cluster-policy-governance-model.md` - cluster selection and governance remain explicit post-capability controls

### References

* [CDI API Reference](https://kubevirt.io/cdi-api-reference/main/definitions.html)
* [KubeVirt Runbook: CDIStorageProfilesIncomplete](https://kubevirt.io/monitoring/runbooks/CDIStorageProfilesIncomplete.html)
* [KubeVirt Labs: DataVolume / StorageProfile behavior](https://kubevirt.io/labs/kubernetes/lab2.html)
* Related Issue: [#359](https://github.com/kv-shepherd/shepherd/issues/359)

### Implementation Notes

While this ADR remains proposed:

* Backend storage-profile reads should expose the full supported
  claim-property combinations, not only a single default volume mode.
* Approval should treat `auto` as an unresolved authoring value and reject
  completion until it is mapped to explicit runtime values.
* If the target cluster exposes multiple candidate storage classes or multiple
  supported claim-property combinations, approval UX may need a follow-up
  choice step.
* Persisted VM/runtime state must store the resolved storage settings so later
  CPU, memory, or disk changes reuse them without re-running `auto`.
* Ordinary post-create edits must not implicitly change root-volume access mode
  or volume mode; such changes require an explicit storage migration or
  reprovisioning flow.

Revisit this ADR if:

* the platform introduces cluster-agnostic persistent storage capability abstractions
* CDI upstream defines a stable deterministic precedence rule for multiple
  `claimPropertySets` that the product chooses to adopt directly
* the project adds a dedicated storage migration workflow for existing VMs

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-12 | @jindyzhao | Initial draft |
