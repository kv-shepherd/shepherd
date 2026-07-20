package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel/attribute"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/observability"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
	sqlcrepo "kv-shepherd.io/shepherd/internal/repository/sqlc"
)

const (
	// BatchSubmissionAdvisoryLockKey serializes quota/cooldown checks and the
	// durable parent insert across every generic and power batch writer. The
	// lock is transaction-scoped and must be acquired by the business transaction.
	BatchSubmissionAdvisoryLockKey    = "kv-shepherd:batch-submission:v1"
	batchPowerEventType               = "BATCH_POWER_REQUESTED"
	batchPowerType                    = "BATCH_POWER"
	batchPowerTicketOperation         = "POWER"
	batchPowerAggregateType           = "batch"
	batchPowerChildAggregateType      = "vm"
	batchPowerEventStatusPending      = "PENDING"
	batchPowerEventStatusProcessing   = "PROCESSING"
	batchPowerTicketStatusExecuting   = "EXECUTING"
	batchPowerTicketStatusFailed      = "FAILED"
	batchPowerBatchStatusInProgress   = "IN_PROGRESS"
	batchPowerBatchStatusPending      = "PENDING_APPROVAL"
	batchOperationCreate              = "CREATE"
	batchOperationModify              = "MODIFY"
	batchOperationDelete              = "DELETE"
	batchProjectionTypeCreate         = "BATCH_CREATE"
	batchProjectionTypeModify         = "BATCH_MODIFY"
	batchProjectionTypeDelete         = "BATCH_DELETE"
	batchPowerOperationStart          = "POWER_START"
	batchPowerOperationStop           = "POWER_STOP"
	batchPowerOperationRestart        = "POWER_RESTART"
	powerOperationStart               = "start"
	powerOperationStop                = "stop"
	powerOperationRestart             = "restart"
	maxBatchPowerChildCountForSQLCInt = 1<<31 - 1
)

// BatchPowerSubmissionInput describes the durable parent/child rows for a
// batch power request. No River job is visible until this transaction commits.
type BatchPowerSubmissionInput struct {
	ParentID         string
	Actor            string
	Operation        string
	RequestID        string
	Reason           string
	ParentPayload    []byte
	RequiresApproval bool
	Children         []BatchPowerChildInput
}

// BatchPowerSubmissionTxValidator runs inside the writer's transaction so
// submission policy can share the same atomic boundary as durable rows and
// River jobs.
type BatchPowerSubmissionTxValidator func(context.Context, pgx.Tx) error

// BatchPowerSubmissionTxPolicy supplies the handler-owned phases that must run
// inside the power writer's transaction. LockActor runs immediately after the
// global submission lock, preserving the canonical global -> actor -> request
// -> VM lock order. Validate runs only after idempotency replay is ruled out,
// so a replay never consumes rate limit or depends on mutable submission policy.
type BatchPowerSubmissionTxPolicy struct {
	LockActor BatchPowerSubmissionTxValidator
	Validate  BatchPowerSubmissionTxValidator
}

// BatchSubmissionReplayError reports that the same actor, operation, and
// request ID already committed a batch. Callers should return the existing
// batch representation instead of treating the replay as a new submission.
type BatchSubmissionReplayError struct {
	BatchID string
}

func (e *BatchSubmissionReplayError) Error() string {
	return fmt.Sprintf("batch submission already exists as %s", e.BatchID)
}

// BatchIdempotencyLockKey returns the shared PostgreSQL advisory-lock key used
// by every batch writer. Length prefixes keep otherwise ambiguous user input
// from producing the same logical key before PostgreSQL hashes it.
func BatchIdempotencyLockKey(actor, operation, requestID string) string {
	actor = strings.TrimSpace(actor)
	operation = batchIdempotencyOperation(operation)
	requestID = batchreplay.Normalize(requestID)
	return fmt.Sprintf(
		"batch:request:%d:%s:%d:%s:%d:%s",
		len(actor), actor,
		len(operation), operation,
		len(requestID), requestID,
	)
}

func batchIdempotencyOperation(operation string) string {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case batchOperationCreate, batchProjectionTypeCreate:
		return batchOperationCreate
	case batchOperationModify, batchProjectionTypeModify:
		return batchOperationModify
	case batchOperationDelete, batchProjectionTypeDelete:
		return batchOperationDelete
	case "START", batchPowerOperationStart, powerEventTypeStart:
		return batchPowerOperationStart
	case "STOP", batchPowerOperationStop, powerEventTypeStop:
		return batchPowerOperationStop
	case "RESTART", batchPowerOperationRestart, powerEventTypeRestart:
		return batchPowerOperationRestart
	default:
		return strings.ToUpper(strings.TrimSpace(operation))
	}
}

func batchIdempotencyType(operation string) string {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case batchOperationCreate, batchProjectionTypeCreate:
		return batchProjectionTypeCreate
	case batchOperationModify, batchProjectionTypeModify:
		return batchProjectionTypeModify
	case batchOperationDelete, batchProjectionTypeDelete:
		return batchProjectionTypeDelete
	default:
		// This package owns the batch-power writer; POWER_START/STOP/RESTART
		// and its internal test/event aliases share the BATCH_POWER projection.
		// Their idempotency locks and replay checks remain action-specific.
		return batchPowerType
	}
}

func isBatchPowerOperation(operation string) bool {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case "START", "STOP", "RESTART",
		batchPowerOperationStart, batchPowerOperationStop, batchPowerOperationRestart,
		powerEventTypeStart, powerEventTypeStop, powerEventTypeRestart:
		return true
	default:
		return false
	}
}

