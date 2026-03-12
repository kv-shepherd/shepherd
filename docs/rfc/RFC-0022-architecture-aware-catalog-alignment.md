# RFC-0022: Architecture-Aware Catalog Alignment for Templates, Instance Sizes, and Clusters

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: First Arm64 rollout, heterogeneous cluster adoption, or architecture-related catalog mismatch incidents  
> **Related**: [ADR-0018](../adr/ADR-0018-instance-size-abstraction.md), [ADR-0036](../adr/ADR-0036-template-instancesize-boundary-enforcement.md), [ADR-0040](../adr/ADR-0040-catalog-scope-for-template-and-instancesize.md)

---

## Problem

KubeVirt supports guest architecture as part of VM spec, and Arm64 is not just
"x86 with a multi-arch image". Image portability helps, but it does not remove
virtual hardware and feature differences.

Examples from KubeVirt documentation:

- Arm64 requires different virtual hardware defaults than AMD64/x86_64
- Some features commonly used on x86 are unavailable or behave differently on Arm64
- A template, instance size, and cluster may each be individually valid, but the
  combination can still be architecture-incompatible

This means the platform may eventually need an architecture-aware alignment
layer between:

- `Template`
- `InstanceSize`
- target `Cluster`

Without that layer, a future heterogeneous environment may expose catalog items
that look valid in isolation but fail or degrade at runtime.

---

## Current Decision

Do **not** change the backend data model yet.

Reasons:

1. The current rollout environment is pure x86, so architecture mismatch is not
   a live operational problem today.
2. Current template catalog APIs do not persist an `architecture` field, so a
   frontend-only selector would be misleading.
3. A correct solution is larger than template metadata alone; it likely affects
   template curation, instance-size matching, cluster capability scanning, and
   request-time validation.

So the immediate product decision is:

- V1 remains architecture-agnostic at the persisted catalog layer
- no fake `architecture` field is exposed in current admin forms
- architecture-aware governance is deferred to this RFC

---

## Why Multi-Arch Images Are Not the Full Answer

Multi-architecture container images, `buildx`, and CDI clone/import flows solve
artifact distribution, but they do **not** fully solve runtime compatibility.

They do not answer questions such as:

- Is this template intended for `amd64`, `arm64`, or either?
- Does this instance size enable hardware features that only make sense on x86?
- Does the target cluster contain nodes that can actually satisfy the VM's
  architecture and virtual hardware assumptions?

So "the image can be pulled on both architectures" is necessary, but not
sufficient, for platform governance.

---

## Future Solution Direction

When the trigger condition is met, revisit architecture as a first-class
catalog concern.

### Candidate Model

- `Template.architecture`: `amd64 | arm64 | any`
- `InstanceSize.architecture`: `amd64 | arm64 | any`
- `Cluster.supported_architectures`: detected from cluster/node capability scan

### Candidate Matching Rules

1. A template marked `amd64` must not be offered against an Arm64-only cluster
2. An instance size marked `amd64` must not be paired with an Arm64-only cluster
3. Request-time matching should validate:
   - template ↔ cluster
   - instance size ↔ cluster
   - template ↔ instance size
4. `any` is allowed only when the platform has strong evidence that the image
   source and VM hardware expectations are architecture-neutral

### Candidate UX

- Admin catalog forms expose architecture only after the backend persists it
- Request wizard filters incompatible combinations before submission
- Catalog badges clearly show `amd64`, `arm64`, or `any`

---

## Non-Goals

This RFC does not propose:

- implementing architecture persistence in the current release
- adding frontend-only temporary architecture selectors
- assuming multi-arch images eliminate all KubeVirt-level differences

---

## References

- KubeVirt user guide: Virtual Machines on Arm64  
  https://kubevirt.io/user-guide/cluster_admin/virtual_machines_on_Arm64/
- KubeVirt user guide: Virtual Hardware  
  https://kubevirt.io/user-guide/compute/virtual_hardware/
- KubeVirt user guide: Installation  
  https://kubevirt.io/user-guide/cluster_admin/installation/
