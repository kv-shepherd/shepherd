package service

import (
	"fmt"
	"strings"
)

// ValidateSpecOverrides validates that all spec_overrides keys use the required
// "spec.*" path prefix, as mandated by ADR-0018 §4.
//
// This prevents invalid override paths from reaching the KubeVirt provider layer,
// where normalizeSpecOverridePaths() would reject them with a less helpful error.
func ValidateSpecOverrides(overrides map[string]interface{}) error {
	if len(overrides) == 0 {
		return nil
	}
	for key := range overrides {
		path := strings.TrimSpace(key)
		if path == "" {
			continue
		}
		if path != "spec" && !strings.HasPrefix(path, "spec.") {
			return fmt.Errorf(
				"spec_overrides key %q must start with \"spec.\" prefix; "+
					"only KubeVirt VirtualMachine spec paths are allowed (ADR-0018)",
				key,
			)
		}
	}
	return nil
}

// DetectSpecOverridesConflicts checks for logical conflicts between the indexed
// database columns (cpu_cores, memory_mb, etc.) and the spec_overrides JSONB.
//
// Returns a list of warning messages. These are informational — the save proceeds,
// but administrators are warned about potential inconsistencies.
//
// ADR-0018 §4: Hybrid Model — indexed columns take precedence for scheduling queries.
func DetectSpecOverridesConflicts(
	cpuCores int,
	memoryMB int,
	dedicatedCPU bool,
	cpuRequest *int,
	overrides map[string]interface{},
) []string {
	var warnings []string

	// Check overcommit conflict: cpuRequest != cpuCores with dedicated_cpu.
	// This check applies to indexed columns and runs regardless of spec_overrides content.
	if dedicatedCPU && cpuRequest != nil && *cpuRequest != cpuCores {
		warnings = append(warnings,
			"dedicated_cpu is true but cpu_request differs from cpu_cores; "+
				"KubeVirt requires Guaranteed QoS (request == limit) for dedicated CPU placement",
		)
	}

	if len(overrides) == 0 {
		return warnings
	}

	// Check CPU conflict: spec_overrides cpu.cores vs indexed cpu_cores
	for _, cpuPath := range []string{
		"spec.template.spec.domain.cpu.cores",
		"spec.domain.cpu.cores",
	} {
		if raw, ok := overrides[cpuPath]; ok {
			if v, isNum := toIntSafe(raw); isNum && v != cpuCores {
				warnings = append(warnings, fmt.Sprintf(
					"spec_overrides path %q value %d conflicts with indexed cpu_cores=%d; "+
						"the indexed field takes precedence for scheduling queries",
					cpuPath, v, cpuCores,
				))
			}
		}
	}

	// Check Memory conflict
	for _, memPath := range []string{
		"spec.template.spec.domain.resources.requests.memory",
		"spec.domain.resources.requests.memory",
	} {
		if _, ok := overrides[memPath]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"spec_overrides path %q may conflict with indexed memory_mb=%d; "+
					"consider using the indexed field instead",
				memPath, memoryMB,
			))
		}
	}

	// Check dedicated_cpu + overcommit via spec_overrides
	if dedicatedCPU {
		for _, dedicatedPath := range []string{
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement",
			"spec.domain.cpu.dedicatedCpuPlacement",
		} {
			if raw, ok := overrides[dedicatedPath]; ok {
				if boolVal, isBool := raw.(bool); isBool && !boolVal {
					warnings = append(warnings, fmt.Sprintf(
						"spec_overrides path %q is false but dedicated_cpu indexed field is true; "+
							"this inconsistency may cause unexpected scheduling behavior",
						dedicatedPath,
					))
				}
			}
		}
	}

	return warnings
}

// toIntSafe converts interface{} to int without panicking.
// Handles int, float64 (JSON numbers), and int64.
func toIntSafe(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), v == float64(int(v)) // only exact integers
	default:
		return 0, false
	}
}
