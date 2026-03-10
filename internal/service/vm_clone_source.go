package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/provider"
)

const pvcCloneHostAssistedFallbackLikelyCode = "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY"
const (
	cloneStrategyHostAssisted = "copy"
	volumeModeBlock           = "Block"
)

// PlacementAdvisory captures a non-blocking placement caveat.
type PlacementAdvisory struct {
	Code    string
	Message string
}

// ValidatePVCCloneSource verifies that the selected cluster can safely clone
// from the requested source PVC before CREATE approval is committed.
func (s *VMService) ValidatePVCCloneSource(
	ctx context.Context,
	cluster, targetNamespace, sourceNamespace, sourcePVC string,
	targetDiskGB int,
	targetStorageClass string,
) error {
	if s == nil || s.infra == nil {
		return nil
	}
	validator, ok := s.infra.(provider.PVCClonePreflightProvider)
	if !ok {
		return nil
	}

	cluster = strings.TrimSpace(cluster)
	targetNamespace = strings.TrimSpace(targetNamespace)
	sourceNamespace = strings.TrimSpace(sourceNamespace)
	sourcePVC = strings.TrimSpace(sourcePVC)

	if cluster == "" {
		return fmt.Errorf("cluster is required for pvc clone preflight")
	}
	if targetNamespace == "" {
		return fmt.Errorf("target namespace is required for pvc clone preflight")
	}
	if sourcePVC == "" {
		return fmt.Errorf("source pvc name is required for pvc clone preflight")
	}
	if sourceNamespace == "" {
		sourceNamespace = targetNamespace
	}

	sourceClaim, err := validator.GetPersistentVolumeClaim(ctx, cluster, sourceNamespace, sourcePVC)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apperrors.BadRequest(
				"PVC_CLONE_SOURCE_NOT_FOUND",
				fmt.Sprintf("source PVC %s/%s was not found on selected cluster", sourceNamespace, sourcePVC),
			)
		}
		return fmt.Errorf("get source pvc %s/%s on cluster %s: %w", sourceNamespace, sourcePVC, cluster, err)
	}
	if validateErr := validateCloneTargetSize(ctx, validator, cluster, sourceClaim, targetDiskGB, targetStorageClass); validateErr != nil {
		return validateErr
	}

	consumers, err := validator.ListPodsUsingPVC(ctx, cluster, sourceNamespace, sourcePVC)
	if err != nil {
		return fmt.Errorf("list pods using source pvc %s/%s on cluster %s: %w", sourceNamespace, sourcePVC, cluster, err)
	}
	if len(consumers) > 0 {
		return apperrors.BadRequest(
			"PVC_CLONE_SOURCE_IN_USE",
			fmt.Sprintf(
				"source PVC %s/%s is currently mounted by active pods: %s",
				sourceNamespace,
				sourcePVC,
				formatObjectRefs(consumers, 3),
			),
		)
	}

	if sourceNamespace != targetNamespace {
		allowed, reason, err := validator.CanClonePVCSource(ctx, cluster, sourceNamespace)
		if err != nil {
			return fmt.Errorf("check clone source RBAC in namespace %s on cluster %s: %w", sourceNamespace, cluster, err)
		}
		if !allowed {
			msg := fmt.Sprintf(
				"selected cluster credential cannot create datavolumes/source in namespace %s",
				sourceNamespace,
			)
			if reason != "" {
				msg += ": " + reason
			}
			return apperrors.BadRequest("PVC_CLONE_SOURCE_ACCESS_DENIED", msg)
		}
	}

	return nil
}

// GetPVCCloneAdvisory returns a non-blocking advisory for CDI PVC clone
// requests. It only reports conditions that are known to trigger a slower
// host-assisted fallback, never a hard rejection.
func (s *VMService) GetPVCCloneAdvisory(
	ctx context.Context,
	cluster, targetNamespace, sourceNamespace, sourcePVC, targetStorageClass string,
) (*PlacementAdvisory, error) {
	if s == nil || s.infra == nil {
		return nil, nil
	}
	validator, ok := s.infra.(provider.PVCClonePreflightProvider)
	if !ok {
		return nil, nil
	}

	cluster = strings.TrimSpace(cluster)
	targetNamespace = strings.TrimSpace(targetNamespace)
	sourceNamespace = strings.TrimSpace(sourceNamespace)
	sourcePVC = strings.TrimSpace(sourcePVC)
	targetStorageClass = strings.TrimSpace(targetStorageClass)

	if cluster == "" || sourcePVC == "" || targetStorageClass == "" {
		return nil, nil
	}
	if sourceNamespace == "" {
		sourceNamespace = targetNamespace
	}

	sourceClaim, err := validator.GetPersistentVolumeClaim(ctx, cluster, sourceNamespace, sourcePVC)
	if err != nil || sourceClaim == nil {
		return nil, err
	}
	sourceStorageClass := strings.TrimSpace(sourceClaim.StorageClassName)
	if sourceStorageClass == "" || sourceStorageClass == targetStorageClass {
		return s.cloneStorageProfileAdvisory(ctx, validator, cluster, sourceClaim, targetStorageClass)
	}

	return &PlacementAdvisory{
		Code: pvcCloneHostAssistedFallbackLikelyCode,
		Message: fmt.Sprintf(
			"source PVC %s/%s uses storage class %q while the clone target uses %q; CDI efficient clone prerequisites are not met and the operation may fall back to host-assisted copy",
			sourceNamespace,
			sourcePVC,
			sourceStorageClass,
			targetStorageClass,
		),
	}, nil
}

