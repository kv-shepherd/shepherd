package provider

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	vm, err := buildVMFromSpec(namespace, spec)
	if err != nil {
		return nil, fmt.Errorf("build vm from spec: %w", err)
	}
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

	if err := applySpecToVM(existing, spec); err != nil {
		return nil, fmt.Errorf("apply vm spec overrides: %w", err)
	}
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

	vm, err := buildVMFromSpec(namespace, spec)
	if err != nil {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}, nil
	}
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
func buildVMFromSpec(namespace string, spec *domain.VMSpec) (*kubevirtv1.VirtualMachine, error) {
	if spec == nil {
		return nil, fmt.Errorf("vm spec is nil")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, fmt.Errorf("vm name is required")
	}
	if spec.CPU <= 0 {
		return nil, fmt.Errorf("vm cpu must be > 0")
	}
	if spec.MemoryMB <= 0 {
		return nil, fmt.Errorf("vm memory_mb must be > 0")
	}
	image := strings.TrimSpace(spec.Image)
	if image == "" {
		return nil, fmt.Errorf("vm image is required")
	}

	running := true
	cpuQty := resource.MustParse(fmt.Sprintf("%d", spec.CPU))
	memQty := resource.MustParse(fmt.Sprintf("%dMi", spec.MemoryMB))

	volumes, disks, err := buildDisksAndVolumes(image, spec.DiskGB)
	if err != nil {
		return nil, err
	}

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: k8smetav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    spec.Labels,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &running,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: k8smetav1.ObjectMeta{
					Labels: spec.Labels,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						CPU: &kubevirtv1.CPU{
							Cores: uint32(spec.CPU),
						},
						Resources: kubevirtv1.ResourceRequirements{
							Requests: k8sv1.ResourceList{
								k8sv1.ResourceCPU:    cpuQty,
								k8sv1.ResourceMemory: memQty,
							},
							Limits: k8sv1.ResourceList{
								k8sv1.ResourceCPU:    cpuQty,
								k8sv1.ResourceMemory: memQty,
							},
						},
						Devices: kubevirtv1.Devices{
							Disks: disks,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	if err := applySpecOverrides(vm, spec.SpecOverrides); err != nil {
		return nil, fmt.Errorf("apply spec_overrides: %w", err)
	}
	return vm, nil
}

// applySpecToVM applies domain spec changes to an existing K8s VM.
func applySpecToVM(vm *kubevirtv1.VirtualMachine, spec *domain.VMSpec) error {
	if vm == nil || spec == nil {
		return nil
	}
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
		if vm.Spec.Template.ObjectMeta.Labels == nil {
			vm.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			vm.Spec.Template.ObjectMeta.Labels[k] = v
		}
	}
	if spec.CPU > 0 {
		vm.Spec.Template.Spec.Domain.CPU = &kubevirtv1.CPU{Cores: uint32(spec.CPU)}
	}
	if spec.CPU > 0 || spec.MemoryMB > 0 {
		if vm.Spec.Template.Spec.Domain.Resources.Requests == nil {
			vm.Spec.Template.Spec.Domain.Resources.Requests = k8sv1.ResourceList{}
		}
		if vm.Spec.Template.Spec.Domain.Resources.Limits == nil {
			vm.Spec.Template.Spec.Domain.Resources.Limits = k8sv1.ResourceList{}
		}
	}
	if spec.CPU > 0 {
		cpuQty := resource.MustParse(fmt.Sprintf("%d", spec.CPU))
		vm.Spec.Template.Spec.Domain.Resources.Requests[k8sv1.ResourceCPU] = cpuQty
		vm.Spec.Template.Spec.Domain.Resources.Limits[k8sv1.ResourceCPU] = cpuQty
	}
	if spec.MemoryMB > 0 {
		memQty := resource.MustParse(fmt.Sprintf("%dMi", spec.MemoryMB))
		vm.Spec.Template.Spec.Domain.Resources.Requests[k8sv1.ResourceMemory] = memQty
		vm.Spec.Template.Spec.Domain.Resources.Limits[k8sv1.ResourceMemory] = memQty
	}
	if image := strings.TrimSpace(spec.Image); image != "" {
		volumes, disks, err := buildDisksAndVolumes(image, spec.DiskGB)
		if err == nil {
			vm.Spec.Template.Spec.Volumes = mergeManagedVolumes(vm.Spec.Template.Spec.Volumes, volumes)
			vm.Spec.Template.Spec.Domain.Devices.Disks = mergeManagedDisks(vm.Spec.Template.Spec.Domain.Devices.Disks, disks)
		}
	}
	return applySpecOverrides(vm, spec.SpecOverrides)
}

func applySpecOverrides(vm *kubevirtv1.VirtualMachine, overrides map[string]interface{}) error {
	if vm == nil || len(overrides) == 0 {
		return nil
	}

	paths, err := normalizeSpecOverridePaths(overrides)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}

	unstructuredVM, err := runtime.DefaultUnstructuredConverter.ToUnstructured(vm)
	if err != nil {
		return fmt.Errorf("to unstructured vm: %w", err)
	}

	for path, value := range paths {
		if err := setUnstructuredPath(unstructuredVM, path, value); err != nil {
			return err
		}
	}

	var patched kubevirtv1.VirtualMachine
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVM, &patched); err != nil {
		return fmt.Errorf("from unstructured vm: %w", err)
	}

	*vm = patched
	return nil
}

