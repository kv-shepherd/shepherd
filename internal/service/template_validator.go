package service

import (
	"fmt"
	"strings"
)

// ValidateTemplateSpec enforces ADR-0018 boundary: Template.spec may only contain
// software-baseline fields (OS image source, cloud-init). Any hardware configuration
// (CPU, memory, GPU, network, etc.) MUST reside in InstanceSize.spec_overrides.
//
// This prevents the "Template explosion" anti-pattern where administrators embed
// hardware settings inside templates, creating tight coupling and maintenance debt.
//
// ADR-0036: Template / InstanceSize Boundary Enforcement
func ValidateTemplateSpec(spec map[string]interface{}) error {
	if len(spec) == 0 {
		return nil
	}
	return validateSpecMap("", spec)
}

// validateSpecMap recursively checks all keys in a nested map against the prohibited
// paths list. The prefix tracks the dot-separated path from the root.
func validateSpecMap(prefix string, m map[string]interface{}) error {
	for key, value := range m {
		fullPath := key
		if prefix != "" {
			fullPath = prefix + "." + key
		}

		if isProhibitedTemplatePath(fullPath) {
			return fmt.Errorf(
				"template spec contains prohibited hardware path %q; "+
					"hardware configuration must be defined in InstanceSize, not Template (ADR-0018)",
				fullPath,
			)
		}

		// Recurse into nested maps
		if nested, ok := value.(map[string]interface{}); ok {
			if err := validateSpecMap(fullPath, nested); err != nil {
				return err
			}
		}
	}
	return nil
}

// prohibitedTemplatePrefixes defines paths that belong exclusively to InstanceSize.
// These cover the KubeVirt VirtualMachineSpec hardware-related sections.
//
// Format: all lowercase, dot-separated path prefixes.
// A key matches if it equals or starts with any prefix followed by a dot.
var prohibitedTemplatePrefixes = []string{
	// Direct hardware resource fields
	"cpu",
	"cpu_cores",
	"cpu_request",
	"memory",
	"memory_gi",
	"memory_request_gi",
	"disk_gb",
	"dedicated_cpu",
	// KubeVirt domain spec hardware paths
	"domain.cpu",
	"domain.resources",
	"domain.memory",
	"domain.devices.gpus",
	"domain.devices.hostdevices",
	"domain.devices.interfaces",
	"domain.features",
	// Full spec.template.spec.domain nesting
	"spec.template.spec.domain.cpu",
	"spec.template.spec.domain.resources",
	"spec.template.spec.domain.memory",
	"spec.template.spec.domain.devices.gpus",
	"spec.template.spec.domain.devices.hostdevices",
	"spec.template.spec.domain.devices.interfaces",
	// Resources shorthand
	"resources",
	"resources.requests",
	"resources.limits",
	// Capability flags
	"requires_gpu",
	"requires_sriov",
	"requires_hugepages",
	"hugepages_size",
	// Overcommit
	"overcommit",
	"overcommit_ratio",
}

// isProhibitedTemplatePath checks if the given dot-separated path matches
// any prohibited hardware configuration prefix.
func isProhibitedTemplatePath(path string) bool {
	normalized := strings.ToLower(strings.TrimSpace(path))
	if normalized == "" {
		return false
	}
	for _, prefix := range prohibitedTemplatePrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+".") {
			return true
		}
	}
	return false
}
