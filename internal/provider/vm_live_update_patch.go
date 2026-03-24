package provider

import (
	"fmt"
	"math"

	"sigs.k8s.io/yaml"

	"kv-shepherd.io/shepherd/internal/domain"
)

// VMLiveUpdateTargets carries the requested online resource expansions.
//
// Scope:
//   - CPU: integer total vCPU expansion only, mapped to KubeVirt socket hotplug
//   - Memory: 0.5 Gi steps via memory.guest + requests/limits
//   - Disk: integer Gi expansion of the root DataVolume request
type VMLiveUpdateTargets struct {
	CPUCores *float64
	MemoryGi *float64
	DiskGB   *int
}

// RenderVMLiveUpdatePatch renders a minimal SSA patch for a live VM resource update.
//
// The patch is intentionally partial. It only declares the fields shepherd owns
// for the live-change operation so the apply remains narrow and auditable.
func RenderVMLiveUpdatePatch(namespace string, current *domain.VM, target VMLiveUpdateTargets) (string, error) {
	if current == nil {
		return "", fmt.Errorf("render vm live update patch: current vm is nil")
	}
	if current.Name == "" {
		return "", fmt.Errorf("render vm live update patch: vm name is required")
	}
	if namespace == "" {
		namespace = current.Namespace
	}
	if namespace == "" {
		return "", fmt.Errorf("render vm live update patch: namespace is required")
	}

	if target.CPUCores == nil && target.MemoryGi == nil && target.DiskGB == nil {
		return "", fmt.Errorf("render vm live update patch: at least one target must be provided")
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
			return "", err
		}
		domainPatch["cpu"] = map[string]interface{}{
			"sockets": newSockets,
		}
		requestsPatch["cpu"] = formatCPU(float64(totalCores))
		limitsPatch["cpu"] = formatCPU(float64(totalCores))
	}

	if target.MemoryGi != nil {
		if !isValidHalfStep(*target.MemoryGi) {
			return "", fmt.Errorf("memory live update target %.1fGi must use 0.5-step values", *target.MemoryGi)
		}
		if *target.MemoryGi <= current.Spec.MemoryGi {
			return "", fmt.Errorf(
				"memory live update target %.1fGi must be greater than current %.1fGi",
				*target.MemoryGi,
				current.Spec.MemoryGi,
			)
		}
		domainPatch["memory"] = map[string]interface{}{
			"guest": formatGi(*target.MemoryGi),
		}
		requestsPatch["memory"] = formatGi(*target.MemoryGi)
		limitsPatch["memory"] = formatGi(*target.MemoryGi)
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
		if current.Spec.RootDataVolumeName == "" || !current.Spec.DiskHotplugSupported {
			return "", fmt.Errorf("disk live update is unavailable for VMs without a root DataVolume")
		}
		if *target.DiskGB <= current.Spec.DiskGB {
			return "", fmt.Errorf(
				"disk live update target %dGi must be greater than current %dGi",
				*target.DiskGB,
				current.Spec.DiskGB,
			)
		}

		dvSpec := map[string]interface{}{}
		requestedStorage := map[string]interface{}{
			"storage": fmt.Sprintf("%dGi", *target.DiskGB),
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
	}

	patchObject := map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      current.Name,
			"namespace": namespace,
		},
		"spec": specPatch,
	}

	yamlBytes, err := yaml.Marshal(patchObject)
	if err != nil {
		return "", fmt.Errorf("render vm live update patch: marshal yaml: %w", err)
	}
	return string(yamlBytes), nil
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
