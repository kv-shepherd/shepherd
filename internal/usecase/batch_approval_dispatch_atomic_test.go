package usecase

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
)

func TestApprovalAtomicWriterClaimBatchApprovalAndEnqueue_CommitsDurableSnapshot(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_claim")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-claim-parent",
		parentStatus: "PENDING",
		parentEvent:  "PENDING",
		batchStatus:  "PENDING_APPROVAL",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-claim-child",
			eventID:     "batch-claim-child-event",
			ticketState: "PENDING",
			eventState:  "PENDING",
		}},
	})

	execution := domain.BatchApprovalExecutionOptions{
		ClusterID:       " cluster-a ",
		StorageClass:    " fast-rwx ",
		DVAccessModes:   []string{" ReadWriteMany ", " ", "ReadWriteOnce"},
		DVVolumeMode:    " Block ",
		EnableOverride:  true,
		CPURequest:      2.5,
		CPULimit:        4,
		MemoryRequestGi: 8,
		MemoryLimitGi:   12,
		DiskGB:          120,
	}
	err := store.writer.ClaimBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalClaimInput{
		ParentTicketID: " batch-claim-parent ",
		ParentEventID:  " batch-claim-parent-event ",
		Approver:       " approver-a ",
		Execution:      execution,
	})
	if err != nil {
		t.Fatalf("ClaimBatchApprovalAndEnqueue() error = %v", err)
	}

	state := loadBatchApprovalDispatchState(t, store.pool, "batch-claim-parent")
	if state.parentStatus != "EXECUTING" || state.parentEvent != "PROCESSING" {
		t.Fatalf("parent state = (%q, %q), want (EXECUTING, PROCESSING)", state.parentStatus, state.parentEvent)
	}
	if state.approver != "approver-a" || state.clusterID != "cluster-a" || state.storageClass != "fast-rwx" {
		t.Fatalf("durable approval fields = (%q, %q, %q), want trimmed values", state.approver, state.clusterID, state.storageClass)
	}
	if state.batchStatus != "IN_PROGRESS" || state.childCount != 1 || state.pendingCount != 1 {
		t.Fatalf("batch projection = %+v, want one pending child in progress", state)
	}
	wantExecution := execution
	wantExecution.ClusterID = "cluster-a"
	wantExecution.StorageClass = "fast-rwx"
	wantExecution.DVAccessModes = []string{"ReadWriteMany", "ReadWriteOnce"}
	wantExecution.DVVolumeMode = "Block"
	assertBatchApprovalExecutionSnapshot(t, state.modifiedSpec, wantExecution)
	assertBatchApprovalChildState(t, store.pool, "batch-claim-child", "PENDING", "PENDING", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-claim-child", "", false)
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-claim-parent", 1)
}

func TestApprovalAtomicWriterClaimBatchApprovalAndEnqueue_SupportsInitialPowerBatch(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_power_claim")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:        "batch-power-claim-parent",
		parentStatus:    "PENDING",
		parentEvent:     "PENDING",
		parentEventType: "BATCH_POWER_REQUESTED",
		operationType:   "POWER",
		batchType:       "BATCH_POWER",
		childEventType:  "VM_START_REQUESTED",
		batchStatus:     "PENDING_APPROVAL",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-power-claim-child",
			eventID:     "batch-power-claim-child-event",
			ticketState: "PENDING",
			eventState:  "PENDING",
		}},
	})

	err := store.writer.ClaimBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalClaimInput{
		ParentTicketID: "batch-power-claim-parent",
		ParentEventID:  "batch-power-claim-parent-event",
		Approver:       "power-approver",
	})
	if err != nil {
		t.Fatalf("ClaimBatchApprovalAndEnqueue() power parent error = %v", err)
	}

	state := loadBatchApprovalDispatchState(t, store.pool, "batch-power-claim-parent")
	if state.parentStatus != "EXECUTING" || state.parentEvent != "PROCESSING" || state.approver != "power-approver" {
		t.Fatalf("power parent claim state = %+v, want executing/processing with approver", state)
	}
	if state.batchStatus != "IN_PROGRESS" || state.childCount != 1 || state.pendingCount != 1 || state.failedCount != 0 {
		t.Fatalf("power batch projection = %+v, want one pending child in progress", state)
	}
	assertBatchApprovalChildState(t, store.pool, "batch-power-claim-child", "PENDING", "PENDING", 0)
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-power-claim-parent", 1)
}

func TestTicketAttemptCountDatabaseConstraintRejectsNegativeValue(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "ticket_attempt_count_check")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-attempt-check-parent",
		parentStatus: "PENDING",
		parentEvent:  "PENDING",
		batchStatus:  "PENDING_APPROVAL",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-attempt-check-child",
			eventID:     "batch-attempt-check-child-event",
			ticketState: "PENDING",
			eventState:  "PENDING",
		}},
	})

	_, err := store.pool.Exec(
		t.Context(),
		`UPDATE tickets SET attempt_count = -1 WHERE id = $1`,
		"batch-attempt-check-child",
	)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "tickets_attempt_count_nonnegative" {
		t.Fatalf("negative attempt update error = %v, want CHECK violation for tickets_attempt_count_nonnegative", err)
	}
	assertBatchApprovalChildState(t, store.pool, "batch-attempt-check-child", "PENDING", "PENDING", 0)
}

