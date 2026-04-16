package jobs

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestVMDeleteWorker_RejectsRunningVMAtExecutionTime(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_runtime_guard")
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

	payloadBytes, err := domain.VMDeletePayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
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
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   "cluster-a",
		Status:    domain.VMStatusRunning,
	}})

	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(context.Background(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be STOPPED, FAILED, NOT_FOUND, UNKNOWN, or DELETING")

	storedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(eventID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	_, err = mock.GetVM(ctx, "cluster-a", "prod-ns", vmName)
	require.NoError(t, err)
}

func TestVMDeleteWorker_SkipsK8sDeleteWhenStatusNotFound(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_not_found_skip")
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
		SetStatus(entvm.StatusNOT_FOUND).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMDeletePayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
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
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := provider.NewMockProvider()
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(context.Background(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.NoError(t, err)

	_, err = client.VM.Get(ctx, vmID)
	require.True(t, ent.IsNotFound(err))

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(eventID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMDeleteWorker_AttemptsK8sDeleteWhenStatusUnknown(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_unknown_skip")
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
		SetStatus(entvm.StatusUNKNOWN).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMDeletePayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
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
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   "cluster-a",
		Status:    domain.VMStatusUnknown,
	}})
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(context.Background(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.NoError(t, err)

	_, err = client.VM.Get(ctx, vmID)
	require.True(t, ent.IsNotFound(err))

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(eventID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	_, err = mock.GetVM(ctx, "cluster-a", "prod-ns", vmName)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestVMDeleteExecutableStatus(t *testing.T) {
	t.Parallel()

	allowed := []entvm.Status{
		entvm.StatusSTOPPED,
		entvm.StatusFAILED,
		entvm.StatusNOT_FOUND,
		entvm.StatusUNKNOWN,
		entvm.StatusDELETING,
	}

	for _, status := range allowed {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			require.True(t, vmDeleteExecutableStatus(status))
		})
	}
}

func TestVMDeleteWorker_AllowsDeletingStatusAtExecutionTime(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_deleting_allowed")
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
		SetStatus(entvm.StatusDELETING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	payloadBytes, err := domain.VMDeletePayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
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
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      vmName,
		Namespace: "prod-ns",
		Cluster:   "cluster-a",
		Status:    domain.VMStatusStopped,
	}})

	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(context.Background(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.NoError(t, err)

	_, err = client.VM.Get(ctx, vmID)
	require.True(t, ent.IsNotFound(err))

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(eventID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestShouldSkipK8sDelete(t *testing.T) {
	t.Parallel()

	require.True(t, shouldSkipK8sDelete(entvm.StatusNOT_FOUND))

	nonSkippable := []entvm.Status{
		entvm.StatusSTOPPED,
		entvm.StatusFAILED,
		entvm.StatusUNKNOWN,
		entvm.StatusDELETING,
	}

	for _, status := range nonSkippable {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			require.False(t, shouldSkipK8sDelete(status))
		})
	}
}
