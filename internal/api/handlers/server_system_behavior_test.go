package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	rrb "kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestSystemHandler_ListSystems_RespectsResourceBindings(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	sysVisible := mustCreateSystem(t, client, "sys-visible", "shop", "owner-1")
	_ = mustCreateSystem(t, client, "sys-hidden", "finance", "owner-2")
	mustCreateSystemBinding(t, client, "user-a", sysVisible.ID, "viewer")

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems", "", "user-a", []string{"system:read"})
	srv.ListSystems(c, generated.ListSystemsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != sysVisible.ID {
		t.Fatalf("visible system id = %s, want %s", resp.Items[0].Id, sysVisible.ID)
	}
}

func TestSystemHandler_UpdateSystem_DescriptionOnly(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID,
		`{"description":"new description"}`,
		"owner-1",
		[]string{"system:write"},
	)
	srv.UpdateSystem(c, sys.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := client.System.Get(c.Request.Context(), sys.ID)
	if err != nil {
		t.Fatalf("query updated system: %v", err)
	}
	if updated.Name != "shop" {
		t.Fatalf("system name changed unexpectedly: got %q want %q", updated.Name, "shop")
	}
	if updated.Description != "new description" {
		t.Fatalf("description = %q, want %q", updated.Description, "new description")
	}
}

func TestSystemHandler_UpdateSystem_ForbiddenWithoutSystemRole(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID,
		`{"description":"new description"}`,
		"user-no-role",
		[]string{"system:write"},
	)
	srv.UpdateSystem(c, sys.ID)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestSystemHandler_UpdateService_DescriptionOnly(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-1", "redis", sys.ID, "old")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID+"/services/"+svc.ID,
		`{"description":"service updated"}`,
		"owner-1",
		[]string{"service:create"},
	)
	srv.UpdateService(c, sys.ID, svc.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := client.Service.Get(c.Request.Context(), svc.ID)
	if err != nil {
		t.Fatalf("query updated service: %v", err)
	}
	if updated.Name != "redis" {
		t.Fatalf("service name changed unexpectedly: got %q want %q", updated.Name, "redis")
	}
	if updated.Description != "service updated" {
		t.Fatalf("service description = %q, want %q", updated.Description, "service updated")
	}
}

func TestSystemHandler_UpdateService_NotFoundWhenSystemMismatch(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sysA := mustCreateSystem(t, client, "sys-a", "shop", "owner-a")
	sysB := mustCreateSystem(t, client, "sys-b", "finance", "owner-b")
	svc := mustCreateService(t, client, "svc-1", "redis", sysB.ID, "old")
	mustCreateSystemBinding(t, client, "owner-a", sysA.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sysA.ID+"/services/"+svc.ID,
		`{"description":"service updated"}`,
		"owner-a",
		[]string{"service:create"},
	)
	srv.UpdateService(c, sysA.ID, svc.ID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSystemHandler_DeleteSystem_RequiresConfirmNameMatch(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c1, w1 := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID,
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c1, sys.ID, generated.DeleteSystemParams{})
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w1.Code, http.StatusBadRequest, w1.Body.String())
	}
	assertErrorCode(t, w1.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")

	c2, w2 := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=wrong",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c2, sys.ID, generated.DeleteSystemParams{ConfirmName: "wrong"})
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w2.Code, http.StatusBadRequest, w2.Body.String())
	}
	assertErrorCode(t, w2.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")
}

func TestSystemHandler_DeleteSystem_ConflictWhenServicesExist(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	_ = mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=shop",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c, sys.ID, generated.DeleteSystemParams{ConfirmName: "shop"})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SYSTEM_HAS_SERVICES")
}

func TestSystemHandler_DeleteSystem_Success(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=shop",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c, sys.ID, generated.DeleteSystemParams{ConfirmName: "shop"})
	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}

	if _, err := client.System.Get(t.Context(), sys.ID); !ent.IsNotFound(err) {
		t.Fatalf("system still exists after delete, err=%v", err)
	}
}

func TestSystemHandler_DeleteService_RequiresConfirmTrue(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID,
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")
}

func TestSystemHandler_DeleteService_ConflictWhenVMsExist(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateVMForService(t, client, "vm-del-1", "shop-redis-01", svc.ID)
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID+"?confirm=true",
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{Confirm: true})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SERVICE_HAS_VMS")
}

func TestSystemHandler_DeleteService_Success(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID+"?confirm=true",
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{Confirm: true})
	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}

	if _, err := client.Service.Get(t.Context(), svc.ID); !ent.IsNotFound(err) {
		t.Fatalf("service still exists after delete, err=%v", err)
	}
}

func newSystemBehaviorTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	client := testutil.OpenEntPostgres(t, "system_handler_behavior")
	return NewServer(ServerDeps{EntClient: client}), client
}

func newAuthedGinContext(
	t *testing.T,
	method string,
	target string,
	body string,
	userID string,
	permissions []string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var req *http.Request
	if strings.TrimSpace(body) == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	req = req.WithContext(middleware.SetUserContext(req.Context(), userID, userID, nil))
	c.Request = req
	c.Set("permissions", permissions)
	return c, w
}

func mustCreateSystem(t *testing.T, client *ent.Client, id, name, createdBy string) *ent.System {
	t.Helper()
	obj, err := client.System.Create().
		SetID(id).
		SetName(name).
		SetCreatedBy(createdBy).
		SetDescription("init").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	return obj
}

func mustCreateService(t *testing.T, client *ent.Client, id, name, systemID, description string) *ent.Service {
	t.Helper()
	obj, err := client.Service.Create().
		SetID(id).
		SetName(name).
		SetDescription(description).
		SetSystemID(systemID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return obj
}

func mustCreateVMForService(t *testing.T, client *ent.Client, id, name, serviceID string) *ent.VM {
	t.Helper()
	obj, err := client.VM.Create().
		SetID(id).
		SetName(name).
		SetInstance("01").
		SetNamespace("ns-test").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-1").
		SetServiceID(serviceID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return obj
}

func mustCreateSystemBinding(t *testing.T, client *ent.Client, userID, systemID, role string) {
	t.Helper()
	_, err := client.ResourceRoleBinding.Create().
		SetID(uuid.NewString()).
		SetUserID(userID).
		SetResourceType("system").
		SetResourceID(systemID).
		SetRole(rrb.Role(role)).
		SetCreatedBy("test-seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create resource role binding: %v", err)
	}
}
