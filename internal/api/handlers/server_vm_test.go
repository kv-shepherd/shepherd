package handlers

// Tests for ListVMs and GetVM covering the cluster-environment resolution logic
// introduced per ADR-0015 §15.
//
// Coverage matrix:
//   - vmToAPI: environment populated when clusterEnv is "prod" or "test"
//   - vmToAPI: environment left empty when clusterEnv is blank
//   - ListVMs: batch-fetches cluster environments and maps each VM correctly
//   - ListVMs: VMs with no ClusterID get an empty environment
//   - ListVMs: multiple VMs spanning different environments are each resolved
//   - GetVM: single-VM path fetches cluster environment
//   - GetVM: no ClusterID → environment empty (non-fatal path)
//   - GetVM: returns 404 for unknown VM ID

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type failingVMLiveStatusProvider struct {
	*provider.MockProvider
	err error
}

func (p *failingVMLiveStatusProvider) GetVM(ctx context.Context, _, namespace, name string) (*domain.VM, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.MockProvider.GetVM(ctx, "", namespace, name)
}

func (p *failingVMLiveStatusProvider) ListVMs(ctx context.Context, _, namespace string, _ provider.ListOptions) (*domain.VMList, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.MockProvider.ListVMs(ctx, "", namespace, provider.ListOptions{})
}

// ---- Pure unit tests for vmToAPI converter ------------------------------------------------

// minimalEntVM returns a minimal *ent.VM for unit tests that do not hit the DB.
func minimalEntVM(t *testing.T) *ent.VM {
	t.Helper()
	return &ent.VM{
		ID:        "vm-" + uuid.NewString(),
		Name:      "test-vm",
		Namespace: "ns-test",
		Status:    entvm.StatusRUNNING,
		Instance:  "01",
		CreatedBy: "user-a",
		CreatedAt: time.Now(),
	}
}

func TestVmToAPI_Environment_Prod(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-a"
	got := vmToAPI(vm, "prod", "", nil)

	if got.Environment != generated.VMEnvironmentProd {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentProd)
	}
}

func TestVmToAPI_Environment_Test(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-b"
	got := vmToAPI(vm, "test", "", nil)

	if got.Environment != generated.VMEnvironmentTest {
		t.Fatalf("environment = %q, want %q", got.Environment, generated.VMEnvironmentTest)
	}
}

func TestVmToAPI_Environment_Empty_WhenClusterEnvBlank(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	// No clusterEnv provided → environment must remain zero value.
	got := vmToAPI(vm, "", "", nil)

	if got.Environment != "" {
		t.Fatalf("environment = %q, want empty string (no cluster env)", got.Environment)
	}
}

func TestVmToAPI_PreservesCoreFields(t *testing.T) {
	t.Parallel()

	vm := minimalEntVM(t)
	vm.ClusterID = "cluster-core"
	got := vmToAPI(vm, "prod", "", nil)

	if got.Id != vm.ID {
		t.Fatalf("id = %q, want %q", got.Id, vm.ID)
	}
	if got.Name != vm.Name {
		t.Fatalf("name = %q, want %q", got.Name, vm.Name)
	}
	if got.Namespace != vm.Namespace {
		t.Fatalf("namespace = %q, want %q", got.Namespace, vm.Namespace)
	}
	if got.ClusterId != vm.ClusterID {
		t.Fatalf("cluster_id = %q, want %q", got.ClusterId, vm.ClusterID)
	}
}

// ---- Integration: ListVMs ----------------------------------------------------------------

func TestListVMs_PopulatesEnvironmentFromCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_env_prod")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-a", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", found.Environment, generated.VMEnvironmentProd)
	}
	if found.ClusterName == "" {
		t.Fatal("vm.cluster_name is empty, want cluster display label fallback")
	}
}

func TestListVMs_EmptyEnvironment_WhenNoClusterID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_no_cluster")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	vmID := "vm-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "user-b")
	// Create VM with no ClusterID set.
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-nocluster" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("test-ns").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("user-b").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-b", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Environment != "" {
		t.Fatalf("vm.environment = %q, want empty (no cluster)", found.Environment)
	}
}

