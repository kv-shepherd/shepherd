package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type transitionalPowerProvider struct {
	*provider.MockProvider
	restartCalls    atomic.Int32
	afterTransition func(context.Context) error
}

type failingPowerProvider struct {
	*provider.MockProvider
	execErr      error
	startCalls   atomic.Int32
	stopCalls    atomic.Int32
	restartCalls atomic.Int32
}

type countingPowerProvider struct {
	*provider.MockProvider
	startCalls   int
	stopCalls    int
	restartCalls int
}

type blockingPowerProvider struct {
	*provider.MockProvider
	startCalls   atomic.Int32
	stopCalls    atomic.Int32
	restartCalls atomic.Int32
	entered      chan struct{}
	release      chan struct{}
	enteredOnce  sync.Once
	releaseOnce  sync.Once
}

type restartRefreshFailureProvider struct {
	*provider.MockProvider
	restartCalls atomic.Int32
	refreshErr   error
}

type payloadMutatingPowerProvider struct {
	*provider.MockProvider
	mutate    func(context.Context) error
	stopCalls atomic.Int32
}

type vmPowerWorkerFixture struct {
	eventID  string
	ticketID string
	vmID     string
	payload  domain.VMPowerPayload
}

func execVMPowerTestSQL(ctx context.Context, client *ent.Client, query string, args ...any) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := tx.ExecContext(ctx, query, args...); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type vmPowerBatchFixture struct {
	parentEventID  string
	parentTicketID string
}

func (p *transitionalPowerProvider) StartVM(ctx context.Context, cluster, namespace, name string) error {
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStarting
	vm.ResourceVersion = "rv-start-1"
	return p.runAfterTransition(ctx)
}

func (p *transitionalPowerProvider) StopVM(ctx context.Context, cluster, namespace, name string) error {
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStopping
	vm.ResourceVersion = "rv-stop-1"
	return p.runAfterTransition(ctx)
}

func (p *transitionalPowerProvider) RestartVM(ctx context.Context, cluster, namespace, name string) error {
	p.restartCalls.Add(1)
	vm, err := p.GetVM(ctx, cluster, namespace, name)
	if err != nil {
		return err
	}
	vm.Status = domain.VMStatusStopping
	vm.ResourceVersion = "rv-restart-1"
	return p.runAfterTransition(ctx)
}

func (p *transitionalPowerProvider) runAfterTransition(ctx context.Context) error {
	if p.afterTransition == nil {
		return nil
	}
	return p.afterTransition(ctx)
}

func (p *failingPowerProvider) StartVM(_ context.Context, _, _, _ string) error {
	p.startCalls.Add(1)
	return p.execErr
}

func (p *failingPowerProvider) StopVM(_ context.Context, _, _, _ string) error {
	p.stopCalls.Add(1)
	return p.execErr
}

func (p *failingPowerProvider) RestartVM(_ context.Context, _, _, _ string) error {
	p.restartCalls.Add(1)
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

func (p *blockingPowerProvider) RestartVM(ctx context.Context, _, _, _ string) error {
	p.restartCalls.Add(1)
	return p.block(ctx)
}

func (p *blockingPowerProvider) StartVM(ctx context.Context, _, _, _ string) error {
	p.startCalls.Add(1)
	return p.block(ctx)
}

func (p *blockingPowerProvider) StopVM(ctx context.Context, _, _, _ string) error {
	p.stopCalls.Add(1)
	return p.block(ctx)
}

func (p *blockingPowerProvider) block(ctx context.Context) error {
	p.enteredOnce.Do(func() { close(p.entered) })
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *blockingPowerProvider) unblock() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *restartRefreshFailureProvider) RestartVM(context.Context, string, string, string) error {
	p.restartCalls.Add(1)
	return nil
}

func (p *restartRefreshFailureProvider) GetVM(context.Context, string, string, string) (*domain.VM, error) {
	return nil, p.refreshErr
}

func (p *payloadMutatingPowerProvider) StopVM(ctx context.Context, _, _, _ string) error {
	p.stopCalls.Add(1)
	if p.mutate == nil {
		return nil
	}
	return p.mutate(ctx)
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
	return seedVMPowerWorkerFixtureForOperationTicketAndPayload(t, operation, createTicket, nil)
}

func seedVMPowerWorkerFixtureForRawPayload(t *testing.T, payload []byte) (*ent.Client, vmPowerWorkerFixture) {
	return seedVMPowerWorkerFixtureForOperationTicketAndPayload(t, powerOpStop, true, payload)
}

func seedVMPowerWorkerFixtureForOperationTicketAndPayload(
	t *testing.T,
	operation string,
	createTicket bool,
	payloadOverride []byte,
) (*ent.Client, vmPowerWorkerFixture) {
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

	payload := domain.VMPowerPayload{
		VMID:      vmID,
		VMName:    vmName,
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Operation: operation,
		Actor:     "seed",
		DispatchMode: func() domain.VMPowerDispatchMode {
			if createTicket {
				return domain.VMPowerDispatchTicket
			}
			return domain.VMPowerDispatchDirect
		}(),
	}
	payloadBytes, err := payload.ToJSON()
	require.NoError(t, err)
	if payloadOverride != nil {
		payloadBytes = append([]byte(nil), payloadOverride...)
	}

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
			SetApprover("approver").
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
		payload:  payload,
	}
}

func attachVMPowerBatchFixture(t *testing.T, client *ent.Client, child vmPowerWorkerFixture) vmPowerBatchFixture {
	t.Helper()
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEventID := "ev-parent-" + uuid.NewString()
	parentPayload, err := (domain.BatchVMRequestPayload{
		Operation:   "POWER_" + strings.ToUpper(strings.TrimSpace(child.payload.Operation)),
		SubmittedBy: child.payload.Actor,
		SubmittedAt: time.Now().UTC(),
		Items: []domain.BatchVMItemPayload{
			batchPowerProvenanceItem(child.payload),
		},
	}).ToJSON()
	require.NoError(t, err)
	_, err = client.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload(parentPayload).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEventID).
		SetRequester("seed").
		SetApprover("approver").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("batch power").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicketID).
		SetBatchType(batchticket.BatchTypeBATCH_POWER).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch power").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Ticket.UpdateOneID(child.ticketID).
		SetParentTicketID(parentTicketID).
		SetAttemptCount(1).
		SetLastAttemptAt(time.Now().UTC()).
		Save(ctx)
	require.NoError(t, err)

	return vmPowerBatchFixture{
		parentEventID:  parentEventID,
		parentTicketID: parentTicketID,
	}
}

func requireFailedVMPowerState(
	t *testing.T,
	client *ent.Client,
	child vmPowerWorkerFixture,
	batch *vmPowerBatchFixture,
) {
	t.Helper()
	ctx := t.Context()

	event, err := client.DomainEvent.Get(ctx, child.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, event.Status)

	if child.ticketID != "" {
		ticket, ticketErr := client.Ticket.Get(ctx, child.ticketID)
		require.NoError(t, ticketErr)
		require.Equal(t, entticket.StatusFAILED, ticket.Status)
	}
	if batch == nil {
		return
	}

	parentEvent, err := client.DomainEvent.Get(ctx, batch.parentEventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, parentEvent.Status)

	parentTicket, err := client.Ticket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, parentTicket.Status)

	projection, err := client.BatchTicket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	require.Equal(t, 1, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 1, projection.FailedCount)
	require.Equal(t, 0, projection.PendingCount)
	require.Equal(t, batchticket.StatusFAILED, projection.Status)
}

func requireAmbiguousRestartFence(
	t *testing.T,
	client *ent.Client,
	child vmPowerWorkerFixture,
	batch *vmPowerBatchFixture,
) {
	t.Helper()
	requireVMPowerProcessingState(t, client, child, batch)
}

func requireVMPowerProcessingState(
	t *testing.T,
	client *ent.Client,
	child vmPowerWorkerFixture,
	batch *vmPowerBatchFixture,
) {
	t.Helper()
	ctx := t.Context()

	event, err := client.DomainEvent.Get(ctx, child.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)

	if child.ticketID != "" {
		ticket, ticketErr := client.Ticket.Get(ctx, child.ticketID)
		require.NoError(t, ticketErr)
		require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	}
	if batch == nil {
		return
	}

	parentEvent, err := client.DomainEvent.Get(ctx, batch.parentEventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, parentEvent.Status)

	parentTicket, err := client.Ticket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, parentTicket.Status)

	projection, err := client.BatchTicket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	require.Equal(t, 1, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 0, projection.FailedCount)
	require.Equal(t, 1, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
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
	require.ElementsMatch(t, []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRetryable,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
	}, opts.UniqueOpts.ByState)
	require.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCompleted)
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
			name:      "network error is not a deterministic terminal classification",
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
		VMID:         vmID,
		VMName:       vmName,
		ClusterID:    "cluster-a",
		Namespace:    "prod-ns",
		Operation:    powerOpStop,
		Actor:        "seed",
		DispatchMode: domain.VMPowerDispatchTicket,
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
		SetApprover("approver").
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