// BatchPowerChildInput is a single child VM operation in a batch power request.
type BatchPowerChildInput struct {
	EventType   string
	AggregateID string
	Payload     []byte
	Reason      string
}

// BatchPowerRetryInput describes failed power children that should be reset
// and re-enqueued atomically.
type BatchPowerRetryInput struct {
	ParentID string
	Children []BatchPowerRetryChildInput
}

// BatchPowerRetryChildInput identifies a persisted child ticket/event pair.
type BatchPowerRetryChildInput struct {
	TicketID string
	EventID  string
}

// PowerRetryJobConflictError reports that River already has an equivalent
// runnable power job. The retry transaction must roll back instead of making
// the ticket/event active without inserting fresh work.
type PowerRetryJobConflictError struct {
	EventID          string
	ExistingJobID    int64
	ExistingJobState string
}

func (e *PowerRetryJobConflictError) Error() string {
	return fmt.Sprintf(
		"power retry event %s already has River job %d in state %s",
		e.EventID,
		e.ExistingJobID,
		e.ExistingJobState,
	)
}

// PowerRetryNotEligibleError reports that a retry request observed stale child
// state after acquiring the per-VM lock. This is an expected conflict (for
// example, a concurrent retry already completed), not an infrastructure error.
type PowerRetryNotEligibleError struct {
	TicketID string
	EventID  string
}

func (e *PowerRetryNotEligibleError) Error() string {
	return fmt.Sprintf("power retry ticket %s for event %s is no longer eligible", e.TicketID, e.EventID)
}

// BatchChildAttemptsExhaustedError reports that the persisted logical dispatch
// count reached the ADR-0015 per-child retry cap. Callers should surface this
// as an expected conflict and must not mutate the child or enqueue fresh work.
type BatchChildAttemptsExhaustedError struct {
	TicketID     string
	AttemptCount int
	MaxAttempts  int
}

func (e *BatchChildAttemptsExhaustedError) Error() string {
	return fmt.Sprintf(
		"batch child ticket %s exhausted %d/%d logical dispatch attempts",
		e.TicketID,
		e.AttemptCount,
		e.MaxAttempts,
	)
}

