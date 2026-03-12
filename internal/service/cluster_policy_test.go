package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

func TestClusterPolicyService_ValidateCreatePlacement_RejectsMissingPolicy(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster: &ent.Cluster{Name: "cluster-a"},
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_NOT_CONFIGURED", appErr.Code)
}

func TestClusterPolicyService_ValidateCreatePlacement_RejectsCPUOvercommit(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster:      &ent.Cluster{Name: "cluster-a"},
		Policy:       &ent.ClusterPolicy{AllowCPUOvercommit: false},
		InstanceSize: &ent.InstanceSize{},
		CPUCores:     4,
		CPURequest:   2,
		MemoryGi:     8,
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_DENIED", appErr.Code)
}

func TestClusterPolicyService_ValidateCreatePlacement_RejectsCloneNamespaceOutsideAllowlist(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster: &ent.Cluster{Name: "cluster-a"},
		Policy: &ent.ClusterPolicy{
			AllowCdiClone:                true,
			AllowedCloneSourceNamespaces: []string{"golden-images"},
		},
		Template: &ent.Template{
			SourceType:   TemplateSourceCDIPVCClone,
			PvcName:      "fedora-golden",
			PvcNamespace: "other-images",
		},
		InstanceSize: &ent.InstanceSize{},
		CPUCores:     2,
		MemoryGi:     4,
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_DENIED", appErr.Code)
}

func TestClusterPolicyService_ValidateCreatePlacement_UsesClusterDefaultStorageClass(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster: &ent.Cluster{
			Name:                "cluster-a",
			DefaultStorageClass: "fast-sc",
		},
		Policy: &ent.ClusterPolicy{
			AllowCdiClone:         true,
			AllowedStorageClasses: []string{"fast-sc"},
		},
		Template: &ent.Template{
			SourceType: TemplateSourceCDIImageImport,
			ImageURL:   "docker://quay.io/containerdisks/fedora:40",
		},
		InstanceSize: &ent.InstanceSize{},
		CPUCores:     2,
		MemoryGi:     4,
	})
	require.NoError(t, err)
}

func TestClusterPolicyService_ValidateCreatePlacement_RequiresExplicitStorageClass(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster: &ent.Cluster{
			Name: "cluster-a",
		},
		Policy: &ent.ClusterPolicy{
			AllowCdiClone:         true,
			AllowedStorageClasses: []string{"fast-sc"},
		},
		Template: &ent.Template{
			SourceType: TemplateSourceCDIImageImport,
			ImageURL:   "docker://quay.io/containerdisks/fedora:40",
		},
		InstanceSize: &ent.InstanceSize{},
		CPUCores:     2,
		MemoryGi:     4,
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED", appErr.Code)
}

func TestClusterPolicyService_ValidateCreatePlacement_RequiresExplicitStorageClassWhenDefaultIsOutsideAllowlist(t *testing.T) {
	t.Parallel()

	svc := &ClusterPolicyService{}
	err := svc.ValidateCreatePlacement(ClusterPolicyValidationInput{
		Cluster: &ent.Cluster{
			Name:                "cluster-a",
			DefaultStorageClass: "slow-sc",
		},
		Policy: &ent.ClusterPolicy{
			AllowCdiClone:         true,
			AllowedStorageClasses: []string{"fast-sc"},
		},
		Template: &ent.Template{
			SourceType: TemplateSourceCDIImageImport,
			ImageURL:   "docker://quay.io/containerdisks/fedora:40",
		},
		InstanceSize: &ent.InstanceSize{},
		CPUCores:     2,
		MemoryGi:     4,
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED", appErr.Code)
}
