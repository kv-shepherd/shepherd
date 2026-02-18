package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

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
		`{"name":"ubuntu-base","display_name":"Ubuntu Base","description":"base image","os_family":"linux","os_version":"22.04","enabled":true}`,
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

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/"+created.Id,
		`{"display_name":"Ubuntu Base v2","enabled":false}`,
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
		`{"name":"m4.large","display_name":"M4 Large","cpu_cores":4,"memory_mb":8192,"disk_gb":80,"dedicated_cpu":false,"enabled":true}`,
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

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/"+created.Id,
		`{"display_name":"M4 Large Updated","requires_gpu":true,"enabled":false}`,
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
	if !updated.RequiresGpu {
		t.Fatal("expected requires_gpu=true after update")
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