// CreateBatchPowerAndMaybeEnqueue atomically persists the batch parent, child
// tickets/events, and direct-execution River jobs. River InsertTx keeps job
// visibility tied to the same commit as the application rows. The optional
// policy phases run inside that transaction in canonical lock order.
func (w *ApprovalAtomicWriter) CreateBatchPowerAndMaybeEnqueue(
	ctx context.Context,
	input BatchPowerSubmissionInput,
	policy *BatchPowerSubmissionTxPolicy,
) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.batch_power.submit",
		attribute.String("shepherd.business.operation", "batch_power.submit"),
		attribute.String("shepherd.batch.type", batchPowerType),
		attribute.Bool("shepherd.approval.required", input.RequiresApproval),
		attribute.Int("shepherd.batch.child_count", len(input.Children)),
	)
	defer func() {
		var active *ActivePowerEventError
		var replay *BatchSubmissionReplayError
		if !errors.As(err, &active) && !errors.As(err, &replay) {
			observability.RecordSpanError(span, err)
		}
		span.End()
	}()

	if w.pool == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if !input.RequiresApproval && w.riverClient == nil {
		return fmt.Errorf("approval atomic writer river client is not initialized")
	}
	if validationErr := validateBatchPowerSubmissionInput(input); validationErr != nil {
		return validationErr
	}

	// READ COMMITTED takes a fresh snapshot after a waiter acquires the global
	// advisory lock. An operator-level REPEATABLE READ default could otherwise
	// retain a pre-wait snapshot and miss the preceding submission's commit.
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch power tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, lockErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		BatchSubmissionAdvisoryLockKey,
	); lockErr != nil {
		return fmt.Errorf("lock batch submissions: %w", lockErr)
	}
	if policy != nil && policy.LockActor != nil {
		if lockErr := policy.LockActor(ctx, tx); lockErr != nil {
			return lockErr
		}
	}

	if batchreplay.Normalize(input.RequestID) != "" {
		if lockErr := lockBatchSubmission(ctx, tx, input.Actor, input.Operation, input.RequestID); lockErr != nil {
			return lockErr
		}
		existingBatchID, findErr := findExistingBatchSubmission(
			ctx,
			tx,
			input.Actor,
			input.Operation,
			input.RequestID,
		)
		if findErr != nil {
			return findErr
		}
		if existingBatchID != "" {
			return &BatchSubmissionReplayError{BatchID: existingBatchID}
		}
	}
	if policy != nil && policy.Validate != nil {
		if validationErr := policy.Validate(ctx, tx); validationErr != nil {
			return validationErr
		}
	}

	vmIDs := make([]string, 0, len(input.Children))
	for _, child := range input.Children {
		vmIDs = append(vmIDs, strings.TrimSpace(child.AggregateID))
	}
	sort.Strings(vmIDs)
	for _, vmID := range vmIDs {
		if lockErr := lockPowerVM(ctx, tx, vmID); lockErr != nil {
			return lockErr
		}
	}
	for _, vmID := range vmIDs {
		active, findErr := findActivePowerEvent(ctx, tx, vmID, "")
		if findErr != nil {
			return findErr
		}
		if active != nil {
			return active
		}
	}

	qtx := w.queries.WithTx(tx)

	parentEventID, err := newV7ID()
	if err != nil {
		return fmt.Errorf("generate parent event id: %w", err)
	}
	childCount, err := sqlcInt32Count(len(input.Children), "batch power child count")
	if err != nil {
		return err
	}

	parentEventStatus := batchPowerEventStatusProcessing
	parentTicketStatus := batchPowerTicketStatusExecuting
	batchStatus := batchPowerBatchStatusInProgress
	if input.RequiresApproval {
		parentEventStatus = batchPowerEventStatusPending
		parentTicketStatus = batchPowerEventStatusPending
		batchStatus = batchPowerBatchStatusPending
	}

	if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
		ID:            parentEventID,
		EventType:     batchPowerEventType,
		AggregateType: batchPowerAggregateType,
		AggregateID:   strings.TrimSpace(input.ParentID),
		Payload:       input.ParentPayload,
		Status:        parentEventStatus,
		CreatedBy:     strings.TrimSpace(input.Actor),
	}); err != nil {
		return fmt.Errorf("insert batch power parent event %s: %w", parentEventID, err)
	}

	if err := qtx.InsertTicket(ctx, sqlcrepo.InsertTicketParams{
		ID:             strings.TrimSpace(input.ParentID),
		EventID:        parentEventID,
		OperationType:  batchPowerTicketOperation,
		Status:         parentTicketStatus,
		Requester:      strings.TrimSpace(input.Actor),
		Reason:         textOrNull(input.Reason),
		ParentTicketID: pgtype.Text{},
	}); err != nil {
		return fmt.Errorf("insert batch power parent ticket %s: %w", input.ParentID, err)
	}

	if err := qtx.InsertBatchTicket(ctx, sqlcrepo.InsertBatchTicketParams{
		ID:           strings.TrimSpace(input.ParentID),
		BatchType:    batchPowerType,
		ChildCount:   childCount,
		PendingCount: childCount,
		Status:       batchStatus,
		CreatedBy:    strings.TrimSpace(input.Actor),
		RequestID:    textOrNull(input.RequestID),
		Reason:       textOrNull(input.Reason),
	}); err != nil {
		return fmt.Errorf("insert batch power projection %s: %w", input.ParentID, err)
	}

	childTicketStatus := batchPowerTicketStatusExecuting
	if input.RequiresApproval {
		childTicketStatus = batchPowerEventStatusPending
	}
	for idx, child := range input.Children {
		childEventID, err := newV7ID()
		if err != nil {
			return fmt.Errorf("generate child event id %d: %w", idx, err)
		}
		childTicketID, err := newV7ID()
		if err != nil {
			return fmt.Errorf("generate child ticket id %d: %w", idx, err)
		}

		if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
			ID:            childEventID,
			EventType:     strings.TrimSpace(child.EventType),
			AggregateType: batchPowerChildAggregateType,
			AggregateID:   strings.TrimSpace(child.AggregateID),
			Payload:       child.Payload,
			Status:        batchPowerEventStatusPending,
			CreatedBy:     strings.TrimSpace(input.Actor),
		}); err != nil {
			return fmt.Errorf("insert batch power child event %s: %w", childEventID, err)
		}
		if err := qtx.InsertTicket(ctx, sqlcrepo.InsertTicketParams{
			ID:             childTicketID,
			EventID:        childEventID,
			OperationType:  batchPowerTicketOperation,
			Status:         childTicketStatus,
			Requester:      strings.TrimSpace(input.Actor),
			Reason:         textOrNull(child.Reason),
			ParentTicketID: textOrNull(input.ParentID),
		}); err != nil {
			return fmt.Errorf("insert batch power child ticket %s: %w", childTicketID, err)
		}
		if !input.RequiresApproval {
			affected, err := qtx.StartInitialBatchChildAttempt(ctx, sqlcrepo.StartInitialBatchChildAttemptParams{
				ID:             childTicketID,
				EventID:        childEventID,
				ParentTicketID: textOrNull(input.ParentID),
			})
			if err != nil {
				return fmt.Errorf("start initial batch power child attempt %s: %w", childTicketID, err)
			}
			if affected != 1 {
				return fmt.Errorf("start initial batch power child attempt %s: expected 1 row, got %d", childTicketID, affected)
			}
			if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{
				EventID: childEventID,
			}, nil); err != nil {
				return fmt.Errorf("enqueue vm_power for child event %s: %w", childEventID, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch power tx: %w", err)
	}
	return nil
}

func sqlcInt32Count(count int, label string) (int32, error) {
	if count < 0 || count > maxBatchPowerChildCountForSQLCInt {
		return 0, fmt.Errorf("%s exceeds int32 limit", label)
	}
	return int32(count), nil // #nosec G115 -- guarded by the int32 bounds check above.
}

