---
status: "proposed"
date: 2026-03-06
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0042: Explicit Cluster Policy Governance Model

> **Review Period**: Until 2026-03-08 (48-hour minimum)<br>
> **Discussion**: [Issue #326](https://github.com/kv-shepherd/shepherd/issues/326)<br>
> **Amends**: `ADR-0014-capability-detection.md` *(clarifies capability vs policy boundary)*<br>
> **Amends**: `ADR-0018-instance-size-abstraction.md` *(adds policy enforcement after capability matching)*

---

## Context and Problem Statement

KubeVirt Shepherd already has a capability layer: cluster health checks detect
KubeVirt version, feature gates, storage classes, and other objective platform
facts, while `InstanceSize` expresses workload requirements such as dedicated
CPU, hugepages, GPU, or SR-IOV. That answers "can this cluster run this
workload?" but not "does the platform allow this workload on this cluster?"

This gap is now operationally significant. A cluster may support hugepages or
CPU overcommit, while the platform still wants to forbid them for ordinary
workloads, for `prod`, or for specific storage/clone patterns. Existing Go code
has no first-class model for this layer; Python production code also does not
provide a reusable cluster-policy abstraction beyond `permission_group`.

## Decision Drivers

* Keep objective cluster capability detection separate from administrative policy
* Support different governance rules across clusters that share the same
  `environment_type`
* Preserve a single platform source of truth across multiple clusters rather
  than depending on per-cluster custom CRDs
* Make approval-time validation and cluster selection deterministic and auditable
* Avoid overloading `catalog_scope`, namespace environment, or capability flags
  with policy meaning

## Considered Options

* **Option 1**: Keep policy implicit in capability detection and environment
  conventions
* **Option 2**: Store policy flags directly on `Cluster`
* **Option 3**: Create an explicit one-to-one `ClusterPolicy` entity

## Decision Outcome

**Chosen option**: "Option 3: Create an explicit one-to-one `ClusterPolicy`
entity", because it keeps live-detected capability facts separate from
admin-controlled governance rules, gives the policy layer a stable place to
expand, and avoids mixing mutable policy with connection metadata on `Cluster`.

### Consequences

* ✅ Good, because `capability` continues to mean objective cluster fact and
  `policy` continues to mean administrative allowance
* ✅ Good, because approval-time validation can report both capability mismatch
  and policy denial explicitly
* ✅ Good, because policy changes become auditable configuration changes instead
  of hidden conventions
* 🟡 Neutral, because rollout requires a new entity and migration/backfill work
* ❌ Bad, because the platform must now maintain one more admin-managed model;
  mitigation is to keep the first version intentionally small and explicit

### Confirmation

Implementation is correct when all of the following are true:

1. `ApprovalValidator` performs capability matching first and policy matching
   second
2. A workload denied by cluster policy is rejected even when the cluster has the
   required technical capability
3. New clusters are not considered eligible for scheduling until an explicit
   policy row exists
4. API/admin surfaces can show both detected capabilities and effective policy
   without conflating them

---

## Pros and Cons of the Options

### Option 1: Keep policy implicit in capability detection and environment conventions

Use `enabled_features`, storage classes, and `environment=test|prod` as the
only routing inputs. Continue encoding policy in handler logic or operator
convention.

* ✅ Good, because it avoids schema changes
* ✅ Good, because it preserves current runtime behavior with minimal work
* ❌ Bad, because it keeps "cluster can" and "platform allows" mixed together
* ❌ Bad, because policy differences cannot be audited or cleanly overridden per
  cluster
* ❌ Bad, because catalog filtering, approval, and cluster selection continue to
  drift toward ad-hoc conventions

### Option 2: Store policy flags directly on `Cluster`

Add governance booleans and allowlists directly to the existing `clusters`
table.

* ✅ Good, because it is simple to query
* ✅ Good, because one row contains all cluster metadata
* ❌ Bad, because `Cluster` already mixes connectivity, health, detected
  capability, and admin overrides; adding governance makes that boundary worse
* ❌ Bad, because future policy expansion becomes harder to review and migrate

### Option 3: Create an explicit one-to-one `ClusterPolicy` entity

Model governance separately and evaluate it after capability matching.

* ✅ Good, because it creates a clean `capability -> policy -> approval`
  pipeline
* ✅ Good, because it aligns with Kubernetes practice where quotas, limit
  ranges, and admission rules are separate policy objects rather than inferred
  from runtime capability
* ✅ Good, because it leaves room for targeted evolution without bloating the
  `clusters` table
* ❌ Bad, because it introduces another table/service pair

---

## More Information

### Proposed Policy Surface (V1)

`ClusterPolicy` is a one-to-one administrative policy record keyed by
`cluster_id`.

V1 fields:

```go
field.String("id").Unique().Immutable()
field.String("cluster_id").Unique().Immutable()
field.Bool("allow_cpu_overcommit").Default(true)
field.Bool("allow_memory_overcommit").Default(true)
field.Bool("allow_dedicated_cpu").Default(true)
field.Bool("allow_gpu").Default(true)
field.Bool("allow_sriov").Default(true)
field.Bool("allow_hugepages").Default(true)
field.JSON("allowed_hugepages_sizes", []string{}).Optional()
field.Bool("allow_cdi_clone").Default(true)
field.JSON("allowed_clone_source_namespaces", []string{}).Optional()
field.JSON("allowed_storage_classes", []string{}).Optional()
field.String("created_by").NotEmpty()
field.String("updated_by").Optional()
```

V1 semantics:

* empty `allowed_hugepages_sizes` means "any size supported by the cluster and
  allowed by policy"
* empty `allowed_clone_source_namespaces` means "any namespace allowed, subject
  to RBAC and source-PVC preflight"
* empty `allowed_storage_classes` means "any detected storage class or admin
  default storage class"

### Enforcement Order

The scheduling / approval contract becomes:

1. namespace environment isolation (`test|prod`)
2. catalog visibility (`catalog_scope`)
3. cluster capability matching (ADR-0014 + ADR-0018)
4. cluster policy matching (this ADR)
5. approval / dry-run / worker execution

### Rollout Strategy

Steady-state requirement:

* every usable cluster must have an explicit `ClusterPolicy` row

Rollout rule:

* existing clusters are backfilled with explicit rows generated from a
  `legacy-compatible` preset so the platform does not rely on "missing policy =
  allow"
* new clusters are not eligible for selection until a policy row exists

### Out of Scope

This ADR does **not** define:

* policy presets UI/UX
* per-namespace cluster policy overrides
* syncing policy from Kubernetes `ResourceQuota`, `LimitRange`, or custom CRDs
* future `KubeVirtInstancetypeAdapter` compatibility work
* whether `containerdisk` should remain exposed to general users

### Related Decisions

* `ADR-0014-capability-detection.md` - objective cluster capability detection
* `ADR-0015-governance-model-v2.md` - admin-selected target cluster after
  namespace environment matching
* `ADR-0018-instance-size-abstraction.md` - InstanceSize capability
  requirements
* `ADR-0040-catalog-scope-for-template-and-instancesize.md` - catalog
  visibility is not cluster policy
* `ADR-0041-power-operation-approval-requirement-service.md` - approval policy
  is separate from cluster runtime policy

### References

* KubeVirt dedicated CPU resources:
  https://kubevirt.io/user-guide/compute/dedicated_cpu_resources/
* KubeVirt NUMA / hugepages prerequisites:
  https://kubevirt.io/user-guide/compute/numa/
* Kubernetes ResourceQuota:
  https://kubernetes.io/docs/concepts/policy/resource-quotas/
* Kubernetes admission controllers / LimitRanger:
  https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/
* Related Issue:
  https://github.com/kv-shepherd/shepherd/issues/326

### Implementation Notes

The first implementation step after acceptance should be a focused issue that:

1. adds `ClusterPolicy` schema and migration
2. introduces a `ClusterPolicyService`
3. integrates policy checks into `ApprovalValidator`
4. exposes read-only policy state on admin cluster detail APIs

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-06 | @jindyzhao | Initial draft |
