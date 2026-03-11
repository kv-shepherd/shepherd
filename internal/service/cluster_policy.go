package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/clusterpolicy"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

// ClusterPolicyInput is the admin-managed policy payload for one cluster.
type ClusterPolicyInput struct {
	AllowCPUOvercommit           bool
	AllowMemoryOvercommit        bool
	AllowDedicatedCPU            bool
	AllowGPU                     bool
	AllowSRIOV                   bool
	AllowHugepages               bool
	AllowedHugepagesSizes        []string
	AllowCDIClone                bool
	AllowedCloneSourceNamespaces []string
	AllowedStorageClasses        []string
}

// ClusterPolicyValidationInput contains the effective request state evaluated
// after capability matching and resource overrides are resolved.
type ClusterPolicyValidationInput struct {
	Cluster         *ent.Cluster
	Policy          *ent.ClusterPolicy
	Template        *ent.Template
	InstanceSize    *ent.InstanceSize
	TargetNamespace string
	SelectedStorage string
	CPUCores        float64
	CPURequest      float64
	MemoryGi        float64
	MemoryRequestGi float64
}

// ClusterPolicyService manages explicit cluster governance policy rows.
type ClusterPolicyService struct {
	client *ent.Client
}

// NewClusterPolicyService creates a new ClusterPolicyService.
func NewClusterPolicyService(client *ent.Client) *ClusterPolicyService {
	return &ClusterPolicyService{client: client}
}

// WithClient returns a shallow copy bound to a different ent client.
// This is used for transaction-scoped work without constructing services
// outside the composition root.
func (s *ClusterPolicyService) WithClient(client *ent.Client) *ClusterPolicyService {
	if s == nil {
		return &ClusterPolicyService{client: client}
	}
	clone := *s
	clone.client = client
	return &clone
}

// GetByClusterID returns the policy row for one cluster.
func (s *ClusterPolicyService) GetByClusterID(ctx context.Context, clusterID string) (*ent.ClusterPolicy, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("cluster policy service is not configured")
	}
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("cluster id is required")
	}
	policy, err := s.client.ClusterPolicy.Query().
		Where(clusterpolicy.ClusterIDEQ(clusterID)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return policy, nil
}

// Upsert creates or replaces the policy row for one cluster.
func (s *ClusterPolicyService) Upsert(
	ctx context.Context,
	clusterID string,
	input ClusterPolicyInput,
	actor string,
) (*ent.ClusterPolicy, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("cluster policy service is not configured")
	}
	clusterID = strings.TrimSpace(clusterID)
	actor = strings.TrimSpace(actor)
	if clusterID == "" {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "cluster id is required")
	}
	if actor == "" {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "actor is required")
	}
	if _, err := s.client.Cluster.Get(ctx, clusterID); err != nil {
		if ent.IsNotFound(err) {
			return nil, apperrors.NotFound("CLUSTER_NOT_FOUND", fmt.Sprintf("cluster %s not found", clusterID))
		}
		return nil, fmt.Errorf("get cluster %s: %w", clusterID, err)
	}

	normalized := normalizeClusterPolicyInput(input)
	existing, err := s.client.ClusterPolicy.Query().
		Where(clusterpolicy.ClusterIDEQ(clusterID)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query cluster policy for %s: %w", clusterID, err)
	}

	if existing == nil {
		id, _ := uuid.NewV7()
		return s.client.ClusterPolicy.Create().
			SetID(id.String()).
			SetClusterID(clusterID).
			SetAllowCPUOvercommit(normalized.AllowCPUOvercommit).
			SetAllowMemoryOvercommit(normalized.AllowMemoryOvercommit).
			SetAllowDedicatedCPU(normalized.AllowDedicatedCPU).
			SetAllowGpu(normalized.AllowGPU).
			SetAllowSriov(normalized.AllowSRIOV).
			SetAllowHugepages(normalized.AllowHugepages).
			SetAllowedHugepagesSizes(normalized.AllowedHugepagesSizes).
			SetAllowCdiClone(normalized.AllowCDIClone).
			SetAllowedCloneSourceNamespaces(normalized.AllowedCloneSourceNamespaces).
			SetAllowedStorageClasses(normalized.AllowedStorageClasses).
			SetCreatedBy(actor).
			Save(ctx)
	}

	return s.client.ClusterPolicy.UpdateOneID(existing.ID).
		SetAllowCPUOvercommit(normalized.AllowCPUOvercommit).
		SetAllowMemoryOvercommit(normalized.AllowMemoryOvercommit).
		SetAllowDedicatedCPU(normalized.AllowDedicatedCPU).
		SetAllowGpu(normalized.AllowGPU).
		SetAllowSriov(normalized.AllowSRIOV).
		SetAllowHugepages(normalized.AllowHugepages).
		SetAllowedHugepagesSizes(normalized.AllowedHugepagesSizes).
		SetAllowCdiClone(normalized.AllowCDIClone).
		SetAllowedCloneSourceNamespaces(normalized.AllowedCloneSourceNamespaces).
		SetAllowedStorageClasses(normalized.AllowedStorageClasses).
		SetUpdatedBy(actor).
		Save(ctx)
}