func validateBatchPowerSubmissionInput(input BatchPowerSubmissionInput) error {
	if strings.TrimSpace(input.ParentID) == "" ||
		strings.TrimSpace(input.Actor) == "" ||
		len(input.ParentPayload) == 0 ||
		len(input.Children) == 0 {
		return fmt.Errorf("batch power input is incomplete")
	}
	canonicalOperation, action, expectedEventType, ok := batchPowerOperationIdentity(input.Operation)
	if !ok {
		return fmt.Errorf("batch power operation %q is invalid", input.Operation)
	}

	var parent domain.BatchVMRequestPayload
	if err := decodeBatchApprovalPayloadExact(input.ParentPayload, &parent); err != nil {
		return fmt.Errorf("batch power parent payload is malformed: %w", err)
	}
	if batchIdempotencyOperation(parent.Operation) != canonicalOperation {
		return fmt.Errorf("batch power parent payload operation does not match input operation")
	}
	if batchreplay.Normalize(parent.RequestID) != batchreplay.Normalize(input.RequestID) {
		return fmt.Errorf("batch power parent payload request id does not match input request id")
	}
	actor := strings.TrimSpace(input.Actor)
	if strings.TrimSpace(parent.SubmittedBy) != actor {
		return fmt.Errorf("batch power parent payload submitter does not match input actor")
	}
	if len(parent.Items) != len(input.Children) {
		return fmt.Errorf("batch power parent payload item count does not match child count")
	}

	remainingItems := make(map[string]int, len(parent.Items))
	for itemIndex := range parent.Items {
		key, err := batchApprovalItemKey(parent.Items[itemIndex])
		if err != nil {
			return fmt.Errorf("batch power parent payload item %d is invalid: %w", itemIndex, err)
		}
		remainingItems[key]++
	}
	seenVMs := make(map[string]int, len(input.Children))
	for idx, child := range input.Children {
		vmID := strings.TrimSpace(child.AggregateID)
		if strings.TrimSpace(child.EventType) == "" ||
			vmID == "" ||
			len(child.Payload) == 0 {
			return fmt.Errorf("batch power child %d is incomplete", idx)
		}
		if firstIndex, exists := seenVMs[vmID]; exists {
			return fmt.Errorf("batch power child %d repeats VM %q from child %d", idx, vmID, firstIndex)
		}
		seenVMs[vmID] = idx
		if !isPowerEventType(child.EventType) {
			return fmt.Errorf("batch power child %d has unsupported event type %q", idx, child.EventType)
		}
		if strings.TrimSpace(child.EventType) != expectedEventType {
			return fmt.Errorf("batch power child %d event type does not match input operation", idx)
		}

		var payload domain.VMPowerPayload
		if err := decodeBatchApprovalPayloadExact(child.Payload, &payload); err != nil {
			return fmt.Errorf("batch power child %d payload is malformed: %w", idx, err)
		}
		if strings.TrimSpace(payload.VMID) != vmID ||
			strings.TrimSpace(payload.Actor) != actor ||
			strings.TrimSpace(payload.VMName) == "" ||
			strings.TrimSpace(payload.ClusterID) == "" ||
			strings.TrimSpace(payload.Namespace) == "" ||
			payload.DispatchMode != domain.VMPowerDispatchTicket {
			return fmt.Errorf("batch power child %d payload identity is inconsistent", idx)
		}
		if strings.ToLower(strings.TrimSpace(payload.Operation)) != action {
			return fmt.Errorf("batch power child %d payload operation does not match input operation", idx)
		}
		itemKey, err := batchApprovalItemKey(batchPowerPayloadItem(payload))
		if err != nil || remainingItems[itemKey] == 0 {
			return fmt.Errorf("batch power child %d does not match the parent payload item set", idx)
		}
		remainingItems[itemKey]--
	}
	for _, count := range remainingItems {
		if count != 0 {
			return fmt.Errorf("batch power parent payload item set does not match children")
		}
	}
	return nil
}

func batchPowerOperationIdentity(operation string) (canonical, action, eventType string, ok bool) {
	canonical = batchIdempotencyOperation(operation)
	switch canonical {
	case batchPowerOperationStart:
		return canonical, powerOperationStart, powerEventTypeStart, true
	case batchPowerOperationStop:
		return canonical, powerOperationStop, powerEventTypeStop, true
	case batchPowerOperationRestart:
		return canonical, powerOperationRestart, powerEventTypeRestart, true
	default:
		return canonical, "", "", false
	}
}

func batchPowerPayloadItem(payload domain.VMPowerPayload) domain.BatchVMItemPayload {
	return domain.BatchVMItemPayload{
		VMID:               payload.VMID,
		VMName:             payload.VMName,
		SystemID:           payload.SystemID,
		SystemName:         payload.SystemName,
		ServiceID:          payload.ServiceID,
		ServiceName:        payload.ServiceName,
		TemplateID:         payload.TemplateID,
		TemplateName:       payload.TemplateName,
		InstanceSizeID:     payload.InstanceSizeID,
		InstanceSizeName:   payload.InstanceSizeName,
		Namespace:          payload.Namespace,
		ClusterID:          payload.ClusterID,
		ClusterName:        payload.ClusterName,
		ClusterEnvironment: payload.ClusterEnvironment,
		OwnerID:            payload.OwnerID,
		OwnerDisplayName:   payload.OwnerDisplayName,
		OwnerUsername:      payload.OwnerUsername,
		RequestVMStatus:    payload.RequestVMStatus,
		CurrentCPUCores:    payload.CurrentCPUCores,
		CurrentMemoryGi:    payload.CurrentMemoryGi,
		CurrentDiskGB:      payload.CurrentDiskGB,
		Operation:          strings.ToLower(strings.TrimSpace(payload.Operation)),
	}
}

func lockBatchSubmission(
	ctx context.Context,
	tx pgx.Tx,
	actor string,
	operation string,
	requestID string,
) error {
	lockKey := BatchIdempotencyLockKey(actor, operation, requestID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return fmt.Errorf("lock batch idempotency key: %w", err)
	}
	return nil
}

