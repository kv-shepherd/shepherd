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
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/batchapprovalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// setTicketStatusByEvent updates the approval ticket status associated with a
// domain event. This is a best-effort operation: failures are logged but
// not propagated, since the ticket status is an auxiliary concern.
func setTicketStatusByEvent(ctx context.Context, client *ent.Client, eventID string, status approvalticket.Status) {
	if client == nil || eventID == "" {
		return
	}
	if _, err := client.ApprovalTicket.Update().
		Where(approvalticket.EventIDEQ(eventID)).
		SetStatus(status).
		Save(ctx); err != nil {
		logger.Warn("failed to update approval ticket status by event",
			zap.String("event_id", eventID),
			zap.String("status", status.String()),
			zap.Error(err),
		)
	}
	syncParentBatchStatusByChildEvent(ctx, client, eventID)
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

func syncParentBatchStatusByChildEvent(ctx context.Context, client *ent.Client, childEventID string) {
	if client == nil || childEventID == "" {
		return
	}
	child, err := client.ApprovalTicket.Query().
		Where(approvalticket.EventIDEQ(childEventID)).
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
	syncParentBatchStatus(ctx, client, parentID)
}

func syncParentBatchStatus(ctx context.Context, client *ent.Client, parentTicketID string) {
	children, err := client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDEQ(parentTicketID)).
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
		case approvalticket.StatusSUCCESS:
			successCount++
		case approvalticket.StatusFAILED, approvalticket.StatusREJECTED:
			failedCount++
		case approvalticket.StatusCANCELLED:
			cancelledCount++
		default:
			activeCount++
		}
	}

	parentStatus := approvalticket.StatusEXECUTING
	projectionStatus := batchapprovalticket.StatusIN_PROGRESS
	switch {
	case activeCount > 0:
		parentStatus = approvalticket.StatusEXECUTING
		projectionStatus = batchapprovalticket.StatusIN_PROGRESS
	case successCount == len(children):
		parentStatus = approvalticket.StatusSUCCESS
		projectionStatus = batchapprovalticket.StatusCOMPLETED
	case cancelledCount == len(children):
		parentStatus = approvalticket.StatusCANCELLED
		projectionStatus = batchapprovalticket.StatusCANCELLED
	case successCount > 0 && (failedCount+cancelledCount) > 0:
		parentStatus = approvalticket.StatusFAILED
		projectionStatus = batchapprovalticket.StatusPARTIAL_SUCCESS
	default:
		// Includes terminal mixed outcomes (PARTIAL_SUCCESS in API view).
		parentStatus = approvalticket.StatusFAILED
		projectionStatus = batchapprovalticket.StatusFAILED
	}

	parent, err := client.ApprovalTicket.UpdateOneID(parentTicketID).
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
	case approvalticket.StatusSUCCESS:
		eventStatus = domainevent.StatusCOMPLETED
	case approvalticket.StatusFAILED:
		eventStatus = domainevent.StatusFAILED
	case approvalticket.StatusCANCELLED:
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

	if _, err := client.BatchApprovalTicket.UpdateOneID(parentTicketID).
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
