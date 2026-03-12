package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestAdminTemplateCRUD(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","display_name":"Ubuntu Base","description":"base image","catalog_scope":"prod","os_family":"linux","os_version":"22.04","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.Template
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.Id == "" || created.Name != "ubuntu-base" {
		t.Fatalf("unexpected created template: %+v", created)
	}
	if created.CatalogScope != "prod" {
		t.Fatalf("catalog_scope = %q, want %q", created.CatalogScope, "prod")
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/"+created.Id,
		`{"display_name":"Ubuntu Base v2","catalog_scope":"all","enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminTemplate(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	var updated generated.Template
	mustDecodeJSON(t, updateW.Body.Bytes(), &updated)
	if updated.DisplayName != "Ubuntu Base v2" {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, "Ubuntu Base v2")
	}
	if updated.CatalogScope != "all" {
		t.Fatalf("catalog_scope = %q, want %q", updated.CatalogScope, "all")
	}
	if updated.Enabled {
		t.Fatal("expected template enabled=false after update")
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/templates?page=1&per_page=20",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAdminTemplates(listCtx, generated.ListAdminTemplatesParams{})
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/templates/"+created.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminTemplate(deleteCtx, created.Id)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	if _, err := client.Template.Get(t.Context(), created.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected template deleted, err=%v", err)
	}
}

func TestAdminInstanceSizeCRUD(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/instance-sizes",
		`{"name":"m4.large","display_name":"M4 Large","catalog_scope":"test","cpu_cores":4,"memory_gi":8,"disk_gb":80,"cpu_request":2,"memory_request_gi":6,"sort_order":20,"dedicated_cpu":false,"enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminInstanceSize(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.InstanceSize
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.Id == "" || created.Name != "m4.large" {
		t.Fatalf("unexpected created instance size: %+v", created)
	}
	if created.CatalogScope != "test" {
		t.Fatalf("catalog_scope = %q, want %q", created.CatalogScope, "test")
	}
	if created.CpuRequest != 2 {
		t.Fatalf("cpu_request = %v, want 2", created.CpuRequest)
	}
	if created.MemoryRequestGi != 6 {
		t.Fatalf("memory_request_gi = %v, want 6", created.MemoryRequestGi)
	}
	if created.SortOrder != 20 {
		t.Fatalf("sort_order = %d, want 20", created.SortOrder)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/"+created.Id,
		`{"display_name":"M4 Large Updated","catalog_scope":"all","requires_gpu":true,"enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminInstanceSize(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	var updated generated.InstanceSize
	mustDecodeJSON(t, updateW.Body.Bytes(), &updated)
	if updated.DisplayName != "M4 Large Updated" {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, "M4 Large Updated")
	}
	if updated.CatalogScope != "all" {
		t.Fatalf("catalog_scope = %q, want %q", updated.CatalogScope, "all")
	}
	if !updated.RequiresGpu {
		t.Fatal("expected requires_gpu=true after update")
	}
	if updated.CpuRequest != 2 {
		t.Fatalf("cpu_request = %v, want 2 after unchanged update", updated.CpuRequest)
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/instance-sizes",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAdminInstanceSizes(listCtx)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/instance-sizes/"+created.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminInstanceSize(deleteCtx, created.Id)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	if _, err := client.InstanceSize.Get(t.Context(), created.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected instance size deleted, err=%v", err)
	}
}

func TestAdminTemplateCreate_StoresCanonicalSourceType(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","source_type":"cdi_pvc_clone","pvc_name":"golden-ubuntu","pvc_namespace":"golden-images","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.Template
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if got, want := string(created.SourceType), "cdi_pvc_clone"; got != want {
		t.Fatalf("source_type = %q, want %q", got, want)
	}

	stored, err := client.Template.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("load created template: %v", err)
	}
	if got, want := stored.SourceType, "cdi_pvc_clone"; got != want {
		t.Fatalf("stored source_type = %q, want %q", got, want)
	}
}

func TestAdminTemplateCreate_RejectsLegacySourceTypeAlias(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","source_type":"pvc","pvc_name":"golden-ubuntu","pvc_namespace":"golden-images","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusBadRequest, createW.Body.String())
	}
}

func TestAdminTemplateCreate_RejectsContainerDiskForProdCatalog(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"fedora-ephemeral","source_type":"containerdisk","image_url":"quay.io/containerdisks/fedora:40","catalog_scope":"prod","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusBadRequest, createW.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, createW.Body.Bytes(), &resp)
	if got, want := resp.Code, "INVALID_TEMPLATE_SOURCE_SCOPE"; got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

func TestAdminTemplateUpdate_RejectsPromotingContainerDiskToAllCatalog(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	templateID := "tpl-" + uuid.NewString()
	_, err := client.Template.Create().
		SetID(templateID).
		SetName("fedora-ephemeral").
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/fedora:40").
		SetCatalogScope("test").
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/"+templateID,
		`{"catalog_scope":"all"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminTemplate(updateCtx, templateID)
	if updateW.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusBadRequest, updateW.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, updateW.Body.Bytes(), &resp)
	if got, want := resp.Code, "INVALID_TEMPLATE_SOURCE_SCOPE"; got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

func newAdminCatalogTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client := testutil.OpenEntPostgres(t, "admin_catalog")
	return NewServer(ServerDeps{EntClient: client}), client
}

func mustDecodeJSON(t *testing.T, payload []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(payload, out); err != nil {
		t.Fatalf("decode json: %v; payload=%s", err, string(payload))
	}
}
