package jobs

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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
		childEvent, childEventErr := client.DomainEvent.Create().
			SetID("ev-child-" + uuid.NewString()).
			SetEventType("vm.requested").
			SetAggregateType("vm").
			SetAggregateID("vm-" + uuid.NewString()).
			SetPayload([]byte(`{}`)).
			SetStatus(domainevent.StatusPROCESSING).
			SetCreatedBy("seed").
			Save(ctx)
		require.NoError(t, childEventErr)

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

func TestSyncParentBatchStatusRejectsTerminalParentEventMismatch(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_terminal_event_mismatch")
	ctx := t.Context()

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusCANCELLED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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
		childEvent, childEventErr := client.DomainEvent.Create().
			SetID("ev-child-" + uuid.NewString()).
			SetEventType("vm.requested").
			SetAggregateType("vm").
			SetAggregateID("vm-" + uuid.NewString()).
			SetPayload([]byte(`{}`)).
			SetStatus(domainevent.StatusPROCESSING).
			SetCreatedBy("seed").
			Save(ctx)
		require.NoError(t, childEventErr)

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

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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
		childEvent, childEventErr := client.DomainEvent.Create().
			SetID("ev-child-" + uuid.NewString()).
			SetEventType("vm.requested").
			SetAggregateType("vm").
			SetAggregateID("vm-" + uuid.NewString()).
			SetPayload([]byte(`{}`)).
			SetStatus(domainevent.StatusPROCESSING).
			SetCreatedBy("seed").
			Save(ctx)
		require.NoError(t, childEventErr)

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

func TestSyncParentBatchStatusAllowReopenReopensFailedParent(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is not configured")
	}

	client := testutil.OpenEntPostgres(t, "sync_parent_batch_allow_reopen")
	ctx := t.Context()

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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
		childEvent, childEventErr := client.DomainEvent.Create().
			SetID("ev-child-" + uuid.NewString()).
			SetEventType("vm.requested").
			SetAggregateType("vm").
			SetAggregateID("vm-" + uuid.NewString()).
			SetPayload([]byte(`{}`)).
			SetStatus(domainevent.StatusPROCESSING).
			SetCreatedBy("seed").
			Save(ctx)
		require.NoError(t, childEventErr)

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

	require.NoError(t, SyncParentBatchStatusAllowReopen(ctx, client, parentTicket.ID))

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

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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

	childEvent, err := client.DomainEvent.Create().
		SetID("ev-child-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

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

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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

	childEvent, err := client.DomainEvent.Create().
		SetID("ev-child-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID("vm-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

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
		ent.OpUpdateOne,
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

	parentEvent, err := client.DomainEvent.Create().
		SetID("ev-parent-" + uuid.NewString()).
		SetEventType("batch.requested").
		SetAggregateType("ticket").
		SetAggregateID("agg-" + uuid.NewString()).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	parentTicket, err := client.Ticket.Create().
		SetID("ticket-parent-" + uuid.NewString()).
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

	childEvent, err := client.DomainEvent.Create().
		SetID("ev-child-" + uuid.NewString()).
		SetEventType("vm.requested").
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

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
		ent.OpUpdateOne,
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
