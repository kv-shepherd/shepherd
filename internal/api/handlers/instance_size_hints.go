package handlers

import (
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/service"
)

func effectiveInstanceSizeCapabilityHints(
	sz *ent.InstanceSize,
) service.InstanceSizeCapabilityHints {
	if sz == nil {
		return service.InstanceSizeCapabilityHints{}
	}

	hints := service.DeriveInstanceSizeCapabilityHints(sz.SpecOverrides)
	if sz.RequiresGpu {
		hints.RequiresGPU = true
	}
	if sz.RequiresHugepages {
		hints.RequiresHugepages = true
	}
	if v := strings.TrimSpace(sz.HugepagesSize); v != "" {
		hints.HugepagesSize = v
		hints.RequiresHugepages = true
	}
	return hints
}

func effectiveInstanceSizeCapabilityHintsFromSpec(
	spec map[string]interface{},
	explicitRequiresGPU *bool,
	explicitRequiresHugepages *bool,
	explicitHugepagesSize *string,
) service.InstanceSizeCapabilityHints {
	hints := service.DeriveInstanceSizeCapabilityHints(spec)
	if explicitRequiresGPU != nil {
		hints.RequiresGPU = hints.RequiresGPU || *explicitRequiresGPU
	}
	if explicitRequiresHugepages != nil {
		hints.RequiresHugepages = hints.RequiresHugepages || *explicitRequiresHugepages
	}
	if explicitHugepagesSize != nil {
		if v := strings.TrimSpace(*explicitHugepagesSize); v != "" {
			hints.HugepagesSize = v
		}
	}
	if strings.TrimSpace(hints.HugepagesSize) != "" {
		hints.RequiresHugepages = true
	}
	return hints
}
