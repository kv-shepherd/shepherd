// Package jobs defines River Queue job types for async processing.
//
// ADR-0006: River Queue for async task execution.
// ADR-0009: Claim-check pattern — job carries only EventID.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/jobs
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/predicate"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const batchAggregateType = "batch"

type jobsTransactionRollbackError struct {
	cause       error
	rollbackErr error
}

func (e *jobsTransactionRollbackError) Error() string {
	return fmt.Sprintf("%v: rollback jobs transaction: %v", e.cause, e.rollbackErr)
}

func (e *jobsTransactionRollbackError) Unwrap() []error {
	return []error{e.cause, e.rollbackErr}
}

func withJobsTx(ctx context.Context, client *ent.Client, fn func(txClient *ent.Client) error) error {
	return withJobsEntTx(ctx, client, func(_ *ent.Tx, txClient *ent.Client) error {
		return fn(txClient)
	})
}

// withJobsEntTx is the transaction variant for jobs that must include raw SQL
// through the same Ent transaction. Most jobs should keep using withJobsTx.
func withJobsEntTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx, txClient *ent.Client) error) error {
	// Jobs that acquire advisory or row locks must re-read mutable state after
	// any wait. Pin READ COMMITTED so an operator-level REPEATABLE READ default
	// cannot preserve a snapshot taken before the lock was acquired.
	tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin jobs transaction: %w", err)
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()
	if err := fn(tx, tx.Client()); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return &jobsTransactionRollbackError{cause: err, rollbackErr: rerr}
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit jobs transaction: %w", err)
	}
	return nil
}

// persistProcessingEventAndExecutingTicketByEvent marks a VM lifecycle event
// as actively processing before the worker performs external side effects.
func persistProcessingEventAndExecutingTicketByEvent(ctx context.Context, client *ent.Client, eventID string) error {
	return persistProcessingEventAndMaybeExecutingTicketByEvent(ctx, client, eventID, true)
}

func persistProcessingEventAndMaybeExecutingTicketByEvent(ctx context.Context, client *ent.Client, eventID string, requireTicket bool) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusPROCESSING, domainevent.StatusPENDING, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClientMaybe(ctx, txClient, eventID, entticket.StatusEXECUTING, requireTicket); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func ticketStatusForTerminalDomainEvent(status domainevent.Status) (entticket.Status, bool) {
	switch status {
	case domainevent.StatusCOMPLETED:
		return entticket.StatusSUCCESS, true
	case domainevent.StatusFAILED:
		return entticket.StatusFAILED, true
	case domainevent.StatusCANCELLED:
		return entticket.StatusCANCELLED, true
	default:
		return "", false
	}
}

// updateTicketStatusByEvent updates the ticket status associated with a
// domain event and propagates persistence failures to callers that need a
// durable ticket transition before the River job can finish successfully.
func updateTicketStatusByEvent(ctx context.Context, client *ent.Client, eventID string, status entticket.Status) error {
	return updateTicketStatusByEventMaybe(ctx, client, eventID, status, true)
}

func updateTicketStatusByEventMaybe(ctx context.Context, client *ent.Client, eventID string, status entticket.Status, requireTicket bool) error {
	if client == nil || eventID == "" {
		return nil
	}
	if err := updateTicketStatusByEventWithClientMaybe(ctx, client, eventID, status, requireTicket); err != nil {
		return err
	}
	return syncParentBatchStatusByChildEvent(ctx, client, eventID)
}

func updateTicketStatusByEventWithClient(ctx context.Context, client *ent.Client, eventID string, status entticket.Status) error {
	return updateTicketStatusByEventWithClientMaybe(ctx, client, eventID, status, true)
}

func updateTicketStatusByEventWithClientMaybe(ctx context.Context, client *ent.Client, eventID string, status entticket.Status, requireTicket bool) error {
	if client == nil || eventID == "" {
		return nil
	}
	affected, err := client.Ticket.Update().
		Where(entticket.EventIDEQ(eventID)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		logger.Warn("failed to update ticket status by event",
			zap.String("event_id", eventID),
			zap.String("status", status.String()),
			zap.Error(err),
		)
		return err
	}
	if affected == 1 {
		return nil
	}
	if affected == 0 && !requireTicket {
		logger.Debug("ticket status update skipped because event has no ticket",
			zap.String("event_id", eventID),
			zap.String("status", status.String()),
		)
		return nil
	}
	if affected != 1 {
		err := fmt.Errorf("update ticket status by event %s to %s: expected 1 row, got %d", eventID, status, affected)
		logger.Warn("ticket status by event affected unexpected row count",
			zap.String("event_id", eventID),
			zap.String("status", status.String()),
			zap.Int("affected", affected),
		)
		return err
	}
	return nil
}

