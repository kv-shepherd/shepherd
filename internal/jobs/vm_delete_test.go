package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type failingDeleteProvider struct {
	*provider.MockProvider
	deleteErr   error
	deleteCalls int
}

type vmDeleteWorkerFixture struct {
	eventID  string
	ticketID string
	vmID     string
	vmName   string
}

func (p *failingDeleteProvider) DeleteVM(_ context.Context, _, _, _ string) error {
	p.deleteCalls++
	return p.deleteErr
}

func seedVMDeleteWorkerFixture(t *testing.T, dbName string) (*ent.Client, vmDeleteWorkerFixture, *provider.MockProvider) {
	t.Helper()

	client := testutil.OpenEntPostgres(t, dbName+"_"+uuid.NewString()[:8])
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
		SetStatus(entvm.StatusSTOPPED).
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

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
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

	return client, vmDeleteWorkerFixture{
		eventID:  eventID,
		ticketID: ticketID,
		vmID:     vmID,
		vmName:   vmName,
	}, mock
}

func TestVMDeleteArgs(t *testing.T) {
	t.Parallel()

	var args VMDeleteArgs
	if got := args.Kind(); got != "vm_delete" {
		t.Fatalf("Kind() = %q, want vm_delete", got)
	}
	opts := args.InsertOpts()
	if opts.Queue != "vm_operations" {
		t.Fatalf("InsertOpts().Queue = %q, want vm_operations", opts.Queue)
	}
	if opts.MaxAttempts != 3 {
		t.Fatalf("InsertOpts().MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs || !opts.UniqueOpts.ByQueue {
		t.Fatalf("InsertOpts().UniqueOpts = %+v, want ByArgs and ByQueue", opts.UniqueOpts)
	}
}

func TestNewVMDeleteWorker(t *testing.T) {
	t.Parallel()

	worker := NewVMDeleteWorker(nil, nil, nil)
	if worker == nil {
		t.Fatal("NewVMDeleteWorker() = nil")
	}
	if worker.entClient != nil || worker.vmService != nil || worker.auditLogger != nil {
		t.Fatalf("NewVMDeleteWorker(nil, nil, nil) dependencies = %#v/%#v/%#v, want nils", worker.entClient, worker.vmService, worker.auditLogger)
	}
}

func TestVMDeleteWorker_SkipsCompletedEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_completed_skip")
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
		SetStatus(entvm.StatusSTOPPED).
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
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	infra := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    fmt.Errorf("delete should not be called"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(ctx, &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.NoError(t, err)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	stored, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusSTOPPED, stored.Status)
}

func TestVMDeleteWorker_SkipsFailedEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_failed_skip")
	ctx := t.Context()
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusFAILED).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(ctx)
	require.NoError(t, err)

	infra := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    fmt.Errorf("delete should not be called"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(ctx, &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	stored, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusSTOPPED, stored.Status)
}

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

	tests := []struct {
		status entvm.Status
		want   bool
	}{
		{status: entvm.StatusSTOPPED, want: true},
		{status: entvm.StatusFAILED, want: true},
		{status: entvm.StatusNOT_FOUND, want: true},
		{status: entvm.StatusUNKNOWN, want: true},
		{status: entvm.StatusDELETING, want: true},
		{status: entvm.StatusRUNNING, want: false},
		{status: entvm.StatusSTARTING, want: false},
		{status: entvm.StatusSTOPPING, want: false},
		{status: entvm.StatusPENDING, want: false},
		{status: entvm.StatusCREATING, want: false},
		{status: entvm.StatusMIGRATING, want: false},
		{status: entvm.StatusPAUSED, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, vmDeleteExecutableStatus(tc.status))
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

func TestVMDeleteWorker_DoesNotDeleteWhenStatusBecomesNonExecutableBeforeClaim(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_stale_claim_running")
	injectVMStatusBeforeNextJobsUpdate(t, client, fixture.vmID, entvm.StatusRUNNING)
	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    errors.New("provider delete must not execute after stale claim"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Contains(t, err.Error(), "status changed from STOPPED to RUNNING")
	require.Zero(t, mock.deleteCalls)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)
}

func TestVMDeleteWorker_HardDeleteFailureCompletesWithTombstone(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, mock := seedVMDeleteWorkerFixture(t, "vm_delete_hard_delete_fallback")
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.New("hard delete blocked by dependent row")),
		ent.OpDeleteOne,
	))
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	_, err = mock.GetVM(t.Context(), "cluster-a", "prod-ns", fixture.vmName)
	require.Error(t, err)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMDeleteWorker_DeletingStatusPersistFailureDoesNotExecuteProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_deleting_status_persist_failed")
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.New("deleting status persist unavailable")),
		ent.OpUpdateOne,
	))
	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    errors.New("provider delete should not execute"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist DELETING status for delete event")
	require.Contains(t, err.Error(), "deleting status persist unavailable")
	require.Zero(t, mock.deleteCalls)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusSTOPPED, storedVM.Status)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}

