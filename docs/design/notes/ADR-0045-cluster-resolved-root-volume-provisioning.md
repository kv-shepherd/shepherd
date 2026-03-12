# Design Note: ADR-0045 — Cluster-Resolved Root Volume Provisioning

> **Status**: Proposed (ADR-0045 under review until 2026-03-14)  
> **Related ADR**: [ADR-0045](../../adr/ADR-0045-cluster-resolved-root-volume-provisioning.md)  
> **Owner**: @jindyzhao  
> **Created**: 2026-03-12  
> **Last Updated**: 2026-03-12

## Summary

ADR-0045 proposes a lifecycle boundary for root-volume provisioning settings.
`InstanceSize` may keep portable storage intent such as `auto`, but approval and
runtime must never operate on unresolved values. Before approval completes, the
platform resolves storage intent against the target cluster's `StorageProfile`
and persists explicit runtime values for the VM.

## Scope

- In scope: root-volume `storageClass`, `dv_access_modes`, and `dv_volume_mode`
- In scope: approval-time resolution of catalog intent against target-cluster storage capabilities
- In scope: persisting resolved runtime values for later VM updates
- Out of scope: redesigning `Template` vs `InstanceSize` catalog boundaries
- Out of scope: implementing a storage migration workflow for existing VMs

## Pending Changes

Until ADR-0045 is accepted, implementation should follow these constraints:

1. `auto` is allowed only as authoring/catalog intent.
2. Approval must reject completion if root-volume provisioning remains unresolved.
3. If target-cluster storage capability resolution is unique, approval may resolve automatically.
4. If multiple valid combinations remain, approval must require an explicit operator choice.
5. Persist the resolved storage values with the approved VM/runtime state; do not mutate the shared `InstanceSize`.
6. Later CPU/memory/disk updates must reuse persisted resolved values instead of re-running `auto`.

## Affected Components

- `internal/domain/provisioning.go`
- `internal/provider/kubevirt.go`
- `internal/governance/approval/`
- `internal/api/handlers/server_approval*.go`
- `api/openapi.yaml` and generated API types
- `web/src/features/admin-approvals/`
- `web/src/features/admin-instance-sizes/`

## Rollout Notes

- Extend backend storage-profile reads to surface complete supported claim-property combinations.
- Treat approval as the only place where `auto` may be resolved.
- Rendered VM YAML and persisted runtime records must always contain explicit storage values.
- If existing VMs do not yet have persisted resolved values, any future rollout must define a safe fallback based on observed PVC/DataVolume state before allowing ordinary updates.

## Revisit Conditions

- ADR-0045 is accepted, rejected, or superseded
- the project adds a first-class storage migration workflow
- upstream CDI storage-profile semantics change materially
