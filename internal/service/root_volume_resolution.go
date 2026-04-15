package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

const (
	rootVolumeIntentAuto     = "auto"
	rootVolumeIntentExplicit = "explicit"
)

const (
	rootVolumeResolutionNotApplicable        = "not_applicable"
	rootVolumeResolutionResolved             = "resolved"
	rootVolumeResolutionStorageClassRequired = "storage_class_required"
	rootVolumeResolutionModeRequired         = "mode_required"
	rootVolumeResolutionProfileIncomplete    = "profile_incomplete"
	rootVolumeResolutionUnsupported          = "unsupported"
	rootVolumeReasonStorageClassRequired     = "ROOT_VOLUME_STORAGE_CLASS_REQUIRED"
	rootVolumeReasonModeRequired             = "ROOT_VOLUME_MODE_REQUIRED"
	rootVolumeReasonProfileIncomplete        = "ROOT_VOLUME_PROFILE_INCOMPLETE"
	rootVolumeReasonUnsupported              = "ROOT_VOLUME_MODE_UNSUPPORTED"
)

// RootVolumeResolution captures whether CDI-backed root-volume provisioning can
// be fully resolved for a selected cluster during approval.
type RootVolumeResolution struct {
	IntentMode            string                           `json:"intent_mode,omitempty"`
	State                 string                           `json:"state,omitempty"`
	Message               string                           `json:"message,omitempty"`
	RequestedStorageClass string                           `json:"requested_storage_class,omitempty"`
	EffectiveStorageClass string                           `json:"effective_storage_class,omitempty"`
	RequestedAccessModes  []string                         `json:"requested_access_modes,omitempty"`
	RequestedVolumeMode   string                           `json:"requested_volume_mode,omitempty"`
	EffectiveAccessModes  []string                         `json:"effective_access_modes,omitempty"`
	EffectiveVolumeMode   string                           `json:"effective_volume_mode,omitempty"`
	ModeOptions           []domain.StorageClaimPropertySet `json:"mode_options,omitempty"`
}