func TestListVMs_MultipleVMs_DifferentEnvironments(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_multi_env")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterTestID := "cluster-t-" + uuid.NewString()
	clusterProdID := "cluster-p-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterTestID, entcluster.EnvironmentTest)
	mustCreateClusterWithEnv(t, client, clusterProdID, entcluster.EnvironmentProd)

	vmTestID := "vm-t-" + uuid.NewString()
	vmProdID := "vm-p-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmTestID, clusterTestID, "test-ns")
	mustCreateVMWithCluster(t, client, vmProdID, clusterProdID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-c", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	foundTest := findVMInList(t, resp.Items, vmTestID)
	foundProd := findVMInList(t, resp.Items, vmProdID)

	if foundTest.Environment != generated.VMEnvironmentTest {
		t.Fatalf("vm-test environment = %q, want %q", foundTest.Environment, generated.VMEnvironmentTest)
	}
	if foundProd.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm-prod environment = %q, want %q", foundProd.Environment, generated.VMEnvironmentProd)
	}
}

func TestListVMs_RefreshesStatusFromLiveCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_live_status")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm" + vmID[len(vmID)-4:],
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-1",
	}})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-live", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Status != generated.VMStatusSTOPPED {
		t.Fatalf("vm.status = %q, want %q", found.Status, generated.VMStatusSTOPPED)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusSTOPPED {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusSTOPPED)
	}
}

func TestListVMs_MarksUnknownWhenClusterIsUnavailable(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "list_vms_unavailable_cluster")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	failing := &failingVMLiveStatusProvider{
		MockProvider: provider.NewMockProvider(),
		err:          fmt.Errorf("cluster health check failed: unreachable"),
	}

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(failing),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms", "", "user-unavailable", []string{"vm:read", "platform:admin"})
	srv.ListVMs(c, generated.ListVMsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMList
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	found := findVMInList(t, resp.Items, vmID)
	if found.Status != generated.VMStatusUNKNOWN {
		t.Fatalf("vm.status = %q, want %q", found.Status, generated.VMStatusUNKNOWN)
	}
}

// ---- Integration: GetVM ------------------------------------------------------------------

func TestGetVM_PopulatesEnvironmentFromCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_env_prod")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Environment != generated.VMEnvironmentProd {
		t.Fatalf("vm.environment = %q, want %q", resp.Environment, generated.VMEnvironmentProd)
	}
	if resp.ClusterName == "" {
		t.Fatal("vm.cluster_name is empty, want cluster display label fallback")
	}
}

func TestGetVM_EmptyEnvironment_WhenNoClusterID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_no_cluster")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	vmID := "vm-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "user-a")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-nocluster" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("test-ns").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("user-a").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Environment != "" {
		t.Fatalf("vm.environment = %q, want empty (no cluster)", resp.Environment)
	}
}

func TestGetVM_RefreshesStatusFromLiveCluster(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_live_status")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm" + vmID[len(vmID)-4:],
		Namespace:       "prod-ns",
		Cluster:         clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-live-2",
	}})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.VMStatusSTOPPED {
		t.Fatalf("vm.status = %q, want %q", resp.Status, generated.VMStatusSTOPPED)
	}
}

func TestGetVM_MarksUnknownWhenLiveVMIsMissing(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_missing_live_object")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")
	_, err := client.VM.UpdateOneID(vmID).
		SetPollingTier(entvm.PollingTierLow).
		SetPollIntervalSec(1800).
		Save(t.Context())
	if err != nil {
		t.Fatalf("stabilize vm polling tier: %v", err)
	}

	mock := provider.NewMockProvider()
	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID, "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VM
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.Status != generated.VMStatusUNKNOWN {
		t.Fatalf("vm.status = %q, want %q", resp.Status, generated.VMStatusUNKNOWN)
	}

	stored, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("reload vm: %v", err)
	}
	if stored.Status != entvm.StatusUNKNOWN {
		t.Fatalf("stored status = %q, want %q", stored.Status, entvm.StatusUNKNOWN)
	}
}

