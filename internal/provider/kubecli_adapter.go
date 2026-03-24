package provider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// KubeconfigLoader resolves cluster kubeconfig bytes by cluster ID/name.
type KubeconfigLoader func(cluster string) ([]byte, error)

// NewClusterClientFactoryFromKubeconfigLoader builds a provider client factory
// backed by kubeconfig bytes loaded from persistence.
func NewClusterClientFactoryFromKubeconfigLoader(loader KubeconfigLoader) ClusterClientFactory {
	factory := &kubeconfigClusterFactory{
		loader:      loader,
		buildClient: buildClusterClientFromKubeconfig,
		cache:       make(map[string]kubeconfigClusterCacheEntry),
	}
	return factory.get
}

type kubeconfigClusterCacheEntry struct {
	kubeconfigHash [sha256.Size]byte
	client         KubeVirtClusterClient
}

type kubeconfigClusterFactory struct {
	loader      KubeconfigLoader
	buildClient func(cluster string, kubeconfig []byte) (KubeVirtClusterClient, error)
	mu          sync.RWMutex
	cache       map[string]kubeconfigClusterCacheEntry
}

func (f *kubeconfigClusterFactory) get(cluster string) (KubeVirtClusterClient, error) {
	cluster = strings.TrimSpace(cluster)
	if cluster == "" {
		return nil, fmt.Errorf("cluster is required")
	}
	if f.loader == nil {
		return nil, fmt.Errorf("kubeconfig loader is not configured")
	}

	kubeconfig, err := f.loader(cluster)
	if err != nil {
		f.evict(cluster)
		return nil, fmt.Errorf("load kubeconfig for cluster %s: %w", cluster, err)
	}
	if len(kubeconfig) == 0 {
		f.evict(cluster)
		return nil, fmt.Errorf("cluster %s kubeconfig is empty", cluster)
	}

	kubeconfigHash := sha256.Sum256(kubeconfig)

	f.mu.RLock()
	if entry, ok := f.cache[cluster]; ok && entry.kubeconfigHash == kubeconfigHash {
		f.mu.RUnlock()
		return entry.client, nil
	}
	f.mu.RUnlock()

	client, err := f.buildClient(cluster, kubeconfig)
	if err != nil {
		f.evict(cluster)
		return nil, err
	}

	f.mu.Lock()
	f.cache[cluster] = kubeconfigClusterCacheEntry{
		kubeconfigHash: kubeconfigHash,
		client:         client,
	}
	f.mu.Unlock()

	return client, nil
}

func (f *kubeconfigClusterFactory) evict(cluster string) {
	f.mu.Lock()
	delete(f.cache, cluster)
	f.mu.Unlock()
}

func buildClusterClientFromKubeconfig(cluster string, kubeconfig []byte) (KubeVirtClusterClient, error) {
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig for cluster %s: %w", cluster, err)
	}

	virtClient, err := kubecli.GetKubevirtClientFromRESTConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubevirt client for cluster %s: %w", cluster, err)
	}

	// Build dynamic client from the same REST config for SSA operations (ADR-0011).
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client for cluster %s: %w", cluster, err)
	}

	return &kubevirtClusterClient{
		client:    virtClient,
		ssaClient: NewKubevirtSSAApplier(dynClient),
	}, nil
}

type kubevirtClusterClient struct {
	client    kubecli.KubevirtClient
	ssaClient *KubevirtSSAApplier
}

func (c *kubevirtClusterClient) VM() VirtualMachineClient {
	return &kubevirtVMClient{client: c.client}
}

func (c *kubevirtClusterClient) VMI() VirtualMachineInstanceClient {
	return &kubevirtVMIClient{client: c.client}
}

func (c *kubevirtClusterClient) DataVolume() DataVolumeClient {
	return &kubevirtDataVolumeClient{client: c.client}
}

func (c *kubevirtClusterClient) StorageProfile() StorageProfileClient {
	return &kubevirtStorageProfileClient{client: c.client}
}

func (c *kubevirtClusterClient) PVC() PersistentVolumeClaimClient {
	return &kubevirtPVCClient{client: c.client}
}

func (c *kubevirtClusterClient) StorageClass() StorageClassClient {
	return &kubevirtStorageClassClient{client: c.client}
}

func (c *kubevirtClusterClient) Events() EventClient {
	return &kubevirtEventClient{client: c.client}
}

func (c *kubevirtClusterClient) Namespaces() NamespaceClient {
	return &kubevirtNamespaceClient{client: c.client}
}