func TestVMPowerWorker_RejectsInconsistentImmutableIdentityBeforeProviderCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(context.Context, *ent.Client, vmPowerWorkerFixture) error
	}{
		{
			name: "event type and payload operation",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(
					ctx,
					client,
					`UPDATE domain_events SET event_type = $1 WHERE id = $2`,
					string(domain.EventVMRestartRequested),
					fixture.eventID,
				)
			},
		},
		{
			name: "aggregate type",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(ctx, client, `UPDATE domain_events SET aggregate_type = 'batch' WHERE id = $1`, fixture.eventID)
			},
		},
		{
			name: "aggregate id and payload vm",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(ctx, client, `UPDATE domain_events SET aggregate_id = 'foreign-vm' WHERE id = $1`, fixture.eventID)
			},
		},
		{
			name: "event actor and payload actor",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(ctx, client, `UPDATE domain_events SET created_by = 'foreign-actor' WHERE id = $1`, fixture.eventID)
			},
		},
		{
			name: "ticket requester and payload actor",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(ctx, client, `UPDATE tickets SET requester = 'foreign-actor' WHERE id = $1`, fixture.ticketID)
			},
		},
		{
			name: "provider coordinates",
			mutate: func(ctx context.Context, client *ent.Client, fixture vmPowerWorkerFixture) error {
				return execVMPowerTestSQL(ctx, client, `UPDATE vms SET cluster_id = 'foreign-cluster' WHERE id = $1`, fixture.vmID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
			require.NoError(t, tt.mutate(t.Context(), client, fixture))

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				Args: VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)

			event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, loadErr)
			require.Equal(t, domainevent.StatusPENDING, event.Status)
			ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, loadErr)
			require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
		})
	}
}

func TestVMPowerWorker_ClaimRejectsPayloadSnapshotTOCTOUBeforeProviderCall(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
	tampered := fixture.payload
	tampered.VMID = "foreign-vm-" + uuid.NewString()
	tamperedPayload, err := tampered.ToJSON()
	require.NoError(t, err)

	var (
		mutateOnce  sync.Once
		mutationErr error
	)
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			mutateOnce.Do(func() {
				mutationErr = execVMPowerTestSQL(
					ctx,
					client,
					`UPDATE domain_events SET payload = $1 WHERE id = $2`,
					tamperedPayload,
					fixture.eventID,
				)
			})
			if mutationErr != nil {
				return nil, mutationErr
			}
			return next.Query(ctx, query)
		})
	}))

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
		t.Context(),
		&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
	)
	require.NoError(t, mutationErr)
	requirePowerDispatchRejected(t, err)
	require.Zero(t, infra.startCalls)

	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_ProviderPayloadTOCTOUCannotTerminalizeEvent(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStop)
	batch := attachVMPowerBatchFixture(t, client, fixture)
	tampered := fixture.payload
	tampered.Actor = "foreign-actor-" + uuid.NewString()
	tamperedPayload, err := tampered.ToJSON()
	require.NoError(t, err)

	infra := &payloadMutatingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		mutate: func(ctx context.Context) error {
			return execVMPowerTestSQL(
				ctx,
				client,
				`UPDATE domain_events SET payload = $1 WHERE id = $2`,
				tamperedPayload,
				fixture.eventID,
			)
		},
	}
	err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
		t.Context(),
		&river.Job[VMPowerArgs]{
			JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
			Args:   VMPowerArgs{EventID: fixture.eventID},
		},
	)
	requirePowerDispatchRejected(t, err)
	require.Equal(t, int32(1), infra.stopCalls.Load())

	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	parentEvent, loadErr := client.DomainEvent.Get(t.Context(), batch.parentEventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPROCESSING, parentEvent.Status)
	parentTicket, loadErr := client.Ticket.Get(t.Context(), batch.parentTicketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusEXECUTING, parentTicket.Status)
	projection, loadErr := client.BatchTicket.Get(t.Context(), batch.parentTicketID)
	require.NoError(t, loadErr)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
	require.Equal(t, 1, projection.PendingCount)
	require.Zero(t, projection.SuccessCount)
	require.Zero(t, projection.FailedCount)
}

func TestVMPowerWorker_InconsistentIdentityPreservesProcessingFence(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
	err := execVMPowerTestSQL(
		t.Context(),
		client,
		`UPDATE domain_events SET event_type = $1, status = 'PROCESSING' WHERE id = $2`,
		string(domain.EventVMRestartRequested),
		fixture.eventID,
	)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMPowerWorker_TicketProvenanceRejectsMissingOrNonExecutableBinding(t *testing.T) {
	t.Parallel()

	t.Run("missing ticket", func(t *testing.T) {
		t.Parallel()
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
		require.NoError(t, client.Ticket.DeleteOneID(fixture.ticketID).Exec(t.Context()))

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		err := NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
			t.Context(),
			&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
		)
		requirePowerDispatchRejected(t, err)
		require.Zero(t, infra.startCalls)
		event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
		require.NoError(t, loadErr)
		require.Equal(t, domainevent.StatusPENDING, event.Status)
	})

	for _, status := range []entticket.Status{
		entticket.StatusPENDING,
		entticket.StatusREJECTED,
		entticket.StatusCANCELLED,
	} {
		status := status
		t.Run("ticket status "+status.String(), func(t *testing.T) {
			t.Parallel()
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
			_, err := client.Ticket.UpdateOneID(fixture.ticketID).SetStatus(status).Save(t.Context())
			require.NoError(t, err)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
				t.Context(),
				&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
			)
			requirePowerDispatchRejected(t, err)
			require.Zero(t, infra.startCalls)
			event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, loadErr)
			require.Equal(t, domainevent.StatusPENDING, event.Status)
			ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, loadErr)
			require.Equal(t, status, ticket.Status)
		})
	}

	t.Run("processing ticket has no approver", func(t *testing.T) {
		t.Parallel()
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
		_, err := client.DomainEvent.UpdateOneID(fixture.eventID).SetStatus(domainevent.StatusPROCESSING).Save(t.Context())
		require.NoError(t, err)
		_, err = client.Ticket.UpdateOneID(fixture.ticketID).
			SetStatus(entticket.StatusEXECUTING).
			ClearApprover().
			Save(t.Context())
		require.NoError(t, err)

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
			t.Context(),
			&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
		)
		requirePowerDispatchRejected(t, err)
		require.Zero(t, infra.startCalls)
	})
}

func TestVMPowerWorker_DirectProvenanceRejectsUnexpectedTicket(t *testing.T) {
	t.Parallel()

	client, fixture := seedDirectVMPowerWorkerFixtureForOperation(t, powerOpStart)
	_, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(fixture.eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypePOWER).
		Save(t.Context())
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
		t.Context(),
		&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
	)
	requirePowerDispatchRejected(t, err)
	require.Zero(t, infra.startCalls)
	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
}

