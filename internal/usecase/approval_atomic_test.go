package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestApprovalAtomicWriterValidateCreateInput(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}

	tests := []struct {
		name      string
		ticketID  string
		eventID   string
		approver  string
		clusterID string
		serviceID string
		namespace string
		requester string
		wantErr   bool
	}{
		{
			name:      "valid input",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "user-1",
			wantErr:   false,
		},
		{
			name:      "namespace required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "",
			requester: "user-1",
			wantErr:   true,
		},
		{
			name:      "cluster required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "user-1",
			wantErr:   true,
		},
		{
			name:      "requester required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := w.validateCreateInput(
				tc.ticketID,
				tc.eventID,
				tc.approver,
				tc.clusterID,
				tc.serviceID,
				tc.namespace,
				tc.requester,
			)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMarshalJSONOrNull(t *testing.T) {
	t.Parallel()

	if b, err := marshalJSONOrNull(nil); err != nil || b != nil {
		t.Fatalf("marshalJSONOrNull(nil) = (%v, %v), want (nil, nil)", b, err)
	}
	if b, err := marshalJSONOrNull(map[string]interface{}{}); err != nil || b != nil {
		t.Fatalf("marshalJSONOrNull(empty) = (%v, %v), want (nil, nil)", b, err)
	}
	if b, err := marshalJSONOrNull(map[string]interface{}{"a": "b"}); err != nil || len(b) == 0 {
		t.Fatalf("marshalJSONOrNull(non-empty) unexpected: (%s, %v)", string(b), err)
	}
}

func TestMarshalJSONOrNull_NestedSnapshot(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"source_type": "containerdisk",
		"image_url":   "quay.io/containerdisks/ubuntu:22.04",
		"spec_overrides": map[string]interface{}{
			"spec.template.spec.domain.cpu.cores": float64(4),
		},
	}

	b, err := marshalJSONOrNull(payload)
	if err != nil {
		t.Fatalf("marshalJSONOrNull(nested) unexpected error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("marshalJSONOrNull(nested) should return non-empty json bytes")
	}
}

func TestMarshalJSONOrNull_RejectsUnsupportedValue(t *testing.T) {
	t.Parallel()

	_, err := marshalJSONOrNull(map[string]interface{}{
		"unsupported": func() {},
	})
	if err == nil {
		t.Fatal("marshalJSONOrNull() error = nil, want unsupported value error")
	}
}

func TestApprovalAtomicWriterApprovePowerAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.ApprovePowerAndEnqueue(t.Context(), "ticket-1", "event-1", "admin-1", "start")
	if err == nil {
		t.Fatal("ApprovePowerAndEnqueue() expected initialization error, got nil")
	}
}

func TestApprovalAtomicWriterCreatePowerEventAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.CreatePowerEventAndEnqueue(t.Context(), PowerEventInput{
		EventID:       "event-1",
		EventType:     "VM_START_REQUESTED",
		AggregateType: "vm",
		AggregateID:   "vm-1",
		Payload:       []byte(`{"operation":"start","dispatch_mode":"direct"}`),
		CreatedBy:     "user-1",
	})
	if err == nil {
		t.Fatal("CreatePowerEventAndEnqueue() expected initialization error, got nil")
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.RetryBatchPowerAndEnqueue(t.Context(), BatchPowerRetryInput{
		ParentID: "batch-1",
		Children: []BatchPowerRetryChildInput{
			{TicketID: "ticket-1", EventID: "event-1"},
		},
	})
	if err == nil {
		t.Fatal("RetryBatchPowerAndEnqueue() expected initialization error, got nil")
	}
}

func TestApprovalAtomicWriterBusinessSpansRecordErrors(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	w := &ApprovalAtomicWriter{}
	if err := w.ApprovePowerAndEnqueue(t.Context(), "ticket-1", "event-1", "admin-1", "start"); err == nil {
		t.Fatal("ApprovePowerAndEnqueue() expected initialization error, got nil")
	}
	if err := w.CreateBatchPowerAndMaybeEnqueue(t.Context(), BatchPowerSubmissionInput{
		ParentID: "batch-1",
		Actor:    "user-1",
	}, nil); err == nil {
		t.Fatal("CreateBatchPowerAndMaybeEnqueue() expected initialization error, got nil")
	}

	want := map[string]bool{
		"business.approval.approve_power": false,
		"business.batch_power.submit":     false,
	}
	for _, span := range recorder.Ended() {
		if _, ok := want[span.Name()]; !ok {
			continue
		}
		if span.Status().Code != codes.Error {
			t.Fatalf("span %q status = %v, want error", span.Name(), span.Status().Code)
		}
		want[span.Name()] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("span %q was not recorded", name)
		}
	}
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_CommitsRowsAndJobsAtomically(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "r")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	validatorCalled := false
	input := batchPowerRequestBehaviorInput(
		"batch-atomic-1",
		"vm-1",
		"request-1",
		"POWER_START",
		powerEventTypeStart,
	)
	input.Reason = "batch power start"
	input.Children[0].Reason = "start vm"
	err = w.CreateBatchPowerAndMaybeEnqueue(ctx, input, &BatchPowerSubmissionTxPolicy{
		Validate: func(validationCtx context.Context, tx pgx.Tx) error {
			validatorCalled = true
			var isolation string
			if queryErr := tx.QueryRow(validationCtx, `SHOW transaction_isolation`).Scan(&isolation); queryErr != nil {
				return queryErr
			}
			if isolation != "read committed" {
				return errors.New("batch power transaction isolation is " + isolation)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("CreateBatchPowerAndMaybeEnqueue() unexpected error: %v", err)
	}
	if !validatorCalled {
		t.Fatal("batch power transaction validator was not called")
	}

	var parentStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, "batch-atomic-1").Scan(&parentStatus); err != nil {
		t.Fatalf("query parent ticket: %v", err)
	}
	if parentStatus != "EXECUTING" {
		t.Fatalf("parent status = %q, want EXECUTING", parentStatus)
	}

	var (
		childStatus  string
		attemptCount int
		lastAttempt  pgtype.Timestamptz
	)
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count, last_attempt_at FROM tickets WHERE parent_ticket_id=$1`, "batch-atomic-1").Scan(&childStatus, &attemptCount, &lastAttempt); err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if childStatus != "EXECUTING" {
		t.Fatalf("child status = %q, want EXECUTING", childStatus)
	}
	if attemptCount != 1 || !lastAttempt.Valid {
		t.Fatalf("child attempt = %d at %#v, want 1 with timestamp", attemptCount, lastAttempt)
	}

	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, "vm_power").Scan(&jobCount); err != nil {
		t.Fatalf("query river jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("river vm_power job count = %d, want 1", jobCount)
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_InvalidChildStateRollsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "retry_batch_power_invalid_child")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	tests := []struct {
		name            string
		suffix          string
		eventStatus     string
		useMismatchedID bool
	}{
		{
			name:            "event mismatch rejected before ticket reset",
			suffix:          "mismatch",
			eventStatus:     "FAILED",
			useMismatchedID: true,
		},
		{
			name:        "event not failed rolls back ticket reset",
			suffix:      "event-processing",
			eventStatus: "PROCESSING",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parentID := "batch-retry-" + tc.suffix
			ticketID := "ticket-retry-" + tc.suffix
			eventID := "event-retry-" + tc.suffix
			callEventID := eventID

			if _, insertEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', 'vm-1', '{}'::bytea, $2, 'requester-1')
`, eventID, tc.eventStatus); insertEventErr != nil {
				t.Fatalf("insert domain event: %v", insertEventErr)
			}
			if tc.useMismatchedID {
				callEventID = eventID + "-other"
				if _, insertOtherEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', 'vm-2', '{}'::bytea, 'FAILED', 'requester-1')
`, callEventID); insertOtherEventErr != nil {
					t.Fatalf("insert mismatched domain event: %v", insertOtherEventErr)
				}
			}
			if _, insertTicketErr := pool.Exec(ctx, `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason, reject_reason, parent_ticket_id)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'requester-1', 'retry child', 'seed failure', $3)
