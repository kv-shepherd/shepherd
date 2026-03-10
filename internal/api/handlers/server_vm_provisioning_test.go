package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestGetVM_IncludesProvisioningStatus_WhenAvailable(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "get_vm_provisioning")
	_ = logger.Init("error", "json")

	clusterID := "cluster-" + uuid.NewString()
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)

	vmID := "vm-" + uuid.NewString()
	mustCreateVMWithCluster(t, client, vmID, clusterID, "prod-ns")

	mock := provider.NewMockProvider()
	mock.SeedDataVolumes([]*domain.DataVolume{
		{
			Name:         "vm" + vmID[len(vmID)-4:] + "-rootdisk",
			Namespace:    "prod-ns",
			UID:          "dv-uid-" + vmID[len(vmID)-6:],
			ClaimName:    "rootdisk-pvc",
			Phase:        "CloneInProgress",
			Progress:     "42.0%",
			RestartCount: 1,
		},
	})
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{
		{
			Name:                "rootdisk-pvc",
			Namespace:           "prod-ns",
			Phase:               "Bound",
			CloneType:           "copy",
			ClonePhase:          "Succeeded",
			CloneFallbackReason: "The volume modes of source and target are incompatible",
		},
	})
	mock.SeedEvents(domain.ObjectReference{
		Kind:      "DataVolume",
		Name:      "vm" + vmID[len(vmID)-4:] + "-rootdisk",
		Namespace: "prod-ns",
		UID:       "dv-uid-" + vmID[len(vmID)-6:],
	}, []domain.ProvisioningEvent{
		{
			Type:          "Warning",
			Reason:        "CloneSourceInUse",
			Message:       "source PVC is in use",
			LastObserved:  time.Now().UTC(),
			FirstObserved: time.Now().UTC(),
		},
	})

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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Provisioning == nil {
		t.Fatal("Provisioning = nil, want non-nil")
	}
	if resp.Provisioning.Phase != "CloneInProgress" {
		t.Fatalf("Provisioning.Phase = %q, want %q", resp.Provisioning.Phase, "CloneInProgress")
	}
	if resp.Provisioning.ClaimName != "rootdisk-pvc" {
		t.Fatalf("Provisioning.ClaimName = %q, want %q", resp.Provisioning.ClaimName, "rootdisk-pvc")
	}
	if resp.Provisioning.PvcPhase != "Bound" {
		t.Fatalf("Provisioning.PvcPhase = %q, want %q", resp.Provisioning.PvcPhase, "Bound")
	}
	if resp.Provisioning.CloneType != "copy" {
		t.Fatalf("Provisioning.CloneType = %q, want %q", resp.Provisioning.CloneType, "copy")
	}
	if resp.Provisioning.CloneFallbackReason == "" {
		t.Fatal("Provisioning.CloneFallbackReason = empty, want non-empty")
	}
	if len(resp.Provisioning.RecentEvents) != 1 {
		t.Fatalf("len(Provisioning.RecentEvents) = %d, want 1", len(resp.Provisioning.RecentEvents))
	}
}
