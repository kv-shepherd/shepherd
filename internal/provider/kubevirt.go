package provider

import (
	"context"
	"fmt"
	"time"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"kv-shepherd.io/shepherd/internal/domain"
)

// KubeVirtProviderImpl implements KubeVirtProvider using our client abstraction.
// ADR-0001: Use official kubevirt.io/client-go client (bound at composition root).
// ADR-0004: Interface composition (implements InfrastructureProvider + sub-providers).
type KubeVirtProviderImpl struct {
	clientFactory    ClusterClientFactory
	mapper           *KubeVirtMapper
	operationTimeout time.Duration // ISSUE-011: enforce K8s op timeout
}

// NewKubeVirtProvider creates a new KubeVirtProvider.
// clientFactory creates a cluster client for the specified cluster.
func NewKubeVirtProvider(clientFactory ClusterClientFactory, operationTimeout time.Duration) *KubeVirtProviderImpl {
	if operationTimeout <= 0 {
		operationTimeout = 5 * time.Minute // same default as config.go
	}
	return &KubeVirtProviderImpl{
		clientFactory:    clientFactory,
		mapper:           NewKubeVirtMapper(),
		operationTimeout: operationTimeout,
	}
}

// withTimeout wraps ctx with the configured K8s operation timeout.
func (p *KubeVirtProviderImpl) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.operationTimeout)
}

// Name returns the provider name.
func (p *KubeVirtProviderImpl) Name() string { return "kubevirt" }

// Type returns the provider type.
func (p *KubeVirtProviderImpl) Type() string { return "kubevirt" }

// GetVM retrieves a VM from the specified cluster.
func (p *KubeVirtProviderImpl) GetVM(ctx context.Context, cluster, namespace, name string) (*domain.VM, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	vm, err := client.VM().Get(ctx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get vm %s/%s: %w", namespace, name, err)
	}

	// Try to get VMI for status enrichment
	vmi, _ := client.VMI().Get(ctx, namespace, name, k8smetav1.GetOptions{})

	return p.mapper.MapVM(vm, vmi)
}

// ListVMs lists VMs in the specified namespace.
func (p *KubeVirtProviderImpl) ListVMs(ctx context.Context, cluster, namespace string, opts ListOptions) (*domain.VMList, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	listOpts := k8smetav1.ListOptions{}
	if opts.LabelSelector != "" {
		listOpts.LabelSelector = opts.LabelSelector
	}
	if opts.Limit > 0 {
		listOpts.Limit = int64(opts.Limit)
	}
	if opts.Continue != "" {
		listOpts.Continue = opts.Continue
	}

	vmList, err := client.VM().List(ctx, namespace, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list vms in %s: %w", namespace, err)
	}

	// Batch fetch VMIs for status enrichment
	vmiList, _ := client.VMI().List(ctx, namespace, k8smetav1.ListOptions{})
	var vmis []kubevirtv1.VirtualMachineInstance
	if vmiList != nil {
		vmis = vmiList.Items
	}

	result, err := p.mapper.MapVMList(vmList.Items, vmis)
	if err != nil {
		return nil, fmt.Errorf("map vm list: %w", err)
	}

	if vmList.Continue != "" {
		result.Continue = vmList.Continue
	}

	return result, nil
}

// CreateVM creates a VM via SSA Apply (ADR-0011).
func (p *KubeVirtProviderImpl) CreateVM(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.VM, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	vm := buildVMFromSpec(namespace, spec)
	created, err := client.VM().Create(opCtx, namespace, vm, k8smetav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create vm in %s: %w", namespace, err)
	}

	return p.mapper.MapVM(created, nil)
}

// UpdateVM updates a VM specification.
func (p *KubeVirtProviderImpl) UpdateVM(ctx context.Context, cluster, namespace, name string, spec *domain.VMSpec) (*domain.VM, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	existing, err := client.VM().Get(ctx, namespace, name, k8smetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get vm %s/%s for update: %w", namespace, name, err)
	}

	applySpecToVM(existing, spec)
	updated, err := client.VM().Update(ctx, namespace, existing, k8smetav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update vm %s/%s: %w", namespace, name, err)
	}

	return p.mapper.MapVM(updated, nil)
}

// DeleteVM deletes a VM.
func (p *KubeVirtProviderImpl) DeleteVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()

	return client.VM().Delete(opCtx, namespace, name, k8smetav1.DeleteOptions{})
}

// StartVM starts a stopped VM.
func (p *KubeVirtProviderImpl) StartVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Start(opCtx, namespace, name, &kubevirtv1.StartOptions{})
}

// StopVM stops a running VM.
func (p *KubeVirtProviderImpl) StopVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Stop(opCtx, namespace, name, &kubevirtv1.StopOptions{})
}

// RestartVM restarts a VM.
func (p *KubeVirtProviderImpl) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VM().Restart(opCtx, namespace, name, &kubevirtv1.RestartOptions{})
}

// PauseVM pauses a running VM.
func (p *KubeVirtProviderImpl) PauseVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VMI().Pause(opCtx, namespace, name, &kubevirtv1.PauseOptions{})
}

// UnpauseVM unpauses a paused VM.
func (p *KubeVirtProviderImpl) UnpauseVM(ctx context.Context, cluster, namespace, name string) error {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}
	opCtx, cancel := p.withTimeout(ctx)
	defer cancel()
	return client.VMI().Unpause(opCtx, namespace, name, &kubevirtv1.UnpauseOptions{})
}

// ValidateSpec performs dry-run validation (ADR-0011).
func (p *KubeVirtProviderImpl) ValidateSpec(ctx context.Context, cluster, namespace string, spec *domain.VMSpec) (*domain.ValidationResult, error) {
	client, err := p.clientFactory(cluster)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", cluster, err)
	}

	vm := buildVMFromSpec(namespace, spec)
	_, err = client.VM().Create(ctx, namespace, vm, k8smetav1.CreateOptions{
		DryRun: []string{k8smetav1.DryRunAll},
	})
	if err != nil {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}, nil
	}

	return &domain.ValidationResult{Valid: true}, nil
}

// buildVMFromSpec creates a KubeVirt VM object from a domain spec.
func buildVMFromSpec(namespace string, spec *domain.VMSpec) *kubevirtv1.VirtualMachine {
	running := true
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: k8smetav1.ObjectMeta{
			Namespace: namespace,
			Labels:    spec.Labels,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running:  &running,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
		},
	}
	return vm
}

// applySpecToVM applies domain spec changes to an existing K8s VM.
func applySpecToVM(vm *kubevirtv1.VirtualMachine, spec *domain.VMSpec) {
	if vm.Spec.Template == nil {
		vm.Spec.Template = &kubevirtv1.VirtualMachineInstanceTemplateSpec{}
	}
	if spec.Labels != nil {
		if vm.Labels == nil {
			vm.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			vm.Labels[k] = v
		}
	}
}