func (s *VMService) cloneStorageProfileAdvisory(
	ctx context.Context,
	validator provider.PVCClonePreflightProvider,
	cluster string,
	sourceClaim *domain.PersistentVolumeClaim,
	targetStorageClass string,
) (*PlacementAdvisory, error) {
	storageProfile, _ := validator.GetStorageProfile(ctx, cluster, targetStorageClass)
	if storageProfile == nil {
		return nil, nil
	}

	if strings.EqualFold(strings.TrimSpace(storageProfile.CloneStrategy), cloneStrategyHostAssisted) {
		return &PlacementAdvisory{
			Code: pvcCloneHostAssistedFallbackLikelyCode,
			Message: fmt.Sprintf(
				"target storage class %q prefers CDI clone strategy %q in StorageProfile; the clone will likely use host-assisted copy instead of an efficient clone path",
				targetStorageClass,
				storageProfile.CloneStrategy,
			),
		}, nil
	}

	sourceVolumeMode := normalizeVolumeMode(sourceClaim.VolumeMode)
	targetVolumeMode := normalizeVolumeMode(storageProfile.DefaultVolumeMode)
	if sourceVolumeMode == volumeModeBlock && targetVolumeMode != "" && targetVolumeMode != sourceVolumeMode {
		return &PlacementAdvisory{
			Code: pvcCloneHostAssistedFallbackLikelyCode,
			Message: fmt.Sprintf(
				"source PVC %s/%s uses volume mode %q while target storage profile %q defaults to %q; CDI efficient clone prerequisites are not met and the operation may fall back to host-assisted copy",
				sourceClaim.Namespace,
				sourceClaim.Name,
				sourceVolumeMode,
				targetStorageClass,
				targetVolumeMode,
			),
		}, nil
	}

	return nil, nil
}

func validateCloneTargetSize(
	ctx context.Context,
	validator provider.PVCClonePreflightProvider,
	cluster string,
	sourceClaim *domain.PersistentVolumeClaim,
	targetDiskGB int,
	targetStorageClass string,
) error {
	if sourceClaim == nil || targetDiskGB <= 0 {
		return nil
	}
	sourceBytes := maxInt64(sourceClaim.CapacityBytes, sourceClaim.RequestedStorageBytes)
	if sourceBytes <= 0 {
		return nil
	}

	targetBytes := int64(targetDiskGB) * 1024 * 1024 * 1024
	if targetBytes >= sourceBytes {
		return validateCloneTargetStorageClass(ctx, validator, cluster, sourceClaim, targetBytes > sourceBytes, targetDiskGB, targetStorageClass, sourceBytes)
	}

	return apperrors.BadRequest(
		"PVC_CLONE_TARGET_TOO_SMALL",
		fmt.Sprintf(
			"target root disk size %dGi is smaller than source PVC %s/%s size %s; clone targets must be equal or larger",
			targetDiskGB,
			sourceClaim.Namespace,
			sourceClaim.Name,
			formatBytesAsGiString(sourceBytes),
		),
	)
}

func validateCloneTargetStorageClass(
	ctx context.Context,
	validator provider.PVCClonePreflightProvider,
	cluster string,
	sourceClaim *domain.PersistentVolumeClaim,
	requiresExpansion bool,
	targetDiskGB int,
	targetStorageClass string,
	sourceBytes int64,
) error {
	storageClassName := strings.TrimSpace(targetStorageClass)
	if storageClassName == "" {
		return nil
	}

	storageClass, err := validator.GetStorageClass(ctx, cluster, storageClassName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apperrors.BadRequest(
				"PVC_CLONE_TARGET_STORAGE_CLASS_NOT_FOUND",
				fmt.Sprintf("target storage class %q was not found on selected cluster", storageClassName),
			)
		}
		return fmt.Errorf("get storage class %s on cluster %s: %w", storageClassName, cluster, err)
	}
	if !requiresExpansion || storageClass.AllowVolumeExpansion {
		return nil
	}

	return apperrors.BadRequest(
		"PVC_CLONE_TARGET_EXPANSION_UNSUPPORTED",
		fmt.Sprintf(
			"target root disk size %dGi exceeds source PVC %s/%s size %s, but storage class %q does not allow volume expansion",
			targetDiskGB,
			sourceClaim.Namespace,
			sourceClaim.Name,
			formatBytesAsGiString(sourceBytes),
			storageClass.Name,
		),
	)
}

func formatBytesAsGiString(bytes int64) string {
	if bytes <= 0 {
		return "0Gi"
	}
	gi := float64(bytes) / float64(1024*1024*1024)
	if math.Abs(gi-math.Round(gi)) < 1e-9 {
		return fmt.Sprintf("%dGi", int(math.Round(gi)))
	}
	return fmt.Sprintf("%.1fGi", gi)
}

func normalizeVolumeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "block":
		return volumeModeBlock
	case "filesystem":
		return "Filesystem"
	default:
		return strings.TrimSpace(value)
	}
}

func maxInt64(left, right int64) int64 {
	if left >= right {
		return left
	}
	return right
}

func formatObjectRefs(items []domain.ObjectReference, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		ref := items[i]
		if ref.Namespace != "" {
			parts = append(parts, ref.Namespace+"/"+ref.Name)
			continue
		}
		parts = append(parts, ref.Name)
	}
	if len(items) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(items)-limit))
	}
	return strings.Join(parts, ", ")
}
