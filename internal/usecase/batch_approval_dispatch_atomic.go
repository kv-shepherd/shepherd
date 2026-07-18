package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	sqlcrepo "kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// BatchApprovalDispatchConflictError reports that a runnable parent-scoped
// dispatcher already exists. Callers must leave child state untouched and
// surface the retry as an expected in-progress conflict.
type BatchApprovalDispatchConflictError struct {
	ParentTicketID   string
	ExistingJobID    int64
	ExistingJobState string
}

func (e *BatchApprovalDispatchConflictError) Error() string {
	return fmt.Sprintf(
		"batch approval parent %s already has River dispatcher %d in state %s",
		e.ParentTicketID,
		e.ExistingJobID,
		e.ExistingJobState,
	)
}

// BatchApprovalRetryNotEligibleError identifies a generic child whose state
// changed after the handler selected it for retry.
type BatchApprovalRetryNotEligibleError struct {
	TicketID string
	EventID  string
}

// BatchRetryParentNotEligibleError reports that the durable parent ticket or
// event lost the approved execution state before an atomic retry could commit.
type BatchRetryParentNotEligibleError struct {
	ParentTicketID string
	ParentEventID  string
}

func (e *BatchRetryParentNotEligibleError) Error() string {
	return fmt.Sprintf(
		"batch retry parent %s/event %s is no longer in an approved execution state",
		e.ParentTicketID,
		e.ParentEventID,
	)
}

// BatchMutationLockKey serializes explicit retry/cancel intents for one parent
// across handler instances. Child workers remain independently idempotent.
func BatchMutationLockKey(parentID string) string {
	return "batch:mutation:" + strings.TrimSpace(parentID)
}

func (e *BatchApprovalRetryNotEligibleError) Error() string {
	return fmt.Sprintf("batch approval retry ticket %s for event %s is no longer eligible", e.TicketID, e.EventID)
}

// ClaimBatchApprovalAndEnqueue atomically claims a pending batch parent,
// persists the normalized execution plan, and inserts its durable dispatcher.
func (w *ApprovalAtomicWriter) ClaimBatchApprovalAndEnqueue(
	ctx context.Context,
	input domain.BatchApprovalClaimInput,
) (err error) {
	if validationErr := validateBatchApprovalClaimInput(input); validationErr != nil {
		return validationErr
	}
	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	execution := normalizeBatchApprovalExecution(input.Execution)
	executionJSON, err := json.Marshal(execution)
	if err != nil {
		return fmt.Errorf("marshal batch approval execution plan: %w", err)
	}

	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch approval claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.queries.WithTx(tx)
	if _, _, _, identityErr := validateAndLockBatchApprovalIdentity(
		ctx,
		tx,
		strings.TrimSpace(input.ParentTicketID),
		strings.TrimSpace(input.ParentEventID),
		batchApprovalIdentityClaim,
		nil,
	); identityErr != nil {
		return identityErr
	}

	parentRows, err := qtx.ClaimBatchApprovalDispatch(ctx, sqlcrepo.ClaimBatchApprovalDispatchParams{
		Approver:             requiredText(input.Approver),
		SelectedClusterID:    textOrNull(execution.ClusterID),
		SelectedStorageClass: textOrNull(execution.StorageClass),
		ExecutionOptions:     executionJSON,
		ID:                   strings.TrimSpace(input.ParentTicketID),
		EventID:              strings.TrimSpace(input.ParentEventID),
	})
	if err != nil {
		return fmt.Errorf("claim batch approval parent %s: %w", input.ParentTicketID, err)
	}
	if parentRows != 1 {
		return fmt.Errorf("claim batch approval parent %s: expected 1 row, got %d", input.ParentTicketID, parentRows)
	}
	eventRows, err := qtx.ClaimBatchApprovalEventProcessing(ctx, sqlcrepo.ClaimBatchApprovalEventProcessingParams{
		EventID:  strings.TrimSpace(input.ParentEventID),
		ParentID: strings.TrimSpace(input.ParentTicketID),
	})
	if err != nil {
		return fmt.Errorf("claim batch approval event %s: %w", input.ParentEventID, err)
	}
	if eventRows != 1 {
		return fmt.Errorf("claim batch approval event %s: expected 1 row, got %d", input.ParentEventID, eventRows)
	}
	if err := refreshBatchApprovalProjection(ctx, qtx, input.ParentTicketID); err != nil {
		return err
	}
	if err := w.insertBatchApprovalDispatcher(ctx, tx, input.ParentTicketID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch approval claim tx: %w", err)
	}
	return nil
}

// RetryBatchApprovalAndEnqueue atomically records one logical retry attempt,
// reopens the parent/event/projection, and inserts the durable dispatcher.
func (w *ApprovalAtomicWriter) RetryBatchApprovalAndEnqueue(
	ctx context.Context,
	input domain.BatchApprovalRetryInput,
) (err error) {
	if validationErr := validateBatchApprovalRetryInput(input); validationErr != nil {
		return validationErr
	}
	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	normalizedChildren, err := normalizeBatchApprovalSelectedChildren(input.Children)
	if err != nil {
		return err
	}
	input.Children = make([]domain.BatchApprovalRetryChild, 0, len(normalizedChildren))
	for _, child := range normalizedChildren {
		input.Children = append(input.Children, domain.BatchApprovalRetryChild{
			TicketID: child.TicketID,
			EventID:  child.EventID,
		})
	}
	execution := normalizeBatchApprovalExecution(input.Execution)
	executionJSON, err := json.Marshal(execution)
	if err != nil {
		return fmt.Errorf("marshal batch approval retry execution plan: %w", err)
	}

	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch approval retry tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.queries.WithTx(tx)
	if _, lockErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		BatchMutationLockKey(input.ParentTicketID),
	); lockErr != nil {
		return fmt.Errorf("lock batch approval retry parent %s: %w", input.ParentTicketID, lockErr)
	}
	// Probe/insert while holding the parent mutation lock, before requiring the
	// selected child to still be FAILED. A request queued behind the winning
	// retry then observes its runnable dispatcher as an in-progress conflict
	// instead of misreporting that there is nothing to retry. Any new probe is
	// provisional and rolls back if the exact graph validation below fails.
	if insertErr := w.insertBatchApprovalDispatcher(ctx, tx, input.ParentTicketID); insertErr != nil {
		return insertErr
	}
	if _, _, _, identityErr := validateAndLockBatchApprovalIdentity(
		ctx,
		tx,
		strings.TrimSpace(input.ParentTicketID),
		strings.TrimSpace(input.ParentEventID),
		batchApprovalIdentityGenericRetry,
		normalizedChildren,
	); identityErr != nil {
		return identityErr
	}

	for _, child := range input.Children {
		ticketRows, resetErr := qtx.ResetBatchApprovalRetryChild(ctx, sqlcrepo.ResetBatchApprovalRetryChildParams{
			ID:             strings.TrimSpace(child.TicketID),
			EventID:        strings.TrimSpace(child.EventID),
			ParentTicketID: requiredText(input.ParentTicketID),
			MaxAttempts:    int32(domain.BatchChildMaxAttempts),
		})
		if resetErr != nil {
			return fmt.Errorf("reset batch approval retry ticket %s: %w", child.TicketID, resetErr)
		}
		if ticketRows != 1 {
			return w.classifyBatchApprovalRetryConflict(ctx, tx, input.ParentTicketID, child)
		}
		eventRows, resetEventErr := qtx.ResetBatchApprovalRetryEvent(ctx, sqlcrepo.ResetBatchApprovalRetryEventParams{
			EventID:        strings.TrimSpace(child.EventID),
			TicketID:       strings.TrimSpace(child.TicketID),
			ParentTicketID: requiredText(input.ParentTicketID),
		})
		if resetEventErr != nil {
			return fmt.Errorf("reset batch approval retry event %s: %w", child.EventID, resetEventErr)
		}
		if eventRows != 1 {
			return &BatchApprovalRetryNotEligibleError{TicketID: child.TicketID, EventID: child.EventID}
		}
	}

	parentRows, err := qtx.ReopenBatchApprovalDispatch(ctx, sqlcrepo.ReopenBatchApprovalDispatchParams{
		Approver:             requiredText(input.Approver),
		SelectedClusterID:    textOrNull(execution.ClusterID),
		SelectedStorageClass: textOrNull(execution.StorageClass),
		ExecutionOptions:     executionJSON,
		ID:                   strings.TrimSpace(input.ParentTicketID),
		EventID:              strings.TrimSpace(input.ParentEventID),
	})
	if err != nil {
		return fmt.Errorf("reopen batch approval parent %s: %w", input.ParentTicketID, err)
	}
	if parentRows != 1 {
		return &BatchRetryParentNotEligibleError{
			ParentTicketID: strings.TrimSpace(input.ParentTicketID),
			ParentEventID:  strings.TrimSpace(input.ParentEventID),
		}
	}
	eventRows, err := qtx.SetBatchApprovalEventProcessing(ctx, sqlcrepo.SetBatchApprovalEventProcessingParams{
		EventID:  strings.TrimSpace(input.ParentEventID),
		ParentID: strings.TrimSpace(input.ParentTicketID),
	})
	if err != nil {
		return fmt.Errorf("reopen batch approval event %s: %w", input.ParentEventID, err)
	}
	if eventRows != 1 {
		return &BatchRetryParentNotEligibleError{
			ParentTicketID: strings.TrimSpace(input.ParentTicketID),
			ParentEventID:  strings.TrimSpace(input.ParentEventID),
		}
	}
	if err := refreshBatchApprovalProjection(ctx, qtx, input.ParentTicketID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch approval retry tx: %w", err)
	}
	return nil
}

