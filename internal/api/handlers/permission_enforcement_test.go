package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestPermissionEnforcement_CreateSystem_RequiresSystemWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems",
		`{"name":"shop"}`,
		"user-a",
		[]string{"system:read"},
	)

	srv.CreateSystem(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListSystems_RequiresSystemRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems", "", "user-a", nil)

	srv.ListSystems(c, generated.ListSystemsParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_GetVMRequestContext_RequiresVMCreate(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/request-context", "", "user-a", []string{"vm:read"})

	srv.GetVMRequestContext(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_RequestVNC_RequiresVncAccess(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/vm-1/console/request", "", "user-a", []string{"vm:read"})

	srv.RequestVMConsoleAccess(c, "vm-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListUsers_RequiresUserOrRbacPermission(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/users", "", "user-a", []string{"system:read"})

	srv.ListUsers(c, generated.ListUsersParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListNamespaces_RequiresClusterPermission(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/namespaces", "", "user-a", []string{"vm:read"})

	srv.ListNamespaces(c, generated.ListNamespacesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateNamespace_RequiresClusterWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodPost, "/admin/namespaces", `{"name":"ns-a","environment":"test"}`, "user-a", []string{"cluster:read"})

	srv.CreateNamespace(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListTemplates_RequiresVmCreateOrTemplateRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/templates", "", "user-a", []string{"vm:read"})

	srv.ListTemplates(c, generated.ListTemplatesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListInstanceSizes_RequiresVmCreateOrInstanceSizeRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/instance-sizes", "", "user-a", []string{"vm:read"})

	srv.ListInstanceSizes(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteSystem_RequiresSystemDelete(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodDelete, "/systems/sys-1", "", "user-a", []string{"system:write"})

	srv.DeleteSystem(c, "sys-1", generated.DeleteSystemParams{ConfirmName: "shop"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteService_RequiresServiceDelete(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/sys-1/services/svc-1",
		"",
		"user-a",
		[]string{"service:create"},
	)

	srv.DeleteService(c, "sys-1", "svc-1", generated.DeleteServiceParams{Confirm: true})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListServices_RequiresServiceRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-1/services", "", "user-a", []string{"system:read"})

	srv.ListServices(c, "sys-1", generated.ListServicesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateService_RequiresServiceCreate(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems/sys-1/services",
		`{"name":"svc-a"}`,
		"user-a",
		[]string{"service:read"},
	)

	srv.CreateService(c, "sys-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_UpdateService_RequiresServiceCreate(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/sys-1/services/svc-1",
		`{"description":"updated"}`,
		"user-a",
		[]string{"service:read"},
	)

	srv.UpdateService(c, "sys-1", "svc-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListAdminTemplates_RequiresTemplateRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/templates", "", "user-a", []string{"vm:read"})

	srv.ListAdminTemplates(c, generated.ListAdminTemplatesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateAdminTemplate_RequiresTemplateWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base"}`,
		"user-a",
		[]string{"template:read"},
	)

	srv.CreateAdminTemplate(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListAdminInstanceSizes_RequiresInstanceSizeRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/instance-sizes", "", "user-a", []string{"vm:create"})

	srv.ListAdminInstanceSizes(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateAdminInstanceSize_RequiresInstanceSizeWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/instance-sizes",
		`{"name":"m4.large","cpu_cores":4,"memory_mb":8192}`,
		"user-a",
		[]string{"instance_size:read"},
	)

	srv.CreateAdminInstanceSize(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListRoles_RequiresRbacRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/roles", "", "user-a", []string{"system:read"})

	srv.ListRoles(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateRole_RequiresRbacManage(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/roles",
		`{"name":"viewer-extra","permissions":["vm:read"]}`,
		"user-a",
		[]string{"rbac:read"},
	)

	srv.CreateRole(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListAuthProviders_RequiresAuthProviderRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/auth-providers", "", "user-a", []string{"system:read"})

	srv.ListAuthProviders(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateAuthProvider_RequiresAuthProviderConfigure(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{"name":"corp-sso","auth_type":"generic","config":{"issuer":"https://idp.example.com"}}`,
		"user-a",
		[]string{"auth_provider:read"},
	)

	srv.CreateAuthProvider(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}
