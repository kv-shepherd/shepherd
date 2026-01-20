# RFC-0014: VM Clone

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Rapid VM duplication required

> **Implementation Boundary**:
> - **Provider-level methods** (CloneVM, GetVMClone, etc.) are implemented in **Phase 2**
> - **This RFC covers Service-level orchestration**: data masking, cross-cluster clone, CI/CD integration

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
    return s.provider.CloneVM(ctx, CloneSpec{
        SourceVMName:  input.SourceVMName,
        Namespace:     input.Namespace,
        ClusterName:   input.ClusterName,
        TargetName:    input.TargetName,
        TargetNS:      input.TargetNamespace,
    })
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
