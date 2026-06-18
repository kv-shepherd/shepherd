package provider

import (
	"encoding/json"
	"fmt"
	"math"

	"kv-shepherd.io/shepherd/internal/domain"
)

// VMLiveUpdateTargets carries requested VM resource changes.
//
// Scope:
//   - CPU: integer total vCPU values; running VM plans stage topology changes
//     for restart instead of applying socket hotplug.
//   - Memory: 0.5 Gi steps via memory.guest + requests/limits.
//   - Disk: integer Gi expansion of the root DataVolume request
type VMLiveUpdateTargets struct {
	CPUCores        *float64
	MemoryGi        *float64
	DiskGB          *int
	CPURequest      *float64
	MemoryRequestGi *float64
}

type VMResourceUpdatePlan struct {
	Mutation        *domain.VMMutation
	RequiresRestart bool
	ApplyMode       string
}

// RenderVMResourceUpdatePatch renders a VM resource patch using the safest
// supported path for the current VM state.
//
// Running VMs use the strict live-update path for memory/disk changes. CPU
// topology changes are staged through PlanVMResourceUpdatePatch when restart is
// required. Stopped VMs can accept broader CPU/memory reconfiguration while disk
// remains expansion-only.
func RenderVMResourceUpdatePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (*domain.VMMutation, error) {
	if current == nil {
		return nil, fmt.Errorf("render vm resource update patch: current vm is nil")
	}
	if current.Status == domain.VMStatusStopped {
		return renderVMOfflineResourcePatch(namespace, current, target)
	}
	return RenderVMLiveUpdatePatch(namespace, current, target)
}

func PlanVMResourceUpdatePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (*VMResourceUpdatePlan, error) {
	if current == nil {
		return nil, fmt.Errorf("plan vm resource update patch: current vm is nil")
	}
	if current.Status == domain.VMStatusStopped {
		rendered, err := renderVMOfflineResourcePatch(namespace, current, target)
		if err != nil {
			return nil, err
		}
		return &VMResourceUpdatePlan{
			Mutation:        rendered,
			RequiresRestart: false,
			ApplyMode:       "offline",
		}, nil
	}

	if target.CPUCores != nil {
		rendered, err := renderVMOfflineResourcePatch(namespace, current, target)
		if err != nil {
			return nil, err
		}
		return &VMResourceUpdatePlan{
			Mutation:        rendered,
			RequiresRestart: true,
			ApplyMode:       "restart_required",
		}, nil
	}

	rendered, err := RenderVMLiveUpdatePatch(namespace, current, target)
	if err == nil {
		return &VMResourceUpdatePlan{
			Mutation:        rendered,
			RequiresRestart: false,
			ApplyMode:       "live",
		}, nil
	}

	if target.CPUCores == nil && target.MemoryGi == nil {
		return nil, err
	}

	offlineRendered, offlineErr := renderVMOfflineResourcePatch(namespace, current, target)
	if offlineErr != nil {
		return nil, err
	}
	return &VMResourceUpdatePlan{
		Mutation:        offlineRendered,
		RequiresRestart: true,
		ApplyMode:       "restart_required",
	}, nil
}