func findExistingBatchSubmission(
	ctx context.Context,
	tx pgx.Tx,
	actor string,
	operation string,
	requestID string,
) (string, error) {
	actor = strings.TrimSpace(actor)
	requestID = batchreplay.Normalize(requestID)
	wantedOperation := batchIdempotencyOperation(operation)
	wantedProjectionType := batchIdempotencyType(operation)
	rows, err := tx.Query(ctx, `
SELECT
  batch.id,
  batch.created_by,
  batch.batch_type::text,
  COALESCE(batch.request_id, ''),
  COALESCE(parent.id, ''),
  COALESCE(parent.requester, ''),
  COALESCE(parent.operation_type::text, ''),
  COALESCE(parent.event_id, ''),
  COALESCE(event.id, ''),
  COALESCE(event.event_type, ''),
  COALESCE(event.aggregate_type, ''),
  COALESCE(event.aggregate_id, ''),
  COALESCE(event.created_by, ''),
  COALESCE(event.payload, ''::bytea)
FROM batch_tickets AS batch
LEFT JOIN tickets AS parent
  ON parent.id = batch.id
 AND parent.parent_ticket_id IS NULL
LEFT JOIN domain_events AS event
  ON event.id = parent.event_id
WHERE batch.created_by = $1
  AND batch.batch_type::text = $2
  AND batch.request_id IS NOT NULL
  AND "shepherd_batch_replay_sha256"(BTRIM(batch.request_id, `+batchreplay.PostgreSQLTrimCutsetLiteral+`)) = $3
  AND BTRIM(batch.request_id, `+batchreplay.PostgreSQLTrimCutsetLiteral+`) = $4
ORDER BY batch.created_at, batch.id
LIMIT $5
`, actor, wantedProjectionType, batchreplay.Digest(requestID), requestID, batchreplay.CandidateLimit+1)
	if err != nil {
		return "", fmt.Errorf("query existing batch submission: %w", err)
	}
	defer rows.Close()

	type replayCandidate struct {
		batchID, projectionOwner, projectionType, projectionRequestID string
		parentID, parentRequester, parentOperation, parentEventID     string
		eventID, eventType, aggregateType, aggregateID, eventCreator  string
		payload                                                       []byte
	}
	candidates := make([]replayCandidate, 0, batchreplay.CandidateLimit+1)
	for rows.Next() {
		var candidate replayCandidate
		if scanErr := rows.Scan(
			&candidate.batchID,
			&candidate.projectionOwner,
			&candidate.projectionType,
			&candidate.projectionRequestID,
			&candidate.parentID,
			&candidate.parentRequester,
			&candidate.parentOperation,
			&candidate.parentEventID,
			&candidate.eventID,
			&candidate.eventType,
			&candidate.aggregateType,
			&candidate.aggregateID,
			&candidate.eventCreator,
			&candidate.payload,
		); scanErr != nil {
			return "", fmt.Errorf("scan existing batch submission: %w", scanErr)
		}
		candidates = append(candidates, candidate)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return "", fmt.Errorf("iterate existing batch submissions: %w", rowsErr)
	}
	if len(candidates) > batchreplay.CandidateLimit {
		return "", fmt.Errorf(
			"batch replay integrity violation: more than %d matching projections",
			batchreplay.CandidateLimit,
		)
	}

	matchedBatchID := ""
	for candidateIndex := range candidates {
		candidate := &candidates[candidateIndex]
		if strings.TrimSpace(candidate.projectionOwner) != actor ||
			strings.TrimSpace(candidate.projectionType) != wantedProjectionType ||
			batchreplay.Normalize(candidate.projectionRequestID) != requestID {
			return "", fmt.Errorf("batch replay integrity violation for projection %s: projection identity is inconsistent", candidate.batchID)
		}
		if candidate.parentID != candidate.batchID || strings.TrimSpace(candidate.parentRequester) != actor ||
			strings.TrimSpace(candidate.parentOperation) != batchPowerTicketOperation ||
			strings.TrimSpace(candidate.parentEventID) == "" {
			return "", fmt.Errorf("batch replay integrity violation for projection %s: root ticket identity is inconsistent", candidate.batchID)
		}
		if candidate.eventID != candidate.parentEventID || strings.TrimSpace(candidate.eventType) != batchPowerEventType ||
			strings.TrimSpace(candidate.aggregateType) != batchPowerAggregateType || candidate.aggregateID != candidate.batchID ||
			strings.TrimSpace(candidate.eventCreator) != actor {
			return "", fmt.Errorf("batch replay integrity violation for projection %s: parent event identity is inconsistent", candidate.batchID)
		}
		var persisted domain.BatchVMRequestPayload
		if decodeErr := decodeBatchApprovalPayloadExact(candidate.payload, &persisted); decodeErr != nil {
			return "", fmt.Errorf("batch replay integrity violation for projection %s: parent event payload is malformed: %w", candidate.batchID, decodeErr)
		}
		persistedOperation := batchIdempotencyOperation(persisted.Operation)
		if batchreplay.Normalize(persisted.RequestID) != requestID ||
			strings.TrimSpace(persisted.SubmittedBy) != actor {
			return "", fmt.Errorf("batch replay integrity violation for projection %s: parent payload identity is inconsistent", candidate.batchID)
		}
		if persistedOperation != wantedOperation {
			if !isBatchPowerOperation(persistedOperation) {
				return "", fmt.Errorf("batch replay integrity violation for projection %s: parent payload operation is inconsistent", candidate.batchID)
			}
			continue
		}
		if matchedBatchID == "" {
			matchedBatchID = candidate.batchID
		}
	}
	return matchedBatchID, nil
}

