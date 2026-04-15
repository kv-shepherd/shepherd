package provider

import (
	"context"
	"net"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
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

	// ApplyClusterScopedYAML submits cluster-scoped YAML bytes as an SSA Patch.
	// Used for non-namespaced resources such as Namespace.
	ApplyClusterScopedYAML(ctx context.Context, gvr schema.GroupVersionResource, yamlData []byte) (*unstructured.Unstructured, error)

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
	Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts k8smetav1.PatchOptions, subresources ...string) (*kubevirtv1.VirtualMachine, error)
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
	VNC(namespace, name string, preserveSession bool) (net.Conn, error)
	SerialConsole(namespace, name string, connectionTimeout time.Duration) (net.Conn, error)
}

// DataVolumeClient abstracts CDI DataVolume read operations.
type DataVolumeClient interface {
	Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*cdiv1beta1.DataVolume, error)
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*cdiv1beta1.DataVolumeList, error)
}

// StorageProfileClient abstracts CDI StorageProfile reads.
type StorageProfileClient interface {
	Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*cdiv1beta1.StorageProfile, error)
}

// PersistentVolumeClaimClient abstracts PVC read operations.
type PersistentVolumeClaimClient interface {
	Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*corev1.PersistentVolumeClaim, error)
}

// StorageClassClient abstracts cluster-scoped StorageClass reads.
type StorageClassClient interface {
	Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*storagev1.StorageClass, error)
	List(ctx context.Context, opts k8smetav1.ListOptions) (*storagev1.StorageClassList, error)
}

// EventClient abstracts namespace-scoped Kubernetes Event reads.
type EventClient interface {
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*corev1.EventList, error)
}

// NamespaceClient abstracts cluster-scoped Namespace reads.
type NamespaceClient interface {
	Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*corev1.Namespace, error)
}

// NodeClient abstracts cluster-scoped Node reads used for host placement enrichment.
type NodeClient interface {
	Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*corev1.Node, error)
	List(ctx context.Context, opts k8smetav1.ListOptions) (*corev1.NodeList, error)
}

// PodClient abstracts namespace-scoped Pod reads used for PVC clone preflight checks.
type PodClient interface {
	List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*corev1.PodList, error)
}

// AuthorizationClient abstracts access reviews needed for CDI clone RBAC preflight.
type AuthorizationClient interface {
	CreateSelfSubjectAccessReview(
		ctx context.Context,
		review *authorizationv1.SelfSubjectAccessReview,
		opts k8smetav1.CreateOptions,
	) (*authorizationv1.SelfSubjectAccessReview, error)
}

// KubeVirtCRClient provides access to the cluster-scoped KubeVirt CR.
// Used by CapabilityDetector to fetch enabled feature gates and running version (ADR-0014).
//
// The KubeVirt CR is always: namespace="kubevirt", name="kubevirt".
// Separation from VirtualMachineClient keeps the VM CRUD ACL from CR read ACL.
type KubeVirtCRClient interface {
	// GetFeatureGates fetches explicitly configured feature gates from the cluster-level KubeVirt CR.
	// Source: spec.configuration.developerConfiguration.featureGates ([]string).
	// Returns nil slice (not error) if DeveloperConfiguration is nil or FeatureGates is empty.
	// Returns error only on API failure (e.g., permission denied, cluster unreachable).
	GetFeatureGates(ctx context.Context) ([]string, error)

	// GetVersion fetches the observed running KubeVirt version from the cluster-level KubeVirt CR.
	// Source: status.observedKubeVirtVersion (set by the KubeVirt operator on successful reconciliation).
	// Returns empty string (not error) if the field is not yet populated (e.g., operator still deploying).
	// Returns error only on API failure (e.g., permission denied, cluster unreachable).
	GetVersion(ctx context.Context) (string, error)
}

// KubeVirtClusterClient provides kubevirt clients for a specific cluster.
// Composition root creates the actual implementation using kubecli.
type KubeVirtClusterClient interface {
	VM() VirtualMachineClient          // Read + lifecycle (type-safe)
	VMI() VirtualMachineInstanceClient // VMI read + pause/unpause
	DataVolume() DataVolumeClient      // CDI DataVolume reads for provisioning observability
	StorageProfile() StorageProfileClient
	PVC() PersistentVolumeClaimClient   // PVC reads for provisioning observability
	StorageClass() StorageClassClient   // StorageClass reads for clone expansion preflight
	Events() EventClient                // CoreV1 Events for best-effort failure summaries
	Namespaces() NamespaceClient        // CoreV1 Namespaces for idempotent namespace creation
	Nodes() NodeClient                  // CoreV1 Nodes for host placement enrichment
	Pods() PodClient                    // CoreV1 Pods for PVC clone in-use preflight
	Authorization() AuthorizationClient // SAR for CDI clone source RBAC preflight
	SSA() DynamicSSAClient              // Write: CreateVM/UpdateVM (Unstructured SSA, ADR-0011)
	KubeVirt() KubeVirtCRClient         // KubeVirt CR access for capability detection (ADR-0014)
}

// ClusterClientFactory creates KubeVirtClusterClient for a given cluster name.
type ClusterClientFactory func(clusterName string) (KubeVirtClusterClient, error)