// ValidateBatchApprovalDispatchGraph reuses the claim/retry exact graph
// validator at dispatcher execution time. The durable graph can be edited or
// corrupted after the claim commits; a worker must fail closed before it
// dispatches or terminalizes any child in that case.
func (w *ApprovalAtomicWriter) ValidateBatchApprovalDispatchGraph(
	ctx context.Context,
	parentTicketID string,
	parentEventID string,
) (domain.BatchApprovalDispatchGuard, error) {
	parentTicketID = strings.TrimSpace(parentTicketID)
	parentEventID = strings.TrimSpace(parentEventID)
	if parentTicketID == "" || parentEventID == "" {
		return domain.BatchApprovalDispatchGuard{}, fmt.Errorf("batch approval dispatch identity is incomplete")
	}
	if w == nil || w.pool == nil {
		return domain.BatchApprovalDispatchGuard{}, fmt.Errorf("approval atomic writer is not initialized")
	}
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.BatchApprovalDispatchGuard{}, fmt.Errorf("begin batch approval dispatch validation tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, lockErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		BatchMutationLockKey(parentTicketID),
	); lockErr != nil {
		return domain.BatchApprovalDispatchGuard{}, fmt.Errorf("lock batch approval dispatch parent %s: %w", parentTicketID, lockErr)
	}
	fingerprint, parent, childInputs, err := validateAndLockBatchApprovalIdentity(
		ctx,
		tx,
		parentTicketID,
		parentEventID,
		batchApprovalIdentityDispatch,
		nil,
	)
	if err != nil {
		return domain.BatchApprovalDispatchGuard{}, err
	}
	execution, err := batchApprovalExecutionFromLockedParent(parent)
	if err != nil {
		return domain.BatchApprovalDispatchGuard{}, batchApprovalParentIdentityError(
			batchApprovalIdentityDispatch,
			parentTicketID,
			parentEventID,
			"persisted execution plan is malformed",
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.BatchApprovalDispatchGuard{}, fmt.Errorf("commit batch approval dispatch validation tx: %w", err)
	}
	return domain.BatchApprovalDispatchGuard{
		ParentTicketID:        parentTicketID,
		ParentEventID:         parentEventID,
		GraphFingerprint:      fingerprint,
		ChildInputFingerprint: childInputs,
		Approver:              strings.TrimSpace(parent.Approver.String),
		Execution:             execution,
	}, nil
}

type batchApprovalIdentityMode uint8

const (
	batchApprovalIdentityClaim batchApprovalIdentityMode = iota
	batchApprovalIdentityGenericRetry
	batchApprovalIdentityPowerRetry
	batchApprovalIdentityDispatch
	batchApprovalIdentityChildDispatch

	batchApprovalTicketStatusApproved  = "APPROVED"
	batchApprovalTicketStatusRejected  = "REJECTED"
	batchApprovalTicketStatusCancelled = "CANCELLED"
)

type batchApprovalSelectedChild struct {
	TicketID string
	EventID  string
}

type lockedBatchApprovalChild struct {
	TicketID            string
	EventID             string
	Operation           string
	Status              string
	Requester           string
	Approver            pgtype.Text
	Reason              pgtype.Text
	RejectReason        pgtype.Text
	SelectedCluster     pgtype.Text
	SelectedStorage     pgtype.Text
	TemplateSnapshot    []byte
	InstanceSnapshot    []byte
	PlacementEvaluation []byte
	ModifiedSpec        []byte
	AttemptCount        int
}

type lockedBatchApprovalEvent struct {
	EventID       string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       []byte
	Status        string
	CreatedBy     string
}

type batchApprovalParentIdentity struct {
	Operation        string
	Status           string
	Requester        string
	Approver         pgtype.Text
	SelectedCluster  pgtype.Text
	SelectedStorage  pgtype.Text
	ModifiedSpec     []byte
	EventType        string
	EventAggregate   string
	EventAggregateID string
	EventStatus      string
	EventCreatedBy   string
	EventPayload     []byte
	BatchType        string
	BatchStatus      string
	BatchCreatedBy   string
	ChildCount       int
	SuccessCount     int
	FailedCount      int
	PendingCount     int
}

type batchApprovalChildGraphIdentity struct {
	ItemKey     string
	Target      string
	PowerAction string
}

// validateAndLockBatchApprovalIdentity validates the complete durable batch
// graph before a claim/retry consumes any state. The two separate FOR UPDATE
// queries intentionally establish the global row-lock order used by decision
// paths: all child tickets by ID, then all child events by ID. Parent/event and
// projection writes remain exact CAS operations after these locks are held.
func validateAndLockBatchApprovalIdentity(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	parentEventID string,
	mode batchApprovalIdentityMode,
	selected []batchApprovalSelectedChild,
) (graphFingerprint string, lockedParent batchApprovalParentIdentity, childInputFingerprints map[string]string, validationErr error) {
	parentID = strings.TrimSpace(parentID)
	parentEventID = strings.TrimSpace(parentEventID)

	children, err := lockAllBatchApprovalChildren(ctx, tx, parentID)
	if err != nil {
		return "", batchApprovalParentIdentity{}, nil, err
	}
	if len(children) == 0 {
		return "", batchApprovalParentIdentity{}, nil, batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent has no child tickets")
	}
	events, err := lockAllBatchApprovalChildEvents(ctx, tx, parentID, children)
	if err != nil {
		return "", batchApprovalParentIdentity{}, nil, err
	}
	parent, err := loadBatchApprovalParentIdentity(ctx, tx, parentID, parentEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", batchApprovalParentIdentity{}, nil, batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent ticket/event/projection identity is incomplete")
	}
	if err != nil {
		return "", batchApprovalParentIdentity{}, nil, fmt.Errorf("load batch approval parent %s identity: %w", parentID, err)
	}
	if parentValidationErr := validateBatchApprovalParentIdentity(mode, parentID, parentEventID, parent); parentValidationErr != nil {
		return "", parent, nil, parentValidationErr
	}
	if mode != batchApprovalIdentityChildDispatch {
		if projectionErr := validateBatchApprovalProjectionCounts(mode, parentID, parentEventID, parent, children); projectionErr != nil {
			return "", parent, nil, projectionErr
		}
	}

	selectedByTicket := make(map[string]string, len(selected))
	for _, child := range selected {
		selectedByTicket[child.TicketID] = child.EventID
	}
	childItems := make([]string, 0, len(children))
	powerTargets := make(map[string]string, len(children))
	powerAction := ""
	for childIndex := range children {
		child := &children[childIndex]
		event := events[child.EventID]
		if child.AttemptCount < 0 {
			return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "child attempt count is negative")
		}
		if event == nil ||
			strings.TrimSpace(child.Operation) != strings.TrimSpace(parent.Operation) ||
			strings.TrimSpace(child.Requester) == "" ||
			strings.TrimSpace(child.Requester) != strings.TrimSpace(parent.Requester) ||
			strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(child.Requester) {
			return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "ticket/event identity does not match its parent")
		}
		graphIdentity, ok := batchApprovalChildGraphIdentityMatches(*child, *event)
		if !ok {
			return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "ticket/event/payload identity does not match its parent")
		}
		childItems = append(childItems, graphIdentity.ItemKey)
		if child.Operation == "POWER" {
			if previous, exists := powerTargets[graphIdentity.Target]; exists {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "power siblings target the same VM as "+previous)
			}
			powerTargets[graphIdentity.Target] = child.TicketID
			if powerAction == "" {
				powerAction = graphIdentity.PowerAction
			} else if powerAction != graphIdentity.PowerAction {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "power sibling action differs from the batch action")
			}
		}

		switch mode {
		case batchApprovalIdentityClaim:
			if child.Status != "PENDING" || event.Status != "PENDING" || child.AttemptCount != 0 {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "initial child is not untouched PENDING state")
			}
		case batchApprovalIdentityGenericRetry, batchApprovalIdentityPowerRetry:
			selectedEventID, ok := selectedByTicket[child.TicketID]
			if !ok {
				if !batchApprovalUnselectedRetryStateAllowed(mode, child.Status, event.Status) {
					return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "unselected sibling is not in a stable non-dispatchable state")
				}
				continue
			}
			if selectedEventID != child.EventID {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, selectedEventID, "selected ticket/event binding changed")
			}
			if child.Status != "FAILED" || !batchApprovalRetryEventStatusAllowed(mode, event.Status) {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "selected child is not in a retryable terminal state")
			}
			if child.AttemptCount >= domain.BatchChildMaxAttempts {
				return "", parent, nil, &BatchChildAttemptsExhaustedError{
					TicketID:     child.TicketID,
					AttemptCount: child.AttemptCount,
					MaxAttempts:  domain.BatchChildMaxAttempts,
				}
			}
			delete(selectedByTicket, child.TicketID)
		case batchApprovalIdentityDispatch:
			if !batchApprovalDispatchChildStateAllowed(child.Status, event.Status) {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "child ticket/event state pair is invalid for dispatch")
			}
		case batchApprovalIdentityChildDispatch:
			selectedEventID, selectedChild := selectedByTicket[child.TicketID]
			if !selectedChild {
				if !batchApprovalDispatchChildStateAllowed(child.Status, event.Status) {
					return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "sibling ticket/event state pair is invalid for dispatch")
				}
				continue
			}
			if selectedEventID != child.EventID || child.Status != "PENDING" || event.Status != "PENDING" {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, selectedEventID, "selected child is no longer pending")
			}
			delete(selectedByTicket, child.TicketID)
		}
	}
	if payloadErr := validateBatchApprovalParentPayload(mode, parentID, parentEventID, parent, childItems, powerAction); payloadErr != nil {
		return "", parent, nil, payloadErr
	}
	if len(selectedByTicket) != 0 {
		for _, child := range selected {
			if _, missing := selectedByTicket[child.TicketID]; missing {
				return "", parent, nil, batchApprovalChildIdentityError(mode, child.TicketID, child.EventID, "selected child is not owned by the parent")
			}
		}
	}
	fingerprint, err := batchApprovalGraphFingerprint(parentID, parentEventID, parent, children, events)
	if err != nil {
		return "", parent, nil, fmt.Errorf("fingerprint batch approval parent %s graph: %w", parentID, err)
	}
	childInputs, err := batchApprovalChildInputFingerprints(children)
	if err != nil {
		return "", parent, nil, fmt.Errorf("fingerprint batch approval parent %s child inputs: %w", parentID, err)
	}
	return fingerprint, parent, childInputs, nil
}

