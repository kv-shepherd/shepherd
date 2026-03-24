package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestListApprovals_CREATE_ProvisioningIncludedWhenVMExists(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "approval_create_provisioning")
	_ = logger.Init("error", "json")

	eventID := "ev-" + uuid.NewString()
	payload := map[string]interface{}{
		"service_id": "svc-" + uuid.NewString(),
		"namespace":  "team-prod",
		"reason":     "need a VM",
	}
	rawPayload, _ := json.Marshal(payload)
	mustCreateDomainEvent(t, client, eventID, rawPayload)

	ticketID := "ticket-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-a")

	vmID := "vm-" + uuid.NewString()
	vmName := "vm" + vmID[len(vmID)-4:]
	dvUID := "dv-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "user-a")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("user-a").
		SetClusterID("cluster-a").
		SetTicketID(ticketID).
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.SeedDataVolumes([]*domain.DataVolume{
		{
			Name:         vmName + "-rootfs",
			Namespace:    "team-prod",
			UID:          dvUID,
			ClaimName:    vmName + "-rootfs",
			Phase:        "ImportInProgress",
			Progress:     "55.0%",
			RestartCount: 0,
		},
	})
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{
		{
			Name:                vmName + "-rootfs",
			Namespace:           "team-prod",
			Phase:               "Bound",
			CloneType:           "copy",
			ClonePhase:          "Succeeded",
			CloneFallbackReason: "The volume modes of source and target are incompatible",
		},
	})
	mock.SeedEvents(domain.ObjectReference{
		Kind:      "DataVolume",
		Name:      vmName + "-rootfs",
		Namespace: "team-prod",
		UID:       dvUID,
	}, []domain.ProvisioningEvent{
		{
			Type:          "Normal",
			Reason:        "ImportScheduled",
			Message:       "import scheduled",
			LastObserved:  time.Now().UTC(),
			FirstObserved: time.Now().UTC(),
		},
	})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.Provisioning == nil {
		t.Fatal("Provisioning = nil, want non-nil")
	}
	if found.Provisioning.Phase != "ImportInProgress" {
		t.Fatalf("Provisioning.Phase = %q, want %q", found.Provisioning.Phase, "ImportInProgress")
	}
	if found.Provisioning.ClaimName != vmName+"-rootfs" {
		t.Fatalf("Provisioning.ClaimName = %q, want %q", found.Provisioning.ClaimName, vmName+"-rootfs")
	}
	if found.Provisioning.CloneType != "copy" {
		t.Fatalf("Provisioning.CloneType = %q, want %q", found.Provisioning.CloneType, "copy")
	}
}
