package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

func TestVMHandler_GetVMRequestContext_CatalogAndVisibility(t *testing.T) {
	t.Parallel()
	srv, client := newSystemBehaviorTestServer(t)

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-test-id").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed namespace test: %v", err)
	}
	_, err = client.NamespaceRegistry.Create().
		SetID("ns-prod-id").
		SetName("team-prod").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed namespace prod: %v", err)
	}
	_, err = client.NamespaceRegistry.Create().
		SetID("ns-disabled-id").
		SetName("team-disabled").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed namespace disabled: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-test").
		SetName("ubuntu-22-04").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeTest).
		SetSourceType("containerdisk").
		SetImageURL("docker://quay.io/containerdisks/fedora:40").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed test template: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-all").
		SetName("alpine").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeAll).
		SetSourceType("cdi_image_import").
		SetImageURL("docker://quay.io/containerdisks/alpine:3.19").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed all template: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-prod").
		SetName("rhel-hardened").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeProd).
		SetSourceType("cdi_pvc_clone").
		SetPvcNamespace("golden-images").
		SetPvcName("rhel-hardened-golden").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed prod template: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-unclassified").
		SetName("needs-review").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeUnclassified).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed unclassified template: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-disabled").
		SetName("legacy").
		SetCreatedBy("seed").
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed disabled template: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("size-test").
		SetName("small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetSpecOverrides(map[string]interface{}{
			"spec.domain.cpu.dedicatedCpuPlacement": true,
		}).
		SetSortOrder(1).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed test instance size: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-all").
		SetName("shared").
		SetCPUCores(3).
		SetMemoryGi(6).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeAll).
		SetSortOrder(2).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed all instance size: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-prod").
		SetName("prod-large").
		SetCPUCores(8).
		SetMemoryGi(16).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetSortOrder(3).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed prod instance size: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-unclassified").
		SetName("review-me").
		SetCPUCores(1).
		SetMemoryGi(2).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeUnclassified).
		SetSortOrder(4).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed unclassified instance size: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-disabled").
		SetName("legacy").
		SetCPUCores(1).
		SetMemoryGi(1).
		SetCreatedBy("seed").
		SetSortOrder(2).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed disabled instance size: %v", err)
	}

	t.Run("platform admin gets enabled namespaces and enabled catalog", func(t *testing.T) {
		c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "admin-1", []string{"platform:admin"})
		srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{})

		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var resp generated.VMRequestContext
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Namespaces) != 2 || resp.Namespaces[0] != "team-prod" || resp.Namespaces[1] != "team-test" {
			t.Fatalf("unexpected namespaces: %+v", resp.Namespaces)
		}
		if len(resp.Templates) != 2 || resp.Templates[0].Id != "tpl-all" || resp.Templates[1].Id != "tpl-prod" {
			t.Fatalf("unexpected templates: %+v", resp.Templates)
		}
		if len(resp.InstanceSizes) != 3 || resp.InstanceSizes[0].Id != "size-test" || resp.InstanceSizes[1].Id != "size-all" || resp.InstanceSizes[2].Id != "size-prod" {
			t.Fatalf("unexpected instance sizes: %+v", resp.InstanceSizes)
		}
		if resp.InstanceSizes[0].SpecOverrides != nil || resp.InstanceSizes[1].SpecOverrides != nil || resp.InstanceSizes[2].SpecOverrides != nil {
			t.Fatalf("public request-context should omit spec_overrides: %+v", resp.InstanceSizes)
		}
	})

	t.Run("user without role bindings gets empty namespace list and empty catalog", func(t *testing.T) {
		c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "user-no-binding", []string{"vm:create"})
		srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{})

		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var resp generated.VMRequestContext
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Namespaces) != 0 {
			t.Fatalf("expected no namespaces, got %+v", resp.Namespaces)
		}
		if len(resp.Templates) != 0 {
			t.Fatalf("unexpected templates: %+v", resp.Templates)
		}
		if len(resp.InstanceSizes) != 0 {
			t.Fatalf("unexpected instance sizes: %+v", resp.InstanceSizes)
		}
	})

	t.Run("user with test environment restriction sees only test and all catalog items", func(t *testing.T) {
		userID := "user-test-" + uuid.NewString()
		roleID := "role-test-" + uuid.NewString()
		if _, err := client.User.Create().SetID(userID).SetUsername(userID).Save(t.Context()); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		roleObj, err := client.Role.Create().
			SetID(roleID).
			SetName(roleID).
			SetPermissions([]string{"vm:create"}).
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed role: %v", err)
		}
		if _, err := client.RoleBinding.Create().
			SetID("rb-test-" + uuid.NewString()).
			SetCreatedBy("seed").
			SetAllowedEnvironments([]string{"test"}).
			SetUserID(userID).
			SetRole(roleObj).
			Save(t.Context()); err != nil {
			t.Fatalf("seed role binding: %v", err)
		}

		c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", userID, []string{"vm:create"})
		srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{})
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}

		var resp generated.VMRequestContext
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Namespaces) != 1 || resp.Namespaces[0] != "team-test" {
			t.Fatalf("unexpected namespaces: %+v", resp.Namespaces)
		}
		if len(resp.Templates) != 1 || resp.Templates[0].Id != "tpl-all" {
			t.Fatalf("unexpected templates: %+v", resp.Templates)
		}
		if len(resp.InstanceSizes) != 2 || resp.InstanceSizes[0].Id != "size-test" || resp.InstanceSizes[1].Id != "size-all" {
			t.Fatalf("unexpected instance sizes: %+v", resp.InstanceSizes)
		}
	})
}

