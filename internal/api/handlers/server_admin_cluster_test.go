package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entclusterpolicy "kv-shepherd.io/shepherd/ent/clusterpolicy"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

func mustEncodeTestClusterKubeconfig(t *testing.T, serverURL string) string {
	t.Helper()
	raw := strings.TrimSpace(`
apiVersion: v1
kind: Config
clusters:
- name: runtime
  cluster:
    server: `+serverURL+`
users:
- name: runtime-user
  user:
    token: cluster-token
contexts:
- name: runtime
  context:
    cluster: runtime
    user: runtime-user
current-context: runtime
`) + "\n"
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

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

func TestCreateCluster_CreatesDefaultPolicy(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/clusters",
		`{"name":"cluster-a","display_name":"Cluster A","environment":"prod","kubeconfig":"`+mustEncodeTestClusterKubeconfig(t, "https://cluster.example.com")+`"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateCluster(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create cluster status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp generated.Cluster
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if !resp.PolicyConfigured {
		t.Fatal("policy_configured = false, want true")
	}
	if resp.PolicySummary.Mode != generated.ClusterPolicySummaryMode("GUARDED") {
		t.Fatalf("policy_summary.mode = %q, want GUARDED", resp.PolicySummary.Mode)
	}

	stored, err := client.Cluster.Query().Where(entcluster.NameEQ("cluster-a")).Only(ctx)
	if err != nil {
		t.Fatalf("load created cluster: %v", err)
	}
	if stored.EncryptionKeyID == "" {
		t.Fatal("encryption_key_id = empty, want populated")
	}
	if strings.Contains(string(stored.EncryptedKubeconfig), "cluster-token") {
		t.Fatalf("stored kubeconfig leaked plaintext token: %s", string(stored.EncryptedKubeconfig))
	}
	policy, err := client.ClusterPolicy.Query().
		Where(entclusterpolicy.ClusterIDEQ(stored.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("load default cluster policy: %v", err)
	}
	if !policy.AllowCPUOvercommit {
		t.Fatal("allow_cpu_overcommit = false, want true")
	}
	if !policy.AllowMemoryOvercommit {
		t.Fatal("allow_memory_overcommit = false, want true")
	}
	if !policy.AllowCdiClone {
		t.Fatal("allow_cdi_clone = false, want true")
	}
	if policy.AllowDedicatedCPU {
		t.Fatal("allow_dedicated_cpu = true, want false")
	}
	if policy.AllowGpu {
		t.Fatal("allow_gpu = true, want false")
	}
	if policy.AllowSriov {
		t.Fatal("allow_sriov = true, want false")
	}
	if policy.AllowHugepages {
		t.Fatal("allow_hugepages = true, want false")
	}
}

func TestCreateCluster_RejectsUnsafeKubeconfigExecPlugin(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)
	unsafeKubeconfig := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(`
apiVersion: v1
kind: Config
clusters:
- name: runtime
  cluster:
    server: https://cluster.example.com
users:
- name: runtime-user
  user:
    exec:
      command: /bin/sh
      apiVersion: client.authentication.k8s.io/v1
      interactiveMode: Never
contexts:
- name: runtime
  context:
    cluster: runtime
    user: runtime-user
current-context: runtime
`) + "\n"))

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/clusters",
		`{"name":"cluster-a","display_name":"Cluster A","environment":"prod","kubeconfig":"`+unsafeKubeconfig+`"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateCluster(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create cluster status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "exec credential plugins are not supported") {
		t.Fatalf("response body = %s, want exec-plugin rejection", w.Body.String())
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
	if itemsByID["cl-2"].Compatibility.ReasonCode != "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED" {
		t.Fatalf(
			"cluster-b reason_code = %q, want CLUSTER_POLICY_STORAGE_CLASS_REQUIRED",
			itemsByID["cl-2"].Compatibility.ReasonCode,
		)
	}
	if itemsByID["cl-2"].Compatibility.ReasonMessage == "" {
		t.Fatal("cluster-b reason_message is empty")
	}
}

func TestListClusters_CreateCompatibilityFilterMarksExplicitStorageClassRequirement(t *testing.T) {
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
		SetStorageClasses([]string{"gold-sc", "silver-sc"}).
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
		SetAllowDedicatedCPU(false).
		SetAllowGpu(false).
		SetAllowSriov(false).
		SetAllowHugepages(false).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"gold-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy policy-1: %v", err)
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
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Compatibility.Eligible {
		t.Fatalf("cluster compatibility = %#v, want incompatible", resp.Items[0].Compatibility)
	}
	if resp.Items[0].Compatibility.ReasonCode != "CLUSTER_POLICY_STORAGE_CLASS_REQUIRED" {
		t.Fatalf("cluster reason_code = %q, want CLUSTER_POLICY_STORAGE_CLASS_REQUIRED", resp.Items[0].Compatibility.ReasonCode)
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
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name: "target-sc",
		ClaimPropertySets: []domain.StorageClaimPropertySet{{
			AccessModes: []string{"ReadWriteOnce"},
			VolumeMode:  "Filesystem",
		}},
		DefaultVolumeMode: "Filesystem",
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

func TestListClusters_CreateCompatibilityFilterResolvedRootVolumeKeepsModeOptions(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mock := provider.NewMockProvider()
	mock.SeedStorageProfiles([]*domain.StorageProfile{{
		Name: "rook-ceph-block",
		ClaimPropertySets: []domain.StorageClaimPropertySet{
			{
				AccessModes: []string{"ReadWriteMany"},
				VolumeMode:  "Block",
			},
			{
				AccessModes: []string{"ReadWriteOnce"},
				VolumeMode:  "Block",
			},
		},
		DefaultVolumeMode: "Block",
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
		SetDefaultStorageClass("rook-ceph-block").
		SetStorageClasses([]string{"rook-ceph-block"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
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
		SetAllowedStorageClasses([]string{"rook-ceph-block"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20&namespace=prod-a&template_id=tpl-1&instance_size_id=sz-1&selected_storage_class=rook-ceph-block&selected_dv_access_modes=ReadWriteMany&selected_dv_volume_mode=Block&include_incompatible=true",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:                  1,
		PerPage:               20,
		Namespace:             "prod-a",
		TemplateId:            "tpl-1",
		InstanceSizeId:        "sz-1",
		SelectedStorageClass:  "rook-ceph-block",
		SelectedDvAccessModes: []string{"ReadWriteMany"},
		SelectedDvVolumeMode:  generated.ListClustersParamsSelectedDvVolumeMode("Block"),
		IncludeIncompatible:   true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}

	resolution := resp.Items[0].Compatibility.RootVolumeResolution
	if resolution.State != generated.RootVolumeResolutionStateResolved {
		t.Fatalf("root volume resolution state = %q, want %q", resolution.State, generated.RootVolumeResolutionStateResolved)
	}
	if resolution.EffectiveStorageClass != "rook-ceph-block" {
		t.Fatalf("effective storage class = %q, want rook-ceph-block", resolution.EffectiveStorageClass)
	}
	if got := len(resolution.ModeOptions); got != 2 {
		t.Fatalf("mode options len = %d, want 2", got)
	}
}

func TestListClusters_CreateCompatibilityFilterReturnsBadRequestForInvalidSpec(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-invalid-1").
		SetName("prod-invalid").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-invalid-1").
		SetName("fedora-invalid").
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		SetCatalogScope("prod").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("sz-invalid-1").
		SetName("invalid-overcommit").
		SetCatalogScope("prod").
		SetCPUCores(2).
		SetCPURequest(4).
		SetMemoryGi(4).
		SetMemoryRequestGi(2).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create invalid instance size: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-invalid-1").
		SetName("cluster-invalid").
		SetDisplayName("cluster-invalid").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=20&namespace=prod-invalid&template_id=tpl-invalid-1&instance_size_id=sz-invalid-1&include_incompatible=true",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:                1,
		PerPage:             20,
		Namespace:           "prod-invalid",
		TemplateId:          "tpl-invalid-1",
		InstanceSizeId:      "sz-invalid-1",
		IncludeIncompatible: true,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("list clusters invalid spec status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var apiErr generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &apiErr)
	if apiErr.Code != "OVERCOMMIT_INVALID" {
		t.Fatalf("error.code = %q, want OVERCOMMIT_INVALID", apiErr.Code)
	}
	if apiErr.Message == "" {
		t.Fatal("error.message = empty, want validation detail")
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
		SetAllowedHugepagesSizes([]string{"2mi", "1gi"}).
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
	if diff := cmp.Diff([]string{"2Mi", "1Gi"}, resp.AllowedHugepagesSizes); diff != "" {
		t.Fatalf("allowed_hugepages_sizes mismatch (-want +got):\n%s", diff)
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
		`{"allow_cpu_overcommit":false,"allow_memory_overcommit":true,"allow_dedicated_cpu":true,"allow_gpu":true,"allow_sriov":true,"allow_hugepages":true,"allowed_hugepages_sizes":["2mi","2Mi","512"],"allow_cdi_clone":true,"allowed_clone_source_namespaces":["golden-images"],"allowed_storage_classes":["fast-sc"]}`,
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
	if diff := cmp.Diff([]string{"2Mi", "512Mi"}, created.AllowedHugepagesSizes); diff != "" {
		t.Fatalf("allowed_hugepages_sizes mismatch (-want +got):\n%s", diff)
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
	if diff := cmp.Diff([]string{"2Mi", "512Mi"}, stored.AllowedHugepagesSizes); diff != "" {
		t.Fatalf("stored allowed_hugepages_sizes mismatch (-want +got):\n%s", diff)
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

func TestListClusters_IncludesDetectedStorageClasses(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-storage").
		SetName("cluster-storage").
		SetDisplayName("cluster-storage").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetDefaultStorageClass("fast-sc").
		SetStorageClasses([]string{"fast-sc", "bulk-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
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
	if got := resp.Items[0].StorageClasses; len(got) != 2 || got[0] != "fast-sc" || got[1] != "bulk-sc" {
		t.Fatalf("storage_classes = %#v, want [fast-sc bulk-sc]", got)
	}
	if got := resp.Items[0].DefaultStorageClass; got != "fast-sc" {
		t.Fatalf("default_storage_class = %q, want fast-sc", got)
	}
}

func TestUpdateCluster_ReplacesKubeconfigAndRefreshesHealth(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-update").
		SetName("cluster-old").
		SetDisplayName("Cluster Old").
		SetAPIServerURL("https://old.example.com").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\nclusters:\n- name: old\n  cluster:\n    server: https://old.example.com\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetKubevirtVersion("1.1.0").
		SetEnabledFeatures([]string{"LiveMigration"}).
		SetStorageClasses([]string{"fast-sc"}).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	srv.refreshClusterHealth = func(ctx context.Context, clusterID string) error {
		_, updateErr := client.Cluster.UpdateOneID(clusterID).
			SetStatus(entcluster.StatusHEALTHY).
			SetKubevirtVersion("1.2.0").
			SetEnabledFeatures([]string{"GPU"}).
			SetStorageClasses([]string{"gold-sc"}).
			Save(ctx)
		return updateErr
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/clusters/cl-update",
		`{"display_name":"Cluster New","environment":"prod","enabled":true,"kubeconfig":"`+mustEncodeTestClusterKubeconfig(t, "https://new.example.com")+`"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateCluster(c, "cl-update")
	if w.Code != http.StatusOK {
		t.Fatalf("update cluster status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.Cluster
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := resp.Name; got != "cluster-old" {
		t.Fatalf("response name = %q, want cluster-old", got)
	}
	if got := resp.ApiServerUrl; got != "https://new.example.com" {
		t.Fatalf("response api_server_url = %q, want https://new.example.com", got)
	}
	if got := resp.Status; got != generated.ClusterStatus(entcluster.StatusHEALTHY) {
		t.Fatalf("response status = %q, want HEALTHY", got)
	}
	if got := resp.KubevirtVersion; got != "1.2.0" {
		t.Fatalf("response kubevirt_version = %q, want 1.2.0", got)
	}

	stored, err := client.Cluster.Get(ctx, "cl-update")
	if err != nil {
		t.Fatalf("load updated cluster: %v", err)
	}
	if got := stored.Name; got != "cluster-old" {
		t.Fatalf("stored name = %q, want cluster-old", got)
	}
	if got := stored.APIServerURL; got != "https://new.example.com" {
		t.Fatalf("stored api_server_url = %q, want https://new.example.com", got)
	}
	if got := stored.Environment; got != entcluster.EnvironmentProd {
		t.Fatalf("stored environment = %q, want prod", got)
	}
	if got := stored.DisplayName; got != "Cluster New" {
		t.Fatalf("stored display_name = %q, want Cluster New", got)
	}
	if stored.EncryptionKeyID == "" {
		t.Fatal("stored encryption_key_id = empty, want populated")
	}
	if strings.Contains(string(stored.EncryptedKubeconfig), "cluster-token") {
		t.Fatalf("stored kubeconfig leaked plaintext token: %s", string(stored.EncryptedKubeconfig))
	}
}

func TestUpdateCluster_DisablingClusterForcesUnknownStatus(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-disable").
		SetName("cluster-disable").
		SetAPIServerURL("https://cluster.example.com").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/clusters/cl-disable",
		`{"enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateCluster(c, "cl-disable")
	if w.Code != http.StatusOK {
		t.Fatalf("disable cluster status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	stored, err := client.Cluster.Get(ctx, "cl-disable")
	if err != nil {
		t.Fatalf("load disabled cluster: %v", err)
	}
	if stored.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if got := stored.Status; got != entcluster.StatusUNKNOWN {
		t.Fatalf("status = %q, want UNKNOWN", got)
	}
}

func TestDeleteCluster_RejectsClusterStillUsedByVMs(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-in-use").
		SetName("cluster-in-use").
		SetAPIServerURL("https://cluster.example.com").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	system, err := client.System.Create().
		SetID("sys-1").
		SetName("system-a").
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	serviceEnt, err := client.Service.Create().
		SetID("svc-1").
		SetName("servicea").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = client.VM.Create().
		SetID("vm-1").
		SetName("vm-a").
		SetInstance("01").
		SetNamespace("ns-a").
		SetClusterID("cl-in-use").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("test").
		SetService(serviceEnt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/clusters/cl-in-use",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteCluster(c, "cl-in-use")
	if w.Code != http.StatusConflict {
		t.Fatalf("delete cluster status = %d, want %d, body=%s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != "CLUSTER_IN_USE" {
		t.Fatalf("error code = %q, want CLUSTER_IN_USE", resp.Code)
	}
	if _, err := client.Cluster.Get(ctx, "cl-in-use"); err != nil {
		t.Fatalf("cluster should still exist: %v", err)
	}
}

func TestDeleteCluster_RejectsClusterSelectedByActiveRequests(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-active-req").
		SetName("cluster-active-req").
		SetAPIServerURL("https://cluster.example.com").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	payload, err := json.Marshal(domain.VMCreationPayload{
		RequesterID:    "user-a",
		ServiceID:      "svc-a",
		TemplateID:     "tpl-a",
		InstanceSizeID: "size-a",
		Namespace:      "ns-a",
		Reason:         "pending request",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID("ev-active-req").
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-a").
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("user-a").
		Save(ctx); err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID("ticket-active-req").
		SetEventID("ev-active-req").
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusEXECUTING).
		SetRequester("user-a").
		SetSelectedClusterID("cl-active-req").
		SetReason("pending request").
		Save(ctx); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/clusters/cl-active-req",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteCluster(c, "cl-active-req")
	if w.Code != http.StatusConflict {
		t.Fatalf("delete cluster status = %d, want %d, body=%s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != "CLUSTER_HAS_ACTIVE_REQUESTS" {
		t.Fatalf("error code = %q, want CLUSTER_HAS_ACTIVE_REQUESTS", resp.Code)
	}
}

func TestDeleteCluster_DeletesClusterAndPolicyWhenUnused(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-delete").
		SetName("cluster-delete").
		SetAPIServerURL("https://cluster.example.com").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-delete").
		SetClusterID("cl-delete").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(false).
		SetAllowGpu(false).
		SetAllowSriov(false).
		SetAllowHugepages(false).
		SetAllowCdiClone(true).
		SetCreatedBy("test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/clusters/cl-delete",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteCluster(c, "cl-delete")
	if got := c.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete cluster status = %d, want %d, body=%s", got, http.StatusNoContent, w.Body.String())
	}

	if _, err := client.Cluster.Get(ctx, "cl-delete"); !ent.IsNotFound(err) {
		t.Fatalf("cluster get err = %v, want not found", err)
	}
	if _, err := client.ClusterPolicy.Query().
		Where(entclusterpolicy.ClusterIDEQ("cl-delete")).
		Only(ctx); !ent.IsNotFound(err) {
		t.Fatalf("cluster policy err = %v, want not found", err)
	}
}
