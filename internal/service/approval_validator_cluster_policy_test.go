package service

import (
	"testing"

	"github.com/stretchr/testify/require"

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
		SetMemoryGi(4).
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
