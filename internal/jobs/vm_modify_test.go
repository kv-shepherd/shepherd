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
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
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

type vmModifyWorkerFixture struct {
	clusterID string
	eventID   string
	ticketID  string
	vmID      string
	vmName    string
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

func seedVMModifyWorkerFixture(t *testing.T, dbName string) (*ent.Client, vmModifyWorkerFixture, *capturingModifyProvider) {
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
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("seed").
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

	return client, vmModifyWorkerFixture{
		clusterID: clusterID,
		eventID:   eventID,
		ticketID:  ticketID,
		vmID:      vmID,
		vmName:    vmName,
	}, mock
}

func TestVMModifyArgs(t *testing.T) {
	t.Parallel()

	var args VMModifyArgs
	if got := args.Kind(); got != "vm_modify" {
		t.Fatalf("Kind() = %q, want vm_modify", got)
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
	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetReason("modify memory").
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

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMModifyWorker_CancelsWhenVMIsDeletingBeforeMutation(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_deleting_precheck")
	ctx := t.Context()
	_, err := client.VM.UpdateOneID(fixture.vmID).
		SetStatus(entvm.StatusDELETING).
		Save(ctx)
	require.NoError(t, err)

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Empty(t, mock.lastMutationPayload)

	stored, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, stored.Status)
	require.Nil(t, stored.LastK8sRv)
	require.Nil(t, stored.LastPolledAt)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)
	require.Contains(t, ticket.RejectReason, "is deleting")
}

func TestVMModifyWorker_DoesNotOverwriteConcurrentDeletingStatusAfterMutation(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_concurrent_deleting")
	ctx := t.Context()
	injectVMStatusBeforeNextJobsUpdate(t, client, fixture.vmID, entvm.StatusDELETING)

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err := worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mock.lastMutationPayload)

	stored, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, stored.Status)
	require.Nil(t, stored.LastK8sRv)
	require.Nil(t, stored.LastPolledAt)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMModifyWorker_SkipsCompletedEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_completed_skip")
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
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetStatus(entticket.StatusEXECUTING).
		SetRequester("seed").
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
	require.Empty(t, mock.lastMutationPayload)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	stored, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, stored.Status)
	require.Nil(t, stored.LastK8sRv)
	require.Nil(t, stored.LastPolledAt)
}

func TestVMModifyWorker_SkipsFailedEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_failed_skip")
	ctx := t.Context()
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusFAILED).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(ctx)
	require.NoError(t, err)

	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Empty(t, mock.lastMutationPayload)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	stored, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, stored.Status)
	require.Nil(t, stored.LastK8sRv)
	require.Nil(t, stored.LastPolledAt)
}

func TestVMModifyWorker_RetryableMutationFailureDoesNotPersistTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, _ := seedVMModifyWorkerFixture(t, "vm_modify_retryable_mutation_failure")
	mock := &failingModifyProvider{
		MockProvider: provider.NewMockProvider(),
		updateErr:    errors.New("kubevirt api temporarily rejected the patch"),
	}
	mock.Seed([]*domain.VM{{
		ID:        fixture.vmID,
		Name:      fixture.vmName,
		Namespace: "prod-ns",
		Cluster:   fixture.clusterID,
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
	err := worker.Work(t.Context(), &river.Job[VMModifyArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMModifyArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute vm mutation")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "retryable modify failure must not cancel the job")
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "ordinary modify failure should remain a retryable error")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
}

func TestPersistFinalModifyFailure_PersistsRejectReasonAtomically(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMModifyWorkerFixture(t, "vm_modify_final_failure_atomic")
	ctx := t.Context()

	err := persistFinalModifyFailure(ctx, client, fixture.eventID, errors.New("provider rejected resize"))
	require.NoError(t, err)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, refreshedTicket.Status)
	require.Equal(t, "provider rejected resize", refreshedTicket.RejectReason)
}

func TestPersistFinalModifyFailure_RollsBackTerminalStateOnRejectReasonPersistFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture, _ := seedVMModifyWorkerFixture(t, "vm_modify_final_failure_rollback")
	ctx := t.Context()

	client.Ticket.Use(enthook.On(
		enthook.FixedError(errors.New("failed ticket reason persist unavailable")),
		ent.OpUpdate,
	))

	err := persistFinalModifyFailure(ctx, client, fixture.eventID, errors.New("provider rejected resize"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed ticket reason persist unavailable")

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusAPPROVED, refreshedTicket.Status)
	require.Empty(t, refreshedTicket.RejectReason)
}

func TestVMModifyWorker_FinalMutationFailurePersistsTerminalFailure(t *testing.T) {
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
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMModifyArgs{EventID: eventID},
	})
	require.Error(t, err)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)
	require.Contains(t, ticket.RejectReason, "kubevirt api rejected the patch")
}

func TestVMModifyWorker_SnoozesTransientClusterErrorsWithoutFailingTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_transient_cluster_error")
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
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("seed").
		Save(ctx)
	require.NoError(t, err)

	mock := &failingModifyProvider{
		MockProvider: provider.NewMockProvider(),
		updateErr:    fmt.Errorf("dial tcp 10.0.0.1:443: connect: connection refused"),
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
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, clusterRuntimeUnavailableSnoozeDuration, snoozeErr.Duration)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	storedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)
}

func TestVMModifyWorker_ContextCancellationDoesNotFailTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "vm_modify_context_canceled")
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
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("seed").
		Save(ctx)
	require.NoError(t, err)

	mock := &failingModifyProvider{
		MockProvider: provider.NewMockProvider(),
		updateErr:    context.Canceled,
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
	require.ErrorIs(t, err, context.Canceled)

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	event, err := client.DomainEvent.Get(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	storedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)
}

func TestVMModifyWorker_StatusPersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_status_persist_canceled")
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("modified vm status persist interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)
	require.Contains(t, mock.lastMutationPayload, "\"guest\":\"8Gi\"")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)
	require.Nil(t, storedVM.LastK8sRv)
	require.Nil(t, storedVM.LastPolledAt)
}

func TestVMModifyWorker_EventCompletePersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_event_complete_persist_failed")
	client.DomainEvent.Use(enthook.On(
		failDomainEventStatusUpdateHook(domainevent.StatusCOMPLETED, errors.New("event complete persist failed")),
		ent.OpUpdate,
	))
	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed modify state for event")
	require.Contains(t, err.Error(), "event complete persist failed")
	require.Contains(t, mock.lastMutationPayload, "\"guest\":\"8Gi\"")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)
	require.Nil(t, storedVM.LastK8sRv)
	require.Nil(t, storedVM.LastPolledAt)
}

func TestVMModifyWorker_TicketSuccessPersistFailureRollsBackTerminalState(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}
	_ = logger.Init("error", "json")

	client, fixture, mock := seedVMModifyWorkerFixture(t, "vm_modify_ticket_success_persist_failed")
	updateCount := 0
	client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.New("ticket success persist failed")
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	worker := NewVMModifyWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMModifyArgs]{
		Args: VMModifyArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist completed modify state for event")
	require.Contains(t, err.Error(), "ticket success persist failed")
	require.Contains(t, mock.lastMutationPayload, "\"guest\":\"8Gi\"")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	require.Empty(t, ticket.RejectReason)

	storedVM, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, storedVM.Status)
	require.Nil(t, storedVM.LastK8sRv)
	require.Nil(t, storedVM.LastPolledAt)
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
