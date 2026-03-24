package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestGetVMModifyContext_ReturnsLiveCapabilities(t *testing.T) {
	t.Parallel()

	srv, _, vmID := newVMModifyTestServer(t)

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/"+vmID+"/modify-context", "", "owner-1", []string{"vm:operate", "platform:admin"})
	srv.GetVMModifyContext(c, vmID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMModifyContext
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.CpuSupported || !resp.MemorySupported || !resp.DiskSupported {
		t.Fatalf("expected all live-update flags true, got cpu=%v memory=%v disk=%v", resp.CpuSupported, resp.MemorySupported, resp.DiskSupported)
	}
	if resp.CurrentCpuCores != 2 || resp.CurrentMemoryGi != 4 || resp.CurrentDiskGb != 20 {
		t.Fatalf("unexpected current resources: cpu=%v mem=%v disk=%d", resp.CurrentCpuCores, resp.CurrentMemoryGi, resp.CurrentDiskGb)
	}
	if resp.ClusterName == "" {
		t.Fatal("cluster_name is empty")
	}
}

func TestCreateVMModifyRequest_CreatesPendingModifyTicket(t *testing.T) {
	t.Parallel()

	srv, client, vmID := newVMModifyTestServer(t)

	body := mustJSON(t, generated.VMModifyRequest{
		Reason:         "scale resources",
		TargetMemoryGi: 8,
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/modify-request", body, "owner-1", []string{"vm:operate", "platform:admin"})
	srv.CreateVMModifyRequest(c, vmID)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.TicketResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.OperationType != generated.TicketResponseOperationTypeMODIFY {
		t.Fatalf("operation_type = %q, want %q", resp.OperationType, generated.TicketResponseOperationTypeMODIFY)
	}

	ticket, err := client.Ticket.Get(t.Context(), resp.TicketId)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.OperationType != entticket.OperationTypeMODIFY {
		t.Fatalf("ticket operation_type = %q, want %q", ticket.OperationType, entticket.OperationTypeMODIFY)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want %q", ticket.Status, entticket.StatusPENDING)
	}

	event, err := client.DomainEvent.Get(t.Context(), ticket.EventID)
	if err != nil {
		t.Fatalf("query domain event: %v", err)
	}
	if event.EventType != string(domain.EventVMModifyRequested) {
		t.Fatalf("event_type = %q, want %q", event.EventType, domain.EventVMModifyRequested)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusPENDING)
	}

	var payload domain.VMModifyPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	if payload.TargetMemoryGi == nil || *payload.TargetMemoryGi != 8 {
		t.Fatalf("target_memory_gi = %v, want 8", payload.TargetMemoryGi)
	}
	if payload.VMID != vmID {
		t.Fatalf("payload vm_id = %q, want %q", payload.VMID, vmID)
	}
}

func newVMModifyTestServer(t *testing.T) (*Server, *ent.Client, string) {
	t.Helper()

	client := testutil.OpenEntPostgres(t, "server_vm_modify")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	_, err := client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetDisplayName("Cluster Modify").
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(entcluster.EnvironmentProd).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(true).
		SetEnabledFeatures([]string{"VMLiveUpdateFeatures", "ExpandDisks"}).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	serviceID := mustCreateServiceForVM(t, client, "owner-1")
	vmID := "vm-" + uuid.NewString()
	vmName := "vm" + vmID[len(vmID)-4:]
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-1").
		SetClusterID(clusterID).
		SetServiceID(serviceID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   clusterID,
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			DiskGB:                   20,
			RootDataVolumeName:       "rootdisk",
			DiskHotplugSupported:     true,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
		ResourceVersion: "rv-modify-1",
	}})

	return NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	}), client, vmID
}
