package provider

import (
	"context"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// VirtualMachineClient abstracts KubeVirt VM operations.
// Anti-Corruption Layer: decouples provider from kubevirt.io/client-go/kubecli.
// Actual kubecli binding is done at composition root level.
type VirtualMachineClient interface {
	Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error)
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error)
	Create(ctx context.Context, namespace string, vm *kubevirtv1.VirtualMachine, opts k8smetav1.CreateOptions) (*kubevirtv1.VirtualMachine, error)
	Update(ctx context.Context, namespace string, vm *kubevirtv1.VirtualMachine, opts k8smetav1.UpdateOptions) (*kubevirtv1.VirtualMachine, error)
	Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error
	Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error
	Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error
	Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error
}

// VirtualMachineInstanceClient abstracts KubeVirt VMI operations.
type VirtualMachineInstanceClient interface {
	Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachineInstance, error)
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceList, error)
	Pause(ctx context.Context, namespace, name string, opts *kubevirtv1.PauseOptions) error
	Unpause(ctx context.Context, namespace, name string, opts *kubevirtv1.UnpauseOptions) error
}

// KubeVirtClusterClient provides kubevirt clients for a specific cluster.
// Composition root creates the actual implementation using kubecli.
type KubeVirtClusterClient interface {
	VM() VirtualMachineClient
	VMI() VirtualMachineInstanceClient
}

// ClusterClientFactory creates KubeVirtClusterClient for a given cluster name.
type ClusterClientFactory func(clusterName string) (KubeVirtClusterClient, error)