func lockAllBatchApprovalChildren(ctx context.Context, tx pgx.Tx, parentID string) ([]lockedBatchApprovalChild, error) {
	rows, err := tx.Query(ctx, `
/* batch_approval_lock_child_tickets */
SELECT
  id,
  event_id,
  operation_type,
  status,
  requester,
  approver,
  reason,
  reject_reason,
  selected_cluster_id,
  selected_storage_class,
  template_snapshot,
  instance_size_snapshot,
  placement_evaluation,
  modified_spec,
  attempt_count
FROM tickets
WHERE parent_ticket_id = $1
ORDER BY id
FOR UPDATE
`, parentID)
	if err != nil {
		return nil, fmt.Errorf("lock batch approval parent %s child tickets: %w", parentID, err)
	}
	defer rows.Close()

	children := make([]lockedBatchApprovalChild, 0)
	for rows.Next() {
		var child lockedBatchApprovalChild
		if err := rows.Scan(
			&child.TicketID,
			&child.EventID,
			&child.Operation,
			&child.Status,
			&child.Requester,
			&child.Approver,
			&child.Reason,
			&child.RejectReason,
			&child.SelectedCluster,
			&child.SelectedStorage,
			&child.TemplateSnapshot,
			&child.InstanceSnapshot,
			&child.PlacementEvaluation,
			&child.ModifiedSpec,
			&child.AttemptCount,
		); err != nil {
			return nil, fmt.Errorf("scan batch approval parent %s child ticket: %w", parentID, err)
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch approval parent %s child tickets: %w", parentID, err)
	}
	return children, nil
}

func lockAllBatchApprovalChildEvents(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	children []lockedBatchApprovalChild,
) (map[string]*lockedBatchApprovalEvent, error) {
	eventIDs := make([]string, 0, len(children))
	eventOwner := make(map[string]string, len(children))
	for childIndex := range children {
		child := &children[childIndex]
		eventID := strings.TrimSpace(child.EventID)
		if eventID == "" {
			return nil, fmt.Errorf("batch approval parent %s child %s has no event id", parentID, child.TicketID)
		}
		if previous, exists := eventOwner[eventID]; exists {
			return nil, fmt.Errorf("batch approval parent %s children %s and %s share event %s", parentID, previous, child.TicketID, eventID)
		}
		eventOwner[eventID] = child.TicketID
		eventIDs = append(eventIDs, eventID)
	}
	sort.Strings(eventIDs)
	rows, err := tx.Query(ctx, `
/* batch_approval_lock_child_events */
SELECT id, event_type, aggregate_type, aggregate_id, payload, status, created_by
FROM domain_events
WHERE id = ANY($1::text[])
ORDER BY id
FOR UPDATE
`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("lock batch approval parent %s child events: %w", parentID, err)
	}
	defer rows.Close()

	events := make(map[string]*lockedBatchApprovalEvent, len(eventIDs))
	for rows.Next() {
		event := &lockedBatchApprovalEvent{}
		if err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.AggregateType,
			&event.AggregateID,
			&event.Payload,
			&event.Status,
			&event.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan batch approval parent %s child event: %w", parentID, err)
		}
		events[event.EventID] = event
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch approval parent %s child events: %w", parentID, err)
	}
	return events, nil
}