func TestVMPowerWorker_DirectPreDispatchFailureNeverMutatesUnexpectedTicket(t *testing.T) {
	t.Parallel()

	client, fixture := seedDirectVMPowerWorkerFixtureForOperation(t, powerOpStart)
	require.NoError(t, client.VM.DeleteOneID(fixture.vmID).Exec(t.Context()))
	ticketID := "ticket-" + uuid.NewString()
	_, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(fixture.eventID).
		SetRequester("seed").
		SetApprover("foreign-approver").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypePOWER).
		Save(t.Context())
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
		t.Context(),
		&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
	)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls)
	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, loadErr := client.Ticket.Get(t.Context(), ticketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_BatchProvenanceRejectsTamperingBeforeProviderCall(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, client *ent.Client, fixture vmPowerWorkerFixture, batch vmPowerBatchFixture)
	}{
		{
			name: "child approver mismatch",
			mutate: func(t *testing.T, client *ent.Client, fixture vmPowerWorkerFixture, _ vmPowerBatchFixture) {
				_, err := client.Ticket.UpdateOneID(fixture.ticketID).SetApprover("foreign-approver").Save(t.Context())
				require.NoError(t, err)
			},
		},
		{
			name: "missing durable attempt",
			mutate: func(t *testing.T, client *ent.Client, fixture vmPowerWorkerFixture, _ vmPowerBatchFixture) {
				_, err := client.Ticket.UpdateOneID(fixture.ticketID).
					SetAttemptCount(0).
					ClearLastAttemptAt().
					Save(t.Context())
				require.NoError(t, err)
			},
		},
		{
			name: "parent action mismatch",
			mutate: func(t *testing.T, client *ent.Client, fixture vmPowerWorkerFixture, batch vmPowerBatchFixture) {
				payload, err := (domain.BatchVMRequestPayload{
					Operation:   "POWER_RESTART",
					SubmittedBy: fixture.payload.Actor,
					SubmittedAt: time.Now().UTC(),
					Items:       []domain.BatchVMItemPayload{batchPowerProvenanceItem(fixture.payload)},
				}).ToJSON()
				require.NoError(t, err)
				require.NoError(t, execVMPowerTestSQL(
					t.Context(),
					client,
					`UPDATE domain_events SET payload = $1 WHERE id = $2`,
					payload,
					batch.parentEventID,
				))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
			batch := attachVMPowerBatchFixture(t, client, fixture)
			tc.mutate(t, client, fixture, batch)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			err := NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
				t.Context(),
				&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
			)
			requirePowerDispatchRejected(t, err)
			require.Zero(t, infra.startCalls)
			event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, loadErr)
			require.Equal(t, domainevent.StatusPENDING, event.Status)
		})
	}
}

func TestVMPowerWorker_TerminalProjectionRepairRejectsInvalidProvenance(t *testing.T) {
	t.Parallel()

	t.Run("direct event with unexpected ticket", func(t *testing.T) {
		t.Parallel()
		client, fixture := seedDirectVMPowerWorkerFixtureForOperation(t, powerOpStart)
		_, err := client.DomainEvent.UpdateOneID(fixture.eventID).SetStatus(domainevent.StatusCOMPLETED).Save(t.Context())
		require.NoError(t, err)
		ticketID := "ticket-" + uuid.NewString()
		_, err = client.Ticket.Create().
			SetID(ticketID).
			SetEventID(fixture.eventID).
			SetRequester("seed").
			SetApprover("approver").
			SetStatus(entticket.StatusEXECUTING).
			SetOperationType(entticket.OperationTypePOWER).
			Save(t.Context())
		require.NoError(t, err)

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
			t.Context(),
			&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
		)
		requirePowerDispatchRejected(t, err)
		require.Zero(t, infra.startCalls)
		ticket, loadErr := client.Ticket.Get(t.Context(), ticketID)
		require.NoError(t, loadErr)
		require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	})

	for _, status := range []entticket.Status{entticket.StatusREJECTED, entticket.StatusCANCELLED} {
		status := status
		t.Run("completed event conflicts with "+status.String(), func(t *testing.T) {
			t.Parallel()
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).SetStatus(domainevent.StatusCOMPLETED).Save(t.Context())
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).SetStatus(status).Save(t.Context())
			require.NoError(t, err)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
				t.Context(),
				&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
			)
			requirePowerDispatchRejected(t, err)
			ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, loadErr)
			require.Equal(t, status, ticket.Status)
		})
	}

	t.Run("cancelled event preserves approved rejection", func(t *testing.T) {
		t.Parallel()
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
		_, err := client.DomainEvent.UpdateOneID(fixture.eventID).SetStatus(domainevent.StatusCANCELLED).Save(t.Context())
		require.NoError(t, err)
		_, err = client.Ticket.UpdateOneID(fixture.ticketID).SetStatus(entticket.StatusREJECTED).Save(t.Context())
		require.NoError(t, err)
		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		require.NoError(t, NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
			t.Context(),
			&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
		))
		ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
		require.NoError(t, loadErr)
		require.Equal(t, entticket.StatusREJECTED, ticket.Status)
	})

	t.Run("malformed terminal payload never mutates ticket", func(t *testing.T) {
		t.Parallel()
		client, fixture := seedVMPowerWorkerFixtureForRawPayload(t, []byte(`{"operation":`))
		_, err := client.DomainEvent.UpdateOneID(fixture.eventID).SetStatus(domainevent.StatusFAILED).Save(t.Context())
		require.NoError(t, err)
		_, err = client.Ticket.UpdateOneID(fixture.ticketID).SetStatus(entticket.StatusEXECUTING).Save(t.Context())
		require.NoError(t, err)
		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
			t.Context(),
			&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
		)
		var cancelErr *river.JobCancelError
		require.ErrorAs(t, err, &cancelErr)
		ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
		require.NoError(t, loadErr)
		require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
	})
}

func TestRepairTerminalVMPowerProjectionOnce_ExactTerminalBatchIsReadOnly(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	batch := attachVMPowerBatchFixture(t, client, fixture)
	ctx := t.Context()

	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusSUCCESS).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, syncParentBatchStatusByChildEvent(ctx, client, fixture.eventID))

	parentBefore, err := client.Ticket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	projectionBefore, err := client.BatchTicket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)

	var writes atomic.Int32
	countUpdate := func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			writes.Add(1)
			return next.Mutate(ctx, mutation)
		})
	}
	client.Ticket.Use(enthook.On(countUpdate, ent.OpUpdate))
	client.DomainEvent.Use(enthook.On(countUpdate, ent.OpUpdate))
	client.BatchTicket.Use(enthook.On(countUpdate, ent.OpUpdate))

	require.NoError(t, repairTerminalVMPowerProjectionOnce(
		ctx,
		client,
		fixture.eventID,
		fixture.payload,
		powerOpStop,
	))
	require.Zero(t, writes.Load(), "an already exact terminal batch must not be rewritten")

	parentAfter, err := client.Ticket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	projectionAfter, err := client.BatchTicket.Get(ctx, batch.parentTicketID)
	require.NoError(t, err)
	require.Equal(t, parentBefore.UpdatedAt, parentAfter.UpdatedAt)
	require.Equal(t, projectionBefore.UpdatedAt, projectionAfter.UpdatedAt)
}

func TestVMPowerWorker_LegacyMissingDispatchModeFailsClosed(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Payload, &payload))
	delete(payload, "dispatch_mode")
	legacyPayload, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, execVMPowerTestSQL(
		t.Context(),
		client,
		`UPDATE domain_events SET payload = $1 WHERE id = $2`,
		legacyPayload,
		fixture.eventID,
	))

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	err = NewVMPowerWorker(client, service.NewVMService(infra), nil).Work(
		t.Context(),
		&river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}},
	)
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls)
	stored, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, stored.Status)
}

func requirePowerDispatchRejected(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var rejected *powerDispatchRejectedError
	require.ErrorAs(t, err, &rejected)
}

func TestVMPowerWorker_MissingEventCancelsWithoutSnooze(t *testing.T) {
	t.Parallel()

	for _, attempt := range []int{1, 3} {
		t.Run(fmt.Sprintf("attempt_%d", attempt), func(t *testing.T) {
			t.Parallel()
			client := testutil.OpenEntPostgres(t, "vm_power_missing_event")
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: "missing-power-event"},
			})
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			var snoozeErr *river.JobSnoozeError
			require.NotErrorAs(t, err, &snoozeErr)
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)
		})
	}
}