func TestVMHandler_GetVMRequestContext_OrdersCatalogDeterministically(t *testing.T) {
	t.Parallel()
	srv, client := newSystemBehaviorTestServer(t)

	_, err := client.Template.Create().
		SetID("tpl-b").
		SetName("z-template").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeTest).
		SetSourceType("cdi_pvc_clone").
		SetPvcNamespace("golden-images").
		SetPvcName("z-template-golden").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template b: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-a").
		SetName("a-template").
		SetCreatedBy("seed").
		SetCatalogScope(enttemplate.CatalogScopeAll).
		SetSourceType("cdi_image_import").
		SetImageURL("docker://quay.io/containerdisks/alpine:3.19").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template a: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("size-b").
		SetName("medium").
		SetCPUCores(4).
		SetMemoryGi(8).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeAll).
		SetSortOrder(20).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed size b: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-a").
		SetName("small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetSortOrder(10).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed size a: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "admin-order", []string{"platform:admin"})
	srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp generated.VMRequestContext
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if len(resp.Templates) != 2 || resp.Templates[0].Id != "tpl-a" || resp.Templates[1].Id != "tpl-b" {
		t.Fatalf("template order mismatch: %+v", resp.Templates)
	}
	if len(resp.InstanceSizes) != 2 || resp.InstanceSizes[0].Id != "size-a" || resp.InstanceSizes[1].Id != "size-b" {
		t.Fatalf("instance size order mismatch: %+v", resp.InstanceSizes)
	}

	// Sanity check this test assumptions stay aligned with queried fields.
	enabledTemplates, err := client.Template.Query().Where(enttemplate.EnabledEQ(true)).All(t.Context())
	if err != nil {
		t.Fatalf("query templates sanity check: %v", err)
	}
	if len(enabledTemplates) != 2 {
		t.Fatalf("expected 2 enabled templates, got %d", len(enabledTemplates))
	}
	enabledSizes, err := client.InstanceSize.Query().Where(instancesize.EnabledEQ(true)).All(t.Context())
	if err != nil {
		t.Fatalf("query sizes sanity check: %v", err)
	}
	if len(enabledSizes) != 2 {
		t.Fatalf("expected 2 enabled sizes, got %d", len(enabledSizes))
	}
}

