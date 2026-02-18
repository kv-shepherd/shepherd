package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

// ApprovalValidator performs pre-approval checks per master-flow.md Stage 5.B.
type ApprovalValidator struct {
	client *ent.Client
}

// NewApprovalValidator creates a new ApprovalValidator.
func NewApprovalValidator(client *ent.Client) *ApprovalValidator {
	return &ApprovalValidator{client: client}
}

// ValidateApproval checks:
// 1. Selected cluster exists and is healthy
// 2. Namespace environment matches cluster environment (ADR-0015 §15)
// 3. Instance size overcommit + dedicatedCpuPlacement constraint
// Returns nil if validation passes.
func (v *ApprovalValidator) ValidateApproval(
	ctx context.Context,
	clusterID string,
	instanceSizeID string,
	namespace string,
) error {
	var (
		cl               *ent.Cluster
		clusterCapSet    map[string]struct{}
		clusterDisplayID string
	)

	// 1. Validate cluster exists and is healthy.
	if clusterID != "" {
		var err error
		cl, err = v.client.Cluster.Get(ctx, clusterID)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster not found")
			}
			return fmt.Errorf("query cluster: %w", err)
		}
		if cl.Status != cluster.StatusHEALTHY {
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("cluster %s is not healthy (status: %s)", cl.Name, cl.Status))
		}
		clusterCapSet = buildClusterCapabilitySet(cl.EnabledFeatures)
		clusterDisplayID = cl.Name
		if clusterDisplayID == "" {
			clusterDisplayID = cl.ID
		}
	}

	// 2. Validate namespace environment isolation.
	if strings.TrimSpace(namespace) != "" {
		ns, err := v.client.NamespaceRegistry.Query().
			Where(namespaceregistry.NameEQ(strings.TrimSpace(namespace))).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "namespace not found in registry")
			}
			return fmt.Errorf("query namespace registry by name: %w", err)
		}
		if !ns.Enabled {
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("namespace %s is disabled", ns.Name))
		}
		if cl == nil {
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				"selected cluster is required for namespace environment matching")
		}
		if err := validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment)); err != nil {
			return err
		}
	}

	// 3. Validate InstanceSize constraints and capability matching.
	if instanceSizeID != "" {
		size, err := v.client.InstanceSize.Get(ctx, instanceSizeID)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "instance size not found")
			}
			return fmt.Errorf("query instance size: %w", err)
		}

		if err := ValidateOvercommit(size.CPUCores, size.CPURequest, size.MemoryMB, size.MemoryRequestMB, size.DedicatedCPU); err != nil {
			return err
		}

		requiredCaps := ExtractRequiredCapabilities(size)
		if len(requiredCaps) > 0 {
			if cl == nil {
				return apperrors.BadRequest(apperrors.CodeValidationFailed,
					"selected cluster is required for instance size capability matching")
			}
			missing := MissingCapabilities(requiredCaps, clusterCapSet)
			if len(missing) > 0 {
				return apperrors.BadRequest(
					apperrors.CodeValidationFailed,
					fmt.Sprintf("cluster %s is missing required capabilities: %s", clusterDisplayID, strings.Join(missing, ", ")),
				)
			}
		}
	}

	return nil
}

func validateNamespaceClusterEnvironment(namespaceEnv, clusterEnv string) error {
	nsEnv := strings.TrimSpace(strings.ToLower(namespaceEnv))
	clEnv := strings.TrimSpace(strings.ToLower(clusterEnv))
	if nsEnv == "" || clEnv == "" {
		return apperrors.BadRequest(
			apperrors.CodeValidationFailed,
			fmt.Sprintf("namespace/cluster environment is incomplete (namespace=%q cluster=%q)", namespaceEnv, clusterEnv),
		)
	}
	if nsEnv != clEnv {
		return apperrors.BadRequest(
			"NAMESPACE_CLUSTER_ENV_MISMATCH",
			fmt.Sprintf("namespace environment %q does not match selected cluster environment %q", nsEnv, clEnv),
		)
	}
	return nil
}

// ValidateOvercommit checks overcommit constraints per master-flow.md Stage 5.B.
// KubeVirt dedicatedCpuPlacement requires Guaranteed QoS: CPU request == limit.
//
// Rules:
//  1. dedicatedCPU + overcommit (cpu_request != cpu_cores) → BLOCKING ERROR
//  2. cpu_request > cpu_cores → BLOCKING ERROR (invalid overcommit ratio)
//  3. memory_request > memory_limit → BLOCKING ERROR
func ValidateOvercommit(cpuCores, cpuRequest, memoryMb, memoryRequestMb int, dedicatedCPU bool) error {
	// cpu_request == 0 means "use cpu_cores" (no overcommit).
	overcommitActive := cpuRequest > 0 && cpuRequest != cpuCores

	// Rule 1: Dedicated CPU + overcommit is mutually exclusive.
	// KubeVirt: dedicatedCpuPlacement requires Guaranteed QoS (request == limit).
	if dedicatedCPU && overcommitActive {
		return apperrors.BadRequest("DEDICATED_CPU_OVERCOMMIT_CONFLICT",
			fmt.Sprintf("dedicated CPU requires Guaranteed QoS: CPU request (%d) must equal CPU limit (%d); overcommit is not allowed with dedicatedCpuPlacement",
				cpuRequest, cpuCores))
	}

	// Rule 2: CPU request cannot exceed limit (invalid overcommit direction).
	if overcommitActive && cpuRequest > cpuCores {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("CPU request (%d) cannot exceed CPU limit (%d)", cpuRequest, cpuCores))
	}

	// Rule 3: Memory request cannot exceed limit.
	if memoryRequestMb > 0 && memoryRequestMb > memoryMb {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("memory request (%dMB) cannot exceed memory limit (%dMB)", memoryRequestMb, memoryMb))
	}
	return nil
}