func TestVMPowerWorker_RestartPreClaimVMValidationPreservesProcessingFence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutateVM func(context.Context, *ent.Client, string) error
	}{
		{
			name: "deleting",
			mutateVM: func(ctx context.Context, client *ent.Client, vmID string) error {
				_, err := client.VM.UpdateOneID(vmID).SetStatus(entvm.StatusDELETING).Save(ctx)
				return err
			},
		},
		{
			name: "missing",
			mutateVM: func(ctx context.Context, client *ent.Client, vmID string) error {
				return client.VM.DeleteOneID(vmID).Exec(ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			ctx := t.Context()
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusPROCESSING).
				Save(ctx)
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(entticket.StatusEXECUTING).
				Save(ctx)
			require.NoError(t, err)
			require.NoError(t, tt.mutateVM(ctx, client, fixture.vmID))

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err = worker.Work(ctx, &river.Job[VMPowerArgs]{
				Args: VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			require.Zero(t, infra.restartCalls)

			event, err := client.DomainEvent.Get(ctx, fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusPROCESSING, event.Status)
			ticket, err := client.Ticket.Get(ctx, fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
		})
	}
}

func TestVMPowerWorker_DoesNotOverwriteConcurrentDeletingStatusAfterProviderSuccess(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	ctx := t.Context()
	vmRow, err := client.VM.Get(ctx, fixture.vmID)
	require.NoError(t, err)
	mock := &transitionalPowerProvider{
		MockProvider: provider.NewMockProvider(),
		afterTransition: func(ctx context.Context) error {
			_, updateErr := client.VM.UpdateOneID(fixture.vmID).
				SetStatus(entvm.StatusDELETING).
				Save(ctx)
			return updateErr
		},
	}
	mock.Seed([]*domain.VM{{
		Name:            vmRow.Name,
		Namespace:       "prod-ns",
		Cluster:         "cluster-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-before-1",
	}})
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

func TestVMPowerWorker_RestartPendingDispatchClaimCallsProviderOnce(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Equal(t, 1, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMPowerWorker_RestartProcessingRetryPreservesAmbiguousFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Contains(t, err.Error(), "already PROCESSING")
	require.Contains(t, err.Error(), "read-only verification and escalation")
	require.Contains(t, err.Error(), "provider receipt")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMPowerWorker_RestartClaimLossToTerminalDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		eventStatus  domainevent.Status
		ticketStatus entticket.Status
	}{
		{name: "completed", eventStatus: domainevent.StatusCOMPLETED, ticketStatus: entticket.StatusSUCCESS},
		{name: "cancelled", eventStatus: domainevent.StatusCANCELLED, ticketStatus: entticket.StatusCANCELLED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			var transitionOnce sync.Once
			client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
				return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
					var transitionErr error
					transitionOnce.Do(func() {
						_, transitionErr = client.DomainEvent.UpdateOneID(fixture.eventID).
							SetStatus(tc.eventStatus).
							Save(ctx)
						if transitionErr == nil {
							_, transitionErr = client.Ticket.UpdateOneID(fixture.ticketID).
								SetStatus(tc.ticketStatus).
								Save(ctx)
						}
					})
					if transitionErr != nil {
						return nil, transitionErr
					}
					return next.Query(ctx, query)
				})
			}))

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})
			require.NoError(t, err)
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)

			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, tc.eventStatus, event.Status)
			ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, tc.ticketStatus, ticket.Status)
		})
	}
}

func TestVMPowerWorker_RestartClaimLossHandlesPersistedOutcomesWithoutDispatch(t *testing.T) {
	t.Parallel()

	t.Run("pending claim loss converges exact failure before dispatch", func(t *testing.T) {
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
		client.DomainEvent.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
				domainEventMutation := mutation.(*ent.DomainEventMutation)
				status, changed := domainEventMutation.Status()
				if changed && status == domainevent.StatusPROCESSING {
					return 0, nil
				}
				return next.Mutate(ctx, mutation)
			})
		}, ent.OpUpdate))

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
		err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})

		require.Error(t, err)
		var cancelErr *river.JobCancelError
		require.ErrorAs(t, err, &cancelErr)
		require.Contains(t, err.Error(), "PENDING dispatch claim was not acquired")
		require.Zero(t, infra.restartCalls)
		event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
		require.NoError(t, eventErr)
		require.Equal(t, domainevent.StatusFAILED, event.Status)
		ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
		require.NoError(t, ticketErr)
		require.Equal(t, entticket.StatusFAILED, ticket.Status)
	})

	t.Run("missing event after claim loss cancels without dispatch", func(t *testing.T) {
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
		var deleteOnce sync.Once
		client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
				var deleteErr error
				deleteOnce.Do(func() {
					deleteErr = client.DomainEvent.DeleteOneID(fixture.eventID).Exec(ctx)
				})
				if deleteErr != nil {
					return nil, deleteErr
				}
				return next.Query(ctx, query)
			})
		}))

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
		err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})

		require.Error(t, err)
		var cancelErr *river.JobCancelError
		require.ErrorAs(t, err, &cancelErr)
		require.Contains(t, err.Error(), "domain event no longer exists")
		require.Zero(t, infra.restartCalls)
		_, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
		require.True(t, ent.IsNotFound(eventErr))
		ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
		require.NoError(t, ticketErr)
		require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
	})

	t.Run("unexpected observed status cancels without changing the durable row", func(t *testing.T) {
		client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
		var eventReads atomic.Int32
		client.DomainEvent.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
				value, err := next.Query(ctx, query)
				if err != nil {
					return nil, err
				}
				if eventReads.Add(1) == 2 {
					events := value.([]*ent.DomainEvent)
					require.Len(t, events, 1)
					events[0].Status = domainevent.Status("UNKNOWN")
				}
				return value, nil
			})
		}))

		infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
		worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
		err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})

		require.Error(t, err)
		var cancelErr *river.JobCancelError
		require.ErrorAs(t, err, &cancelErr)
		require.Contains(t, err.Error(), "unexpected status UNKNOWN")
		require.Zero(t, infra.restartCalls)
		event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
		require.NoError(t, eventErr)
		require.Equal(t, domainevent.StatusPENDING, event.Status)
	})
}

func TestVMPowerWorker_RestartClaimTargetRejectionConvergesExactPendingBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutateVM func(context.Context, *ent.Client, string) error
	}{
		{
			name: "deleting",
			mutateVM: func(ctx context.Context, client *ent.Client, vmID string) error {
				_, err := client.VM.UpdateOneID(vmID).SetStatus(entvm.StatusDELETING).Save(ctx)
				return err
			},
		},
		{
			name: "missing",
			mutateVM: func(ctx context.Context, client *ent.Client, vmID string) error {
				return client.VM.DeleteOneID(vmID).Exec(ctx)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			batch := attachVMPowerBatchFixture(t, client, fixture)
			var vmReads atomic.Int32
			client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
				return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
					read := vmReads.Add(1)
					value, err := next.Query(ctx, query)
					if err == nil && read == 1 {
						require.NoError(t, tc.mutateVM(ctx, client, fixture.vmID))
					}
					return value, err
				})
			}))
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))

			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})

			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			require.Equal(t, int32(2), vmReads.Load(), "target must change after the unlocked precheck")
			require.Zero(t, infra.restartCalls)
			requireFailedVMPowerState(t, client, fixture, &batch)
			auditRows, auditErr := client.AuditLog.Query().All(t.Context())
			require.NoError(t, auditErr)
			require.Len(t, auditRows, 1)
			require.Equal(t, "vm.restart_failed", auditRows[0].Action)
		})
	}
}

func TestVMPowerWorker_DirectRestartClaimTargetRejectionConvergesExactPendingEvent(t *testing.T) {
	t.Parallel()

	client, fixture := seedDirectVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	var vmReads atomic.Int32
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			read := vmReads.Add(1)
			value, err := next.Query(ctx, query)
			if err == nil && read == 1 {
				require.NoError(t, client.VM.DeleteOneID(fixture.vmID).Exec(ctx))
			}
			return value, err
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})

	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Equal(t, int32(2), vmReads.Load())
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
	ticketCount, ticketErr := client.Ticket.Query().
		Where(entticket.EventIDEQ(fixture.eventID)).
		Count(t.Context())
	require.NoError(t, ticketErr)
	require.Zero(t, ticketCount)
	auditRows, auditErr := client.AuditLog.Query().All(t.Context())
	require.NoError(t, auditErr)
	require.Len(t, auditRows, 1)
	require.Equal(t, "vm.restart_failed", auditRows[0].Action)
}

