package jobs

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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

	SyncParentBatchStatus(ctx, client, parentTicket.ID)

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