func (v *ApprovalValidator) resolveRootVolumeProvisioning(
	ctx context.Context,
	clusterEntity *ent.Cluster,
	policy *ent.ClusterPolicy,
	templateEntity *ent.Template,
	instanceSizeEntity *ent.InstanceSize,
	input ApprovalValidationInput,
) (*RootVolumeResolution, error) {
	if templateEntity == nil || instanceSizeEntity == nil {
		return nil, nil
	}

	if !templateRequiresRootDataVolume(templateEntity) {
		return &RootVolumeResolution{
			IntentMode: rootVolumeIntentMode(instanceSizeEntity),
			State:      rootVolumeResolutionNotApplicable,
		}, nil
	}
	if clusterEntity == nil {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "selected cluster is required for root volume resolution")
	}
	if v == nil || v.vmService == nil {
		return nil, nil
	}

	resolution := &RootVolumeResolution{
		IntentMode:            rootVolumeIntentMode(instanceSizeEntity),
		RequestedStorageClass: strings.TrimSpace(input.StorageClass),
	}
	requested := requestedRootVolumeClaimPropertySet(instanceSizeEntity, input)
	explicitRequested := claimPropertySetKey(requested) != ""
	candidateStorageClasses := rootVolumeCandidateStorageClasses(clusterEntity, policy)
	effectiveStorageClass := strings.TrimSpace(input.StorageClass)
	if effectiveStorageClass == "" {
		if len(candidateStorageClasses) == 1 {
			effectiveStorageClass = candidateStorageClasses[0]
		} else {
			resolution.State = rootVolumeResolutionStorageClassRequired
			if len(candidateStorageClasses) == 0 {
				resolution.Message = "no eligible storage class is available for CDI-backed root volume provisioning"
			} else {
				resolution.Message = "approval must select a target storage class before root volume provisioning can be resolved"
			}
			return resolution, apperrors.BadRequest(rootVolumeReasonStorageClassRequired, resolution.Message)
		}
	}
	resolution.EffectiveStorageClass = effectiveStorageClass

	resolution.RequestedAccessModes = cloneStringSlice(requested.AccessModes)
	resolution.RequestedVolumeMode = requested.VolumeMode

	storageProfile, err := v.vmService.GetStorageProfile(ctx, clusterEntity.ID, effectiveStorageClass)
	if err != nil {
		resolution.State = rootVolumeResolutionProfileIncomplete
		resolution.Message = fmt.Sprintf(
			"unable to read StorageProfile for storage class %q on cluster %s: %v",
			effectiveStorageClass,
			clusterDisplayName(clusterEntity),
			err,
		)
		return resolution, apperrors.BadRequest(rootVolumeReasonProfileIncomplete, resolution.Message)
	}
	claimPropertySets := normalizeClaimPropertySets(storageProfile)
	if len(claimPropertySets) == 0 {
		if explicitRequested {
			resolution.State = rootVolumeResolutionResolved
			resolution.Message = fmt.Sprintf(
				"storage class %q does not expose claimPropertySets in StorageProfile; using explicit root volume mode %s without automatic StorageProfile validation",
				effectiveStorageClass,
				formatClaimPropertySet(requested),
			)
			resolution.EffectiveAccessModes = cloneStringSlice(requested.AccessModes)
			resolution.EffectiveVolumeMode = requested.VolumeMode
			return resolution, nil
		}
		resolution.State = rootVolumeResolutionProfileIncomplete
		resolution.Message = fmt.Sprintf(
			"storage class %q does not expose claimPropertySets in StorageProfile; approval cannot resolve root volume mode automatically",
			effectiveStorageClass,
		)
		return resolution, apperrors.BadRequest(rootVolumeReasonProfileIncomplete, resolution.Message)
	}
	if len(claimPropertySets) > 1 {
		resolution.ModeOptions = cloneClaimPropertySets(claimPropertySets)
	}

	if resolution.IntentMode == rootVolumeIntentAuto && !explicitRequested {
		if len(claimPropertySets) == 1 {
			resolution.State = rootVolumeResolutionResolved
			resolution.EffectiveAccessModes = cloneStringSlice(claimPropertySets[0].AccessModes)
			resolution.EffectiveVolumeMode = claimPropertySets[0].VolumeMode
			return resolution, nil
		}
		resolution.State = rootVolumeResolutionModeRequired
		resolution.Message = fmt.Sprintf(
			"storage class %q supports multiple root volume modes; approval must choose one explicit combination",
			effectiveStorageClass,
		)
		return resolution, apperrors.BadRequest(rootVolumeReasonModeRequired, resolution.Message)
	}

	for _, candidate := range claimPropertySets {
		if claimPropertySetsEqual(candidate, requested) {
			resolution.State = rootVolumeResolutionResolved
			resolution.EffectiveAccessModes = cloneStringSlice(candidate.AccessModes)
			resolution.EffectiveVolumeMode = candidate.VolumeMode
			return resolution, nil
		}
	}

	resolution.State = rootVolumeResolutionUnsupported
	resolution.Message = fmt.Sprintf(
		"storage class %q does not support the requested root volume mode %s",
		effectiveStorageClass,
		formatClaimPropertySet(requested),
	)
	return resolution, apperrors.BadRequest(rootVolumeReasonUnsupported, resolution.Message)
}

func templateRequiresRootDataVolume(templateEntity *ent.Template) bool {
	if templateEntity == nil {
		return false
	}
	sourceType := EffectiveTemplateSourceType(templateEntity.SourceType, templateEntity.ImageURL, templateEntity.PvcName)
	return sourceType == TemplateSourceCDIImageImport || sourceType == TemplateSourceCDIPVCClone
}

func rootVolumeIntentMode(instanceSizeEntity *ent.InstanceSize) string {
	if instanceSizeEntity == nil {
		return rootVolumeIntentAuto
	}
	if len(instanceSizeEntity.DvAccessModes) > 0 || strings.TrimSpace(instanceSizeEntity.DvVolumeMode) != "" {
		return rootVolumeIntentExplicit
	}
	return rootVolumeIntentAuto
}