func loadBatchApprovalParentIdentity(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	parentEventID string,
) (batchApprovalParentIdentity, error) {
	var parent batchApprovalParentIdentity
	err := tx.QueryRow(ctx, `
SELECT
  parent.operation_type,
  parent.status,
  parent.requester,
  parent.approver,
  parent.selected_cluster_id,
  parent.selected_storage_class,
  parent.modified_spec,
  event.event_type,
  event.aggregate_type,
  event.aggregate_id,
  event.status,
  event.created_by,
	  event.payload,
  batch.batch_type,
  batch.status,
	  batch.created_by,
	  batch.child_count,
	  batch.success_count,
	  batch.failed_count,
	  batch.pending_count
FROM tickets AS parent
JOIN domain_events AS event ON event.id = parent.event_id
JOIN batch_tickets AS batch ON batch.id = parent.id
WHERE parent.id = $1
  AND parent.event_id = $2
  AND parent.parent_ticket_id IS NULL
`, parentID, parentEventID).Scan(
		&parent.Operation,
		&parent.Status,
		&parent.Requester,
		&parent.Approver,
		&parent.SelectedCluster,
		&parent.SelectedStorage,
		&parent.ModifiedSpec,
		&parent.EventType,
		&parent.EventAggregate,
		&parent.EventAggregateID,
		&parent.EventStatus,
		&parent.EventCreatedBy,
		&parent.EventPayload,
		&parent.BatchType,
		&parent.BatchStatus,
		&parent.BatchCreatedBy,
		&parent.ChildCount,
		&parent.SuccessCount,
		&parent.FailedCount,
		&parent.PendingCount,
	)
	return parent, err
}

func batchApprovalExecutionFromLockedParent(parent batchApprovalParentIdentity) (domain.BatchApprovalExecutionOptions, error) {
	execution := domain.BatchApprovalExecutionOptions{
		ClusterID:    strings.TrimSpace(parent.SelectedCluster.String),
		StorageClass: strings.TrimSpace(parent.SelectedStorage.String),
	}
	if len(parent.ModifiedSpec) == 0 || bytes.Equal(bytes.TrimSpace(parent.ModifiedSpec), []byte("null")) {
		return normalizeBatchApprovalExecution(execution), nil
	}
	var modified map[string]json.RawMessage
	if err := json.Unmarshal(parent.ModifiedSpec, &modified); err != nil {
		return domain.BatchApprovalExecutionOptions{}, err
	}
	raw, ok := modified["batch_approval_execution"]
	if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return normalizeBatchApprovalExecution(execution), nil
	}
	if err := json.Unmarshal(raw, &execution); err != nil {
		return domain.BatchApprovalExecutionOptions{}, err
	}
	return normalizeBatchApprovalExecution(execution), nil
}

type batchApprovalFingerprintParent struct {
	ParentID         string `json:"parent_id"`
	ParentEventID    string `json:"parent_event_id"`
	Operation        string `json:"operation"`
	Requester        string `json:"requester"`
	Approver         string `json:"approver"`
	SelectedCluster  string `json:"selected_cluster"`
	SelectedStorage  string `json:"selected_storage"`
	ModifiedSpec     []byte `json:"modified_spec"`
	EventType        string `json:"event_type"`
	EventAggregate   string `json:"event_aggregate"`
	EventAggregateID string `json:"event_aggregate_id"`
	EventCreatedBy   string `json:"event_created_by"`
	EventPayload     []byte `json:"event_payload"`
	BatchType        string `json:"batch_type"`
	BatchCreatedBy   string `json:"batch_created_by"`
}

type batchApprovalFingerprintChild struct {
	TicketID      string `json:"ticket_id"`
	EventID       string `json:"event_id"`
	Operation     string `json:"operation"`
	Requester     string `json:"requester"`
	EventType     string `json:"event_type"`
	AggregateType string `json:"aggregate_type"`
	AggregateID   string `json:"aggregate_id"`
	Payload       []byte `json:"payload"`
	CreatedBy     string `json:"created_by"`
}

