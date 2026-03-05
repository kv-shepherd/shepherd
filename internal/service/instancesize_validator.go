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
// database columns (cpu_cores, memory_gi, etc.) and the spec_overrides JSONB.
//
// Returns a list of warning messages. These are informational — the save proceeds,
// but administrators are warned about potential inconsistencies.
//
// ADR-0018 §4: Hybrid Model — indexed columns take precedence for scheduling queries.
func DetectSpecOverridesConflicts(
	cpuCores float64,
	memoryGi float64,
	dedicatedCPU bool,
	cpuRequest *float64,
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
			if v, isNum := toFloat64Safe(raw); isNum && v != cpuCores {
				warnings = append(warnings, fmt.Sprintf(
					"spec_overrides path %q value %.1f conflicts with indexed cpu_cores=%.1f; "+
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
				"spec_overrides path %q may conflict with indexed memory_gi=%.1f; "+
					"consider using the indexed field instead",
				memPath, memoryGi,
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
	} else if HasDedicatedCPUInSpecOverrides(overrides) {
		// Reverse direction: spec_overrides sets dedicatedCpuPlacement=true without
		// the indexed dedicated_cpu flag. This creates a data inconsistency where
		// the scheduler index disagrees with the KubeVirt spec that is actually applied.
		warnings = append(warnings,
			"spec_overrides sets dedicatedCpuPlacement=true but indexed dedicated_cpu field is false; "+
				"set dedicated_cpu=true so the value is indexed for scheduling and approval queries",
		)
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

func toFloat64Safe(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// HasDedicatedCPUInSpecOverrides reports whether spec_overrides explicitly sets
// KubeVirt's dedicatedCpuPlacement to true.
//
// This is the single source of truth for the "spec path bypass" detection,
// shared between the InstanceSize create/update validation (handlers) and the
// approval validator (service). Both use different call sites but the same
// semantic: does the effective dedicated-CPU intent in spec_overrides disagree
// with the indexed dedicated_cpu field?
//
// Supported key formats — both are checked to handle all callers:
//
//  1. Flat dot-notation key (DB-stored / API round-trip):
//     "spec.template.spec.domain.cpu.dedicatedCpuPlacement" = true
//
//  2. Nested map (produced by DynamicSchemaForm via Ant Design Form.getFieldsValue):
//     {"spec": {"template": {"spec": {"domain": {"cpu": {"dedicatedCpuPlacement": true}}}}}}
//
// The DynamicSchemaForm component uses namePath = field.path.split('.') when
// registering field names, so Ant Design stores values as a nested object tree.
// getFieldsValue() / JSON.stringify() therefore produces the nested structure,
// not the flat key. Both formats must be handled here to prevent constraint bypass.
func HasDedicatedCPUInSpecOverrides(overrides map[string]interface{}) bool {
	for _, path := range []string{
		"spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		"spec.domain.cpu.dedicatedCpuPlacement",
	} {
		// Check flat dot-notation key (database-stored format).
		if raw, ok := overrides[path]; ok && raw != nil {
			if boolFromRaw(raw) {
				return true
			}
		}
		// Check nested map format (DynamicSchemaForm / Ant Design output format).
		if v := extractNestedBool(overrides, strings.Split(path, ".")); v != nil && *v {
			return true
		}
	}
	return false
}

// boolFromRaw converts an interface{} to bool, accepting both bool and string "true".
func boolFromRaw(raw interface{}) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}

// extractNestedBool traverses a nested map[string]interface{} along the given
// path segments and returns a pointer to the boolean value at the leaf, or nil
// if the path does not exist or the leaf is not a recognizable boolean.
//
// This handles the DynamicSchemaForm output format where Ant Design Form
// stores field values as a nested object tree keyed by path segments.
func extractNestedBool(m map[string]interface{}, path []string) *bool {
	if len(path) == 0 || m == nil {
		return nil
	}
	val, ok := m[path[0]]
	if !ok || val == nil {
		return nil
	}
	if len(path) == 1 {
		b := boolFromRaw(val)
		return &b
	}
	if nested, ok := val.(map[string]interface{}); ok {
		return extractNestedBool(nested, path[1:])
	}
	return nil
}

// SpecOverrideSetsExplicitFalseForDedicatedCPU checks whether spec_overrides
// **explicitly** sets dedicatedCpuPlacement to false.
//
// This is the counterpart to HasDedicatedCPUInSpecOverrides.  Both must be
// checked in tandem:
//
//   - HasDedicatedCPUInSpecOverrides: spec sets it true  → inconsistent if dedicated_cpu=false
//   - SpecOverrideSetsExplicitFalseForDedicatedCPU: spec sets it false → inconsistent if dedicated_cpu=true
//
// Like the sibling function, both flat dot-notation keys and nested map format
// (produced by DynamicSchemaForm / Ant Design Form.getFieldsValue) are checked.
//
// Returns (conflicting path description, true) when an explicit false is found;
// ("", false) when no explicit false is present (path absent = not a conflict).
func SpecOverrideSetsExplicitFalseForDedicatedCPU(overrides map[string]interface{}) (string, bool) {
	for _, path := range []string{
		"spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		"spec.domain.cpu.dedicatedCpuPlacement",
	} {
		// Check flat dot-notation key (database-stored / API round-trip format).
		if raw, ok := overrides[path]; ok && raw != nil {
			// Use safe type assertion (Go spec: v, ok := x.(T) never panics).
			if b, isBool := raw.(bool); isBool && !b {
				return path, true
			}
			if s, isStr := raw.(string); isStr && strings.EqualFold(strings.TrimSpace(s), "false") {
				return path, true
			}
		}
		// Check nested map format (DynamicSchemaForm / Ant Design format).
		segments := strings.Split(path, ".")
		if v := extractNestedBool(overrides, segments); v != nil && !*v {
			return path + " (nested)", true
		}
	}
	return "", false
}