// ValidateCreatePlacement enforces the explicit policy row after capability
// matching has already succeeded.
func (s *ClusterPolicyService) ValidateCreatePlacement(input ClusterPolicyValidationInput) error {
	if input.Cluster == nil {
		return apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster is required for cluster policy evaluation")
	}
	if input.Policy == nil {
		return apperrors.BadRequest(
			"CLUSTER_POLICY_NOT_CONFIGURED",
			fmt.Sprintf("selected cluster %s has no cluster policy configured", clusterDisplayName(input.Cluster)),
		)
	}
	if input.InstanceSize == nil {
		return nil
	}

	policy := input.Policy
	effectiveDedicatedCPU := input.InstanceSize.DedicatedCPU || hasDedicatedCPUInSpecOverrides(input.InstanceSize.SpecOverrides)
	cpuOvercommit := input.CPURequest > 0 && input.CPURequest != input.CPUCores
	memoryOvercommit := input.MemoryRequestGi > 0 && input.MemoryRequestGi != input.MemoryGi
	if cpuOvercommit && !policy.AllowCPUOvercommit {
		return policyDeny("CPU overcommit is disabled by cluster policy")
	}
	if memoryOvercommit && !policy.AllowMemoryOvercommit {
		return policyDeny("memory overcommit is disabled by cluster policy")
	}
	if effectiveDedicatedCPU && !policy.AllowDedicatedCPU {
		return policyDeny("dedicated CPU is disabled by cluster policy")
	}
	if (input.InstanceSize.RequiresGpu || hasGPURequirement(input.InstanceSize.SpecOverrides)) && !policy.AllowGpu {
		return policyDeny("GPU workloads are disabled by cluster policy")
	}
	if (input.InstanceSize.RequiresSriov || hasSRIOVRequirement(input.InstanceSize.SpecOverrides)) && !policy.AllowSriov {
		return policyDeny("SR-IOV workloads are disabled by cluster policy")
	}

	hugepagesRequested := input.InstanceSize.RequiresHugepages
	hugepagesSize := normalizeHugepagesSize(input.InstanceSize.HugepagesSize)
	if hugepagesSize == "" {
		hugepagesSize = normalizeHugepagesSize(extractHugepagesSize(input.InstanceSize.SpecOverrides))
	}
	if hugepagesSize != "" {
		hugepagesRequested = true
	}
	if hugepagesRequested {
		if !policy.AllowHugepages {
			return policyDeny("hugepages are disabled by cluster policy")
		}
		if len(policy.AllowedHugepagesSizes) > 0 {
			if hugepagesSize == "" {
				return policyDeny("cluster policy requires an explicit hugepages size")
			}
			if !stringSliceContains(policy.AllowedHugepagesSizes, hugepagesSize, normalizeHugepagesSize) {
				return policyDeny(fmt.Sprintf("hugepages size %q is not allowed by cluster policy", hugepagesSize))
			}
		}
	}

	if input.Template == nil {
		return nil
	}

	sourceType := EffectiveTemplateSourceType(
		input.Template.SourceType,
		input.Template.ImageURL,
		input.Template.PvcName,
	)
	if sourceType == TemplateSourceCDIPVCClone {
		if !policy.AllowCdiClone {
			return policyDeny("CDI PVC clone boot is disabled by cluster policy")
		}
		if len(policy.AllowedCloneSourceNamespaces) > 0 {
			sourceNamespace := strings.TrimSpace(input.Template.PvcNamespace)
			if sourceNamespace == "" {
				sourceNamespace = strings.TrimSpace(input.TargetNamespace)
			}
			if !stringSliceContains(policy.AllowedCloneSourceNamespaces, sourceNamespace, normalizeNamespaceName) {
				return policyDeny(fmt.Sprintf("clone source namespace %q is not allowed by cluster policy", sourceNamespace))
			}
		}
	}

	if sourceType == TemplateSourceCDIImageImport || sourceType == TemplateSourceCDIPVCClone {
		if len(policy.AllowedStorageClasses) > 0 {
			selectedStorageClass := normalizeStorageClassName(input.SelectedStorage)
			if selectedStorageClass == "" {
				defaultStorageClass := normalizeStorageClassName(input.Cluster.DefaultStorageClass)
				if defaultStorageClass == "" || !stringSliceContains(
					policy.AllowedStorageClasses,
					defaultStorageClass,
					normalizeStorageClassName,
				) {
					return policyDenyWithCode(
						"CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
						"cluster policy requires an explicit allowed storage class",
					)
				}
				selectedStorageClass = defaultStorageClass
			}
			if selectedStorageClass == "" {
				return policyDenyWithCode(
					"CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
					"cluster policy requires an explicit allowed storage class",
				)
			}
			if !stringSliceContains(
				policy.AllowedStorageClasses,
				selectedStorageClass,
				normalizeStorageClassName,
			) {
				return policyDeny(
					fmt.Sprintf("storage class %q is not allowed by cluster policy", selectedStorageClass),
				)
			}
		}
	}

	return nil
}

func normalizeClusterPolicyInput(input ClusterPolicyInput) ClusterPolicyInput {
	input.AllowedHugepagesSizes = normalizeStringList(input.AllowedHugepagesSizes, normalizeHugepagesSize)
	input.AllowedCloneSourceNamespaces = normalizeStringList(input.AllowedCloneSourceNamespaces, normalizeNamespaceName)
	input.AllowedStorageClasses = normalizeStringList(input.AllowedStorageClasses, normalizeStorageClassName)
	return input
}

func normalizeStringList(items []string, normalize func(string) string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, raw := range items {
		value := strings.TrimSpace(raw)
		if normalize != nil {
			value = normalize(value)
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringSliceContains(items []string, candidate string, normalize func(string) string) bool {
	if normalize != nil {
		candidate = normalize(candidate)
	}
	for _, item := range items {
		value := item
		if normalize != nil {
			value = normalize(value)
		}
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeNamespaceName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStorageClassName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func policyDeny(message string) error {
	return policyDenyWithCode("CLUSTER_POLICY_DENIED", message)
}

func policyDenyWithCode(code, message string) error {
	return apperrors.BadRequest(code, message)
}

func clusterDisplayName(cl *ent.Cluster) string {
	if cl == nil {
		return ""
	}
	if strings.TrimSpace(cl.Name) != "" {
		return cl.Name
	}
	return cl.ID
}
