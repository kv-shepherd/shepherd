package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func createValidBatchCreateChildEvent(
	t *testing.T,
	client *ent.Client,
	aggregateID string,
	status domainevent.Status,
) *ent.DomainEvent {
	t.Helper()
	if strings.TrimSpace(aggregateID) == "" {
		aggregateID = "service-" + uuid.NewString()
	}
	payload, err := (domain.VMCreationPayload{ServiceID: aggregateID}).ToJSON()
	require.NoError(t, err)
	event, err := client.DomainEvent.Create().
		SetID("ev-child-" + uuid.NewString()).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID(aggregateID).
		SetPayload(payload).
		SetStatus(status).
		SetCreatedBy("seed").
		Save(t.Context())
	require.NoError(t, err)
	return event
}

func TestTicketStatusForTerminalDomainEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     domainevent.Status
		wantStatus entticket.Status
		wantOK     bool
	}{
		{
			name:       "completed",
			status:     domainevent.StatusCOMPLETED,
			wantStatus: entticket.StatusSUCCESS,
			wantOK:     true,
		},
		{
			name:       "failed",
			status:     domainevent.StatusFAILED,
			wantStatus: entticket.StatusFAILED,
			wantOK:     true,
		},
		{
			name:       "cancelled",
			status:     domainevent.StatusCANCELLED,
			wantStatus: entticket.StatusCANCELLED,
			wantOK:     true,
		},
		{
			name:   "processing",
			status: domainevent.StatusPROCESSING,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotOK := ticketStatusForTerminalDomainEvent(tc.status)
			require.Equal(t, tc.wantOK, gotOK)
			require.Equal(t, tc.wantStatus, gotStatus)
		})
	}
}

func TestPersistProcessingEventAndExecutingTicketByEventTransitionsTogether(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "processing_event_ticket_success")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	err = persistProcessingEventAndExecutingTicketByEvent(ctx, client, event.ID)
	require.NoError(t, err)

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
}

func TestPersistProcessingEventLocksTicketBeforeEvent(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "processing_ticket_before_event_lock")
	ctx := t.Context()
	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("service-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	blocker, err := client.Tx(ctx)
	require.NoError(t, err)
	blockerOpen := true
	defer func() {
		if blockerOpen {
			_ = blocker.Rollback()
		}
	}()
	blockerPID, err := blocker.QueryIntContext(ctx, `SELECT pg_backend_pid()`)
	require.NoError(t, err)
	_, err = blocker.QueryIntContext(ctx, `SELECT 1 FROM tickets WHERE id = $1 FOR UPDATE`, ticket.ID)
	require.NoError(t, err)

	persistDone := make(chan error, 1)
	pools, err := worker.NewPools(ctx, worker.PoolConfig{GeneralPoolSize: 1, K8sPoolSize: 1})
	require.NoError(t, err)
	t.Cleanup(pools.Shutdown)
	require.NoError(t, pools.General.Submit(ctx, func(taskCtx context.Context) {
		persistDone <- persistProcessingEventAndExecutingTicketByEvent(taskCtx, client, event.ID)
	}))

	probe, err := client.Tx(ctx)
	require.NoError(t, err)
	probeOpen := true
	defer func() {
		if probeOpen {
			_ = probe.Rollback()
		}
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		blockedCount, queryErr := probe.QueryIntContext(ctx, `
SELECT COUNT(*)::integer
FROM pg_stat_activity
WHERE pid <> $1
  AND $1 = ANY(pg_blocking_pids(pid))
`, blockerPID)
		require.NoError(t, queryErr)
		if blockedCount > 0 {
			break
		}
		select {
		case persistErr := <-persistDone:
			t.Fatalf("processing helper completed before blocking on ticket: %v", persistErr)
		case <-deadline.C:
			t.Fatal("processing helper did not block on the ticket lock")
		case <-ticker.C:
		}
	}
	require.NoError(t, probe.Rollback())
	probeOpen = false

	eventObserver, err := client.Tx(ctx)
	require.NoError(t, err)
	eventObserverOpen := true
	defer func() {
		if eventObserverOpen {
			_ = eventObserver.Rollback()
		}
	}()
	require.NoError(t, eventObserver.ExecContext(ctx, `SET LOCAL lock_timeout = '300ms'`))
	_, err = eventObserver.QueryIntContext(ctx, `SELECT 1 FROM domain_events WHERE id = $1 FOR UPDATE`, event.ID)
	require.NoError(t, err, "event must remain unlocked while the helper waits for its ticket")
	require.NoError(t, eventObserver.Rollback())
	eventObserverOpen = false

	require.NoError(t, blocker.Rollback())
	blockerOpen = false
	select {
	case persistErr := <-persistDone:
		require.NoError(t, persistErr)
	case <-time.After(5 * time.Second):
		t.Fatal("processing helper did not finish after the ticket lock was released")
	}

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)
}

