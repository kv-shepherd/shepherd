package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

func TestValidateOvercommit_DedicatedCPUConflict(t *testing.T) {
	err := ValidateOvercommit(8, 4, 32768, 32768, true)
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "DEDICATED_CPU_OVERCOMMIT_CONFLICT", appErr.Code)
}

func TestValidateOvercommit_ValidGuaranteedQoS(t *testing.T) {
	err := ValidateOvercommit(8, 8, 32768, 32768, true)
	require.NoError(t, err)
}

func TestExtractRequiredCapabilities_FromFlags(t *testing.T) {
	size := &ent.InstanceSize{
		RequiresGpu:       true,
		RequiresSriov:     true,
		RequiresHugepages: true,
		HugepagesSize:     "2Mi",
	}

	caps := ExtractRequiredCapabilities(size)
	require.ElementsMatch(t, []string{"gpu", "sriov", "hugepages", "hugepages:2mi"}, caps)
}

func TestExtractRequiredCapabilities_FromSpecOverrides(t *testing.T) {
	size := &ent.InstanceSize{
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.gpus": []interface{}{
				map[string]interface{}{"name": "gpu1", "deviceName": "nvidia.com/GA102GL_A10"},
			},
			"spec.template.spec.domain.memory.hugepages.pageSize": "1Gi",
			"spec.template.spec.domain.devices.interfaces": []interface{}{
				map[string]interface{}{"name": "sriov-net-1"},
			},
		},
	}

	caps := ExtractRequiredCapabilities(size)
	require.ElementsMatch(t, []string{"gpu", "sriov", "hugepages", "hugepages:1gi"}, caps)
}

func TestMissingCapabilities(t *testing.T) {
	clusterCaps := buildClusterCapabilitySet([]string{
		"nvidia.com/GA102GL_A10",
		"sriov",
		"hugepages-2Mi",
	})

	require.Empty(t, MissingCapabilities([]string{"gpu", "sriov", "hugepages:2mi"}, clusterCaps))
	require.Equal(t, []string{"hugepages:1gi"}, MissingCapabilities([]string{"hugepages:1gi"}, clusterCaps))
}

func TestValidateNamespaceClusterEnvironment(t *testing.T) {
	testCases := []struct {
		name         string
		namespaceEnv string
		clusterEnv   string
		wantErrCode  string
	}{
		{
			name:         "matching environment passes",
			namespaceEnv: "prod",
			clusterEnv:   "prod",
		},
		{
			name:         "mismatch blocked",
			namespaceEnv: "test",
			clusterEnv:   "prod",
			wantErrCode:  "NAMESPACE_CLUSTER_ENV_MISMATCH",
		},
		{
			name:         "empty value blocked",
			namespaceEnv: "",
			clusterEnv:   "prod",
			wantErrCode:  apperrors.CodeValidationFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNamespaceClusterEnvironment(tc.namespaceEnv, tc.clusterEnv)
			if tc.wantErrCode == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			appErr, ok := apperrors.IsAppError(err)
			require.True(t, ok)
			require.Equal(t, tc.wantErrCode, appErr.Code)
		})
	}
}