`, ticketID, eventID, parentID); insertTicketErr != nil {
				t.Fatalf("insert child ticket: %v", insertTicketErr)
			}

			err := w.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
				ParentID: parentID,
				Children: []BatchPowerRetryChildInput{{
					TicketID: ticketID,
					EventID:  callEventID,
				}},
			})
			if err == nil {
				t.Fatal("RetryBatchPowerAndEnqueue() error = nil, want invalid child error")
			}
			var notEligible *PowerRetryNotEligibleError
			if !errors.As(err, &notEligible) {
				t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *PowerRetryNotEligibleError", err)
			}
			if notEligible.TicketID != ticketID || notEligible.EventID != callEventID {
				t.Fatalf("not-eligible error = %+v, want ticket/event %s/%s", notEligible, ticketID, callEventID)
			}

			assertPowerRetryChildState(ctx, t, pool, ticketID, eventID, "FAILED", "seed failure", tc.eventStatus)
			if tc.useMismatchedID {
				assertDomainEventStatus(ctx, t, pool, callEventID, "FAILED")
			}

			var jobCount int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, "vm_power").Scan(&jobCount); err != nil {
				t.Fatalf("query river jobs: %v", err)
			}
			if jobCount != 0 {
				t.Fatalf("river vm_power job count = %d, want 0", jobCount)
			}
		})
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_RejectedChildFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "r")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	parentID := "batch-retry-rejected"
	ticketID := "ticket-retry-rejected"
	eventID := "event-retry-rejected"
	if _, insertEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', 'vm-1', '{}'::bytea, 'CANCELLED', 'requester-1')
`, eventID); insertEventErr != nil {
		t.Fatalf("insert domain event: %v", insertEventErr)
	}
	if _, insertTicketErr := pool.Exec(ctx, `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason, reject_reason, parent_ticket_id)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'REJECTED', 'requester-1', 'retry child', 'seed rejection', $3)
`, ticketID, eventID, parentID); insertTicketErr != nil {
		t.Fatalf("insert child ticket: %v", insertTicketErr)
	}
	seedFailedBatchPowerRetryParent(t, pool, parentID, "start", "vm-1")

	w := NewApprovalAtomicWriter(pool, riverClient)
	err = w.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{
			TicketID: ticketID,
			EventID:  eventID,
		}},
	})
	var notEligible *PowerRetryNotEligibleError
	if !errors.As(err, &notEligible) {
		t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *PowerRetryNotEligibleError", err)
	}
	if notEligible.TicketID != ticketID || notEligible.EventID != eventID {
		t.Fatalf("not-eligible retry = %+v, want rejected child identifiers", notEligible)
	}

	assertPowerRetryChildState(ctx, t, pool, ticketID, eventID, "REJECTED", "seed rejection", "CANCELLED")
	var attemptCount int
	if queryErr := pool.QueryRow(ctx, `SELECT attempt_count FROM tickets WHERE id=$1`, ticketID).Scan(&attemptCount); queryErr != nil {
		t.Fatalf("query rejected child attempt: %v", queryErr)
	}
	if attemptCount != 0 {
		t.Fatalf("rejected child attempt_count = %d, want unchanged 0", attemptCount)
	}
	result, err := riverClient.JobList(ctx, river.NewJobListParams().Kinds(jobs.VMPowerArgs{}.Kind()))
	if err != nil {
		t.Fatalf("list vm power jobs: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Fatalf("vm power jobs = %d, want 0", len(result.Jobs))
	}
	var parentStatus, parentEventStatus, projectionStatus string
	if err := pool.QueryRow(ctx, `
SELECT parent.status, event.status, projection.status
FROM tickets AS parent
JOIN domain_events AS event ON event.id = parent.event_id
JOIN batch_tickets AS projection ON projection.id = parent.id
WHERE parent.id = $1
`, parentID).Scan(&parentStatus, &parentEventStatus, &projectionStatus); err != nil {
		t.Fatalf("query unchanged rejected-child parent: %v", err)
	}
	if parentStatus != "FAILED" || parentEventStatus != "FAILED" || projectionStatus != "FAILED" {
		t.Fatalf("rejected-child parent state = (%q, %q, %q), want unchanged FAILED", parentStatus, parentEventStatus, projectionStatus)
	}
}

func TestApprovalAtomicWriterApproveModifyAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.ApproveModifyAndEnqueue(t.Context(), "ticket-1", "event-1", "admin-1", nil)
	if err == nil {
		t.Fatal("ApproveModifyAndEnqueue() expected initialization error, got nil")
	}
}

func TestApprovalAtomicWriterApproveDeleteAndEnqueue_MissingVMRollsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "approve_delete_missing_vm")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	const (
		eventID  = "event-delete-missing-vm"
		ticketID = "ticket-delete-missing-vm"
		vmID     = "vm-delete-missing"
	)
	if _, insertEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_DELETION_REQUESTED', 'vm', $2, '{}'::bytea, 'PENDING', 'requester-1')
`, eventID, vmID); insertEventErr != nil {
		t.Fatalf("insert domain event: %v", insertEventErr)
	}
	if _, insertTicketErr := pool.Exec(ctx, `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason)
VALUES ($1, NOW(), NOW(), $2, 'DELETE', 'PENDING', 'requester-1', 'cleanup')
`, ticketID, eventID); insertTicketErr != nil {
		t.Fatalf("insert ticket: %v", insertTicketErr)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	err = w.ApproveDeleteAndEnqueue(ctx, ticketID, eventID, "admin-1", "   ")
	if err == nil {
		t.Fatal("ApproveDeleteAndEnqueue() error = nil for empty vm id, want validation error")
	}
	if !strings.Contains(err.Error(), "approve delete input is incomplete") {
		t.Fatalf("ApproveDeleteAndEnqueue() error = %v, want incomplete input error", err)
	}

	assertDeleteApprovalStillPending(ctx, t, pool, ticketID, eventID)

	err = w.ApproveDeleteAndEnqueue(ctx, ticketID, eventID, "admin-1", vmID)
	if err == nil {
		t.Fatal("ApproveDeleteAndEnqueue() error = nil, want missing VM error")
	}
	if !strings.Contains(err.Error(), "not found while setting status to DELETING") {
		t.Fatalf("ApproveDeleteAndEnqueue() error = %v, want missing VM status error", err)
	}

	assertDeleteApprovalStillPending(ctx, t, pool, ticketID, eventID)
}

func TestApprovalAtomicWriterApproveDeleteAndEnqueue_TerminalEventRollsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "approve_delete_terminal_event")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	const (
		eventID  = "event-delete-terminal"
		ticketID = "ticket-delete-terminal"
		vmID     = "vm-delete-terminal"
	)
	if _, insertEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_DELETION_REQUESTED', 'vm', $2, '{}'::bytea, 'COMPLETED', 'requester-1')
`, eventID, vmID); insertEventErr != nil {
		t.Fatalf("insert domain event: %v", insertEventErr)
	}
	if _, insertTicketErr := pool.Exec(ctx, `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason)
VALUES ($1, NOW(), NOW(), $2, 'DELETE', 'PENDING', 'requester-1', 'cleanup')
`, ticketID, eventID); insertTicketErr != nil {
		t.Fatalf("insert ticket: %v", insertTicketErr)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	err = w.ApproveDeleteAndEnqueue(ctx, ticketID, eventID, "admin-1", vmID)
	if err == nil {
		t.Fatal("ApproveDeleteAndEnqueue() error = nil, want terminal event error")
	}
	if !strings.Contains(err.Error(), "not found or not pending") {
		t.Fatalf("ApproveDeleteAndEnqueue() error = %v, want non-pending event error", err)
	}

	assertDeleteApprovalState(ctx, t, pool, ticketID, eventID, "PENDING", "COMPLETED")
}

func TestApprovalAtomicWriterApprovePowerAndEnqueue_TerminalEventRollsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "approve_power_terminal_event")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	const (
		eventID  = "event-power-terminal"
		ticketID = "ticket-power-terminal"
	)
	if _, insertEventErr := pool.Exec(ctx, `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_RESTART_REQUESTED', 'vm', 'vm-1', '{}'::bytea, 'COMPLETED', 'requester-1')
`, eventID); insertEventErr != nil {
		t.Fatalf("insert domain event: %v", insertEventErr)
	}
	if _, insertTicketErr := pool.Exec(ctx, `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'PENDING', 'requester-1', 'restart')
`, ticketID, eventID); insertTicketErr != nil {
		t.Fatalf("insert ticket: %v", insertTicketErr)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	err = w.ApprovePowerAndEnqueue(ctx, ticketID, eventID, "admin-1", "restart")
	if err == nil {
		t.Fatal("ApprovePowerAndEnqueue() error = nil, want terminal event error")
	}
	if !strings.Contains(err.Error(), "not found or not pending") {
		t.Fatalf("ApprovePowerAndEnqueue() error = %v, want non-pending event error", err)
	}

	assertApprovalTicketAndEventState(ctx, t, pool, ticketID, eventID, "PENDING", "COMPLETED", "vm_power")
}

type approvalAtomicBehaviorStore struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
	writer      *ApprovalAtomicWriter
}

func newApprovalAtomicBehaviorStore(t *testing.T, prefix string) approvalAtomicBehaviorStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, prefix)
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(t.Context(), string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	return approvalAtomicBehaviorStore{
		pool:        pool,
		riverClient: riverClient,
		writer:      NewApprovalAtomicWriter(pool, riverClient),
	}
}

func powerApprovalBehaviorInput(eventID, ticketID, vmID, eventType string) PowerApprovalRequestInput {
	_, action, _, ok := batchPowerOperationIdentity(eventType)
	if !ok {
		action = "start"
	}
	return PowerApprovalRequestInput{
		EventID:     eventID,
		TicketID:    ticketID,
		EventType:   eventType,
		AggregateID: vmID,
		Payload: mustMarshalBatchPowerBehaviorJSON(domain.VMPowerPayload{
			VMID:         vmID,
			VMName:       vmID,
			ClusterID:    "cluster-approval-test",
			Namespace:    "namespace-approval-test",
			Operation:    action,
			Actor:        "approval-requester",
			DispatchMode: domain.VMPowerDispatchTicket,
		}),
		CreatedBy: "approval-requester",
		Reason:    "power request",
	}
}

func directPowerBehaviorInput(eventID, vmID, eventType string) PowerEventInput {
	_, action, _, ok := batchPowerOperationIdentity(eventType)
	if !ok {
		action = "start"
	}
	const actor = "power-operator"
	return PowerEventInput{
		EventID:       eventID,
		EventType:     eventType,
		AggregateType: "vm",
		AggregateID:   vmID,
		Payload: mustMarshalBatchPowerBehaviorJSON(domain.VMPowerPayload{
			VMID:         vmID,
			VMName:       vmID,
			ClusterID:    "cluster-direct-test",
			Namespace:    "namespace-direct-test",
			Operation:    action,
			Actor:        actor,
			DispatchMode: domain.VMPowerDispatchDirect,
		}),
		CreatedBy: actor,
	}
}