func TestPersistProcessingEventWithoutTicketRemainsSupported(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "processing_event_without_ticket")
	ctx := t.Context()
	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType(string(domain.EventVMStopRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	err = persistProcessingEventAndMaybeExecutingTicketByEvent(ctx, client, event.ID, false)
	require.NoError(t, err)
	refreshed, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshed.Status)
	require.False(t, client.Ticket.Query().Where(entticket.EventIDEQ(event.ID)).ExistX(ctx))
}

func TestPersistProcessingEventRejectsDuplicateTicketOwnership(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "processing_event_duplicate_tickets")
	ctx := t.Context()
	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("service-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	for range 2 {
		_, err = client.Ticket.Create().
			SetID("ticket-" + uuid.NewString()).
			SetEventID(event.ID).
			SetRequester("seed").
			SetStatus(entticket.StatusPENDING).
			SetOperationType(entticket.OperationTypeCREATE).
			SetReason("create").
			Save(ctx)
		require.NoError(t, err)
	}

	err = persistProcessingEventAndExecutingTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected at most 1 row, got 2")
	refreshed, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPENDING, refreshed.Status)
	statuses, err := client.Ticket.Query().Where(entticket.EventIDEQ(event.ID)).Select(entticket.FieldStatus).Strings(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"PENDING", "PENDING"}, statuses)
}

func TestPersistProcessingEventAndExecutingTicketByEventRollsBackEventOnTicketFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "processing_event_ticket_failure")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	client.Ticket.Use(enthook.On(
		enthook.FixedError(errors.New("ticket executing persist unavailable")),
		ent.OpUpdate,
	))

	err = persistProcessingEventAndExecutingTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ticket executing persist unavailable")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPENDING, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusPENDING, refreshedTicket.Status)
}

func TestPersistProcessingEventAndExecutingTicketByEventRejectsNonPendingEvent(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "processing_event_ticket_non_pending_event")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	err = persistProcessingEventAndExecutingTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 row, got 0")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusPENDING, refreshedTicket.Status)
}

func TestPersistProcessingEventAndExecutingTicketByEventRollsBackEventWhenTicketMissing(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "processing_event_ticket_missing_ticket")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	err = persistProcessingEventAndExecutingTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 row, got 0")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPENDING, refreshedEvent.Status)
}

func TestPersistCompletedEventAndTicketByEventRejectsNonProcessingEvent(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "completed_event_ticket_non_processing_event")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	err = persistCompletedEventAndTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 row, got 0")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
}

func TestPersistFailedEventAndTicketByEventRejectsTerminalEvent(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "failed_event_ticket_terminal_event")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	err = persistFailedEventAndTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 row, got 0")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCOMPLETED, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
}

func TestSyncParentBatchStatus_RecalculatesBatchAggregates(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_status")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(2).
		SetPendingCount(2).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	for _, childStatus := range []entticket.Status{
		entticket.StatusSUCCESS,
		entticket.StatusFAILED,
	} {
		childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

		_, childTicketErr := client.Ticket.Create().
			SetID("ticket-child-" + uuid.NewString()).
			SetEventID(childEvent.ID).
			SetRequester("seed").
			SetStatus(childStatus).
			SetOperationType(entticket.OperationTypeCREATE).
			SetParentTicketID(parentTicket.ID).
			SetReason("child").
			Save(ctx)
		require.NoError(t, childTicketErr)
	}

	require.NoError(t, SyncParentBatchStatus(ctx, client, parentTicket.ID))

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusFAILED, refreshedParent.Status)

	refreshedEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusFAILED, refreshedEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 2, projection.ChildCount)
	require.Equal(t, 1, projection.SuccessCount)
	require.Equal(t, 1, projection.FailedCount)
	require.Equal(t, 0, projection.PendingCount)
	require.Equal(t, batchticket.StatusPARTIAL_SUCCESS, projection.Status)
}