func persistCompletedEventAndTicketByEvent(ctx context.Context, client *ent.Client, eventID string) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusCOMPLETED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClient(ctx, txClient, eventID, entticket.StatusSUCCESS); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func persistCompletedEventTicketAndVMStatusByEvent(ctx context.Context, client *ent.Client, eventID, vmID string, vmStatus entvm.Status) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if vmID != "" {
			if _, err := txClient.VM.UpdateOneID(vmID).
				Where(entvm.StatusNEQ(entvm.StatusDELETING)).
				SetStatus(vmStatus).
				Save(ctx); err != nil {
				if ent.IsNotFound(err) {
					logger.Info("skipped completed event VM status persistence because row is delete-owned or absent",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
						zap.String("status", vmStatus.String()),
					)
				} else {
					logger.Warn("failed to update VM status during completed event persistence",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
						zap.String("status", vmStatus.String()),
						zap.Error(err),
					)
					return err
				}
			}
		}
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusCOMPLETED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClient(ctx, txClient, eventID, entticket.StatusSUCCESS); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func persistFailedEventTicketAndVMByEventUnlessDeleting(ctx context.Context, client *ent.Client, eventID, vmID string) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if vmID != "" {
			if _, err := txClient.VM.UpdateOneID(vmID).
				Where(entvm.StatusNEQ(entvm.StatusDELETING)).
				SetStatus(entvm.StatusFAILED).
				Save(ctx); err != nil {
				if ent.IsNotFound(err) {
					logger.Info("skipped failed event VM status persistence because row is delete-owned or absent",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
						zap.String("status", entvm.StatusFAILED.String()),
					)
				} else {
					logger.Warn("failed to update VM status during failed event persistence",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
						zap.String("status", entvm.StatusFAILED.String()),
						zap.Error(err),
					)
					return err
				}
			}
		}
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusFAILED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClient(ctx, txClient, eventID, entticket.StatusFAILED); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func persistFailedEventAndTicketByEvent(ctx context.Context, client *ent.Client, eventID string) error {
	return persistFailedEventAndTicketByEventWithRejectReason(ctx, client, eventID, "")
}

func persistFailedEventAndTicketByEventWithRejectReason(ctx context.Context, client *ent.Client, eventID, rejectReason string) error {
	return persistFailedEventTicketAndMaybeVMByEvent(ctx, client, eventID, rejectReason, "", "")
}

func persistFailedEventTicketAndVMByEvent(ctx context.Context, client *ent.Client, eventID, vmID string) error {
	return persistFailedEventTicketAndMaybeVMByEvent(ctx, client, eventID, "", vmID, entvm.StatusFAILED)
}

func persistFailedEventTicketAndMaybeVMByEvent(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	rejectReason string,
	vmID string,
	vmStatus entvm.Status,
) error {
	return persistFailedEventTicketAndMaybeVMByEventWithTicketRequirement(ctx, client, eventID, rejectReason, vmID, vmStatus, true)
}

