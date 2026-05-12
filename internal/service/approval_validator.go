package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/clusterpolicy"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

// ApprovalValidator performs pre-approval checks per master-flow.md Stage 5.B.
type ApprovalValidator struct {
	client    *ent.Client
	policySvc *ClusterPolicyService
	vmService *VMService
}

// NewApprovalValidator creates a new ApprovalValidator.
func NewApprovalValidator(client *ent.Client) *ApprovalValidator {
	return &ApprovalValidator{
		client:    client,
		policySvc: NewClusterPolicyService(client),
	}
}

// SetVMService injects VMService-backed infrastructure reads used for
// non-blocking storage advisories during placement evaluation.
func (v *ApprovalValidator) SetVMService(svc *VMService) *ApprovalValidator {
	v.vmService = svc
	return v
}

// ApprovalValidationInput carries all create-approval context needed for
// capability and policy checks.
type ApprovalValidationInput struct {
	ClusterID      string
	TemplateID     string
	InstanceSizeID string
	Namespace      string
	StorageClass   string
	DVAccessModes  []string
	DVVolumeMode   string
	Override       *ApprovalResourceOverride
}

// ApprovalResourceOverride contains the effective admin override values applied
// during CREATE approval.
type ApprovalResourceOverride struct {
	CPURequest      float64
	CPULimit        float64
	MemoryRequestGi float64
	MemoryLimitGi   float64
	DiskGB          int
}

// ClusterCompatibilityResult is the preflight compatibility verdict for one
// cluster candidate under a CREATE placement context.
type ClusterCompatibilityResult struct {
	Cluster              *ent.Cluster
	Eligible             bool
	ReasonCode           string
	ReasonMessage        string
	AdvisoryCode         string
	AdvisoryMessage      string
	RootVolumeResolution *RootVolumeResolution
}

type resolvedApprovalValidationContext struct {
	namespace       *ent.NamespaceRegistry
	template        *ent.Template
	instanceSize    *ent.InstanceSize
	cpuCores        float64
	cpuRequest      float64
	memoryGi        float64
	memoryRequestGi float64
	requiredCaps    []string
}

