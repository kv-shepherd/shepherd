# Design Note: ADR-0042 — Explicit Cluster Policy Governance Model

> **Status**: Proposed
> **Related ADR**: [ADR-0042](../../adr/ADR-0042-cluster-policy-governance-model.md)
> **Owner**: @jindyzhao
> **Date**: 2026-03-06

## Summary

ADR-0042 proposes a dedicated `ClusterPolicy` model so the platform can decide
not only whether a cluster technically supports a workload, but also whether the
platform allows that workload on that cluster. This note captures the proposed
schema, enforcement order, API impact, and migration approach while the ADR
remains under review.

## Scope

- In scope: one-to-one `ClusterPolicy` persistence keyed by `cluster_id`
- In scope: policy evaluation after capability matching in request approval and
  cluster selection
- In scope: initial policy controls for overcommit, hardware controls, and CDI
  clone governance
- In scope: admin API visibility for detected capability vs effective policy
- Out of scope: quota mirroring from Kubernetes objects
- Out of scope: per-namespace policy overrides
- Out of scope: `containerdisk` product policy decisions

## Pending Changes (Not Yet Normative)

- Affected docs:
  - `docs/adr/ADR-0042-cluster-policy-governance-model.md`
  - governance design docs that describe cluster selection and approval-time
    validation
  - admin cluster-management docs
- Affected components:
  - `ent/schema/cluster_policy.go` (new)
  - `internal/service/cluster_policy.go` (new)
  - `internal/service/approval_validator.go`
  - admin cluster APIs / OpenAPI contract
  - cluster selection helpers that currently stop at capability matching
- Behavior changes:
  - capability match is no longer sufficient for cluster eligibility
  - policy denial reasons become explicit and user-visible at approval time
  - new cluster onboarding requires explicit policy creation before the cluster
    is considered schedulable

## Proposed Schema Shape

```go
type ClusterPolicy struct {
    ClusterID                    string
    AllowCPUOvercommit           bool
    AllowMemoryOvercommit        bool
    AllowDedicatedCPU            bool
    AllowGPU                     bool
    AllowSRIOV                   bool
    AllowHugepages               bool
    AllowedHugepagesSizes        []string
    AllowCDIClone                bool
    AllowedCloneSourceNamespaces []string
    AllowedStorageClasses        []string
}
```

### Interpretation Rules

- `Allow* = false` means hard deny, even if cluster capability exists
- empty allowlists mean "no additional narrowing" rather than deny-all
- capability remains authoritative for what the cluster can do
- policy remains authoritative for what Shepherd may place there

## Enforcement Touchpoints

### Approval-Time Validation

`ApprovalValidator` should gain a policy check step after capability matching.
Representative denials:

- dedicated CPU requested but `allow_dedicated_cpu=false`
- CPU overcommit requested but `allow_cpu_overcommit=false`
- hugepages `2Mi` requested but not in `allowed_hugepages_sizes`
- PVC clone requested from namespace outside `allowed_clone_source_namespaces`
- selected storage class not in `allowed_storage_classes`

### Cluster Selection

Compatible cluster listing must eventually filter by:

1. environment type
2. health / enabled state
3. detected capabilities
4. explicit cluster policy

### Admin APIs

Admin-facing cluster detail should expose both:

- detected capability view (`enabled_features`, storage classes, versions)
- configured policy view (`ClusterPolicy`)

The UI should not merge them into a single undifferentiated feature list.

## Migration / Rollout

1. Add `cluster_policies` table and one-to-one relation
2. Backfill existing clusters with explicit `legacy-compatible` policy rows
3. Update admin APIs so cluster onboarding can create or edit policy
4. Add validator integration behind the new policy service
5. Tighten defaults only after admin review/backfill is complete

## Open Questions

- Should `allowed_storage_classes` treat the cluster default storage class as an
  implicit allow, or must it also be explicitly listed?
- Should clone-source namespace restrictions live here permanently, or move to a
  future storage-policy entity if the matrix grows?
- Does the first rollout need a separate `ClusterPolicyPreset` concept, or is a
  one-time migration/backfill script sufficient?