func TestApprovalAtomicWriterValidateBatchApprovalDispatchGraph_RejectsNegativeAttemptWithoutWrites(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_negative_attempt")
	dropTicketAttemptCountConstraintForCorruptionTest(t, store.pool)
	const (
		parentID = "batch-dispatch-negative-parent"
		childID  = "batch-dispatch-negative-child"
	)
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     parentID,
		parentStatus: "EXECUTING",
		parentEvent:  "PROCESSING",
		batchStatus:  "IN_PROGRESS",
		children: []batchApprovalDispatchChild{{
			ticketID:     childID,
			eventID:      childID + "-event",
			ticketState:  "PENDING",
			eventState:   "PENDING",
			attemptCount: -1,
		}},
	})
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	_, err := store.writer.ValidateBatchApprovalDispatchGraph(t.Context(), parentID, parentID+"-event")
	if err == nil || !strings.Contains(err.Error(), "attempt count is negative") {
		t.Fatalf("ValidateBatchApprovalDispatchGraph() error = %v, want negative attempt rejection", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("negative dispatch validation changed graph\nbefore: %s\nafter:  %s", before, after)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
}

func TestApprovalAtomicWriterGuardedBatchDispatch_AllowsEarlierSiblingOutput(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_guard_siblings")
	const parentID = "batch-dispatch-guard-parent"
	children := []batchApprovalDispatchChild{
		{ticketID: "batch-dispatch-guard-child-1", eventID: "batch-dispatch-guard-child-1-event", ticketState: "PENDING", eventState: "PENDING"},
		{ticketID: "batch-dispatch-guard-child-2", eventID: "batch-dispatch-guard-child-2-event", ticketState: "PENDING", eventState: "PENDING"},
	}
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:        parentID,
		parentStatus:    "EXECUTING",
		parentEvent:     "PROCESSING",
		parentEventType: "BATCH_POWER_REQUESTED",
		operationType:   "POWER",
		batchType:       "BATCH_POWER",
		childEventType:  "VM_START_REQUESTED",
		batchStatus:     "IN_PROGRESS",
		children:        children,
	})

	guard, err := store.writer.ValidateBatchApprovalDispatchGraph(t.Context(), parentID, parentID+"-event")
	if err != nil {
		t.Fatalf("ValidateBatchApprovalDispatchGraph() error = %v", err)
	}
	for _, child := range children {
		if err := store.writer.ApproveBatchPowerAndEnqueue(
			t.Context(), guard, child.ticketID, child.eventID, guard.Approver, "start",
		); err != nil {
			t.Fatalf("ApproveBatchPowerAndEnqueue(%s) error = %v", child.ticketID, err)
		}
		assertBatchApprovalChildState(t, store.pool, child.ticketID, "APPROVED", "PENDING", 1)
	}
	assertRiverJobKindCount(t, store.pool, jobs.VMPowerArgs{}.Kind(), 2)
}

func TestApprovalAtomicWriterGuardedBatchDispatch_RejectsLaterSiblingInputTampering(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_dispatch_guard_tamper")
	const (
		parentID = "batch-dispatch-tamper-parent"
		child1ID = "batch-dispatch-tamper-child-1"
		child2ID = "batch-dispatch-tamper-child-2"
	)
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:        parentID,
		parentStatus:    "EXECUTING",
		parentEvent:     "PROCESSING",
		parentEventType: "BATCH_POWER_REQUESTED",
		operationType:   "POWER",
		batchType:       "BATCH_POWER",
		childEventType:  "VM_START_REQUESTED",
		batchStatus:     "IN_PROGRESS",
		children: []batchApprovalDispatchChild{
			{ticketID: child1ID, eventID: child1ID + "-event", ticketState: "PENDING", eventState: "PENDING"},
			{ticketID: child2ID, eventID: child2ID + "-event", ticketState: "PENDING", eventState: "PENDING"},
		},
	})

	guard, err := store.writer.ValidateBatchApprovalDispatchGraph(t.Context(), parentID, parentID+"-event")
	if err != nil {
		t.Fatalf("ValidateBatchApprovalDispatchGraph() error = %v", err)
	}
	if approvalErr := store.writer.ApproveBatchPowerAndEnqueue(
		t.Context(), guard, child1ID, child1ID+"-event", guard.Approver, "start",
	); approvalErr != nil {
		t.Fatalf("ApproveBatchPowerAndEnqueue(first child) error = %v", approvalErr)
	}
	if _, tamperErr := store.pool.Exec(
		t.Context(),
		`UPDATE tickets SET modified_spec = '{"template_id":"tampered"}'::jsonb WHERE id = $1`,
		child2ID,
	); tamperErr != nil {
		t.Fatalf("tamper second child modified_spec: %v", tamperErr)
	}

	err = store.writer.ApproveBatchPowerAndEnqueue(
		t.Context(), guard, child2ID, child2ID+"-event", guard.Approver, "start",
	)
	if err == nil || !strings.Contains(err.Error(), "child input changed after validation") {
		t.Fatalf("ApproveBatchPowerAndEnqueue(tampered child) error = %v, want guarded input rejection", err)
	}
	assertBatchApprovalChildState(t, store.pool, child1ID, "APPROVED", "PENDING", 1)
	assertBatchApprovalChildState(t, store.pool, child2ID, "PENDING", "PENDING", 0)
	assertRiverJobKindCount(t, store.pool, jobs.VMPowerArgs{}.Kind(), 1)
}

func TestApprovalAtomicWriterClaimBatchApprovalAndEnqueue_StaleParentOrEventRollsBack(t *testing.T) {
	tests := []struct {
		name         string
		parentStatus string
		parentEvent  string
	}{
		{name: "parent already failed", parentStatus: "FAILED", parentEvent: "PENDING"},
		{name: "parent event already completed", parentStatus: "PENDING", parentEvent: "COMPLETED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_approval_stale_claim")
			const (
				parentID = "batch-stale-claim-parent"
				childID  = "batch-stale-claim-child"
			)
			seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
				parentID:     parentID,
				parentStatus: tt.parentStatus,
				parentEvent:  tt.parentEvent,
				batchStatus:  "PENDING_APPROVAL",
				children: []batchApprovalDispatchChild{{
					ticketID:    childID,
					eventID:     childID + "-event",
					ticketState: "PENDING",
					eventState:  "PENDING",
				}},
			})

			err := store.writer.ClaimBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalClaimInput{
				ParentTicketID: parentID,
				ParentEventID:  parentID + "-event",
				Approver:       "new-approver",
			})
			if err == nil {
				t.Fatal("ClaimBatchApprovalAndEnqueue() stale state error = nil")
			}

			state := loadBatchApprovalDispatchState(t, store.pool, parentID)
			if state.parentStatus != tt.parentStatus || state.parentEvent != tt.parentEvent || state.batchStatus != "PENDING_APPROVAL" {
				t.Fatalf("stale claim durable state = %+v, want original %s/%s/PENDING_APPROVAL", state, tt.parentStatus, tt.parentEvent)
			}
			wantApprover := ""
			if tt.parentStatus != "PENDING" {
				wantApprover = "previous-approver"
			}
			if state.approver != wantApprover || state.clusterID != "" || state.storageClass != "" || state.modifiedSpec != "null" {
				t.Fatalf("stale claim approval snapshot changed: %+v", state)
			}
			assertBatchApprovalChildState(t, store.pool, childID, "PENDING", "PENDING", 0)
			assertBatchApprovalChildRetryMetadata(t, store.pool, childID, "", false)
			assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
		})
	}
}