func batchPowerBehaviorInput(parentID, vmID string) BatchPowerSubmissionInput {
	const actor = "batch-operator"
	payload := domain.VMPowerPayload{
		VMID:         vmID,
		VMName:       vmID,
		ClusterID:    "cluster-batch-test",
		Namespace:    "namespace-batch-test",
		Operation:    "start",
		Actor:        actor,
		DispatchMode: domain.VMPowerDispatchTicket,
	}
	return BatchPowerSubmissionInput{
		ParentID:  parentID,
		Actor:     actor,
		Operation: "POWER_START",
		Reason:    "batch power",
		ParentPayload: mustMarshalBatchPowerBehaviorJSON(domain.BatchVMRequestPayload{
			Operation:   "POWER_START",
			SubmittedBy: actor,
			Items:       []domain.BatchVMItemPayload{batchPowerPayloadItem(payload)},
		}),
		Children: []BatchPowerChildInput{{
			EventType:   powerEventTypeStart,
			AggregateID: vmID,
			Payload:     mustMarshalBatchPowerBehaviorJSON(payload),
			Reason:      "start VM",
		}},
	}
}

func batchPowerRequestBehaviorInput(parentID, vmID, requestID, operation, eventType string) BatchPowerSubmissionInput {
	input := batchPowerBehaviorInput(parentID, vmID)
	input.RequestID = requestID
	input.Operation = operation
	input.Children[0].EventType = eventType
	_, action, _, ok := batchPowerOperationIdentity(operation)
	if ok {
		payload := domain.VMPowerPayload{
			VMID:         vmID,
			VMName:       vmID,
			ClusterID:    "cluster-batch-test",
			Namespace:    "namespace-batch-test",
			Operation:    action,
			Actor:        input.Actor,
			DispatchMode: domain.VMPowerDispatchTicket,
		}
		input.Children[0].Payload = mustMarshalBatchPowerBehaviorJSON(payload)
		input.ParentPayload = mustMarshalBatchPowerBehaviorJSON(domain.BatchVMRequestPayload{
			Operation:   operation,
			RequestID:   requestID,
			SubmittedBy: input.Actor,
			Items:       []domain.BatchVMItemPayload{batchPowerPayloadItem(payload)},
		})
	}
	return input
}

func mustMarshalBatchPowerBehaviorJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestApprovalAtomicWriterCreatePowerApprovalRequest_PublicFailureContracts(t *testing.T) {
	t.Run("uninitialized writer", func(t *testing.T) {
		t.Parallel()
		err := (&ApprovalAtomicWriter{}).CreatePowerApprovalRequest(
			t.Context(),
			powerApprovalBehaviorInput("event-uninitialized", "ticket-uninitialized", "vm-uninitialized", powerEventTypeStart),
		)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("CreatePowerApprovalRequest() error = %v, want initialization failure", err)
		}
	})

	t.Run("input validation", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_validation")
		incomplete := powerApprovalBehaviorInput("", "ticket-validation", "vm-validation", powerEventTypeStart)
		if err := store.writer.CreatePowerApprovalRequest(t.Context(), incomplete); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("incomplete request error = %v", err)
		}
		unsupported := powerApprovalBehaviorInput("event-validation", "ticket-validation", "vm-validation", "VM_SUSPEND_REQUESTED")
		if err := store.writer.CreatePowerApprovalRequest(t.Context(), unsupported); err == nil || !strings.Contains(err.Error(), "unsupported power event type") {
			t.Fatalf("unsupported event error = %v", err)
		}
		missingMode := powerApprovalBehaviorInput("event-missing-mode", "ticket-missing-mode", "vm-missing-mode", powerEventTypeStart)
		var payload domain.VMPowerPayload
		if err := json.Unmarshal(missingMode.Payload, &payload); err != nil {
			t.Fatalf("decode approval provenance fixture: %v", err)
		}
		payload.DispatchMode = ""
		missingMode.Payload = mustMarshalBatchPowerBehaviorJSON(payload)
		if err := store.writer.CreatePowerApprovalRequest(t.Context(), missingMode); err == nil || !strings.Contains(err.Error(), "payload identity") {
			t.Fatalf("missing approval dispatch mode error = %v", err)
		}

		for _, tc := range []struct {
			name   string
			mutate func(*domain.VMPowerPayload)
		}{
			{name: "VM id", mutate: func(payload *domain.VMPowerPayload) { payload.VMID = "different-vm" }},
			{name: "actor", mutate: func(payload *domain.VMPowerPayload) { payload.Actor = "different-actor" }},
			{name: "operation", mutate: func(payload *domain.VMPowerPayload) { payload.Operation = "stop" }},
			{name: "provider coordinates", mutate: func(payload *domain.VMPowerPayload) { payload.ClusterID = "" }},
		} {
			input := powerApprovalBehaviorInput("event-identity-"+strings.ReplaceAll(tc.name, " ", "-"), "ticket-identity-"+strings.ReplaceAll(tc.name, " ", "-"), "vm-identity", powerEventTypeStart)
			var decoded domain.VMPowerPayload
			require.NoError(t, json.Unmarshal(input.Payload, &decoded))
			tc.mutate(&decoded)
			input.Payload = mustMarshalBatchPowerBehaviorJSON(decoded)
			err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
			require.ErrorContains(t, err, "payload identity", tc.name)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("active approval returns stable conflict and preserves original", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_active")
		first := powerApprovalBehaviorInput("event-active-first", "ticket-active-first", "vm-active", powerEventTypeStart)
		if err := store.writer.CreatePowerApprovalRequest(t.Context(), first); err != nil {
			t.Fatalf("create original approval request: %v", err)
		}
		second := powerApprovalBehaviorInput("event-active-second", "ticket-active-second", "vm-active", powerEventTypeStop)
		err := store.writer.CreatePowerApprovalRequest(t.Context(), second)
		var active *ActivePowerEventError
		if !errors.As(err, &active) {
			t.Fatalf("second approval error = %v, want *ActivePowerEventError", err)
		}
		if active.ExistingEventID != first.EventID || active.ExistingTicketID != first.TicketID {
			t.Fatalf("active conflict = %+v, want original event/ticket", active)
		}
		if !strings.Contains(err.Error(), first.EventID) || !strings.Contains(err.Error(), powerEventTypeStart) {
			t.Fatalf("active conflict string = %q, want existing event and type", err.Error())
		}
		assertAtomicBehaviorCounts(t, store.pool, 1, 1, 0, 0)
	})

	t.Run("event insert failure rolls back ticket", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_event_failure")
		input := powerApprovalBehaviorInput("event-insert-conflict", "ticket-after-event-conflict", "vm-event-conflict", powerEventTypeStart)
		if _, err := store.pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', 'other-vm', '{}'::bytea, 'COMPLETED', 'seed')
`, input.EventID); err != nil {
			t.Fatalf("seed conflicting event: %v", err)
		}
		err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "insert power approval event") {
			t.Fatalf("event insert error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 1, 0, 0, 0)
	})

	t.Run("ticket insert failure rolls back event", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_ticket_failure")
		input := powerApprovalBehaviorInput("event-before-ticket-conflict", "ticket-insert-conflict", "vm-ticket-conflict", powerEventTypeStart)
		if _, err := store.pool.Exec(t.Context(), `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester)