func TestSyncParentBatchStatusFailsClosedOnInvalidParentIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		parentIsChild     bool
		wrongEventBinding bool
		omitProjection    bool
	}{
		{name: "parent is another ticket child", parentIsChild: true},
		{name: "parent event aggregate is mismatched", wrongEventBinding: true},
		{name: "parent projection is missing", omitProjection: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := testutil.OpenEntPostgres(t, "sync_parent_identity_guard")
			ctx := t.Context()
			parentID := "ticket-parent-" + uuid.NewString()
			aggregateID := parentID
			if tc.wrongEventBinding {
				aggregateID = "another-batch-" + uuid.NewString()
			}
			parentEvent, err := client.DomainEvent.Create().
				SetID("ev-parent-" + uuid.NewString()).
				SetEventType("BATCH_CREATE_REQUESTED").
				SetAggregateType("batch").
				SetAggregateID(aggregateID).
				SetPayload([]byte(`{}`)).
				SetStatus(domainevent.StatusPROCESSING).
				SetCreatedBy("seed").
				Save(ctx)
			require.NoError(t, err)

			parentCreate := client.Ticket.Create().
				SetID(parentID).
				SetEventID(parentEvent.ID).
				SetRequester("seed").
				SetStatus(entticket.StatusEXECUTING).
				SetOperationType(entticket.OperationTypeCREATE).
				SetReason("batch create")
			if tc.parentIsChild {
				parentCreate = parentCreate.SetParentTicketID("unrelated-parent-" + uuid.NewString())
			}
			parent, err := parentCreate.Save(ctx)
			require.NoError(t, err)

			if !tc.omitProjection {
				_, err = client.BatchTicket.Create().
					SetID(parentID).
					SetBatchType(batchticket.BatchTypeBATCH_CREATE).
					SetChildCount(1).
					SetPendingCount(1).
					SetStatus(batchticket.StatusIN_PROGRESS).
					SetCreatedBy("seed").
					SetReason("batch create").
					Save(ctx)
				require.NoError(t, err)
			}

			childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusCOMPLETED)
			child, err := client.Ticket.Create().
				SetID("ticket-child-" + uuid.NewString()).
				SetEventID(childEvent.ID).
				SetRequester("seed").
				SetStatus(entticket.StatusSUCCESS).
				SetOperationType(entticket.OperationTypeCREATE).
				SetParentTicketID(parentID).
				SetReason("child").
				Save(ctx)
			require.NoError(t, err)

			parentBefore, err := client.Ticket.Get(ctx, parent.ID)
			require.NoError(t, err)
			eventBefore, err := client.DomainEvent.Get(ctx, parentEvent.ID)
			require.NoError(t, err)
			childBefore, err := client.Ticket.Get(ctx, child.ID)
			require.NoError(t, err)
			var projectionBefore *ent.BatchTicket
			if !tc.omitProjection {
				projectionBefore, err = client.BatchTicket.Get(ctx, parentID)
				require.NoError(t, err)
			}

			err = SyncParentBatchStatus(ctx, client, parentID)
			require.Error(t, err)

			parentAfter, err := client.Ticket.Get(ctx, parent.ID)
			require.NoError(t, err)
			require.Equal(t, parentBefore.Status, parentAfter.Status)
			require.Equal(t, parentBefore.UpdatedAt, parentAfter.UpdatedAt)
			eventAfter, err := client.DomainEvent.Get(ctx, parentEvent.ID)
			require.NoError(t, err)
			require.Equal(t, eventBefore.Status, eventAfter.Status)
			childAfter, err := client.Ticket.Get(ctx, child.ID)
			require.NoError(t, err)
			require.Equal(t, childBefore.Status, childAfter.Status)
			require.Equal(t, childBefore.UpdatedAt, childAfter.UpdatedAt)
			if projectionBefore != nil {
				projectionAfter, projectionErr := client.BatchTicket.Get(ctx, parentID)
				require.NoError(t, projectionErr)
				require.Equal(t, projectionBefore.Status, projectionAfter.Status)
				require.Equal(t, projectionBefore.ChildCount, projectionAfter.ChildCount)
				require.Equal(t, projectionBefore.SuccessCount, projectionAfter.SuccessCount)
				require.Equal(t, projectionBefore.FailedCount, projectionAfter.FailedCount)
				require.Equal(t, projectionBefore.PendingCount, projectionAfter.PendingCount)
				require.Equal(t, projectionBefore.UpdatedAt, projectionAfter.UpdatedAt)
			} else {
				_, projectionErr := client.BatchTicket.Get(ctx, parentID)
				require.True(t, ent.IsNotFound(projectionErr))
			}
		})
	}
}

