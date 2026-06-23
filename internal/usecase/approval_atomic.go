// Package usecase provides application use cases (Clean Architecture).
//
// ADR-0012: Core approval writes + River enqueue must be atomic in a single pgx.Tx.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"

	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/observability"
	sqlcrepo "kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// ApprovalAtomicWriter executes approval state transition + River enqueue in one pgx transaction.
type ApprovalAtomicWriter struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
	queries     *sqlcrepo.Queries
}

// PowerEventInput is the immutable event snapshot for a direct no-approval
// power request. The River job receives only EventID and resolves this row.
type PowerEventInput struct {
	EventID       string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       []byte
	CreatedBy     string
}

// NewApprovalAtomicWriter creates a new ADR-0012 atomic writer.
func NewApprovalAtomicWriter(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *ApprovalAtomicWriter {
	return &ApprovalAtomicWriter{
		pool:        pool,
		riverClient: riverClient,
		queries:     sqlcrepo.New(pool),
	}
}

// ApproveCreateAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) marks event PROCESSING,
// 3) allocates VM instance/index + inserts VM row,
// 4) inserts River vm_create job via InsertTx.
// 5) inserts River vm_status_sync bootstrap job via InsertTx.
func (w *ApprovalAtomicWriter) ApproveCreateAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
	templateSnapshot map[string]interface{},
	instanceSizeSnapshot map[string]interface{},
	placementEvaluation map[string]interface{},
	modifiedSpec map[string]interface{},
) (vmID, vmName string, err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.approval.approve_create",
		attribute.String("shepherd.business.operation", "approval.approve_create"),
		attribute.String("shepherd.approval.operation_type", "CREATE"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return "", "", fmt.Errorf("approval atomic writer is not initialized")
	}
	if validateErr := w.validateCreateInput(ticketID, eventID, approver, clusterID, serviceID, namespace, requesterID); validateErr != nil {
		return "", "", validateErr
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin approval create tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)
	templateSnapshotBytes, err := marshalJSONOrNull(templateSnapshot)
	if err != nil {
		return "", "", fmt.Errorf("marshal template snapshot: %w", err)
	}
	instanceSizeSnapshotBytes, err := marshalJSONOrNull(instanceSizeSnapshot)
	if err != nil {
		return "", "", fmt.Errorf("marshal instance size snapshot: %w", err)
	}
	placementEvaluationBytes, err := marshalJSONOrNull(placementEvaluation)
	if err != nil {
		return "", "", fmt.Errorf("marshal placement evaluation: %w", err)
	}
	modifiedSpecBytes, err := marshalJSONOrNull(modifiedSpec)
	if err != nil {
		return "", "", fmt.Errorf("marshal modified spec: %w", err)
	}
	affected, err := qtx.ApproveCreateTicket(ctx, sqlcrepo.ApproveCreateTicketParams{
		Approver:             pgtype.Text{String: approver, Valid: true},
		SelectedClusterID:    pgtype.Text{String: clusterID, Valid: true},
		SelectedStorageClass: strings.TrimSpace(storageClass),
		TemplateSnapshot:     templateSnapshotBytes,
		InstanceSizeSnapshot: instanceSizeSnapshotBytes,
		PlacementEvaluation:  placementEvaluationBytes,
		ModifiedSpec:         modifiedSpecBytes,
		ID:                   ticketID,
		EventID:              eventID,
	})
	if err != nil {
		return "", "", fmt.Errorf("approve create ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return "", "", fmt.Errorf("approve create ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return "", "", fmt.Errorf("set event %s to PROCESSING: %w", eventID, err)
	}
	if affected == 0 {
		return "", "", fmt.Errorf("domain event %s not found or not pending", eventID)
	}

	allocated, err := qtx.AllocateServiceInstance(ctx, serviceID)
	if err != nil {
		return "", "", fmt.Errorf("allocate service instance for service %s: %w", serviceID, err)
	}

	instance := fmt.Sprintf("%02d", allocated.AllocatedIndex)
	vmName = fmt.Sprintf("%s-%s-%s-%s", namespace, allocated.SystemName, allocated.ServiceName, instance)

	vmUUID, err := uuid.NewV7()
	if err != nil {
		return "", "", fmt.Errorf("generate vm id: %w", err)
	}
	vmID = vmUUID.String()

	rootVolumeAccessModes := snapshotStringSlice(instanceSizeSnapshot)
	rootVolumeVolumeMode := snapshotString(instanceSizeSnapshot, "dv_volume_mode")
	rootVolumeAccessModesBytes, err := marshalJSONArrayOrNull(rootVolumeAccessModes)
	if err != nil {
		return "", "", fmt.Errorf("marshal root volume access modes: %w", err)
	}

	if err := qtx.InsertVM(ctx, sqlcrepo.InsertVMParams{
		ID:                     vmID,
		Name:                   vmName,
		Instance:               instance,
		Namespace:              namespace,
		ClusterID:              pgtype.Text{String: clusterID, Valid: true},
		Hostname:               pgtype.Text{String: vmName, Valid: true},
		CreatedBy:              requesterID,
		TicketID:               pgtype.Text{String: ticketID, Valid: true},
		RootVolumeStorageClass: pgtype.Text{String: strings.TrimSpace(storageClass), Valid: strings.TrimSpace(storageClass) != ""},
		RootVolumeAccessModes:  rootVolumeAccessModesBytes,
		RootVolumeVolumeMode:   pgtype.Text{String: rootVolumeVolumeMode, Valid: rootVolumeVolumeMode != ""},
		ServiceVms:             serviceID,
	}); err != nil {
		return "", "", fmt.Errorf("insert vm %s: %w", vmID, err)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMCreateArgs{
		EventID: eventID,
	}, nil); err != nil {
		return "", "", fmt.Errorf("enqueue vm_create for event %s: %w", eventID, err)
	}
	// Bootstrap the VM status sync polling chain (ADR-0038).
	// This is the initial insert — runs immediately (no ScheduledAt).
	// Subsequent polls are self-scheduled by VMStatusSyncWorker.scheduleNext().
	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMStatusSyncArgs{
		EventID: eventID,
	}, vmStatusSyncInsertOpts()); err != nil {
		return "", "", fmt.Errorf("enqueue vm_status_sync for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit approval create tx: %w", err)
	}

	return vmID, vmName, nil
}

func vmStatusSyncInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       jobs.VMStatusSyncJobKind,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	}
}

func marshalJSONOrNull(value map[string]interface{}) ([]byte, error) {
	if len(value) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func marshalJSONArrayOrNull(values []string) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func snapshotString(values map[string]interface{}, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func snapshotStringSlice(values map[string]interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["dv_access_modes"]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		items := make([]string, len(typed))
		copy(items, typed)
		return items
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				items = append(items, text)
			}
		}
		if len(items) == 0 {
			return nil
		}
		return items
	default:
		return nil
	}
}

// ApproveDeleteAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) marks event PROCESSING,
// 3) marks VM DELETING,
// 4) inserts River vm_delete job via InsertTx.
func (w *ApprovalAtomicWriter) ApproveDeleteAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver, vmID string,
) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.approval.approve_delete",
		attribute.String("shepherd.business.operation", "approval.approve_delete"),
		attribute.String("shepherd.approval.operation_type", "DELETE"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(ticketID) == "" ||
		strings.TrimSpace(eventID) == "" ||
		strings.TrimSpace(approver) == "" ||
		strings.TrimSpace(vmID) == "" {
		return fmt.Errorf("approve delete input is incomplete")
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approval delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)

	affected, err := qtx.ApproveDeleteTicket(ctx, sqlcrepo.ApproveDeleteTicketParams{
		Approver: pgtype.Text{String: approver, Valid: true},
		ID:       ticketID,
		EventID:  eventID,
	})
	if err != nil {
		return fmt.Errorf("approve delete ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return fmt.Errorf("approve delete ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return fmt.Errorf("set event %s to PROCESSING: %w", eventID, err)
	}
	if affected == 0 {
		return fmt.Errorf("domain event %s not found or not pending", eventID)
	}

	affected, err = qtx.SetVMStatus(ctx, sqlcrepo.SetVMStatusParams{
		ID:     strings.TrimSpace(vmID),
		Status: "DELETING",
	})
	if err != nil {
		return fmt.Errorf("set vm %s status to DELETING: %w", vmID, err)
	}
	if affected == 0 {
		return fmt.Errorf("vm %s not found while setting status to DELETING", vmID)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMDeleteArgs{
		EventID: eventID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue vm_delete for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval delete tx: %w", err)
	}
	return nil
}

// ApproveModifyAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) marks event PROCESSING,
// 3) inserts River vm_modify job via InsertTx.
func (w *ApprovalAtomicWriter) ApproveModifyAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver string,
	modifiedSpec map[string]interface{},
) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.approval.approve_modify",
		attribute.String("shepherd.business.operation", "approval.approve_modify"),
		attribute.String("shepherd.approval.operation_type", "MODIFY"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(ticketID) == "" || strings.TrimSpace(eventID) == "" || strings.TrimSpace(approver) == "" {
		return fmt.Errorf("approve modify input is incomplete")
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approval modify tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)
	modifiedSpecBytes, err := marshalJSONOrNull(modifiedSpec)
	if err != nil {
		return fmt.Errorf("marshal modify approved spec: %w", err)
	}

	affected, err := qtx.ApproveModifyTicket(ctx, sqlcrepo.ApproveModifyTicketParams{
		Approver:     pgtype.Text{String: approver, Valid: true},
		ID:           ticketID,
		EventID:      eventID,
		ModifiedSpec: modifiedSpecBytes,
	})
	if err != nil {
		return fmt.Errorf("approve modify ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return fmt.Errorf("approve modify ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return fmt.Errorf("set event %s to PROCESSING: %w", eventID, err)
	}
	if affected == 0 {
		return fmt.Errorf("domain event %s not found or not pending", eventID)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMModifyArgs{
		EventID: eventID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue vm_modify for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval modify tx: %w", err)
	}
	return nil
}

// ApprovePowerAndEnqueue atomically:
// 1) marks ticket APPROVED,
// 2) inserts River vm_power job via InsertTx.
//
// The associated DomainEvent remains PENDING until the worker starts and
// resolves the operation outcome.
func (w *ApprovalAtomicWriter) ApprovePowerAndEnqueue(
	ctx context.Context,
	ticketID, eventID, approver, operation string,
) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.approval.approve_power",
		attribute.String("shepherd.business.operation", "approval.approve_power"),
		attribute.String("shepherd.approval.operation_type", "POWER"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(ticketID) == "" ||
		strings.TrimSpace(eventID) == "" ||
		strings.TrimSpace(approver) == "" ||
		strings.TrimSpace(operation) == "" {
		return fmt.Errorf("approve power input is incomplete")
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approval power tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)

	affected, err := qtx.ApprovePowerTicket(ctx, sqlcrepo.ApprovePowerTicketParams{
		Approver: pgtype.Text{String: approver, Valid: true},
		ID:       ticketID,
		EventID:  eventID,
	})
	if err != nil {
		return fmt.Errorf("approve power ticket %s: %w", ticketID, err)
	}
	if affected == 0 {
		return fmt.Errorf("approve power ticket %s: not pending or operation type mismatch", ticketID)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlcrepo.SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PENDING",
	})
	if err != nil {
		return fmt.Errorf("confirm event %s is PENDING: %w", eventID, err)
	}
	if affected == 0 {
		return fmt.Errorf("domain event %s not found or not pending", eventID)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{
		EventID: eventID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue vm_power for event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval power tx: %w", err)
	}
	return nil
}

// CreatePowerEventAndEnqueue atomically creates a no-approval power DomainEvent
// and inserts the River job in the same pgx transaction. This mirrors River's
// recommended InsertTx pattern so the worker can only observe a committed event.
func (w *ApprovalAtomicWriter) CreatePowerEventAndEnqueue(ctx context.Context, input PowerEventInput) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.vm.request_power",
		attribute.String("shepherd.business.operation", "vm.request_power"),
		attribute.String("shepherd.approval.operation_type", "POWER"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(input.EventID) == "" ||
		strings.TrimSpace(input.EventType) == "" ||
		strings.TrimSpace(input.AggregateType) == "" ||
		strings.TrimSpace(input.AggregateID) == "" ||
		strings.TrimSpace(input.CreatedBy) == "" ||
		len(input.Payload) == 0 {
		return fmt.Errorf("power event input is incomplete")
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin direct power tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)
	if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
		ID:            input.EventID,
		EventType:     strings.TrimSpace(input.EventType),
		AggregateType: strings.TrimSpace(input.AggregateType),
		AggregateID:   strings.TrimSpace(input.AggregateID),
		Payload:       input.Payload,
		Status:        "PENDING",
		CreatedBy:     strings.TrimSpace(input.CreatedBy),
	}); err != nil {
		return fmt.Errorf("insert power domain event %s: %w", input.EventID, err)
	}

	if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{
		EventID: input.EventID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue vm_power for event %s: %w", input.EventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit direct power tx: %w", err)
	}
	return nil
}

func (w *ApprovalAtomicWriter) validateCreateInput(
	ticketID, eventID, approver, clusterID, serviceID, namespace, requesterID string,
) error {
	switch {
	case strings.TrimSpace(ticketID) == "":
		return fmt.Errorf("ticket id is required")
	case strings.TrimSpace(eventID) == "":
		return fmt.Errorf("event id is required")
	case strings.TrimSpace(approver) == "":
		return fmt.Errorf("approver is required")
	case strings.TrimSpace(clusterID) == "":
		return fmt.Errorf("selected cluster is required for create approval")
	case strings.TrimSpace(serviceID) == "":
		return fmt.Errorf("service id is required")
	case strings.TrimSpace(namespace) == "":
		return fmt.Errorf("namespace is required")
	case strings.TrimSpace(requesterID) == "":
		return fmt.Errorf("requester id is required")
	default:
		return nil
	}
}