func persistFailedEventTicketAndMaybeVMByEventWithTicketRequirement(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	rejectReason string,
	vmID string,
	vmStatus entvm.Status,
	requireTicket bool,
) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if vmID != "" {
			if _, err := txClient.VM.UpdateOneID(vmID).
				SetStatus(vmStatus).
				Save(ctx); err != nil {
				logger.Warn("failed to update VM status during failed event persistence",
					zap.String("event_id", eventID),
					zap.String("vm_id", vmID),
					zap.String("status", vmStatus.String()),
					zap.Error(err),
				)
				return err
			}
		}
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusFAILED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		ticketUpdate := txClient.Ticket.Update().
			Where(entticket.EventIDEQ(eventID)).
			SetStatus(entticket.StatusFAILED)
		if rejectReason != "" {
			ticketUpdate = ticketUpdate.SetRejectReason(rejectReason)
		}
		affected, err := ticketUpdate.Save(ctx)
		if err != nil {
			logger.Warn("failed to update ticket status by event",
				zap.String("event_id", eventID),
				zap.String("status", entticket.StatusFAILED.String()),
				zap.Error(err),
			)
			return err
		}
		if affected == 1 {
			return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
		}
		if affected == 0 && !requireTicket {
			logger.Debug("failed ticket status update skipped because event has no ticket",
				zap.String("event_id", eventID),
			)
			return nil
		}
		if affected != 1 {
			return fmt.Errorf("update failed ticket status by event %s: expected 1 row, got %d", eventID, affected)
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func updateDomainEventStatusWithExpected(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	next domainevent.Status,
	expected ...domainevent.Status,
) error {
	if client == nil || eventID == "" {
		return nil
	}
	updated, err := tryUpdateDomainEventStatusWithExpected(ctx, client, eventID, next, expected...)
	if err != nil {
		return err
	}
	if !updated {
		err := fmt.Errorf("update event %s from %v to %s: expected 1 row, got 0", eventID, domainEventStatusStrings(expected), next)
		logger.Warn("event status update affected unexpected row count",
			zap.String("event_id", eventID),
			zap.Strings("expected", domainEventStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Int("affected", 0),
		)
		return err
	}
	return nil
}

// tryUpdateDomainEventStatusWithExpected performs a single conditional update.
// A false result with a nil error means another transaction won the state
// transition. Callers that use the event status as a dispatch fence can handle
// that expected contention without turning it into a database failure.
func tryUpdateDomainEventStatusWithExpected(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	next domainevent.Status,
	expected ...domainevent.Status,
) (bool, error) {
	if client == nil || eventID == "" {
		return false, nil
	}
	if len(expected) == 0 {
		return false, fmt.Errorf("expected event status is required for event %s", eventID)
	}
	// Every production caller supplies the client of an active jobs transaction.
	// Lock the owning ticket before the event CAS so jobs, handlers, and
	// ticketing all use ticket -> event -> parent order. Direct events are valid
	// and therefore allow zero tickets; duplicate ticket ownership is corrupt
	// and fails closed after locking at most two rows.
	tickets, err := client.Ticket.Query().
		Where(
			entticket.EventIDEQ(eventID),
			lockTicketRowForUpdate(),
		).
		Order(entticket.ByID()).
		Limit(2).
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("lock ticket for event %s status transition: %w", eventID, err)
	}
	if len(tickets) > 1 {
		return false, fmt.Errorf("lock ticket for event %s status transition: expected at most 1 row, got %d", eventID, len(tickets))
	}
	predicates := []predicate.DomainEvent{
		domainevent.ID(eventID),
	}
	if len(expected) == 1 {
		predicates = append(predicates, domainevent.StatusEQ(expected[0]))
	} else {
		predicates = append(predicates, domainevent.StatusIn(expected...))
	}
	affected, err := client.DomainEvent.Update().
		Where(predicates...).
		SetStatus(next).
		Save(ctx)
	if err != nil {
		logger.Warn("failed to update event status",
			zap.String("event_id", eventID),
			zap.Strings("expected", domainEventStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Error(err),
		)
		return false, err
	}
	if affected > 1 {
		err := fmt.Errorf("update event %s from %v to %s: expected 1 row, got %d", eventID, domainEventStatusStrings(expected), next, affected)
		logger.Warn("event status update affected unexpected row count",
			zap.String("event_id", eventID),
			zap.Strings("expected", domainEventStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Int("affected", affected),
		)
		return false, err
	}
	return affected == 1, nil
}

func domainEventStatusStrings(statuses []domainevent.Status) []string {
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		values = append(values, status.String())
	}
	return values
}

// logAuditVMOp is a helper for writing VM operation audit log entries. Failures
// are logged at warn level but never propagated. Every worker in this package
// follows the same pattern so we centralise it here to avoid repetition.
func logAuditVMOp(ctx context.Context, auditLogger *audit.Logger, action, resourceID, actor, eventID string) {
	if auditLogger == nil {
		return
	}
	if err := auditLogger.LogVMOperation(ctx, action, resourceID, actor); err != nil {
		logger.Warn("failed to write audit log",
			zap.String("action", action),
			zap.String("event_id", eventID),
			zap.Error(err),
		)
	}
}

// SyncParentBatchStatusByChildEvent refreshes the parent batch aggregates for
// the parent ticket that owns the provided child event.
func SyncParentBatchStatusByChildEvent(ctx context.Context, client *ent.Client, childEventID string) error {
	return syncParentBatchStatusByChildEvent(ctx, client, childEventID)
}

func syncParentBatchStatusByChildEvent(ctx context.Context, client *ent.Client, childEventID string) error {
	if client == nil || childEventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, childEventID)
	})
}

