package provider

import (
	"context"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// DynamicSSAClient submits unstructured resources via Server-Side Apply.
// Used for all VM write operations (CreateVM / UpdateVM / ValidateSpec).
//
// ADR-0011: Backend is a "YAML porter", not a "Struct assembly factory".
// All VM writes go through rendered YAML → Unstructured → SSA Patch.
type DynamicSSAClient interface {
	// ApplyYAML submits YAML bytes as an SSA Patch to Kubernetes.
	// fieldManager is always FieldOwner ("kubevirt-shepherd").
	ApplyYAML(ctx context.Context, namespace string, yamlData []byte) (*unstructured.Unstructured, error)

	// DryRunApplyYAML validates YAML via SSA DryRun without creating the resource.
	DryRunApplyYAML(ctx context.Context, namespace string, yamlData []byte) error
}

// VirtualMachineClient abstracts KubeVirt VM read operations and lifecycle commands.
// Anti-Corruption Layer: decouples provider from kubevirt.io/client-go/kubecli.
//
// Create and Update are intentionally absent (ADR-0011):
// All writes must go through DynamicSSAClient.ApplyYAML().
type VirtualMachineClient interface {
	// Read operations (type-safe via kubevirt.io/client-go)
	Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error)
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error)
	// Delete remains on typed client (not SSA-related, standard K8s operation)
	Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error
	// Lifecycle sub-resource methods (stable across KubeVirt versions)
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
	VM() VirtualMachineClient          // Read + lifecycle (type-safe)
	VMI() VirtualMachineInstanceClient // VMI read + pause/unpause
	SSA() DynamicSSAClient             // Write: CreateVM/UpdateVM (Unstructured SSA, ADR-0011)
}

// ClusterClientFactory creates KubeVirtClusterClient for a given cluster name.
type ClusterClientFactory func(clusterName string) (KubeVirtClusterClient, error)
