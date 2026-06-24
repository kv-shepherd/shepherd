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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

type transitionalPowerProvider struct {
	*provider.MockProvider
}

type failingPowerProvider struct {
	*provider.MockProvider
	execErr error
}

type countingPowerProvider struct {
	*provider.MockProvider
	startCalls   int
	stopCalls    int
	restartCalls int
}

type vmPowerWorkerFixture struct {
	eventID  string
	ticketID string
	vmID     string
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

func (p *failingPowerProvider) StartVM(_ context.Context, _, _, _ string) error {
	return p.execErr
}

func (p *failingPowerProvider) StopVM(_ context.Context, _, _, _ string) error {
	return p.execErr
}

func (p *failingPowerProvider) RestartVM(_ context.Context, _, _, _ string) error {
	return p.execErr
}

func (p *countingPowerProvider) StartVM(context.Context, string, string, string) error {
	p.startCalls++
	return nil
}

func (p *countingPowerProvider) StopVM(context.Context, string, string, string) error {
	p.stopCalls++
	return nil
}

func (p *countingPowerProvider) RestartVM(context.Context, string, string, string) error {
	p.restartCalls++
	return nil
}

func failDomainEventStatusUpdateHook(status domainevent.Status, err error) func(ent.Mutator) ent.Mutator {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			domainEventMutation, ok := mutation.(*ent.DomainEventMutation)
			if ok {
				nextStatus, exists := domainEventMutation.Status()
				if exists && nextStatus == status {
					return nil, err
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func seedVMPowerWorkerFixture(t *testing.T) (*ent.Client, vmPowerWorkerFixture) {
	t.Helper()
	return seedVMPowerWorkerFixtureForOperation(t, powerOpStop)
}

func seedVMPowerWorkerFixtureForOperation(t *testing.T, operation string) (*ent.Client, vmPowerWorkerFixture) {
	return seedVMPowerWorkerFixtureForOperationAndTicket(t, operation, true)
}

func seedDirectVMPowerWorkerFixtureForOperation(t *testing.T, operation string) (*ent.Client, vmPowerWorkerFixture) {
	return seedVMPowerWorkerFixtureForOperationAndTicket(t, operation, false)
}

func seedVMPowerWorkerFixtureForOperationAndTicket(t *testing.T, operation string, createTicket bool) (*ent.Client, vmPowerWorkerFixture) {
	t.Helper()
	client := testutil.OpenEntPostgres(t, "vm_power_worker_stop_"+uuid.NewString()[:8])
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
		Operation: operation,
		Actor:     "seed",
	}.ToJSON()
	require.NoError(t, err)

	eventType := domain.EventVMStopRequested
	switch operation {
	case powerOpStart:
		eventType = domain.EventVMStartRequested
	case powerOpRestart:
		eventType = domain.EventVMRestartRequested
	}

	eventID := "ev-" + uuid.NewString()
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(eventType)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticketID := ""
	if createTicket {
		ticketID = "ticket-" + uuid.NewString()
		_, err = client.Ticket.Create().
			SetID(ticketID).
			SetEventID(eventID).
			SetRequester("seed").
			SetStatus(entticket.StatusAPPROVED).
			SetOperationType(entticket.OperationTypePOWER).
			SetReason("power op").
			Save(ctx)
		require.NoError(t, err)
	}

	return client, vmPowerWorkerFixture{
		eventID:  eventID,
		ticketID: ticketID,
		vmID:     vmID,
	}
}

func TestVMPowerArgs(t *testing.T) {
	t.Parallel()

	var args VMPowerArgs
	if got := args.Kind(); got != "vm_power" {
		t.Fatalf("Kind() = %q, want vm_power", got)
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

func TestFallbackPowerOperationStatus_ReturnsTransitionalStates(t *testing.T) {
	t.Parallel()

	require.Equal(t, entvm.StatusSTARTING, fallbackPowerOperationStatus(powerOpStart))
	require.Equal(t, entvm.StatusSTOPPING, fallbackPowerOperationStatus(powerOpStop))
	require.Equal(t, entvm.StatusSTOPPING, fallbackPowerOperationStatus(powerOpRestart))
}

func TestIsIdempotentPowerConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		err       error
		want      bool
	}{
		{
			name:      "start already running",
			operation: powerOpStart,
			err:       fmt.Errorf("virtualmachine prod/vm-a already running"),
			want:      true,
		},
		{
			name:      "start manual start unsupported is not idempotent",
			operation: powerOpStart,
			err:       fmt.Errorf("does not support manual start requests"),
			want:      false,
		},
		{
			name:      "stop already stopped",
			operation: powerOpStop,
			err:       fmt.Errorf("VM is already stopped"),
			want:      true,
		},
		{
			name:      "stop not running",
			operation: powerOpStop,
			err:       fmt.Errorf("VirtualMachine is not running"),
			want:      true,
		},
		{
			name:      "stop manual stop unsupported is not idempotent",
			operation: powerOpStop,
			err:       fmt.Errorf("does not support manual stop requests"),
			want:      false,
		},
		{
			name:      "restart conflict is not idempotent",
			operation: powerOpRestart,
			err:       fmt.Errorf("VirtualMachine is not running"),
			want:      false,
		},
		{
			name:      "nil error",
			operation: powerOpStop,
			want:      false,
		},
		{
			name:      "unrelated provider error",
			operation: powerOpStart,
			err:       fmt.Errorf("admission webhook rejected request"),
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isIdempotentPowerConflict(tc.operation, tc.err))
		})
	}
}

func TestIsPowerTargetNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "k8s not found status error",
			err: k8serrors.NewNotFound(
				schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"},
				"vm-a",
			),
			want: true,
		},
		{
			name: "provider virtualmachine not found text",
			err:  fmt.Errorf("virtualmachine prod/vm-a not found"),
			want: true,
		},
		{
			name: "provider vm not found text",
			err:  fmt.Errorf("target vm prod/vm-a not found"),
			want: true,
		},
		{
			name: "generic not found text is not enough",
			err:  fmt.Errorf("storageclass fast not found"),
			want: false,
		},
		{
			name: "nil error",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isPowerTargetNotFound(tc.err))
		})
	}
}

func TestIsRestartStateConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "manual restart unsupported",
			err:  fmt.Errorf("does not support manual restart requests"),
			want: true,
		},
		{
			name: "vm is not running",
			err:  fmt.Errorf("vm is not running"),
			want: true,
		},
		{
			name: "virtualmachine not running",
			err:  fmt.Errorf("VirtualMachine prod/vm-a is not running"),
			want: true,
		},
		{
			name: "unrelated conflict",
			err:  fmt.Errorf("admission webhook rejected restart"),
			want: false,
		},
		{
			name: "nil error",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isRestartStateConflict(tc.err))
		})
	}
}

func TestIsTerminalPowerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		err       error
		want      bool
	}{
		{
			name:      "start missing target is terminal",
			operation: powerOpStart,
			err:       fmt.Errorf("virtualmachine prod/vm-a not found"),
			want:      true,
		},
		{
			name:      "restart non-running target is terminal",
			operation: powerOpRestart,
			err:       fmt.Errorf("vm is not running"),
			want:      true,
		},
		{
			name:      "start manual start unsupported is terminal",
			operation: powerOpStart,
			err:       fmt.Errorf("does not support manual start requests"),
			want:      true,
		},
		{
			name:      "stop manual stop unsupported is terminal",
			operation: powerOpStop,
			err:       fmt.Errorf("does not support manual stop requests"),
			want:      true,
		},
		{
			name:      "stop non-running target remains idempotent not terminal",
			operation: powerOpStop,
			err:       fmt.Errorf("vm is not running"),
			want:      false,
		},
		{
			name:      "transient network error is retryable",
			operation: powerOpRestart,
			err:       context.DeadlineExceeded,
			want:      false,
		},
		{
			name:      "nil error",
			operation: powerOpStart,
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isTerminalPowerError(tc.operation, tc.err))
		})
	}
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
	ticketID := "ticket-" + uuid.NewString()
	_, err = client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("power stop").
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
			EventID: eventID,
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

	ticket, err := client.Ticket.Get(ctx, ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMPowerWorker_DirectPowerEventDoesNotRequireTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedDirectVMPowerWorkerFixtureForOperation(t, powerOpStop)
	ctx := t.Context()
	vmRow, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)

	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       vmRow.Namespace,
		Cluster:         vmRow.ClusterID,
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-direct",
	}})

	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)

	stored, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusSTOPPING, stored.Status)
	require.NotNil(t, stored.LastK8sRv)
	require.Equal(t, "rv-stop-1", *stored.LastK8sRv)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticketCount, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(fixture.eventID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, ticketCount)
}

func TestVMPowerWorker_CancelsWhenVMIsDeletingBeforeProviderCall(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	ctx := t.Context()
	_, err := client.VM.UpdateOneID(fixture.vmID).
		SetStatus(entvm.StatusDELETING).
		Save(ctx)
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

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
}

func TestVMPowerWorker_DoesNotOverwriteConcurrentDeletingStatusAfterProviderSuccess(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	ctx := t.Context()
	vmRow, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	injectVMStatusBeforeNextJobsUpdate(t, client, fixture.vmID, entvm.StatusDELETING)

	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)
	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)

	liveVM, err := mock.GetVM(ctx, "cluster-a", "prod-ns", vmRow.Name)
	require.NoError(t, err)
	require.Equal(t, domain.VMStatusStopping, liveVM.Status)
	require.Equal(t, "rv-stop-1", liveVM.ResourceVersion)

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