func TestVMPowerWorker_RestartClaimTargetRejectionPreservesConcurrentProcessingFence(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	batch := attachVMPowerBatchFixture(t, client, fixture)
	var vmReads atomic.Int32
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			read := vmReads.Add(1)
			value, err := next.Query(ctx, query)
			if err != nil || read != 1 {
				return value, err
			}

			mutationErr := func() error {
				tx, txErr := client.Tx(ctx)
				if txErr != nil {
					return txErr
				}
				defer func() { _ = tx.Rollback() }()
				txClient := tx.Client()
				if _, txErr = txClient.DomainEvent.UpdateOneID(fixture.eventID).
					SetStatus(domainevent.StatusPROCESSING).
					Save(ctx); txErr != nil {
					return txErr
				}
				if _, txErr = txClient.Ticket.UpdateOneID(fixture.ticketID).
					SetStatus(entticket.StatusEXECUTING).
					Save(ctx); txErr != nil {
					return txErr
				}
				if _, txErr = txClient.VM.UpdateOneID(fixture.vmID).
					SetStatus(entvm.StatusDELETING).
					Save(ctx); txErr != nil {
					return txErr
				}
				return tx.Commit()
			}()
			require.NoError(t, mutationErr)
			return value, nil
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})

	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Equal(t, int32(2), vmReads.Load(), "claim must re-read the concurrently changed target")
	require.Zero(t, infra.restartCalls)
	requireAmbiguousRestartFence(t, client, fixture, &batch)
	auditCount, auditErr := client.AuditLog.Query().Count(t.Context())
	require.NoError(t, auditErr)
	require.Zero(t, auditCount, "a retained PROCESSING fence is not a terminal failure")
}

func TestVMPowerWorker_RestartClaimTicketFailureRollsBackBeforeDispatch(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	client.Ticket.Use(enthook.On(
		enthook.FixedError(errors.New("ticket execution state unavailable")),
		ent.OpUpdate,
	))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "claim power dispatch")
	require.Contains(t, err.Error(), "ticket execution state unavailable")
	require.Zero(t, infra.restartCalls)
	event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, eventErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, ticketErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_RestartVMReadFailureRemainsRetryableBeforeDispatchClaim(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	var queryCount atomic.Int32
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if queryCount.Add(1) == 1 {
				return nil, errors.New("power vm query unavailable")
			}
			return next.Query(ctx, query)
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})

	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "pre-dispatch read failure must remain retryable")
	require.Contains(t, err.Error(), "query vm")
	require.Zero(t, infra.restartCalls)
	event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, eventErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, ticketErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_RestartRefreshCancellationFailsClosedAfterOneDispatch(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	infra := &restartRefreshFailureProvider{
		MockProvider: provider.NewMockProvider(),
		refreshErr:   errors.Join(errors.New("refresh interrupted"), context.Canceled),
	}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})

	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, err.Error(), "refresh live VM after successful restart")
	require.Equal(t, int32(1), infra.restartCalls.Load())
	requireAmbiguousRestartFence(t, client, fixture, nil)
}

func TestVMPowerWorker_FinalFailurePersistenceSnoozesAndReexecutionConverges(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		persistErr error
	}{
		{name: "ordinary persistence failure", persistErr: errors.New("event store remains unavailable")},
		{
			name:       "canceled persistence failure",
			persistErr: errors.Join(errors.New("event store interrupted"), context.Canceled),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixture(t)
			var failedWrites atomic.Int32
			client.DomainEvent.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					if failedWrites.Add(1) <= 2 {
						return nil, tc.persistErr
					}
					return next.Mutate(ctx, mutation)
				})
			}, ent.OpUpdate))
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			job := &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			}

			err := worker.Work(t.Context(), job)

			require.Error(t, err)
			var snoozeErr *river.JobSnoozeError
			require.ErrorAs(t, err, &snoozeErr)
			require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
			require.Zero(t, infra.stopCalls)
			event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, eventErr)
			require.Equal(t, domainevent.StatusPENDING, event.Status)

			require.NoError(t, worker.Work(t.Context(), job))
			require.Equal(t, 1, infra.stopCalls)
			event, eventErr = client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, eventErr)
			require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
			ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, ticketErr)
			require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
		})
	}
}

func TestVMPowerWorker_NonFinalPreDispatchReadErrorsRemainRetryable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		readErr    error
		wantCtxErr bool
	}{
		{name: "ordinary read failure", readErr: errors.New("event read unavailable")},
		{
			name:       "canceled read failure",
			readErr:    errors.Join(errors.New("event read interrupted"), context.Canceled),
			wantCtxErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixture(t)
			client.DomainEvent.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
				return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
					return nil, tc.readErr
				})
			}))
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})

			require.Error(t, err)
			if tc.wantCtxErr {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.Contains(t, err.Error(), "fetch domain event")
			}
			require.Zero(t, infra.stopCalls)
		})
	}
}

func TestVMPowerWorker_TerminalProjectionRepairFailuresRespectAttemptBudget(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		attempt int
	}{
		{name: "non-final delivery returns the first repair error", attempt: 1},
		{name: "final delivery snoozes while the bounded repair fails", attempt: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixture(t)
			batch := attachVMPowerBatchFixture(t, client, fixture)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusCOMPLETED).
				Save(t.Context())
			require.NoError(t, err)
			var failedWrites atomic.Int32
			client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					limit := int32(1)
					if tc.attempt == 3 {
						limit = 2
					}
					if failedWrites.Add(1) <= limit {
						return nil, errors.New("terminal ticket projection unavailable")
					}
					return next.Mutate(ctx, mutation)
				})
			}, ent.OpUpdate))
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			job := &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: tc.attempt, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			}

			err = worker.Work(t.Context(), job)

			require.Error(t, err)
			if tc.attempt == 3 {
				var snoozeErr *river.JobSnoozeError
				require.ErrorAs(t, err, &snoozeErr)
				require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
				ticket, ticketErr := client.Ticket.Get(t.Context(), fixture.ticketID)
				require.NoError(t, ticketErr)
				require.Equal(t, entticket.StatusAPPROVED, ticket.Status)

				require.NoError(t, worker.Work(t.Context(), job))
				ticket, ticketErr = client.Ticket.Get(t.Context(), fixture.ticketID)
				require.NoError(t, ticketErr)
				require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
				parent, parentErr := client.Ticket.Get(t.Context(), batch.parentTicketID)
				require.NoError(t, parentErr)
				require.Equal(t, entticket.StatusSUCCESS, parent.Status)
				projection, projectionErr := client.BatchTicket.Get(t.Context(), batch.parentTicketID)
				require.NoError(t, projectionErr)
				require.Equal(t, batchticket.StatusCOMPLETED, projection.Status)
			} else {
				require.Contains(t, err.Error(), "terminal ticket projection unavailable")
			}
			require.Zero(t, infra.stopCalls)
			event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, eventErr)
			require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
		})
	}
}

func TestFinalizePowerPreDispatchFailureOnLastAttempt_NeverGuessesProjection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		eventStatus  domainevent.Status
		ticketStatus entticket.Status
	}{
		{
			name:         "pending provenance has not been transactionally proved",
			eventStatus:  domainevent.StatusPENDING,
			ticketStatus: entticket.StatusAPPROVED,
		},
		{
			name:         "processing belongs to a possible claimant",
			eventStatus:  domainevent.StatusPROCESSING,
			ticketStatus: entticket.StatusEXECUTING,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(tc.eventStatus).
				Save(t.Context())
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(tc.ticketStatus).
				Save(t.Context())
			require.NoError(t, err)

			err = finalizePowerPreDispatchFailureOnLastAttempt(
				t.Context(),
				&river.Job[VMPowerArgs]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}},
				fixture.eventID,
				errors.New("restart dispatch claim outcome is unknown"),
			)
			require.Error(t, err)
			var snoozeErr *river.JobSnoozeError
			require.ErrorAs(t, err, &snoozeErr)
			require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)

			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, tc.eventStatus, event.Status)
			ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, tc.ticketStatus, ticket.Status)
		})
	}
}

func TestVMPowerWorker_FinalPreDispatchReadFailurePreservesProcessingRestartFence(t *testing.T) {
	t.Parallel()
	for _, failurePoint := range []string{"event fetch", "ticket lookup"} {
		t.Run(failurePoint, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusPROCESSING).
				Save(t.Context())
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(entticket.StatusEXECUTING).
				Save(t.Context())
			require.NoError(t, err)

			var queryCount atomic.Int32
			var workerFinished atomic.Bool
			switch failurePoint {
			case "event fetch":
				client.DomainEvent.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
					return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
						if queryCount.Add(1) == 1 {
							return nil, errors.New("restart event fetch unavailable")
						}
						return next.Query(ctx, query)
					})
				}))
			case "ticket lookup":
				client.Ticket.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
					return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
						if !workerFinished.Load() && queryCount.Add(1) == 1 {
							return nil, errors.New("restart ticket lookup unavailable")
						}
						return next.Query(ctx, query)
					})
				}))
			default:
				t.Fatalf("unknown failure point %q", failurePoint)
			}

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})
			workerFinished.Store(true)
			require.Error(t, err)
			if failurePoint == "event fetch" {
				var snoozeErr *river.JobSnoozeError
				require.ErrorAs(t, err, &snoozeErr)
				require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
			} else {
				var cancelErr *river.JobCancelError
				require.ErrorAs(t, err, &cancelErr)
				require.Zero(t, queryCount.Load(), "an existing restart fence must bypass ticket lookup")
			}
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)

			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusPROCESSING, event.Status)
			ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
		})
	}
}

