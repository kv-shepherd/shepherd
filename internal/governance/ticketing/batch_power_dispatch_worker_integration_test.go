package ticketing

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

type batchPowerDispatchWorkerIntegrationProvider struct {
	*provider.MockProvider
	startCalls atomic.Int32
}

func (p *batchPowerDispatchWorkerIntegrationProvider) StartVM(
	ctx context.Context,
	cluster, namespace, name string,
) error {
	p.startCalls.Add(1)
	return p.MockProvider.StartVM(ctx, cluster, namespace, name)
}

func TestBatchPowerDispatchWorkers_ParentApprovalConvergesTicketBackedChild(t *testing.T) {
	ctx := t.Context()
	client, pool := newTicketingE2EStore(t, "batch_power_dispatch_worker_integration")
	riverClient := newTicketingE2ERiverClient(t, pool)

	const (
		systemID      = "batch-power-system"
		serviceID     = "batch-power-service"
		vmID          = "batch-power-vm"
		vmName        = "batch-power-vm-01"
		clusterID     = "batch-power-cluster"
		namespace     = "batch-power-namespace"
		requester     = "batch-power-requester"
		approver      = "batch-power-approver"
		parentID      = "batch-power-parent"
		parentEventID = "batch-power-parent-event"
		childID       = "batch-power-child"
		childEventID  = "batch-power-child-event"
	)

	system, err := client.System.Create().
		SetID(systemID).
		SetName("batchpwr-sys").
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	svc, err := client.Service.Create().
		SetID(serviceID).
		SetName("batchpwr-svc").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, createVMErr := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace(namespace).
		SetClusterID(clusterID).
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy(requester).
		SetService(svc).
		Save(ctx); createVMErr != nil {
		t.Fatalf("create VM: %v", createVMErr)
	}

	powerPayload := domain.VMPowerPayload{
		VMID:         vmID,
		VMName:       vmName,
		ClusterID:    clusterID,
		Namespace:    namespace,
		Operation:    "start",
		Actor:        requester,
		DispatchMode: domain.VMPowerDispatchTicket,
	}
	childPayload, err := powerPayload.ToJSON()
	if err != nil {
		t.Fatalf("marshal child power payload: %v", err)
	}
	parentPayload, err := (domain.BatchVMRequestPayload{
		Operation:   "POWER_START",
		SubmittedBy: requester,
		Items: []domain.BatchVMItemPayload{{
			VMID:      vmID,
			VMName:    vmName,
			ClusterID: clusterID,
			Namespace: namespace,
			Operation: "start",
		}},
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal parent batch payload: %v", err)
	}

	if _, createParentEventErr := client.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayload).
		SetCreatedBy(requester).
		Save(ctx); createParentEventErr != nil {
		t.Fatalf("create parent event: %v", createParentEventErr)
	}
	if _, createChildEventErr := client.DomainEvent.Create().
		SetID(childEventID).
		SetEventType(string(domain.EventVMStartRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(childPayload).
		SetCreatedBy(requester).
		Save(ctx); createChildEventErr != nil {
		t.Fatalf("create child event: %v", createChildEventErr)
	}
	if _, createParentTicketErr := client.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetRequester(requester).
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("start batch").
		Save(ctx); createParentTicketErr != nil {
		t.Fatalf("create parent ticket: %v", createParentTicketErr)
	}
	if _, createChildTicketErr := client.Ticket.Create().
		SetID(childID).
		SetEventID(childEventID).
		SetRequester(requester).
		SetParentTicketID(parentID).
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("start VM").
		Save(ctx); createChildTicketErr != nil {
		t.Fatalf("create child ticket: %v", createChildTicketErr)
	}
	if _, createProjectionErr := client.BatchTicket.Create().
		SetID(parentID).
		SetBatchType(batchticket.BatchTypeBATCH_POWER).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(batchticket.StatusPENDING_APPROVAL).
		SetCreatedBy(requester).
		SetReason("start batch").
		Save(ctx); createProjectionErr != nil {
		t.Fatalf("create batch projection: %v", createProjectionErr)
	}

	mockProvider := &batchPowerDispatchWorkerIntegrationProvider{
		MockProvider: provider.NewMockProvider(),
	}
	mockProvider.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: namespace,
		Cluster:   clusterID,
		Status:    domain.VMStatusStopped,
	}})
	vmService := service.NewVMService(mockProvider)
	ticketService := NewService(client, nil, usecase.NewApprovalAtomicWriter(pool, riverClient))

	if approveErr := ticketService.Approve(ctx, parentID, approver, ExecutionOptions{}); approveErr != nil {
		t.Fatalf("approve batch parent: %v", approveErr)
	}
	dispatchJobs, err := riverClient.JobList(
		ctx,
		river.NewJobListParams().Kinds(jobs.BatchApprovalDispatchJobKind),
	)
	if err != nil {
		t.Fatalf("list batch dispatcher jobs: %v", err)
	}
	if len(dispatchJobs.Jobs) != 1 {
		t.Fatalf("batch dispatcher jobs = %d, want 1", len(dispatchJobs.Jobs))
	}
	var dispatchArgs jobs.BatchApprovalDispatchArgs
	if decodeErr := json.Unmarshal(dispatchJobs.Jobs[0].EncodedArgs, &dispatchArgs); decodeErr != nil {
		t.Fatalf("decode batch dispatcher args: %v", decodeErr)
	}
	if dispatchArgs.BatchID != parentID {
		t.Fatalf("batch dispatcher parent = %q, want %q", dispatchArgs.BatchID, parentID)
	}
	if dispatchErr := jobs.NewBatchApprovalDispatchWorker(ticketService).Work(
		ctx,
		&river.Job[jobs.BatchApprovalDispatchArgs]{
			JobRow: dispatchJobs.Jobs[0],
			Args:   dispatchArgs,
		},
	); dispatchErr != nil {
		t.Fatalf("BatchApprovalDispatchWorker.Work(): %v", dispatchErr)
	}

	parent, err := client.Ticket.Get(ctx, parentID)
	if err != nil {
		t.Fatalf("load dispatched parent ticket: %v", err)
	}
	child, err := client.Ticket.Get(ctx, childID)
	if err != nil {
		t.Fatalf("load dispatched child ticket: %v", err)
	}
	if parent.Status != entticket.StatusEXECUTING || child.Status != entticket.StatusAPPROVED {
		t.Fatalf(
			"dispatched ticket states = parent:%s child:%s, want EXECUTING/APPROVED",
			parent.Status,
			child.Status,
		)
	}
	if parent.Approver != approver || child.Approver != approver || child.Approver != parent.Approver {
		t.Fatalf(
			"dispatch approvers = parent:%q child:%q, want matching %q",
			parent.Approver,
			child.Approver,
			approver,
		)
	}
	if child.AttemptCount != 1 || child.LastAttemptAt == nil || child.LastAttemptAt.IsZero() {
		t.Fatalf(
			"child attempt provenance = count:%d last_at:%v, want 1/non-zero",
			child.AttemptCount,
			child.LastAttemptAt,
		)
	}

	childEvent, err := client.DomainEvent.Get(ctx, childEventID)
	if err != nil {
		t.Fatalf("load dispatched child event: %v", err)
	}
	var persistedPowerPayload domain.VMPowerPayload
	if decodeErr := json.Unmarshal(childEvent.Payload, &persistedPowerPayload); decodeErr != nil {
		t.Fatalf("decode persisted child power payload: %v", decodeErr)
	}
	if persistedPowerPayload.DispatchMode != domain.VMPowerDispatchTicket {
		t.Fatalf(
			"persisted dispatch mode = %q, want %q",
			persistedPowerPayload.DispatchMode,
			domain.VMPowerDispatchTicket,
		)
	}

	powerJobs, err := riverClient.JobList(
		ctx,
		river.NewJobListParams().Kinds(jobs.VMPowerArgs{}.Kind()),
	)
	if err != nil {
		t.Fatalf("list vm_power jobs: %v", err)
	}
	if len(powerJobs.Jobs) != 1 {
		t.Fatalf("vm_power jobs = %d, want 1", len(powerJobs.Jobs))
	}
	var powerArgs jobs.VMPowerArgs
	if decodeErr := json.Unmarshal(powerJobs.Jobs[0].EncodedArgs, &powerArgs); decodeErr != nil {
		t.Fatalf("decode vm_power args: %v", decodeErr)
	}
	if powerArgs.EventID != childEventID {
		t.Fatalf("vm_power event = %q, want %q", powerArgs.EventID, childEventID)
	}
	powerJob := &river.Job[jobs.VMPowerArgs]{
		JobRow: powerJobs.Jobs[0],
		Args:   powerArgs,
	}
	powerWorker := jobs.NewVMPowerWorker(client, vmService, nil)
	if workErr := powerWorker.Work(ctx, powerJob); workErr != nil {
		t.Fatalf("VMPowerWorker.Work(): %v", workErr)
	}
	// A duplicate delivery must repair/check the terminal projection without a
	// second provider side effect.
	if workErr := powerWorker.Work(ctx, powerJob); workErr != nil {
		t.Fatalf("duplicate VMPowerWorker.Work(): %v", workErr)
	}
	if got := mockProvider.startCalls.Load(); got != 1 {
		t.Fatalf("provider StartVM calls = %d, want exactly 1", got)
	}

	child, err = client.Ticket.Get(ctx, childID)
	if err != nil {
		t.Fatalf("reload terminal child ticket: %v", err)
	}
	parent, err = client.Ticket.Get(ctx, parentID)
	if err != nil {
		t.Fatalf("reload terminal parent ticket: %v", err)
	}
	childEvent, err = client.DomainEvent.Get(ctx, childEventID)
	if err != nil {
		t.Fatalf("reload terminal child event: %v", err)
	}
	parentEvent, err := client.DomainEvent.Get(ctx, parentEventID)
	if err != nil {
		t.Fatalf("reload terminal parent event: %v", err)
	}
	projection, err := client.BatchTicket.Get(ctx, parentID)
	if err != nil {
		t.Fatalf("reload terminal batch projection: %v", err)
	}
	if child.Status != entticket.StatusSUCCESS || childEvent.Status != domainevent.StatusCOMPLETED {
		t.Fatalf(
			"terminal child state = ticket:%s event:%s, want SUCCESS/COMPLETED",
			child.Status,
			childEvent.Status,
		)
	}
	if parent.Status != entticket.StatusSUCCESS || parentEvent.Status != domainevent.StatusCOMPLETED {
		t.Fatalf(
			"terminal parent state = ticket:%s event:%s, want SUCCESS/COMPLETED",
			parent.Status,
			parentEvent.Status,
		)
	}
	if parent.Approver != approver || child.Approver != approver || child.Approver != parent.Approver {
		t.Fatalf(
			"terminal approvers = parent:%q child:%q, want matching %q",
			parent.Approver,
			child.Approver,
			approver,
		)
	}
	if child.AttemptCount != 1 || child.LastAttemptAt == nil || child.LastAttemptAt.IsZero() {
		t.Fatalf(
			"terminal child attempt provenance = count:%d last_at:%v, want 1/non-zero",
			child.AttemptCount,
			child.LastAttemptAt,
		)
	}
	if projection.Status != batchticket.StatusCOMPLETED ||
		projection.ChildCount != 1 ||
		projection.SuccessCount != 1 ||
		projection.FailedCount != 0 ||
		projection.PendingCount != 0 {
		t.Fatalf(
			"terminal projection = status:%s children:%d success:%d failed:%d pending:%d, want COMPLETED/1/1/0/0",
			projection.Status,
			projection.ChildCount,
			projection.SuccessCount,
			projection.FailedCount,
			projection.PendingCount,
		)
	}
	vmRow, err := client.VM.Get(ctx, vmID)
	if err != nil {
		t.Fatalf("reload powered VM: %v", err)
	}
	if vmRow.Status != entvm.StatusRUNNING {
		t.Fatalf("powered VM status = %s, want RUNNING", vmRow.Status)
	}
}