func TestApprovalAtomicWriterClaimBatchApprovalAndEnqueue_MismatchedParentEventIdentityRollsBack(t *testing.T) {
	tests := []struct {
		name   string
		mutate string
	}{
		{
			name:   "wrong aggregate",
			mutate: `UPDATE domain_events SET aggregate_id = 'another-batch' WHERE id = $1`,
		},
		{
			name:   "wrong operation event type",
			mutate: `UPDATE domain_events SET event_type = 'BATCH_DELETE_REQUESTED' WHERE id = $1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_approval_claim_identity")
			const parentID = "batch-claim-identity-parent"
			seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
				parentID:     parentID,
				parentStatus: "PENDING",
				parentEvent:  "PENDING",
				batchStatus:  "PENDING_APPROVAL",
				children: []batchApprovalDispatchChild{{
					ticketID:    "batch-claim-identity-child",
					eventID:     "batch-claim-identity-child-event",
					ticketState: "PENDING",
					eventState:  "PENDING",
				}},
			})
			if _, err := store.pool.Exec(t.Context(), tt.mutate, parentID+"-event"); err != nil {
				t.Fatalf("corrupt parent event identity: %v", err)
			}

			err := store.writer.ClaimBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalClaimInput{
				ParentTicketID: parentID,
				ParentEventID:  parentID + "-event",
				Approver:       "approver-a",
			})
			if err == nil {
				t.Fatal("ClaimBatchApprovalAndEnqueue() mismatched identity error = nil")
			}
			state := loadBatchApprovalDispatchState(t, store.pool, parentID)
			if state.parentStatus != "PENDING" || state.parentEvent != "PENDING" || state.approver != "" || state.batchStatus != "PENDING_APPROVAL" {
				t.Fatalf("mismatched claim durable state = %+v, want untouched PENDING state", state)
			}
			assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
		})
	}
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_RollsBackEarlierChildOnLaterConflict(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_rollback")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-rollback-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{
			{
				ticketID:    "batch-retry-rollback-first",
				eventID:     "batch-retry-rollback-first-event",
				ticketState: "FAILED",
				eventState:  "FAILED",
			},
			{
				ticketID:     "batch-retry-rollback-exhausted",
				eventID:      "batch-retry-rollback-exhausted-event",
				ticketState:  "FAILED",
				eventState:   "FAILED",
				attemptCount: domain.BatchChildMaxAttempts,
			},
		},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-rollback-parent",
		ParentEventID:  "batch-retry-rollback-parent-event",
		Approver:       "approver-retry",
		Children: []domain.BatchApprovalRetryChild{
			{TicketID: "batch-retry-rollback-first", EventID: "batch-retry-rollback-first-event"},
			{TicketID: "batch-retry-rollback-exhausted", EventID: "batch-retry-rollback-exhausted-event"},
		},
	})
	var exhausted *BatchChildAttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() error = %v, want *BatchChildAttemptsExhaustedError", err)
	}
	if exhausted.TicketID != "batch-retry-rollback-exhausted" || exhausted.AttemptCount != domain.BatchChildMaxAttempts {
		t.Fatalf("attempt exhaustion = %+v, want exhausted second child", exhausted)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-rollback-first", "FAILED", "FAILED", 0)
	assertBatchApprovalChildState(t, store.pool, "batch-retry-rollback-exhausted", "FAILED", "FAILED", domain.BatchChildMaxAttempts)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-rollback-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.batchStatus != "FAILED" {
		t.Fatalf("rolled-back retry state = %+v, want original FAILED state", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-rollback-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_RejectsNegativeAttemptWithoutWrites(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_retry_negative_attempt")
	dropTicketAttemptCountConstraintForCorruptionTest(t, store.pool)
	const (
		parentID = "batch-retry-negative-parent"
		childID  = "batch-retry-negative-child"
	)
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     parentID,
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:     childID,
			eventID:      childID + "-event",
			ticketState:  "FAILED",
			eventState:   "FAILED",
			attemptCount: -1,
		}},
	})
	before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: parentID,
		ParentEventID:  parentID + "-event",
		Approver:       "retry-approver",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: childID,
			EventID:  childID + "-event",
		}},
	})
	var notEligible *BatchApprovalRetryNotEligibleError
	if !errors.As(err, &notEligible) || notEligible.TicketID != childID {
		t.Fatalf("RetryBatchApprovalAndEnqueue() error = %v, want negative child not eligible", err)
	}
	if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
		t.Fatalf("negative generic retry changed graph\nbefore: %s\nafter:  %s", before, after)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_RejectedChildFailsClosed(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_rejected")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-rejected-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-rejected-child",
			eventID:     "batch-retry-rejected-child-event",
			ticketState: "REJECTED",
			eventState:  "CANCELLED",
		}},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-rejected-parent",
		ParentEventID:  "batch-retry-rejected-parent-event",
		Approver:       "retry-actor",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-rejected-child",
			EventID:  "batch-retry-rejected-child-event",
		}},
	})
	var notEligible *BatchApprovalRetryNotEligibleError
	if !errors.As(err, &notEligible) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() rejected child error = %v, want *BatchApprovalRetryNotEligibleError", err)
	}
	if notEligible.TicketID != "batch-retry-rejected-child" || notEligible.EventID != "batch-retry-rejected-child-event" {
		t.Fatalf("rejected child conflict = %+v, want durable child identity", notEligible)
	}
	if !strings.Contains(err.Error(), "no longer eligible") {
		t.Fatalf("rejected child error = %q, want stable conflict explanation", err.Error())
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-rejected-child", "REJECTED", "CANCELLED", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-rejected-child", "previous failure", false)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-rejected-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.approver != "previous-approver" || state.batchStatus != "FAILED" {
		t.Fatalf("rejected child retry mutated parent state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-rejected-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_MismatchedChildEventIdentityRollsBack(t *testing.T) {
	tests := []struct {
		name   string
		mutate string
	}{
		{
			name:   "wrong aggregate type",
			mutate: `UPDATE domain_events SET aggregate_type = 'batch' WHERE id = $1`,
		},
		{
			name:   "wrong operation event type",
			mutate: `UPDATE domain_events SET event_type = 'VM_DELETION_REQUESTED' WHERE id = $1`,
		},
		{
			name:   "payload target differs from aggregate",
			mutate: `UPDATE domain_events SET payload = convert_to('{"vm_id":"another-vm"}', 'UTF8') WHERE id = $1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_child_identity")
			const (
				parentID = "batch-retry-child-identity-parent"
				childID  = "batch-retry-child-identity-child"
				eventID  = "batch-retry-child-identity-event"
			)
			seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
				parentID:     parentID,
				parentStatus: "FAILED",
				parentEvent:  "FAILED",
				batchStatus:  "FAILED",
				children: []batchApprovalDispatchChild{{
					ticketID:    childID,
					eventID:     eventID,
					ticketState: "FAILED",
					eventState:  "FAILED",
				}},
			})
			if _, err := store.pool.Exec(t.Context(), tt.mutate, eventID); err != nil {
				t.Fatalf("corrupt child event identity: %v", err)
			}

			err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
				ParentTicketID: parentID,
				ParentEventID:  parentID + "-event",
				Approver:       "retry-approver",
				Children: []domain.BatchApprovalRetryChild{{
					TicketID: childID,
					EventID:  eventID,
				}},
			})
			var notEligible *BatchApprovalRetryNotEligibleError
			if !errors.As(err, &notEligible) {
				t.Fatalf("RetryBatchApprovalAndEnqueue() identity error = %v, want *BatchApprovalRetryNotEligibleError", err)
			}
			assertBatchApprovalChildState(t, store.pool, childID, "FAILED", "FAILED", 0)
			state := loadBatchApprovalDispatchState(t, store.pool, parentID)
			if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.batchStatus != "FAILED" {
				t.Fatalf("mismatched retry state = %+v, want untouched FAILED state", state)
			}
			assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
		})
	}
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_ExactIdentityMismatchRollsBackGraph(t *testing.T) {
	tests := []struct {
		name               string
		mutation           string
		mutationTarget     string
		wantParentConflict bool
		wantConflictTicket string
	}{
		{
			name:               "parent event actor",
			mutation:           `UPDATE domain_events SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name:               "projection actor",
			mutation:           `UPDATE batch_tickets SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection type",
			mutation:           `UPDATE batch_tickets SET batch_type = 'BATCH_DELETE' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection terminal status",
			mutation:           `UPDATE batch_tickets SET status = 'COMPLETED' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection counters",
			mutation:           `UPDATE batch_tickets SET failed_count = 0, pending_count = 2 WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name: "parent payload submitter",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{submitted_by}', to_jsonb('foreign-actor'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name: "parent payload operation",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{operation}', to_jsonb('DELETE'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name: "parent payload item target",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{items,0,vm_id}', to_jsonb('foreign-vm'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name:               "selected child requester",
			mutation:           `UPDATE tickets SET requester = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "selected",
			wantConflictTicket: "batch-retry-exact-selected",
		},
		{
			name:               "selected child event actor",
			mutation:           `UPDATE domain_events SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "selected_event",
			wantConflictTicket: "batch-retry-exact-selected",
		},
		{
			name: "selected child payload actor",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{actor}', to_jsonb('foreign-actor'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "selected_event",
			wantConflictTicket: "batch-retry-exact-selected",
		},
		{
			name:               "selected child operation",
			mutation:           `UPDATE tickets SET operation_type = 'DELETE' WHERE id = $1`,
			mutationTarget:     "selected",
			wantConflictTicket: "batch-retry-exact-selected",
		},
		{
			name:               "unselected sibling actor",
			mutation:           `UPDATE tickets SET requester = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "sibling",
			wantConflictTicket: "batch-retry-exact-sibling",
		},
		{
			name: "unselected pending sibling",
			mutation: `WITH updated_ticket AS (
  UPDATE tickets SET status = 'PENDING' WHERE id = $1 RETURNING id
), updated_event AS (
  UPDATE domain_events SET status = 'PENDING'
  WHERE id = 'batch-retry-exact-sibling-event'
  RETURNING id
), updated_projection AS (
  UPDATE batch_tickets
  SET success_count = 0, failed_count = 1, pending_count = 1
  WHERE id = 'batch-retry-exact-parent'
  RETURNING id
)
SELECT 1`,
			mutationTarget:     "sibling",
			wantConflictTicket: "batch-retry-exact-sibling",
		},
		{
			name:               "missing sibling event",
			mutation:           `DELETE FROM domain_events WHERE id = $1`,
			mutationTarget:     "sibling_event",
			wantConflictTicket: "batch-retry-exact-sibling",
		},
		{
			name: "injected sibling",
			mutation: `WITH inserted_event AS (
  INSERT INTO domain_events (
    id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
  ) VALUES (
    'batch-retry-exact-injected-event', NOW(), 'VM_MODIFY_REQUESTED', 'vm',
    'batch-retry-exact-injected',
    convert_to('{"vm_id":"batch-retry-exact-injected","actor":"batch-requester"}', 'UTF8'),
    'COMPLETED', 'batch-requester'
  )
  RETURNING id
)
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, parent_ticket_id, attempt_count
)
SELECT
  'batch-retry-exact-injected', NOW(), NOW(), id, 'MODIFY', 'SUCCESS',
  'batch-requester', 'injected sibling', $1, 1
FROM inserted_event`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_exact_identity")
			const (
				parentID       = "batch-retry-exact-parent"
				selectedID     = "batch-retry-exact-selected"
				selectedEvent  = "batch-retry-exact-selected-event"
				siblingID      = "batch-retry-exact-sibling"
				siblingEventID = "batch-retry-exact-sibling-event"
			)
			seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
				parentID:     parentID,
				parentStatus: "FAILED",
				parentEvent:  "FAILED",
				batchStatus:  "FAILED",
				children: []batchApprovalDispatchChild{
					{
						ticketID:    selectedID,
						eventID:     selectedEvent,
						ticketState: "FAILED",
						eventState:  "FAILED",
					},
					{
						ticketID:     siblingID,
						eventID:      siblingEventID,
						ticketState:  "SUCCESS",
						eventState:   "COMPLETED",
						attemptCount: 1,
					},
				},
			})
			mutationID := parentID
			switch tt.mutationTarget {
			case "parent_event":
				mutationID = parentID + "-event"
			case "selected":
				mutationID = selectedID
			case "selected_event":
				mutationID = selectedEvent
			case "sibling":
				mutationID = siblingID
			case "sibling_event":
				mutationID = siblingEventID
			}
			if _, err := store.pool.Exec(t.Context(), tt.mutation, mutationID); err != nil {
				t.Fatalf("mutate retry graph identity: %v", err)
			}
			before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)

			err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
				ParentTicketID: parentID,
				ParentEventID:  parentID + "-event",
				Approver:       "retry-approver",
				Children: []domain.BatchApprovalRetryChild{{
					TicketID: selectedID,
					EventID:  selectedEvent,
				}},
			})
			if tt.wantParentConflict {
				var conflict *BatchRetryParentNotEligibleError
				if !errors.As(err, &conflict) {
					t.Fatalf("RetryBatchApprovalAndEnqueue() error = %v, want parent conflict", err)
				}
			} else {
				var conflict *BatchApprovalRetryNotEligibleError
				if !errors.As(err, &conflict) {
					t.Fatalf("RetryBatchApprovalAndEnqueue() error = %v, want child conflict", err)
				}
				if conflict.TicketID != tt.wantConflictTicket {
					t.Fatalf("retry child conflict = %+v, want ticket %q", conflict, tt.wantConflictTicket)
				}
			}
			if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
				t.Fatalf("identity mismatch changed durable graph:\nbefore=%s\nafter=%s", before, after)
			}
			assertBatchApprovalDispatcherJobs(t, store.riverClient, parentID, 0)
		})
	}
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_PendingParentEventRollsBackChildAndJob(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_pending_parent")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-pending-event-parent",
		parentStatus: "FAILED",
		parentEvent:  "PENDING",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-pending-event-child",
			eventID:     "batch-retry-pending-event-child-event",
			ticketState: "FAILED",
			eventState:  "FAILED",
		}},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-pending-event-parent",
		ParentEventID:  "batch-retry-pending-event-parent-event",
		Approver:       "retry-actor",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-pending-event-child",
			EventID:  "batch-retry-pending-event-child-event",
		}},
	})
	var parentNotEligible *BatchRetryParentNotEligibleError
	if !errors.As(err, &parentNotEligible) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() pending parent event error = %v, want *BatchRetryParentNotEligibleError", err)
	}
	if parentNotEligible.ParentTicketID != "batch-retry-pending-event-parent" ||
		parentNotEligible.ParentEventID != "batch-retry-pending-event-parent-event" {
		t.Fatalf("pending parent event conflict = %+v, want parent/event identity", parentNotEligible)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-pending-event-child", "FAILED", "FAILED", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-pending-event-child", "previous failure", false)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-pending-event-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "PENDING" || state.approver != "previous-approver" || state.batchStatus != "FAILED" {
		t.Fatalf("pending parent event retry did not roll back durable state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-pending-event-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_TerminalChildEventRollsBackTicketAndJob(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_terminal_child_event")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-terminal-child-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-terminal-child",
			eventID:     "batch-retry-terminal-child-event",
			ticketState: "FAILED",
			eventState:  "COMPLETED",
		}},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-terminal-child-parent",
		ParentEventID:  "batch-retry-terminal-child-parent-event",
		Approver:       "retry-actor",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-terminal-child",
			EventID:  "batch-retry-terminal-child-event",
		}},
	})
	var notEligible *BatchApprovalRetryNotEligibleError
	if !errors.As(err, &notEligible) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() terminal child event error = %v, want *BatchApprovalRetryNotEligibleError", err)
	}
	if notEligible.TicketID != "batch-retry-terminal-child" || notEligible.EventID != "batch-retry-terminal-child-event" {
		t.Fatalf("terminal child event conflict = %+v, want durable child identity", notEligible)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-terminal-child", "FAILED", "COMPLETED", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-terminal-child", "previous failure", false)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-terminal-child-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.batchStatus != "FAILED" {
		t.Fatalf("terminal child event retry mutated parent state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-terminal-child-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_TerminalParentRollsBackChildAndJob(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_terminal_parent")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-terminal-parent",
		parentStatus: "SUCCESS",
		parentEvent:  "COMPLETED",
		batchStatus:  "COMPLETED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-terminal-parent-child",
			eventID:     "batch-retry-terminal-parent-child-event",
			ticketState: "FAILED",
			eventState:  "FAILED",
		}},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-terminal-parent",
		ParentEventID:  "batch-retry-terminal-parent-event",
		Approver:       "retry-actor",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-terminal-parent-child",
			EventID:  "batch-retry-terminal-parent-child-event",
		}},
	})
	var parentNotEligible *BatchRetryParentNotEligibleError
	if !errors.As(err, &parentNotEligible) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() terminal parent error = %v, want *BatchRetryParentNotEligibleError", err)
	}
	if parentNotEligible.ParentTicketID != "batch-retry-terminal-parent" ||
		parentNotEligible.ParentEventID != "batch-retry-terminal-parent-event" ||
		!strings.Contains(parentNotEligible.Error(), "no longer in an approved execution state") {
		t.Fatalf("terminal parent conflict = %+v, want durable parent/event identity", parentNotEligible)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-terminal-parent-child", "FAILED", "FAILED", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-terminal-parent-child", "previous failure", false)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-terminal-parent")
	if state.parentStatus != "SUCCESS" || state.parentEvent != "COMPLETED" || state.approver != "previous-approver" ||
		state.batchStatus != "COMPLETED" {
		t.Fatalf("terminal parent retry mutated durable state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-terminal-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_MissingChildFailsClosed(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_missing_child")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-missing-child-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-existing-child",
			eventID:     "batch-retry-existing-child-event",
			ticketState: "FAILED",
			eventState:  "FAILED",
		}},
	})

	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-missing-child-parent",
		ParentEventID:  "batch-retry-missing-child-parent-event",
		Approver:       "retry-actor",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-vanished-child",
			EventID:  "batch-retry-vanished-child-event",
		}},
	})
	var notEligible *BatchApprovalRetryNotEligibleError
	if !errors.As(err, &notEligible) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() missing child error = %v, want *BatchApprovalRetryNotEligibleError", err)
	}
	if notEligible.TicketID != "batch-retry-vanished-child" || notEligible.EventID != "batch-retry-vanished-child-event" {
		t.Fatalf("missing child conflict = %+v, want requested child identity", notEligible)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-existing-child", "FAILED", "FAILED", 0)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-existing-child", "previous failure", false)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-missing-child-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.batchStatus != "FAILED" {
		t.Fatalf("missing child retry mutated parent state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-missing-child-parent", 0)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_ActiveDispatcherPreservesFailedChild(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_conflict")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-conflict-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-conflict-child",
			eventID:     "batch-retry-conflict-child-event",
			ticketState: "FAILED",
			eventState:  "FAILED",
		}},
	})
	existing, err := store.riverClient.Insert(t.Context(), jobs.BatchApprovalDispatchArgs{
		BatchID: "batch-retry-conflict-parent",
	}, nil)
	if err != nil || existing == nil || existing.Job == nil || existing.UniqueSkippedAsDuplicate {
		t.Fatalf("seed active dispatcher = (%+v, %v), want inserted job", existing, err)
	}

	err = store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: " batch-retry-conflict-parent ",
		ParentEventID:  "batch-retry-conflict-parent-event",
		Approver:       "different-approver",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-conflict-child",
			EventID:  "batch-retry-conflict-child-event",
		}},
		Execution: domain.BatchApprovalExecutionOptions{
			ClusterID:    "different-cluster",
			StorageClass: "different-storage",
			DiskGB:       777,
		},
	})
	var conflict *BatchApprovalDispatchConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("RetryBatchApprovalAndEnqueue() error = %v, want *BatchApprovalDispatchConflictError", err)
	}
	if conflict.ParentTicketID != "batch-retry-conflict-parent" || conflict.ExistingJobID != existing.Job.ID {
		t.Fatalf("dispatcher conflict = %+v, want existing parent-scoped job %d", conflict, existing.Job.ID)
	}
	if conflict.ExistingJobState != string(existing.Job.State) {
		t.Fatalf("dispatcher conflict state = %q, want %q", conflict.ExistingJobState, existing.Job.State)
	}
	if !strings.Contains(conflict.Error(), "batch-retry-conflict-parent") || !strings.Contains(conflict.Error(), string(existing.Job.State)) {
		t.Fatalf("dispatcher conflict error = %q, want parent and state", conflict.Error())
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-conflict-child", "FAILED", "FAILED", 0)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-conflict-parent")
	if state.parentStatus != "FAILED" || state.parentEvent != "FAILED" || state.approver != "previous-approver" ||
		state.clusterID != "" || state.storageClass != "" || state.modifiedSpec != "null" || state.batchStatus != "FAILED" {
		t.Fatalf("active-dispatch conflict mutated durable state: %+v", state)
	}
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-conflict-parent", 1)
}