func TestVMPowerWorker_SkipsCompletedEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	ctx := t.Context()
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(ctx)
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.NoError(t, err)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)

	vmRow, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
}

func TestVMPowerWorker_RestartTerminalPersistFailureDoesNotMarkTicketSuccess(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	ctx := t.Context()
	vmRow, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusCOMPLETED,
		errors.New("power event completed persist unavailable"),
	))

	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist terminal status after restart power event")
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)

	event, err := client.DomainEvent.Get(ctx, fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMPowerWorker_SkipsTerminalEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	tests := []struct {
		name         string
		eventStatus  domainevent.Status
		ticketStatus entticket.Status
	}{
		{
			name:         "failed",
			eventStatus:  domainevent.StatusFAILED,
			ticketStatus: entticket.StatusFAILED,
		},
		{
			name:         "cancelled",
			eventStatus:  domainevent.StatusCANCELLED,
			ticketStatus: entticket.StatusCANCELLED,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixture(t)
			ctx := t.Context()
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(tc.eventStatus).
				Save(ctx)
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(entticket.StatusEXECUTING).
				Save(ctx)
			require.NoError(t, err)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

			err = worker.Work(ctx, &river.Job[VMPowerArgs]{
				Args: VMPowerArgs{
					EventID: fixture.eventID,
				},
			})
			require.NoError(t, err)
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)

			event, err := client.DomainEvent.Get(ctx, fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, tc.eventStatus, event.Status)

			ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, tc.ticketStatus, ticket.Status)

			vmRow, err := client.VM.Get(ctx, fixture.vmID)
			require.NoError(t, err)
			require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
		})
	}
}

func TestVMPowerWorker_EventProcessingPersistFailureDoesNotExecuteProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("power event processing persist unavailable")),
		ent.OpUpdate,
	))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist PROCESSING/EXECUTING status for power event")
	require.Contains(t, err.Error(), "power event processing persist unavailable")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPENDING, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_SnoozesTransientClusterErrorsWithoutFailingTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	mock := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      context.DeadlineExceeded,
	}
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, clusterRuntimeUnavailableSnoozeDuration, snoozeErr.Duration)

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

func TestVMPowerWorker_RetryablePowerFailureDoesNotPersistTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	mock := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      errors.New("temporary power operation admission failure"),
	}
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "retryable power failure must not cancel the job")
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "ordinary power failure should remain a retryable error")

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

func TestVMPowerWorker_FinalPowerFailurePersistsTerminalFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	mock := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      errors.New("final power operation admission failure"),
	}
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute k8s")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "final provider failure should let River finalize the ordinary error")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, ticket.Status)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
}

func TestVMPowerWorker_ContextCancellationDoesNotFailEventOrTicket(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	mock := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      context.Canceled,
	}
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.ErrorIs(t, err, context.Canceled)

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

func TestVMPowerWorker_StopStatusPersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.New("power status persist unavailable")),
		ent.OpUpdateOne,
	))
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist VM status after stop power event")
	require.Contains(t, err.Error(), "power status persist unavailable")

	liveVM, err := mock.GetVM(t.Context(), "cluster-a", "prod-ns", vmRow.Name)
	require.NoError(t, err)
	require.Equal(t, domain.VMStatusStopping, liveVM.Status)
	require.Equal(t, "rv-stop-1", liveVM.ResourceVersion)

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

func TestVMPowerWorker_StopEventCompletePersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	updateCount := 0
	client.DomainEvent.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.New("power event complete persist unavailable")
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist terminal status after stop power event")
	require.Contains(t, err.Error(), "power event complete persist unavailable")

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

func TestVMPowerWorker_StatusPersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("power status persist interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Equal(t, context.Canceled, err)

	liveVM, err := mock.GetVM(t.Context(), "cluster-a", "prod-ns", vmRow.Name)
	require.NoError(t, err)
	require.Equal(t, domain.VMStatusStopping, liveVM.Status)
	require.Equal(t, "rv-stop-1", liveVM.ResourceVersion)

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

func TestVMPowerWorker_EventCompletePersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client, fixture := seedVMPowerWorkerFixture(t)
	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{MockProvider: provider.NewMockProvider()}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
	updateCount := 0
	client.DomainEvent.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.Join(errors.New("power event complete persist interrupted"), context.Canceled)
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	worker := NewVMPowerWorker(client, service.NewVMService(mock), nil)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{
			EventID: fixture.eventID,
		},
	})
	require.Equal(t, context.Canceled, err)

	liveVM, err := mock.GetVM(t.Context(), "cluster-a", "prod-ns", vmRow.Name)
	require.NoError(t, err)
	require.Equal(t, domain.VMStatusStopping, liveVM.Status)
	require.Equal(t, "rv-stop-1", liveVM.ResourceVersion)

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
