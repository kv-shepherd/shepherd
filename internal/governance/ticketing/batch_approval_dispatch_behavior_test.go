package ticketing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	domainevent "kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/testutil"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func TestServiceApproveInitialPowerBatch_ClaimsParentAndRejectsPostClaimGraphTampering(t *testing.T) {
	client, pool := testutil.OpenEntPostgresWithPool(t, "batch_power_claim")
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create River migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate River schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create River client: %v", err)
	}

	const (
		parentID      = "batch-power-parent"
		parentEventID = "batch-power-parent-event"
		childID       = "batch-power-child"
		childEventID  = "batch-power-child-event"
	)
	childPayload, err := (domain.VMPowerPayload{
		VMID:         "vm-power-1",
		VMName:       "vm-power-1",
		ClusterID:    "cluster-power",
		Namespace:    "team-power",
		Operation:    "start",
		Actor:        "power-requester",
		DispatchMode: domain.VMPowerDispatchTicket,
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal power child payload: %v", err)
	}
	parentPayload, err := (domain.BatchVMRequestPayload{
		Operation:   "POWER_START",
		SubmittedBy: "power-requester",
		Items: []domain.BatchVMItemPayload{{
			VMID:      "vm-power-1",
			VMName:    "vm-power-1",
			ClusterID: "cluster-power",
			Namespace: "team-power",
			Operation: "start",
		}},
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal power parent payload: %v", err)
	}
	if _, createParentEventErr := client.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayload).
		SetCreatedBy("power-requester").
		Save(t.Context()); createParentEventErr != nil {
		t.Fatalf("create power parent event: %v", createParentEventErr)
	}
	if _, createChildEventErr := client.DomainEvent.Create().
		SetID(childEventID).
		SetEventType(string(domain.EventVMStartRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-power-1").
		SetPayload(childPayload).
		SetCreatedBy("power-requester").
		Save(t.Context()); createChildEventErr != nil {
		t.Fatalf("create power child event: %v", createChildEventErr)
	}
	if _, createParentTicketErr := client.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetRequester("power-requester").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		Save(t.Context()); createParentTicketErr != nil {
		t.Fatalf("create power parent ticket: %v", createParentTicketErr)
	}
	if _, createChildTicketErr := client.Ticket.Create().
		SetID(childID).
		SetEventID(childEventID).
		SetRequester("power-requester").
		SetParentTicketID(parentID).
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		Save(t.Context()); createChildTicketErr != nil {
		t.Fatalf("create power child ticket: %v", createChildTicketErr)
	}
	if _, createProjectionErr := client.BatchTicket.Create().
		SetID(parentID).
		SetBatchType(entbatchticket.BatchTypeBATCH_POWER).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(entbatchticket.StatusPENDING_APPROVAL).
		SetCreatedBy("power-requester").
		Save(t.Context()); createProjectionErr != nil {
		t.Fatalf("create power batch projection: %v", createProjectionErr)
	}

	service := NewService(client, nil, usecase.NewApprovalAtomicWriter(pool, riverClient))
	if approveErr := service.Approve(t.Context(), parentID, "power-approver", ExecutionOptions{}); approveErr != nil {
		t.Fatalf("Approve() initial power batch error = %v", approveErr)
	}

	parent, err := client.Ticket.Get(t.Context(), parentID)
	if err != nil {
		t.Fatalf("load claimed power parent: %v", err)
	}
	if parent.Status != entticket.StatusEXECUTING || parent.Approver != "power-approver" {
		t.Fatalf("power parent decision = status %s approver %q, want EXECUTING/power-approver", parent.Status, parent.Approver)
	}
	parentEvent, err := client.DomainEvent.Get(t.Context(), parentEventID)
	if err != nil {
		t.Fatalf("load claimed power parent event: %v", err)
	}
	if parentEvent.Status != domainevent.StatusPROCESSING {
		t.Fatalf("power parent event status = %s, want PROCESSING", parentEvent.Status)
	}
	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("load pending power child: %v", err)
	}
	if child.Status != entticket.StatusPENDING || child.AttemptCount != 0 {
		t.Fatalf("power child state = %s attempt %d, want untouched PENDING/0", child.Status, child.AttemptCount)
	}
	projection, err := client.BatchTicket.Get(t.Context(), parentID)
	if err != nil {
		t.Fatalf("load power projection: %v", err)
	}
	if projection.Status != entbatchticket.StatusIN_PROGRESS || projection.PendingCount != 1 {
		t.Fatalf("power projection = status %s pending %d, want IN_PROGRESS/1", projection.Status, projection.PendingCount)
	}

	jobsResult, err := riverClient.JobList(t.Context(), river.NewJobListParams().Kinds(jobs.BatchApprovalDispatchJobKind))
	if err != nil {
		t.Fatalf("list batch approval dispatchers: %v", err)
	}
	if len(jobsResult.Jobs) != 1 {
		t.Fatalf("batch approval dispatcher jobs = %d, want 1", len(jobsResult.Jobs))
	}
	var args jobs.BatchApprovalDispatchArgs
	if decodeErr := json.Unmarshal(jobsResult.Jobs[0].EncodedArgs, &args); decodeErr != nil {
		t.Fatalf("decode batch approval dispatcher args: %v", decodeErr)
	}
	if args.BatchID != parentID || jobsResult.Jobs[0].Queue != jobs.BatchApprovalDispatchJobKind {
		t.Fatalf("dispatcher = batch %q queue %q, want %q/%q", args.BatchID, jobsResult.Jobs[0].Queue, parentID, jobs.BatchApprovalDispatchJobKind)
	}

	tamperedPayload, err := (domain.VMPowerPayload{
		VMID:         "vm-power-1",
		VMName:       "vm-power-1",
		ClusterID:    "cluster-power",
		Namespace:    "team-power",
		Operation:    "start",
		Actor:        "different-actor",
		DispatchMode: domain.VMPowerDispatchTicket,
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal tampered power child payload: %v", err)
	}
	if _, tamperErr := pool.Exec(t.Context(), `UPDATE domain_events SET payload = $1 WHERE id = $2`, tamperedPayload, childEventID); tamperErr != nil {
		t.Fatalf("tamper claimed power child actor: %v", tamperErr)
	}
	err = service.DispatchBatchApproval(t.Context(), parentID)
	var consistency *jobs.BatchApprovalDispatchConsistencyError
	if !errors.As(err, &consistency) {
		t.Fatalf("post-claim tamper dispatch error = %v, want *BatchApprovalDispatchConsistencyError", err)
	}
	child, err = client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("reload child after rejected tamper: %v", err)
	}
	if child.Status != entticket.StatusPENDING || child.AttemptCount != 0 {
		t.Fatalf("tampered child state = %s attempt %d, want unchanged PENDING/0", child.Status, child.AttemptCount)
	}
	var powerJobs int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'vm_power'`).Scan(&powerJobs); err != nil {
		t.Fatalf("count vm_power jobs after rejected tamper: %v", err)
	}
	if powerJobs != 0 {
		t.Fatalf("vm_power jobs after rejected tamper = %d, want 0", powerJobs)
	}
}

func TestServiceBatchApprovalDispatch_MissingDurableOwnerIsConsistencyViolation(t *testing.T) {
	client, _ := testutil.OpenEntPostgresWithPool(t, "batch_dispatch_missing_owner")
	service := NewService(client, nil, &fakeAtomicWriter{client: client})

	for _, call := range []struct {
		name string
		run  func() error
	}{
		{name: "dispatch", run: func() error { return service.DispatchBatchApproval(t.Context(), "missing-parent") }},
		{name: "finalize", run: func() error {
			return service.FailPendingBatchApprovalDispatch(t.Context(), "missing-parent", errors.New("exhausted"))
		}},
	} {
		t.Run(call.name, func(t *testing.T) {
			var consistency *jobs.BatchApprovalDispatchConsistencyError
			if err := call.run(); !errors.As(err, &consistency) {
				t.Fatalf("missing owner error = %v, want *BatchApprovalDispatchConsistencyError", err)
			}
			if consistency.BatchID != "missing-parent" || !strings.Contains(consistency.Detail, "missing") {
				t.Fatalf("missing owner consistency = %+v", consistency)
			}
		})
	}
}

func TestServiceDispatchBatchApproval_PermanentChildFailureCommitsPairAndParentSummary(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_permanent")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	writer := &fakeAtomicWriter{
		client:    client,
		deleteErr: apperrors.BadRequest(apperrors.CodeValidationFailed, "delete request is no longer valid"),
	}
	service := NewService(client, nil, writer)

	if err := service.DispatchBatchApproval(t.Context(), "  "+fixture.parentTicketID+"  "); err != nil {
		t.Fatalf("DispatchBatchApproval() permanent child failure error = %v", err)
	}
	if !writer.deleteCalled {
		t.Fatal("DispatchBatchApproval() did not invoke the child delete writer")
	}

	assertFailedBatchApprovalChild(t, client, fixture, domain.BatchApprovalDispatchFailureValidation)
	assertFailedBatchApprovalParent(t, client, fixture)
}

func TestServiceDispatchBatchApproval_TransientFailureRemainsPendingUntilFinalizer(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_transient")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	transientErr := errors.New("delete provider temporarily unavailable")
	writer := &fakeAtomicWriter{client: client, deleteErr: transientErr}
	service := NewService(client, nil, writer)

	err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
	if !errors.Is(err, transientErr) {
		t.Fatalf("DispatchBatchApproval() error = %v, want transient child error", err)
	}
	if !writer.deleteCalled {
		t.Fatal("DispatchBatchApproval() did not invoke the child delete writer")
	}
	assertPendingBatchApprovalDispatch(t, client, fixture)

	if err := service.FailPendingBatchApprovalDispatch(t.Context(), fixture.parentTicketID, transientErr); err != nil {
		t.Fatalf("FailPendingBatchApprovalDispatch() error = %v", err)
	}
	assertFailedBatchApprovalChild(t, client, fixture, domain.BatchApprovalDispatchFailureExhausted)
	assertFailedBatchApprovalParent(t, client, fixture)

	// Finalizer delivery is idempotent once no PENDING child remains.
	if err := service.FailPendingBatchApprovalDispatch(t.Context(), fixture.parentTicketID, transientErr); err != nil {
		t.Fatalf("second FailPendingBatchApprovalDispatch() error = %v", err)
	}
	assertFailedBatchApprovalChild(t, client, fixture, domain.BatchApprovalDispatchFailureExhausted)
}

func TestServiceBatchApprovalDispatch_UntrustedCauseIsNotPersisted(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_secret_hygiene")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	const sentinel = "postgres://svc:super-secret@example.com/shepherd Bearer token-secret-value"
	providerErr := errors.New(sentinel)
	writer := &fakeAtomicWriter{client: client, deleteErr: providerErr}
	service := NewService(client, nil, writer)

	if err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID); !errors.Is(err, providerErr) {
		t.Fatalf("DispatchBatchApproval() error = %v, want provider error", err)
	}
	if err := service.FailPendingBatchApprovalDispatch(t.Context(), fixture.parentTicketID, providerErr); err != nil {
		t.Fatalf("FailPendingBatchApprovalDispatch() error = %v", err)
	}
	child, err := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if err != nil {
		t.Fatalf("load failed child: %v", err)
	}
	if child.RejectReason != domain.BatchApprovalDispatchFailureExhausted {
		t.Fatalf("child reject_reason = %q, want stable exhausted reason", child.RejectReason)
	}
	if strings.Contains(child.RejectReason, sentinel) || strings.Contains(child.RejectReason, "super-secret") || strings.Contains(child.RejectReason, "token-secret-value") {
		t.Fatalf("child reject_reason leaked untrusted cause: %q", child.RejectReason)
	}
}

func TestServiceDispatchBatchApproval_ReconcilesFailedParentWithPendingChild(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_failed_parent_reconcile")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	if _, err := client.Ticket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed dispatch parent: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed dispatch parent event: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entbatchticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed dispatch projection: %v", err)
	}

	transientErr := errors.New("delete provider remains unavailable")
	writer := &fakeAtomicWriter{client: client, deleteErr: transientErr}
	service := NewService(client, nil, writer)
	err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
	if !errors.Is(err, transientErr) {
		t.Fatalf("DispatchBatchApproval() error = %v, want transient child error", err)
	}
	if !writer.deleteCalled {
		t.Fatal("reconciled dispatcher did not invoke the pending child writer")
	}
	assertPendingBatchApprovalDispatch(t, client, fixture)
}

func TestServiceFailPendingBatchApprovalDispatch_ReconcilesFailedParentWithPendingChild(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_finalizer_failed_parent_reconcile")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	if _, err := client.Ticket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed finalizer parent: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed finalizer parent event: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entbatchticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed finalizer projection: %v", err)
	}

	cause := errors.New("dispatcher attempts exhausted")
	service := NewService(client, nil, &fakeAtomicWriter{client: client})
	if err := service.FailPendingBatchApprovalDispatch(t.Context(), fixture.parentTicketID, cause); err != nil {
		t.Fatalf("FailPendingBatchApprovalDispatch() error = %v", err)
	}
	assertFailedBatchApprovalChild(t, client, fixture, domain.BatchApprovalDispatchFailureExhausted)
	assertFailedBatchApprovalParent(t, client, fixture)
}

func TestServiceBatchApprovalDispatch_TerminalParentWithPendingChildIsUnchangedConsistencyViolation(t *testing.T) {
	tests := []struct {
		name             string
		parentStatus     entticket.Status
		parentEvent      domainevent.Status
		projectionStatus entbatchticket.Status
	}{
		{
			name:             "successful parent",
			parentStatus:     entticket.StatusSUCCESS,
			parentEvent:      domainevent.StatusCOMPLETED,
			projectionStatus: entbatchticket.StatusCOMPLETED,
		},
		{
			name:             "cancelled parent",
			parentStatus:     entticket.StatusCANCELLED,
			parentEvent:      domainevent.StatusCANCELLED,
			projectionStatus: entbatchticket.StatusCANCELLED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fixture := seedBatchDeleteApprovalFixture(t, "batch_terminal_parent_pending_child")
			prepareBatchApprovalDispatcherFixture(t, client, fixture)
			if _, err := client.Ticket.UpdateOneID(fixture.parentTicketID).
				SetStatus(tt.parentStatus).
				Save(t.Context()); err != nil {
				t.Fatalf("seed terminal dispatch parent: %v", err)
			}
			if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
				SetStatus(tt.parentEvent).
				Save(t.Context()); err != nil {
				t.Fatalf("seed terminal dispatch parent event: %v", err)
			}
			if _, err := client.BatchTicket.UpdateOneID(fixture.parentTicketID).
				SetStatus(tt.projectionStatus).
				Save(t.Context()); err != nil {
				t.Fatalf("seed terminal dispatch projection: %v", err)
			}

			parentBefore, parentEventBefore, childBefore, childEventBefore, projectionBefore :=
				loadBatchApprovalDispatchFixtureState(t, client, fixture)
			writer := &fakeAtomicWriter{client: client}
			service := NewService(client, nil, writer)
			assertBatchApprovalDispatchConsistencyError(
				t,
				service.DispatchBatchApproval(t.Context(), fixture.parentTicketID),
				fixture.parentTicketID,
				tt.parentStatus,
				tt.parentEvent,
				1,
				1,
			)
			if writer.deleteCalled {
				t.Fatal("dispatcher invoked a child writer under a terminal parent")
			}
			assertBatchApprovalDispatchConsistencyError(
				t,
				service.FailPendingBatchApprovalDispatch(
					t.Context(),
					fixture.parentTicketID,
					errors.New("dispatcher exhausted"),
				),
				fixture.parentTicketID,
				tt.parentStatus,
				tt.parentEvent,
				1,
				1,
			)
			assertBatchApprovalDispatchFixtureUnchanged(
				t,
				client,
				fixture,
				parentBefore,
				parentEventBefore,
				childBefore,
				childEventBefore,
				projectionBefore,
			)
		})
	}
}

func TestServiceBatchApprovalDispatch_TerminalOutcomeMismatchIsUnchangedConsistencyViolation(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_terminal_outcome_mismatch")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	if _, err := client.Ticket.UpdateOneID(fixture.childTicketID).
		SetStatus(entticket.StatusSUCCESS).
		Save(t.Context()); err != nil {
		t.Fatalf("seed successful dispatch child: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(fixture.childEventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed completed dispatch child event: %v", err)
	}
	if _, err := client.Ticket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed mismatched failed dispatch parent: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed mismatched failed dispatch parent event: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(fixture.parentTicketID).
		SetSuccessCount(1).
		SetPendingCount(0).
		SetStatus(entbatchticket.StatusCOMPLETED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed terminal dispatch projection: %v", err)
	}

	parentBefore, parentEventBefore, childBefore, childEventBefore, projectionBefore :=
		loadBatchApprovalDispatchFixtureState(t, client, fixture)
	writer := &fakeAtomicWriter{client: client}
	service := NewService(client, nil, writer)
	assertBatchApprovalDispatchConsistencyError(
		t,
		service.DispatchBatchApproval(t.Context(), fixture.parentTicketID),
		fixture.parentTicketID,
		entticket.StatusFAILED,
		domainevent.StatusFAILED,
		0,
		0,
	)
	assertBatchApprovalDispatchConsistencyError(
		t,
		service.FailPendingBatchApprovalDispatch(
			t.Context(),
			fixture.parentTicketID,
			errors.New("dispatcher exhausted"),
		),
		fixture.parentTicketID,
		entticket.StatusFAILED,
		domainevent.StatusFAILED,
		0,
		0,
	)
	if writer.deleteCalled {
		t.Fatal("terminal dispatcher invoked a child writer")
	}
	assertBatchApprovalDispatchFixtureUnchanged(
		t,
		client,
		fixture,
		parentBefore,
		parentEventBefore,
		childBefore,
		childEventBefore,
		projectionBefore,
	)
}

func TestServiceDispatchBatchApproval_StaleChildEventDoesNotPartiallyPersistFailure(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_stale_child")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	if _, err := client.DomainEvent.UpdateOneID(fixture.childEventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); err != nil {
		t.Fatalf("make child event stale: %v", err)
	}
	writer := &fakeAtomicWriter{
		client:    client,
		deleteErr: apperrors.BadRequest(apperrors.CodeValidationFailed, "delete request is stale"),
	}
	service := NewService(client, nil, writer)

	err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
	if err == nil || !strings.Contains(err.Error(), "expected 1 row, got 0") {
		t.Fatalf("DispatchBatchApproval() stale child event error = %v, want row conflict", err)
	}
	child, getErr := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if getErr != nil {
		t.Fatalf("load child after stale event conflict: %v", getErr)
	}
	if child.Status != entticket.StatusPENDING || child.Approver != "" || child.RejectReason != "" {
		t.Fatalf("child after stale event conflict = status %s approver %q reason %q, want untouched PENDING", child.Status, child.Approver, child.RejectReason)
	}
	if child.AttemptCount != 0 || child.LastAttemptAt != nil {
		t.Fatalf("child attempt after stale event conflict = (%d, %v), want untouched", child.AttemptCount, child.LastAttemptAt)
	}
	childEvent, getErr := client.DomainEvent.Get(t.Context(), fixture.childEventID)
	if getErr != nil {
		t.Fatalf("load stale child event: %v", getErr)
	}
	if childEvent.Status != domainevent.StatusCOMPLETED {
		t.Fatalf("stale child event status = %s, want COMPLETED", childEvent.Status)
	}
	assertPendingBatchApprovalParent(t, client, fixture)
}

func TestServiceDispatchBatchApproval_StaleParentEventFailsBeforeChildMutation(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_stale_parent")
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); err != nil {
		t.Fatalf("make parent event stale: %v", err)
	}
	writer := &fakeAtomicWriter{
		client:    client,
		deleteErr: apperrors.BadRequest(apperrors.CodeValidationFailed, "delete child permanently invalid"),
	}
	service := NewService(client, nil, writer)

	err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
	var consistency *jobs.BatchApprovalDispatchConsistencyError
	if !errors.As(err, &consistency) {
		t.Fatalf("DispatchBatchApproval() stale parent event error = %v, want *BatchApprovalDispatchConsistencyError", err)
	}
	if consistency.BatchID != fixture.parentTicketID || consistency.ParentStatus != "EXECUTING" ||
		consistency.ParentEventStatus != "COMPLETED" || consistency.PendingChildren != 1 || consistency.ActiveChildren != 1 {
		t.Fatalf("stale parent consistency error = %+v, want executing/completed parent with one active child", consistency)
	}
	if writer.deleteCalled {
		t.Fatal("stale parent guard ran child writer before rejecting inconsistent state")
	}
	child, getErr := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if getErr != nil {
		t.Fatalf("load child after stale parent guard: %v", getErr)
	}
	if child.Status != entticket.StatusPENDING || child.Approver != "" || child.RejectReason != "" || child.AttemptCount != 0 {
		t.Fatalf("child after stale parent guard = %+v, want untouched PENDING", child)
	}
	childEvent, getErr := client.DomainEvent.Get(t.Context(), fixture.childEventID)
	if getErr != nil {
		t.Fatalf("load child event after stale parent guard: %v", getErr)
	}
	if childEvent.Status != domainevent.StatusPENDING {
		t.Fatalf("child event after stale parent guard = %s, want PENDING", childEvent.Status)
	}

	parent, getErr := client.Ticket.Get(t.Context(), fixture.parentTicketID)
	if getErr != nil {
		t.Fatalf("load parent after stale event sync: %v", getErr)
	}
	if parent.Status != entticket.StatusEXECUTING {
		t.Fatalf("parent status after stale event sync = %s, want EXECUTING transaction rollback", parent.Status)
	}
	parentEvent, getErr := client.DomainEvent.Get(t.Context(), fixture.parentEventID)
	if getErr != nil {
		t.Fatalf("load stale parent event: %v", getErr)
	}
	if parentEvent.Status != domainevent.StatusCOMPLETED {
		t.Fatalf("stale parent event status = %s, want COMPLETED", parentEvent.Status)
	}
	projection, getErr := client.BatchTicket.Get(t.Context(), fixture.parentTicketID)
	if getErr != nil {
		t.Fatalf("load projection after stale parent event: %v", getErr)
	}
	if projection.Status != entbatchticket.StatusIN_PROGRESS || projection.PendingCount != 1 || projection.FailedCount != 0 {
		t.Fatalf("projection after stale parent event = status %s pending %d failed %d, want unchanged IN_PROGRESS", projection.Status, projection.PendingCount, projection.FailedCount)
	}
}

func TestServiceDispatchBatchApproval_MismatchedChildEventIdentityFailsBeforeMutation(t *testing.T) {
	client, fixture := seedBatchDeleteApprovalFixtureWithChildPayload(
		t,
		"batch_dispatch_child_identity",
		[]byte(`{"vm_id":"another-vm","operation":"delete"}`),
	)
	prepareBatchApprovalDispatcherFixture(t, client, fixture)
	parentBefore, parentEventBefore, childBefore, childEventBefore, projectionBefore :=
		loadBatchApprovalDispatchFixtureState(t, client, fixture)
	writer := &fakeAtomicWriter{
		client:    client,
		deleteErr: apperrors.BadRequest(apperrors.CodeValidationFailed, "must never dispatch"),
	}
	service := NewService(client, nil, writer)

	err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
	assertBatchApprovalDispatchConsistencyError(
		t,
		err,
		fixture.parentTicketID,
		entticket.StatusEXECUTING,
		domainevent.StatusPROCESSING,
		1,
		1,
	)
	if writer.deleteCalled {
		t.Fatal("child identity guard ran the delete writer")
	}
	assertBatchApprovalDispatchFixtureUnchanged(
		t,
		client,
		fixture,
		parentBefore,
		parentEventBefore,
		childBefore,
		childEventBefore,
		projectionBefore,
	)
}

func TestServiceDispatchBatchApproval_MismatchedChildOwnershipFailsBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(*testing.T, *ent.Client, batchDecisionFixture)
	}{
		{
			name: "requester differs from parent",
			corrupt: func(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
				t.Helper()
				event, err := client.DomainEvent.Get(t.Context(), fixture.childEventID)
				if err != nil {
					t.Fatalf("load child event before ownership corruption: %v", err)
				}
				if err := client.Ticket.DeleteOneID(fixture.childTicketID).Exec(t.Context()); err != nil {
					t.Fatalf("delete child ticket before ownership corruption: %v", err)
				}
				if err := client.DomainEvent.DeleteOneID(fixture.childEventID).Exec(t.Context()); err != nil {
					t.Fatalf("delete child event before ownership corruption: %v", err)
				}
				if _, err := client.DomainEvent.Create().
					SetID(event.ID).
					SetEventType(event.EventType).
					SetAggregateType(event.AggregateType).
					SetAggregateID(event.AggregateID).
					SetPayload(event.Payload).
					SetStatus(event.Status).
					SetCreatedBy("another-requester").
					Save(t.Context()); err != nil {
					t.Fatalf("recreate child event with foreign requester: %v", err)
				}
				if _, err := client.Ticket.Create().
					SetID(fixture.childTicketID).
					SetEventID(fixture.childEventID).
					SetRequester("another-requester").
					SetParentTicketID(fixture.parentTicketID).
					SetStatus(entticket.StatusPENDING).
					SetOperationType(entticket.OperationTypeDELETE).
					Save(t.Context()); err != nil {
					t.Fatalf("recreate child ticket with foreign requester: %v", err)
				}
			},
		},
		{
			name: "operation differs from parent",
			corrupt: func(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
				t.Helper()
				if _, err := client.Ticket.UpdateOneID(fixture.childTicketID).
					SetOperationType(entticket.OperationTypeMODIFY).
					Save(t.Context()); err != nil {
					t.Fatalf("corrupt child operation: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fixture := seedBatchDeleteApprovalFixture(t, "batch_dispatch_child_ownership")
			prepareBatchApprovalDispatcherFixture(t, client, fixture)
			tt.corrupt(t, client, fixture)
			parentBefore, parentEventBefore, childBefore, childEventBefore, projectionBefore :=
				loadBatchApprovalDispatchFixtureState(t, client, fixture)
			writer := &fakeAtomicWriter{
				client:    client,
				deleteErr: apperrors.BadRequest(apperrors.CodeValidationFailed, "must never dispatch"),
			}
			service := NewService(client, nil, writer)

			err := service.DispatchBatchApproval(t.Context(), fixture.parentTicketID)
			assertBatchApprovalDispatchConsistencyError(
				t,
				err,
				fixture.parentTicketID,
				entticket.StatusEXECUTING,
				domainevent.StatusPROCESSING,
				1,
				1,
			)
			if writer.deleteCalled {
				t.Fatal("child ownership guard ran the delete writer")
			}
			assertBatchApprovalDispatchFixtureUnchanged(
				t,
				client,
				fixture,
				parentBefore,
				parentEventBefore,
				childBefore,
				childEventBefore,
				projectionBefore,
			)
		})
	}
}

func prepareBatchApprovalDispatcherFixture(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
	t.Helper()
	seedBatchDecisionProjection(t, client, fixture.parentTicketID)
	if _, err := client.Ticket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entticket.StatusEXECUTING).
		SetApprover("admin-dispatch").
		Save(t.Context()); err != nil {
		t.Fatalf("claim batch parent ticket: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(fixture.parentEventID).
		SetStatus(domainevent.StatusPROCESSING).
		Save(t.Context()); err != nil {
		t.Fatalf("claim batch parent event: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(fixture.parentTicketID).
		SetStatus(entbatchticket.StatusIN_PROGRESS).
		Save(t.Context()); err != nil {
		t.Fatalf("claim batch projection: %v", err)
	}
}

func assertFailedBatchApprovalChild(
	t *testing.T,
	client *ent.Client,
	fixture batchDecisionFixture,
	wantReason string,
) {
	t.Helper()
	const wantApprover = "admin-dispatch"
	child, err := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if err != nil {
		t.Fatalf("load failed batch child: %v", err)
	}
	if child.Status != entticket.StatusFAILED || child.Approver != wantApprover {
		t.Fatalf("failed child decision = status %s approver %q, want FAILED/%q", child.Status, child.Approver, wantApprover)
	}
	if !strings.Contains(child.RejectReason, wantReason) {
		t.Fatalf("failed child reason = %q, want %q", child.RejectReason, wantReason)
	}
	if child.AttemptCount != 1 || child.LastAttemptAt == nil || child.LastAttemptAt.IsZero() {
		t.Fatalf("failed child attempt = (%d, %v), want initial logical attempt metadata", child.AttemptCount, child.LastAttemptAt)
	}
	childEvent, err := client.DomainEvent.Get(t.Context(), fixture.childEventID)
	if err != nil {
		t.Fatalf("load failed child event: %v", err)
	}
	if childEvent.Status != domainevent.StatusFAILED {
		t.Fatalf("failed child event status = %s, want FAILED", childEvent.Status)
	}
}

func assertFailedBatchApprovalParent(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
	t.Helper()
	parent, err := client.Ticket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load failed batch parent: %v", err)
	}
	if parent.Status != entticket.StatusFAILED {
		t.Fatalf("failed parent status = %s, want FAILED", parent.Status)
	}
	parentEvent, err := client.DomainEvent.Get(t.Context(), fixture.parentEventID)
	if err != nil {
		t.Fatalf("load failed parent event: %v", err)
	}
	if parentEvent.Status != domainevent.StatusFAILED {
		t.Fatalf("failed parent event status = %s, want FAILED", parentEvent.Status)
	}
	projection, err := client.BatchTicket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load failed batch projection: %v", err)
	}
	if projection.Status != entbatchticket.StatusFAILED || projection.FailedCount != 1 || projection.PendingCount != 0 {
		t.Fatalf("failed projection = status %s failed %d pending %d, want FAILED/1/0", projection.Status, projection.FailedCount, projection.PendingCount)
	}
}

func assertPendingBatchApprovalDispatch(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
	t.Helper()
	child, err := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if err != nil {
		t.Fatalf("load pending batch child: %v", err)
	}
	if child.Status != entticket.StatusPENDING || child.Approver != "" || child.RejectReason != "" || child.AttemptCount != 0 || child.LastAttemptAt != nil {
		t.Fatalf("pending child mutated by transient failure: %+v", child)
	}
	childEvent, err := client.DomainEvent.Get(t.Context(), fixture.childEventID)
	if err != nil {
		t.Fatalf("load pending child event: %v", err)
	}
	if childEvent.Status != domainevent.StatusPENDING {
		t.Fatalf("pending child event status = %s, want PENDING", childEvent.Status)
	}
	assertPendingBatchApprovalParent(t, client, fixture)
}

func assertPendingBatchApprovalParent(t *testing.T, client *ent.Client, fixture batchDecisionFixture) {
	t.Helper()
	parent, err := client.Ticket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load executing batch parent: %v", err)
	}
	if parent.Status != entticket.StatusEXECUTING || parent.Approver != "admin-dispatch" {
		t.Fatalf("pending parent = status %s approver %q, want EXECUTING/admin-dispatch", parent.Status, parent.Approver)
	}
	parentEvent, err := client.DomainEvent.Get(t.Context(), fixture.parentEventID)
	if err != nil {
		t.Fatalf("load processing parent event: %v", err)
	}
	if parentEvent.Status != domainevent.StatusPROCESSING {
		t.Fatalf("pending parent event status = %s, want PROCESSING", parentEvent.Status)
	}
	projection, err := client.BatchTicket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load in-progress batch projection: %v", err)
	}
	if projection.Status != entbatchticket.StatusIN_PROGRESS || projection.PendingCount != 1 || projection.FailedCount != 0 {
		t.Fatalf("pending projection = status %s pending %d failed %d, want IN_PROGRESS/1/0", projection.Status, projection.PendingCount, projection.FailedCount)
	}
}

func assertBatchApprovalDispatchConsistencyError(
	t *testing.T,
	err error,
	wantBatchID string,
	wantParent entticket.Status,
	wantParentEvent domainevent.Status,
	wantPending int,
	wantActive int,
) {
	t.Helper()
	var consistency *jobs.BatchApprovalDispatchConsistencyError
	if !errors.As(err, &consistency) {
		t.Fatalf("batch dispatch error = %v, want *BatchApprovalDispatchConsistencyError", err)
	}
	if consistency.BatchID != wantBatchID || consistency.ParentStatus != wantParent.String() ||
		consistency.ParentEventStatus != wantParentEvent.String() ||
		consistency.PendingChildren != wantPending || consistency.ActiveChildren != wantActive {
		t.Fatalf(
			"batch dispatch consistency error = %+v, want batch=%q parent=%s/%s pending=%d active=%d",
			consistency,
			wantBatchID,
			wantParent,
			wantParentEvent,
			wantPending,
			wantActive,
		)
	}
}

func loadBatchApprovalDispatchFixtureState(
	t *testing.T,
	client *ent.Client,
	fixture batchDecisionFixture,
) (loadedParent *ent.Ticket, loadedParentEvent *ent.DomainEvent, loadedChild *ent.Ticket, loadedChildEvent *ent.DomainEvent, loadedProjection *ent.BatchTicket) {
	t.Helper()
	parent, err := client.Ticket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load batch dispatch parent snapshot: %v", err)
	}
	parentEvent, err := client.DomainEvent.Get(t.Context(), fixture.parentEventID)
	if err != nil {
		t.Fatalf("load batch dispatch parent event snapshot: %v", err)
	}
	child, err := client.Ticket.Get(t.Context(), fixture.childTicketID)
	if err != nil {
		t.Fatalf("load batch dispatch child snapshot: %v", err)
	}
	childEvent, err := client.DomainEvent.Get(t.Context(), fixture.childEventID)
	if err != nil {
		t.Fatalf("load batch dispatch child event snapshot: %v", err)
	}
	projection, err := client.BatchTicket.Get(t.Context(), fixture.parentTicketID)
	if err != nil {
		t.Fatalf("load batch dispatch projection snapshot: %v", err)
	}
	return parent, parentEvent, child, childEvent, projection
}

func assertBatchApprovalDispatchFixtureUnchanged(
	t *testing.T,
	client *ent.Client,
	fixture batchDecisionFixture,
	parentBefore *ent.Ticket,
	parentEventBefore *ent.DomainEvent,
	childBefore *ent.Ticket,
	childEventBefore *ent.DomainEvent,
	projectionBefore *ent.BatchTicket,
) {
	t.Helper()
	parentAfter, parentEventAfter, childAfter, childEventAfter, projectionAfter :=
		loadBatchApprovalDispatchFixtureState(t, client, fixture)
	if parentAfter.Status != parentBefore.Status || parentAfter.Approver != parentBefore.Approver ||
		parentAfter.RejectReason != parentBefore.RejectReason || !parentAfter.UpdatedAt.Equal(parentBefore.UpdatedAt) {
		t.Fatalf("batch dispatch parent changed: before=%+v after=%+v", parentBefore, parentAfter)
	}
	if parentEventAfter.Status != parentEventBefore.Status {
		t.Fatalf("batch dispatch parent event changed: before=%+v after=%+v", parentEventBefore, parentEventAfter)
	}
	if childAfter.Status != childBefore.Status || childAfter.Approver != childBefore.Approver ||
		childAfter.RejectReason != childBefore.RejectReason || childAfter.AttemptCount != childBefore.AttemptCount ||
		!childAfter.UpdatedAt.Equal(childBefore.UpdatedAt) {
		t.Fatalf("batch dispatch child changed: before=%+v after=%+v", childBefore, childAfter)
	}
	if childEventAfter.Status != childEventBefore.Status {
		t.Fatalf("batch dispatch child event changed: before=%+v after=%+v", childEventBefore, childEventAfter)
	}
	if projectionAfter.Status != projectionBefore.Status ||
		projectionAfter.ChildCount != projectionBefore.ChildCount ||
		projectionAfter.SuccessCount != projectionBefore.SuccessCount ||
		projectionAfter.FailedCount != projectionBefore.FailedCount ||
		projectionAfter.PendingCount != projectionBefore.PendingCount ||
		!projectionAfter.UpdatedAt.Equal(projectionBefore.UpdatedAt) {
		t.Fatalf("batch dispatch projection changed: before=%+v after=%+v", projectionBefore, projectionAfter)
	}
}
