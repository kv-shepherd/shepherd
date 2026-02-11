package provider

import (
	"fmt"
	"time"

	kubevirtv1 "kubevirt.io/api/core/v1"

	"kv-shepherd.io/shepherd/internal/domain"
)

// KubeVirtMapper maps between KubeVirt K8s types and domain types.
// Anti-Corruption Layer: isolates domain logic from K8s API changes.
type KubeVirtMapper struct{}

// NewKubeVirtMapper creates a new KubeVirtMapper.
func NewKubeVirtMapper() *KubeVirtMapper {
	return &KubeVirtMapper{}
}

// MapVM maps a KubeVirt VirtualMachine (and optional VMI) to a domain VM.
// Defensive programming: all pointer fields must check nil.
func (m *KubeVirtMapper) MapVM(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) (*domain.VM, error) {
	if vm == nil {
		return nil, fmt.Errorf("mapper: vm is nil")
	}
	if vm.Name == "" || vm.Namespace == "" {
		return nil, fmt.Errorf("mapper: vm name or namespace is empty")
	}

	status := mapVMStatus(vm, vmi)
	spec := mapVMSpec(vm)

	result := &domain.VM{
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Status:    status,
		Spec:      spec,
	}

	// Extract creation timestamp
	if !vm.CreationTimestamp.IsZero() {
		result.CreatedAt = vm.CreationTimestamp.Time
	}

	// Extract cluster from labels (set by platform)
	if vm.Labels != nil {
		if cluster, ok := vm.Labels["kubevirt-shepherd.io/cluster"]; ok {
			result.Cluster = cluster
		}
	}

	return result, nil
}

// MapVMList maps a slice of KubeVirt VMs to domain VMList.
func (m *KubeVirtMapper) MapVMList(vms []kubevirtv1.VirtualMachine, vmis []kubevirtv1.VirtualMachineInstance) (*domain.VMList, error) {
	// Build VMI lookup map for efficient matching
	vmiMap := make(map[string]*kubevirtv1.VirtualMachineInstance, len(vmis))
	for i := range vmis {
		key := vmis[i].Namespace + "/" + vmis[i].Name
		vmiMap[key] = &vmis[i]
	}

	items := make([]*domain.VM, 0, len(vms))
	for i := range vms {
		key := vms[i].Namespace + "/" + vms[i].Name
		vmi := vmiMap[key] // may be nil
		domainVM, err := m.MapVM(&vms[i], vmi)
		if err != nil {
			continue // Skip unmappable VMs, log in production
		}
		items = append(items, domainVM)
	}

	return &domain.VMList{
		Items:      items,
		TotalCount: len(items),
	}, nil
}

// MapSnapshot maps a VirtualMachineSnapshot to a domain Snapshot.
func (m *KubeVirtMapper) MapSnapshot(name, vmName, namespace string, ready bool, createdAt time.Time) *domain.Snapshot {
	return &domain.Snapshot{
		Name:      name,
		VMName:    vmName,
		Namespace: namespace,
		Ready:     ready,
		CreatedAt: createdAt,
	}
}

// mapVMStatus extracts VM status from K8s objects.
func mapVMStatus(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) domain.VMStatus {
	if vm.Status.PrintableStatus != "" {
		switch vm.Status.PrintableStatus {
		case kubevirtv1.VirtualMachineStatusRunning:
			return domain.VMStatusRunning
		case kubevirtv1.VirtualMachineStatusStopped:
			return domain.VMStatusStopped
		case kubevirtv1.VirtualMachineStatusStopping:
			return domain.VMStatusStopping
		case kubevirtv1.VirtualMachineStatusProvisioning:
			return domain.VMStatusCreating
		case kubevirtv1.VirtualMachineStatusTerminating:
			return domain.VMStatusDeleting
		case kubevirtv1.VirtualMachineStatusMigrating:
			return domain.VMStatusMigrating
		case kubevirtv1.VirtualMachineStatusPaused:
			return domain.VMStatusPaused
		}
	}

	// Fallback: check VMI phase
	if vmi != nil {
		switch vmi.Status.Phase {
		case kubevirtv1.Running:
			return domain.VMStatusRunning
		case kubevirtv1.Scheduling, kubevirtv1.Scheduled, kubevirtv1.Pending:
			return domain.VMStatusCreating
		case kubevirtv1.Failed:
			return domain.VMStatusFailed
		}
	}

	// Check if VM is stopped (running=false)
	if vm.Spec.Running != nil && !*vm.Spec.Running {
		return domain.VMStatusStopped
	}

	return domain.VMStatusUnknown
}

// mapVMSpec extracts resource spec from VM.
func mapVMSpec(vm *kubevirtv1.VirtualMachine) domain.VMSpec {
	spec := domain.VMSpec{}

	if vm.Spec.Template == nil {
		return spec
	}

	domainRes := vm.Spec.Template.Spec.Domain.Resources

	// CPU
	if req, ok := domainRes.Requests["cpu"]; ok {
		spec.CPU = int(req.Value())
	}

	// Memory
	if req, ok := domainRes.Requests["memory"]; ok {
		spec.MemoryMB = int(req.Value() / (1024 * 1024))
	}

	// Labels
	if vm.Spec.Template.ObjectMeta.Labels != nil {
		spec.Labels = vm.Spec.Template.ObjectMeta.Labels
	}

	return spec
}
