package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type capturingModifyProvider struct {
	*provider.MockProvider
	lastMutationPayload string
}

type failingModifyProvider struct {
	*provider.MockProvider
	updateErr error
}

func (p *capturingModifyProvider) ExecuteVMMutation(ctx context.Context, clusterID, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error) {
	updated, err := p.GetVM(ctx, clusterID, namespace, name)
	if err != nil {
		return nil, err
	}
	p.lastMutationPayload = string(mutation.Payload)
	updated.Spec.MemoryGi = 8
	updated.ResourceVersion = "rv-modify-1"
	updated.Status = domain.VMStatusRunning
	return updated, nil
}

func (p *failingModifyProvider) ExecuteVMMutation(ctx context.Context, clusterID, namespace, name string, mutation *domain.VMMutation) (*domain.VM, error) {
	_, _, _ = ctx, mutation, clusterID
	return nil, fmt.Errorf("apply update %s/%s on %s: %w", namespace, name, clusterID, p.updateErr)
}

func TestVMModifyWorker_ExecutesLiveMemoryUpdateAndPersistsStatus(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_live_update")
	ctx := t.Context()

	system, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	svc, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(system).
		Save(ctx)
	require.NoError(t, err)

	clusterID := "cluster-" + uuid.NewString()
	_, err = client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(cluster.EnvironmentProd).
		SetStatus(cluster.StatusHEALTHY).
		SetEnabled(true).
		SetEnabledFeatures([]string{"VMLiveUpdateFeatures"}).
		Save(ctx)
	require.NoError(t, err)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm-" + uuid.NewString()[:8]
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID(clusterID).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMModifyPayload{
		VMID:            vmID,
		VMName:          vmName,
		ClusterID:       clusterID,
		Namespace:       "prod-ns",
		Actor:           "seed",
		CurrentCPUCores: 2,
		CurrentMemoryGi: 4,
		TargetMemoryGi:  ptrFloat64(8),
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMModifyRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	mock := &capturingModifyProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   clusterID,
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
		ResourceVersion: "rv-before-1",
	}})

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: eventID},
	})
	require.NoError(t, err)
	require.Contains(t, mock.lastMutationPayload, "\"guest\":\"8Gi\"")

	stored, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, stored.Status)
	require.Equal(t, entvm.PollingTierLow, stored.PollingTier)
	require.Equal(t, lowTierIntervalSec, stored.PollIntervalSec)
	require.NotNil(t, stored.LastK8sRv)
	require.Equal(t, "rv-modify-1", *stored.LastK8sRv)
	require.NotNil(t, stored.LastPolledAt)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
}

func TestVMModifyWorker_PersistsFailureReasonOnTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_failure_reason")
	ctx := t.Context()

	system, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	svc, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(system).
		Save(ctx)
	require.NoError(t, err)

	clusterID := "cluster-" + uuid.NewString()
	_, err = client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(cluster.EnvironmentProd).
		SetStatus(cluster.StatusHEALTHY).
		SetEnabled(true).
		SetEnabledFeatures([]string{"VMLiveUpdateFeatures"}).
		Save(ctx)
	require.NoError(t, err)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm-" + uuid.NewString()[:8]
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID(clusterID).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMModifyPayload{
		VMID:            vmID,
		VMName:          vmName,
		ClusterID:       clusterID,
		Namespace:       "prod-ns",
		Actor:           "seed",
		CurrentCPUCores: 2,
		CurrentMemoryGi: 4,
		TargetMemoryGi:  ptrFloat64(8),
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMModifyRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType("MODIFY").
		SetStatus("APPROVED").
		SetRequester("seed").
		Save(ctx)
	require.NoError(t, err)

	mock := &failingModifyProvider{
		MockProvider: provider.NewMockProvider(),
		updateErr:    fmt.Errorf("kubevirt api rejected the patch"),
	}
	mock.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   clusterID,
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
		ResourceVersion: "rv-before-1",
	}})

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: eventID},
	})
	require.Error(t, err)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)
	require.Contains(t, ticket.RejectReason, "kubevirt api rejected the patch")
}

func TestVMModifyWorker_PrefersApprovedMutationSnapshot(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_snapshot_payload")
	ctx := t.Context()

	system, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	svc, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(system).
		Save(ctx)
	require.NoError(t, err)

	clusterID := "cluster-" + uuid.NewString()
	_, err = client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(cluster.EnvironmentProd).
		SetStatus(cluster.StatusHEALTHY).
		SetEnabled(true).
		SetEnabledFeatures([]string{"VMLiveUpdateFeatures"}).
		Save(ctx)
	require.NoError(t, err)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm-" + uuid.NewString()[:8]
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID(clusterID).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMModifyPayload{
		VMID:            vmID,
		VMName:          vmName,
		ClusterID:       clusterID,
		Namespace:       "prod-ns",
		Actor:           "seed",
		CurrentCPUCores: 2,
		CurrentMemoryGi: 4,
		TargetMemoryGi:  ptrFloat64(8),
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMModifyRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetModifiedSpec(map[string]interface{}{
			"vm_mutation": map[string]interface{}{
				"mode":       domain.VMMutationModePatch,
				"patch_type": domain.VMMutationPatchTypeMerge,
				"payload":    `{"spec":{"template":{"spec":{"domain":{"memory":{"guest":"6Gi"},"resources":{"limits":{"memory":"6Gi"}}}}}}}`,
			},
			"apply_mode":       "restart_required",
			"requires_restart": true,
		}).
		Save(ctx)
	require.NoError(t, err)

	mock := &capturingModifyProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   clusterID,
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
		ResourceVersion: "rv-before-1",
	}})

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: eventID},
	})
	require.NoError(t, err)
	require.Contains(t, mock.lastMutationPayload, "\"guest\":\"6Gi\"")
	require.NotContains(t, mock.lastMutationPayload, "\"guest\":\"8Gi\"")
}

func ptrFloat64(v float64) *float64 {
	return &v
}