func normalizeSpecOverridePaths(overrides map[string]interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(overrides))
	for rawPath, rawValue := range overrides {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		switch {
		case path == "spec":
			nested, ok := rawValue.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid spec_overrides path %q: value must be object", rawPath)
			}
			flattenSpecMap("spec", nested, out)
		case strings.HasPrefix(path, "spec."):
			out[path] = normalizeSpecOverrideValue(rawValue)
		default:
			return nil, fmt.Errorf("invalid spec_overrides path %q: only spec.* is allowed", rawPath)
		}
	}
	return out, nil
}

func flattenSpecMap(prefix string, values map[string]interface{}, out map[string]interface{}) {
	for rawKey, rawValue := range values {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		path := prefix + "." + key
		if nested, ok := rawValue.(map[string]interface{}); ok {
			flattenSpecMap(path, nested, out)
			continue
		}
		out[path] = normalizeSpecOverrideValue(rawValue)
	}
}

func normalizeSpecOverrideValue(raw interface{}) interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, value := range v {
			out[key] = normalizeSpecOverrideValue(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, item := range v {
			out = append(out, normalizeSpecOverrideValue(item))
		}
		return out
	case float64:
		if math.Trunc(v) == v {
			return int64(v)
		}
		return v
	default:
		return raw
	}
}

func setUnstructuredPath(root map[string]interface{}, path string, value interface{}) error {
	segments := strings.Split(path, ".")
	current := root
	for idx, segment := range segments {
		if segment == "" {
			return fmt.Errorf("invalid path segment in %q", path)
		}
		if idx == len(segments)-1 {
			current[segment] = value
			return nil
		}

		next, ok := current[segment]
		if !ok || next == nil {
			child := map[string]interface{}{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid override path %q: %q is not an object", path, strings.Join(segments[:idx+1], "."))
		}
		current = child
	}
	return nil
}

func buildDisksAndVolumes(image string, diskGB int) ([]kubevirtv1.Volume, []kubevirtv1.Disk, error) {
	const (
		rootVolumeName = "rootdisk"
		dataVolumeName = "datadisk"
	)
	image = strings.TrimSpace(image)
	if image == "" {
		return nil, nil, fmt.Errorf("vm image is required")
	}

	rootDisk := kubevirtv1.Disk{
		Name: rootVolumeName,
		DiskDevice: kubevirtv1.DiskDevice{
			Disk: &kubevirtv1.DiskTarget{Bus: kubevirtv1.DiskBusVirtio},
		},
	}

	rootVolume := kubevirtv1.Volume{
		Name: rootVolumeName,
	}
	switch {
	case strings.HasPrefix(image, "pvc:"):
		claimName := strings.TrimSpace(strings.TrimPrefix(image, "pvc:"))
		if claimName == "" {
			return nil, nil, fmt.Errorf("pvc image reference is empty")
		}
		rootVolume.VolumeSource = kubevirtv1.VolumeSource{
			PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: k8sv1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		}
	default:
		rootVolume.VolumeSource = kubevirtv1.VolumeSource{
			ContainerDisk: &kubevirtv1.ContainerDiskSource{
				Image: image,
			},
		}
	}

	volumes := []kubevirtv1.Volume{rootVolume}
	disks := []kubevirtv1.Disk{rootDisk}

	if diskGB > 0 {
		dataDisk := kubevirtv1.Disk{
			Name: dataVolumeName,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{Bus: kubevirtv1.DiskBusVirtio},
			},
		}
		dataVolume := kubevirtv1.Volume{
			Name: dataVolumeName,
			VolumeSource: kubevirtv1.VolumeSource{
				EmptyDisk: &kubevirtv1.EmptyDiskSource{
					Capacity: resource.MustParse(fmt.Sprintf("%dGi", diskGB)),
				},
			},
		}
		volumes = append(volumes, dataVolume)
		disks = append(disks, dataDisk)
	}

	return volumes, disks, nil
}

func mergeManagedVolumes(existing, desired []kubevirtv1.Volume) []kubevirtv1.Volume {
	merged := make([]kubevirtv1.Volume, 0, len(existing)+len(desired))
	for _, volume := range existing {
		if !isManagedVMStorageName(volume.Name) {
			merged = append(merged, volume)
		}
	}
	return append(merged, desired...)
}

func mergeManagedDisks(existing, desired []kubevirtv1.Disk) []kubevirtv1.Disk {
	merged := make([]kubevirtv1.Disk, 0, len(existing)+len(desired))
	for _, disk := range existing {
		if !isManagedVMStorageName(disk.Name) {
			merged = append(merged, disk)
		}
	}
	return append(merged, desired...)
}

func isManagedVMStorageName(name string) bool {
	switch name {
	case "rootdisk", "datadisk":
		return true
	default:
		return false
	}
}