func TestApprovalAtomicWriterRetryBatchApprovalAndEnqueue_AcceptsAlreadyPendingChildEvent(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_approval_retry_pending_event")
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:     "batch-retry-pending-parent",
		parentStatus: "FAILED",
		parentEvent:  "FAILED",
		batchStatus:  "FAILED",
		children: []batchApprovalDispatchChild{{
			ticketID:    "batch-retry-pending-child",
			eventID:     "batch-retry-pending-child-event",
			ticketState: "FAILED",
			eventState:  "PENDING",
		}},
	})
	execution := domain.BatchApprovalExecutionOptions{
		ClusterID:       "cluster-retry",
		StorageClass:    "storage-retry",
		DVAccessModes:   []string{"ReadWriteOnce"},
		MemoryRequestGi: 6,
		DiskGB:          88,
	}
	err := store.writer.RetryBatchApprovalAndEnqueue(t.Context(), domain.BatchApprovalRetryInput{
		ParentTicketID: "batch-retry-pending-parent",
		ParentEventID:  "batch-retry-pending-parent-event",
		Approver:       "approver-pending",
		Children: []domain.BatchApprovalRetryChild{{
			TicketID: "batch-retry-pending-child",
			EventID:  "batch-retry-pending-child-event",
		}},
		Execution: execution,
	})
	if err != nil {
		t.Fatalf("RetryBatchApprovalAndEnqueue() with PENDING child event error = %v", err)
	}

	assertBatchApprovalChildState(t, store.pool, "batch-retry-pending-child", "PENDING", "PENDING", 1)
	state := loadBatchApprovalDispatchState(t, store.pool, "batch-retry-pending-parent")
	if state.parentStatus != "EXECUTING" || state.parentEvent != "PROCESSING" || state.batchStatus != "IN_PROGRESS" {
		t.Fatalf("successful retry state = %+v, want executing parent and in-progress batch", state)
	}
	if state.childCount != 1 || state.pendingCount != 1 || state.failedCount != 0 {
		t.Fatalf("successful retry projection = %+v, want one pending child and no failures", state)
	}
	if state.approver != "approver-pending" || state.clusterID != "cluster-retry" || state.storageClass != "storage-retry" {
		t.Fatalf("retry approval snapshot fields = %+v, want new execution inputs", state)
	}
	assertBatchApprovalExecutionSnapshot(t, state.modifiedSpec, execution)
	assertBatchApprovalChildRetryMetadata(t, store.pool, "batch-retry-pending-child", "", true)
	assertBatchApprovalDispatcherJobs(t, store.riverClient, "batch-retry-pending-parent", 1)
}