// ValidateApproval checks:
// 1. Selected cluster exists and is healthy
// 2. Namespace environment matches cluster environment (ADR-0015 §15)
// 3. Instance size overcommit + dedicatedCpuPlacement constraint
// Returns nil if validation passes.
func (v *ApprovalValidator) ValidateApproval(
	ctx context.Context,
	input ApprovalValidationInput,
) error {
	var (
		cl               *ent.Cluster
		policy           *ent.ClusterPolicy
		tpl              *ent.Template
		size             *ent.InstanceSize
		clusterCapSet    map[string]struct{}
		clusterDisplayID string
		cpuCores         float64
		cpuRequest       float64
		memoryGi         float64
		memoryRequestGi  float64
	)

	// 1. Validate cluster exists and is healthy.
	if input.ClusterID != "" {
		var err error
		cl, err = v.client.Cluster.Get(ctx, input.ClusterID)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster not found")
			}
			return fmt.Errorf("query cluster: %w", err)
		}
		if !cl.Enabled {
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("cluster %s is disabled", cl.Name))
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
		if v.policySvc != nil {
			policy, err = v.policySvc.GetByClusterID(ctx, cl.ID)
			if err != nil {
				if ent.IsNotFound(err) {
					return apperrors.BadRequest(
						"CLUSTER_POLICY_NOT_CONFIGURED",
						fmt.Sprintf("selected cluster %s has no cluster policy configured", clusterDisplayID),
					)
				}
				return fmt.Errorf("query cluster policy for %s: %w", cl.ID, err)
			}
		}
	}

	// 2. Validate namespace environment isolation.
	if strings.TrimSpace(input.Namespace) != "" {
		ns, err := v.client.NamespaceRegistry.Query().
			Where(namespaceregistry.NameEQ(strings.TrimSpace(input.Namespace))).
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

	if input.TemplateID != "" {
		var err error
		tpl, err = v.client.Template.Get(ctx, input.TemplateID)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "template not found")
			}
			return fmt.Errorf("query template: %w", err)
		}
	}

	// 3. Validate InstanceSize constraints and capability matching.
	if input.InstanceSizeID != "" {
		var err error
		size, err = v.client.InstanceSize.Get(ctx, input.InstanceSizeID)
		if err != nil {
			if ent.IsNotFound(err) {
				return apperrors.BadRequest(apperrors.CodeValidationFailed, "instance size not found")
			}
			return fmt.Errorf("query instance size: %w", err)
		}

		// Resolve effective dedicated_cpu: consider both the indexed column AND any
		// spec_overrides path that sets dedicatedCpuPlacement (ADR-0036 constraint guard).
		// This prevents bypassing the dedicated+overcommit conflict check by only setting
		// the flag inside spec_overrides while leaving the top-level dedicated_cpu unset.
		effectiveDedicatedCPU := size.DedicatedCPU || hasDedicatedCPUInSpecOverrides(size.SpecOverrides)
		cpuCores = size.CPUCores
		cpuRequest = size.CPURequest
		memoryGi = size.MemoryGi
		memoryRequestGi = size.MemoryRequestGi
		if input.Override != nil {
			if input.Override.CPULimit > 0 {
				cpuCores = input.Override.CPULimit
			}
			if input.Override.CPURequest > 0 {
				cpuRequest = input.Override.CPURequest
			}
			if input.Override.MemoryLimitGi > 0 {
				memoryGi = input.Override.MemoryLimitGi
			}
			if input.Override.MemoryRequestGi > 0 {
				memoryRequestGi = input.Override.MemoryRequestGi
			}
		}
		if err := ValidateOvercommit(cpuCores, cpuRequest, memoryGi, memoryRequestGi, effectiveDedicatedCPU); err != nil {
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

	if tpl != nil && size != nil && !TemplateInstanceSizeCompatible(tpl.SystemLabels, size.SystemLabels) {
		return apperrors.BadRequest(
			"TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH",
			"selected instance size is not compatible with selected template system labels",
		).WithParams(map[string]interface{}{
			"template_system_labels":      NormalizeSystemLabelsForRead(tpl.SystemLabels),
			"instance_size_system_labels": NormalizeSystemLabelsForRead(size.SystemLabels),
		})
	}

	if v.policySvc != nil && cl != nil {
		if err := v.policySvc.ValidateCreatePlacement(ClusterPolicyValidationInput{
			Cluster:         cl,
			Policy:          policy,
			Template:        tpl,
			InstanceSize:    size,
			TargetNamespace: input.Namespace,
			SelectedStorage: input.StorageClass,
			CPUCores:        cpuCores,
			CPURequest:      cpuRequest,
			MemoryGi:        memoryGi,
			MemoryRequestGi: memoryRequestGi,
		}); err != nil {
			return err
		}
	}

	return nil
}

// FilterCompatibleClusters evaluates CREATE placement compatibility against a
// preloaded cluster page. AppErrors are treated as incompatibility and only
// unexpected errors abort the whole operation.
func (v *ApprovalValidator) FilterCompatibleClusters(
	ctx context.Context,
	clusters []*ent.Cluster,
	input ApprovalValidationInput,
) ([]*ent.Cluster, error) {
	results, err := v.EvaluateClusterCompatibility(ctx, clusters, input)
	if err != nil {
		return nil, err
	}
	filtered := make([]*ent.Cluster, 0, len(results))
	for _, result := range results {
		if result.Eligible && result.Cluster != nil {
			filtered = append(filtered, result.Cluster)
		}
	}
	return filtered, nil
}

// EvaluateClusterCompatibility evaluates CREATE placement compatibility for one
// cluster page. Invalid request inputs still return an AppError; per-cluster
// incompatibilities are returned as machine-readable reason codes/messages.
func (v *ApprovalValidator) EvaluateClusterCompatibility(
	ctx context.Context,
	clusters []*ent.Cluster,
	input ApprovalValidationInput,
) ([]ClusterCompatibilityResult, error) {
	if len(clusters) == 0 {
		return nil, nil
	}

	resolved, err := v.resolveValidationContext(ctx, input)
	if err != nil {
		return nil, err
	}
	policyByClusterID, err := v.loadPoliciesForClusters(ctx, clusters)
	if err != nil {
		return nil, err
	}

	results := make([]ClusterCompatibilityResult, 0, len(clusters))
	for _, cl := range clusters {
		if cl == nil {
			continue
		}
		result := ClusterCompatibilityResult{
			Cluster:  cl,
			Eligible: true,
		}
		if err := v.validateResolvedCluster(cl, policyByClusterID[cl.ID], resolved, input); err != nil {
			if appErr, ok := apperrors.IsAppError(err); ok {
				result.Eligible = false
				result.ReasonCode = appErr.Code
				result.ReasonMessage = appErr.Message
				results = append(results, result)
				continue
			}
			return nil, err
		}
		rootVolumeResolution, resolveErr := v.resolveRootVolumeProvisioning(
			ctx,
			cl,
			policyByClusterID[cl.ID],
			resolved.template,
			resolved.instanceSize,
			input,
		)
		if rootVolumeResolution != nil {
			result.RootVolumeResolution = rootVolumeResolution
		}
		if resolveErr != nil {
			if appErr, ok := apperrors.IsAppError(resolveErr); ok {
				result.Eligible = false
				result.ReasonCode = appErr.Code
				result.ReasonMessage = appErr.Message
				results = append(results, result)
				continue
			}
			return nil, resolveErr
		}
		v.attachCloneAdvisory(ctx, &result, resolved, input)
		results = append(results, result)
	}
	return results, nil
}

// EvaluateClusterPlacement evaluates one selected cluster and returns a
// machine-readable compatibility verdict. Request-shape errors are still
// returned directly; cluster-specific incompatibilities are encoded in the
// returned result with Eligible=false.
func (v *ApprovalValidator) EvaluateClusterPlacement(
	ctx context.Context,
	input ApprovalValidationInput,
) (*ClusterCompatibilityResult, error) {
	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster is required for cluster policy evaluation")
	}

	resolved, err := v.resolveValidationContext(ctx, input)
	if err != nil {
		return nil, err
	}

	cl, err := v.client.Cluster.Get(ctx, clusterID)
	if err != nil {
		if ent.IsNotFound(err) {
			return &ClusterCompatibilityResult{
				Eligible:      false,
				ReasonCode:    apperrors.CodeValidationFailed,
				ReasonMessage: "selected cluster not found",
			}, nil
		}
		return nil, fmt.Errorf("query cluster: %w", err)
	}

	var policy *ent.ClusterPolicy
	if v.policySvc != nil {
		policy, err = v.policySvc.GetByClusterID(ctx, cl.ID)
		if err != nil && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("query cluster policy for %s: %w", cl.ID, err)
		}
	}

	result := &ClusterCompatibilityResult{
		Cluster:  cl,
		Eligible: true,
	}
	if err := v.validateResolvedCluster(cl, policy, resolved, input); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			result.Eligible = false
			result.ReasonCode = appErr.Code
			result.ReasonMessage = appErr.Message
			return result, nil
		}
		return nil, err
	}
	rootVolumeResolution, resolveErr := v.resolveRootVolumeProvisioning(ctx, cl, policy, resolved.template, resolved.instanceSize, input)
	if rootVolumeResolution != nil {
		result.RootVolumeResolution = rootVolumeResolution
	}
	if resolveErr != nil {
		if appErr, ok := apperrors.IsAppError(resolveErr); ok {
			result.Eligible = false
			result.ReasonCode = appErr.Code
			result.ReasonMessage = appErr.Message
			return result, nil
		}
		return nil, resolveErr
	}
	v.attachCloneAdvisory(ctx, result, resolved, input)
	return result, nil
}