func TestVMPowerWorker_FinalRestartDeliveryPreservesProcessingFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusPROCESSING,
		errors.New("restart claim persistence outcome unknown"),
	))

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already PROCESSING")
	require.Contains(t, err.Error(), "provider receipt")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMPowerWorker_MalformedPayloadCannotReleaseProcessingRestartFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForRawPayload(t, []byte(`{"operation":`))
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}})
	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)
}

func TestVMPowerWorker_ConcurrentRestartPendingDispatchCallsProviderOnce(t *testing.T) {
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	infra := &blockingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		entered:      make(chan struct{}),
		release:      make(chan struct{}),
	}
	defer infra.unblock()
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	var workers errgroup.Group

	workers.Go(func() error {
		firstResult <- worker.Work(t.Context(), &river.Job[VMPowerArgs]{
			JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
			Args:   VMPowerArgs{EventID: fixture.eventID},
		})
		return nil
	})

	select {
	case <-infra.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first restart provider call did not begin")
	}

	workers.Go(func() error {
		secondResult <- worker.Work(t.Context(), &river.Job[VMPowerArgs]{
			JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
			Args:   VMPowerArgs{EventID: fixture.eventID},
		})
		return nil
	})

	var secondErr error
	select {
	case secondErr = <-secondResult:
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate restart worker did not converge")
	}
	require.Error(t, secondErr)
	var secondCancelErr *river.JobCancelError
	require.ErrorAs(t, secondErr, &secondCancelErr)
	require.Equal(t, int32(1), infra.restartCalls.Load())

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)

	infra.unblock()
	var firstErr error
	select {
	case firstErr = <-firstResult:
	case <-time.After(5 * time.Second):
		t.Fatal("first restart worker did not finish")
	}
	require.NoError(t, firstErr)
	require.NoError(t, workers.Wait())
	require.Equal(t, int32(1), infra.restartCalls.Load())

	event, err = client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
	ticket, err = client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestVMPowerWorker_PreClaimFailureCannotClearConcurrentStartStopDispatch(t *testing.T) {
	for _, operation := range []string{powerOpStart, powerOpStop} {
		t.Run(operation, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, operation)
			infra := &blockingPowerProvider{
				MockProvider: provider.NewMockProvider(),
				entered:      make(chan struct{}),
				release:      make(chan struct{}),
			}
			defer infra.unblock()

			var failNextVMRead atomic.Bool
			client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
				return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
					if failNextVMRead.CompareAndSwap(true, false) {
						return nil, errors.New("losing pre-claim VM read unavailable")
					}
					return next.Query(ctx, query)
				})
			}))

			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			winnerResult := make(chan error, 1)
			var workers errgroup.Group
			workers.Go(func() error {
				winnerResult <- worker.Work(t.Context(), &river.Job[VMPowerArgs]{
					JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
					Args:   VMPowerArgs{EventID: fixture.eventID},
				})
				return nil
			})

			select {
			case <-infra.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("winning provider call did not begin")
			}
			failNextVMRead.Store(true)
			loserErr := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, loserErr)
			var snoozeErr *river.JobSnoozeError
			require.ErrorAs(t, loserErr, &snoozeErr)
			require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
			require.False(t, failNextVMRead.Load(), "losing delivery did not reach its pre-claim VM read")
			require.Equal(t, int32(1), infra.startCalls.Load()+infra.stopCalls.Load())

			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusPROCESSING, event.Status)
			ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, entticket.StatusEXECUTING, ticket.Status)

			infra.unblock()
			select {
			case winnerErr := <-winnerResult:
				require.NoError(t, winnerErr)
			case <-time.After(5 * time.Second):
				t.Fatal("winning provider delivery did not finish")
			}
			require.NoError(t, workers.Wait())
			require.Equal(t, int32(1), infra.startCalls.Load()+infra.stopCalls.Load())
			event, err = client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
			ticket, err = client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
		})
	}
}

func TestVMPowerWorker_OrphanedStartStopPreClaimFailureSnoozesThenResumes(t *testing.T) {
	for _, operation := range []string{powerOpStart, powerOpStop} {
		t.Run(operation, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, operation)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusPROCESSING).
				Save(t.Context())
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(entticket.StatusEXECUTING).
				Save(t.Context())
			require.NoError(t, err)

			var failFirstVMRead atomic.Bool
			failFirstVMRead.Store(true)
			client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
				return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
					if failFirstVMRead.CompareAndSwap(true, false) {
						return nil, errors.New("orphan recovery VM read unavailable")
					}
					return next.Query(ctx, query)
				})
			}))
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			job := &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			}

			firstErr := worker.Work(t.Context(), job)
			require.Error(t, firstErr)
			var snoozeErr *river.JobSnoozeError
			require.ErrorAs(t, firstErr, &snoozeErr)
			require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
			require.Zero(t, infra.startCalls+infra.stopCalls)
			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusPROCESSING, event.Status)

			require.NoError(t, worker.Work(t.Context(), job))
			require.Equal(t, 1, infra.startCalls+infra.stopCalls)
			event, err = client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
			ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, err)
			require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
		})
	}
}

func TestVMPowerWorker_ProcessingStartStopTargetRejectionConsumesAttemptsThenConverges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		target    string
		mode      string
	}{
		{name: "start deleting batch", operation: powerOpStart, target: "deleting", mode: "batch"},
		{name: "start missing direct", operation: powerOpStart, target: "missing", mode: "direct"},
		{name: "stop deleting standalone", operation: powerOpStop, target: "deleting", mode: "standalone"},
		{name: "stop missing batch", operation: powerOpStop, target: "missing", mode: "batch"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				client  *ent.Client
				fixture vmPowerWorkerFixture
				batch   *vmPowerBatchFixture
			)
			if tc.mode == "direct" {
				client, fixture = seedDirectVMPowerWorkerFixtureForOperation(t, tc.operation)
			} else {
				client, fixture = seedVMPowerWorkerFixtureForOperation(t, tc.operation)
				if tc.mode == "batch" {
					attached := attachVMPowerBatchFixture(t, client, fixture)
					batch = &attached
				}
			}

			ctx := t.Context()
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusPROCESSING).
				Save(ctx)
			require.NoError(t, err)
			if fixture.ticketID != "" {
				_, err = client.Ticket.UpdateOneID(fixture.ticketID).
					SetStatus(entticket.StatusEXECUTING).
					Save(ctx)
				require.NoError(t, err)
			}
			switch tc.target {
			case "deleting":
				_, err = client.VM.UpdateOneID(fixture.vmID).
					SetStatus(entvm.StatusDELETING).
					Save(ctx)
			case "missing":
				err = client.VM.DeleteOneID(fixture.vmID).Exec(ctx)
			default:
				t.Fatalf("unknown target mutation %q", tc.target)
			}
			require.NoError(t, err)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))
			nonFinal := &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			}

			err = worker.Work(ctx, nonFinal)
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.NotErrorAs(t, err, &cancelErr, "non-final target rejection must consume an attempt")
			var snoozeErr *river.JobSnoozeError
			require.NotErrorAs(t, err, &snoozeErr, "non-final target rejection must not snooze")
			require.Zero(t, infra.startCalls+infra.stopCalls+infra.restartCalls)
			requireVMPowerProcessingState(t, client, fixture, batch)
			auditCount, auditErr := client.AuditLog.Query().Count(ctx)
			require.NoError(t, auditErr)
			require.Zero(t, auditCount)

			final := &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			}
			err = worker.Work(ctx, final)
			require.Error(t, err)
			require.ErrorAs(t, err, &cancelErr)
			require.Zero(t, infra.startCalls+infra.stopCalls+infra.restartCalls)
			requireFailedVMPowerState(t, client, fixture, batch)
			auditRows, auditErr := client.AuditLog.Query().All(ctx)
			require.NoError(t, auditErr)
			require.Len(t, auditRows, 1)
			require.Equal(t, "vm."+tc.operation+"_failed", auditRows[0].Action)
		})
	}
}

