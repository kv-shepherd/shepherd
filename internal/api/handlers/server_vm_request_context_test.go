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
		SetID("tpl-enabled").
		SetName("ubuntu-22-04").
		SetCreatedBy("seed").
		SetEnabled(true).
		SetSpec(map[string]interface{}{"image": "quay.io/kubevirt/fedora-cloud-container-disk-demo"}).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed enabled template: %v", err)
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
		SetID("size-enabled").
		SetName("small").
		SetCPUCores(2).
		SetMemoryMB(4096).
		SetCreatedBy("seed").
		SetSortOrder(1).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed enabled instance size: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-disabled").
		SetName("legacy").
		SetCPUCores(1).
		SetMemoryMB(1024).
		SetCreatedBy("seed").
		SetSortOrder(2).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed disabled instance size: %v", err)
	}

	t.Run("platform admin gets enabled namespaces and enabled catalog", func(t *testing.T) {
		c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "admin-1", []string{"platform:admin"})
		srv.GetVMRequestContext(c)

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
		if len(resp.Templates) != 1 || resp.Templates[0].Id != "tpl-enabled" {
			t.Fatalf("unexpected templates: %+v", resp.Templates)
		}
		if len(resp.InstanceSizes) != 1 || resp.InstanceSizes[0].Id != "size-enabled" {
			t.Fatalf("unexpected instance sizes: %+v", resp.InstanceSizes)
		}
	})

	t.Run("user without role bindings gets empty namespace list but same enabled catalog", func(t *testing.T) {
		c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "user-no-binding", []string{"vm:create"})
		srv.GetVMRequestContext(c)

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
		if len(resp.Templates) != 1 || resp.Templates[0].Id != "tpl-enabled" {
			t.Fatalf("unexpected templates: %+v", resp.Templates)
		}
		if len(resp.InstanceSizes) != 1 || resp.InstanceSizes[0].Id != "size-enabled" {
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
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template b: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-a").
		SetName("a-template").
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed template a: %v", err)
	}

	_, err = client.InstanceSize.Create().
		SetID("size-b").
		SetName("medium").
		SetCPUCores(4).
		SetMemoryMB(8192).
		SetCreatedBy("seed").
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
		SetMemoryMB(4096).
		SetCreatedBy("seed").
		SetSortOrder(10).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed size a: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "admin-order", []string{"platform:admin"})
	srv.GetVMRequestContext(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp generated.VMRequestContext
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
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