func batchApprovalGraphFingerprint(
	parentID string,
	parentEventID string,
	parent batchApprovalParentIdentity,
	children []lockedBatchApprovalChild,
	events map[string]*lockedBatchApprovalEvent,
) (string, error) {
	fingerprint := struct {
		Parent   batchApprovalFingerprintParent  `json:"parent"`
		Children []batchApprovalFingerprintChild `json:"children"`
	}{
		Parent: batchApprovalFingerprintParent{
			ParentID:         strings.TrimSpace(parentID),
			ParentEventID:    strings.TrimSpace(parentEventID),
			Operation:        strings.TrimSpace(parent.Operation),
			Requester:        strings.TrimSpace(parent.Requester),
			Approver:         strings.TrimSpace(parent.Approver.String),
			SelectedCluster:  strings.TrimSpace(parent.SelectedCluster.String),
			SelectedStorage:  strings.TrimSpace(parent.SelectedStorage.String),
			ModifiedSpec:     parent.ModifiedSpec,
			EventType:        strings.TrimSpace(parent.EventType),
			EventAggregate:   strings.TrimSpace(parent.EventAggregate),
			EventAggregateID: strings.TrimSpace(parent.EventAggregateID),
			EventCreatedBy:   strings.TrimSpace(parent.EventCreatedBy),
			EventPayload:     parent.EventPayload,
			BatchType:        strings.TrimSpace(parent.BatchType),
			BatchCreatedBy:   strings.TrimSpace(parent.BatchCreatedBy),
		},
		Children: make([]batchApprovalFingerprintChild, 0, len(children)),
	}
	for childIndex := range children {
		child := &children[childIndex]
		event := events[child.EventID]
		if event == nil {
			return "", fmt.Errorf("child %s event %s is missing", child.TicketID, child.EventID)
		}
		fingerprint.Children = append(fingerprint.Children, batchApprovalFingerprintChild{
			TicketID:      strings.TrimSpace(child.TicketID),
			EventID:       strings.TrimSpace(child.EventID),
			Operation:     strings.TrimSpace(child.Operation),
			Requester:     strings.TrimSpace(child.Requester),
			EventType:     strings.TrimSpace(event.EventType),
			AggregateType: strings.TrimSpace(event.AggregateType),
			AggregateID:   strings.TrimSpace(event.AggregateID),
			Payload:       event.Payload,
			CreatedBy:     strings.TrimSpace(event.CreatedBy),
		})
	}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func batchApprovalChildInputFingerprints(children []lockedBatchApprovalChild) (map[string]string, error) {
	fingerprints := make(map[string]string, len(children))
	for childIndex := range children {
		child := &children[childIndex]
		input := struct {
			TicketID            string `json:"ticket_id"`
			EventID             string `json:"event_id"`
			Approver            string `json:"approver"`
			Reason              string `json:"reason"`
			RejectReason        string `json:"reject_reason"`
			SelectedCluster     string `json:"selected_cluster"`
			SelectedStorage     string `json:"selected_storage"`
			TemplateSnapshot    []byte `json:"template_snapshot"`
			InstanceSnapshot    []byte `json:"instance_snapshot"`
			PlacementEvaluation []byte `json:"placement_evaluation"`
			ModifiedSpec        []byte `json:"modified_spec"`
			AttemptCount        int    `json:"attempt_count"`
		}{
			TicketID:            strings.TrimSpace(child.TicketID),
			EventID:             strings.TrimSpace(child.EventID),
			Approver:            strings.TrimSpace(child.Approver.String),
			Reason:              child.Reason.String,
			RejectReason:        child.RejectReason.String,
			SelectedCluster:     strings.TrimSpace(child.SelectedCluster.String),
			SelectedStorage:     strings.TrimSpace(child.SelectedStorage.String),
			TemplateSnapshot:    child.TemplateSnapshot,
			InstanceSnapshot:    child.InstanceSnapshot,
			PlacementEvaluation: child.PlacementEvaluation,
			ModifiedSpec:        child.ModifiedSpec,
			AttemptCount:        child.AttemptCount,
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(encoded)
		fingerprints[child.TicketID] = fmt.Sprintf("%x", digest[:])
	}
	return fingerprints, nil
}

func (w *ApprovalAtomicWriter) validateAndLockBatchApprovalChildDispatch(
	ctx context.Context,
	tx pgx.Tx,
	guard domain.BatchApprovalDispatchGuard,
	ticketID string,
	eventID string,
	approver string,
) error {
	guard.ParentTicketID = strings.TrimSpace(guard.ParentTicketID)
	guard.ParentEventID = strings.TrimSpace(guard.ParentEventID)
	guard.GraphFingerprint = strings.TrimSpace(guard.GraphFingerprint)
	ticketID = strings.TrimSpace(ticketID)
	eventID = strings.TrimSpace(eventID)
	if guard.ParentTicketID == "" || guard.ParentEventID == "" || guard.GraphFingerprint == "" ||
		ticketID == "" || eventID == "" {
		return fmt.Errorf("batch approval child dispatch guard is incomplete")
	}
	if strings.TrimSpace(approver) == "" || strings.TrimSpace(approver) != strings.TrimSpace(guard.Approver) {
		return fmt.Errorf("batch approval child dispatch approver does not match its validated parent")
	}
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		BatchMutationLockKey(guard.ParentTicketID),
	); err != nil {
		return fmt.Errorf("lock batch approval child parent %s: %w", guard.ParentTicketID, err)
	}
	fingerprint, _, childInputs, err := validateAndLockBatchApprovalIdentity(
		ctx,
		tx,
		guard.ParentTicketID,
		guard.ParentEventID,
		batchApprovalIdentityChildDispatch,
		[]batchApprovalSelectedChild{{TicketID: ticketID, EventID: eventID}},
	)
	if err != nil {
		return err
	}
	if fingerprint != guard.GraphFingerprint {
		return fmt.Errorf("batch approval dispatch graph changed after validation")
	}
	expectedChildInput := strings.TrimSpace(guard.ChildInputFingerprint[ticketID])
	currentChildInput := strings.TrimSpace(childInputs[ticketID])
	if expectedChildInput == "" || currentChildInput == "" || currentChildInput != expectedChildInput {
		return fmt.Errorf("batch approval dispatch child input changed after validation")
	}
	return nil
}

// FailBatchApprovalChildDispatch terminalizes one still-pending child while
// holding the same parent lock and exact graph guard used by successful child
// dispatch. No raw provider or infrastructure error text is accepted here.
func (w *ApprovalAtomicWriter) FailBatchApprovalChildDispatch(
	ctx context.Context,
	guard domain.BatchApprovalDispatchGuard,
	ticketID string,
	eventID string,
	approver string,
	publicReason string,
) error {
	if w == nil || w.pool == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if !batchApprovalDispatchPublicReasonAllowed(publicReason) {
		return fmt.Errorf("batch approval dispatch public failure reason is not allowlisted")
	}
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch approval child failure tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if validationErr := w.validateAndLockBatchApprovalChildDispatch(ctx, tx, guard, ticketID, eventID, approver); validationErr != nil {
		return fmt.Errorf("validate failed batch approval child dispatch: %w", validationErr)
	}
	command, err := tx.Exec(ctx, `
UPDATE tickets
SET status = 'FAILED',
    approver = $4,
    reject_reason = $5,
    attempt_count = CASE WHEN attempt_count = 0 THEN 1 ELSE attempt_count END,
    last_attempt_at = CASE WHEN attempt_count = 0 THEN NOW() ELSE last_attempt_at END,
    updated_at = NOW()
WHERE id = $1
  AND event_id = $2
  AND parent_ticket_id = $3
  AND status = 'PENDING'
  AND attempt_count >= 0
`, strings.TrimSpace(ticketID), strings.TrimSpace(eventID), strings.TrimSpace(guard.ParentTicketID), strings.TrimSpace(approver), publicReason)
	if err != nil {
		return fmt.Errorf("fail batch approval child ticket %s: %w", ticketID, err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("fail batch approval child ticket %s: expected 1 row, got %d", ticketID, command.RowsAffected())
	}
	command, err = tx.Exec(ctx, `
UPDATE domain_events
SET status = 'FAILED'
WHERE id = $1
  AND status = 'PENDING'
`, strings.TrimSpace(eventID))
	if err != nil {
		return fmt.Errorf("fail batch approval child event %s: %w", eventID, err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("fail batch approval child event %s: expected 1 row, got %d", eventID, command.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch approval child failure: %w", err)
	}
	return nil
}

func batchApprovalDispatchPublicReasonAllowed(reason string) bool {
	switch strings.TrimSpace(reason) {
	case domain.BatchApprovalDispatchFailureValidation,
		domain.BatchApprovalDispatchFailureUnsupported,
		domain.BatchApprovalDispatchFailureExhausted:
		return true
	default:
		return false
	}
}

func validateBatchApprovalParentIdentity(
	mode batchApprovalIdentityMode,
	parentID string,
	parentEventID string,
	parent batchApprovalParentIdentity,
) error {
	expectedEventType, expectedBatchType, ok := expectedBatchApprovalParentIdentity(parent.Operation)
	operationAllowed := ok
	if mode == batchApprovalIdentityGenericRetry {
		operationAllowed = operationAllowed && parent.Operation != "POWER"
	}
	if mode == batchApprovalIdentityPowerRetry {
		operationAllowed = operationAllowed && parent.Operation == "POWER"
	}
	identityMatches := operationAllowed &&
		strings.TrimSpace(parent.Requester) != "" &&
		parent.EventType == expectedEventType &&
		strings.TrimSpace(parent.EventAggregate) == "batch" &&
		strings.TrimSpace(parent.EventAggregateID) == parentID &&
		strings.TrimSpace(parent.EventCreatedBy) == strings.TrimSpace(parent.Requester) &&
		parent.BatchType == expectedBatchType &&
		strings.TrimSpace(parent.BatchCreatedBy) == strings.TrimSpace(parent.Requester)
	if !identityMatches {
		return batchApprovalParentIdentityError(mode, parentID, parentEventID, "ticket/event/projection identity is inconsistent")
	}
	if (mode == batchApprovalIdentityDispatch || mode == batchApprovalIdentityChildDispatch) &&
		strings.TrimSpace(parent.Approver.String) == "" {
		return batchApprovalParentIdentityError(mode, parentID, parentEventID, "approved parent has no approver")
	}
	switch mode {
	case batchApprovalIdentityClaim:
		if parent.Status != batchPowerEventStatusPending ||
			parent.EventStatus != batchPowerEventStatusPending ||
			parent.BatchStatus != batchPowerBatchStatusPending {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent is not pending")
		}
	case batchApprovalIdentityGenericRetry:
		if (parent.Status != "EXECUTING" && parent.Status != "FAILED") ||
			(parent.EventStatus != "PROCESSING" && parent.EventStatus != "FAILED") ||
			!batchApprovalRetryProjectionStatusMatches(parent.Status, parent.BatchStatus) {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent is not in generic retry state")
		}
	case batchApprovalIdentityPowerRetry:
		if (parent.Status != "EXECUTING" && parent.Status != "FAILED") ||
			(parent.EventStatus != "PROCESSING" && parent.EventStatus != "FAILED") ||
			!batchApprovalRetryProjectionStatusMatches(parent.Status, parent.BatchStatus) {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent is not in power retry state")
		}
	case batchApprovalIdentityDispatch:
		if !batchApprovalDispatchParentStateAllowed(parent.Status, parent.EventStatus, parent.BatchStatus) {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent state pair is invalid for dispatch")
		}
	case batchApprovalIdentityChildDispatch:
		if parent.Status != "EXECUTING" || parent.EventStatus != "PROCESSING" || parent.BatchStatus != "IN_PROGRESS" {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent is no longer dispatchable")
		}
	}
	return nil
}

func batchApprovalDispatchParentStateAllowed(parentStatus, eventStatus, projectionStatus string) bool {
	switch parentStatus {
	case "EXECUTING":
		return eventStatus == "PROCESSING" && projectionStatus == "IN_PROGRESS"
	case "SUCCESS":
		return eventStatus == "COMPLETED" && projectionStatus == "COMPLETED"
	case "FAILED":
		return eventStatus == "FAILED" && (projectionStatus == "FAILED" || projectionStatus == "PARTIAL_SUCCESS")
	case batchApprovalTicketStatusCancelled:
		return eventStatus == batchApprovalTicketStatusCancelled && projectionStatus == batchApprovalTicketStatusCancelled
	default:
		return false
	}
}

func batchApprovalRetryProjectionStatusMatches(parentStatus, projectionStatus string) bool {
	switch parentStatus {
	case "EXECUTING":
		return projectionStatus == "IN_PROGRESS"
	case "FAILED":
		return projectionStatus == "FAILED" || projectionStatus == "PARTIAL_SUCCESS"
	default:
		return false
	}
}

func expectedBatchApprovalParentIdentity(operation string) (eventType, projectionType string, ok bool) {
	switch strings.TrimSpace(operation) {
	case batchOperationCreate:
		return string(domain.EventBatchCreateRequested), batchProjectionTypeCreate, true
	case batchOperationModify:
		return string(domain.EventBatchModifyRequested), batchProjectionTypeModify, true
	case batchOperationDelete:
		return string(domain.EventBatchDeleteRequested), batchProjectionTypeDelete, true
	case "POWER":
		return string(domain.EventBatchPowerRequested), "BATCH_POWER", true
	default:
		return "", "", false
	}
}

func batchApprovalParentIdentityError(
	mode batchApprovalIdentityMode,
	parentID string,
	parentEventID string,
	detail string,
) error {
	if mode == batchApprovalIdentityClaim {
		return fmt.Errorf("claim batch approval parent %s/event %s: %s", parentID, parentEventID, detail)
	}
	if mode == batchApprovalIdentityDispatch || mode == batchApprovalIdentityChildDispatch {
		return &domain.BatchApprovalDispatchGraphInvalidError{Detail: fmt.Sprintf(
			"parent %s/event %s: %s",
			parentID,
			parentEventID,
			detail,
		)}
	}
	return &BatchRetryParentNotEligibleError{ParentTicketID: parentID, ParentEventID: parentEventID}
}

func batchApprovalChildIdentityError(
	mode batchApprovalIdentityMode,
	ticketID string,
	eventID string,
	detail string,
) error {
	if mode == batchApprovalIdentityClaim {
		return fmt.Errorf("claim batch approval child %s/event %s: %s", ticketID, eventID, detail)
	}
	if mode == batchApprovalIdentityPowerRetry {
		return &PowerRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
	}
	if mode == batchApprovalIdentityDispatch || mode == batchApprovalIdentityChildDispatch {
		return &domain.BatchApprovalDispatchGraphInvalidError{Detail: fmt.Sprintf(
			"child %s/event %s: %s",
			ticketID,
			eventID,
			detail,
		)}
	}
	return &BatchApprovalRetryNotEligibleError{TicketID: ticketID, EventID: eventID}
}

func batchApprovalRetryEventStatusAllowed(mode batchApprovalIdentityMode, status string) bool {
	if mode == batchApprovalIdentityPowerRetry {
		return status == batchPowerTicketStatusFailed || status == batchApprovalTicketStatusCancelled
	}
	return status == batchPowerEventStatusPending ||
		status == batchPowerTicketStatusFailed ||
		status == batchApprovalTicketStatusCancelled
}

func batchApprovalUnselectedRetryStateAllowed(
	mode batchApprovalIdentityMode,
	ticketStatus string,
	eventStatus string,
) bool {
	switch ticketStatus {
	case "SUCCESS":
		return eventStatus == "COMPLETED"
	case "FAILED":
		return batchApprovalRetryEventStatusAllowed(mode, eventStatus)
	case batchApprovalTicketStatusRejected, batchApprovalTicketStatusCancelled:
		return eventStatus == batchApprovalTicketStatusCancelled
	case batchApprovalTicketStatusApproved:
		if mode == batchApprovalIdentityPowerRetry {
			return eventStatus == "PENDING" || eventStatus == "PROCESSING"
		}
		return eventStatus == "PROCESSING"
	case "EXECUTING":
		return eventStatus == "PENDING" || eventStatus == "PROCESSING"
	default:
		// In particular, an unselected PENDING ticket is unsafe for a generic
		// retry because the parent-scoped dispatcher would dispatch it too.
		return false
	}
}

func batchApprovalDispatchChildStateAllowed(ticketStatus, eventStatus string) bool {
	switch ticketStatus {
	case "PENDING":
		return eventStatus == "PENDING"
	case batchApprovalTicketStatusApproved, batchPowerTicketStatusExecuting:
		return eventStatus == "PENDING" || eventStatus == "PROCESSING"
	case "SUCCESS":
		return eventStatus == "COMPLETED"
	case "FAILED":
		return eventStatus == batchPowerTicketStatusFailed || eventStatus == batchApprovalTicketStatusCancelled
	case batchApprovalTicketStatusRejected, batchApprovalTicketStatusCancelled:
		return eventStatus == batchApprovalTicketStatusCancelled
	default:
		return false
	}
}

func validateBatchApprovalProjectionCounts(
	mode batchApprovalIdentityMode,
	parentID string,
	parentEventID string,
	parent batchApprovalParentIdentity,
	children []lockedBatchApprovalChild,
) error {
	var successCount, failedCount, pendingCount int
	for childIndex := range children {
		child := &children[childIndex]
		switch child.Status {
		case "SUCCESS":
			successCount++
		case batchPowerTicketStatusFailed, batchApprovalTicketStatusRejected:
			failedCount++
		case batchApprovalTicketStatusCancelled:
			// Cancelled children are terminal but deliberately excluded from
			// all three summary buckets by the projection refresh queries.
		default:
			pendingCount++
		}
	}
	if parent.ChildCount != len(children) ||
		parent.SuccessCount != successCount ||
		parent.FailedCount != failedCount ||
		parent.PendingCount != pendingCount {
		return batchApprovalParentIdentityError(
			mode,
			parentID,
			parentEventID,
			"projection counters do not match the complete child graph",
		)
	}
	return nil
}

func validateBatchApprovalParentPayload(
	mode batchApprovalIdentityMode,
	parentID string,
	parentEventID string,
	parent batchApprovalParentIdentity,
	childItems []string,
	powerAction string,
) error {
	var payload domain.BatchVMRequestPayload
	if err := decodeBatchApprovalPayloadExact(parent.EventPayload, &payload); err != nil {
		return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent event payload is malformed")
	}
	if strings.TrimSpace(payload.SubmittedBy) == "" ||
		strings.TrimSpace(payload.SubmittedBy) != strings.TrimSpace(parent.Requester) ||
		len(payload.Items) != len(childItems) {
		return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent event payload does not match its ticket or child set")
	}

	expectedOperation := strings.TrimSpace(parent.Operation)
	if expectedOperation == "POWER" {
		if powerAction == "" {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "power batch has no common child action")
		}
		expectedOperation = "POWER_" + strings.ToUpper(powerAction)
	}
	if strings.ToUpper(strings.TrimSpace(payload.Operation)) != expectedOperation {
		return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent payload operation does not match the durable batch operation")
	}

	remaining := make(map[string]int, len(childItems))
	for _, itemKey := range childItems {
		remaining[itemKey]++
	}
	for itemIndex := range payload.Items {
		itemKey, err := batchApprovalItemKey(payload.Items[itemIndex])
		if err != nil || remaining[itemKey] == 0 {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent payload item set does not match the complete child graph")
		}
		remaining[itemKey]--
	}
	for _, count := range remaining {
		if count != 0 {
			return batchApprovalParentIdentityError(mode, parentID, parentEventID, "parent payload item set does not match the complete child graph")
		}
	}
	return nil
}

func normalizeBatchApprovalSelectedChildren(children []domain.BatchApprovalRetryChild) ([]batchApprovalSelectedChild, error) {
	selected := make([]batchApprovalSelectedChild, len(children))
	for childIndex := range children {
		selected[childIndex] = batchApprovalSelectedChild{
			TicketID: children[childIndex].TicketID,
			EventID:  children[childIndex].EventID,
		}
	}
	return normalizeBatchSelectedChildren(selected, "batch approval retry")
}

func normalizeBatchSelectedChildren(
	children []batchApprovalSelectedChild,
	subject string,
) ([]batchApprovalSelectedChild, error) {
	normalized := make([]batchApprovalSelectedChild, 0, len(children))
	byTicket := make(map[string]string, len(children))
	byEvent := make(map[string]string, len(children))
	for childIndex := range children {
		ticketID := strings.TrimSpace(children[childIndex].TicketID)
		eventID := strings.TrimSpace(children[childIndex].EventID)
		if ticketID == "" || eventID == "" {
			return nil, fmt.Errorf("%s child %d is incomplete", subject, childIndex)
		}
		if existingEvent, exists := byTicket[ticketID]; exists {
			if existingEvent != eventID {
				return nil, fmt.Errorf("%s ticket %s is bound to multiple requested events", subject, ticketID)
			}
			continue
		}
		if existingTicket, exists := byEvent[eventID]; exists {
			return nil, fmt.Errorf("%s event %s is requested by tickets %s and %s", subject, eventID, existingTicket, ticketID)
		}
		byTicket[ticketID] = eventID
		byEvent[eventID] = ticketID
		normalized = append(normalized, batchApprovalSelectedChild{TicketID: ticketID, EventID: eventID})
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].TicketID < normalized[j].TicketID
	})
	return normalized, nil
}

func batchApprovalChildGraphIdentityMatches(
	child lockedBatchApprovalChild,
	event lockedBatchApprovalEvent,
) (batchApprovalChildGraphIdentity, bool) {
	if strings.TrimSpace(event.AggregateType) != "vm" || strings.TrimSpace(event.AggregateID) == "" {
		return batchApprovalChildGraphIdentity{}, false
	}
	aggregateID := strings.TrimSpace(event.AggregateID)
	requester := strings.TrimSpace(child.Requester)
	var item domain.BatchVMItemPayload
	identity := batchApprovalChildGraphIdentity{Target: aggregateID}

	switch strings.TrimSpace(child.Operation) {
	case batchOperationCreate:
		if event.EventType != string(domain.EventVMCreationRequested) {
			return batchApprovalChildGraphIdentity{}, false
		}
		var decoded domain.VMCreationPayload
		if decodeBatchApprovalPayloadExact(event.Payload, &decoded) != nil ||
			strings.TrimSpace(decoded.ServiceID) != aggregateID ||
			strings.TrimSpace(decoded.RequesterID) != requester {
			return batchApprovalChildGraphIdentity{}, false
		}
		item = domain.BatchVMItemPayload{
			SystemID:         decoded.SystemID,
			SystemName:       decoded.SystemName,
			ServiceID:        decoded.ServiceID,
			ServiceName:      decoded.ServiceName,
			TemplateID:       decoded.TemplateID,
			TemplateName:     decoded.TemplateName,
			InstanceSizeID:   decoded.InstanceSizeID,
			InstanceSizeName: decoded.InstanceSizeName,
			Namespace:        decoded.Namespace,
			OwnerID:          firstNonBlank(decoded.OwnerID, decoded.RequesterID),
			OwnerDisplayName: decoded.OwnerDisplayName,
			OwnerUsername:    decoded.OwnerUsername,
			TargetCPUCores:   positiveBatchFloat(decoded.TargetCPUCores),
			TargetMemoryGi:   positiveBatchFloat(decoded.TargetMemoryGi),
			TargetDiskGB:     positiveBatchInt(decoded.TargetDiskGB),
		}
	case batchOperationModify:
		if event.EventType != string(domain.EventVMModifyRequested) {
			return batchApprovalChildGraphIdentity{}, false
		}
		var decoded domain.VMModifyPayload
		if decodeBatchApprovalPayloadExact(event.Payload, &decoded) != nil ||
			strings.TrimSpace(decoded.VMID) != aggregateID ||
			strings.TrimSpace(decoded.Actor) != requester {
			return batchApprovalChildGraphIdentity{}, false
		}
		item = domain.BatchVMItemPayload{
			VMID:               decoded.VMID,
			VMName:             decoded.VMName,
			SystemID:           decoded.SystemID,
			SystemName:         decoded.SystemName,
			ServiceID:          decoded.ServiceID,
			ServiceName:        decoded.ServiceName,
			TemplateID:         decoded.TemplateID,
			TemplateName:       decoded.TemplateName,
			InstanceSizeID:     decoded.InstanceSizeID,
			InstanceSizeName:   decoded.InstanceSizeName,
			Namespace:          decoded.Namespace,
			ClusterID:          decoded.ClusterID,
			ClusterName:        decoded.ClusterName,
			ClusterEnvironment: decoded.ClusterEnvironment,
			OwnerID:            decoded.OwnerID,
			OwnerDisplayName:   decoded.OwnerDisplayName,
			OwnerUsername:      decoded.OwnerUsername,
			RequestVMStatus:    decoded.RequestVMStatus,
			CurrentCPUCores:    decoded.CurrentCPUCores,
			CurrentMemoryGi:    decoded.CurrentMemoryGi,
			CurrentDiskGB:      decoded.CurrentDiskGB,
			TargetCPUCores:     decoded.TargetCPUCores,
			TargetMemoryGi:     decoded.TargetMemoryGi,
			TargetDiskGB:       decoded.TargetDiskGB,
		}
	case batchOperationDelete:
		if event.EventType != string(domain.EventVMDeletionRequested) {
			return batchApprovalChildGraphIdentity{}, false
		}
		var decoded domain.VMDeletePayload
		if decodeBatchApprovalPayloadExact(event.Payload, &decoded) != nil ||
			strings.TrimSpace(decoded.VMID) != aggregateID ||
			strings.TrimSpace(decoded.Actor) != requester {
			return batchApprovalChildGraphIdentity{}, false
		}
		item = domain.BatchVMItemPayload{
			VMID:               decoded.VMID,
			VMName:             decoded.VMName,
			SystemID:           decoded.SystemID,
			SystemName:         decoded.SystemName,
			ServiceID:          decoded.ServiceID,
			ServiceName:        decoded.ServiceName,
			TemplateID:         decoded.TemplateID,
			TemplateName:       decoded.TemplateName,
			InstanceSizeID:     decoded.InstanceSizeID,
			InstanceSizeName:   decoded.InstanceSizeName,
			Namespace:          decoded.Namespace,
			ClusterID:          decoded.ClusterID,
			ClusterName:        decoded.ClusterName,
			ClusterEnvironment: decoded.ClusterEnvironment,
			OwnerID:            decoded.OwnerID,
			OwnerDisplayName:   decoded.OwnerDisplayName,
			OwnerUsername:      decoded.OwnerUsername,
			RequestVMStatus:    decoded.RequestVMStatus,
			CurrentCPUCores:    decoded.CurrentCPUCores,
			CurrentMemoryGi:    decoded.CurrentMemoryGi,
			CurrentDiskGB:      decoded.CurrentDiskGB,
		}
	case "POWER":
		var decoded domain.VMPowerPayload
		if decodeBatchApprovalPayloadExact(event.Payload, &decoded) != nil ||
			strings.TrimSpace(decoded.VMID) != aggregateID ||
			strings.TrimSpace(decoded.Actor) != requester {
			return batchApprovalChildGraphIdentity{}, false
		}
		action := strings.ToLower(strings.TrimSpace(decoded.Operation))
		expectedEventType := ""
		switch action {
		case powerOperationStart:
			expectedEventType = string(domain.EventVMStartRequested)
		case powerOperationStop:
			expectedEventType = string(domain.EventVMStopRequested)
		case powerOperationRestart:
			expectedEventType = string(domain.EventVMRestartRequested)
		default:
			return batchApprovalChildGraphIdentity{}, false
		}
		if event.EventType != expectedEventType {
			return batchApprovalChildGraphIdentity{}, false
		}
		identity.PowerAction = action
		item = domain.BatchVMItemPayload{
			VMID:               decoded.VMID,
			VMName:             decoded.VMName,
			SystemID:           decoded.SystemID,
			SystemName:         decoded.SystemName,
			ServiceID:          decoded.ServiceID,
			ServiceName:        decoded.ServiceName,
			TemplateID:         decoded.TemplateID,
			TemplateName:       decoded.TemplateName,
			InstanceSizeID:     decoded.InstanceSizeID,
			InstanceSizeName:   decoded.InstanceSizeName,
			Namespace:          decoded.Namespace,
			ClusterID:          decoded.ClusterID,
			ClusterName:        decoded.ClusterName,
			ClusterEnvironment: decoded.ClusterEnvironment,
			OwnerID:            decoded.OwnerID,
			OwnerDisplayName:   decoded.OwnerDisplayName,
			OwnerUsername:      decoded.OwnerUsername,
			RequestVMStatus:    decoded.RequestVMStatus,
			CurrentCPUCores:    decoded.CurrentCPUCores,
			CurrentMemoryGi:    decoded.CurrentMemoryGi,
			CurrentDiskGB:      decoded.CurrentDiskGB,
			Operation:          action,
		}
	default:
		return batchApprovalChildGraphIdentity{}, false
	}

	itemKey, err := batchApprovalItemKey(item)
	if err != nil {
		return batchApprovalChildGraphIdentity{}, false
	}
	identity.ItemKey = itemKey
	return identity, true
}

func decodeBatchApprovalPayloadExact(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("payload contains multiple JSON values")
		}
		return err
	}
	return nil
}

