package handlers

import (
	"net/http"
	"testing"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entclusterpolicy "kv-shepherd.io/shepherd/ent/clusterpolicy"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

func TestListClusters_RequiresFilterPaginationUsesFilteredTotals(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mustCreateCluster := func(id, name string, features []string) {
		t.Helper()
		_, err := client.Cluster.Create().
			SetID(id).
			SetName(name).
			SetDisplayName(name).
			SetAPIServerURL("https://cluster.invalid").
			SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
			SetStatus(entcluster.StatusHEALTHY).
			SetCreatedBy("test").
			SetEnabledFeatures(features).
			Save(ctx)
		if err != nil {
			t.Fatalf("create cluster %s: %v", id, err)
		}
	}

	mustCreateCluster("cl-1", "cluster-a", []string{"LiveMigration", "Snapshot"})
	mustCreateCluster("cl-2", "cluster-b", []string{"LiveMigration"})
	mustCreateCluster("cl-3", "cluster-c", []string{"GPU"})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=1&requires=LiveMigration,Snapshot",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:     1,
		PerPage:  1,
		Requires: "LiveMigration,Snapshot",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Name != "cluster-a" {
		t.Fatalf("items[0].name = %q, want cluster-a", resp.Items[0].Name)
	}
	if got := resp.Pagination.Total; got != 1 {
		t.Fatalf("pagination.total = %d, want 1 (filtered total)", got)
	}
	if got := resp.Pagination.TotalPages; got != 1 {
		t.Fatalf("pagination.total_pages = %d, want 1", got)
	}
}

func TestListClusters_RequiresFilterSecondPageKeepsFilteredTotals(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		SetEnabledFeatures([]string{"LiveMigration", "Snapshot"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=2&per_page=1&requires=LiveMigration,Snapshot",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:     2,
		PerPage:  1,
		Requires: "LiveMigration,Snapshot",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters page2 status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 0 {
		t.Fatalf("items len page2 = %d, want 0", got)
	}
	if got := resp.Pagination.Total; got != 1 {
		t.Fatalf("pagination.total page2 = %d, want 1 (filtered total)", got)
	}
	if got := resp.Pagination.TotalPages; got != 1 {
		t.Fatalf("pagination.total_pages page2 = %d, want 1", got)
	}
}

func TestListClusters_CreateCompatibilityFilterReturnsOnlyCompatibleTargets(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-1").
		SetName("fedora").
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("sz-1").
		SetName("gpu-small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetRequiresGpu(true).
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	mustCreateCluster := func(id, name string, env entcluster.Environment, features []string, defaultStorageClass string) {
		t.Helper()
		_, createErr := client.Cluster.Create().
			SetID(id).
			SetName(name).
			SetDisplayName(name).
			SetAPIServerURL("https://cluster.invalid").
			SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
			SetStatus(entcluster.StatusHEALTHY).
			SetEnvironment(env).
			SetDefaultStorageClass(defaultStorageClass).
			SetEnabledFeatures(features).
			SetCreatedBy("test").
			Save(ctx)
		if createErr != nil {
			t.Fatalf("create cluster %s: %v", id, createErr)
		}
	}
	mustCreatePolicy := func(id, clusterID string, allowedStorageClasses []string) {
		t.Helper()
		_, createErr := client.ClusterPolicy.Create().
			SetID(id).
			SetClusterID(clusterID).
			SetAllowCPUOvercommit(true).
			SetAllowMemoryOvercommit(true).
			SetAllowDedicatedCPU(true).
			SetAllowGpu(true).
			SetAllowSriov(true).
			SetAllowHugepages(true).
			SetAllowCdiClone(true).
			SetAllowedStorageClasses(allowedStorageClasses).
			SetCreatedBy("test").
			Save(ctx)
		if createErr != nil {
			t.Fatalf("create cluster policy %s: %v", id, createErr)
		}
	}

	mustCreateCluster("cl-1", "cluster-a", entcluster.EnvironmentProd, []string{"GPU"}, "gold-sc")
	mustCreatePolicy("policy-1", "cl-1", []string{"gold-sc"})

	mustCreateCluster("cl-2", "cluster-b", entcluster.EnvironmentProd, []string{"LiveMigration"}, "gold-sc")
	mustCreatePolicy("policy-2", "cl-2", []string{"gold-sc"})

	mustCreateCluster("cl-3", "cluster-c", entcluster.EnvironmentProd, []string{"GPU"}, "silver-sc")
	mustCreatePolicy("policy-3", "cl-3", []string{"silver-sc"})

	mustCreateCluster("cl-4", "cluster-d", entcluster.EnvironmentTest, []string{"GPU"}, "gold-sc")
	mustCreatePolicy("policy-4", "cl-4", []string{"gold-sc"})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20&namespace=prod-a&template_id=tpl-1&instance_size_id=sz-1&selected_storage_class=gold-sc",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:                 1,
		PerPage:              20,
		Namespace:            "prod-a",
		TemplateId:           "tpl-1",
		InstanceSizeId:       "sz-1",
		SelectedStorageClass: "gold-sc",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters compatibility status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Id != "cl-1" {
		t.Fatalf("cluster id = %q, want cl-1", resp.Items[0].Id)
	}
	if got := resp.Pagination.Total; got != 1 {
		t.Fatalf("pagination.total = %d, want 1", got)
	}
}

func TestListClusters_CreateCompatibilityFilterCanIncludeIncompatibleWithReasons(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-1").
		SetName("fedora").
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("sz-1").
		SetName("small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetDefaultStorageClass("gold-sc").
		SetEnabledFeatures([]string{"LiveMigration"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster cl-1: %v", err)
	}
	_, err = client.Cluster.Create().
		SetID("cl-2").
		SetName("cluster-b").
		SetDisplayName("cluster-b").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetDefaultStorageClass("fast-sc").
		SetEnabledFeatures([]string{"LiveMigration"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster cl-2: %v", err)
	}

	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cl-1").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"gold-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy policy-1: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-2").
		SetClusterID("cl-2").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"gold-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy policy-2: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20&namespace=prod-a&template_id=tpl-1&instance_size_id=sz-1&include_incompatible=true",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:                1,
		PerPage:             20,
		Namespace:           "prod-a",
		TemplateId:          "tpl-1",
		InstanceSizeId:      "sz-1",
		IncludeIncompatible: true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters include incompatible status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 2 {
		t.Fatalf("items len = %d, want 2", got)
	}
	itemsByID := make(map[string]generated.Cluster, len(resp.Items))
	for _, item := range resp.Items {
		itemsByID[item.Id] = item
	}
	if !itemsByID["cl-1"].Compatibility.Eligible {
		t.Fatalf("cluster-a compatibility = %#v, want eligible", itemsByID["cl-1"].Compatibility)
	}
	if itemsByID["cl-2"].Compatibility.Eligible {
		t.Fatalf("cluster-b compatibility = %#v, want incompatible", itemsByID["cl-2"].Compatibility)
	}
	if itemsByID["cl-2"].Compatibility.ReasonCode != "CLUSTER_POLICY_DENIED" {
		t.Fatalf("cluster-b reason_code = %q, want CLUSTER_POLICY_DENIED", itemsByID["cl-2"].Compatibility.ReasonCode)
	}
	if itemsByID["cl-2"].Compatibility.ReasonMessage == "" {
		t.Fatal("cluster-b reason_message is empty")
	}
}

func TestListClusters_CreateCompatibilityFilterReturnsCloneFallbackAdvisory(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:             "ubuntu-golden",
		Namespace:        "golden-images",
		Phase:            "Bound",
		StorageClassName: "source-sc",
	}})
	srv.vmService = service.NewVMService(mock)

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-1").
		SetName("golden-ubuntu").
		SetSourceType(service.TemplateSourceCDIPVCClone).
		SetPvcNamespace("golden-images").
		SetPvcName("ubuntu-golden").
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("sz-1").
		SetName("small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetDefaultStorageClass("target-sc").
		SetEnabledFeatures([]string{"LiveMigration"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster cl-1: %v", err)
	}

	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cl-1").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"target-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy policy-1: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20&namespace=prod-a&template_id=tpl-1&instance_size_id=sz-1",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:           1,
		PerPage:        20,
		Namespace:      "prod-a",
		TemplateId:     "tpl-1",
		InstanceSizeId: "sz-1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if !resp.Items[0].Compatibility.Eligible {
		t.Fatalf("compatibility = %#v, want eligible", resp.Items[0].Compatibility)
	}
	if resp.Items[0].Compatibility.AdvisoryCode != "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY" {
		t.Fatalf("advisory_code = %q, want PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY", resp.Items[0].Compatibility.AdvisoryCode)
	}
	if resp.Items[0].Compatibility.AdvisoryMessage == "" {
		t.Fatal("advisory_message is empty")
	}
}

func TestGetClusterPolicy_ReturnsPolicy(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cl-1").
		SetAllowCPUOvercommit(false).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"fast-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters/cl-1/policy",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetClusterPolicy(c, "cl-1")
	if w.Code != http.StatusOK {
		t.Fatalf("get cluster policy status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterPolicy
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if !resp.AllowCdiClone {
		t.Fatal("allow_cdi_clone = false, want true")
	}
	if len(resp.AllowedStorageClasses) != 1 || resp.AllowedStorageClasses[0] != "fast-sc" {
		t.Fatalf("allowed_storage_classes = %#v, want [fast-sc]", resp.AllowedStorageClasses)
	}
}

func TestUpsertClusterPolicy_CreatesAndUpdatesPolicy(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/clusters/cl-1/policy",
		`{"allow_cpu_overcommit":false,"allow_memory_overcommit":true,"allow_dedicated_cpu":true,"allow_gpu":true,"allow_sriov":true,"allow_hugepages":true,"allowed_hugepages_sizes":["2Mi","2Mi"],"allow_cdi_clone":true,"allowed_clone_source_namespaces":["golden-images"],"allowed_storage_classes":["fast-sc"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpsertClusterPolicy(createCtx, "cl-1")
	if createW.Code != http.StatusOK {
		t.Fatalf("upsert cluster policy status = %d, want %d, body=%s", createW.Code, http.StatusOK, createW.Body.String())
	}

	var created generated.ClusterPolicy
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.AllowCpuOvercommit {
		t.Fatal("allow_cpu_overcommit = true, want false")
	}
	if got := len(created.AllowedHugepagesSizes); got != 1 {
		t.Fatalf("allowed_hugepages_sizes len = %d, want 1 after normalization", got)
	}

	stored, err := client.ClusterPolicy.Query().
		Where(entclusterpolicy.ClusterIDEQ("cl-1")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load stored policy: %v", err)
	}
	if stored.CreatedBy != "admin-1" {
		t.Fatalf("created_by = %q, want admin-1", stored.CreatedBy)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/clusters/cl-1/policy",
		`{"allow_cpu_overcommit":true,"allow_memory_overcommit":true,"allow_dedicated_cpu":true,"allow_gpu":false,"allow_sriov":true,"allow_hugepages":false,"allowed_hugepages_sizes":[],"allow_cdi_clone":false,"allowed_clone_source_namespaces":[],"allowed_storage_classes":["gold-sc"]}`,
		"admin-2",
		[]string{"platform:admin"},
	)
	srv.UpsertClusterPolicy(updateCtx, "cl-1")
	if updateW.Code != http.StatusOK {
		t.Fatalf("update cluster policy status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	updated, err := client.ClusterPolicy.Query().
		Where(entclusterpolicy.ClusterIDEQ("cl-1")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load updated policy: %v", err)
	}
	if updated.CreatedBy != "admin-1" {
		t.Fatalf("created_by changed to %q, want admin-1", updated.CreatedBy)
	}
	if updated.UpdatedBy != "admin-2" {
		t.Fatalf("updated_by = %q, want admin-2", updated.UpdatedBy)
	}
	if updated.AllowGpu {
		t.Fatal("allow_gpu = true, want false after update")
	}
	if updated.AllowCdiClone {
		t.Fatal("allow_cdi_clone = true, want false after update")
	}
}

func TestListClusters_ReportsPolicyConfigured(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster a: %v", err)
	}
	_, err = client.Cluster.Create().
		SetID("cl-2").
		SetName("cluster-b").
		SetDisplayName("cluster-b").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster b: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cl-2").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{Page: 1, PerPage: 20})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].Id != "cl-2" || !resp.Items[0].PolicyConfigured {
		t.Fatalf("cluster-b policy_configured = %v, want true", resp.Items[0].PolicyConfigured)
	}
	if resp.Items[0].PolicySummary.Mode != generated.ClusterPolicySummaryMode("OPEN") {
		t.Fatalf("cluster-b policy_summary.mode = %q, want OPEN", resp.Items[0].PolicySummary.Mode)
	}
	if resp.Items[1].Id != "cl-1" || resp.Items[1].PolicyConfigured {
		t.Fatalf("cluster-a policy_configured = %v, want false", resp.Items[1].PolicyConfigured)
	}
	if resp.Items[1].PolicySummary.Mode != generated.ClusterPolicySummaryMode("MISSING") {
		t.Fatalf("cluster-a policy_summary.mode = %q, want MISSING", resp.Items[1].PolicySummary.Mode)
	}
}

func TestListClusters_ReportsGuardedPolicySummary(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-guarded").
		SetName("cluster-guarded").
		SetDisplayName("cluster-guarded").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create guarded cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-guarded").
		SetClusterID("cl-guarded").
		SetAllowCPUOvercommit(false).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(false).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowedHugepagesSizes([]string{"2Mi", "1Gi"}).
		SetAllowCdiClone(true).
		SetAllowedCloneSourceNamespaces([]string{"golden-images"}).
		SetAllowedStorageClasses([]string{"gold-sc", "silver-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create guarded cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{Page: 1, PerPage: 20})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	summary := resp.Items[0].PolicySummary
	if summary.Mode != generated.ClusterPolicySummaryMode("GUARDED") {
		t.Fatalf("policy_summary.mode = %q, want GUARDED", summary.Mode)
	}
	if len(summary.DeniedControls) != 2 {
		t.Fatalf("denied_controls = %#v, want 2 items", summary.DeniedControls)
	}
	if len(summary.ScopedControls) != 3 {
		t.Fatalf("scoped_controls = %#v, want 3 items", summary.ScopedControls)
	}
	if summary.AllowedStorageClassCount != 2 {
		t.Fatalf("allowed_storage_class_count = %d, want 2", summary.AllowedStorageClassCount)
	}
	if summary.AllowedCloneSourceNamespaceCount != 1 {
		t.Fatalf("allowed_clone_source_namespace_count = %d, want 1", summary.AllowedCloneSourceNamespaceCount)
	}
	if summary.AllowedHugepagesSizeCount != 2 {
		t.Fatalf("allowed_hugepages_size_count = %d, want 2", summary.AllowedHugepagesSizeCount)
	}
}