func TestVMPowerWorker_FinalProcessingStartFailurePersistenceSnoozesThenConverges(t *testing.T) {
	t.Parallel()

	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpStart)
	ctx := t.Context()
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.VM.UpdateOneID(fixture.vmID).
		SetStatus(entvm.StatusDELETING).
		Save(ctx)
	require.NoError(t, err)

	var failedWrites atomic.Int32
	client.DomainEvent.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			domainEventMutation, ok := mutation.(*ent.DomainEventMutation)
			if ok {
				nextStatus, exists := domainEventMutation.Status()
				if exists && nextStatus == domainevent.StatusFAILED && failedWrites.Add(1) == 1 {
					return nil, errors.New("processing start failure persistence unavailable")
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))
	job := &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	}

	err = worker.Work(ctx, job)
	require.Error(t, err)
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
	require.Zero(t, infra.startCalls+infra.stopCalls+infra.restartCalls)
	requireVMPowerProcessingState(t, client, fixture, nil)
	auditCount, auditErr := client.AuditLog.Query().Count(ctx)
	require.NoError(t, auditErr)
	require.Zero(t, auditCount)

	err = worker.Work(ctx, job)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.startCalls+infra.stopCalls+infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
	auditRows, auditErr := client.AuditLog.Query().All(ctx)
	require.NoError(t, auditErr)
	require.Len(t, auditRows, 1)
	require.Equal(t, "vm.start_failed", auditRows[0].Action)
}

func TestVMPowerWorker_StartStopProcessingRetriesStillCallProvider(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{powerOpStart, powerOpStop} {
		t.Run(operation, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, operation)
			_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
				SetStatus(domainevent.StatusPROCESSING).
				Save(t.Context())
			require.NoError(t, err)
			_, err = client.Ticket.UpdateOneID(fixture.ticketID).
				SetStatus(entticket.StatusEXECUTING).
				Save(t.Context())
			require.NoError(t, err)

			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})
			require.NoError(t, err)
			require.Equal(t, 1, infra.startCalls+infra.stopCalls)
			require.Zero(t, infra.restartCalls)

			event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, err)
			require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
		})
	}
}

func TestVMPowerWorker_RestartTerminalPersistFailurePreservesBatchFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	batch := attachVMPowerBatchFixture(t, client, fixture)
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
	require.Equal(t, int32(1), mock.restartCalls.Load())
	requireAmbiguousRestartFence(t, client, fixture, &batch)

	// Even if a queue race invokes the same args again, the PROCESSING event is
	// the durable fence and the provider is not restarted.
	err = worker.Work(ctx, &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &cancelErr)
	require.Contains(t, err.Error(), "already PROCESSING")
	require.Equal(t, int32(1), mock.restartCalls.Load())
	requireAmbiguousRestartFence(t, client, fixture, &batch)
}

func TestVMPowerWorker_RestartProviderErrorsAreAtMostOnce(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		execErr error
	}{
		{
			name:    "worker cancellation",
			execErr: context.Canceled,
		},
		{
			name:    "provider deadline",
			execErr: context.DeadlineExceeded,
		},
		{
			name:    "provider 503",
			execErr: k8serrors.NewServiceUnavailable("restart response unavailable"),
		},
		{
			name:    "transport response loss",
			execErr: errors.New("transport connection reset after request write"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
			infra := &failingPowerProvider{
				MockProvider: provider.NewMockProvider(),
				execErr:      tc.execErr,
			}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
				Args:   VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			require.Equal(t, int32(1), infra.restartCalls.Load())
			requireAmbiguousRestartFence(t, client, fixture, nil)

			// A queue race or duplicate delivery sees the PROCESSING durable fence and
			// cannot invoke the non-idempotent provider operation a second time.
			err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				Args: VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, err)
			require.ErrorAs(t, err, &cancelErr)
			require.Contains(t, err.Error(), "already PROCESSING")
			require.Equal(t, int32(1), infra.restartCalls.Load())
			requireAmbiguousRestartFence(t, client, fixture, nil)
		})
	}
}

func TestVMPowerWorker_InvalidPayloadCancelsWithoutUnvalidatedProjectionWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		seedFixture func(t *testing.T) (*ent.Client, vmPowerWorkerFixture)
	}{
		{
			name: "malformed JSON",
			seedFixture: func(t *testing.T) (*ent.Client, vmPowerWorkerFixture) {
				return seedVMPowerWorkerFixtureForRawPayload(t, []byte(`{"operation":`))
			},
		},
		{
			name: "unknown operation",
			seedFixture: func(t *testing.T) (*ent.Client, vmPowerWorkerFixture) {
				return seedVMPowerWorkerFixtureForOperation(t, "hibernate")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, fixture := tc.seedFixture(t)
			infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

			err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
				Args: VMPowerArgs{EventID: fixture.eventID},
			})
			require.Error(t, err)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			require.Zero(t, infra.startCalls)
			require.Zero(t, infra.stopCalls)
			require.Zero(t, infra.restartCalls)
			event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, loadErr)
			require.Equal(t, domainevent.StatusPENDING, event.Status)
			ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
			require.NoError(t, loadErr)
			require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
		})
	}
}

func TestVMPowerWorker_PreDispatchRestartFailurePersistenceSnoozesWithoutDispatch(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	_, err := client.VM.UpdateOneID(fixture.vmID).SetStatus(entvm.StatusDELETING).Save(t.Context())
	require.NoError(t, err)
	var failedWrites atomic.Int32
	client.DomainEvent.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			domainEventMutation, ok := mutation.(*ent.DomainEventMutation)
			if ok {
				nextStatus, exists := domainEventMutation.Status()
				if exists && nextStatus == domainevent.StatusFAILED && failedWrites.Add(1) == 1 {
					return nil, errors.New("pre-dispatch restart failure persistence unavailable")
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), audit.NewLogger(client))
	job := &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	}

	err = worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
	require.Zero(t, infra.restartCalls)
	event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, eventErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	auditCount, auditErr := client.AuditLog.Query().Count(t.Context())
	require.NoError(t, auditErr)
	require.Zero(t, auditCount, "uncommitted failure persistence must not emit a terminal audit")

	err = worker.Work(t.Context(), job)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
	auditRows, auditErr := client.AuditLog.Query().All(t.Context())
	require.NoError(t, auditErr)
	require.Len(t, auditRows, 1)
	require.Equal(t, "vm.restart_failed", auditRows[0].Action)
}

func TestVMPowerWorker_DeterministicStartStopPersistenceSnoozesThenConverges(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{powerOpStart, powerOpStop} {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			client, fixture := seedVMPowerWorkerFixtureForOperation(t, operation)
			var failedWrites atomic.Int32
			client.DomainEvent.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					domainEventMutation, ok := mutation.(*ent.DomainEventMutation)
					if ok {
						nextStatus, exists := domainEventMutation.Status()
						if exists && nextStatus == domainevent.StatusFAILED && failedWrites.Add(1) == 1 {
							return nil, errors.New("deterministic power failure persistence unavailable")
						}
					}
					return next.Mutate(ctx, mutation)
				})
			})
			infra := &failingPowerProvider{
				MockProvider: provider.NewMockProvider(),
				execErr:      fmt.Errorf("does not support manual %s requests", operation),
			}
			worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
			job := &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}}

			err := worker.Work(t.Context(), job)
			var snoozeErr *river.JobSnoozeError
			require.ErrorAs(t, err, &snoozeErr)
			require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
			event, eventErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
			require.NoError(t, eventErr)
			require.Equal(t, domainevent.StatusPROCESSING, event.Status)

			err = worker.Work(t.Context(), job)
			var cancelErr *river.JobCancelError
			require.ErrorAs(t, err, &cancelErr)
			requireFailedVMPowerState(t, client, fixture, nil)
			if operation == powerOpStart {
				require.Equal(t, int32(2), infra.startCalls.Load())
				require.Zero(t, infra.stopCalls.Load())
			} else {
				require.Equal(t, int32(2), infra.stopCalls.Load())
				require.Zero(t, infra.startCalls.Load())
			}
			require.Zero(t, infra.restartCalls.Load())
		})
	}
}

