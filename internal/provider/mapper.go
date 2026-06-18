package provider

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
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
		Name:            vm.Name,
		Namespace:       vm.Namespace,
		Status:          status,
		OSName:          extractGuestOSName(vmi),
		OSVersion:       extractGuestOSVersion(vmi),
		OSFamily:        extractGuestOSFamily(vmi),
		NodeName:        extractVMINodeName(vmi),
		IPAddress:       extractPrimaryVMIPAddress(vmi),
		Spec:            spec,
		ResourceVersion: vm.ResourceVersion, // ADR-0038: capture for watch-cache routing
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

func extractVMINodeName(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi == nil {
		return ""
	}
	return vmi.Status.NodeName
}

func extractPrimaryVMIPAddress(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi == nil {
		return ""
	}
	for i := range vmi.Status.Interfaces {
		if ip := vmi.Status.Interfaces[i].IP; ip != "" {
			return ip
		}
		if len(vmi.Status.Interfaces[i].IPs) > 0 && vmi.Status.Interfaces[i].IPs[0] != "" {
			return vmi.Status.Interfaces[i].IPs[0]
		}
	}
	return ""
}

func extractGuestOSName(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi == nil {
		return ""
	}
	info := vmi.Status.GuestOSInfo
	return firstNonEmptyTrimmed(info.PrettyName, info.Name, info.ID)
}

func extractGuestOSVersion(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi == nil {
		return ""
	}
	info := vmi.Status.GuestOSInfo
	return firstNonEmptyTrimmed(info.VersionID, info.Version)
}

func extractGuestOSFamily(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi == nil {
		return ""
	}
	info := vmi.Status.GuestOSInfo
	source := strings.ToLower(firstNonEmptyTrimmed(info.ID, info.Name, info.PrettyName))
	switch {
	case source == "":
		return ""
	case strings.Contains(source, "windows"),
		strings.Contains(source, "win32"),
		strings.Contains(source, "msdos"):
		return "windows"
	default:
		return "linux"
	}
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
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
		case kubevirtv1.VirtualMachineStatusStarting:
			return domain.VMStatusStarting
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
		case kubevirtv1.VirtualMachineStatusWaitingForVolumeBinding,
			kubevirtv1.VirtualMachineStatusWaitingForReceiver:
			return domain.VMStatusPending
		case kubevirtv1.VirtualMachineStatusCrashLoopBackOff,
			kubevirtv1.VirtualMachineStatusUnschedulable,
			kubevirtv1.VirtualMachineStatusErrImagePull,
			kubevirtv1.VirtualMachineStatusImagePullBackOff,
			kubevirtv1.VirtualMachineStatusDataVolumeError:
			return domain.VMStatusFailed
		case kubevirtv1.VirtualMachineStatusUnknown:
			return domain.VMStatusUnknown
		default:
			// Fall back to VMI phase and RunStrategy mapping below.
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
		default:
			// Fall back to VM-level strategy/status below.
		}
	}

	// ADR-0011 / KV-005: Prefer RunStrategy over deprecated spec.Running.
	// RunStrategy is the recommended field since KubeVirt v1.x.
	if vm.Spec.RunStrategy != nil {
		switch *vm.Spec.RunStrategy {
		case kubevirtv1.RunStrategyHalted:
			return domain.VMStatusStopped
		case kubevirtv1.RunStrategyAlways,
			kubevirtv1.RunStrategyRerunOnFailure,
			kubevirtv1.RunStrategyManual,
			kubevirtv1.RunStrategyOnce:
			// These strategies don't definitively indicate stopped status.
			// Fall through to unknown — actual running state comes from VMI/PrintableStatus above.
		default:
			// Unknown/new strategy values fall back to unknown.
		}
	}

	// Legacy fallback: spec.Running (deprecated, kept for backward compatibility
	// with older KubeVirt clusters that may not set RunStrategy).
	if vm.Spec.Running != nil && !*vm.Spec.Running {
		return domain.VMStatusStopped
	}

	return domain.VMStatusUnknown
}