VALUES ($1, NOW(), NOW(), 'seed-event', 'POWER', 'FAILED', 'seed')
`, input.TicketID); err != nil {
			t.Fatalf("seed conflicting ticket: %v", err)
		}
		err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "insert power approval ticket") {
			t.Fatalf("ticket insert error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 1, 0, 0)
	})

	t.Run("lookup failure leaves no partial rows", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_lookup_failure")
		if _, err := store.pool.Exec(t.Context(), `DROP TABLE domain_events`); err != nil {
			t.Fatalf("remove event store: %v", err)
		}
		input := powerApprovalBehaviorInput("event-lookup-failure", "ticket-lookup-failure", "vm-lookup-failure", powerEventTypeStart)
		err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "check active power event") {
			t.Fatalf("lookup error = %v", err)
		}
		var ticketCount int
		if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM tickets`).Scan(&ticketCount); err != nil {
			t.Fatalf("count tickets: %v", err)
		}
		if ticketCount != 0 {
			t.Fatalf("ticket count = %d, want 0", ticketCount)
		}
	})

	t.Run("deferred commit failure rolls back event and ticket", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_commit_failure")
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION reject_power_approval_commit() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RAISE EXCEPTION 'forced power approval commit failure';
END;
$$;
CREATE CONSTRAINT TRIGGER reject_power_approval_commit
AFTER INSERT ON tickets
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_power_approval_commit();
`); err != nil {
			t.Fatalf("install deferred commit failure: %v", err)
		}
		input := powerApprovalBehaviorInput("event-commit-failure", "ticket-commit-failure", "vm-commit-failure", powerEventTypeStart)
		err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "commit power approval request") {
			t.Fatalf("commit error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("closed store fails before mutation", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "power_approval_closed_store")
		store.pool.Close()
		input := powerApprovalBehaviorInput("event-closed-store", "ticket-closed-store", "vm-closed-store", powerEventTypeStart)
		err := store.writer.CreatePowerApprovalRequest(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "begin power approval request tx") {
			t.Fatalf("closed store error = %v", err)
		}
	})
}

func TestApprovalAtomicWriterCreatePowerEventAndEnqueue_PublicConflictContracts(t *testing.T) {
	t.Run("missing direct dispatch mode is rejected", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "dm")
		input := directPowerBehaviorInput("event-missing-direct-mode", "vm-missing-direct-mode", powerEventTypeStart)
		input.Payload = []byte(`{"operation":"start"}`)
		err := store.writer.CreatePowerEventAndEnqueue(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "direct power payload") {
			t.Fatalf("missing direct dispatch mode error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("unsupported event is rejected before persistence", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "du")
		input := directPowerBehaviorInput("event-unsupported", "vm-unsupported", "VM_SUSPEND_REQUESTED")
		err := store.writer.CreatePowerEventAndEnqueue(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "unsupported power event type") {
			t.Fatalf("unsupported event error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("immutable payload identity mismatch is rejected before persistence", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			mutate func(*domain.VMPowerPayload)
		}{
			{name: "VM id", mutate: func(payload *domain.VMPowerPayload) { payload.VMID = "different-vm" }},
			{name: "actor", mutate: func(payload *domain.VMPowerPayload) { payload.Actor = "different-actor" }},
			{name: "operation", mutate: func(payload *domain.VMPowerPayload) { payload.Operation = "stop" }},
			{name: "provider coordinates", mutate: func(payload *domain.VMPowerPayload) { payload.Namespace = "" }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				store := newApprovalAtomicBehaviorStore(t, "direct_identity_"+strings.ReplaceAll(tc.name, " ", "_"))
				input := directPowerBehaviorInput("event-direct-identity", "vm-direct-identity", powerEventTypeStart)
				var decoded domain.VMPowerPayload
				require.NoError(t, json.Unmarshal(input.Payload, &decoded))
				tc.mutate(&decoded)
				input.Payload = mustMarshalBatchPowerBehaviorJSON(decoded)
				err := store.writer.CreatePowerEventAndEnqueue(t.Context(), input)
				require.ErrorContains(t, err, "payload identity")
				assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
			})
		}
	})

	t.Run("active lookup failure leaves no direct event", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "dq")
		if _, err := store.pool.Exec(t.Context(), `DROP TABLE river_job`); err != nil {
			t.Fatalf("remove River job store: %v", err)
		}
		input := directPowerBehaviorInput("event-direct-lookup", "vm-direct-lookup", powerEventTypeStart)
		err := store.writer.CreatePowerEventAndEnqueue(t.Context(), input)
		if err == nil || !strings.Contains(err.Error(), "check active power event") {
			t.Fatalf("active lookup error = %v", err)
		}
		assertAtomicApplicationRows(t, store.pool, 0, 0, 0)
	})

	t.Run("same direct action returns existing event", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "dd")
		first := directPowerBehaviorInput("event-direct-first", "vm-direct-duplicate", powerEventTypeStart)
		if err := store.writer.CreatePowerEventAndEnqueue(t.Context(), first); err != nil {
			t.Fatalf("create original direct event: %v", err)
		}
		second := directPowerBehaviorInput("event-direct-second", first.AggregateID, powerEventTypeStart)
		err := store.writer.CreatePowerEventAndEnqueue(t.Context(), second)
		var duplicate *DuplicatePowerEventError
		if !errors.As(err, &duplicate) {
			t.Fatalf("duplicate direct error = %v, want *DuplicatePowerEventError", err)
		}
		if duplicate.ExistingEventID != first.EventID || !strings.Contains(err.Error(), first.EventID) {
			t.Fatalf("duplicate error = %+v (%q), want event %s", duplicate, err.Error(), first.EventID)
		}
		assertAtomicBehaviorCounts(t, store.pool, 1, 0, 0, 1)
	})

	t.Run("different direct action returns active conflict", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "da")
		first := directPowerBehaviorInput("event-direct-active", "vm-direct-active", powerEventTypeStart)
		if err := store.writer.CreatePowerEventAndEnqueue(t.Context(), first); err != nil {
			t.Fatalf("create original direct event: %v", err)
		}
		second := directPowerBehaviorInput("event-direct-conflict", first.AggregateID, powerEventTypeStop)
		err := store.writer.CreatePowerEventAndEnqueue(t.Context(), second)
		var active *ActivePowerEventError
		if !errors.As(err, &active) {
			t.Fatalf("different direct action error = %v, want *ActivePowerEventError", err)
		}
		if active.ExistingEventID != first.EventID || !strings.Contains(err.Error(), first.EventID) {
			t.Fatalf("active error = %+v (%q), want event %s", active, err.Error(), first.EventID)
		}
		assertAtomicBehaviorCounts(t, store.pool, 1, 0, 0, 1)
	})
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_PublicFailureContracts(t *testing.T) {
	t.Run("actor policy failure prevents all writes", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bl")
		input := batchPowerBehaviorInput("batch-lock-policy", "vm-lock-policy")
		validateCalled := false
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, &BatchPowerSubmissionTxPolicy{
			LockActor: func(context.Context, pgx.Tx) error {
				return errors.New("actor policy unavailable")
			},
			Validate: func(context.Context, pgx.Tx) error {
				validateCalled = true
				return nil
			},
		})
		if err == nil || !strings.Contains(err.Error(), "actor policy unavailable") {
			t.Fatalf("actor policy error = %v", err)
		}
		if validateCalled {
			t.Fatal("validation ran after actor policy failure")
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("submission validation failure prevents all writes", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bv")
		input := batchPowerBehaviorInput("batch-validation-policy", "vm-validation-policy")
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, &BatchPowerSubmissionTxPolicy{
			Validate: func(context.Context, pgx.Tx) error {
				return errors.New("batch policy rejected")
			},
		})
		if err == nil || !strings.Contains(err.Error(), "batch policy rejected") {
			t.Fatalf("batch validation error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("unsupported child event is rejected before persistence", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bi")
		input := batchPowerBehaviorInput("batch-invalid-child", "vm-invalid-child")
		input.Children[0].EventType = "VM_SUSPEND_REQUESTED"
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
		if err == nil || !strings.Contains(err.Error(), "unsupported event type") {
			t.Fatalf("invalid child error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("active lookup failure leaves no batch rows", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bq")
		if _, err := store.pool.Exec(t.Context(), `DROP TABLE river_job`); err != nil {
			t.Fatalf("remove River job store: %v", err)
		}
		input := batchPowerBehaviorInput("batch-lookup-failure", "vm-lookup-failure")
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
		if err == nil || !strings.Contains(err.Error(), "check active power event") {
			t.Fatalf("active lookup error = %v", err)
		}
		assertAtomicApplicationRows(t, store.pool, 0, 0, 0)
	})

	t.Run("initial attempt persistence failure rolls back parent and child", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "ba")
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION reject_initial_power_attempt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RAISE EXCEPTION 'forced initial attempt failure';
END;
$$;
CREATE TRIGGER reject_initial_power_attempt
BEFORE UPDATE ON tickets
FOR EACH ROW EXECUTE FUNCTION reject_initial_power_attempt();
`); err != nil {
			t.Fatalf("install initial attempt failure: %v", err)
		}
		input := batchPowerBehaviorInput("batch-attempt-failure", "vm-attempt-failure")
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
		if err == nil || !strings.Contains(err.Error(), "start initial batch power child attempt") {
			t.Fatalf("initial attempt error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("canceled actor policy rolls back before VM mutation", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bc")
		input := batchPowerBehaviorInput("batch-canceled-policy", "vm-canceled-policy")
		ctx, cancel := context.WithCancel(t.Context())
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, &BatchPowerSubmissionTxPolicy{
			LockActor: func(context.Context, pgx.Tx) error {
				cancel()
				return nil
			},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled policy error = %v, want context.Canceled", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("canceled actor policy rolls back before request replay", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "br")
		input := batchPowerRequestBehaviorInput(
			"batch-canceled-request",
			"vm-canceled-request",
			"request-canceled-policy",
			"POWER_START",
			powerEventTypeStart,
		)
		ctx, cancel := context.WithCancel(t.Context())
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, &BatchPowerSubmissionTxPolicy{
			LockActor: func(context.Context, pgx.Tx) error {
				cancel()
				return nil
			},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled replay policy error = %v, want context.Canceled", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("missing initial attempt transition fails closed", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "bz")
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION skip_initial_power_attempt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RETURN NULL;
END;
$$;
CREATE TRIGGER skip_initial_power_attempt
BEFORE UPDATE ON tickets
FOR EACH ROW EXECUTE FUNCTION skip_initial_power_attempt();
`); err != nil {
			t.Fatalf("install missing initial attempt transition: %v", err)
		}
		input := batchPowerBehaviorInput("batch-missing-attempt", "vm-missing-attempt")
		err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
		if err == nil || !strings.Contains(err.Error(), "expected 1 row, got 0") {
			t.Fatalf("missing initial attempt error = %v", err)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_OperationScopedReplay(t *testing.T) {
	t.Parallel()

	store := newApprovalAtomicBehaviorStore(t, "or")
	const requestID = "shared-operation-request"

	stop := batchPowerRequestBehaviorInput("batch-operation-stop", "vm-operation-stop", requestID, "POWER_STOP", powerEventTypeStop)
	start := batchPowerRequestBehaviorInput("batch-operation-start", "vm-operation-start", requestID, "POWER_START", powerEventTypeStart)
	restart := batchPowerRequestBehaviorInput("batch-operation-restart", "vm-operation-restart", requestID, "RESTART", powerEventTypeRestart)
	for _, input := range []BatchPowerSubmissionInput{stop, start, restart} {
		if err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil); err != nil {
			t.Fatalf("create %s operation batch: %v", input.Operation, err)
		}
	}
	genericPower := batchPowerRequestBehaviorInput(
		"batch-operation-power",
		"vm-operation-power",
		requestID,
		"POWER_START",
		powerEventTypeStart,
	)
	genericPower.Operation = "POWER"
	if err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), genericPower, nil); err == nil ||
		!strings.Contains(err.Error(), "operation") {
		t.Fatalf("generic power operation error = %v, want validation failure", err)
	}

	for _, tc := range []struct {
		name      string
		input     BatchPowerSubmissionInput
		wantBatch string
	}{
		{
			name:      "start event alias replays start only",
			input:     batchPowerRequestBehaviorInput("batch-replay-start", "vm-replay-start", requestID, powerEventTypeStart, powerEventTypeStart),
			wantBatch: start.ParentID,
		},
		{
			name:      "restart alias replays restart only",
			input:     batchPowerRequestBehaviorInput("batch-replay-restart", "vm-replay-restart", requestID, "POWER_RESTART", powerEventTypeRestart),
			wantBatch: restart.ParentID,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), tc.input, nil)
			var replay *BatchSubmissionReplayError
			if !errors.As(err, &replay) {
				t.Fatalf("operation replay error = %v, want *BatchSubmissionReplayError", err)
			}
			if replay.BatchID != tc.wantBatch || !strings.Contains(err.Error(), tc.wantBatch) {
				t.Fatalf("operation replay = %+v (%q), want %s", replay, err.Error(), tc.wantBatch)
			}
		})
	}

	assertAtomicBehaviorCounts(t, store.pool, 6, 6, 3, 3)
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_MalformedReplayHistoryFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		payload   string
		wantError string
	}{
		{
			name:      "malformed JSON",
			payload:   "{",
			wantError: "parent event payload is malformed",
		},
		{
			name:      "missing operation",
			payload:   `{"request_id":"request-malformed-replay","submitted_by":"batch-operator","items":[]}`,
			wantError: "parent payload operation is inconsistent",
		},
		{
			name:      "missing submitter",
			payload:   `{"operation":"POWER_START","request_id":"request-malformed-replay","items":[]}`,
			wantError: "parent payload identity is inconsistent",
		},
		{
			name:      "foreign submitter",
			payload:   `{"operation":"POWER_START","request_id":"request-malformed-replay","submitted_by":"other-operator","items":[]}`,
			wantError: "parent payload identity is inconsistent",
		},
		{
			name:      "foreign request id",
			payload:   `{"operation":"POWER_START","request_id":"other-request","submitted_by":"batch-operator","items":[]}`,
			wantError: "parent payload identity is inconsistent",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "mh")
			const requestID = "request-malformed-replay"
			if _, err := store.pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ('event-malformed-replay', NOW(), 'BATCH_POWER_REQUESTED', 'batch', 'batch-malformed-replay', $1::bytea, 'PENDING', 'batch-operator')
`, []byte(tc.payload)); err != nil {
				t.Fatalf("seed malformed replay event: %v", err)
			}
			if _, err := store.pool.Exec(t.Context(), `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester)
VALUES ('batch-malformed-replay', NOW(), NOW(), 'event-malformed-replay', 'POWER', 'PENDING', 'batch-operator')
`); err != nil {
				t.Fatalf("seed malformed replay ticket: %v", err)
			}
			if _, err := store.pool.Exec(t.Context(), `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, pending_count,
  status, request_id, created_by
)
VALUES (
  'batch-malformed-replay', NOW(), NOW(), 'BATCH_POWER', 0, 0,
  'PENDING_APPROVAL', $1, 'batch-operator'
)
`, requestID); err != nil {
				t.Fatalf("seed malformed replay projection: %v", err)
			}

			input := batchPowerRequestBehaviorInput(
				"batch-after-malformed-replay",
				"vm-after-malformed-replay",
				requestID,
				"POWER_START",
				powerEventTypeStart,
			)
			err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("CreateBatchPowerAndMaybeEnqueue() error = %v, want %q", err, tc.wantError)
			}
			var replay *BatchSubmissionReplayError
			if errors.As(err, &replay) {
				t.Fatalf("malformed replay error = %+v, must fail closed instead of replaying", replay)
			}
			assertAtomicBehaviorCounts(t, store.pool, 1, 1, 1, 0)
		})
	}
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_CorruptReplayGraphFailsClosed(t *testing.T) {
	const requestID = "request-corrupt-replay-graph"
	validPayload := []byte(`{"operation":"POWER_START","request_id":"request-corrupt-replay-graph","submitted_by":"batch-operator","items":[]}`)
	tests := []struct {
		name      string
		seed      func(t *testing.T, store approvalAtomicBehaviorStore)
		wantError string
	}{
		{
			name: "missing root ticket",
			seed: func(t *testing.T, store approvalAtomicBehaviorStore) {
				seedPowerReplayProjectionOnly(t, store.pool, "batch-replay-missing-root", requestID, time.Now().UTC())
			},
			wantError: "root ticket identity is inconsistent",
		},
		{
			name: "foreign root requester",
			seed: func(t *testing.T, store approvalAtomicBehaviorStore) {
				seedPowerReplayGraph(t, store.pool, "batch-replay-foreign-root", requestID, time.Now().UTC(), validPayload)
				if _, err := store.pool.Exec(t.Context(), `UPDATE tickets SET requester = 'other-operator' WHERE id = 'batch-replay-foreign-root'`); err != nil {
					t.Fatalf("corrupt replay root requester: %v", err)
				}
			},
			wantError: "root ticket identity is inconsistent",
		},
		{
			name: "foreign parent event creator",
			seed: func(t *testing.T, store approvalAtomicBehaviorStore) {
				seedPowerReplayGraph(t, store.pool, "batch-replay-foreign-event", requestID, time.Now().UTC(), validPayload)
				if _, err := store.pool.Exec(t.Context(), `UPDATE domain_events SET created_by = 'other-operator' WHERE id = 'batch-replay-foreign-event-event'`); err != nil {
					t.Fatalf("corrupt replay event creator: %v", err)
				}
			},
			wantError: "parent event identity is inconsistent",
		},
		{
			name: "valid oldest cannot mask corrupt duplicate",
			seed: func(t *testing.T, store approvalAtomicBehaviorStore) {
				seedPowerReplayGraph(t, store.pool, "batch-replay-valid-oldest", requestID, time.Now().UTC().Add(-time.Hour), validPayload)
				seedPowerReplayProjectionOnly(t, store.pool, "batch-replay-corrupt-later", requestID, time.Now().UTC())
			},
			wantError: "root ticket identity is inconsistent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "rg")
			tt.seed(t, store)
			input := batchPowerRequestBehaviorInput(
				"batch-after-corrupt-replay",
				"vm-after-corrupt-replay",
				requestID,
				"POWER_START",
				powerEventTypeStart,
			)
			err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("CreateBatchPowerAndMaybeEnqueue() error = %v, want %q", err, tt.wantError)
			}
			var replay *BatchSubmissionReplayError
			if errors.As(err, &replay) {
				t.Fatalf("corrupt replay graph returned replay result %+v", replay)
			}
			var inserted int
			if queryErr := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM batch_tickets WHERE id = $1`, input.ParentID).Scan(&inserted); queryErr != nil {
				t.Fatalf("count candidate side effects: %v", queryErr)
			}
			if inserted != 0 {
				t.Fatalf("new batch side effects = %d, want 0", inserted)
			}
		})
	}
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_BoundsReplayCandidates(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "rb")
	const requestID = "request-oversized-replay-history"
	createdAt := time.Now().UTC()
	for i := 0; i <= batchreplay.CandidateLimit; i++ {
		seedPowerReplayProjectionOnly(
			t,
			store.pool,
			fmt.Sprintf("batch-replay-history-%03d", i),
			requestID,
			createdAt.Add(time.Duration(i)*time.Millisecond),
		)
	}
	input := batchPowerRequestBehaviorInput(
		"batch-after-oversized-replay",
		"vm-after-oversized-replay",
		requestID,
		"POWER_START",
		powerEventTypeStart,
	)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
	if err == nil || !strings.Contains(err.Error(), "more than 64 matching projections") {
		t.Fatalf("oversized replay history error = %v, want bounded-history failure", err)
	}
}

func seedPowerReplayGraph(
	t *testing.T,
	pool *pgxpool.Pool,
	batchID, requestID string,
	createdAt time.Time,
	payload []byte,
) {
	t.Helper()
	eventID := batchID + "-event"
	if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, $2, 'BATCH_POWER_REQUESTED', 'batch', $3, $4, 'PENDING', 'batch-operator')
`, eventID, createdAt, batchID, payload); err != nil {
		t.Fatalf("seed replay event %s: %v", eventID, err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester)
VALUES ($1, $2, $2, $3, 'POWER', 'PENDING', 'batch-operator')
`, batchID, createdAt, eventID); err != nil {
		t.Fatalf("seed replay root %s: %v", batchID, err)
	}
	seedPowerReplayProjectionOnly(t, pool, batchID, requestID, createdAt)
}

func seedPowerReplayProjectionOnly(
	t *testing.T,
	pool *pgxpool.Pool,
	batchID, requestID string,
	createdAt time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, pending_count,
  status, request_id, created_by
)
VALUES ($1, $2, $2, 'BATCH_POWER', 0, 0, 'PENDING_APPROVAL', $3, 'batch-operator')
`, batchID, createdAt, requestID); err != nil {
		t.Fatalf("seed replay projection %s: %v", batchID, err)
	}
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_ReplayLookupFailureRollsBack(t *testing.T) {
	t.Parallel()

	store := newApprovalAtomicBehaviorStore(t, "oq")
	if _, err := store.pool.Exec(t.Context(), `DROP TABLE batch_tickets`); err != nil {
		t.Fatalf("remove batch projection store: %v", err)
	}
	input := batchPowerRequestBehaviorInput(
		"batch-replay-lookup-failure",
		"vm-replay-lookup-failure",
		"request-replay-lookup-failure",
		"POWER_START",
		powerEventTypeStart,
	)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(t.Context(), input, nil)
	if err == nil || !strings.Contains(err.Error(), "query existing batch submission") {
		t.Fatalf("replay lookup error = %v", err)
	}
	var count int
	if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM domain_events`).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 0 {
		t.Fatalf("event count = %d, want 0", count)
	}
	if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM tickets`).Scan(&count); err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 0 {
		t.Fatalf("ticket count = %d, want 0", count)
	}
}