type batchApprovalDispatchChild struct {
	ticketID     string
	eventID      string
	ticketState  string
	eventState   string
	attemptCount int
}

type batchApprovalDispatchFixture struct {
	parentID        string
	parentStatus    string
	parentEvent     string
	parentEventType string
	operationType   string
	batchType       string
	childEventType  string
	batchStatus     string
	children        []batchApprovalDispatchChild
}

func seedBatchApprovalDispatchRows(t *testing.T, pool *pgxpool.Pool, fixture batchApprovalDispatchFixture) {
	t.Helper()
	if fixture.parentEventType == "" {
		fixture.parentEventType = "BATCH_MODIFY_REQUESTED"
	}
	if fixture.operationType == "" {
		fixture.operationType = "MODIFY"
	}
	if fixture.batchType == "" {
		fixture.batchType = "BATCH_MODIFY"
	}
	if fixture.childEventType == "" {
		fixture.childEventType = "VM_MODIFY_REQUESTED"
	}
	childPayloads := make(map[string][]byte, len(fixture.children))
	parentItems := make([]domain.BatchVMItemPayload, 0, len(fixture.children))
	for _, child := range fixture.children {
		var (
			payload any
			item    domain.BatchVMItemPayload
		)
		switch fixture.operationType {
		case "CREATE":
			payload = domain.VMCreationPayload{
				RequesterID: "batch-requester",
				ServiceID:   child.ticketID,
			}
			item = domain.BatchVMItemPayload{ServiceID: child.ticketID, OwnerID: "batch-requester"}
		case "DELETE":
			payload = domain.VMDeletePayload{VMID: child.ticketID, Actor: "batch-requester"}
			item = domain.BatchVMItemPayload{VMID: child.ticketID}
		case "POWER":
			payload = domain.VMPowerPayload{
				VMID: child.ticketID, Operation: "start", Actor: "batch-requester",
				DispatchMode: domain.VMPowerDispatchTicket,
			}
			item = domain.BatchVMItemPayload{VMID: child.ticketID, Operation: "start"}
		default:
			payload = domain.VMModifyPayload{VMID: child.ticketID, Actor: "batch-requester"}
			item = domain.BatchVMItemPayload{VMID: child.ticketID}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal batch child payload %s: %v", child.eventID, err)
		}
		childPayloads[child.eventID] = encoded
		parentItems = append(parentItems, item)
	}
	parentOperation := fixture.operationType
	if fixture.operationType == "POWER" {
		parentOperation = "POWER_START"
	}
	parentPayload, err := json.Marshal(domain.BatchVMRequestPayload{
		Operation:   parentOperation,
		SubmittedBy: "batch-requester",
		Items:       parentItems,
	})
	if err != nil {
		t.Fatalf("marshal batch parent payload: %v", err)
	}
	parentEventID := fixture.parentID + "-event"
	if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), $2, 'batch', $3, $4, $5, 'batch-requester')
