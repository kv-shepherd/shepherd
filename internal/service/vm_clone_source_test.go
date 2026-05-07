package service

import (
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestValidatePVCCloneSource_SourceNotFound(t *testing.T) {
	t.Parallel()

	svc := NewVMService(provider.NewMockProvider())
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 40, "")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want source not found")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_SOURCE_NOT_FOUND" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_SOURCE_NOT_FOUND")
	}
}

func TestValidatePVCCloneSource_SourceInUse(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:      "ubuntu-golden",
		Namespace: "golden-images",
		Phase:     "Bound",
	}})
	mock.SeedPVCConsumers("golden-images", "ubuntu-golden", []domain.ObjectReference{
		{Kind: "Pod", Namespace: "golden-images", Name: "virt-launcher-a"},
		{Kind: "Pod", Namespace: "golden-images", Name: "virt-launcher-b"},
	})

	svc := NewVMService(mock)
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 40, "")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want in-use rejection")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_SOURCE_IN_USE" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_SOURCE_IN_USE")
	}
}

func TestValidatePVCCloneSource_CrossNamespaceAccessDenied(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:      "ubuntu-golden",
		Namespace: "golden-images",
		Phase:     "Bound",
	}})
	mock.SetCloneSourceAccess("golden-images", false, "RBAC: missing create on datavolumes/source")

	svc := NewVMService(mock)
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 40, "")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want access denied")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_SOURCE_ACCESS_DENIED" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_SOURCE_ACCESS_DENIED")
	}
}

func TestValidatePVCCloneSource_SameNamespaceSkipsCloneSourceRBAC(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:      "ubuntu-golden",
		Namespace: "team-a",
		Phase:     "Bound",
	}})
	mock.SetCloneSourceAccess("team-a", false, "should not be consulted for same-namespace clone")

	svc := NewVMService(mock)
	if err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "", "ubuntu-golden", 40, ""); err != nil {
		t.Fatalf("ValidatePVCCloneSource error = %v, want nil for same-namespace clone", err)
	}
}

func TestValidatePVCCloneSource_TargetTooSmall(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 40 * 1024 * 1024 * 1024,
	}})

	svc := NewVMService(mock)
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 20, "")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want target-too-small rejection")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_TOO_SMALL" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_TOO_SMALL")
	}
}

func TestValidatePVCCloneSource_ExpansionRequiresExpandableStorageClass(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 20 * 1024 * 1024 * 1024,
	}})
	mock.SeedStorageClasses([]*domain.StorageClass{{
		Name:                 "slow-sc",
		AllowVolumeExpansion: false,
	}})

	svc := NewVMService(mock)
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 40, "slow-sc")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want expansion unsupported rejection")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_EXPANSION_UNSUPPORTED" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_EXPANSION_UNSUPPORTED")
	}
}

func TestValidatePVCCloneSource_TargetStorageClassMustExist(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 20 * 1024 * 1024 * 1024,
	}})

	svc := NewVMService(mock)
	err := svc.ValidatePVCCloneSource(t.Context(), "cluster-a", "team-a", "golden-images", "ubuntu-golden", 20, "missing-sc")
	if err == nil {
		t.Fatal("ValidatePVCCloneSource error = nil, want missing storage class rejection")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("ValidatePVCCloneSource error = %T, want AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_STORAGE_CLASS_NOT_FOUND" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_STORAGE_CLASS_NOT_FOUND")
	}
}

func TestGetPVCCloneAdvisory_StorageClassMismatchWarnsHostAssistedFallback(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:             "ubuntu-golden",
		Namespace:        "golden-images",
		Phase:            "Bound",
		StorageClassName: "source-sc",
	}})

	svc := NewVMService(mock)
	advisory, err := svc.GetPVCCloneAdvisory(
		t.Context(),
		"cluster-a",
		"team-a",
		"golden-images",
		"ubuntu-golden",
		"target-sc",
	)
	if err != nil {
		t.Fatalf("GetPVCCloneAdvisory error = %v, want nil", err)
	}
	if advisory == nil {
		t.Fatal("GetPVCCloneAdvisory advisory = nil, want non-nil")
		return
	}
	if advisory.Code != pvcCloneHostAssistedFallbackLikelyCode {
		t.Fatalf("advisory.Code = %q, want %q", advisory.Code, pvcCloneHostAssistedFallbackLikelyCode)
	}
	if advisory.Message == "" {
		t.Fatal("advisory.Message is empty")
	}
}

func TestGetPVCCloneAdvisory_StorageProfileCopyCloneStrategyWarnsHostAssistedFallback(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:             "ubuntu-golden",
		Namespace:        "golden-images",
		Phase:            "Bound",
		StorageClassName: "target-sc",
	}})
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name:          "target-sc",
		CloneStrategy: "copy",
	}})

	svc := NewVMService(mock)
	advisory, err := svc.GetPVCCloneAdvisory(
		t.Context(),
		"cluster-a",
		"team-a",
		"golden-images",
		"ubuntu-golden",
		"target-sc",
	)
	if err != nil {
		t.Fatalf("GetPVCCloneAdvisory error = %v, want nil", err)
	}
	if advisory == nil {
		t.Fatal("GetPVCCloneAdvisory advisory = nil, want non-nil")
		return
	}
	if advisory.Code != pvcCloneHostAssistedFallbackLikelyCode {
		t.Fatalf("advisory.Code = %q, want %q", advisory.Code, pvcCloneHostAssistedFallbackLikelyCode)
	}
	if !strings.Contains(advisory.Message, "clone strategy") {
		t.Fatalf("advisory.Message = %q, want clone strategy hint", advisory.Message)
	}
}

func TestGetPVCCloneAdvisory_BlockVolumeModeMismatchWarnsHostAssistedFallback(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:             "ubuntu-golden",
		Namespace:        "golden-images",
		Phase:            "Bound",
		StorageClassName: "target-sc",
		VolumeMode:       "Block",
	}})
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name:              "target-sc",
		DefaultVolumeMode: "Filesystem",
	}})

	svc := NewVMService(mock)
	advisory, err := svc.GetPVCCloneAdvisory(
		t.Context(),
		"cluster-a",
		"team-a",
		"golden-images",
		"ubuntu-golden",
		"target-sc",
	)
	if err != nil {
		t.Fatalf("GetPVCCloneAdvisory error = %v, want nil", err)
	}
	if advisory == nil {
		t.Fatal("GetPVCCloneAdvisory advisory = nil, want non-nil")
		return
	}
	if advisory.Code != pvcCloneHostAssistedFallbackLikelyCode {
		t.Fatalf("advisory.Code = %q, want %q", advisory.Code, pvcCloneHostAssistedFallbackLikelyCode)
	}
	if !strings.Contains(advisory.Message, "volume mode") {
		t.Fatalf("advisory.Message = %q, want volume mode hint", advisory.Message)
	}
}