func syncParentBatchStatusByChildEventWithClient(ctx context.Context, client *ent.Client, childEventID string) error {
	if client == nil || childEventID == "" {
		return nil
	}
	child, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(childEventID)).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			logger.Warn("failed to load child ticket for parent batch status sync",
				zap.String("event_id", childEventID),
				zap.Error(err),
			)
		}
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	parentID := child.ParentTicketID
	if parentID == "" {
		return nil
	}
	return syncParentBatchStatusWithClient(ctx, client, parentID, false)
}

// SyncParentBatchStatus recalculates parent batch ticket, event, and projection
// state from its child ticket statuses.
func SyncParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string) error {
	return syncParentBatchStatus(ctx, client, parentTicketID, false)
}

// SyncParentBatchStatusInTx recalculates the parent inside a caller-owned Ent
// transaction. Callers should invoke it after mutating child rows so child,
// parent event/ticket, and projection changes commit or roll back together.
func SyncParentBatchStatusInTx(ctx context.Context, tx *ent.Tx, parentTicketID string) error {
	if tx == nil {
		return fmt.Errorf("parent batch status sync transaction is required")
	}
	if strings.TrimSpace(parentTicketID) == "" {
		return nil
	}
	return syncParentBatchStatusWithClient(ctx, tx.Client(), strings.TrimSpace(parentTicketID), false)
}

// LockParentBatchTicketInTx acquires the reviewed parent row lock without
// mutating aggregate state. Recovery handlers use it to validate the exact
// parent/event/projection identity before changing an anomalous child.
func LockParentBatchTicketInTx(ctx context.Context, tx *ent.Tx, parentTicketID string) (*ent.Ticket, error) {
	if tx == nil {
		return nil, fmt.Errorf("parent batch ticket lock transaction is required")
	}
	parentTicketID = strings.TrimSpace(parentTicketID)
	if parentTicketID == "" {
		return nil, fmt.Errorf("parent batch ticket id is required")
	}
	parent, err := tx.Client().Ticket.Query().
		Where(entticket.ID(parentTicketID), lockTicketRowForUpdate()).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock parent batch ticket %s: %w", parentTicketID, err)
	}
	return parent, nil
}

// ReconcileFailedParentBatchStatus repairs the one state regression that can
// arise when an explicit retry races a stale parent aggregation: a FAILED
// parent with active children must return to EXECUTING. It does not reopen any
// other terminal state.
func ReconcileFailedParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string) error {
	return syncParentBatchStatus(ctx, client, parentTicketID, true)
}

func syncParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string, allowFailedReconcile bool) error {
	if client == nil || parentTicketID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		return syncParentBatchStatusWithClient(ctx, txClient, parentTicketID, allowFailedReconcile)
	})
}

