# ADR-0001: KubeVirt Client Selection

> **Status**: Accepted  
> **Date**: 2026-01-14

---

## Decision

Use the official KubeVirt `kubevirt.io/client-go` client library.

| Item | Value |
|------|-------|
| Package | `kubevirt.io/client-go` |
| API Definitions | `kubevirt.io/api` |
| Version | See [DEPENDENCIES.md](../design/DEPENDENCIES.md) (single source of truth) |

---

## Context

### Problem

Need to select a Go client library for interacting with KubeVirt. Two options were considered:

1. Generic `k8s.io/client-go` with dynamic client
2. Official `kubevirt.io/client-go`

### Constraints

- Must support all KubeVirt resource types (VM, VMI, VMIPreset, etc.)
- Must provide type-safe operations
- Version must be compatible with target KubeVirt clusters

---

## Options Considered

| Option | Type Safety | Maintainer | Feature Completeness |
|--------|-------------|------------|----------------------|
| `k8s.io/client-go` dynamic client | ❌ `map[string]interface{}` | K8s Official | ⚠️ Manual parsing required |
| `kubevirt.io/client-go` | ✅ Strongly typed | KubeVirt Official | ✅ Full support |

---

## Rationale

### 1. Type Safety

`kubevirt.io/client-go` provides complete type definitions:

```go
import (
    kubevirtv1 "kubevirt.io/api/core/v1"
    "kubevirt.io/client-go/kubecli"
)

// Type-safe VM operations
vm, err := virtClient.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
// vm is *kubevirtv1.VirtualMachine, not map[string]interface{}
```

### 2. Complete Feature Support

Provides clients for all KubeVirt resources:

- `VirtualMachine` / `VirtualMachineInstance`
- `VirtualMachineInstanceMigration`
- `VirtualMachineSnapshot` / `VirtualMachineRestore`
- `VirtualMachineClone`
- `VirtualMachineInstancePreset`
- `VirtualMachineInstanceReplicaSet`

### 3. Official Maintenance

- Released in sync with KubeVirt versions
- API changes automatically reflected in client
- Maintained by official team with quality assurance

### 4. Rich Helper Functions

```go
// kubecli provides convenient methods
virtClient, err := kubecli.GetKubevirtClient()

// Direct subresource operations
err = virtClient.VirtualMachine(namespace).Start(ctx, name, &kubevirtv1.StartOptions{})
err = virtClient.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{})
err = virtClient.VirtualMachine(namespace).Restart(ctx, name, &kubevirtv1.RestartOptions{})
```

---

## Code Examples

### Creating Client

```go
package provider

import (
    "kubevirt.io/client-go/kubecli"
)

type KubeVirtClient struct {
    client kubecli.KubevirtClient
}

func NewKubeVirtClient(kubeconfig string) (*KubeVirtClient, error) {
    // Create client from kubeconfig
    clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
        &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
        &clientcmd.ConfigOverrides{},
    )
    
    restConfig, err := clientConfig.ClientConfig()
    if err != nil {
        return nil, err
    }
    
    virtClient, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
    if err != nil {
        return nil, err
    }
    
    return &KubeVirtClient{client: virtClient}, nil
}
```

### CRUD Operations

```go
func (c *KubeVirtClient) GetVM(ctx context.Context, namespace, name string) (*kubevirtv1.VirtualMachine, error) {
    return c.client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *KubeVirtClient) ListVMs(ctx context.Context, namespace string) (*kubevirtv1.VirtualMachineList, error) {
    return c.client.VirtualMachine(namespace).List(ctx, metav1.ListOptions{})
}

func (c *KubeVirtClient) CreateVM(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
    return c.client.VirtualMachine(vm.Namespace).Create(ctx, vm, metav1.CreateOptions{})
}
```

### VM Lifecycle Operations

```go
func (c *KubeVirtClient) StartVM(ctx context.Context, namespace, name string) error {
    return c.client.VirtualMachine(namespace).Start(ctx, name, &kubevirtv1.StartOptions{})
}

func (c *KubeVirtClient) StopVM(ctx context.Context, namespace, name string) error {
    return c.client.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{})
}

func (c *KubeVirtClient) RestartVM(ctx context.Context, namespace, name string) error {
    return c.client.VirtualMachine(namespace).Restart(ctx, name, &kubevirtv1.RestartOptions{})
}
```

---

## Consequences

### Positive

- Type safety catches errors at compile time
- API stays in sync with KubeVirt versions
- Reduces manual type conversion code
- Official maintenance ensures long-term support

### Negative

- Dependency version must align with `k8s.io/client-go`
- KubeVirt version upgrades may require client upgrades

### Mitigation

- Version constraints documented in `DEPENDENCIES.md`
- CI includes dependency compatibility checks
- Regular tracking of KubeVirt releases

---

## References

- [KubeVirt client-go](https://pkg.go.dev/kubevirt.io/client-go)
- [KubeVirt API](https://pkg.go.dev/kubevirt.io/api)
- [Official Examples](https://github.com/kubevirt/client-go/tree/main/examples)