type powerRetryBehaviorChild struct {
	parentID string
	ticketID string
	eventID  string
	vmID     string
}

func seedPowerRetryBehaviorChild(
	t *testing.T,
	pool *pgxpool.Pool,
	suffix string,
	vmID string,
	attemptCount int,
) powerRetryBehaviorChild {
	t.Helper()

	child := powerRetryBehaviorChild{
		parentID: "batch-retry-" + suffix,
		ticketID: "ticket-retry-" + suffix,
		eventID:  "event-retry-" + suffix,
		vmID:     vmID,
	}
	seedFailedBatchPowerRetryParent(t, pool, child.parentID, "start", child.vmID)
	payload := mustPowerRetryEventPayload(t, child.vmID, "start")
	if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, child.eventID, child.vmID, payload); err != nil {
		t.Fatalf("seed retry event: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (
    id, created_at, updated_at, event_id, operation_type, status, requester,
    reject_reason, parent_ticket_id, attempt_count
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor', 'seed failure', $3, $4)
`, child.ticketID, child.eventID, child.parentID, attemptCount); err != nil {
		t.Fatalf("seed retry ticket: %v", err)
	}
	return child
}

func mustPowerRetryEventPayload(t *testing.T, vmID, operation string) []byte {
	t.Helper()
	payload, err := (domain.VMPowerPayload{
		VMID:         vmID,
		VMName:       vmID,
		ClusterID:    "cluster-retry-test",
		Namespace:    "namespace-retry-test",
		Operation:    operation,
		Actor:        "retry-actor",
		DispatchMode: domain.VMPowerDispatchTicket,
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal %s power retry payload for %s: %v", operation, vmID, err)
	}
	return payload
}

func retryBehaviorInput(children ...powerRetryBehaviorChild) BatchPowerRetryInput {
	input := BatchPowerRetryInput{ParentID: children[0].parentID}
	for _, child := range children {
		input.Children = append(input.Children, BatchPowerRetryChildInput{
			TicketID: child.ticketID,
			EventID:  child.eventID,
		})
	}
	return input
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_PublicFailureContracts(t *testing.T) {
	t.Run("missing event preserves terminal ticket", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rm")
		input := BatchPowerRetryInput{
			ParentID: "batch-retry-missing",
			Children: []BatchPowerRetryChildInput{{
				TicketID: "ticket-retry-missing",
				EventID:  "event-retry-missing",
			}},
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), input)
		var notEligible *PowerRetryNotEligibleError
		if !errors.As(err, &notEligible) {
			t.Fatalf("missing event error = %v, want *PowerRetryNotEligibleError", err)
		}
		if notEligible.TicketID != input.Children[0].TicketID || notEligible.EventID != input.Children[0].EventID {
			t.Fatalf("missing event conflict = %+v, want requested ticket/event identity", notEligible)
		}
		assertAtomicBehaviorCounts(t, store.pool, 0, 0, 0, 0)
	})

	t.Run("missing parent fails before child mutation", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rmp")
		child := seedPowerRetryBehaviorChild(t, store.pool, "missing-parent", "vm-retry-missing-parent", 0)
		persistedParentID := child.parentID
		child.parentID += "-absent"
		if _, err := store.pool.Exec(t.Context(), `UPDATE tickets SET parent_ticket_id = $1 WHERE id = $2`, child.parentID, child.ticketID); err != nil {
			t.Fatalf("disconnect retry child from parent: %v", err)
		}
		if _, err := store.pool.Exec(t.Context(), `DELETE FROM batch_tickets WHERE id = $1`, persistedParentID); err != nil {
			t.Fatalf("remove retry parent projection: %v", err)
		}
		if _, err := store.pool.Exec(t.Context(), `DELETE FROM tickets WHERE id = $1`, persistedParentID); err != nil {
			t.Fatalf("remove retry parent ticket: %v", err)
		}
		if _, err := store.pool.Exec(t.Context(), `DELETE FROM domain_events WHERE id = $1`, persistedParentID+"-event"); err != nil {
			t.Fatalf("remove retry parent event: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var parentNotEligible *BatchRetryParentNotEligibleError
		if !errors.As(err, &parentNotEligible) {
			t.Fatalf("missing parent retry error = %v, want *BatchRetryParentNotEligibleError", err)
		}
		if parentNotEligible.ParentTicketID != child.parentID || parentNotEligible.ParentEventID != "" {
			t.Fatalf("missing parent retry conflict = %+v, want missing parent identity", parentNotEligible)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
		assertAtomicBehaviorCounts(t, store.pool, 1, 1, 0, 0)
	})

	t.Run("duplicate VM targets are rejected before mutation", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rd")
		first := seedPowerRetryBehaviorChild(t, store.pool, "duplicate-first", "vm-retry-duplicate", 0)
		second := seedPowerRetryBehaviorChild(t, store.pool, "duplicate-second", first.vmID, 0)
		second.parentID = first.parentID
		if _, err := store.pool.Exec(t.Context(), `UPDATE tickets SET parent_ticket_id = $1 WHERE id = $2`, first.parentID, second.ticketID); err != nil {
			t.Fatalf("align duplicate child's persisted parent: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(first, second))
		if err == nil || !strings.Contains(err.Error(), "target the same VM") {
			t.Fatalf("duplicate VM retry error = %v", err)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, first.ticketID, first.eventID, "FAILED", "seed failure", "FAILED")
		assertPowerRetryChildState(t.Context(), t, store.pool, second.ticketID, second.eventID, "FAILED", "seed failure", "FAILED")
	})

	t.Run("early River probe failure preserves terminal child", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rq")
		child := seedPowerRetryBehaviorChild(t, store.pool, "lookup-failure", "vm-retry-lookup", 0)
		if _, err := store.pool.Exec(t.Context(), `DROP TABLE river_job`); err != nil {
			t.Fatalf("remove River job store: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		if err == nil || !strings.Contains(err.Error(), "probe runnable vm_power retry") {
			t.Fatalf("early River probe error = %v", err)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
	})

	t.Run("no-op ticket transition returns a stable not-eligible conflict", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rn")
		child := seedPowerRetryBehaviorChild(t, store.pool, "not-eligible", "vm-retry-not-eligible", 0)
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION skip_power_retry_ticket_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RETURN NULL;
END;
$$;
CREATE TRIGGER skip_power_retry_ticket_update
BEFORE UPDATE ON tickets
FOR EACH ROW EXECUTE FUNCTION skip_power_retry_ticket_update();
`); err != nil {
			t.Fatalf("install no-op ticket transition: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var notEligible *PowerRetryNotEligibleError
		if !errors.As(err, &notEligible) {
			t.Fatalf("no-op retry error = %v, want *PowerRetryNotEligibleError", err)
		}
		if notEligible.TicketID != child.ticketID || !strings.Contains(err.Error(), child.eventID) {
			t.Fatalf("not-eligible error = %+v (%q), want child ticket/event", notEligible, err.Error())
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
	})

	t.Run("attempt cap returns a stable exhausted conflict", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "re")
		child := seedPowerRetryBehaviorChild(t, store.pool, "exhausted", "vm-retry-exhausted", domain.BatchChildMaxAttempts)
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var exhausted *BatchChildAttemptsExhaustedError
		if !errors.As(err, &exhausted) {
			t.Fatalf("exhausted retry error = %v, want *BatchChildAttemptsExhaustedError", err)
		}
		if exhausted.AttemptCount != domain.BatchChildMaxAttempts ||
			!strings.Contains(err.Error(), child.ticketID) ||
			!strings.Contains(err.Error(), "3/3") {
			t.Fatalf("exhausted error = %+v (%q), want persisted 3/3 cap", exhausted, err.Error())
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
	})

	t.Run("River insert failure rolls back ticket and event reset", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "ri")
		child := seedPowerRetryBehaviorChild(t, store.pool, "insert-failure", "vm-retry-insert", 0)
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION reject_power_retry_job_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RAISE EXCEPTION 'forced retry enqueue failure';
END;
$$;
CREATE TRIGGER reject_power_retry_job_insert
BEFORE INSERT ON river_job
FOR EACH ROW EXECUTE FUNCTION reject_power_retry_job_insert();
`); err != nil {
			t.Fatalf("install retry enqueue failure: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		if err == nil || !strings.Contains(err.Error(), "enqueue vm_power retry") {
			t.Fatalf("retry enqueue error = %v", err)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
		assertAtomicBehaviorCounts(t, store.pool, 2, 2, 1, 0)
	})

	t.Run("stale terminal ticket returns stable conflict without River probe", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rp")
		child := seedPowerRetryBehaviorChild(t, store.pool, "probe-failure", "vm-retry-probe", 0)
		if _, err := store.pool.Exec(t.Context(), `
UPDATE tickets
SET status = 'SUCCESS', reject_reason = NULL
WHERE id = $1
`, child.ticketID); err != nil {
			t.Fatalf("mark retry ticket successful: %v", err)
		}
		if _, err := store.pool.Exec(t.Context(), `
UPDATE batch_tickets
SET success_count = 1, failed_count = 0, pending_count = 0
WHERE id = $1
`, child.parentID); err != nil {
			t.Fatalf("align stale terminal projection counters: %v", err)
		}
		if _, err := store.pool.Exec(t.Context(), `
CREATE FUNCTION reject_power_retry_probe_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	RAISE EXCEPTION 'forced retry probe failure';
END;
$$;
CREATE TRIGGER reject_power_retry_probe_insert
BEFORE INSERT ON river_job
FOR EACH ROW EXECUTE FUNCTION reject_power_retry_probe_insert();
`); err != nil {
			t.Fatalf("install retry probe failure: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var notEligible *PowerRetryNotEligibleError
		if !errors.As(err, &notEligible) {
			t.Fatalf("stale terminal retry error = %v, want *PowerRetryNotEligibleError", err)
		}
		if notEligible.TicketID != child.ticketID || notEligible.EventID != child.eventID {
			t.Fatalf("stale terminal conflict = %+v, want child ticket/event identity", notEligible)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "SUCCESS", "", "FAILED")
		assertAtomicBehaviorCounts(t, store.pool, 2, 2, 1, 0)
	})

	t.Run("existing runnable job returns stable conflict and rolls back reset", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "rj")
		child := seedPowerRetryBehaviorChild(t, store.pool, "job-conflict", "vm-retry-job-conflict", 0)
		inserted, err := store.riverClient.Insert(t.Context(), jobs.VMPowerArgs{EventID: child.eventID}, nil)
		if err != nil {
			t.Fatalf("seed existing River job: %v", err)
		}
		err = store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var conflict *PowerRetryJobConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("existing-job retry error = %v, want *PowerRetryJobConflictError", err)
		}
		if conflict.ExistingJobID != inserted.Job.ID ||
			!strings.Contains(err.Error(), child.eventID) ||
			!strings.Contains(err.Error(), conflict.ExistingJobState) {
			t.Fatalf("job conflict = %+v (%q), want existing River job", conflict, err.Error())
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
		assertAtomicBehaviorCounts(t, store.pool, 2, 2, 1, 1)
	})

	t.Run("different active power event blocks retry", func(t *testing.T) {
		t.Parallel()
		store := newApprovalAtomicBehaviorStore(t, "ra")
		child := seedPowerRetryBehaviorChild(t, store.pool, "active-conflict", "vm-retry-active", 0)
		activeInput := directPowerBehaviorInput("event-retry-active-blocker", child.vmID, powerEventTypeStop)
		if err := store.writer.CreatePowerEventAndEnqueue(t.Context(), activeInput); err != nil {
			t.Fatalf("create active power blocker: %v", err)
		}
		err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), retryBehaviorInput(child))
		var active *ActivePowerEventError
		if !errors.As(err, &active) {
			t.Fatalf("active retry error = %v, want *ActivePowerEventError", err)
		}
		if active.ExistingEventID != activeInput.EventID || !strings.Contains(err.Error(), activeInput.EventID) {
			t.Fatalf("active retry error = %+v (%q), want blocker %s", active, err.Error(), activeInput.EventID)
		}
		assertPowerRetryChildState(t.Context(), t, store.pool, child.ticketID, child.eventID, "FAILED", "seed failure", "FAILED")
		assertAtomicBehaviorCounts(t, store.pool, 3, 2, 1, 1)
	})
}

func assertAtomicApplicationRows(t *testing.T, pool *pgxpool.Pool, events, tickets, batches int) {
	t.Helper()

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{name: "events", query: `SELECT count(*) FROM domain_events`, want: events},
		{name: "tickets", query: `SELECT count(*) FROM tickets`, want: tickets},
		{name: "batches", query: `SELECT count(*) FROM batch_tickets`, want: batches},
	}
	for _, check := range checks {
		var got int
		if err := pool.QueryRow(t.Context(), check.query).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, got, check.want)
		}
	}
}

func assertAtomicBehaviorCounts(t *testing.T, pool *pgxpool.Pool, events, tickets, batches, jobsCount int) {
	t.Helper()

	queries := []struct {
		name  string
		query string
		want  int
	}{
		{name: "events", query: `SELECT count(*) FROM domain_events`, want: events},
		{name: "tickets", query: `SELECT count(*) FROM tickets`, want: tickets},
		{name: "batches", query: `SELECT count(*) FROM batch_tickets`, want: batches},
		{name: "jobs", query: `SELECT count(*) FROM river_job`, want: jobsCount},
	}
	for _, check := range queries {
		var got int
		if err := pool.QueryRow(t.Context(), check.query).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, got, check.want)
		}
	}
}

func assertDeleteApprovalStillPending(ctx context.Context, t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ticketID, eventID string) {
	t.Helper()

	assertDeleteApprovalState(ctx, t, pool, ticketID, eventID, "PENDING", "PENDING")
}

func assertDeleteApprovalState(ctx context.Context, t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ticketID, eventID, wantTicketStatus, wantEventStatus string) {
	t.Helper()

	assertApprovalTicketAndEventState(ctx, t, pool, ticketID, eventID, wantTicketStatus, wantEventStatus, "vm_delete")
}

func assertApprovalTicketAndEventState(ctx context.Context, t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ticketID, eventID, wantTicketStatus, wantEventStatus, jobKind string) {
	t.Helper()

	var ticketStatus string
	var approver *string
	if err := pool.QueryRow(ctx, `SELECT status, approver FROM tickets WHERE id=$1`, ticketID).Scan(&ticketStatus, &approver); err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticketStatus != wantTicketStatus {
		t.Fatalf("ticket status = %q, want %s", ticketStatus, wantTicketStatus)
	}
	if approver != nil {
		t.Fatalf("ticket approver = %q, want nil", *approver)
	}

	var eventStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if eventStatus != wantEventStatus {
		t.Fatalf("event status = %q, want %s", eventStatus, wantEventStatus)
	}

	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, jobKind).Scan(&jobCount); err != nil {
		t.Fatalf("query river jobs: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("river %s job count = %d, want 0", jobKind, jobCount)
	}
}

func assertPowerRetryChildState(ctx context.Context, t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ticketID, eventID, wantTicketStatus, wantRejectReason, wantEventStatus string) {
	t.Helper()

	var ticketStatus string
	var rejectReason *string
	if err := pool.QueryRow(ctx, `SELECT status, reject_reason FROM tickets WHERE id=$1`, ticketID).Scan(&ticketStatus, &rejectReason); err != nil {
		t.Fatalf("query retry ticket: %v", err)
	}
	if ticketStatus != wantTicketStatus {
		t.Fatalf("ticket status = %q, want %q", ticketStatus, wantTicketStatus)
	}
	if wantRejectReason == "" {
		if rejectReason != nil {
			t.Fatalf("ticket reject_reason = %q, want nil", *rejectReason)
		}
		assertDomainEventStatus(ctx, t, pool, eventID, wantEventStatus)
		return
	}
	if rejectReason == nil {
		t.Fatalf("ticket reject_reason = nil, want %q", wantRejectReason)
	}
	if *rejectReason != wantRejectReason {
		t.Fatalf("ticket reject_reason = %q, want %q", *rejectReason, wantRejectReason)
	}

	assertDomainEventStatus(ctx, t, pool, eventID, wantEventStatus)
}

func assertDomainEventStatus(ctx context.Context, t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, eventID, wantStatus string) {
	t.Helper()

	var eventStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query domain event: %v", err)
	}
	if eventStatus != wantStatus {
		t.Fatalf("event status = %q, want %q", eventStatus, wantStatus)
	}
}

func TestSQLCInt32Count(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		count   int
		want    int32
		wantErr bool
	}{
		{name: "zero", count: 0, want: 0},
		{name: "max int32", count: maxBatchPowerChildCountForSQLCInt, want: maxBatchPowerChildCountForSQLCInt},
		{name: "negative", count: -1, wantErr: true},
		{name: "overflow", count: maxBatchPowerChildCountForSQLCInt + 1, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sqlcInt32Count(tc.count, "test count")
			if tc.wantErr {
				if err == nil {
					t.Fatal("sqlcInt32Count() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("sqlcInt32Count() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("sqlcInt32Count() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestValidateBatchPowerSubmissionInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*BatchPowerSubmissionInput)
		valid  bool
	}{
		{name: "valid", valid: true},
		{name: "missing parent", mutate: func(input *BatchPowerSubmissionInput) { input.ParentID = "" }},
		{name: "missing actor", mutate: func(input *BatchPowerSubmissionInput) { input.Actor = "" }},
		{name: "missing payload", mutate: func(input *BatchPowerSubmissionInput) { input.ParentPayload = nil }},
		{name: "missing children", mutate: func(input *BatchPowerSubmissionInput) { input.Children = nil }},
		{name: "missing operation", mutate: func(input *BatchPowerSubmissionInput) { input.Operation = "" }},
		{name: "generic power operation", mutate: func(input *BatchPowerSubmissionInput) { input.Operation = "POWER" }},
		{name: "non-power operation", mutate: func(input *BatchPowerSubmissionInput) { input.Operation = "CREATE" }},
		{
			name: "long opaque request ID",
			mutate: func(input *BatchPowerSubmissionInput) {
				requestID := strings.Repeat("😀", 4096)
				input.RequestID = requestID
				var parent domain.BatchVMRequestPayload
				if err := json.Unmarshal(input.ParentPayload, &parent); err != nil {
					panic(err)
				}
				parent.RequestID = requestID
				input.ParentPayload = mustMarshalBatchPowerBehaviorJSON(parent)
			},
			valid: true,
		},
		{
			name: "parent operation mismatch",
			mutate: func(input *BatchPowerSubmissionInput) {
				var parent domain.BatchVMRequestPayload
				_ = json.Unmarshal(input.ParentPayload, &parent)
				parent.Operation = "POWER_STOP"
				input.ParentPayload = mustMarshalBatchPowerBehaviorJSON(parent)
			},
		},
		{
			name: "parent actor mismatch",
			mutate: func(input *BatchPowerSubmissionInput) {
				var parent domain.BatchVMRequestPayload
				_ = json.Unmarshal(input.ParentPayload, &parent)
				parent.SubmittedBy = "another-actor"
				input.ParentPayload = mustMarshalBatchPowerBehaviorJSON(parent)
			},
		},
		{name: "child missing event type", mutate: func(input *BatchPowerSubmissionInput) { input.Children[0].EventType = "" }},
		{name: "child mismatched event type", mutate: func(input *BatchPowerSubmissionInput) { input.Children[0].EventType = powerEventTypeStop }},
		{name: "child missing aggregate", mutate: func(input *BatchPowerSubmissionInput) { input.Children[0].AggregateID = "" }},
		{name: "child missing payload", mutate: func(input *BatchPowerSubmissionInput) { input.Children[0].Payload = nil }},
		{
			name: "child actor mismatch",
			mutate: func(input *BatchPowerSubmissionInput) {
				var payload domain.VMPowerPayload
				_ = json.Unmarshal(input.Children[0].Payload, &payload)
				payload.Actor = "another-actor"
				input.Children[0].Payload = mustMarshalBatchPowerBehaviorJSON(payload)
			},
		},
		{
			name: "child missing ticket dispatch mode",
			mutate: func(input *BatchPowerSubmissionInput) {
				var payload domain.VMPowerPayload
				_ = json.Unmarshal(input.Children[0].Payload, &payload)
				payload.DispatchMode = ""
				input.Children[0].Payload = mustMarshalBatchPowerBehaviorJSON(payload)
			},
		},
		{
			name: "child operation mismatch",
			mutate: func(input *BatchPowerSubmissionInput) {
				var payload domain.VMPowerPayload
				_ = json.Unmarshal(input.Children[0].Payload, &payload)
				payload.Operation = "stop"
				input.Children[0].Payload = mustMarshalBatchPowerBehaviorJSON(payload)
			},
		},
		{
			name: "parent item mismatch",
			mutate: func(input *BatchPowerSubmissionInput) {
				var parent domain.BatchVMRequestPayload
				_ = json.Unmarshal(input.ParentPayload, &parent)
				parent.Items[0].VMID = "another-vm"
				input.ParentPayload = mustMarshalBatchPowerBehaviorJSON(parent)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := batchPowerBehaviorInput("batch-1", "vm-1")
			input.ParentPayload = append([]byte(nil), input.ParentPayload...)
			input.Children = append([]BatchPowerChildInput(nil), input.Children...)
			input.Children[0].Payload = append([]byte(nil), input.Children[0].Payload...)
			if tc.mutate != nil {
				tc.mutate(&input)
			}
			err := validateBatchPowerSubmissionInput(input)
			if !tc.valid && err == nil {
				t.Fatal("validateBatchPowerSubmissionInput() error = nil, want error")
			}
			if tc.valid && err != nil {
				t.Fatalf("validateBatchPowerSubmissionInput() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateBatchPowerRetryInput(t *testing.T) {
	t.Parallel()

	validChild := BatchPowerRetryChildInput{TicketID: "ticket-1", EventID: "event-1"}
	valid := BatchPowerRetryInput{
		ParentID: "batch-1",
		Children: []BatchPowerRetryChildInput{validChild},
	}

	tests := []struct {
		name    string
		input   BatchPowerRetryInput
		wantErr bool
	}{
		{name: "valid", input: valid},
		{name: "missing parent", input: BatchPowerRetryInput{Children: valid.Children}, wantErr: true},
		{name: "missing children", input: BatchPowerRetryInput{ParentID: valid.ParentID}, wantErr: true},
		{name: "child missing ticket", input: BatchPowerRetryInput{ParentID: valid.ParentID, Children: []BatchPowerRetryChildInput{{EventID: validChild.EventID}}}, wantErr: true},
		{name: "child missing event", input: BatchPowerRetryInput{ParentID: valid.ParentID, Children: []BatchPowerRetryChildInput{{TicketID: validChild.TicketID}}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBatchPowerRetryInput(tc.input)
			if tc.wantErr && err == nil {
				t.Fatal("validateBatchPowerRetryInput() error = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateBatchPowerRetryInput() unexpected error: %v", err)
			}
		})
	}
}

func TestVMStatusSyncInsertOpts(t *testing.T) {
	t.Parallel()

	opts := vmStatusSyncInsertOpts()
	if opts == nil {
		t.Fatal("vmStatusSyncInsertOpts() returned nil")
		return
	}
	if opts.Queue != jobs.VMStatusSyncJobKind {
		t.Fatalf("queue = %q, want %q", opts.Queue, jobs.VMStatusSyncJobKind)
	}
	if opts.MaxAttempts != 3 {
		t.Fatalf("max attempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs || !opts.UniqueOpts.ByQueue {
		t.Fatalf("unique opts = %+v, want ByArgs=true and ByQueue=true", opts.UniqueOpts)
	}
}

func TestSnapshotRootVolumeHelpers(t *testing.T) {
	t.Parallel()

	values := map[string]interface{}{
		"dv_access_modes": []interface{}{"ReadWriteMany", "  ", "ReadWriteOnce"},
		"dv_volume_mode":  " Block ",
	}

	if got := snapshotString(values, "dv_volume_mode"); got != "Block" {
		t.Fatalf("snapshotString(volume_mode) = %q, want Block", got)
	}

	gotModes := snapshotStringSlice(values)
	if len(gotModes) != 2 || gotModes[0] != "ReadWriteMany" || gotModes[1] != "ReadWriteOnce" {
		t.Fatalf("snapshotStringSlice(access_modes) = %#v, want [ReadWriteMany ReadWriteOnce]", gotModes)
	}
}

func TestSnapshotString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values map[string]interface{}
		key    string
		want   string
	}{
		{
			name:   "trims string",
			values: map[string]interface{}{"dv_volume_mode": " Block "},
			key:    "dv_volume_mode",
			want:   "Block",
		},
		{
			name:   "missing key",
			values: map[string]interface{}{"other": "Block"},
			key:    "dv_volume_mode",
		},
		{
			name:   "non-string value",
			values: map[string]interface{}{"dv_volume_mode": 42},
			key:    "dv_volume_mode",
		},
		{
			name: "empty map",
			key:  "dv_volume_mode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotString(tc.values, tc.key); got != tc.want {
				t.Fatalf("snapshotString() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSnapshotStringSlice(t *testing.T) {
	t.Parallel()

	t.Run("clones string slice", func(t *testing.T) {
		source := []string{"ReadWriteMany", "ReadWriteOnce"}
		got := snapshotStringSlice(map[string]interface{}{"dv_access_modes": source})
		if !slices.Equal(got, source) {
			t.Fatalf("snapshotStringSlice() = %#v, want %#v", got, source)
		}
		got[0] = "mutated"
		if source[0] != "ReadWriteMany" {
			t.Fatal("snapshotStringSlice() returned a slice aliased to the source")
		}
	})

	t.Run("filters interface slice", func(t *testing.T) {
		got := snapshotStringSlice(map[string]interface{}{
			"dv_access_modes": []interface{}{" ReadWriteMany ", "", 42, "ReadWriteOnce"},
		})
		want := []string{"ReadWriteMany", "ReadWriteOnce"}
		if !slices.Equal(got, want) {
			t.Fatalf("snapshotStringSlice() = %#v, want %#v", got, want)
		}
	})

	t.Run("empty and invalid values return nil", func(t *testing.T) {
		tests := []map[string]interface{}{
			nil,
			{"dv_access_modes": []string{}},
			{"dv_access_modes": []interface{}{" ", 42}},
			{"dv_access_modes": "ReadWriteMany"},
			{"other": []string{"ReadWriteMany"}},
		}
		for _, values := range tests {
			if got := snapshotStringSlice(values); got != nil {
				t.Fatalf("snapshotStringSlice(%#v) = %#v, want nil", values, got)
			}
		}
	})
}

func TestMarshalJSONArrayOrNull(t *testing.T) {
	t.Parallel()

	data, err := marshalJSONArrayOrNull(nil)
	if err != nil {
		t.Fatalf("marshalJSONArrayOrNull(nil) error = %v", err)
	}
	if data != nil {
		t.Fatalf("marshalJSONArrayOrNull(nil) = %s, want nil", string(data))
	}

	data, err = marshalJSONArrayOrNull([]string{"ReadWriteMany"})
	if err != nil {
		t.Fatalf("marshalJSONArrayOrNull(non-empty) error = %v", err)
	}
	if string(data) != `["ReadWriteMany"]` {
		t.Fatalf("marshalJSONArrayOrNull(non-empty) = %s, want [\"ReadWriteMany\"]", string(data))
	}
}
