package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/domain"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestResolveRootVolumeProvisioning_AllowsExplicitApprovalModeWithoutClaimPropertySets(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name:              "block-sc",
		DefaultVolumeMode: "Filesystem",
	}})

	validator := (&ApprovalValidator{}).SetVMService(NewVMService(mock))
	resolution, err := validator.resolveRootVolumeProvisioning(
		context.Background(),
		&ent.Cluster{ID: "cl-1", Name: "cluster-a"},
		nil,
		&ent.Template{SourceType: TemplateSourceCDIImageImport},
		&ent.InstanceSize{Name: "small"},
		ApprovalValidationInput{
			StorageClass:  "block-sc",
			DVAccessModes: []string{"ReadWriteOnce"},
			DVVolumeMode:  "Filesystem",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, rootVolumeResolutionResolved, resolution.State)
	require.Equal(t, []string{"ReadWriteOnce"}, resolution.EffectiveAccessModes)
	require.Equal(t, "Filesystem", resolution.EffectiveVolumeMode)
	require.Contains(t, resolution.Message, "does not expose claimPropertySets")
	require.Contains(t, resolution.Message, "ReadWriteOnce")
}

func TestResolveRootVolumeProvisioning_AllowsPersistedExplicitModeWithoutClaimPropertySets(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name:              "block-sc",
		DefaultVolumeMode: "Filesystem",
	}})

	validator := (&ApprovalValidator{}).SetVMService(NewVMService(mock))
	resolution, err := validator.resolveRootVolumeProvisioning(
		context.Background(),
		&ent.Cluster{ID: "cl-1", Name: "cluster-a"},
		nil,
		&ent.Template{SourceType: TemplateSourceCDIImageImport},
		&ent.InstanceSize{
			Name:          "small",
			DvAccessModes: []string{"ReadWriteOnce"},
			DvVolumeMode:  "Filesystem",
		},
		ApprovalValidationInput{StorageClass: "block-sc"},
	)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, rootVolumeResolutionResolved, resolution.State)
	require.Equal(t, rootVolumeIntentExplicit, resolution.IntentMode)
	require.Equal(t, []string{"ReadWriteOnce"}, resolution.EffectiveAccessModes)
	require.Equal(t, "Filesystem", resolution.EffectiveVolumeMode)
}

func TestResolveRootVolumeProvisioning_StillBlocksAutoModeWithoutClaimPropertySets(t *testing.T) {
	t.Parallel()

	mock := provider.NewMockProvider()
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name:              "block-sc",
		DefaultVolumeMode: "Filesystem",
	}})

	validator := (&ApprovalValidator{}).SetVMService(NewVMService(mock))
	resolution, err := validator.resolveRootVolumeProvisioning(
		context.Background(),
		&ent.Cluster{ID: "cl-1", Name: "cluster-a"},
		nil,
		&ent.Template{SourceType: TemplateSourceCDIImageImport},
		&ent.InstanceSize{Name: "small"},
		ApprovalValidationInput{StorageClass: "block-sc"},
	)
	require.Error(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, rootVolumeResolutionProfileIncomplete, resolution.State)
	require.True(t, strings.Contains(resolution.Message, "claimPropertySets"))

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, rootVolumeReasonProfileIncomplete, appErr.Code)
}