func syncParentBatchStatusWithClient(ctx context.Context, client *ent.Client, parentTicketID string, allowFailedReconcile bool) error {
	if client == nil || parentTicketID == "" {
		return nil
	}
	// Lock the parent row before reading any child status. Explicit retry resets
	// children before updating this same parent row, so FOR UPDATE makes a stale
	// aggregator wait for that transaction and then re-read its committed child
	// state. A parent advisory lock here would invert retry's parent->child lock
	// order because worker transactions already hold their child row.
	parent, err := client.Ticket.Query().
		Where(
			entticket.ID(parentTicketID),
			lockTicketRowForUpdate(),
		).
		Only(ctx)
	if err != nil {
		logger.Warn("failed to load parent batch ticket for status sync",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
		return err
	}
	parentEvent, projection, err := loadExactParentBatchIdentity(ctx, client, parent)
	if err != nil {
		return err
	}
	children, err := loadExactParentBatchChildren(ctx, client, parent)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return nil
	}

	var (
		successCount   int
		failedCount    int
		cancelledCount int
		activeCount    int
	)
	for _, child := range children {
		switch child.Status {
		case entticket.StatusSUCCESS:
			successCount++
		case entticket.StatusFAILED, entticket.StatusREJECTED:
			failedCount++
		case entticket.StatusCANCELLED:
			cancelledCount++
		default:
			activeCount++
		}
	}

	parentStatus := entticket.StatusEXECUTING
	projectionStatus := entbatchticket.StatusIN_PROGRESS
	switch {
	case activeCount > 0:
	case successCount == len(children):
		parentStatus = entticket.StatusSUCCESS
		projectionStatus = entbatchticket.StatusCOMPLETED
	case cancelledCount == len(children):
		parentStatus = entticket.StatusCANCELLED
		projectionStatus = entbatchticket.StatusCANCELLED
	case successCount > 0 && (failedCount+cancelledCount) > 0:
		parentStatus = entticket.StatusFAILED
		projectionStatus = entbatchticket.StatusPARTIAL_SUCCESS
	default:
		// Includes terminal mixed outcomes (PARTIAL_SUCCESS in API view).
		parentStatus = entticket.StatusFAILED
		projectionStatus = entbatchticket.StatusFAILED
	}

	eventStatus := domainevent.StatusPROCESSING
	switch parentStatus {
	case entticket.StatusSUCCESS:
		eventStatus = domainevent.StatusCOMPLETED
	case entticket.StatusFAILED:
		eventStatus = domainevent.StatusFAILED
	case entticket.StatusCANCELLED:
		eventStatus = domainevent.StatusCANCELLED
	default:
	}

	if updateTicketErr := updateParentBatchTicketStatusWithExpected(
		ctx,
		client,
		parent,
		parentStatus,
		expectedParentBatchTicketStatuses(parentStatus, allowFailedReconcile)...,
	); updateTicketErr != nil {
		return updateTicketErr
	}

	if updateEventErr := updateParentBatchEventStatusWithExpected(
		ctx,
		client,
		parent,
		parentEvent,
		eventStatus,
		expectedParentBatchEventStatuses(eventStatus, allowFailedReconcile)...,
	); updateEventErr != nil {
		return updateEventErr
	}

	projectionRows, err := client.BatchTicket.Update().
		Where(
			entbatchticket.ID(parent.ID),
			entbatchticket.BatchTypeEQ(projection.BatchType),
			entbatchticket.CreatedByEQ(projection.CreatedBy),
		).
		SetChildCount(len(children)).
		SetSuccessCount(successCount).
		SetFailedCount(failedCount).
		SetPendingCount(activeCount).
		SetStatus(projectionStatus).
		Save(ctx)
	if err != nil {
		logger.Warn("failed to update batch projection counters",
			zap.String("parent_ticket_id", parentTicketID),
			zap.String("status", projectionStatus.String()),
			zap.Error(err),
		)
		return err
	}
	if projectionRows != 1 {
		return fmt.Errorf("update batch projection %s with exact identity: expected 1 row, got %d", parent.ID, projectionRows)
	}
	return nil
}

func loadExactParentBatchIdentity(
	ctx context.Context,
	client *ent.Client,
	parent *ent.Ticket,
) (*ent.DomainEvent, *ent.BatchTicket, error) {
	if client == nil || parent == nil {
		return nil, nil, fmt.Errorf("parent batch identity requires a client and parent ticket")
	}
	if strings.TrimSpace(parent.ParentTicketID) != "" {
		return nil, nil, fmt.Errorf("parent batch ticket %s is itself a child ticket", parent.ID)
	}
	expectedEventType, expectedBatchType, ok := expectedParentBatchIdentity(parent.OperationType)
	if !ok {
		return nil, nil, fmt.Errorf("parent batch ticket %s has unsupported operation type %s", parent.ID, parent.OperationType)
	}
	parentEvent, err := client.DomainEvent.Get(ctx, parent.EventID)
	if err != nil {
		return nil, nil, fmt.Errorf("load parent batch event %s: %w", parent.EventID, err)
	}
	projection, err := client.BatchTicket.Get(ctx, parent.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load parent batch projection %s: %w", parent.ID, err)
	}
	if strings.TrimSpace(parent.EventID) == "" ||
		strings.TrimSpace(parent.EventID) != strings.TrimSpace(parentEvent.ID) ||
		parentEvent.EventType != expectedEventType ||
		strings.TrimSpace(parentEvent.AggregateType) != batchAggregateType ||
		strings.TrimSpace(parentEvent.AggregateID) != strings.TrimSpace(parent.ID) ||
		projection.BatchType != expectedBatchType ||
		strings.TrimSpace(parent.Requester) == "" ||
		strings.TrimSpace(parentEvent.CreatedBy) != strings.TrimSpace(parent.Requester) ||
		strings.TrimSpace(projection.CreatedBy) != strings.TrimSpace(parent.Requester) {
		return nil, nil, fmt.Errorf("parent batch %s ticket/event/projection identity is inconsistent", parent.ID)
	}
	return parentEvent, projection, nil
}