func (c *kubevirtClusterClient) Pods() PodClient {
	return &kubevirtPodClient{client: c.client}
}

func (c *kubevirtClusterClient) Authorization() AuthorizationClient {
	return &kubevirtAuthorizationClient{client: c.client}
}

// SSA returns the DynamicSSAClient for Server-Side Apply operations (ADR-0011).
func (c *kubevirtClusterClient) SSA() DynamicSSAClient {
	return c.ssaClient
}

type kubevirtVMClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtVMClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	return c.client.VirtualMachine(namespace).Get(ctx, name, opts)
}

func (c *kubevirtVMClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	return c.client.VirtualMachine(namespace).List(ctx, opts)
}

// Create and Update are intentionally removed (ADR-0011).
// All VM writes use DynamicSSAClient.ApplyYAML() via SSA().

func (c *kubevirtVMClient) Delete(ctx context.Context, namespace, name string, opts k8smetav1.DeleteOptions) error {
	return c.client.VirtualMachine(namespace).Delete(ctx, name, opts)
}

func (c *kubevirtVMClient) Start(ctx context.Context, namespace, name string, opts *kubevirtv1.StartOptions) error {
	return c.client.VirtualMachine(namespace).Start(ctx, name, opts)
}

func (c *kubevirtVMClient) Stop(ctx context.Context, namespace, name string, opts *kubevirtv1.StopOptions) error {
	return c.client.VirtualMachine(namespace).Stop(ctx, name, opts)
}

func (c *kubevirtVMClient) Restart(ctx context.Context, namespace, name string, opts *kubevirtv1.RestartOptions) error {
	return c.client.VirtualMachine(namespace).Restart(ctx, name, opts)
}

type kubevirtVMIClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtVMIClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*kubevirtv1.VirtualMachineInstance, error) {
	return c.client.VirtualMachineInstance(namespace).Get(ctx, name, opts)
}

func (c *kubevirtVMIClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceList, error) {
	return c.client.VirtualMachineInstance(namespace).List(ctx, opts)
}

func (c *kubevirtVMIClient) Pause(ctx context.Context, namespace, name string, opts *kubevirtv1.PauseOptions) error {
	return c.client.VirtualMachineInstance(namespace).Pause(ctx, name, opts)
}

func (c *kubevirtVMIClient) Unpause(ctx context.Context, namespace, name string, opts *kubevirtv1.UnpauseOptions) error {
	return c.client.VirtualMachineInstance(namespace).Unpause(ctx, name, opts)
}

type kubevirtDataVolumeClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtDataVolumeClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*cdiv1beta1.DataVolume, error) {
	return c.client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, name, opts)
}

func (c *kubevirtDataVolumeClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*cdiv1beta1.DataVolumeList, error) {
	return c.client.CdiClient().CdiV1beta1().DataVolumes(namespace).List(ctx, opts)
}

type kubevirtStorageProfileClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtStorageProfileClient) Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*cdiv1beta1.StorageProfile, error) {
	return c.client.CdiClient().CdiV1beta1().StorageProfiles().Get(ctx, name, opts)
}

type kubevirtPVCClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtPVCClient) Get(ctx context.Context, namespace, name string, opts k8smetav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	return c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, opts)
}

type kubevirtStorageClassClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtStorageClassClient) Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*storagev1.StorageClass, error) {
	return c.client.StorageV1().StorageClasses().Get(ctx, name, opts)
}

func (c *kubevirtStorageClassClient) List(ctx context.Context, opts k8smetav1.ListOptions) (*storagev1.StorageClassList, error) {
	return c.client.StorageV1().StorageClasses().List(ctx, opts)
}

type kubevirtEventClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtEventClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*corev1.EventList, error) {
	return c.client.CoreV1().Events(namespace).List(ctx, opts)
}

type kubevirtNamespaceClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtNamespaceClient) Get(ctx context.Context, name string, opts k8smetav1.GetOptions) (*corev1.Namespace, error) {
	return c.client.CoreV1().Namespaces().Get(ctx, name, opts)
}

type kubevirtPodClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtPodClient) List(ctx context.Context, namespace string, opts k8smetav1.ListOptions) (*corev1.PodList, error) {
	return c.client.CoreV1().Pods(namespace).List(ctx, opts)
}

type kubevirtAuthorizationClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtAuthorizationClient) CreateSelfSubjectAccessReview(
	ctx context.Context,
	review *authorizationv1.SelfSubjectAccessReview,
	opts k8smetav1.CreateOptions,
) (*authorizationv1.SelfSubjectAccessReview, error) {
	return c.client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, opts)
}

// KubeVirt returns a KubeVirtCRClient that can read the cluster-level KubeVirt CR.
// Used by CapabilityDetector during health checks to fetch live featureGates (ADR-0014).
func (c *kubevirtClusterClient) KubeVirt() KubeVirtCRClient {
	return &kubevirtKVCRClient{client: c.client}
}

// kubevirtKVCRClient implements KubeVirtCRClient via kubecli.KubevirtClient.
// The KubeVirt CR is always: namespace="kubevirt", name="kubevirt" (singleton per cluster).
//
// Optimization: the CR object is fetched once per instance (sync.Once) and shared
// between GetVersion() and GetFeatureGates(). Since KubeVirt() creates a new instance
// per health check cycle, the cache is naturally scoped per cycle — not per application
// lifetime. This halves K8s API calls from 2 to 1 per cluster per health check.
type kubevirtKVCRClient struct {
	client kubecli.KubevirtClient
	// lazy-loaded CR object (sync.Once ensures exactly one GET per instance).
	crOnce sync.Once
	crObj  *kubevirtv1.KubeVirt
	crErr  error
}

// loadCR performs the single K8s GET for the KubeVirt CR, cached via sync.Once.
func (c *kubevirtKVCRClient) loadCR(ctx context.Context) (*kubevirtv1.KubeVirt, error) {
	c.crOnce.Do(func() {
		c.crObj, c.crErr = c.client.KubeVirt(kubeVirtNamespace).Get(ctx, kubeVirtCRName, k8smetav1.GetOptions{})
		if c.crErr != nil {
			c.crErr = fmt.Errorf("get kubevirt CR %s/%s: %w", kubeVirtNamespace, kubeVirtCRName, c.crErr)
		}
	})
	return c.crObj, c.crErr
}

// GetFeatureGates reads the KubeVirt CR and returns its explicitly configured feature gates.
//
// Source: spec.configuration.developerConfiguration.featureGates ([]string).
// Features omitted here may still be active if they have GA-graduated to always-on by version.
//
// Returns nil slice when:
//   - DeveloperConfiguration is nil (no explicit gates, use GA table only)
//   - FeatureGates slice is empty
//
// Returns error only on API failure (permission, unreachable, etc.)
func (c *kubevirtKVCRClient) GetFeatureGates(ctx context.Context) ([]string, error) {
	kv, err := c.loadCR(ctx)
	if err != nil {
		return nil, err
	}
	if kv.Spec.Configuration.DeveloperConfiguration == nil {
		return nil, nil // no explicit feature gates configured
	}
	gates := kv.Spec.Configuration.DeveloperConfiguration.FeatureGates
	if len(gates) == 0 {
		return nil, nil
	}
	// Return a copy to avoid caller mutation of the CR object's slice.
	result := make([]string, len(gates))
	copy(result, gates)
	return result, nil
}

// GetVersion reads the KubeVirt CR and returns the observed running KubeVirt version.
//
// Source: status.observedKubeVirtVersion
//   - Populated by the KubeVirt operator once it has successfully reconciled the deployment.
//   - Returns empty string (not an error) when the operator has not yet filled the field
//     (e.g., freshly installed cluster, operator still rolling out).
//
// Returns error only on API failure (permission, unreachable, etc.)
func (c *kubevirtKVCRClient) GetVersion(ctx context.Context) (string, error) {
	kv, err := c.loadCR(ctx)
	if err != nil {
		return "", err
	}
	// status.observedKubeVirtVersion is the authoritative running version.
	// It may differ from spec: an in-progress upgrade will show the old version here
	// until the operator finishes. This is intentional — we want the *running* version
	// for GA feature table lookups, not the desired version.
	return kv.Status.ObservedKubeVirtVersion, nil
}

const (
	// kubeVirtNamespace is the standard namespace where the KubeVirt CR resides.
	// Per KubeVirt convention, the operator always installs in "kubevirt" namespace.
	kubeVirtNamespace = "kubevirt"

	// kubeVirtCRName is the standard name of the singleton KubeVirt CR.
	// Per KubeVirt convention, there is exactly one KubeVirt CR per cluster named "kubevirt".
	kubeVirtCRName = "kubevirt"
)
