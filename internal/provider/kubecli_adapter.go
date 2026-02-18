package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

// KubeconfigLoader resolves cluster kubeconfig bytes by cluster ID/name.
type KubeconfigLoader func(cluster string) ([]byte, error)

// NewClusterClientFactoryFromKubeconfigLoader builds a provider client factory
// backed by kubeconfig bytes loaded from persistence.
func NewClusterClientFactoryFromKubeconfigLoader(loader KubeconfigLoader) ClusterClientFactory {
	factory := &kubeconfigClusterFactory{
		loader: loader,
		cache:  make(map[string]KubeVirtClusterClient),
	}
	return factory.get
}

type kubeconfigClusterFactory struct {
	loader KubeconfigLoader
	mu     sync.RWMutex
	cache  map[string]KubeVirtClusterClient
}

func (f *kubeconfigClusterFactory) get(cluster string) (KubeVirtClusterClient, error) {
	cluster = strings.TrimSpace(cluster)
	if cluster == "" {
		return nil, fmt.Errorf("cluster is required")
	}
	if f.loader == nil {
		return nil, fmt.Errorf("kubeconfig loader is not configured")
	}

	f.mu.RLock()
	if client, ok := f.cache[cluster]; ok {
		f.mu.RUnlock()
		return client, nil
	}
	f.mu.RUnlock()

	kubeconfig, err := f.loader(cluster)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig for cluster %s: %w", cluster, err)
	}
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("cluster %s kubeconfig is empty", cluster)
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig for cluster %s: %w", cluster, err)
	}

	virtClient, err := kubecli.GetKubevirtClientFromRESTConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubevirt client for cluster %s: %w", cluster, err)
	}

	client := &kubevirtClusterClient{client: virtClient}

	f.mu.Lock()
	f.cache[cluster] = client
	f.mu.Unlock()

	return client, nil
}

type kubevirtClusterClient struct {
	client kubecli.KubevirtClient
}

func (c *kubevirtClusterClient) VM() VirtualMachineClient {
	return &kubevirtVMClient{client: c.client}
}

func (c *kubevirtClusterClient) VMI() VirtualMachineInstanceClient {
	return &kubevirtVMIClient{client: c.client}
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

func (c *kubevirtVMClient) Create(ctx context.Context, namespace string, vm *kubevirtv1.VirtualMachine, opts k8smetav1.CreateOptions) (*kubevirtv1.VirtualMachine, error) {
	return c.client.VirtualMachine(namespace).Create(ctx, vm, opts)
}

func (c *kubevirtVMClient) Update(ctx context.Context, namespace string, vm *kubevirtv1.VirtualMachine, opts k8smetav1.UpdateOptions) (*kubevirtv1.VirtualMachine, error) {
	return c.client.VirtualMachine(namespace).Update(ctx, vm, opts)
}

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
