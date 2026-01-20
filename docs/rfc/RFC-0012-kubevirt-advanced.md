# RFC-0012: KubeVirt Advanced Features

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Advanced KubeVirt features required beyond basic VM lifecycle

> **Implementation Boundary**:
> - **Provider-level methods** (MigrateVM, GetVMMigration, etc.) are implemented in **Phase 2**
> - **This RFC covers Service-level orchestration**: automated migration policies, hot-plug orchestration, maintenance mode

---

## Problem

V1.0 focuses on basic VM lifecycle (create, start, stop, delete). Advanced features may be needed:
- Hot-plug resources (CPU, Memory, Disk, NIC)
- GPU passthrough management
- SR-IOV network configuration
- Memory overcommit controls

---

## Features Overview

### Hot-plug Support

| Feature | KubeVirt Version | Status |
|---------|------------------|--------|
| CPU hot-plug | v1.0+ | GA |
| Memory hot-plug | v1.2+ | Alpha |
| Disk hot-plug | v1.0+ | GA |
| Network hot-plug | v1.4+ | GA |

### GPU Passthrough

```yaml
# Template example
spec:
  domain:
    devices:
      gpus:
        - name: gpu1
          deviceName: nvidia.com/GP100GL
```

### SR-IOV Network

```yaml
spec:
  domain:
    devices:
      interfaces:
        - name: sriov-net
          sriov: {}
  networks:
    - name: sriov-net
      multus:
        networkName: sriov-network
```

---

## Interface Extension

```go
// internal/provider/kubevirt_provider.go

type KubeVirtProvider interface {
    // ... existing methods
    
    // Advanced features
    HotPlugDisk(ctx, vmiName, diskSpec) error
    HotUnplugDisk(ctx, vmiName, diskName) error
    HotPlugNetwork(ctx, vmiName, netSpec) error
    AttachGPU(ctx, vmiName, gpuSpec) error
}
```

---

## Trigger Conditions

- Users need hot-plug resource management
- GPU workloads require passthrough
- High-performance networking (SR-IOV) needed

---

## References

- [KubeVirt Hot-plug](https://kubevirt.io/user-guide/virtual_machines/hotplug_volumes/)
- [KubeVirt GPU Support](https://kubevirt.io/user-guide/virtual_machines/host-devices/)
- [ADR-0014: Capability Detection](../adr/ADR-0014-capability-detection.md)