// ValidateParentBatchChildrenInTx locks and validates the current child set
// using a caller-owned transaction. It locks child tickets by ID, then their
// events by ID, and deliberately does not lock the parent. Decision paths must
// mutate the returned tickets without querying them again so the validated
// rows remain the decision write set. The caller owns any parent-level
// serialization needed to prevent a new child from being inserted.
func ValidateParentBatchChildrenInTx(
	ctx context.Context,
	tx *ent.Tx,
	parent *ent.Ticket,
) ([]*ent.Ticket, error) {
	if tx == nil {
		return nil, fmt.Errorf("parent batch child validation transaction is required")
	}
	if parent == nil || strings.TrimSpace(parent.ID) == "" {
		return nil, fmt.Errorf("parent batch child validation requires a parent ticket")
	}

	children, err := tx.Client().Ticket.Query().
		Where(
			entticket.ParentTicketIDEQ(parent.ID),
			lockTicketRowForUpdate(),
		).
		Order(entticket.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock parent batch %s child tickets: %w", parent.ID, err)
	}
	if len(children) == 0 {
		return children, nil
	}

	eventIDs := make([]string, 0, len(children))
	seenEventIDs := make(map[string]string, len(children))
	for _, child := range children {
		if child == nil || strings.TrimSpace(child.ID) == "" || strings.TrimSpace(child.EventID) == "" {
			return nil, fmt.Errorf("parent batch %s has a child with incomplete ticket identity", parent.ID)
		}
		if previousTicketID, duplicate := seenEventIDs[child.EventID]; duplicate {
			return nil, fmt.Errorf(
				"parent batch %s child tickets %s and %s share event %s",
				parent.ID,
				previousTicketID,
				child.ID,
				child.EventID,
			)
		}
		seenEventIDs[child.EventID] = child.ID
		eventIDs = append(eventIDs, child.EventID)
	}
	sort.Strings(eventIDs)

	events, err := tx.Client().DomainEvent.Query().
		Where(
			domainevent.IDIn(eventIDs...),
			lockDomainEventRowForUpdate(),
		).
		Order(domainevent.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock parent batch %s child events: %w", parent.ID, err)
	}
	eventsByID := make(map[string]*ent.DomainEvent, len(events))
	for _, event := range events {
		if event != nil {
			eventsByID[event.ID] = event
		}
	}
	if len(eventsByID) != len(eventIDs) {
		return nil, incompleteParentBatchChildEventsError(parent.ID, eventIDs, eventsByID)
	}
	for _, child := range children {
		if !exactParentBatchChildIdentityMatches(parent, child, eventsByID[child.EventID]) {
			return nil, fmt.Errorf(
				"parent batch %s child ticket %s/event %s identity is inconsistent",
				parent.ID,
				child.ID,
				child.EventID,
			)
		}
	}
	return children, nil
}

// loadExactParentBatchChildren validates every child ticket and event before
// parent aggregation writes begin. Events are loaded in one query so large
// batches do not turn identity validation into an N+1 read path.
func loadExactParentBatchChildren(
	ctx context.Context,
	client *ent.Client,
	parent *ent.Ticket,
) ([]*ent.Ticket, error) {
	if client == nil || parent == nil {
		return nil, fmt.Errorf("parent batch children require a client and parent ticket")
	}
	parentID := strings.TrimSpace(parent.ID)
	if parentID == "" {
		return nil, fmt.Errorf("parent batch child identity requires a parent ticket id")
	}
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to query child tickets for parent batch status sync",
			zap.String("parent_ticket_id", parent.ID),
			zap.Error(err),
		)
		return nil, err
	}
	if len(children) == 0 {
		return children, nil
	}

	eventIDs := make([]string, 0, len(children))
	seenEventIDs := make(map[string]string, len(children))
	for _, child := range children {
		if child == nil || strings.TrimSpace(child.ID) == "" || strings.TrimSpace(child.EventID) == "" {
			return nil, fmt.Errorf("parent batch %s has a child with incomplete ticket identity", parent.ID)
		}
		if previousTicketID, duplicate := seenEventIDs[child.EventID]; duplicate {
			return nil, fmt.Errorf(
				"parent batch %s child tickets %s and %s share event %s",
				parent.ID,
				previousTicketID,
				child.ID,
				child.EventID,
			)
		}
		seenEventIDs[child.EventID] = child.ID
		eventIDs = append(eventIDs, child.EventID)
	}

	events, err := client.DomainEvent.Query().
		Where(domainevent.IDIn(eventIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load parent batch %s child events: %w", parent.ID, err)
	}
	eventsByID := make(map[string]*ent.DomainEvent, len(events))
	for _, event := range events {
		if event != nil {
			eventsByID[event.ID] = event
		}
	}
	if len(eventsByID) != len(eventIDs) {
		return nil, incompleteParentBatchChildEventsError(parent.ID, eventIDs, eventsByID)
	}

	for _, child := range children {
		event := eventsByID[child.EventID]
		if !exactParentBatchChildIdentityMatches(parent, child, event) {
			return nil, fmt.Errorf(
				"parent batch %s child ticket %s/event %s identity is inconsistent",
				parent.ID,
				child.ID,
				child.EventID,
			)
		}
	}
	return children, nil
}

