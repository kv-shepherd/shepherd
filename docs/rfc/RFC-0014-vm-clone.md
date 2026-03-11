# RFC-0014: VM Clone

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Rapid VM duplication required

> **Implementation Boundary**:
> - **Provider interfaces and domain types** are defined in Phase 1-2
> - **Runtime clone provider methods** are **not** implemented in V1
> - **This RFC covers both future provider implementation and service-level orchestration**: data masking, cross-cluster clone, CI/CD integration

## Current State (2026-03-11)

The `CloneProvider` seam exists in `internal/provider/interface.go`, but no
KubeVirt-backed clone runtime is currently wired into services or handlers.
Treat this RFC as deferred capability work rather than completed Phase 2 scope.

---

## Problem

Users may need to quickly duplicate existing VMs for:
- Scaling similar workloads
- Creating test environments from production
- Disaster recovery scenarios

---

## Proposed Solution

### Clone Operations

```go
type CloneService struct {
    provider provider.KubeVirtProvider
}

// CloneVM creates a clone from existing VM
func (s *CloneService) CloneVM(ctx context.Context, input CloneVMInput) (*Clone, error) {
    return s.provider.CloneVM(
        ctx,
        input.ClusterName,
        input.Namespace,
        input.SourceVMName,
        input.TargetName,
    )
}
```

### API Endpoints

```
POST /api/v1/vms/{id}/clone
GET  /api/v1/clones/{id}
```

### Clone Request

```json
POST /api/v1/vms/vm-001/clone
{
    "target_name": "vm-001-clone",
    "target_namespace": "production",
    "start_after_clone": true
}
```

---

## Prerequisites

- VirtualMachineClone CRD (KubeVirt v1.1+)
- Sufficient storage for cloned volumes
- Clone feature gate enabled

---

## Trigger Conditions

- Need to duplicate VMs quickly
- Environment templating from existing VMs
- Disaster recovery preparation

---

## References

- [KubeVirt Clone](https://kubevirt.io/user-guide/virtual_machines/clone_api/)
- [ADR-0004: Provider Interface](../adr/ADR-0004-provider-interface.md)