var hugepagesPattern = regexp.MustCompile(`hugepages[-_:]?([0-9]+(?:mi|gi))`)

// ExtractRequiredCapabilities derives scheduling requirements from InstanceSize flags/spec_overrides.
// Returned capability keys are normalized (lowercase) values used by cluster matching:
// - gpu
// - sriov
// - hugepages
// - hugepages:<size> (e.g. hugepages:2mi)
func ExtractRequiredCapabilities(size *ent.InstanceSize) []string {
	if size == nil {
		return nil
	}

	req := make(map[string]struct{}, 4)

	if size.RequiresGpu || hasGPURequirement(size.SpecOverrides) {
		req["gpu"] = struct{}{}
	}
	if size.RequiresSriov || hasSRIOVRequirement(size.SpecOverrides) {
		req["sriov"] = struct{}{}
	}

	hugepagesSize := normalizeHugepagesSize(size.HugepagesSize)
	if hugepagesSize == "" {
		hugepagesSize = normalizeHugepagesSize(extractHugepagesSize(size.SpecOverrides))
	}
	if size.RequiresHugepages || hugepagesSize != "" {
		req["hugepages"] = struct{}{}
		if hugepagesSize != "" {
			req["hugepages:"+hugepagesSize] = struct{}{}
		}
	}

	out := make([]string, 0, len(req))
	for capKey := range req {
		out = append(out, capKey)
	}
	sort.Strings(out)
	return out
}

// MissingCapabilities returns capabilities required by InstanceSize but unavailable on cluster.
func MissingCapabilities(required []string, clusterCaps map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, req := range required {
		key := normalizeCapability(req)
		if key == "" {
			continue
		}
		if _, ok := clusterCaps[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func buildClusterCapabilitySet(enabledFeatures []string) map[string]struct{} {
	set := make(map[string]struct{}, len(enabledFeatures)*2)
	for _, raw := range enabledFeatures {
		capKey := normalizeCapability(raw)
		if capKey == "" {
			continue
		}
		set[capKey] = struct{}{}

		if strings.Contains(capKey, "gpu") || strings.HasPrefix(capKey, "nvidia.com/") {
			set["gpu"] = struct{}{}
		}
		if strings.Contains(capKey, "sriov") {
			set["sriov"] = struct{}{}
		}
		if strings.Contains(capKey, "hugepages") {
			set["hugepages"] = struct{}{}
			if hp := extractHugepagesFromToken(capKey); hp != "" {
				set["hugepages:"+hp] = struct{}{}
			}
		}
	}
	return set
}

func normalizeCapability(in string) string {
	s := strings.TrimSpace(strings.ToLower(in))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func normalizeHugepagesSize(in string) string {
	s := strings.TrimSpace(strings.ToLower(in))
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func hasGPURequirement(spec map[string]interface{}) bool {
	val, ok := getSpecOverrideValue(spec, "spec.template.spec.domain.devices.gpus")
	if !ok || val == nil {
		return false
	}
	switch typed := val.(type) {
	case []interface{}:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		// Any non-nil, non-empty value means a GPU field is present.
		return true
	}
}

func hasSRIOVRequirement(spec map[string]interface{}) bool {
	val, ok := getSpecOverrideValue(spec, "spec.template.spec.domain.devices.interfaces")
	if ok && val != nil {
		switch typed := val.(type) {
		case []interface{}:
			for _, item := range typed {
				if strings.Contains(strings.ToLower(fmt.Sprint(item)), "sriov") {
					return true
				}
			}
		default:
			if strings.Contains(strings.ToLower(fmt.Sprint(typed)), "sriov") {
				return true
			}
		}
	}

	// Alternate location in some specs.
	networks, ok := getSpecOverrideValue(spec, "spec.template.spec.networks")
	if !ok || networks == nil {
		return false
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(networks)), "sriov")
}

func extractHugepagesSize(spec map[string]interface{}) string {
	val, ok := getSpecOverrideValue(spec, "spec.template.spec.domain.memory.hugepages.pageSize")
	if !ok || val == nil {
		return ""
	}
	return fmt.Sprint(val)
}

func getSpecOverrideValue(spec map[string]interface{}, path string) (interface{}, bool) {
	if len(spec) == 0 || path == "" {
		return nil, false
	}
	// Path-flattened mode: "a.b.c": value.
	if val, ok := spec[path]; ok {
		return val, true
	}
	// Nested mode: {"a":{"b":{"c":value}}}
	parts := strings.Split(path, ".")
	var current interface{} = spec
	for _, p := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func extractHugepagesFromToken(token string) string {
	match := hugepagesPattern.FindStringSubmatch(strings.ToLower(token))
	if len(match) < 2 {
		return ""
	}
	return normalizeHugepagesSize(match[1])
}