// RetryBatchPowerAndEnqueue resets failed power children and inserts their
// River jobs in one transaction.
func (w *ApprovalAtomicWriter) RetryBatchPowerAndEnqueue(ctx context.Context, input BatchPowerRetryInput) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.batch_power.retry",
		attribute.String("shepherd.business.operation", "batch_power.retry"),
		attribute.String("shepherd.batch.type", batchPowerType),
		attribute.Int("shepherd.batch.child_count", len(input.Children)),
	)
	defer func() {
		var active *ActivePowerEventError
		var retryConflict *PowerRetryJobConflictError
		var notEligible *PowerRetryNotEligibleError
		var exhausted *BatchChildAttemptsExhaustedError
		var parentNotEligible *BatchRetryParentNotEligibleError
		if !errors.As(err, &active) &&
			!errors.As(err, &retryConflict) &&
			!errors.As(err, &notEligible) &&
			!errors.As(err, &exhausted) &&
			!errors.As(err, &parentNotEligible) {
			observability.RecordSpanError(span, err)
		}
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if validationErr := validateBatchPowerRetryInput(input); validationErr != nil {
		return validationErr
	}
	normalizedChildren, err := normalizeBatchPowerSelectedChildren(input.Children)
	if err != nil {
		return err
	}
	input.Children = make([]BatchPowerRetryChildInput, 0, len(normalizedChildren))
	for _, child := range normalizedChildren {
		input.Children = append(input.Children, BatchPowerRetryChildInput(child))
	}

	// A retry can wait behind per-VM advisory locks before reading mutable child
	// state. Pin READ COMMITTED so that waiters cannot retain a stale snapshot
	// when the database default is REPEATABLE READ.
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch power retry tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, lockErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		BatchMutationLockKey(input.ParentID),
	); lockErr != nil {
		return fmt.Errorf("lock batch power retry parent %s: %w", input.ParentID, lockErr)
	}

	type retryPowerEvent struct {
		vmID     string
		ticketID string
		eventID  string
	}
	retryEvents := make([]retryPowerEvent, 0, len(input.Children))
	seenVMs := make(map[string]string, len(input.Children))
	for _, child := range input.Children {
		ticketID := strings.TrimSpace(child.TicketID)
		eventID := strings.TrimSpace(child.EventID)
		vmID, identityErr := loadBatchPowerRetryIdentity(ctx, tx, input.ParentID, ticketID, eventID)
		if identityErr != nil {
			return identityErr
		}
		if existingEventID, exists := seenVMs[vmID]; exists {
			return fmt.Errorf("power retry events %s and %s target the same VM %s", existingEventID, eventID, vmID)
		}
		seenVMs[vmID] = eventID
		retryEvents = append(retryEvents, retryPowerEvent{vmID: vmID, ticketID: ticketID, eventID: eventID})
	}
	// Probe each same-event job before checking the mutable FAILED state. A
	// request queued behind the parent lock then reports the runnable job
	// committed by the winner as an in-progress conflict. The actual retry job
	// remains coupled to the child reset later in this transaction.
	for _, retryEvent := range retryEvents {
		if probeErr := probeRunnablePowerRetryJob(ctx, tx, retryEvent.eventID); probeErr != nil {
			return probeErr
		}
	}
	sort.Slice(retryEvents, func(i, j int) bool {
		return retryEvents[i].vmID < retryEvents[j].vmID
	})
	for _, retryEvent := range retryEvents {
		if lockErr := lockPowerVM(ctx, tx, retryEvent.vmID); lockErr != nil {
			return lockErr
		}
	}
	for _, retryEvent := range retryEvents {
		active, activeErr := findActivePowerEvent(ctx, tx, retryEvent.vmID, retryEvent.eventID)
		if activeErr != nil {
			return activeErr
		}
		if active != nil {
			return active
		}
	}

	parentID := strings.TrimSpace(input.ParentID)
	parentEventID, identityErr := loadBatchPowerParentEventID(ctx, tx, parentID)
	if identityErr != nil {
		return identityErr
	}
	if _, _, _, identityErr := validateAndLockBatchApprovalIdentity(
		ctx,
		tx,
		parentID,
		parentEventID,
		batchApprovalIdentityPowerRetry,
		normalizedChildren,
	); identityErr != nil {
		return identityErr
	}
	for _, retryEvent := range retryEvents {
		lockedVMID, identityErr := loadBatchPowerRetryIdentity(
			ctx,
			tx,
			parentID,
			retryEvent.ticketID,
			retryEvent.eventID,
		)
		if identityErr != nil {
			return identityErr
		}
		if lockedVMID != retryEvent.vmID {
			return &PowerRetryNotEligibleError{TicketID: retryEvent.ticketID, EventID: retryEvent.eventID}
		}
	}

	qtx := w.queries.WithTx(tx)
	for _, child := range input.Children {
		ticketID := strings.TrimSpace(child.TicketID)
		eventID := strings.TrimSpace(child.EventID)
		affected, resetErr := qtx.ResetPowerRetryTicket(ctx, sqlcrepo.ResetPowerRetryTicketParams{
			ID:             ticketID,
			EventID:        eventID,
			ParentTicketID: textOrNull(parentID),
			MaxAttempts:    domain.BatchChildMaxAttempts,
		})
		if resetErr != nil {
			return fmt.Errorf("reset power retry ticket %s: %w", ticketID, resetErr)
		}
		if affected == 0 {
			return w.classifyPowerRetryStateConflict(ctx, tx, parentID, ticketID, eventID, true)
		}
		affected, err = qtx.ResetBatchPowerRetryEvent(ctx, sqlcrepo.ResetBatchPowerRetryEventParams{
			EventID:        eventID,
			TicketID:       ticketID,
			ParentTicketID: textOrNull(parentID),
		})
		if err != nil {
			return fmt.Errorf("reset power retry event %s: %w", eventID, err)
		}
		if affected == 0 {
			return w.classifyPowerRetryStateConflict(ctx, tx, parentID, ticketID, eventID, false)
		}
		if insertErr := w.insertPowerRetryJob(ctx, tx, ticketID, eventID); insertErr != nil {
			return insertErr
		}
	}

	// Reopen the parent in the same transaction as the child resets and River
	// inserts. Otherwise a worker can observe the committed retry job while the
	// parent is still FAILED, and its ordinary (non-reopening) parent
	// sync will reject the child transition. A missing or incompatible parent
	// is treated as corruption and rolls the entire retry back.
	reopenedParentEventID, err := qtx.ReopenBatchPowerParentForRetry(ctx, parentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &BatchRetryParentNotEligibleError{ParentTicketID: parentID}
		}
		return fmt.Errorf("reopen batch power parent %s for retry: %w", parentID, err)
	}
	if strings.TrimSpace(reopenedParentEventID) == "" {
		return fmt.Errorf("reopen batch power parent %s for retry: parent event id is empty", parentID)
	}
	if strings.TrimSpace(reopenedParentEventID) != parentEventID {
		return &BatchRetryParentNotEligibleError{ParentTicketID: parentID, ParentEventID: parentEventID}
	}
	projectionRows, err := qtx.RefreshBatchPowerProjectionForRetry(ctx, parentID)
	if err != nil {
		return fmt.Errorf("refresh batch power projection %s for retry: %w", parentID, err)
	}
	if projectionRows != 1 {
		return fmt.Errorf(
			"refresh batch power projection %s for retry: expected 1 row, got %d",
			parentID,
			projectionRows,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch power retry tx: %w", err)
	}
	return nil
}