// RenderVMLiveUpdatePatch builds an exact KubeVirt VM patch for a live VM
// resource update.
func RenderVMLiveUpdatePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (*domain.VMMutation, error) {
	if current == nil {
		return nil, fmt.Errorf("render vm live update patch: current vm is nil")
	}
	if current.Name == "" {
		return nil, fmt.Errorf("render vm live update patch: vm name is required")
	}
	if namespace == "" {
		namespace = current.Namespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("render vm live update patch: namespace is required")
	}

	if target.CPUCores == nil && target.MemoryGi == nil && target.DiskGB == nil {
		return nil, fmt.Errorf("render vm live update patch: at least one target must be provided")
	}

	specPatch := map[string]interface{}{}
	templatePatch := map[string]interface{}{}
	domainPatch := map[string]interface{}{}
	resourcesPatch := map[string]interface{}{}
	requestsPatch := map[string]interface{}{}
	limitsPatch := map[string]interface{}{}

	if target.CPUCores != nil {
		totalCores, newSockets, err := resolveLiveCPUHotplugSockets(current, *target.CPUCores)
		if err != nil {
			return nil, err
		}
		domainPatch["cpu"] = map[string]interface{}{
			"sockets": newSockets,
		}
		limitsPatch["cpu"] = formatCPU(float64(totalCores))
	}

	if target.MemoryGi != nil {
		if !isValidHalfStep(*target.MemoryGi) {
			return nil, fmt.Errorf("memory live update target %.1fGi must use 0.5-step values", *target.MemoryGi)
		}
		if *target.MemoryGi <= current.Spec.MemoryGi {
			return nil, fmt.Errorf(
				"memory live update target %.1fGi must be greater than current %.1fGi",
				*target.MemoryGi,
				current.Spec.MemoryGi,
			)
		}
		domainPatch["memory"] = map[string]interface{}{
			"guest": formatGi(*target.MemoryGi),
		}
		limitsPatch["memory"] = formatGi(*target.MemoryGi)
	}

	if target.CPURequest != nil {
		requestsPatch["cpu"] = formatCPU(*target.CPURequest)
	}
	if target.MemoryRequestGi != nil {
		requestsPatch["memory"] = formatGi(*target.MemoryRequestGi)
	}

	if len(requestsPatch) > 0 {
		resourcesPatch["requests"] = requestsPatch
	}
	if len(limitsPatch) > 0 {
		resourcesPatch["limits"] = limitsPatch
	}
	if len(resourcesPatch) > 0 {
		domainPatch["resources"] = resourcesPatch
	}
	if len(domainPatch) > 0 {
		templatePatch["spec"] = map[string]interface{}{
			"domain": domainPatch,
		}
		specPatch["template"] = templatePatch
	}

	if target.DiskGB != nil {
		if err := applyDiskExpansionPatch(specPatch, current, *target.DiskGB, "disk live update", "disk live update target"); err != nil {
			return nil, err
		}
	}

	return newMergePatchMutation(specPatch)
}

func renderVMOfflineResourcePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (*domain.VMMutation, error) {
	if current == nil {
		return nil, fmt.Errorf("render vm offline resource patch: current vm is nil")
	}
	if current.Name == "" {
		return nil, fmt.Errorf("render vm offline resource patch: vm name is required")
	}
	if namespace == "" {
		namespace = current.Namespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("render vm offline resource patch: namespace is required")
	}
	if target.CPUCores == nil && target.MemoryGi == nil && target.DiskGB == nil {
		return nil, fmt.Errorf("render vm offline resource patch: at least one target must be provided")
	}

	specPatch := map[string]interface{}{}
	templatePatch := map[string]interface{}{}
	domainPatch := map[string]interface{}{}
	resourcesPatch := map[string]interface{}{}
	requestsPatch := map[string]interface{}{}
	limitsPatch := map[string]interface{}{}

	if target.CPUCores != nil {
		sockets, cores, threads, totalCores, err := resolveOfflineCPUAllocation(current, *target.CPUCores)
		if err != nil {
			return nil, err
		}
		domainPatch["cpu"] = map[string]interface{}{
			"sockets": sockets,
			"cores":   cores,
			"threads": threads,
		}
		limitsPatch["cpu"] = formatCPU(float64(totalCores))
	}

	if target.MemoryGi != nil {
		if !isValidHalfStep(*target.MemoryGi) {
			return nil, fmt.Errorf("memory target %.1fGi must use 0.5-step values", *target.MemoryGi)
		}
		if *target.MemoryGi <= 0 {
			return nil, fmt.Errorf("memory target %.1fGi must be positive", *target.MemoryGi)
		}
		domainPatch["memory"] = map[string]interface{}{
			"guest": formatGi(*target.MemoryGi),
		}
		limitsPatch["memory"] = formatGi(*target.MemoryGi)
	}

	if target.CPURequest != nil {
		requestsPatch["cpu"] = formatCPU(*target.CPURequest)
	}
	if target.MemoryRequestGi != nil {
		requestsPatch["memory"] = formatGi(*target.MemoryRequestGi)
	}

	if len(requestsPatch) > 0 {
		resourcesPatch["requests"] = requestsPatch
	}
	if len(limitsPatch) > 0 {
		resourcesPatch["limits"] = limitsPatch
	}
	if len(resourcesPatch) > 0 {
		domainPatch["resources"] = resourcesPatch
	}
	if len(domainPatch) > 0 {
		templatePatch["spec"] = map[string]interface{}{
			"domain": domainPatch,
		}
		specPatch["template"] = templatePatch
	}

	if target.DiskGB != nil {
		if err := applyDiskExpansionPatch(specPatch, current, *target.DiskGB, "disk update", "disk target"); err != nil {
			return nil, err
		}
	}

	return newMergePatchMutation(specPatch)
}