func (v *ApprovalValidator) resolveValidationContext(
	ctx context.Context,
	input ApprovalValidationInput,
) (*resolvedApprovalValidationContext, error) {
	resolved := &resolvedApprovalValidationContext{}

	if strings.TrimSpace(input.Namespace) != "" {
		ns, err := v.client.NamespaceRegistry.Query().
			Where(namespaceregistry.NameEQ(strings.TrimSpace(input.Namespace))).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "namespace not found in registry")
			}
			return nil, fmt.Errorf("query namespace registry by name: %w", err)
		}
		if !ns.Enabled {
			return nil, apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("namespace %s is disabled", ns.Name))
		}
		resolved.namespace = ns
	}

	if input.TemplateID != "" {
		tpl, err := v.client.Template.Get(ctx, input.TemplateID)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "template not found")
			}
			return nil, fmt.Errorf("query template: %w", err)
		}
		resolved.template = tpl
	}

	if input.InstanceSizeID != "" {
		size, err := v.client.InstanceSize.Get(ctx, input.InstanceSizeID)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "instance size not found")
			}
			return nil, fmt.Errorf("query instance size: %w", err)
		}

		effectiveDedicatedCPU := size.DedicatedCPU || hasDedicatedCPUInSpecOverrides(size.SpecOverrides)
		resolved.instanceSize = size
		resolved.cpuCores = size.CPUCores
		resolved.cpuRequest = size.CPURequest
		resolved.memoryGi = size.MemoryGi
		resolved.memoryRequestGi = size.MemoryRequestGi
		if input.Override != nil {
			if input.Override.CPULimit > 0 {
				resolved.cpuCores = input.Override.CPULimit
			}
			if input.Override.CPURequest > 0 {
				resolved.cpuRequest = input.Override.CPURequest
			}
			if input.Override.MemoryLimitGi > 0 {
				resolved.memoryGi = input.Override.MemoryLimitGi
			}
			if input.Override.MemoryRequestGi > 0 {
				resolved.memoryRequestGi = input.Override.MemoryRequestGi
			}
		}
		if err := ValidateOvercommit(
			resolved.cpuCores,
			resolved.cpuRequest,
			resolved.memoryGi,
			resolved.memoryRequestGi,
			effectiveDedicatedCPU,
		); err != nil {
			return nil, err
		}
		resolved.requiredCaps = ExtractRequiredCapabilities(size)
	}
	if resolved.template != nil && resolved.instanceSize != nil &&
		!TemplateInstanceSizeCompatible(resolved.template.SystemLabels, resolved.instanceSize.SystemLabels) {
		return nil, apperrors.BadRequest(
			"TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH",
			"selected instance size is not compatible with selected template system labels",
		).WithParams(map[string]interface{}{
			"template_system_labels":      NormalizeSystemLabelsForRead(resolved.template.SystemLabels),
			"instance_size_system_labels": NormalizeSystemLabelsForRead(resolved.instanceSize.SystemLabels),
		})
	}

	return resolved, nil
}