func incompleteParentBatchChildEventsError(
	parentID string,
	eventIDs []string,
	eventsByID map[string]*ent.DomainEvent,
) error {
	missingEventID := ""
	for _, eventID := range eventIDs {
		if _, found := eventsByID[eventID]; !found {
			missingEventID = eventID
			break
		}
	}

	return fmt.Errorf(
		"parent batch %s child event %s: expected 1 row, got 0; child event set is incomplete: expected %d events, got %d",
		parentID,
		missingEventID,
		len(eventIDs),
		len(eventsByID),
	)
}

func exactParentBatchChildIdentityMatches(parent, child *ent.Ticket, event *ent.DomainEvent) bool {
	if parent == nil || child == nil || event == nil ||
		strings.TrimSpace(child.ParentTicketID) != strings.TrimSpace(parent.ID) ||
		strings.TrimSpace(child.EventID) != strings.TrimSpace(event.ID) ||
		child.OperationType != parent.OperationType ||
		strings.TrimSpace(child.Requester) == "" ||
		strings.TrimSpace(child.Requester) != strings.TrimSpace(parent.Requester) ||
		strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(child.Requester) ||
		strings.TrimSpace(event.AggregateType) != "vm" ||
		strings.TrimSpace(event.AggregateID) == "" {
		return false
	}

	targetID := strings.TrimSpace(event.AggregateID)
	switch child.OperationType {
	case entticket.OperationTypeCREATE:
		if event.EventType != string(domain.EventVMCreationRequested) {
			return false
		}
		var payload domain.VMCreationPayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.ServiceID) == targetID
	case entticket.OperationTypeMODIFY:
		if event.EventType != string(domain.EventVMModifyRequested) {
			return false
		}
		var payload domain.VMModifyPayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.VMID) == targetID
	case entticket.OperationTypeDELETE:
		if event.EventType != string(domain.EventVMDeletionRequested) {
			return false
		}
		var payload domain.VMDeletePayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.VMID) == targetID
	case entticket.OperationTypePOWER:
		var payload domain.VMPowerPayload
		if json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.VMID) != targetID {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(payload.Operation)) {
		case "start":
			return event.EventType == string(domain.EventVMStartRequested)
		case "stop":
			return event.EventType == string(domain.EventVMStopRequested)
		case "restart":
			return event.EventType == string(domain.EventVMRestartRequested)
		default:
			return false
		}
	default:
		return false
	}
}

func expectedParentBatchIdentity(operation entticket.OperationType) (string, entbatchticket.BatchType, bool) {
	switch operation {
	case entticket.OperationTypeCREATE:
		return string(domain.EventBatchCreateRequested), entbatchticket.BatchTypeBATCH_CREATE, true
	case entticket.OperationTypeMODIFY:
		return string(domain.EventBatchModifyRequested), entbatchticket.BatchTypeBATCH_MODIFY, true
	case entticket.OperationTypeDELETE:
		return string(domain.EventBatchDeleteRequested), entbatchticket.BatchTypeBATCH_DELETE, true
	case entticket.OperationTypePOWER:
		return string(domain.EventBatchPowerRequested), entbatchticket.BatchTypeBATCH_POWER, true
	default:
		return "", "", false
	}
}