func TestGetVMRequestPrefill_ReturnsReusableCreateContext(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_request_prefill_success")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	systemID := uuid.NewString()
	serviceID := uuid.NewString()
	templateID := uuid.NewString()
	instanceSizeID := uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	eventID := "event-" + uuid.NewString()

	mustCreateSystem(t, client, systemID, "shop-prefill", "user-a")
	mustCreateService(t, client, serviceID, "redis-prefill", systemID, "seed")

	payload, err := json.Marshal(map[string]interface{}{
		"service_id":       serviceID,
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
		"namespace":        "team-prod",
		"reason":           "scale from existing vm",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("VM_CREATION_REQUESTED").
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payload).
		SetCreatedBy("user-a").
		Save(t.Context()); err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-a").
		SetReason("scale from existing vm").
		Save(t.Context()); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-prefill").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetTicketID(ticketID).
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/request-prefill", "", "user-a", []string{"vm:create", "platform:admin"})
	srv.GetVMRequestPrefill(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMRequestPrefill
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if resp.SystemId.String() != systemID {
		t.Fatalf("system_id = %q, want %q", resp.SystemId.String(), systemID)
	}
	if resp.ServiceId.String() != serviceID {
		t.Fatalf("service_id = %q, want %q", resp.ServiceId.String(), serviceID)
	}
	if resp.TemplateId.String() != templateID {
		t.Fatalf("template_id = %q, want %q", resp.TemplateId.String(), templateID)
	}
	if resp.InstanceSizeId.String() != instanceSizeID {
		t.Fatalf("instance_size_id = %q, want %q", resp.InstanceSizeId.String(), instanceSizeID)
	}
	if resp.Namespace != "team-prod" {
		t.Fatalf("namespace = %q, want %q", resp.Namespace, "team-prod")
	}
	if resp.Reason != "scale from existing vm" {
		t.Fatalf("reason = %q, want %q", resp.Reason, "scale from existing vm")
	}
	if resp.BatchCount != 1 {
		t.Fatalf("batch_count = %d, want 1", resp.BatchCount)
	}
}

func TestGetVMRequestPrefill_ConflictWhenPayloadIsNotReusable(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_request_prefill_conflict")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	systemID := uuid.NewString()
	serviceID := uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	eventID := "event-" + uuid.NewString()

	mustCreateSystem(t, client, systemID, "shop-conflict", "user-a")
	mustCreateService(t, client, serviceID, "redis-conflict", systemID, "seed")

	payload, err := json.Marshal(map[string]interface{}{
		"service_id": serviceID,
		"namespace":  "team-prod",
		"reason":     "missing template and size",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("VM_CREATION_REQUESTED").
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payload).
		SetCreatedBy("user-a").
		Save(t.Context()); err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-a").
		SetReason("missing template and size").
		Save(t.Context()); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-prefill-missing-shape").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetTicketID(ticketID).
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/request-prefill", "", "user-a", []string{"vm:create", "platform:admin"})
	srv.GetVMRequestPrefill(c, vmID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp generated.Error
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("decode error response: %v", decodeErr)
	}
	if resp.Code != "VM_REQUEST_PREFILL_UNAVAILABLE" {
		t.Fatalf("error.code = %q, want %q", resp.Code, "VM_REQUEST_PREFILL_UNAVAILABLE")
	}
}

func TestGetVM_ReturnsNotFound_ForUnknownID(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_not_found")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{EntClient: client})

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/nonexistent-id", "", "user-a", []string{"vm:read", "platform:admin"})
	srv.GetVM(c, "nonexistent-id")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_FOUND")
}

// ---- local seed helpers ------------------------------------------------------------------

func mustCreateClusterWithEnv(t *testing.T, client *ent.Client, id string, env entcluster.Environment) {
	t.Helper()
	_, err := client.Cluster.Create().
		SetID(id).
		SetName("cl" + id[len(id)-4:]).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("test-seed").
		SetEnvironment(env).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create cluster %s: %v", id, err)
	}
}

func mustCreateVMWithCluster(t *testing.T, client *ent.Client, vmID, clusterID, namespace string) {
	t.Helper()
	svcID := mustCreateServiceForVM(t, client, "user-a")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace(namespace).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetClusterID(clusterID).
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm %s: %v", vmID, err)
	}
}

// mustCreateServiceForVM creates the minimal System + Service required by the VM foreign key.
func mustCreateServiceForVM(t *testing.T, client *ent.Client, actor string) string {
	t.Helper()
	sysID := "sys-" + uuid.NewString()
	svcID := "svc-" + uuid.NewString()
	_ = mustCreateSystem(t, client, sysID, "shop"+sysID[len(sysID)-4:], actor)
	mustCreateService(t, client, svcID, "redis"+svcID[len(svcID)-4:], sysID, "seed")
	return svcID
}

// findVMInList locates a VM by ID from the list, fatally fails if not found.
func findVMInList(t *testing.T, items []generated.VM, vmID string) generated.VM {
	t.Helper()
	for i := range items {
		item := &items[i]
		if item.Id == vmID {
			return *item
		}
	}
	t.Fatalf("VM %q not found in list response", vmID)
	return generated.VM{}
}