// mapVMSpec extracts resource spec from VM.
func mapVMSpec(vm *kubevirtv1.VirtualMachine) domain.VMSpec {
	spec := domain.VMSpec{
		AutoattachGraphicsDevice: true,
		AutoattachSerialConsole:  true,
	}

	if vm.Spec.Template == nil {
		return spec
	}

	templateSpec := vm.Spec.Template.Spec
	domainSpec := templateSpec.Domain
	domainRes := domainSpec.Resources
	if domainSpec.Devices.AutoattachGraphicsDevice != nil {
		spec.AutoattachGraphicsDevice = *domainSpec.Devices.AutoattachGraphicsDevice
	}
	if domainSpec.Devices.AutoattachSerialConsole != nil {
		spec.AutoattachSerialConsole = *domainSpec.Devices.AutoattachSerialConsole
	}

	// CPU topology.
	if domainSpec.CPU != nil {
		sockets := int(domainSpec.CPU.Sockets)
		if sockets <= 0 {
			sockets = 1
		}
		coresPerSocket := int(domainSpec.CPU.Cores)
		if coresPerSocket <= 0 {
			coresPerSocket = 1
		}
		threads := int(domainSpec.CPU.Threads)
		if threads <= 0 {
			threads = 1
		}
		spec.CurrentCPUSockets = sockets
		spec.CurrentCPUCoresPerSocket = coresPerSocket
		spec.CurrentCPUThreads = threads
		spec.CPU = float64(sockets * coresPerSocket * threads)
	}

	if spec.CPU == 0 {
		// Fall back to requests/limits when CPU topology is absent.
		if req, ok := domainRes.Requests[corev1.ResourceCPU]; ok {
			spec.CPU = float64(req.MilliValue()) / 1000.0
		} else if limit, ok := domainRes.Limits[corev1.ResourceCPU]; ok {
			spec.CPU = float64(limit.MilliValue()) / 1000.0
		}
	}

	// Memory (bytes → Gi). Prefer guest memory when explicitly configured.
	if domainSpec.Memory != nil && domainSpec.Memory.Guest != nil {
		spec.MemoryGi = float64(domainSpec.Memory.Guest.Value()) / (1024 * 1024 * 1024)
	} else if req, ok := domainRes.Requests[corev1.ResourceMemory]; ok {
		spec.MemoryGi = float64(req.Value()) / (1024 * 1024 * 1024)
	} else if limit, ok := domainRes.Limits[corev1.ResourceMemory]; ok {
		spec.MemoryGi = float64(limit.Value()) / (1024 * 1024 * 1024)
	}
	if domainSpec.Memory != nil && domainSpec.Memory.Hugepages != nil {
		spec.HugepagesPageSize = strings.TrimSpace(domainSpec.Memory.Hugepages.PageSize)
	}

	mapVMLiveDiskSpec(vm, &spec)

	// The VM-level live-update support check is completed by the handler/worker
	// after cluster capability evaluation. Here we only capture whether the VM
	// topology uses a root DataVolume that can be targeted by a patch.
	if spec.RootDataVolumeName != "" {
		spec.DiskHotplugSupported = true
	}

	// Labels
	if vm.Spec.Template.ObjectMeta.Labels != nil {
		spec.Labels = vm.Spec.Template.ObjectMeta.Labels
	}

	return spec
}

func mapVMLiveDiskSpec(vm *kubevirtv1.VirtualMachine, spec *domain.VMSpec) {
	if vm == nil || spec == nil || vm.Spec.Template == nil {
		return
	}

	rootVolumeName := ""
	if len(vm.Spec.Template.Spec.Domain.Devices.Disks) > 0 {
		rootVolumeName = vm.Spec.Template.Spec.Domain.Devices.Disks[0].Name
	}
	if rootVolumeName == "" {
		return
	}

	var rootDataVolumeName string
	for i := range vm.Spec.Template.Spec.Volumes {
		volume := &vm.Spec.Template.Spec.Volumes[i]
		if volume.Name != rootVolumeName {
			continue
		}
		if volume.DataVolume != nil {
			rootDataVolumeName = volume.DataVolume.Name
		}
		break
	}
	if rootDataVolumeName == "" {
		// Fallback for containerDisk-based templates with a data emptyDisk.
		for i := range vm.Spec.Template.Spec.Volumes {
			volume := &vm.Spec.Template.Spec.Volumes[i]
			if volume.EmptyDisk == nil || volume.EmptyDisk.Capacity.IsZero() {
				continue
			}
			spec.DiskGB = quantityBytesToGi(volume.EmptyDisk.Capacity.Value())
			return
		}
		return
	}

	spec.RootDataVolumeName = rootDataVolumeName

	for i := range vm.Spec.DataVolumeTemplates {
		template := &vm.Spec.DataVolumeTemplates[i]
		if template.Name != rootDataVolumeName {
			continue
		}
		if template.Spec.PVC != nil {
			spec.RootVolumeUsesPVCSpec = true
			if qty, ok := template.Spec.PVC.Resources.Requests[corev1.ResourceStorage]; ok {
				spec.DiskGB = quantityBytesToGi(qty.Value())
			}
			return
		}
		if template.Spec.Storage != nil {
			if qty, ok := template.Spec.Storage.Resources.Requests[corev1.ResourceStorage]; ok {
				spec.DiskGB = quantityBytesToGi(qty.Value())
			}
			return
		}
	}
}

func quantityBytesToGi(bytes int64) int {
	const gib = int64(1024 * 1024 * 1024)
	if bytes <= 0 {
		return 0
	}
	return int((bytes + gib - 1) / gib)
}