func updateParentBatchTicketStatusWithExpected(
	ctx context.Context,
	client *ent.Client,
	parent *ent.Ticket,
	next entticket.Status,
	expected ...entticket.Status,
) error {
	if client == nil || parent == nil || parent.ID == "" {
		return nil
	}
	parentTicketID := parent.ID
	if len(expected) == 0 {
		return fmt.Errorf("expected parent batch ticket status is required for ticket %s", parentTicketID)
	}
	update := client.Ticket.Update().
		Where(
			entticket.ID(parentTicketID),
			entticket.EventIDEQ(parent.EventID),
			entticket.OperationTypeEQ(parent.OperationType),
			entticket.RequesterEQ(parent.Requester),
			entticket.ParentTicketIDIsNil(),
		).
		SetStatus(next)
	if len(expected) == 1 {
		update = update.Where(entticket.StatusEQ(expected[0]))
	} else {
		update = update.Where(entticket.StatusIn(expected...))
	}
	affected, err := update.Save(ctx)
	if err != nil {
		logger.Warn("failed to update parent batch ticket status",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Strings("expected", ticketStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Error(err),
		)
		return err
	}
	if affected != 1 {
		rowErr := fmt.Errorf("update parent batch ticket %s from %v to %s with exact identity: expected 1 row, got %d", parentTicketID, ticketStatusStrings(expected), next, affected)
		logger.Warn("parent batch ticket status update affected unexpected row count",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Strings("expected", ticketStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Int("affected", affected),
		)
		return rowErr
	}
	return nil
}

func updateParentBatchEventStatusWithExpected(
	ctx context.Context,
	client *ent.Client,
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	next domainevent.Status,
	expected ...domainevent.Status,
) error {
	if client == nil || parent == nil || parentEvent == nil {
		return fmt.Errorf("parent batch event identity is required")
	}
	if len(expected) == 0 {
		return fmt.Errorf("expected parent batch event status is required for event %s", parentEvent.ID)
	}
	update := client.DomainEvent.Update().
		Where(
			domainevent.ID(parentEvent.ID),
			domainevent.EventTypeEQ(parentEvent.EventType),
			domainevent.AggregateTypeEQ(batchAggregateType),
			domainevent.AggregateIDEQ(parent.ID),
			domainevent.CreatedByEQ(parentEvent.CreatedBy),
		).
		SetStatus(next)
	if len(expected) == 1 {
		update = update.Where(domainevent.StatusEQ(expected[0]))
	} else {
		update = update.Where(domainevent.StatusIn(expected...))
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("update parent batch event %s: %w", parentEvent.ID, err)
	}
	if affected != 1 {
		return fmt.Errorf("update parent batch event %s from %v to %s with exact identity: expected 1 row, got %d", parentEvent.ID, domainEventStatusStrings(expected), next, affected)
	}
	return nil
}

func expectedParentBatchTicketStatuses(next entticket.Status, allowFailedReconcile bool) []entticket.Status {
	switch next {
	case entticket.StatusEXECUTING:
		if allowFailedReconcile {
			return []entticket.Status{
				entticket.StatusEXECUTING,
				entticket.StatusFAILED,
			}
		}
		return []entticket.Status{entticket.StatusEXECUTING}
	case entticket.StatusSUCCESS:
		return []entticket.Status{entticket.StatusEXECUTING, entticket.StatusSUCCESS}
	case entticket.StatusFAILED:
		return []entticket.Status{entticket.StatusEXECUTING, entticket.StatusFAILED}
	case entticket.StatusCANCELLED:
		return []entticket.Status{entticket.StatusPENDING, entticket.StatusEXECUTING, entticket.StatusCANCELLED}
	default:
		return []entticket.Status{next}
	}
}

func expectedParentBatchEventStatuses(next domainevent.Status, allowFailedReconcile bool) []domainevent.Status {
	switch next {
	case domainevent.StatusPROCESSING:
		if allowFailedReconcile {
			return []domainevent.Status{
				domainevent.StatusPROCESSING,
				domainevent.StatusFAILED,
			}
		}
		return []domainevent.Status{domainevent.StatusPROCESSING}
	case domainevent.StatusCOMPLETED:
		return []domainevent.Status{domainevent.StatusPROCESSING, domainevent.StatusCOMPLETED}
	case domainevent.StatusFAILED:
		return []domainevent.Status{domainevent.StatusPROCESSING, domainevent.StatusFAILED}
	case domainevent.StatusCANCELLED:
		return []domainevent.Status{domainevent.StatusPENDING, domainevent.StatusPROCESSING, domainevent.StatusCANCELLED}
	default:
		return []domainevent.Status{next}
	}
}

func ticketStatusStrings(statuses []entticket.Status) []string {
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		values = append(values, status.String())
	}
	return values
}