func batchApprovalItemKey(item domain.BatchVMItemPayload) (string, error) {
	item.VMID = strings.TrimSpace(item.VMID)
	item.VMName = strings.TrimSpace(item.VMName)
	item.SystemID = strings.TrimSpace(item.SystemID)
	item.SystemName = strings.TrimSpace(item.SystemName)
	item.ServiceID = strings.TrimSpace(item.ServiceID)
	item.ServiceName = strings.TrimSpace(item.ServiceName)
	item.TemplateID = strings.TrimSpace(item.TemplateID)
	item.TemplateName = strings.TrimSpace(item.TemplateName)
	item.InstanceSizeID = strings.TrimSpace(item.InstanceSizeID)
	item.InstanceSizeName = strings.TrimSpace(item.InstanceSizeName)
	item.Namespace = strings.TrimSpace(item.Namespace)
	item.ClusterID = strings.TrimSpace(item.ClusterID)
	item.ClusterName = strings.TrimSpace(item.ClusterName)
	item.ClusterEnvironment = strings.TrimSpace(item.ClusterEnvironment)
	item.OwnerID = strings.TrimSpace(item.OwnerID)
	item.OwnerDisplayName = strings.TrimSpace(item.OwnerDisplayName)
	item.OwnerUsername = strings.TrimSpace(item.OwnerUsername)
	item.RequestVMStatus = strings.TrimSpace(item.RequestVMStatus)
	item.Operation = strings.ToLower(strings.TrimSpace(item.Operation))
	// Item reasons are presentation/audit input and are not present in every
	// child event type. All execution-relevant and target fields remain exact.
	item.Reason = ""
	encoded, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func positiveBatchFloat(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func positiveBatchInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func (w *ApprovalAtomicWriter) insertBatchApprovalDispatcher(ctx context.Context, tx pgx.Tx, parentID string) error {
	inserted, err := w.riverClient.InsertTx(ctx, tx, jobs.BatchApprovalDispatchArgs{
		BatchID: strings.TrimSpace(parentID),
	}, nil)
	if err != nil {
		return fmt.Errorf("enqueue batch approval dispatcher for parent %s: %w", parentID, err)
	}
	if inserted == nil || inserted.Job == nil {
		return fmt.Errorf("enqueue batch approval dispatcher for parent %s: River returned no job", parentID)
	}
	if inserted.UniqueSkippedAsDuplicate {
		return &BatchApprovalDispatchConflictError{
			ParentTicketID:   strings.TrimSpace(parentID),
			ExistingJobID:    inserted.Job.ID,
			ExistingJobState: string(inserted.Job.State),
		}
	}
	return nil
}

func (w *ApprovalAtomicWriter) classifyBatchApprovalRetryConflict(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	child domain.BatchApprovalRetryChild,
) error {
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
  AND operation_type <> 'POWER'
FOR UPDATE
	`, strings.TrimSpace(child.TicketID), strings.TrimSpace(child.EventID), strings.TrimSpace(parentID)).Scan(&status, &attemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return &BatchApprovalRetryNotEligibleError{TicketID: child.TicketID, EventID: child.EventID}
	}
	if err != nil {
		return fmt.Errorf("classify batch approval retry ticket %s: %w", child.TicketID, err)
	}
	if status != "FAILED" {
		return &BatchApprovalRetryNotEligibleError{TicketID: child.TicketID, EventID: child.EventID}
	}
	if attemptCount >= domain.BatchChildMaxAttempts {
		return &BatchChildAttemptsExhaustedError{
			TicketID:     child.TicketID,
			AttemptCount: attemptCount,
			MaxAttempts:  domain.BatchChildMaxAttempts,
		}
	}
	return &BatchApprovalRetryNotEligibleError{TicketID: child.TicketID, EventID: child.EventID}
}

func refreshBatchApprovalProjection(ctx context.Context, qtx *sqlcrepo.Queries, parentID string) error {
	rows, err := qtx.RefreshBatchApprovalProjectionForDispatch(ctx, strings.TrimSpace(parentID))
	if err != nil {
		return fmt.Errorf("refresh batch approval projection %s: %w", parentID, err)
	}
	if rows != 1 {
		return fmt.Errorf("refresh batch approval projection %s: expected 1 row, got %d", parentID, rows)
	}
	return nil
}

func validateBatchApprovalClaimInput(input domain.BatchApprovalClaimInput) error {
	if strings.TrimSpace(input.ParentTicketID) == "" ||
		strings.TrimSpace(input.ParentEventID) == "" ||
		strings.TrimSpace(input.Approver) == "" {
		return fmt.Errorf("batch approval claim input is incomplete")
	}
	return nil
}

func validateBatchApprovalRetryInput(input domain.BatchApprovalRetryInput) error {
	if strings.TrimSpace(input.ParentTicketID) == "" ||
		strings.TrimSpace(input.ParentEventID) == "" ||
		strings.TrimSpace(input.Approver) == "" ||
		len(input.Children) == 0 {
		return fmt.Errorf("batch approval retry input is incomplete")
	}
	for idx, child := range input.Children {
		if strings.TrimSpace(child.TicketID) == "" || strings.TrimSpace(child.EventID) == "" {
			return fmt.Errorf("batch approval retry child %d is incomplete", idx)
		}
	}
	return nil
}

func requiredText(value string) pgtype.Text {
	return pgtype.Text{String: strings.TrimSpace(value), Valid: true}
}

func normalizeBatchApprovalExecution(input domain.BatchApprovalExecutionOptions) domain.BatchApprovalExecutionOptions {
	input.ClusterID = strings.TrimSpace(input.ClusterID)
	input.StorageClass = strings.TrimSpace(input.StorageClass)
	input.DVVolumeMode = strings.TrimSpace(input.DVVolumeMode)
	if len(input.DVAccessModes) == 0 {
		input.DVAccessModes = nil
		return input
	}
	normalizedModes := make([]string, 0, len(input.DVAccessModes))
	for _, mode := range input.DVAccessModes {
		if trimmed := strings.TrimSpace(mode); trimmed != "" {
			normalizedModes = append(normalizedModes, trimmed)
		}
	}
	input.DVAccessModes = normalizedModes
	return input
}
