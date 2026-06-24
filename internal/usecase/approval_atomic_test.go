package usecase

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"kv-shepherd.io/shepherd/internal/jobs"
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
		Payload:       []byte(`{"operation":"start"}`),
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
	}); err == nil {
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
	err = w.CreateBatchPowerAndMaybeEnqueue(ctx, BatchPowerSubmissionInput{
		ParentID:      "batch-atomic-1",
		Actor:         "user-1",
		RequestID:     "request-1",
		Reason:        "batch power start",
		ParentPayload: []byte(`{"operation":"POWER_START","items":[]}`),
		Children: []BatchPowerChildInput{
			{
				EventType:   "VM_START_REQUESTED",
				AggregateID: "vm-1",
				Payload:     []byte(`{"vm_id":"vm-1","operation":"start"}`),
				Reason:      "start vm",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatchPowerAndMaybeEnqueue() unexpected error: %v", err)
	}

	var parentStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, "batch-atomic-1").Scan(&parentStatus); err != nil {
		t.Fatalf("query parent ticket: %v", err)
	}
	if parentStatus != "EXECUTING" {
		t.Fatalf("parent status = %q, want EXECUTING", parentStatus)
	}

	var childStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE parent_ticket_id=$1`, "batch-atomic-1").Scan(&childStatus); err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if childStatus != "EXECUTING" {
		t.Fatalf("child status = %q, want EXECUTING", childStatus)
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
		wantErr         string
	}{
		{
			name:            "event mismatch rejected before ticket reset",
			suffix:          "mismatch",
			eventStatus:     "FAILED",
			useMismatchedID: true,
			wantErr:         "event mismatch",
		},
		{
			name:        "event not failed rolls back ticket reset",
			suffix:      "event-processing",
			eventStatus: "PROCESSING",
			wantErr:     "not found or not retryable",
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
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want contains %q", err, tc.wantErr)
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

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_RejectedChildEnqueues(t *testing.T) {
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

	w := NewApprovalAtomicWriter(pool, riverClient)
	if err := w.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{
			TicketID: ticketID,
			EventID:  eventID,
		}},
	}); err != nil {
		t.Fatalf("RetryBatchPowerAndEnqueue() unexpected error: %v", err)
	}

	assertPowerRetryChildState(ctx, t, pool, ticketID, eventID, "EXECUTING", "", "PENDING")
	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, "vm_power").Scan(&jobCount); err != nil {
		t.Fatalf("query river jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("river vm_power job count = %d, want 1", jobCount)
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

	validChild := BatchPowerChildInput{
		EventType:   "VM_START_REQUESTED",
		AggregateID: "vm-1",
		Payload:     []byte(`{"operation":"start"}`),
	}
	valid := BatchPowerSubmissionInput{
		ParentID:      "batch-1",
		Actor:         "user-1",
		ParentPayload: []byte(`{"operation":"POWER_START"}`),
		Children:      []BatchPowerChildInput{validChild},
	}

	tests := []struct {
		name    string
		input   BatchPowerSubmissionInput
		wantErr bool
	}{
		{name: "valid", input: valid},
		{name: "missing parent", input: BatchPowerSubmissionInput{Actor: valid.Actor, ParentPayload: valid.ParentPayload, Children: valid.Children}, wantErr: true},
		{name: "missing actor", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, ParentPayload: valid.ParentPayload, Children: valid.Children}, wantErr: true},
		{name: "missing payload", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, Actor: valid.Actor, Children: valid.Children}, wantErr: true},
		{name: "missing children", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, Actor: valid.Actor, ParentPayload: valid.ParentPayload}, wantErr: true},
		{name: "child missing event type", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, Actor: valid.Actor, ParentPayload: valid.ParentPayload, Children: []BatchPowerChildInput{{AggregateID: "vm-1", Payload: validChild.Payload}}}, wantErr: true},
		{name: "child missing aggregate", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, Actor: valid.Actor, ParentPayload: valid.ParentPayload, Children: []BatchPowerChildInput{{EventType: validChild.EventType, Payload: validChild.Payload}}}, wantErr: true},
		{name: "child missing payload", input: BatchPowerSubmissionInput{ParentID: valid.ParentID, Actor: valid.Actor, ParentPayload: valid.ParentPayload, Children: []BatchPowerChildInput{{EventType: validChild.EventType, AggregateID: validChild.AggregateID}}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBatchPowerSubmissionInput(tc.input)
			if tc.wantErr && err == nil {
				t.Fatal("validateBatchPowerSubmissionInput() error = nil, want error")
			}
			if !tc.wantErr && err != nil {
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
