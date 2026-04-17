package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestListVMs_RestrictsToVisibleSystemMembers(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_system_scope")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	userID := "user-" + uuid.NewString()
	mustCreateGlobalEnvRoleBinding(t, client, userID, []string{"vm:read"}, []string{"prod"})
	mustCreateProdNamespaceRegistryEntry(t, client, "team-prod")

	systemA := mustCreateSystem(t, client, "sys-a-"+uuid.NewString(), "payments", "owner-a")
	systemB := mustCreateSystem(t, client, "sys-b-"+uuid.NewString(), "finance", "owner-b")
	serviceA := mustCreateService(t, client, "svc-a-"+uuid.NewString(), "billing", systemA.ID, "seed")
	serviceB := mustCreateService(t, client, "svc-b-"+uuid.NewString(), "ledger", systemB.ID, "seed")
	mustCreateSystemBinding(t, client, userID, systemA.ID, "viewer")

	vmAID := "vm-a-" + uuid.NewString()
	vmBID := "vm-b-" + uuid.NewString()
	if _, err := client.VM.Create().
		SetID(vmAID).
		SetName("billing-vm").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-a").
		SetServiceID(serviceA.ID).
		Save(t.Context()); err != nil {
		t.Fatalf("create visible vm: %v", err)
	}
	if _, err := client.VM.Create().
		SetID(vmBID).
		SetName("ledger-vm").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-b").
		SetServiceID(serviceB.ID).
		Save(t.Context()); err != nil {
		t.Fatalf("create hidden vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", userID, []string{"vm:read"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != vmAID {
		t.Fatalf("visible vm id = %q, want %q", resp.Items[0].Id, vmAID)
	}
}

func TestGetVM_ReturnsNotFound_WhenSystemMembershipMissing(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_system_scope")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	userID := "user-" + uuid.NewString()
	mustCreateGlobalEnvRoleBinding(t, client, userID, []string{"vm:read"}, []string{"prod"})
	mustCreateProdNamespaceRegistryEntry(t, client, "team-prod")

	systemID := "sys-" + uuid.NewString()
	serviceID := "svc-" + uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	mustCreateSystem(t, client, systemID, "finance", "owner-b")
	mustCreateService(t, client, serviceID, "ledger", systemID, "seed")
	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("ledger-vm").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-b").
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", userID, []string{"vm:read"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_FOUND")
}

func TestCreateVMRequest_ReturnsNotFound_WhenServiceSystemMembershipMissing(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "create_vm_request_service_scope")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	userID := "user-" + uuid.NewString()
	mustCreateGlobalEnvRoleBinding(t, client, userID, []string{"vm:create"}, []string{"prod"})
	mustCreateProdNamespaceRegistryEntry(t, client, "team-prod")

	systemID := "sys-" + uuid.NewString()
	serviceID := uuid.NewString()
	mustCreateSystem(t, client, systemID, "payments", "owner-a")
	mustCreateService(t, client, serviceID, "billing", systemID, "seed")

	body := mustJSON(t, map[string]any{
		"service_id":       serviceID,
		"template_id":      uuid.NewString(),
		"instance_size_id": uuid.NewString(),
		"namespace":        "team-prod",
		"reason":           "need a vm",
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/request", body, userID, []string{"vm:create"})
	srv.CreateVMRequest(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SERVICE_NOT_FOUND")
}

func TestCreateVMModifyRequest_ReturnsNotFound_WhenSystemMembershipMissing(t *testing.T) {
	t.Parallel()

	srv, client, vmID := newVMModifyTestServer(t)

	userID := "user-" + uuid.NewString()
	mustCreateGlobalEnvRoleBinding(t, client, userID, []string{"vm:operate"}, []string{"prod"})
	mustCreateProdNamespaceRegistryEntry(t, client, "prod-ns")

	body := mustJSON(t, generated.VMModifyRequest{
		Reason:         "request resize",
		TargetMemoryGi: 8,
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/modify-request", body, userID, []string{"vm:operate"})
	srv.CreateVMModifyRequest(c, vmID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_FOUND")
}

func TestBatchHandler_SubmitVMBatch_DeleteReturnsNotFound_WhenSystemMembershipMissing(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)

	userID := "user-" + uuid.NewString()
	mustCreateGlobalEnvRoleBinding(t, client, userID, []string{"vm:delete"}, []string{"prod"})
	mustCreateProdNamespaceRegistryEntry(t, client, "prod-shop")

	vmID := mustCreateBatchDeleteTargetVM(t, client)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, userID, []string{"vm:delete"})
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_FOUND")
}

func mustCreateGlobalEnvRoleBinding(
	t *testing.T,
	client *ent.Client,
	userID string,
	permissions []string,
	allowedEnvs []string,
) {
	t.Helper()

	if _, err := client.User.Create().
		SetID(userID).
		SetUsername(userID).
		SetEmail(userID + "@example.com").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create user: %v", err)
	}

	roleID := "role-" + uuid.NewString()
	roleObj, err := client.Role.Create().
		SetID(roleID).
		SetName(roleID).
		SetPermissions(permissions).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	if _, err := client.RoleBinding.Create().
		SetID("rb-" + uuid.NewString()).
		SetScopeType("global").
		SetCreatedBy("test-seed").
		SetAllowedEnvironments(allowedEnvs).
		SetUserID(userID).
		SetRole(roleObj).
		Save(t.Context()); err != nil {
		t.Fatalf("create role binding: %v", err)
	}
}

func mustCreateProdNamespaceRegistryEntry(t *testing.T, client *ent.Client, name string) {
	t.Helper()

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(name).
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("test-seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}
}
