package jobs

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent/domainevent"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type transitionalPowerProvider struct {
	*provider.MockProvider
}

func (p *transitionalPowerProvider) StartVM(ctx context.Context, cluster, namespace, name string) error {
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStarting
	vm.ResourceVersion = "rv-start-1"
	return nil
}

func (p *transitionalPowerProvider) StopVM(ctx context.Context, cluster, namespace, name string) error {
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStopping
	vm.ResourceVersion = "rv-stop-1"
	return nil
}

func (p *transitionalPowerProvider) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStopping
	vm.ResourceVersion = "rv-restart-1"
	return nil
}

func TestFallbackPowerOperationStatus_ReturnsTransitionalStates(t *testing.T) {
	t.Parallel()

	require.Equal(t, entvm.StatusSTARTING, fallbackPowerOperationStatus(powerOpStart))
	require.Equal(t, entvm.StatusSTOPPING, fallbackPowerOperationStatus(powerOpStop))
	require.Equal(t, entvm.StatusSTOPPING, fallbackPowerOperationStatus(powerOpRestart))
}

func TestVMPowerWorker_UsesObservedLiveStatusAfterStop(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_power_live_status")
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

	vmID := "vm-" + uuid.NewString()
	vmName := "vm-" + uuid.NewString()[:8]
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID("cluster-a").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMPowerPayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Operation: powerOpStop,
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMStopRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmName,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})

	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID:   eventID,
			Operation: powerOpStop,
		},
	})
	require.NoError(t, err)

	stored, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusSTOPPING, stored.Status)
	require.Equal(t, entvm.PollingTierHigh, stored.PollingTier)
	require.Equal(t, highTierIntervalSec, stored.PollIntervalSec)
	require.NotNil(t, stored.LastK8sRv)
	require.Equal(t, "rv-stop-1", *stored.LastK8sRv)
	require.NotNil(t, stored.LastPolledAt)
	require.NotNil(t, stored.HighTierSince)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
}