func normalizeBatchPowerSelectedChildren(children []BatchPowerRetryChildInput) ([]batchApprovalSelectedChild, error) {
	selected := make([]batchApprovalSelectedChild, len(children))
	for childIndex := range children {
		selected[childIndex] = batchApprovalSelectedChild(children[childIndex])
	}
	return normalizeBatchSelectedChildren(selected, "batch power retry")
}

func loadBatchPowerParentEventID(ctx context.Context, tx pgx.Tx, parentID string) (string, error) {
	var eventID string
	err := tx.QueryRow(ctx, `
SELECT event_id
FROM tickets
WHERE id = $1
  AND parent_ticket_id IS NULL
`, strings.TrimSpace(parentID)).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", &BatchRetryParentNotEligibleError{ParentTicketID: strings.TrimSpace(parentID)}
	}
	if err != nil {
		return "", fmt.Errorf("load batch power parent %s event id: %w", parentID, err)
	}
	if strings.TrimSpace(eventID) == "" {
		return "", &BatchRetryParentNotEligibleError{ParentTicketID: strings.TrimSpace(parentID)}
	}
	return strings.TrimSpace(eventID), nil
}

func loadBatchPowerRetryIdentity(
	ctx context.Context,
	tx pgx.Tx,
	parentID, ticketID, eventID string,
) (string, error) {
	const query = `
SELECT event.event_type, event.aggregate_id, event.payload
FROM tickets AS ticket
JOIN domain_events AS event ON event.id = ticket.event_id
WHERE ticket.id = $1
  AND ticket.event_id = $2
  AND ticket.parent_ticket_id = $3
  AND ticket.operation_type = 'POWER'
  AND event.aggregate_type = 'vm'
`
	var (
		eventType   string
		aggregateID string
		payload     []byte
	)
	err := tx.QueryRow(
		ctx,
		query,
		strings.TrimSpace(ticketID),
		strings.TrimSpace(eventID),
		strings.TrimSpace(parentID),
	).Scan(&eventType, &aggregateID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", &PowerRetryNotEligibleError{TicketID: strings.TrimSpace(ticketID), EventID: strings.TrimSpace(eventID)}
	}
	if err != nil {
		return "", fmt.Errorf("load power retry identity for ticket %s/event %s: %w", ticketID, eventID, err)
	}
	aggregateID = strings.TrimSpace(aggregateID)
	var decoded domain.VMPowerPayload
	if aggregateID == "" || json.Unmarshal(payload, &decoded) != nil || strings.TrimSpace(decoded.VMID) != aggregateID {
		return "", &PowerRetryNotEligibleError{TicketID: strings.TrimSpace(ticketID), EventID: strings.TrimSpace(eventID)}
	}
	operation := strings.ToLower(strings.TrimSpace(decoded.Operation))
	eventMatches := (operation == powerOperationStart && eventType == powerEventTypeStart) ||
		(operation == powerOperationStop && eventType == powerEventTypeStop) ||
		(operation == powerOperationRestart && eventType == powerEventTypeRestart)
	if !eventMatches {
		return "", &PowerRetryNotEligibleError{TicketID: strings.TrimSpace(ticketID), EventID: strings.TrimSpace(eventID)}
	}
	return aggregateID, nil
}