func TestVMPowerWorker_RestartKnownNotAppliedPersistenceFailureBecomesAmbiguousFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusFAILED,
		errors.New("known-not-applied restart classification persistence unavailable"),
	))
	infra := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      errors.New("vm is not running"),
	}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	job := &river.Job[VMPowerArgs]{Args: VMPowerArgs{EventID: fixture.eventID}}

	err := worker.Work(t.Context(), job)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr))
	require.Contains(t, err.Error(), "persist FAILED status before cancelling power job")
	require.Equal(t, int32(1), infra.restartCalls.Load())
	requireAmbiguousRestartFence(t, client, fixture, nil)

	err = worker.Work(t.Context(), job)
	require.ErrorAs(t, err, &cancelErr)
	require.Contains(t, err.Error(), "already PROCESSING")
	require.Equal(t, int32(1), infra.restartCalls.Load())
	requireAmbiguousRestartFence(t, client, fixture, nil)
}

func TestCancelPowerWithoutRetry_CanceledContextUsesBoundedFailureFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	err = cancelPowerWithoutRetry(cancelledCtx, client, fixture.eventID, fixture.payload, powerOpStop, context.Canceled)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.ErrorIs(t, err, context.Canceled)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestCancelPowerWithoutRetry_TerminalCompletedConflictCancelsWithoutLoop(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusSUCCESS).
		Save(t.Context())
	require.NoError(t, err)

	err = cancelPowerWithoutRetry(t.Context(), client, fixture.eventID, fixture.payload, powerOpStop, errors.New("late failure"))
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "a permanent terminal conflict must not snooze forever")
	require.Contains(t, err.Error(), "outside the exact failure transition")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}

func TestCancelPowerWithoutRetry_TerminalCancelledConflictCancelsWithoutLoop(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusCANCELLED).
		Save(t.Context())
	require.NoError(t, err)

	err = cancelPowerWithoutRetry(t.Context(), client, fixture.eventID, fixture.payload, powerOpStop, errors.New("late failure"))
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr), "a permanent terminal conflict must not snooze forever")
	require.Contains(t, err.Error(), "outside the exact failure transition")

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCANCELLED, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusCANCELLED, ticket.Status)
}

func TestCancelPowerWithoutRetry_TerminalConflictDoesNotGuessProjection(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	var repairWrites atomic.Int32
	client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if repairWrites.Add(1) == 1 {
				return nil, errors.New("terminal projection repair unavailable")
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))

	err = cancelPowerWithoutRetry(t.Context(), client, fixture.eventID, fixture.payload, powerOpStop, errors.New("late failure"))
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.False(t, errors.As(err, &snoozeErr))

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, ticket.Status)

	require.Zero(t, repairWrites.Load())
}

func TestVMPowerWorker_SkipsTerminalEventWithoutExecutingProvider(t *testing.T) {
	t.Parallel()
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

func TestVMPowerWorker_FinalEventFetchFailureSnoozesWithoutGuessingBatchProjection(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	batch := attachVMPowerBatchFixture(t, client, fixture)
	var queryCount atomic.Int32
	client.DomainEvent.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if queryCount.Add(1) == 1 {
				return nil, errors.New("power event fetch unavailable")
			}
			return next.Query(ctx, query)
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)
	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
	parent, loadErr := client.Ticket.Get(t.Context(), batch.parentTicketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusEXECUTING, parent.Status)
}

func TestVMPowerWorker_FinalCanceledWorkerContextSnoozesWithoutProjectionWrites(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	err := worker.Work(cancelledCtx, &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Equal(t, powerFailureConvergenceSnooze, snoozeErr.Duration)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)
	event, loadErr := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, loadErr)
	require.Equal(t, domainevent.StatusPENDING, event.Status)
	ticket, loadErr := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, loadErr)
	require.Equal(t, entticket.StatusAPPROVED, ticket.Status)
}

func TestVMPowerWorker_FinalProcessingPersistFailureConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusPROCESSING,
		errors.New("power event processing persist unavailable"),
	))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist PROCESSING/EXECUTING status")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_FinalProcessingCancellationConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusPROCESSING,
		errors.Join(errors.New("power event processing interrupted"), context.Canceled),
	))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "ordinary final cancellation remains River-owned")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_FinalVMQueryFailureConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	var queryCount atomic.Int32
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if queryCount.Add(1) == 1 {
				return nil, errors.New("power vm query unavailable")
			}
			return next.Query(ctx, query)
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "query vm")
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_FinalRestartVMQueryFailurePreservesConcurrentProcessingFence(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixtureForOperation(t, powerOpRestart)
	var intercepted atomic.Bool
	client.VM.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if !intercepted.CompareAndSwap(false, true) {
				return next.Query(ctx, query)
			}
			transitionErr := withJobsTx(ctx, client, func(txClient *ent.Client) error {
				if _, updateErr := txClient.DomainEvent.UpdateOneID(fixture.eventID).
					SetStatus(domainevent.StatusPROCESSING).
					Save(ctx); updateErr != nil {
					return updateErr
				}
				if _, updateErr := txClient.Ticket.UpdateOneID(fixture.ticketID).
					SetStatus(entticket.StatusEXECUTING).
					Save(ctx); updateErr != nil {
					return updateErr
				}
				return nil
			})
			if transitionErr != nil {
				return nil, transitionErr
			}
			return nil, errors.New("restart VM query unavailable after concurrent claim")
		})
	}))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})

	require.Error(t, err)
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.Zero(t, infra.restartCalls)
	require.True(t, intercepted.Load())
	requireAmbiguousRestartFence(t, client, fixture, nil)
}

func TestVMPowerWorker_SnoozesTransientClusterErrorsWithoutFailingTicket(t *testing.T) {
	t.Parallel()
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
	client, fixture := seedVMPowerWorkerFixture(t)
	batch := attachVMPowerBatchFixture(t, client, fixture)
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
	require.Equal(t, int32(1), mock.stopCalls.Load())
	requireFailedVMPowerState(t, client, fixture, &batch)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		Args: VMPowerArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), mock.stopCalls.Load())
	requireFailedVMPowerState(t, client, fixture, &batch)

	vmRow, err := client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusRUNNING, vmRow.Status)
}

func TestVMPowerWorker_ContextCancellationDoesNotFailEventOrTicket(t *testing.T) {
	t.Parallel()
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

func TestVMPowerWorker_FinalProviderCancellationConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	infra := &failingPowerProvider{
		MockProvider: provider.NewMockProvider(),
		execErr:      context.Canceled,
	}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "idempotent stop cancellation remains River-owned")
	require.Equal(t, int32(1), infra.stopCalls.Load())
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_StopStatusPersistFailureReturnsRetryableError(t *testing.T) {
	t.Parallel()
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

func TestVMPowerWorker_FinalStopTerminalPersistFailureConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusCOMPLETED,
		errors.New("power event completed persist unavailable"),
	))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist terminal status after stop power event")
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "River owns finalization for an ordinary final error")
	require.Equal(t, 1, infra.stopCalls)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_StatusPersistCancellationReturnsContextError(t *testing.T) {
	t.Parallel()
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

func TestVMPowerWorker_FinalEventCompleteCancellationConvergesToFailed(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	var updateCount atomic.Int32
	client.DomainEvent.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if updateCount.Add(1) == 2 {
				return nil, errors.Join(errors.New("power event complete persist interrupted"), context.Canceled)
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)
	var cancelErr *river.JobCancelError
	require.False(t, errors.As(err, &cancelErr), "idempotent stop cancellation remains River-owned")
	require.Equal(t, 1, infra.stopCalls)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.restartCalls)
	requireFailedVMPowerState(t, client, fixture, nil)
}

func TestVMPowerWorker_FinalTerminalTicketCancellationRepairsProjection(t *testing.T) {
	t.Parallel()
	client, fixture := seedVMPowerWorkerFixture(t)
	_, err := client.DomainEvent.UpdateOneID(fixture.eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.UpdateOneID(fixture.ticketID).
		SetStatus(entticket.StatusEXECUTING).
		Save(t.Context())
	require.NoError(t, err)

	var updateCount atomic.Int32
	client.Ticket.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if updateCount.Add(1) == 1 {
				return nil, errors.Join(errors.New("terminal ticket persist interrupted"), context.Canceled)
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdate))
	infra := &countingPowerProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMPowerWorker(client, service.NewVMService(infra), nil)

	err = worker.Work(t.Context(), &river.Job[VMPowerArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   VMPowerArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, infra.startCalls)
	require.Zero(t, infra.stopCalls)
	require.Zero(t, infra.restartCalls)

	event, err := client.DomainEvent.Get(t.Context(), fixture.eventID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, event.Status)
	ticket, err := client.Ticket.Get(t.Context(), fixture.ticketID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, ticket.Status)
}