`, parentEventID, fixture.parentEventType, fixture.parentID, parentPayload, fixture.parentEvent); err != nil {
		t.Fatalf("seed batch parent event: %v", err)
	}
	var parentApprover any
	if fixture.parentStatus != "PENDING" {
		parentApprover = "previous-approver"
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, approver, reason)
VALUES ($1, NOW(), NOW(), $2, $3, $4, 'batch-requester', $5, 'batch approval')
`, fixture.parentID, parentEventID, fixture.operationType, fixture.parentStatus, parentApprover); err != nil {
		t.Fatalf("seed batch parent ticket: %v", err)
	}
	successCount := 0
	failedCount := 0
	pendingCount := 0
	for _, child := range fixture.children {
		switch child.ticketState {
		case "SUCCESS":
			successCount++
		case "FAILED", "REJECTED":
			failedCount++
		case "CANCELLED":
		default:
			pendingCount++
		}
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by, reason
)
VALUES ($1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, 'batch-requester', 'batch approval')
`, fixture.parentID, fixture.batchType, len(fixture.children), successCount, failedCount, pendingCount, fixture.batchStatus); err != nil {
		t.Fatalf("seed batch projection: %v", err)
	}
	for _, child := range fixture.children {
		if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), $2, 'vm', $3, $4, $5, 'batch-requester')
`, child.eventID, fixture.childEventType, child.ticketID, childPayloads[child.eventID], child.eventState); err != nil {
			t.Fatalf("seed batch child event %s: %v", child.eventID, err)
		}
		var rejectReason any
		if child.ticketState == "FAILED" || child.ticketState == "REJECTED" {
			rejectReason = "previous failure"
		}
		if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id, attempt_count
)
VALUES ($1, NOW(), NOW(), $2, $3, $4, 'batch-requester', 'batch child', $5, $6, $7)
`, child.ticketID, child.eventID, fixture.operationType, child.ticketState, rejectReason, fixture.parentID, child.attemptCount); err != nil {
			t.Fatalf("seed batch child ticket %s: %v", child.ticketID, err)
		}
	}
}

type batchApprovalDispatchState struct {
	parentStatus string
	parentEvent  string
	approver     string
	clusterID    string
	storageClass string
	modifiedSpec string
	batchStatus  string
	childCount   int
	pendingCount int
	failedCount  int
}

func loadBatchApprovalDispatchState(t *testing.T, pool *pgxpool.Pool, parentID string) batchApprovalDispatchState {
	t.Helper()
	var state batchApprovalDispatchState
	if err := pool.QueryRow(t.Context(), `
