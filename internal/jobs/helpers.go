// Package jobs defines River Queue job types for async processing.
//
// ADR-0006: River Queue for async task execution.
// ADR-0009: Claim-check pattern — job carries only EventID.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/jobs
package jobs

import (
	"context"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// setTicketStatusByEvent updates the ticket status associated with a
// domain event. This is a best-effort operation: failures are logged but
// not propagated, since the ticket status is an auxiliary concern.
func setTicketStatusByEvent(ctx context.Context, client *ent.Client, eventID string, status entticket.Status) {
	if client == nil || eventID == "" {
		return
	}
	if _, err := client.Ticket.Update().
		Where(entticket.EventIDEQ(eventID)).
		SetStatus(status).
		Save(ctx); err != nil {
		logger.Warn("failed to update ticket status by event",
			zap.String("event_id", eventID),
			zap.String("status", status.String()),
			zap.Error(err),
		)
	}
	SyncParentBatchStatusByChildEvent(ctx, client, eventID)
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
func SyncParentBatchStatusByChildEvent(ctx context.Context, client *ent.Client, childEventID string) {
	if client == nil || childEventID == "" {
		return
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
		return
	}
	parentID := child.ParentTicketID
	if parentID == "" {
		return
	}
	SyncParentBatchStatus(ctx, client, parentID)
}

// SyncParentBatchStatus recalculates parent batch ticket, event, and projection
// state from its child ticket statuses.
func SyncParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string) {
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parentTicketID)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to query child tickets for parent batch status sync",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
		return
	}
	if len(children) == 0 {
		return
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
		parentStatus = entticket.StatusEXECUTING
		projectionStatus = entbatchticket.StatusIN_PROGRESS
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

	parent, err := client.Ticket.UpdateOneID(parentTicketID).
		SetStatus(parentStatus).
		Save(ctx)
	if err != nil {
		logger.Warn("failed to update parent batch ticket status",
			zap.String("parent_ticket_id", parentTicketID),
			zap.String("status", parentStatus.String()),
			zap.Error(err),
		)
		return
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
		eventStatus = domainevent.StatusPROCESSING
	}
	if _, err := client.DomainEvent.UpdateOneID(parent.EventID).
		SetStatus(eventStatus).
		Save(ctx); err != nil {
		logger.Warn("failed to update parent batch event status",
			zap.String("parent_ticket_id", parentTicketID),
			zap.String("event_id", parent.EventID),
			zap.String("status", eventStatus.String()),
			zap.Error(err),
		)
	}

	if _, err := client.BatchTicket.UpdateOneID(parentTicketID).
		SetChildCount(len(children)).
		SetSuccessCount(successCount).
		SetFailedCount(failedCount).
		SetPendingCount(activeCount).
		SetStatus(projectionStatus).
		Save(ctx); err != nil {
		if !ent.IsNotFound(err) {
			logger.Warn("failed to update batch projection counters",
				zap.String("parent_ticket_id", parentTicketID),
				zap.String("status", projectionStatus.String()),
				zap.Error(err),
			)
		}
	}
}
