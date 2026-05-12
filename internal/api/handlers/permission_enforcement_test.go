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

	srv.GetVMRequestContext(c, generated.GetVMRequestContextParams{})
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

func TestPermissionEnforcement_ListSystemMemberCandidates_RequiresRbacManage(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-1/member-candidates", "", "user-a", []string{"system:read"})

	srv.ListSystemMemberCandidates(c, "sys-1", generated.ListSystemMemberCandidatesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListNamespaces_RequiresNamespacePermission(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/namespaces", "", "user-a", []string{"vm:read"})

	srv.ListNamespaces(c, generated.ListNamespacesParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListExternalApprovalSystems_RequiresPlatformAdmin(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/external-approval-systems", "", "user-a", []string{"auth_provider:read"})

	srv.ListExternalApprovalSystems(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateExternalApprovalSystem_RequiresPlatformAdmin(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/external-approval-systems",
		`{"name":"approval","webhook_url":"https://approval.example.com/shepherd","signing_key":"webhook-secret"}`,
		"user-a",
		[]string{"auth_provider:update"},
	)

	srv.CreateExternalApprovalSystem(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateNamespace_RequiresNamespaceWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodPost, "/admin/namespaces", `{"name":"ns-a","environment":"test"}`, "user-a", []string{"namespace:read"})

	srv.CreateNamespace(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_UpdateNamespace_RequiresNamespaceWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/namespaces/ns-a",
		`{"description":"updated"}`,
		"user-a",
		[]string{"namespace:read"},
	)

	srv.UpdateNamespace(c, "ns-a")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteNamespace_RequiresNamespaceWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/namespaces/ns-a",
		"",
		"user-a",
		[]string{"namespace:read"},
	)

	srv.DeleteNamespace(c, "ns-a", generated.DeleteNamespaceParams{ConfirmName: "ns-a"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_CreateCluster_RequiresClusterWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/clusters",
		`{"name":"cluster-a","kubeconfig":"a3ViZWNvbmZpZw=="}`,
		"user-a",
		[]string{"cluster:read"},
	)

	srv.CreateCluster(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_UpdateCluster_RequiresClusterWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/clusters/cl-a",
		`{"display_name":"updated"}`,
		"user-a",
		[]string{"cluster:read"},
	)

	srv.UpdateCluster(c, "cl-a")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteCluster_RequiresClusterWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/clusters/cl-a",
		"",
		"user-a",
		[]string{"cluster:read"},
	)

	srv.DeleteCluster(c, "cl-a")
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

func TestPermissionEnforcement_ListServicesOverview_RequiresServiceRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/services", "", "user-a", []string{"system:read"})

	srv.ListServicesOverview(c, generated.ListServicesOverviewParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_GetService_RequiresServiceRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-1/services/svc-1", "", "user-a", []string{"system:read"})

	srv.GetService(c, "sys-1", "svc-1")
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

func TestPermissionEnforcement_GetServiceWorkspaceContext_RequiresServiceRead(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-1/services/svc-1/context", "", "user-a", []string{"system:read"})

	srv.GetServiceWorkspaceContext(c, "sys-1", "svc-1")
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

func TestPermissionEnforcement_UpdateAdminTemplate_RequiresTemplateWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/tpl-1",
		`{"display_name":"updated"}`,
		"user-a",
		[]string{"template:read"},
	)

	srv.UpdateAdminTemplate(c, "tpl-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteAdminTemplate_RequiresTemplateWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/templates/tpl-1",
		"",
		"user-a",
		[]string{"template:read"},
	)

	srv.DeleteAdminTemplate(c, "tpl-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListAdminInstanceSizes_RequiresInstanceSizeReadOrWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/instance-sizes", "", "user-a", []string{"vm:create"})

	srv.ListAdminInstanceSizes(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ListAdminInstanceSizes_AllowsInstanceSizeWrite(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/instance-sizes", "", "user-a", []string{"instance_size:write"})

	srv.ListAdminInstanceSizes(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestPermissionEnforcement_CreateAdminInstanceSize_RequiresInstanceSizeWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/instance-sizes",
		`{"name":"m4.large","cpu_cores":4,"memory_gi":8}`,
		"user-a",
		[]string{"instance_size:read"},
	)

	srv.CreateAdminInstanceSize(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_UpdateAdminInstanceSize_RequiresInstanceSizeWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/size-1",
		`{"display_name":"updated"}`,
		"user-a",
		[]string{"instance_size:read"},
	)

	srv.UpdateAdminInstanceSize(c, "size-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_DeleteAdminInstanceSize_RequiresInstanceSizeWrite(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/instance-sizes/size-1",
		"",
		"user-a",
		[]string{"instance_size:read"},
	)

	srv.DeleteAdminInstanceSize(c, "size-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_ApproveBuiltinApprovalTask_RequiresBuiltinApprovalApprove(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/builtin-approval/tasks/ticket-1/approve",
		`{"comment":"looks good"}`,
		"user-a",
		[]string{"builtin_approval:view"},
	)

	srv.ApproveBuiltinApprovalTask(c, "ticket-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestPermissionEnforcement_RejectBuiltinApprovalTask_RequiresBuiltinApprovalApprove(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/builtin-approval/tasks/ticket-1/reject",
		`{"reason":"policy mismatch"}`,
		"user-a",
		[]string{"builtin_approval:view"},
	)

	srv.RejectBuiltinApprovalTask(c, "ticket-1")
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
		`{"name":"corp-ldap","auth_type":"ldap","config":{"server_url":"ldaps://ldap.example.com:636","bind_dn":"cn=admin,dc=example,dc=com","base_dn":"dc=example,dc=com"}}`,
		"user-a",
		[]string{"auth_provider:read"},
	)

	srv.CreateAuthProvider(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}