SELECT
  ticket.status,
  event.status,
  COALESCE(ticket.approver, ''),
  COALESCE(ticket.selected_cluster_id, ''),
  COALESCE(ticket.selected_storage_class, ''),
  COALESCE(ticket.modified_spec, 'null'::jsonb)::text,
  batch.status,
  batch.child_count,
  batch.pending_count,
  batch.failed_count
FROM tickets AS ticket
JOIN domain_events AS event ON event.id = ticket.event_id
JOIN batch_tickets AS batch ON batch.id = ticket.id
WHERE ticket.id = $1
`, parentID).Scan(
		&state.parentStatus,
		&state.parentEvent,
		&state.approver,
		&state.clusterID,
		&state.storageClass,
		&state.modifiedSpec,
		&state.batchStatus,
		&state.childCount,
		&state.pendingCount,
		&state.failedCount,
	); err != nil {
		t.Fatalf("load batch approval dispatch state: %v", err)
	}
	return state
}

func loadBatchApprovalGraphSnapshot(t *testing.T, pool *pgxpool.Pool, parentID string) string {
	t.Helper()
	var snapshot string
	if err := pool.QueryRow(t.Context(), `
SELECT jsonb_build_object(
  'parent', to_jsonb(parent),
  'parent_event', to_jsonb(parent_event),
  'projection', to_jsonb(batch),
  'children', (
    SELECT jsonb_agg(
      jsonb_build_object('ticket', to_jsonb(child), 'event', to_jsonb(child_event))
      ORDER BY child.id
    )
    FROM tickets AS child
    JOIN domain_events AS child_event ON child_event.id = child.event_id
    WHERE child.parent_ticket_id = parent.id
  )
)::text
FROM tickets AS parent
JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
JOIN batch_tickets AS batch ON batch.id = parent.id
WHERE parent.id = $1
`, parentID).Scan(&snapshot); err != nil {
		t.Fatalf("load batch approval graph snapshot: %v", err)
	}
	return snapshot
}

func assertBatchApprovalExecutionSnapshot(
	t *testing.T,
	modifiedSpec string,
	want domain.BatchApprovalExecutionOptions,
) {
	t.Helper()
	var envelope struct {
		Execution domain.BatchApprovalExecutionOptions `json:"batch_approval_execution"`
	}
	if err := json.Unmarshal([]byte(modifiedSpec), &envelope); err != nil {
		t.Fatalf("decode durable batch approval execution snapshot %q: %v", modifiedSpec, err)
	}
	if got, wantJSON := mustMarshalBatchExecution(t, envelope.Execution), mustMarshalBatchExecution(t, want); got != wantJSON {
		t.Fatalf("durable execution snapshot = %s, want %s", got, wantJSON)
	}
}

func mustMarshalBatchExecution(t *testing.T, execution domain.BatchApprovalExecutionOptions) string {
	t.Helper()
	raw, err := json.Marshal(execution)
	if err != nil {
		t.Fatalf("marshal batch approval execution: %v", err)
	}
	return string(raw)
}

func assertBatchApprovalChildState(
	t *testing.T,
	pool *pgxpool.Pool,
	ticketID, wantTicketState, wantEventState string,
	wantAttempts int,
) {
	t.Helper()
	var ticketState, eventState string
	var attempts int
	if err := pool.QueryRow(t.Context(), `
