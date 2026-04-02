package handlers

import (
	"net/http"
	"testing"

	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestListAdminTemplates_SupportsQuickSearchAndExactFilters(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	createTemplate := func(id, name, displayName, osFamily, sourceType string, scope enttemplate.CatalogScope, enabled bool) {
		t.Helper()
		builder := client.Template.Create().
			SetID(id).
			SetName(name).
			SetCatalogScope(scope).
			SetSourceType(sourceType).
			SetEnabled(enabled).
			SetCreatedBy("admin-1")
		if displayName != "" {
			builder = builder.SetDisplayName(displayName)
		}
		if osFamily != "" {
			builder = builder.SetOsFamily(osFamily)
		}
		if _, err := builder.Save(ctx); err != nil {
			t.Fatalf("create template %s: %v", id, err)
		}
	}

	createTemplate("tpl-linux", "ubuntu-golden", "Ubuntu Golden", "linux", "cdi_pvc_clone", enttemplate.CatalogScopeTest, false)
	createTemplate("tpl-windows", "windows-ltsc", "Windows LTSC", "windows", "cdi_image_import", enttemplate.CatalogScopeProd, true)

	t.Run("quick search matches id", func(t *testing.T) {
		c, w := newAuthedGinContext(
			t,
			http.MethodGet,
			"/admin/templates?page=1&per_page=20&search=tpl-linux",
			"",
			"admin-1",
			[]string{"platform:admin"},
		)
		srv.ListAdminTemplates(c, generated.ListAdminTemplatesParams{
			Page:    1,
			PerPage: 20,
			Search:  "tpl-linux",
		})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp generated.TemplateList
		mustDecodeJSON(t, w.Body.Bytes(), &resp)
		if got := len(resp.Items); got != 1 {
			t.Fatalf("items len = %d, want 1", got)
		}
		if got := resp.Items[0].Id; got != "tpl-linux" {
			t.Fatalf("items[0].id = %q, want tpl-linux", got)
		}
	})

	t.Run("exact filters use catalog attributes rather than ids", func(t *testing.T) {
		c, w := newAuthedGinContext(
			t,
			http.MethodGet,
			"/admin/templates?page=1&per_page=20&os_family=linux&source_type=cdi_pvc_clone&catalog_scope=test&enabled=false",
			"",
			"admin-1",
			[]string{"platform:admin"},
		)
		srv.ListAdminTemplates(c, generated.ListAdminTemplatesParams{
			Page:         1,
			PerPage:      20,
			OsFamily:     "linux",
			SourceType:   "cdi_pvc_clone",
			CatalogScope: "test",
			Enabled:      false,
		})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp generated.TemplateList
		mustDecodeJSON(t, w.Body.Bytes(), &resp)
		if got := len(resp.Items); got != 1 {
			t.Fatalf("items len = %d, want 1", got)
		}
		if got := resp.Items[0].Name; got != "ubuntu-golden" {
			t.Fatalf("items[0].name = %q, want ubuntu-golden", got)
		}
	})
}