func TestVMHandler_GetVMRequestContext_PlacementHintIsSanitized(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
	templateID := "00000000-0000-0000-0000-000000000001"
	instanceSizeID := "00000000-0000-0000-0000-000000000002"

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-prod").
		SetName("team-prod").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	_, err = client.Template.Create().
		SetID(templateID).
		SetName("ubuntu-prod").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeProd).
		SetSourceType("cdi_image_import").
		SetImageURL("docker://quay.io/containerdisks/ubuntu:22.04").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID(instanceSizeID).
		SetName("prod-medium").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetSortOrder(1).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-ok").
		SetName("cluster-ok").
		SetDisplayName("cluster-ok").
		SetEnvironment(entcluster.EnvironmentProd).
		SetAPIServerURL("https://cluster-ok.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed ok cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-ok").
		SetClusterID("cl-ok").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed ok cluster policy: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-unhealthy").
		SetName("cluster-unhealthy").
		SetDisplayName("cluster-unhealthy").
		SetEnvironment(entcluster.EnvironmentProd).
		SetAPIServerURL("https://cluster-unhealthy.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusUNHEALTHY).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed unhealthy cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-unhealthy").
		SetClusterID("cl-unhealthy").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed unhealthy cluster policy: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-missing-policy").
		SetName("cluster-missing-policy").
		SetDisplayName("cluster-missing-policy").
		SetEnvironment(entcluster.EnvironmentProd).
		SetAPIServerURL("https://cluster-missing.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed missing-policy cluster: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/vms/request-context?namespace=team-prod&template_id="+templateID+"&instance_size_id="+instanceSizeID,
		"",
		"admin-prod",
		[]string{"platform:admin"},
	)
	srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{
		Namespace:      "team-prod",
		TemplateId:     uuid.MustParse(templateID),
		InstanceSizeId: uuid.MustParse(instanceSizeID),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp generated.VMRequestContext
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PlacementHint.Status != generated.AVAILABLE {
		t.Fatalf("placement_hint.status = %q, want AVAILABLE", resp.PlacementHint.Status)
	}
	if resp.PlacementHint.CompatibleClusterCount != 1 {
		t.Fatalf("compatible_cluster_count = %d, want 1", resp.PlacementHint.CompatibleClusterCount)
	}
	if resp.PlacementHint.EvaluatedClusterCount != 3 {
		t.Fatalf("evaluated_cluster_count = %d, want 3", resp.PlacementHint.EvaluatedClusterCount)
	}
	if len(resp.PlacementHint.ReasonCounts) != 2 {
		t.Fatalf("reason_counts = %#v, want 2 items", resp.PlacementHint.ReasonCounts)
	}
	if resp.PlacementHint.PrimaryReasonCode == "" {
		t.Fatal("primary_reason_code is empty, want non-empty")
	}
	for _, reason := range resp.PlacementHint.ReasonCounts {
		if reason.Code == generated.VMPlacementHintReasonCountCodeOther {
			t.Fatalf("unexpected fallback reason code in sanitized hint: %#v", resp.PlacementHint.ReasonCounts)
		}
	}
}

func TestVMHandler_GetVMRequestContext_PlacementHintIncludesSanitizedAdvisory(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
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

	templateID := "00000000-0000-0000-0000-000000000011"
	instanceSizeID := "00000000-0000-0000-0000-000000000012"

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-prod-advisory").
		SetName("team-prod-advisory").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	_, err = client.Template.Create().
		SetID(templateID).
		SetName("ubuntu-clone").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetCatalogScope(enttemplate.CatalogScopeProd).
		SetSourceType(service.TemplateSourceCDIPVCClone).
		SetPvcNamespace("golden-images").
		SetPvcName("ubuntu-golden").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID(instanceSizeID).
		SetName("clone-medium").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetSortOrder(1).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed instance size: %v", err)
	}

	_, err = client.Cluster.Create().
		SetID("cl-advisory").
		SetName("cluster-advisory").
		SetDisplayName("cluster-advisory").
		SetEnvironment(entcluster.EnvironmentProd).
		SetAPIServerURL("https://cluster-advisory.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(true).
		SetDefaultStorageClass("target-sc").
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-advisory").
		SetClusterID("cl-advisory").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"target-sc"}).
		SetCreatedBy("seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed cluster policy: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/vms/request-context?namespace=team-prod-advisory&template_id="+templateID+"&instance_size_id="+instanceSizeID,
		"",
		"admin-prod",
		[]string{"platform:admin"},
	)
	srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{
		Namespace:      "team-prod-advisory",
		TemplateId:     uuid.MustParse(templateID),
		InstanceSizeId: uuid.MustParse(instanceSizeID),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp generated.VMRequestContext
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PlacementHint.Status != generated.AVAILABLE {
		t.Fatalf("placement_hint.status = %q, want AVAILABLE", resp.PlacementHint.Status)
	}
	if resp.PlacementHint.PrimaryAdvisoryCode != generated.VMPlacementHintPrimaryAdvisoryCodeHostAssistedCloneLikely {
		t.Fatalf("primary_advisory_code = %q, want HostAssistedCloneLikely", resp.PlacementHint.PrimaryAdvisoryCode)
	}
	if len(resp.PlacementHint.AdvisoryCounts) != 1 {
		t.Fatalf("advisory_counts = %#v, want 1 item", resp.PlacementHint.AdvisoryCounts)
	}
}