func (v *ApprovalValidator) loadPoliciesForClusters(
	ctx context.Context,
	clusters []*ent.Cluster,
) (map[string]*ent.ClusterPolicy, error) {
	out := make(map[string]*ent.ClusterPolicy, len(clusters))
	if len(clusters) == 0 {
		return out, nil
	}
	clusterIDs := make([]string, 0, len(clusters))
	for _, cl := range clusters {
		if cl != nil && strings.TrimSpace(cl.ID) != "" {
			clusterIDs = append(clusterIDs, cl.ID)
		}
	}
	if len(clusterIDs) == 0 {
		return out, nil
	}
	policies, err := v.client.ClusterPolicy.Query().
		Where(clusterpolicy.ClusterIDIn(clusterIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query cluster policies: %w", err)
	}
	for _, policy := range policies {
		if policy != nil && policy.ClusterID != "" {
			out[policy.ClusterID] = policy
		}
	}
	return out, nil
}

func (v *ApprovalValidator) validateResolvedCluster(
	cl *ent.Cluster,
	policy *ent.ClusterPolicy,
	resolved *resolvedApprovalValidationContext,
	input ApprovalValidationInput,
) error {
	if cl == nil {
		return apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster not found")
	}
	if cl.Status != cluster.StatusHEALTHY {
		return apperrors.BadRequest(
			apperrors.CodeValidationFailed,
			fmt.Sprintf("cluster %s is not healthy (status: %s)", cl.Name, cl.Status),
		)
	}

	if resolved != nil && resolved.namespace != nil {
		if err := validateNamespaceClusterEnvironment(
			string(resolved.namespace.Environment),
			string(cl.Environment),
		); err != nil {
			return err
		}
	}

	if resolved != nil && resolved.instanceSize != nil {
		clusterCapSet := buildClusterCapabilitySet(cl.EnabledFeatures)
		missing := MissingCapabilities(resolved.requiredCaps, clusterCapSet)
		if len(missing) > 0 {
			return apperrors.BadRequest(
				apperrors.CodeValidationFailed,
				fmt.Sprintf("cluster %s is missing required capabilities: %s", clusterDisplayName(cl), strings.Join(missing, ", ")),
			)
		}
	}

	if v.policySvc != nil {
		if policy == nil {
			return apperrors.BadRequest(
				"CLUSTER_POLICY_NOT_CONFIGURED",
				fmt.Sprintf("selected cluster %s has no cluster policy configured", clusterDisplayName(cl)),
			)
		}
		var (
			template        *ent.Template
			instanceSize    *ent.InstanceSize
			cpuCores        float64
			cpuRequest      float64
			memoryGi        float64
			memoryRequestGi float64
		)
		if resolved != nil {
			template = resolved.template
			instanceSize = resolved.instanceSize
			cpuCores = resolved.cpuCores
			cpuRequest = resolved.cpuRequest
			memoryGi = resolved.memoryGi
			memoryRequestGi = resolved.memoryRequestGi
		}
		if err := v.policySvc.ValidateCreatePlacement(ClusterPolicyValidationInput{
			Cluster:         cl,
			Policy:          policy,
			Template:        template,
			InstanceSize:    instanceSize,
			TargetNamespace: input.Namespace,
			SelectedStorage: input.StorageClass,
			CPUCores:        cpuCores,
			CPURequest:      cpuRequest,
			MemoryGi:        memoryGi,
			MemoryRequestGi: memoryRequestGi,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (v *ApprovalValidator) attachCloneAdvisory(
	ctx context.Context,
	result *ClusterCompatibilityResult,
	resolved *resolvedApprovalValidationContext,
	input ApprovalValidationInput,
) {
	if v == nil || v.vmService == nil || result == nil || result.Cluster == nil || !result.Eligible {
		return
	}
	if resolved == nil || resolved.template == nil {
		return
	}
	if EffectiveTemplateSourceType(
		resolved.template.SourceType,
		resolved.template.ImageURL,
		resolved.template.PvcName,
	) != TemplateSourceCDIPVCClone {
		return
	}
	targetStorageClass := strings.TrimSpace(input.StorageClass)
	if result.RootVolumeResolution != nil && strings.TrimSpace(result.RootVolumeResolution.EffectiveStorageClass) != "" {
		targetStorageClass = strings.TrimSpace(result.RootVolumeResolution.EffectiveStorageClass)
	}
	if targetStorageClass == "" {
		targetStorageClass = strings.TrimSpace(result.Cluster.DefaultStorageClass)
	}
	advisory, err := v.vmService.GetPVCCloneAdvisory(
		ctx,
		result.Cluster.ID,
		input.Namespace,
		resolved.template.PvcNamespace,
		resolved.template.PvcName,
		targetStorageClass,
	)
	if err != nil || advisory == nil {
		return
	}
	result.AdvisoryCode = advisory.Code
	result.AdvisoryMessage = advisory.Message
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
func ValidateOvercommit(cpuCores, cpuRequest, memoryGi, memoryRequestGi float64, dedicatedCPU bool) error {
	if cpuCores > 0 && !IsHalfStep(cpuCores) {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("CPU limit (%.3g) must use 0.5-step values (0.5, 1.0, 1.5, ...)", cpuCores))
	}
	if cpuRequest > 0 && !IsHalfStep(cpuRequest) {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("CPU request (%.3g) must use 0.5-step values (0.5, 1.0, 1.5, ...)", cpuRequest))
	}
	if memoryGi > 0 && !IsHalfStep(memoryGi) {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("memory limit (%.3gGi) must use 0.5-step values (0.5Gi, 1.0Gi, 1.5Gi, ...)", memoryGi))
	}
	if memoryRequestGi > 0 && !IsHalfStep(memoryRequestGi) {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("memory request (%.3gGi) must use 0.5-step values (0.5Gi, 1.0Gi, 1.5Gi, ...)", memoryRequestGi))
	}

	// cpu_request == 0 means "use cpu_cores" (no overcommit).
	overcommitActive := cpuRequest > 0 && cpuRequest != cpuCores

	// Rule 1: Dedicated CPU + overcommit is mutually exclusive.
	// KubeVirt: dedicatedCpuPlacement requires Guaranteed QoS (request == limit).
	if dedicatedCPU && overcommitActive {
		return apperrors.BadRequest("DEDICATED_CPU_OVERCOMMIT_CONFLICT",
			fmt.Sprintf("dedicated CPU requires Guaranteed QoS: CPU request (%.1f) must equal CPU limit (%.1f); overcommit is not allowed with dedicatedCpuPlacement",
				cpuRequest, cpuCores))
	}

	// Rule 2: CPU request cannot exceed limit (invalid overcommit direction).
	if overcommitActive && cpuRequest > cpuCores {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("CPU request (%.1f) cannot exceed CPU limit (%.1f)", cpuRequest, cpuCores))
	}

	// Rule 3: Memory request cannot exceed limit.
	if memoryRequestGi > 0 && memoryRequestGi > memoryGi {
		return apperrors.BadRequest("OVERCOMMIT_INVALID",
			fmt.Sprintf("memory request (%.1fGi) cannot exceed memory limit (%.1fGi)", memoryRequestGi, memoryGi))
	}
	return nil
}

var hugepagesPattern = regexp.MustCompile(`hugepages[-_:]?(\d+(?:mi|gi))`)

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

// hasDedicatedCPUInSpecOverrides is a package-internal alias for the exported
// HasDedicatedCPUInSpecOverrides, kept for readability at the call site.
// Single source of truth lives in instancesize_validator.go.
func hasDedicatedCPUInSpecOverrides(spec map[string]interface{}) bool {
	return HasDedicatedCPUInSpecOverrides(spec)
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
