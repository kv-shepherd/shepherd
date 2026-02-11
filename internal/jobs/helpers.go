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