func TestSyncParentBatchStatusFailsClosedOnCorruptedChildIdentity(t *testing.T) {
	t.Parallel()
	for _, corruption := range []string{
		"operation",
		"event type",
		"aggregate type",
		"payload target",
	} {
		corruption := corruption
		t.Run(corruption, func(t *testing.T) {
			t.Parallel()
			client := testutil.OpenEntPostgres(t, "sync_parent_child_identity_guard")
			ctx := t.Context()
			parentID := "ticket-parent-" + uuid.NewString()
			parentEvent, err := client.DomainEvent.Create().
				SetID("ev-parent-" + uuid.NewString()).
				SetEventType(string(domain.EventBatchCreateRequested)).
				SetAggregateType("batch").
				SetAggregateID(parentID).
				SetPayload([]byte(`{}`)).
				SetStatus(domainevent.StatusPROCESSING).
				SetCreatedBy("seed").
				Save(ctx)
			require.NoError(t, err)
			parent, err := client.Ticket.Create().
				SetID(parentID).
				SetEventID(parentEvent.ID).
				SetRequester("seed").
				SetStatus(entticket.StatusEXECUTING).
				SetOperationType(entticket.OperationTypeCREATE).
				SetReason("batch create").
				Save(ctx)
			require.NoError(t, err)
			_, err = client.BatchTicket.Create().
				SetID(parentID).
				SetBatchType(batchticket.BatchTypeBATCH_CREATE).
				SetChildCount(2).
				SetPendingCount(2).
				SetStatus(batchticket.StatusIN_PROGRESS).
				SetCreatedBy("seed").
				SetReason("batch create").
				Save(ctx)
			require.NoError(t, err)

			targetID := "service-" + uuid.NewString()
			payloadTargetID := targetID
			eventType := string(domain.EventVMCreationRequested)
			aggregateType := "vm"
			operation := entticket.OperationTypeCREATE
			switch corruption {
			case "operation":
				operation = entticket.OperationTypeMODIFY
			case "event type":
				eventType = string(domain.EventVMDeletionRequested)
			case "aggregate type":
				aggregateType = "batch"
			case "payload target":
				payloadTargetID = "another-service-" + uuid.NewString()
			default:
				t.Fatalf("unsupported corruption %q", corruption)
			}
			payload, err := (domain.VMCreationPayload{ServiceID: payloadTargetID}).ToJSON()
			require.NoError(t, err)
			corruptedEvent, err := client.DomainEvent.Create().
				SetID("ev-child-corrupted-" + uuid.NewString()).
				SetEventType(eventType).
				SetAggregateType(aggregateType).
				SetAggregateID(targetID).
				SetPayload(payload).
				SetStatus(domainevent.StatusCOMPLETED).
				SetCreatedBy("seed").
				Save(ctx)
			require.NoError(t, err)
			corruptedTicket, err := client.Ticket.Create().
				SetID("ticket-child-corrupted-" + uuid.NewString()).
				SetEventID(corruptedEvent.ID).
				SetRequester("seed").
				SetStatus(entticket.StatusSUCCESS).
				SetOperationType(operation).
				SetParentTicketID(parentID).
				SetReason("corrupted child").
				Save(ctx)
			require.NoError(t, err)

			validEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusFAILED)
			validTicket, err := client.Ticket.Create().
				SetID("ticket-child-valid-" + uuid.NewString()).
				SetEventID(validEvent.ID).
				SetRequester("seed").
				SetStatus(entticket.StatusFAILED).
				SetOperationType(entticket.OperationTypeCREATE).
				SetParentTicketID(parentID).
				SetReason("valid child").
				Save(ctx)
			require.NoError(t, err)

			parentBefore, err := client.Ticket.Get(ctx, parent.ID)
			require.NoError(t, err)
			parentEventBefore, err := client.DomainEvent.Get(ctx, parentEvent.ID)
			require.NoError(t, err)
			projectionBefore, err := client.BatchTicket.Get(ctx, parentID)
			require.NoError(t, err)
			childTicketsBefore := map[string]*ent.Ticket{}
			childEventsBefore := map[string]*ent.DomainEvent{}
			for _, ticket := range []*ent.Ticket{corruptedTicket, validTicket} {
				before, getErr := client.Ticket.Get(ctx, ticket.ID)
				require.NoError(t, getErr)
				childTicketsBefore[ticket.ID] = before
			}
			for _, event := range []*ent.DomainEvent{corruptedEvent, validEvent} {
				before, getErr := client.DomainEvent.Get(ctx, event.ID)
				require.NoError(t, getErr)
				childEventsBefore[event.ID] = before
			}

			err = SyncParentBatchStatus(ctx, client, parentID)
			require.Error(t, err)
			require.Contains(t, err.Error(), "identity is inconsistent")

			parentAfter, err := client.Ticket.Get(ctx, parent.ID)
			require.NoError(t, err)
			require.Equal(t, parentBefore.EventID, parentAfter.EventID)
			require.Equal(t, parentBefore.OperationType, parentAfter.OperationType)
			require.Equal(t, parentBefore.Status, parentAfter.Status)
			require.Equal(t, parentBefore.ParentTicketID, parentAfter.ParentTicketID)
			require.Equal(t, parentBefore.UpdatedAt, parentAfter.UpdatedAt)
			parentEventAfter, err := client.DomainEvent.Get(ctx, parentEvent.ID)
			require.NoError(t, err)
			require.Equal(t, parentEventBefore.EventType, parentEventAfter.EventType)
			require.Equal(t, parentEventBefore.AggregateType, parentEventAfter.AggregateType)
			require.Equal(t, parentEventBefore.AggregateID, parentEventAfter.AggregateID)
			require.Equal(t, parentEventBefore.Payload, parentEventAfter.Payload)
			require.Equal(t, parentEventBefore.Status, parentEventAfter.Status)
			projectionAfter, err := client.BatchTicket.Get(ctx, parentID)
			require.NoError(t, err)
			require.Equal(t, projectionBefore.BatchType, projectionAfter.BatchType)
			require.Equal(t, projectionBefore.Status, projectionAfter.Status)
			require.Equal(t, projectionBefore.ChildCount, projectionAfter.ChildCount)
			require.Equal(t, projectionBefore.SuccessCount, projectionAfter.SuccessCount)
			require.Equal(t, projectionBefore.FailedCount, projectionAfter.FailedCount)
			require.Equal(t, projectionBefore.PendingCount, projectionAfter.PendingCount)
			require.Equal(t, projectionBefore.UpdatedAt, projectionAfter.UpdatedAt)

			for id, before := range childTicketsBefore {
				after, getErr := client.Ticket.Get(ctx, id)
				require.NoError(t, getErr)
				require.Equal(t, before.EventID, after.EventID)
				require.Equal(t, before.OperationType, after.OperationType)
				require.Equal(t, before.Status, after.Status)
				require.Equal(t, before.Requester, after.Requester)
				require.Equal(t, before.ParentTicketID, after.ParentTicketID)
				require.Equal(t, before.AttemptCount, after.AttemptCount)
				require.Equal(t, before.LastAttemptAt, after.LastAttemptAt)
				require.Equal(t, before.UpdatedAt, after.UpdatedAt)
			}
			for id, before := range childEventsBefore {
				after, getErr := client.DomainEvent.Get(ctx, id)
				require.NoError(t, getErr)
				require.Equal(t, before.EventType, after.EventType)
				require.Equal(t, before.AggregateType, after.AggregateType)
				require.Equal(t, before.AggregateID, after.AggregateID)
				require.Equal(t, before.Payload, after.Payload)
				require.Equal(t, before.Status, after.Status)
				require.Equal(t, before.CreatedBy, after.CreatedBy)
				require.Equal(t, before.ArchivedAt, after.ArchivedAt)
			}
		})
	}
}