func rootVolumeCandidateStorageClasses(clusterEntity *ent.Cluster, policy *ent.ClusterPolicy) []string {
	if clusterEntity == nil {
		return nil
	}
	detected := normalizeStorageClassList(append([]string{clusterEntity.DefaultStorageClass}, clusterEntity.StorageClasses...))
	allowed := normalizeStorageClassList(nil)
	if policy != nil {
		allowed = normalizeStorageClassList(policy.AllowedStorageClasses)
	}
	if len(allowed) == 0 {
		return detected
	}

	detectedSet := make(map[string]struct{}, len(detected))
	for _, item := range detected {
		detectedSet[normalizeStorageClassName(item)] = struct{}{}
	}
	intersection := make([]string, 0, len(allowed))
	for _, item := range allowed {
		if _, ok := detectedSet[normalizeStorageClassName(item)]; ok {
			intersection = append(intersection, item)
		}
	}
	if len(intersection) > 0 {
		return intersection
	}
	return allowed
}

func normalizeStorageClassList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		normalized := normalizeStorageClassName(trimmed)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		items = append(items, trimmed)
	}
	return items
}

func requestedRootVolumeClaimPropertySet(
	instanceSizeEntity *ent.InstanceSize,
	input ApprovalValidationInput,
) domain.StorageClaimPropertySet {
	if len(input.DVAccessModes) > 0 || strings.TrimSpace(input.DVVolumeMode) != "" {
		return normalizeClaimPropertySet(domain.StorageClaimPropertySet{
			AccessModes: input.DVAccessModes,
			VolumeMode:  input.DVVolumeMode,
		})
	}
	return normalizeClaimPropertySet(domain.StorageClaimPropertySet{
		AccessModes: instanceSizeEntity.DvAccessModes,
		VolumeMode:  instanceSizeEntity.DvVolumeMode,
	})
}

func normalizeClaimPropertySets(storageProfile *domain.StorageProfile) []domain.StorageClaimPropertySet {
	if storageProfile == nil || len(storageProfile.ClaimPropertySets) == 0 {
		return nil
	}
	items := make([]domain.StorageClaimPropertySet, 0, len(storageProfile.ClaimPropertySets))
	seen := make(map[string]struct{}, len(storageProfile.ClaimPropertySets))
	for _, raw := range storageProfile.ClaimPropertySets {
		set := normalizeClaimPropertySet(raw)
		key := claimPropertySetKey(set)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, set)
	}
	sort.Slice(items, func(i, j int) bool {
		return claimPropertySetKey(items[i]) < claimPropertySetKey(items[j])
	})
	return items
}

func normalizeClaimPropertySet(input domain.StorageClaimPropertySet) domain.StorageClaimPropertySet {
	set := domain.StorageClaimPropertySet{
		AccessModes: cloneStringSlice(input.AccessModes),
		VolumeMode:  strings.TrimSpace(input.VolumeMode),
	}
	if len(set.AccessModes) > 0 {
		seen := make(map[string]struct{}, len(set.AccessModes))
		normalized := make([]string, 0, len(set.AccessModes))
		for _, raw := range set.AccessModes {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			normalized = append(normalized, value)
		}
		sort.Strings(normalized)
		set.AccessModes = normalized
	}
	return set
}

func claimPropertySetKey(input domain.StorageClaimPropertySet) string {
	set := normalizeClaimPropertySet(input)
	if len(set.AccessModes) == 0 || set.VolumeMode == "" {
		return ""
	}
	return strings.Join(set.AccessModes, ",") + "|" + set.VolumeMode
}

func claimPropertySetsEqual(left, right domain.StorageClaimPropertySet) bool {
	return claimPropertySetKey(left) == claimPropertySetKey(right)
}

func formatClaimPropertySet(input domain.StorageClaimPropertySet) string {
	set := normalizeClaimPropertySet(input)
	if len(set.AccessModes) == 0 && set.VolumeMode == "" {
		return "unspecified"
	}
	if len(set.AccessModes) == 0 {
		return set.VolumeMode
	}
	return fmt.Sprintf("%s + %s", set.VolumeMode, strings.Join(set.AccessModes, "/"))
}

func cloneClaimPropertySets(items []domain.StorageClaimPropertySet) []domain.StorageClaimPropertySet {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.StorageClaimPropertySet, len(items))
	for i := range items {
		out[i] = domain.StorageClaimPropertySet{
			AccessModes: cloneStringSlice(items[i].AccessModes),
			VolumeMode:  items[i].VolumeMode,
		}
	}
	return out
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