func TestVMDeleteWorker_EventCompletePersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, mock := seedVMDeleteWorkerFixture(t, "vm_delete_event_complete_persist_failed")
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusCOMPLETED, errors.New("delete event complete persist failed")),
		ent.OpUpdate,
	))
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed delete state for event")
	require.Contains(t, err.Error(), "delete event complete persist failed")

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	_, err = mock.GetVM(t.Context(), "cluster-a", "prod-ns", fixture.vmName)
	require.Error(t, err)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}

func TestVMDeleteWorker_TicketSuccessPersistFailureRollsBackEventCompletion(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, mock := seedVMDeleteWorkerFixture(t, "vm_delete_ticket_success_persist_failed")
	updateCount := 0
	client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.New("delete ticket success persist failed")
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed delete state for event")
	require.Contains(t, err.Error(), "delete ticket success persist failed")

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	_, err = mock.GetVM(t.Context(), "cluster-a", "prod-ns", fixture.vmName)
	require.Error(t, err)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}

func TestVMDeleteWorker_EventCompletePersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, mock := seedVMDeleteWorkerFixture(t, "vm_delete_event_complete_persist_canceled")
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusCOMPLETED, errors.Join(errors.New("delete event complete persist interrupted"), context.Canceled)),
		ent.OpUpdate,
	))
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	_, err = mock.GetVM(t.Context(), "cluster-a", "prod-ns", fixture.vmName)
	require.Error(t, err)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
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

func TestVMDeleteWorker_SnoozesTransientClusterErrorsWithoutFailingTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_transient_cluster_error")
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
		SetStatus(entvm.StatusSTOPPED).
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

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    context.DeadlineExceeded,
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(ctx, &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, clusterRuntimeUnavailableSnoozeDuration, snoozeErr.Duration)

	storedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}

func TestVMDeleteWorker_RetryableDeleteFailureDoesNotPersistTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_retryable_failure")
	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    errors.New("apiserver rejected delete temporarily"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s delete")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "retryable delete failure must not cancel the job")
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "ordinary delete failure should remain a retryable error")

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}

func TestVMDeleteWorker_FinalDeleteFailurePersistsTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_final_failure")
	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    errors.New("apiserver rejected final delete attempt"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s delete")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "final provider failure should let River finalize the ordinary error")

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusFAILED, storedVM.Status)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)
}

func TestVMDeleteWorker_FinalDeleteFailureRollsBackVMOnEventPersistFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMDeleteWorkerFixture(t, "vm_delete_final_failure_rollback")
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusFAILED, errors.New("failed final delete event persist unavailable")),
		ent.OpUpdate,
	))
	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    errors.New("apiserver rejected final delete attempt"),
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMDeleteArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMDeleteArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist final FAILED status for delete event")
	require.Contains(t, err.Error(), "failed final delete event persist unavailable")

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMDeleteWorker_ContextCancellationDoesNotFailEventTicketOrVM(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "vm_delete_context_canceled")
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
		SetStatus(entvm.StatusSTOPPED).
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

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("cleanup").
		Save(ctx)
	require.NoError(t, err)

	mock := &failingDeleteProvider{
		MockProvider: provider.NewMockProvider(),
		deleteErr:    context.Canceled,
	}
	worker := NewVMDeleteWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(ctx, &river.Job[VMDeleteArgs]{
		Args: VMDeleteArgs{EventID: eventID},
	})
	require.ErrorIs(t, err, context.Canceled)

	storedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, storedVM.Status)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)
}