// ResolveVMLiveCPUHotplugSupport validates that the current VM topology can be
// expanded via socket hotplug and returns the current total vCPU count together
// with the per-socket increment size.
func ResolveVMLiveCPUHotplugSupport(current *domain.VM) (currentTotalCores, perSocketIncrement int, err error) {
	if current == nil {
		return 0, 0, fmt.Errorf("cpu live update requires current vm")
	}
	coresPerSocket := current.Spec.CurrentCPUCoresPerSocket
	if coresPerSocket <= 0 {
		coresPerSocket = 1
	}
	threads := current.Spec.CurrentCPUThreads
	if threads <= 0 {
		threads = 1
	}
	currentSockets := current.Spec.CurrentCPUSockets
	if currentSockets <= 0 {
		currentSockets = 1
	}
	perSocketIncrement = coresPerSocket * threads
	if perSocketIncrement <= 0 {
		return 0, 0, fmt.Errorf("cpu live update topology is invalid")
	}
	return currentSockets * perSocketIncrement, perSocketIncrement, nil
}

func resolveLiveCPUHotplugSockets(current *domain.VM, targetTotal float64) (totalCores, newSockets int, err error) {
	currentTotal, perSocketCapacity, err := ResolveVMLiveCPUHotplugSupport(current)
	if err != nil {
		return 0, 0, err
	}
	if targetTotal <= 0 || math.Abs(targetTotal-math.Round(targetTotal)) > 1e-9 {
		return 0, 0, fmt.Errorf("cpu live update target %.1f must be a positive integer", targetTotal)
	}
	target := int(math.Round(targetTotal))
	if target <= currentTotal {
		return 0, 0, fmt.Errorf(
			"cpu live update target %d must be greater than current %d",
			target,
			currentTotal,
		)
	}
	if target%perSocketCapacity != 0 {
		return 0, 0, fmt.Errorf(
			"cpu live update target %d is incompatible with current topology: capacity increases in %d-vCPU socket increments",
			target,
			perSocketCapacity,
		)
	}

	return target, target / perSocketCapacity, nil
}

func applyDiskExpansionPatch(
	specPatch map[string]interface{},
	current *domain.VM,
	targetDiskGB int,
	unavailableMessage string,
	targetMessage string,
) error {
	if current.Spec.RootDataVolumeName == "" || !current.Spec.DiskHotplugSupported {
		return fmt.Errorf("%s is unavailable for VMs without a root DataVolume", unavailableMessage)
	}
	if targetDiskGB <= current.Spec.DiskGB {
		return fmt.Errorf(
			"%s %dGi must be greater than current %dGi",
			targetMessage,
			targetDiskGB,
			current.Spec.DiskGB,
		)
	}

	dvSpec := map[string]interface{}{}
	requestedStorage := map[string]interface{}{
		"storage": fmt.Sprintf("%dGi", targetDiskGB),
	}
	if current.Spec.RootVolumeUsesPVCSpec {
		dvSpec["pvc"] = map[string]interface{}{
			"resources": map[string]interface{}{
				"requests": requestedStorage,
			},
		}
	} else {
		dvSpec["storage"] = map[string]interface{}{
			"resources": map[string]interface{}{
				"requests": requestedStorage,
			},
		}
	}
	specPatch["dataVolumeTemplates"] = []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"name": current.Spec.RootDataVolumeName,
			},
			"spec": dvSpec,
		},
	}
	return nil
}

func resolveOfflineCPUAllocation(current *domain.VM, targetTotal float64) (sockets, cores, threads, totalCores int, err error) {
	if current == nil {
		return 0, 0, 0, 0, fmt.Errorf("cpu target requires current vm")
	}
	if targetTotal <= 0 || math.Abs(targetTotal-math.Round(targetTotal)) > 1e-9 {
		return 0, 0, 0, 0, fmt.Errorf("cpu target %.1f must be a positive integer", targetTotal)
	}

	target := int(math.Round(targetTotal))
	currentThreads := current.Spec.CurrentCPUThreads
	if currentThreads <= 0 {
		currentThreads = 1
	}

	if target%currentThreads == 0 {
		return 1, target / currentThreads, currentThreads, target, nil
	}
	return target, 1, 1, target, nil
}

func newMergePatchMutation(specPatch map[string]interface{}) (*domain.VMMutation, error) {
	if len(specPatch) == 0 {
		return nil, fmt.Errorf("render vm resource update patch: empty spec patch")
	}
	payload, err := json.Marshal(map[string]interface{}{
		"spec": specPatch,
	})
	if err != nil {
		return nil, fmt.Errorf("render vm resource update patch: marshal json: %w", err)
	}
	return &domain.VMMutation{
		Mode:      domain.VMMutationModePatch,
		PatchType: domain.VMMutationPatchTypeMerge,
		Payload:   payload,
	}, nil
}
