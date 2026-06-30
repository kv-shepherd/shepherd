package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestApprovalValidator_RejectsClusterWithoutPolicy(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_validator_missing_cluster_policy")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cluster-1").
		SetName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(entnamespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	validator := NewApprovalValidator(client)
	err = validator.ValidateApproval(ctx, ApprovalValidationInput{
		ClusterID: "cluster-1",
		Namespace: "prod-a",
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_NOT_CONFIGURED", appErr.Code)
}

func TestApprovalValidator_RejectsDisallowedStorageClassByPolicy(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_validator_storage_policy")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cluster-1").
		SetName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetDefaultStorageClass("fast-sc").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cluster-1").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"gold-sc"}).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(entnamespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Template.Create().
		SetID("tpl-1").
		SetName("fedora").
		SetSourceType(TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.InstanceSize.Create().
		SetID("sz-1").
		SetName("small").
		SetCPUCores(2).
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	validator := NewApprovalValidator(client)
	err = validator.ValidateApproval(ctx, ApprovalValidationInput{
		ClusterID:      "cluster-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "sz-1",
		Namespace:      "prod-a",
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED", appErr.Code)
}

func TestApprovalValidator_RejectsDisabledClusterEvenIfStatusIsHealthy(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_validator_disabled_cluster")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cluster-1").
		SetName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(false).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	validator := NewApprovalValidator(client)
	err = validator.ValidateApproval(ctx, ApprovalValidationInput{
		ClusterID: "cluster-1",
	})
	require.Error(t, err)

	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeValidationFailed, appErr.Code)
	require.Contains(t, appErr.Message, "disabled")
}

func TestApprovalValidator_EvaluateClusterCompatibilityReturnsPerClusterVerdicts(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_validator_cluster_compat")
	ctx := t.Context()
	fixture := seedApprovalCompatibilityFixture(t, client)
	validator := NewApprovalValidator(client)

	results, err := validator.EvaluateClusterCompatibility(ctx, []*ent.Cluster{
		fixture.clusters["eligible"],
		nil,
		fixture.clusters["missing-policy"],
		fixture.clusters["missing-gpu"],
		fixture.clusters["env-mismatch"],
		fixture.clusters["disabled"],
	}, fixture.input)
	require.NoError(t, err)
	require.Len(t, results, 5)

	byID := make(map[string]ClusterCompatibilityResult, len(results))
	for _, result := range results {
		if result.Cluster != nil {
			byID[result.Cluster.ID] = result
		}
	}

	eligible := byID[fixture.clusters["eligible"].ID]
	require.True(t, eligible.Eligible)
	require.Empty(t, eligible.ReasonCode)

	missingPolicy := byID[fixture.clusters["missing-policy"].ID]
	require.False(t, missingPolicy.Eligible)
	require.Equal(t, "CLUSTER_POLICY_NOT_CONFIGURED", missingPolicy.ReasonCode)

	missingGPU := byID[fixture.clusters["missing-gpu"].ID]
	require.False(t, missingGPU.Eligible)
	require.Equal(t, apperrors.CodeValidationFailed, missingGPU.ReasonCode)
	require.Contains(t, missingGPU.ReasonMessage, "missing required capabilities")
	require.Contains(t, missingGPU.ReasonMessage, "gpu")

	envMismatch := byID[fixture.clusters["env-mismatch"].ID]
	require.False(t, envMismatch.Eligible)
	require.Equal(t, "NAMESPACE_CLUSTER_ENV_MISMATCH", envMismatch.ReasonCode)

	disabled := byID[fixture.clusters["disabled"].ID]
	require.False(t, disabled.Eligible)
	require.Equal(t, apperrors.CodeValidationFailed, disabled.ReasonCode)
	require.Contains(t, disabled.ReasonMessage, "disabled")

	filtered, err := validator.FilterCompatibleClusters(ctx, []*ent.Cluster{
		fixture.clusters["eligible"],
		fixture.clusters["missing-policy"],
		fixture.clusters["missing-gpu"],
		fixture.clusters["env-mismatch"],
		fixture.clusters["disabled"],
	}, fixture.input)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, fixture.clusters["eligible"].ID, filtered[0].ID)
}

func TestApprovalValidator_EvaluateClusterPlacementSelectedClusterVerdict(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_validator_cluster_placement")
	ctx := t.Context()
	fixture := seedApprovalCompatibilityFixture(t, client)
	validator := NewApprovalValidator(client)

	eligible, err := validator.EvaluateClusterPlacement(ctx, fixture.input)
	require.NoError(t, err)
	require.NotNil(t, eligible)
	require.True(t, eligible.Eligible)
	require.Equal(t, fixture.clusters["eligible"].ID, eligible.Cluster.ID)

	disabledInput := fixture.input
	disabledInput.ClusterID = fixture.clusters["disabled"].ID
	disabled, err := validator.EvaluateClusterPlacement(ctx, disabledInput)
	require.NoError(t, err)
	require.NotNil(t, disabled)
	require.False(t, disabled.Eligible)
	require.Equal(t, apperrors.CodeValidationFailed, disabled.ReasonCode)
	require.Contains(t, disabled.ReasonMessage, "disabled")

	missingInput := fixture.input
	missingInput.ClusterID = "missing-cluster"
	missing, err := validator.EvaluateClusterPlacement(ctx, missingInput)
	require.NoError(t, err)
	require.NotNil(t, missing)
	require.False(t, missing.Eligible)
	require.Equal(t, apperrors.CodeValidationFailed, missing.ReasonCode)
	require.Contains(t, missing.ReasonMessage, "not found")
}

type approvalCompatibilityFixture struct {
	input    ApprovalValidationInput
	clusters map[string]*ent.Cluster
}

func seedApprovalCompatibilityFixture(t *testing.T, client *ent.Client) approvalCompatibilityFixture {
	t.Helper()

	ctx := t.Context()
	_, err := client.NamespaceRegistry.Create().
		SetID("ns-prod").
		SetName("prod-a").
		SetEnvironment(entnamespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Template.Create().
		SetID("tpl-compat").
		SetName("fedora-compat").
		SetSourceType(TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.InstanceSize.Create().
		SetID("size-gpu").
		SetName("gpu-small").
		SetCPUCores(2).
		SetCPURequest(2).
		SetMemoryGi(4).
		SetMemoryRequestGi(4).
		SetRequiresGpu(true).
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	clusters := map[string]*ent.Cluster{
		"eligible":       seedApprovalCompatibilityCluster(t, client, "cluster-eligible", entcluster.EnvironmentProd, true, []string{"nvidia.com/gpu"}, true),
		"missing-policy": seedApprovalCompatibilityCluster(t, client, "cluster-missing-policy", entcluster.EnvironmentProd, true, []string{"gpu"}, false),
		"missing-gpu":    seedApprovalCompatibilityCluster(t, client, "cluster-missing-gpu", entcluster.EnvironmentProd, true, nil, true),
		"env-mismatch":   seedApprovalCompatibilityCluster(t, client, "cluster-env-mismatch", entcluster.EnvironmentTest, true, []string{"gpu"}, true),
		"disabled":       seedApprovalCompatibilityCluster(t, client, "cluster-disabled", entcluster.EnvironmentProd, false, []string{"gpu"}, true),
	}

	return approvalCompatibilityFixture{
		input: ApprovalValidationInput{
			ClusterID:      clusters["eligible"].ID,
			TemplateID:     "tpl-compat",
			InstanceSizeID: "size-gpu",
			Namespace:      "prod-a",
		},
		clusters: clusters,
	}
}

func seedApprovalCompatibilityCluster(
	t *testing.T,
	client *ent.Client,
	id string,
	environment entcluster.Environment,
	enabled bool,
	features []string,
	withPolicy bool,
) *ent.Cluster {
	t.Helper()

	clusterRow, err := client.Cluster.Create().
		SetID(id).
		SetName(id).
		SetAPIServerURL("https://" + id + ".invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(environment).
		SetEnabled(enabled).
		SetDefaultStorageClass("fast-sc").
		SetStorageClasses([]string{"fast-sc"}).
		SetEnabledFeatures(features).
		SetCreatedBy("seed").
		Save(t.Context())
	require.NoError(t, err)

	if withPolicy {
		_, err = client.ClusterPolicy.Create().
			SetID("policy-" + id).
			SetClusterID(id).
			SetAllowCPUOvercommit(true).
			SetAllowMemoryOvercommit(true).
			SetAllowDedicatedCPU(true).
			SetAllowGpu(true).
			SetAllowSriov(true).
			SetAllowHugepages(true).
			SetAllowCdiClone(true).
			SetAllowedStorageClasses([]string{"fast-sc"}).
			SetCreatedBy("seed").
			Save(t.Context())
		require.NoError(t, err)
	}
	return clusterRow
}
