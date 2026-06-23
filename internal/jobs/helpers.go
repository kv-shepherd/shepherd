// Package jobs defines River Queue job types for async processing.
//
// ADR-0006: River Queue for async task execution.
// ADR-0009: Claim-check pattern — job carries only EventID.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/jobs
package jobs

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/predicate"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func withJobsTx(ctx context.Context, client *ent.Client, fn func(txClient *ent.Client) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin jobs transaction: %w", err)
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()
	if err := fn(tx.Client()); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("%w: rollback jobs transaction: %w", err, rerr)
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

func persistFailedEventAndMaybeTicketByEvent(ctx context.Context, client *ent.Client, eventID string, requireTicket bool) error {
	return persistFailedEventTicketAndMaybeVMByEventWithTicketRequirement(ctx, client, eventID, "", "", "", requireTicket)
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

func eventHasTicket(ctx context.Context, client *ent.Client, eventID string) (bool, error) {
	if client == nil || eventID == "" {
		return false, nil
	}
	count, err := client.Ticket.Query().
		Where(entticket.EventIDEQ(eventID)).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
	if len(expected) == 0 {
		return fmt.Errorf("expected event status is required for event %s", eventID)
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
		return err
	}
	if affected != 1 {
		err := fmt.Errorf("update event %s from %v to %s: expected 1 row, got %d", eventID, domainEventStatusStrings(expected), next, affected)
		logger.Warn("event status update affected unexpected row count",
			zap.String("event_id", eventID),
			zap.Strings("expected", domainEventStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Int("affected", affected),
		)
		return err
	}
	return nil
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

// SyncParentBatchStatusAllowReopen recalculates a parent batch after an explicit
// retry action. Unlike ordinary worker-driven sync, retry may intentionally
// reopen a failed/cancelled parent or recompute it back to a terminal state when
// no child could be retried.
func SyncParentBatchStatusAllowReopen(ctx context.Context, client *ent.Client, parentTicketID string) error {
	return syncParentBatchStatus(ctx, client, parentTicketID, true)
}

func syncParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string, allowReopen bool) error {
	if client == nil || parentTicketID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		return syncParentBatchStatusWithClient(ctx, txClient, parentTicketID, allowReopen)
	})
}

func syncParentBatchStatusWithClient(ctx context.Context, client *ent.Client, parentTicketID string, allowReopen bool) error {
	if client == nil || parentTicketID == "" {
		return nil
	}
	parent, err := client.Ticket.Get(ctx, parentTicketID)
	if err != nil {
		logger.Warn("failed to load parent batch ticket for status sync",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
		return err
	}
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parentTicketID)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to query child tickets for parent batch status sync",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
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

	if err := updateParentBatchTicketStatusWithExpected(
		ctx,
		client,
		parentTicketID,
		parentStatus,
		expectedParentBatchTicketStatuses(parentStatus, allowReopen)...,
	); err != nil {
		return err
	}

	if err := updateDomainEventStatusWithExpected(
		ctx,
		client,
		parent.EventID,
		eventStatus,
		expectedParentBatchEventStatuses(eventStatus, allowReopen)...,
	); err != nil {
		return err
	}

	if _, err := client.BatchTicket.UpdateOneID(parentTicketID).
		SetChildCount(len(children)).
		SetSuccessCount(successCount).
		SetFailedCount(failedCount).
		SetPendingCount(activeCount).
		SetStatus(projectionStatus).
		Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		logger.Warn("failed to update batch projection counters",
			zap.String("parent_ticket_id", parentTicketID),
			zap.String("status", projectionStatus.String()),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func updateParentBatchTicketStatusWithExpected(
	ctx context.Context,
	client *ent.Client,
	parentTicketID string,
	next entticket.Status,
	expected ...entticket.Status,
) error {
	if client == nil || parentTicketID == "" {
		return nil
	}
	if len(expected) == 0 {
		return fmt.Errorf("expected parent batch ticket status is required for ticket %s", parentTicketID)
	}
	update := client.Ticket.UpdateOneID(parentTicketID).SetStatus(next)
	if len(expected) == 1 {
		update = update.Where(entticket.StatusEQ(expected[0]))
	} else {
		update = update.Where(entticket.StatusIn(expected...))
	}
	_, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			rowErr := fmt.Errorf("update parent batch ticket %s from %v to %s: expected 1 row, got 0", parentTicketID, ticketStatusStrings(expected), next)
			logger.Warn("parent batch ticket status update affected unexpected row count",
				zap.String("parent_ticket_id", parentTicketID),
				zap.Strings("expected", ticketStatusStrings(expected)),
				zap.String("status", next.String()),
				zap.Int("affected", 0),
			)
			return rowErr
		}
		logger.Warn("failed to update parent batch ticket status",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Strings("expected", ticketStatusStrings(expected)),
			zap.String("status", next.String()),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func expectedParentBatchTicketStatuses(next entticket.Status, allowReopen bool) []entticket.Status {
	switch next {
	case entticket.StatusEXECUTING:
		if allowReopen {
			return []entticket.Status{
				entticket.StatusPENDING,
				entticket.StatusEXECUTING,
				entticket.StatusFAILED,
				entticket.StatusREJECTED,
				entticket.StatusCANCELLED,
			}
		}
		return []entticket.Status{entticket.StatusEXECUTING}
	case entticket.StatusSUCCESS:
		return []entticket.Status{entticket.StatusEXECUTING, entticket.StatusSUCCESS}
	case entticket.StatusFAILED:
		if allowReopen {
			return []entticket.Status{
				entticket.StatusPENDING,
				entticket.StatusEXECUTING,
				entticket.StatusFAILED,
				entticket.StatusREJECTED,
				entticket.StatusCANCELLED,
			}
		}
		return []entticket.Status{entticket.StatusEXECUTING, entticket.StatusFAILED}
	case entticket.StatusCANCELLED:
		if allowReopen {
			return []entticket.Status{
				entticket.StatusPENDING,
				entticket.StatusEXECUTING,
				entticket.StatusFAILED,
				entticket.StatusREJECTED,
				entticket.StatusCANCELLED,
			}
		}
		return []entticket.Status{entticket.StatusPENDING, entticket.StatusEXECUTING, entticket.StatusCANCELLED}
	default:
		return []entticket.Status{next}
	}
}

func expectedParentBatchEventStatuses(next domainevent.Status, allowReopen bool) []domainevent.Status {
	switch next {
	case domainevent.StatusPROCESSING:
		if allowReopen {
			return []domainevent.Status{
				domainevent.StatusPENDING,
				domainevent.StatusPROCESSING,
				domainevent.StatusFAILED,
				domainevent.StatusCANCELLED,
			}
		}
		return []domainevent.Status{domainevent.StatusPROCESSING}
	case domainevent.StatusCOMPLETED:
		return []domainevent.Status{domainevent.StatusPROCESSING, domainevent.StatusCOMPLETED}
	case domainevent.StatusFAILED:
		if allowReopen {
			return []domainevent.Status{
				domainevent.StatusPENDING,
				domainevent.StatusPROCESSING,
				domainevent.StatusFAILED,
				domainevent.StatusCANCELLED,
			}
		}
		return []domainevent.Status{domainevent.StatusPROCESSING, domainevent.StatusFAILED}
	case domainevent.StatusCANCELLED:
		if allowReopen {
			return []domainevent.Status{
				domainevent.StatusPENDING,
				domainevent.StatusPROCESSING,
				domainevent.StatusFAILED,
				domainevent.StatusCANCELLED,
			}
		}
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
