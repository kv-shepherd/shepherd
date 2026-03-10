package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestListTemplates_FiltersNonRequestableTemplateSources(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)

	mustCreateTemplate := func(id, name, sourceType, imageURL, pvcNamespace, pvcName string, scope enttemplate.CatalogScope) {
		t.Helper()
		create := client.Template.Create().
			SetID(id).
			SetName(name).
			SetCatalogScope(scope).
			SetEnabled(true).
			SetCreatedBy("seed")
		if sourceType != "" {
			create = create.SetSourceType(sourceType)
		}
		if imageURL != "" {
			create = create.SetImageURL(imageURL)
		}
		if pvcNamespace != "" {
			create = create.SetPvcNamespace(pvcNamespace)
		}
		if pvcName != "" {
			create = create.SetPvcName(pvcName)
		}
		if _, err := create.Save(t.Context()); err != nil {
			t.Fatalf("create template %s: %v", id, err)
		}
	}

	mustCreateTemplate("tpl-containerdisk", "ephemeral-fedora", "containerdisk", "docker://quay.io/containerdisks/fedora:40", "", "", enttemplate.CatalogScopeTest)
	mustCreateTemplate("tpl-import", "import-ubuntu", "cdi_image_import", "docker://quay.io/containerdisks/ubuntu:24.04", "", "", enttemplate.CatalogScopeProd)
	mustCreateTemplate("tpl-clone", "clone-rhel", "cdi_pvc_clone", "", "golden-images", "rhel-golden", enttemplate.CatalogScopeAll)
	mustCreateTemplate("tpl-draft", "draft-centos", "", "", "", "", enttemplate.CatalogScopeProd)

	c, w := newAuthedGinContext(t, http.MethodGet, "/templates?page=1&per_page=20", "", "admin-1", []string{"platform:admin"})
	srv.ListTemplates(c, generated.ListTemplatesParams{Page: 1, PerPage: 20})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TemplateList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 2 {
		t.Fatalf("items len = %d, want 2", got)
	}
	if resp.Items[0].Id != "tpl-clone" || resp.Items[1].Id != "tpl-import" {
		t.Fatalf("unexpected templates: %+v", resp.Items)
	}
	if got := resp.Pagination.Total; got != 2 {
		t.Fatalf("pagination.total = %d, want 2", got)
	}
}

func TestListInstanceSizes_OmitsSpecOverridesFromPublicResponse(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-test-id").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	if _, err := client.InstanceSize.Create().
		SetID("size-public").
		SetName("public-small").
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		SetCatalogScope(instancesize.CatalogScopeTest).
		SetEnabled(true).
		SetSpecOverrides(map[string]interface{}{
			"spec.domain.cpu.dedicatedCpuPlacement": true,
		}).
		Save(t.Context()); err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/instance-sizes", "", "admin-1", []string{"platform:admin"})
	srv.ListInstanceSizes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.InstanceSizeList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].SpecOverrides != nil {
		t.Fatalf("SpecOverrides = %#v, want nil in public response", resp.Items[0].SpecOverrides)
	}

	var raw struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, found := raw.Items[0]["spec_overrides"]; found {
		t.Fatalf("spec_overrides key should be omitted from public response: %+v", raw.Items[0])
	}
}