func TestValidateParentBatchChildrenInTxLocksSortedTicketAndEventRows(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "validate_parent_children_locks")
	ctx := t.Context()
	parentID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType(string(domain.EventBatchCreateRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)
	parent, err := client.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	ticketIDs := []string{
		"ticket-child-z-" + uuid.NewString(),
		"ticket-child-a-" + uuid.NewString(),
	}
	eventIDs := []string{
		"event-child-z-" + uuid.NewString(),
		"event-child-a-" + uuid.NewString(),
	}
	for idx := range ticketIDs {
		targetID := "service-" + uuid.NewString()
		payload, payloadErr := (domain.VMCreationPayload{ServiceID: targetID}).ToJSON()
		require.NoError(t, payloadErr)
		_, eventErr := client.DomainEvent.Create().
			SetID(eventIDs[idx]).
			SetEventType(string(domain.EventVMCreationRequested)).
			SetAggregateType("vm").
			SetAggregateID(targetID).
			SetPayload(payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy("seed").
			Save(ctx)
		require.NoError(t, eventErr)
		_, ticketErr := client.Ticket.Create().
			SetID(ticketIDs[idx]).
			SetEventID(eventIDs[idx]).
			SetRequester("seed").
			SetStatus(entticket.StatusPENDING).
			SetOperationType(entticket.OperationTypeCREATE).
			SetParentTicketID(parentID).
			SetReason("child").
			Save(ctx)
		require.NoError(t, ticketErr)
	}

	decisionTx, err := client.Tx(ctx)
	require.NoError(t, err)
	decisionOpen := true
	defer func() {
		if decisionOpen {
			_ = decisionTx.Rollback()
		}
	}()
	children, err := ValidateParentBatchChildrenInTx(ctx, decisionTx, parent)
	require.NoError(t, err)
	require.Len(t, children, 2)
	require.Equal(t, ticketIDs[1], children[0].ID)
	require.Equal(t, ticketIDs[0], children[1].ID)

	assertRowLocked := func(table, id string) {
		t.Helper()
		observer, observerErr := client.Tx(ctx)
		require.NoError(t, observerErr)
		observerOpen := true
		defer func() {
			if observerOpen {
				_ = observer.Rollback()
			}
		}()
		require.NoError(t, observer.ExecContext(ctx, `SET LOCAL lock_timeout = '300ms'`))
		lockErr := observer.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET id = id WHERE id = $1`, table), id)
		require.Error(t, lockErr)
		require.Contains(t, lockErr.Error(), "lock timeout")
		require.NoError(t, observer.Rollback())
		observerOpen = false
	}
	assertRowLocked("tickets", ticketIDs[0])
	assertRowLocked("tickets", ticketIDs[1])
	assertRowLocked("domain_events", eventIDs[0])
	assertRowLocked("domain_events", eventIDs[1])

	parentObserver, err := client.Tx(ctx)
	require.NoError(t, err)
	parentObserverOpen := true
	defer func() {
		if parentObserverOpen {
			_ = parentObserver.Rollback()
		}
	}()
	require.NoError(t, parentObserver.ExecContext(ctx, `SET LOCAL lock_timeout = '300ms'`))
	_, err = parentObserver.QueryIntContext(ctx, `SELECT 1 FROM tickets WHERE id = $1 FOR UPDATE`, parentID)
	require.NoError(t, err, "child validation must not lock the parent row")
	require.NoError(t, parentObserver.Rollback())
	parentObserverOpen = false

	require.NoError(t, decisionTx.Rollback())
	decisionOpen = false
}

func TestSyncParentBatchStatusRejectsTerminalParentEventMismatch(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_terminal_event_mismatch")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusCANCELLED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(2).
		SetPendingCount(2).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	for _, childStatus := range []entticket.Status{
		entticket.StatusSUCCESS,
		entticket.StatusFAILED,
	} {
		childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

		_, childTicketErr := client.Ticket.Create().
			SetID("ticket-child-" + uuid.NewString()).
			SetEventID(childEvent.ID).
			SetRequester("seed").
			SetStatus(childStatus).
			SetOperationType(entticket.OperationTypeCREATE).
			SetParentTicketID(parentTicket.ID).
			SetReason("child").
			Save(ctx)
		require.NoError(t, childTicketErr)
	}

	err = SyncParentBatchStatus(ctx, client, parentTicket.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 row, got 0")

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedParent.Status)

	refreshedEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusCANCELLED, refreshedEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 2, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 0, projection.FailedCount)
	require.Equal(t, 2, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}

func TestSyncParentBatchStatusRejectsTerminalParentTicketMismatch(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_terminal_ticket_mismatch")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusCANCELLED).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(2).
		SetPendingCount(2).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	for _, childStatus := range []entticket.Status{
		entticket.StatusSUCCESS,
		entticket.StatusFAILED,
	} {
		childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

		_, childTicketErr := client.Ticket.Create().
			SetID("ticket-child-" + uuid.NewString()).
			SetEventID(childEvent.ID).
			SetRequester("seed").
			SetStatus(childStatus).
			SetOperationType(entticket.OperationTypeCREATE).
			SetParentTicketID(parentTicket.ID).
			SetReason("child").
			Save(ctx)
		require.NoError(t, childTicketErr)
	}

	err = SyncParentBatchStatus(ctx, client, parentTicket.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "update parent batch ticket")

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusCANCELLED, refreshedParent.Status)

	refreshedEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 2, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 0, projection.FailedCount)
	require.Equal(t, 2, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}

func TestReconcileFailedParentBatchStatusReopensFailedParent(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_allow_reopen")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusFAILED).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(2).
		SetFailedCount(1).
		SetStatus(batchticket.StatusFAILED).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	for _, childStatus := range []entticket.Status{
		entticket.StatusSUCCESS,
		entticket.StatusPENDING,
	} {
		childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

		_, childTicketErr := client.Ticket.Create().
			SetID("ticket-child-" + uuid.NewString()).
			SetEventID(childEvent.ID).
			SetRequester("seed").
			SetStatus(childStatus).
			SetOperationType(entticket.OperationTypeCREATE).
			SetParentTicketID(parentTicket.ID).
			SetReason("child").
			Save(ctx)
		require.NoError(t, childTicketErr)
	}

	require.NoError(t, ReconcileFailedParentBatchStatus(ctx, client, parentTicket.ID))

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedParent.Status)

	refreshedEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 2, projection.ChildCount)
	require.Equal(t, 1, projection.SuccessCount)
	require.Equal(t, 0, projection.FailedCount)
	require.Equal(t, 1, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}

func TestUpdateTicketStatusByEventPropagatesParentBatchSyncFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "ticket_status_parent_sync_failure")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

	childTicket, err := client.Ticket.Create().
		SetID("ticket-child-" + uuid.NewString()).
		SetEventID(childEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetParentTicketID(parentTicket.ID).
		SetReason("child").
		Save(ctx)
	require.NoError(t, err)

	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("parent batch event persist failed")),
		ent.OpUpdate,
	))

	err = updateTicketStatusByEvent(ctx, client, childEvent.ID, entticket.StatusSUCCESS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parent batch event persist failed")

	refreshedChild, err := client.Ticket.Get(ctx, childTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, refreshedChild.Status)

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedParent.Status)

	refreshedParentEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedParentEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 1, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 1, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}

func TestUpdateTicketStatusByEventRollsBackParentBatchSyncOnProjectionFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "ticket_status_parent_projection_failure")
	ctx := t.Context()

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	childEvent := createValidBatchCreateChildEvent(t, client, "", domainevent.StatusPROCESSING)

	childTicket, err := client.Ticket.Create().
		SetID("ticket-child-" + uuid.NewString()).
		SetEventID(childEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetParentTicketID(parentTicket.ID).
		SetReason("child").
		Save(ctx)
	require.NoError(t, err)

	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("batch projection persist failed")),
		ent.OpUpdate,
	))

	err = updateTicketStatusByEvent(ctx, client, childEvent.ID, entticket.StatusSUCCESS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch projection persist failed")

	refreshedChild, err := client.Ticket.Get(ctx, childTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusSUCCESS, refreshedChild.Status)

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedParent.Status)

	refreshedParentEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedParentEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 1, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 1, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}

func TestPersistFailedEventAndTicketByEventPropagatesEventFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "failed_event_ticket_event_failure")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("failed event persist unavailable")),
		ent.OpUpdate,
	))

	err = persistFailedEventAndTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed event persist unavailable")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
}

func TestPersistFailedEventAndTicketByEventRollsBackEventOnTicketFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "failed_event_ticket_ticket_failure")
	ctx := t.Context()

	event, err := client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	ticket, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(event.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("create").
		Save(ctx)
	require.NoError(t, err)

	client.Ticket.Use(enthook.On(
		enthook.FixedError(errors.New("failed ticket persist unavailable")),
		ent.OpUpdate,
	))

	err = persistFailedEventAndTicketByEvent(ctx, client, event.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed ticket persist unavailable")

	refreshedEvent, err := client.DomainEvent.Get(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedEvent.Status)

	refreshedTicket, err := client.Ticket.Get(ctx, ticket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedTicket.Status)
}

func TestPersistFailedEventTicketAndVMByEventRollsBackChildOnParentSyncFailure(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "failed_event_ticket_parent_sync_failure")
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
	_, err = client.VM.Create().
		SetID(vmID).
		SetName("vm-" + uuid.NewString()[:8]).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID("cluster-a").
		SetStatus(entvm.StatusDELETING).
		SetCreatedBy("seed").
		SetServiceID(svc.ID).
		Save(ctx)
	require.NoError(t, err)

	parentTicketID := "ticket-parent-" + uuid.NewString()
	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("BATCH_CREATE_REQUESTED").
		SetAggregateType("batch").
		SetAggregateID(parentTicketID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID(parentTicketID).
		SetEventID(parentEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.BatchTicket.Create().
		SetID(parentTicket.ID).
		SetBatchType(batchticket.BatchTypeBATCH_CREATE).
		SetChildCount(1).
		SetPendingCount(1).
		SetStatus(batchticket.StatusIN_PROGRESS).
		SetCreatedBy("seed").
		SetReason("batch create").
		Save(ctx)
	require.NoError(t, err)

	childEvent := createValidBatchCreateChildEvent(t, client, vmID, domainevent.StatusPROCESSING)

	childTicket, err := client.Ticket.Create().
		SetID("ticket-child-" + uuid.NewString()).
		SetEventID(childEvent.ID).
		SetRequester("seed").
		SetStatus(entticket.StatusEXECUTING).
		SetOperationType(entticket.OperationTypeCREATE).
		SetParentTicketID(parentTicket.ID).
		SetReason("child").
		Save(ctx)
	require.NoError(t, err)

	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("parent batch projection persist failed")),
		ent.OpUpdate,
	))

	err = persistFailedEventTicketAndVMByEvent(ctx, client, childEvent.ID, vmID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parent batch projection persist failed")

	refreshedVM, err := client.VM.Get(ctx, vmID)
	require.NoError(t, err)
	require.Equal(t, entvm.StatusDELETING, refreshedVM.Status)

	refreshedChildEvent, err := client.DomainEvent.Get(ctx, childEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedChildEvent.Status)

	refreshedChildTicket, err := client.Ticket.Get(ctx, childTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedChildTicket.Status)

	refreshedParent, err := client.Ticket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, entticket.StatusEXECUTING, refreshedParent.Status)

	refreshedParentEvent, err := client.DomainEvent.Get(ctx, parentEvent.ID)
	require.NoError(t, err)
	require.Equal(t, domainevent.StatusPROCESSING, refreshedParentEvent.Status)

	projection, err := client.BatchTicket.Get(ctx, parentTicket.ID)
	require.NoError(t, err)
	require.Equal(t, 1, projection.ChildCount)
	require.Equal(t, 0, projection.SuccessCount)
	require.Equal(t, 1, projection.PendingCount)
	require.Equal(t, batchticket.StatusIN_PROGRESS, projection.Status)
}