func probeRunnablePowerRetryJob(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
) error {
	var (
		jobID int64
		state string
	)
	err := tx.QueryRow(ctx, `
SELECT id, state::text
FROM river_job
WHERE kind = 'vm_power'
  AND args->>'event_id' = $1
  AND state IN ('available', 'pending', 'retryable', 'running', 'scheduled')
ORDER BY id DESC
LIMIT 1
`, strings.TrimSpace(eventID)).Scan(&jobID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("probe runnable vm_power retry for event %s: %w", eventID, err)
	}
	return &PowerRetryJobConflictError{
		EventID:          strings.TrimSpace(eventID),
		ExistingJobID:    jobID,
		ExistingJobState: state,
	}
}

// insertPowerRetryJob preserves active-job uniqueness while allowing retries
// across the v0.39 uniqueness-policy rollout. Jobs written before completed was
// removed from VMPowerArgs.ByState can retain a terminal unique key until the
// River cleaner runs. The caller invokes this after the exact graph validation
// and child reset inside the same retry transaction, so deleting a terminal
// predecessor and inserting its replacement commit atomically with that reset.
func (w *ApprovalAtomicWriter) insertPowerRetryJob(
	ctx context.Context,
	tx pgx.Tx,
	ticketID string,
	eventID string,
) error {
	insert := func() (*rivertype.JobInsertResult, error) {
		inserted, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{EventID: eventID}, nil)
		if err != nil {
			return nil, fmt.Errorf("enqueue vm_power retry for event %s: %w", eventID, err)
		}
		if inserted == nil || inserted.Job == nil {
			return nil, fmt.Errorf("enqueue vm_power retry for event %s: River returned no job", eventID)
		}
		return inserted, nil
	}

	inserted, err := insert()
	if err != nil {
		return err
	}
	if !inserted.UniqueSkippedAsDuplicate {
		return nil
	}
	switch inserted.Job.State {
	case rivertype.JobStateCancelled, rivertype.JobStateCompleted, rivertype.JobStateDiscarded:
		if _, err := w.riverClient.JobDeleteTx(ctx, tx, inserted.Job.ID); err != nil {
			return fmt.Errorf(
				"delete terminal vm_power predecessor %d for retry event %s: %w",
				inserted.Job.ID,
				eventID,
				err,
			)
		}
		replacement, err := insert()
		if err != nil {
			return err
		}
		if !replacement.UniqueSkippedAsDuplicate {
			return nil
		}
		return powerRetryDuplicateError(ticketID, eventID, replacement.Job.ID, replacement.Job.State)
	default:
		return powerRetryDuplicateError(ticketID, eventID, inserted.Job.ID, inserted.Job.State)
	}
}

// classifyPowerRetryStateConflict asks River whether an equivalent job is
// still covered by the worker's uniqueness policy. A non-duplicate probe is
// intentionally rolled back when the returned not-eligible error unwinds the
// surrounding transaction, so stale retries never enqueue fresh work.
func (w *ApprovalAtomicWriter) classifyPowerRetryStateConflict(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	ticketID string,
	eventID string,
	checkExhausted bool,
) error {
	if !checkExhausted {
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	}

	var (
		status       string
		attemptCount int
	)
	err := tx.QueryRow(ctx, `
SELECT status, attempt_count
FROM tickets
WHERE id = $1
  AND event_id = $2
  AND parent_ticket_id = $3
  AND operation_type = 'POWER'
FOR UPDATE
	`, strings.TrimSpace(ticketID), strings.TrimSpace(eventID), strings.TrimSpace(parentID)).Scan(&status, &attemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	}
	if err != nil {
		return fmt.Errorf("classify power retry ticket %s: %w", ticketID, err)
	}
	if status == batchApprovalTicketStatusRejected {
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	}
	if status == batchPowerTicketStatusFailed && attemptCount >= domain.BatchChildMaxAttempts {
		return &BatchChildAttemptsExhaustedError{
			TicketID:     ticketID,
			AttemptCount: attemptCount,
			MaxAttempts:  domain.BatchChildMaxAttempts,
		}
	}
	if status == batchPowerTicketStatusFailed {
		// The conditional reset already locked this exact row. If it remains
		// retryable and below the attempt cap, no concurrent state transition
		// can explain the zero row count; treat the stale request as ineligible
		// without asking River to insert a probe job.
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	}

	inserted, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{EventID: eventID}, nil)
	if err != nil {
		return fmt.Errorf("classify power retry state for event %s: %w", eventID, err)
	}
	if inserted == nil || inserted.Job == nil {
		return fmt.Errorf("classify power retry state for event %s: River returned no job", eventID)
	}
	if inserted.UniqueSkippedAsDuplicate {
		return powerRetryDuplicateError(ticketID, eventID, inserted.Job.ID, inserted.Job.State)
	}
	return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
}

func powerRetryDuplicateError(
	ticketID string,
	eventID string,
	jobID int64,
	state rivertype.JobState,
) error {
	switch state {
	case rivertype.JobStateCancelled, rivertype.JobStateCompleted, rivertype.JobStateDiscarded:
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	default:
		return &PowerRetryJobConflictError{
			EventID:          eventID,
			ExistingJobID:    jobID,
			ExistingJobState: string(state),
		}
	}
}

func validateBatchPowerRetryInput(input BatchPowerRetryInput) error {
	if strings.TrimSpace(input.ParentID) == "" || len(input.Children) == 0 {
		return fmt.Errorf("batch power retry input is incomplete")
	}
	for idx, child := range input.Children {
		if strings.TrimSpace(child.TicketID) == "" || strings.TrimSpace(child.EventID) == "" {
			return fmt.Errorf("batch power retry child %d is incomplete", idx)
		}
	}
	return nil
}

func textOrNull(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	return pgtype.Text{String: trimmed, Valid: trimmed != ""}
}

func newV7ID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