SELECT ticket.status, event.status, ticket.attempt_count
FROM tickets AS ticket
JOIN domain_events AS event ON event.id = ticket.event_id
WHERE ticket.id = $1
`, ticketID).Scan(&ticketState, &eventState, &attempts); err != nil {
		t.Fatalf("load child state %s: %v", ticketID, err)
	}
	if ticketState != wantTicketState || eventState != wantEventState || attempts != wantAttempts {
		t.Fatalf("child %s state = (%q, %q, %d), want (%q, %q, %d)", ticketID, ticketState, eventState, attempts, wantTicketState, wantEventState, wantAttempts)
	}
}

func assertBatchApprovalChildRetryMetadata(
	t *testing.T,
	pool *pgxpool.Pool,
	ticketID, wantRejectReason string,
	wantLastAttempt bool,
) {
	t.Helper()
	var rejectReason string
	var hasLastAttempt bool
	if err := pool.QueryRow(t.Context(), `
SELECT COALESCE(reject_reason, ''), last_attempt_at IS NOT NULL
FROM tickets
WHERE id = $1
`, ticketID).Scan(&rejectReason, &hasLastAttempt); err != nil {
		t.Fatalf("load child retry metadata %s: %v", ticketID, err)
	}
	if rejectReason != wantRejectReason || hasLastAttempt != wantLastAttempt {
		t.Fatalf("child %s retry metadata = (%q, last_attempt=%t), want (%q, %t)", ticketID, rejectReason, hasLastAttempt, wantRejectReason, wantLastAttempt)
	}
}

func assertBatchApprovalDispatcherJobs(
	t *testing.T,
	client *river.Client[pgx.Tx],
	parentID string,
	want int,
) {
	t.Helper()
	result, err := client.JobList(t.Context(), river.NewJobListParams().Kinds(jobs.BatchApprovalDispatchArgs{}.Kind()))
	if err != nil {
		t.Fatalf("list batch approval dispatcher jobs: %v", err)
	}
	count := 0
	for _, row := range result.Jobs {
		var args jobs.BatchApprovalDispatchArgs
		if err := json.Unmarshal(row.EncodedArgs, &args); err != nil {
			t.Fatalf("decode batch approval dispatcher args for job %d: %v", row.ID, err)
		}
		if args.BatchID == parentID {
			count++
		}
	}
	if count != want {
		t.Fatalf("batch approval dispatcher count for %s = %d, want %d", parentID, count, want)
	}
}

func assertRiverJobKindCount(t *testing.T, pool *pgxpool.Pool, kind string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = $1`, kind).Scan(&got); err != nil {
		t.Fatalf("count River jobs of kind %s: %v", kind, err)
	}
	if got != want {
		t.Fatalf("River jobs of kind %s = %d, want %d", kind, got, want)
	}
}

func dropTicketAttemptCountConstraintForCorruptionTest(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(
		t.Context(),
		`ALTER TABLE tickets DROP CONSTRAINT tickets_attempt_count_nonnegative`,
	); err != nil {
		t.Fatalf("drop attempt-count CHECK for corruption test: %v", err)
	}
}
