// Package audit implements the audit logging service.
//
// Audit logs are append-only compliance records. Hard-delete is NOT allowed.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/governance/audit
package audit

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// Logger writes audit records to the database.
type Logger struct {
	client *ent.Client
}

// NewLogger creates a new audit Logger.
func NewLogger(client *ent.Client) *Logger {
	return &Logger{client: client}
}

// LogAction records an auditable action.
func (l *Logger) LogAction(ctx context.Context, action, resourceType, resourceID, actor string, details map[string]interface{}) error {
	_, err := l.client.AuditLog.Create().
		SetID(generateAuditID()).
		SetAction(action).
		SetResourceType(resourceType).
		SetResourceID(resourceID).
		SetActor(actor).
		SetDetails(details).
		Save(ctx)
	if err != nil {
		logger.Error("Failed to write audit log",
			zap.String("action", action),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// LogApproval records an approval decision.
func (l *Logger) LogApproval(ctx context.Context, ticketID, decision, actor string) error {
	return l.LogApprovalWithDetails(ctx, ticketID, decision, actor, nil)
}

// LogApprovalWithDetails records an approval decision with additional details.
func (l *Logger) LogApprovalWithDetails(
	ctx context.Context,
	ticketID, decision, actor string,
	details map[string]interface{},
) error {
	payload := map[string]interface{}{
		"decision": decision,
	}
	for key, value := range details {
		payload[key] = value
	}
	return l.LogAction(ctx, "approval."+decision, "approval_ticket", ticketID, actor, payload)
}

// LogVMOperation records a VM operation.
func (l *Logger) LogVMOperation(ctx context.Context, operation, vmID, actor string) error {
	return l.LogAction(ctx, "vm."+operation, "vm", vmID, actor, nil)
}

func generateAuditID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return fmt.Sprintf("audit-%s", id.String())
}
