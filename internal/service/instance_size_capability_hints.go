package service

// InstanceSizeCapabilityHints captures the legacy indexed requirement fields that
// are still exposed in API responses and reused by request/approval UX.
type InstanceSizeCapabilityHints struct {
	RequiresGPU       bool
	RequiresHugepages bool
	HugepagesSize     string
}

// DeriveInstanceSizeCapabilityHints derives indexed requirement hints from the
// effective spec_overrides payload. This keeps API responses and persisted
// helper columns aligned with schema-driven spec editing.
func DeriveInstanceSizeCapabilityHints(spec map[string]interface{}) InstanceSizeCapabilityHints {
	hints := InstanceSizeCapabilityHints{
		RequiresGPU:       hasGPURequirement(spec),
		HugepagesSize:     extractHugepagesSize(spec),
		RequiresHugepages: false,
	}
	if normalizeHugepagesSize(hints.HugepagesSize) != "" {
		hints.RequiresHugepages = true
	}
	return hints
}
