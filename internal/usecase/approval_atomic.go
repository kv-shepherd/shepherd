// Package usecase provides application use cases (Clean Architecture).
//
// ADR-0012: Core approval writes + River enqueue must be atomic in a single pgx.Tx.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"

	"kv-shepherd.io/shepherd/internal/domain"
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

// PowerApprovalRequestInput is the immutable event and ticket snapshot for a
// power request that must wait for approval. The event and ticket are created
// under the same per-VM lock used by direct and batch power submissions.
type PowerApprovalRequestInput struct {
	EventID     string
	TicketID    string
	EventType   string
	AggregateID string
	Payload     []byte
	CreatedBy   string
	Reason      string
}

// DuplicatePowerEventError reports the active direct power event that made a
// repeated request idempotent. Callers can return the existing event to the
// client without inserting another event or River job.
type DuplicatePowerEventError struct {
	ExistingEventID string
}

func (e *DuplicatePowerEventError) Error() string {
	return fmt.Sprintf("active power event %s already exists", e.ExistingEventID)
}

// ActivePowerEventError reports an active power operation that cannot be
// treated as an idempotent repeat of the same direct request. Ticket-backed
// events must retain their approval semantics, and a different action must not
// be reported as accepted under an unrelated event ID.
type ActivePowerEventError struct {
	ExistingEventID        string
	ExistingEventType      string
	ExistingEventStatus    string
	ExistingTicketID       string
	ExistingTicketStatus   string
	ExistingParentTicketID string
	AggregateID            string
}

func (e *ActivePowerEventError) Error() string {
	return fmt.Sprintf("active power event %s (%s) is already in progress", e.ExistingEventID, e.ExistingEventType)
}

const (
	powerVMAggregateType  = "vm"
	powerEventTypeStart   = "VM_START_REQUESTED"
	powerEventTypeStop    = "VM_STOP_REQUESTED"
	powerEventTypeRestart = "VM_RESTART_REQUESTED"
)

// PowerVMLockKey returns the cross-process advisory-lock namespace shared by
// power submission and retry transactions.
func PowerVMLockKey(vmID string) string {
	return "power:vm:" + strings.TrimSpace(vmID)
}

func lockPowerVM(ctx context.Context, tx pgx.Tx, vmID string) error {
	normalizedVMID := strings.TrimSpace(vmID)
	if normalizedVMID == "" {
		return fmt.Errorf("power VM id is required")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, PowerVMLockKey(normalizedVMID)); err != nil {
		return fmt.Errorf("lock power request for VM %s: %w", normalizedVMID, err)
	}
	return nil
}

func findActivePowerEvent(ctx context.Context, tx pgx.Tx, vmID, excludedEventID string) (*ActivePowerEventError, error) {
	normalizedVMID := strings.TrimSpace(vmID)
	var (
		existingEventID        string
		existingEventType      string
		existingEventStatus    string
		existingTicketID       pgtype.Text
		existingTicketStatus   pgtype.Text
		existingParentTicketID pgtype.Text
	)
	err := tx.QueryRow(ctx, `
SELECT event.id, event.event_type, event.status, ticket.id, ticket.status, ticket.parent_ticket_id
FROM domain_events AS event
LEFT JOIN LATERAL (
  SELECT id, status, parent_ticket_id
  FROM tickets
  WHERE event_id = event.id
    AND operation_type = 'POWER'
	-- A corrupt duplicate binding must never let a newer terminal ticket hide
	-- an older approval-pending ticket for the same power event.
	ORDER BY (status = 'PENDING') DESC, created_at DESC, id DESC
  LIMIT 1
) AS ticket ON TRUE
WHERE event.aggregate_type = 'vm'
  AND event.aggregate_id = $1
  AND event.event_type IN ('VM_START_REQUESTED', 'VM_STOP_REQUESTED', 'VM_RESTART_REQUESTED')
  AND event.status IN ('PENDING', 'PROCESSING')
  AND (
    $2 = ''
    OR event.id <> $2
	OR event.status = 'PROCESSING'
  )
  AND (
	-- Every PROCESSING power event is a durable ambiguous-dispatch fence.
	-- River may rescue a job while its original worker is still running, so
	-- neither a terminal/non-runnable River row nor a missing ticket proves
		-- that another start, stop, or restart is safe. Operators may verify and
		-- escalate an ambiguous fence, but must not clear it manually.
	event.status = 'PROCESSING'
    OR
    (ticket.id IS NOT NULL AND ticket.status = 'PENDING')
    OR EXISTS (
      SELECT 1
      FROM river_job AS job
      WHERE job.kind = 'vm_power'
        AND job.args->>'event_id' = event.id
        AND job.state IN ('available', 'pending', 'retryable', 'running', 'scheduled')
    )
  )
ORDER BY event.created_at DESC, event.id DESC
LIMIT 1
`, normalizedVMID, strings.TrimSpace(excludedEventID)).Scan(
		&existingEventID,
		&existingEventType,
		&existingEventStatus,
		&existingTicketID,
		&existingTicketStatus,
		&existingParentTicketID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check active power event for VM %s: %w", normalizedVMID, err)
	}
	return &ActivePowerEventError{
		ExistingEventID:        existingEventID,
		ExistingEventType:      existingEventType,
		ExistingEventStatus:    existingEventStatus,
		ExistingTicketID:       existingTicketID.String,
		ExistingTicketStatus:   existingTicketStatus.String,
		ExistingParentTicketID: existingParentTicketID.String,
		AggregateID:            normalizedVMID,
	}, nil
}

func isPowerEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case powerEventTypeStart, powerEventTypeStop, powerEventTypeRestart:
		return true
	default:
		return false
	}
}

func validateVMPowerPayloadIdentity(
	rawPayload []byte,
	eventType string,
	vmID string,
	actor string,
	dispatchMode domain.VMPowerDispatchMode,
) error {
	var payload domain.VMPowerPayload
	if err := decodeBatchApprovalPayloadExact(rawPayload, &payload); err != nil {
		return fmt.Errorf("power payload is malformed: %w", err)
	}
	expectedOperation := ""
	switch strings.TrimSpace(eventType) {
	case powerEventTypeStart:
		expectedOperation = powerOperationStart
	case powerEventTypeStop:
		expectedOperation = powerOperationStop
	case powerEventTypeRestart:
		expectedOperation = powerOperationRestart
	default:
		return fmt.Errorf("unsupported power event type %q", eventType)
	}
	if strings.TrimSpace(payload.VMID) != strings.TrimSpace(vmID) ||
		strings.TrimSpace(payload.Actor) != strings.TrimSpace(actor) ||
		strings.ToLower(strings.TrimSpace(payload.Operation)) != expectedOperation ||
		strings.TrimSpace(payload.VMName) == "" ||
		strings.TrimSpace(payload.ClusterID) == "" ||
		strings.TrimSpace(payload.Namespace) == "" ||
		payload.DispatchMode != dispatchMode {
		return fmt.Errorf("power payload identity does not match its immutable event input")
	}
	return nil
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
	return w.approveCreateAndEnqueue(
		ctx, nil, ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID,
		templateSnapshot, instanceSizeSnapshot, placementEvaluation, modifiedSpec,
	)
}

// ApproveBatchCreateAndEnqueue applies the exact durable batch graph guard in
// the same transaction as the child state, VM allocation, and River inserts.
func (w *ApprovalAtomicWriter) ApproveBatchCreateAndEnqueue(
	ctx context.Context,
	guard domain.BatchApprovalDispatchGuard,
	ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
	templateSnapshot map[string]interface{},
	instanceSizeSnapshot map[string]interface{},
	placementEvaluation map[string]interface{},
	modifiedSpec map[string]interface{},
) (vmID, vmName string, err error) {
	return w.approveCreateAndEnqueue(
		ctx, &guard, ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID,
		templateSnapshot, instanceSizeSnapshot, placementEvaluation, modifiedSpec,
	)
}

func (w *ApprovalAtomicWriter) approveCreateAndEnqueue(
	ctx context.Context,
	guard *domain.BatchApprovalDispatchGuard,
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
	if guard != nil {
		if validationErr := w.validateAndLockBatchApprovalChildDispatch(ctx, tx, *guard, ticketID, eventID, approver); validationErr != nil {
			return "", "", fmt.Errorf("validate batch create child dispatch: %w", validationErr)
		}
		if revisionErr := validateBatchCreateCatalogRevisions(ctx, tx, templateSnapshot, instanceSizeSnapshot); revisionErr != nil {
			return "", "", revisionErr
		}
	}

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

func validateBatchCreateCatalogRevisions(
	ctx context.Context,
	tx pgx.Tx,
	templateSnapshot map[string]interface{},
	instanceSizeSnapshot map[string]interface{},
) error {
	if err := validateBatchCatalogRevision(
		ctx,
		tx,
		"template",
		templateSnapshot,
		`SELECT updated_at FROM templates WHERE id = $1 FOR SHARE`,
	); err != nil {
		return err
	}
	return validateBatchCatalogRevision(
		ctx,
		tx,
		"instance size",
		instanceSizeSnapshot,
		`SELECT updated_at FROM instance_sizes WHERE id = $1 FOR SHARE`,
	)
}

func validateBatchCatalogRevision(
	ctx context.Context,
	tx pgx.Tx,
	label string,
	snapshot map[string]interface{},
	query string,
) error {
	id, idOK := snapshot["id"].(string)
	revisionText, revisionOK := snapshot["updated_at"].(string)
	id = strings.TrimSpace(id)
	revisionText = strings.TrimSpace(revisionText)
	if !idOK || !revisionOK || id == "" || revisionText == "" {
		return fmt.Errorf("batch create %s snapshot revision is incomplete", label)
	}
	expected, err := time.Parse(time.RFC3339Nano, revisionText)
	if err != nil {
		return fmt.Errorf("parse batch create %s snapshot revision: %w", label, err)
	}
	var current time.Time
	if err := tx.QueryRow(ctx, query, id).Scan(&current); err != nil {
		return fmt.Errorf("lock batch create %s %s revision: %w", label, id, err)
	}
	if !current.Equal(expected) {
		return fmt.Errorf("batch create %s %s changed after preflight", label, id)
	}
	return nil
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
	return w.approveDeleteAndEnqueue(ctx, nil, ticketID, eventID, approver, vmID)
}

// ApproveBatchDeleteAndEnqueue guards a batch child delete and its River job
// with the exact durable graph fingerprint.
func (w *ApprovalAtomicWriter) ApproveBatchDeleteAndEnqueue(
	ctx context.Context,
	guard domain.BatchApprovalDispatchGuard,
	ticketID, eventID, approver, vmID string,
) error {
	return w.approveDeleteAndEnqueue(ctx, &guard, ticketID, eventID, approver, vmID)
}

func (w *ApprovalAtomicWriter) approveDeleteAndEnqueue(
	ctx context.Context,
	guard *domain.BatchApprovalDispatchGuard,
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
	if guard != nil {
		if validationErr := w.validateAndLockBatchApprovalChildDispatch(ctx, tx, *guard, ticketID, eventID, approver); validationErr != nil {
			return fmt.Errorf("validate batch delete child dispatch: %w", validationErr)
		}
	}

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
	return w.approveModifyAndEnqueue(ctx, nil, ticketID, eventID, approver, modifiedSpec)
}

// ApproveBatchModifyAndEnqueue guards a batch child modify and its River job
// with the exact durable graph fingerprint.
func (w *ApprovalAtomicWriter) ApproveBatchModifyAndEnqueue(
	ctx context.Context,
	guard domain.BatchApprovalDispatchGuard,
	ticketID, eventID, approver string,
	modifiedSpec map[string]interface{},
) error {
	return w.approveModifyAndEnqueue(ctx, &guard, ticketID, eventID, approver, modifiedSpec)
}

func (w *ApprovalAtomicWriter) approveModifyAndEnqueue(
	ctx context.Context,
	guard *domain.BatchApprovalDispatchGuard,
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
	if guard != nil {
		if validationErr := w.validateAndLockBatchApprovalChildDispatch(ctx, tx, *guard, ticketID, eventID, approver); validationErr != nil {
			return fmt.Errorf("validate batch modify child dispatch: %w", validationErr)
		}
	}

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
	return w.approvePowerAndEnqueue(ctx, nil, ticketID, eventID, approver, operation)
}

// ApproveBatchPowerAndEnqueue guards a batch child power transition and its
// River job with the exact durable graph fingerprint.
func (w *ApprovalAtomicWriter) ApproveBatchPowerAndEnqueue(
	ctx context.Context,
	guard domain.BatchApprovalDispatchGuard,
	ticketID, eventID, approver, operation string,
) error {
	return w.approvePowerAndEnqueue(ctx, &guard, ticketID, eventID, approver, operation)
}

func (w *ApprovalAtomicWriter) approvePowerAndEnqueue(
	ctx context.Context,
	guard *domain.BatchApprovalDispatchGuard,
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
	if guard != nil {
		if validationErr := w.validateAndLockBatchApprovalChildDispatch(ctx, tx, *guard, ticketID, eventID, approver); validationErr != nil {
			return fmt.Errorf("validate batch power child dispatch: %w", validationErr)
		}
	}

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

// CreatePowerApprovalRequest atomically creates a pending power DomainEvent
// and its approval ticket while holding the shared per-VM power lock. Keeping
// the check and both writes in one pgx transaction closes the preflight/write
// race between direct, approval-required, and batch submissions.
func (w *ApprovalAtomicWriter) CreatePowerApprovalRequest(ctx context.Context, input PowerApprovalRequestInput) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.vm.request_power_approval",
		attribute.String("shepherd.business.operation", "vm.request_power_approval"),
		attribute.String("shepherd.approval.operation_type", "POWER"),
	)
	defer func() {
		var active *ActivePowerEventError
		if !errors.As(err, &active) {
			observability.RecordSpanError(span, err)
		}
		span.End()
	}()

	if w.pool == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	eventID := strings.TrimSpace(input.EventID)
	ticketID := strings.TrimSpace(input.TicketID)
	eventType := strings.TrimSpace(input.EventType)
	vmID := strings.TrimSpace(input.AggregateID)
	actor := strings.TrimSpace(input.CreatedBy)
	if eventID == "" || ticketID == "" || vmID == "" || actor == "" || len(input.Payload) == 0 {
		return fmt.Errorf("power approval request input is incomplete")
	}
	if !isPowerEventType(eventType) {
		return fmt.Errorf("unsupported power event type %q", eventType)
	}
	if payloadErr := validateVMPowerPayloadIdentity(
		input.Payload,
		eventType,
		vmID,
		actor,
		domain.VMPowerDispatchTicket,
	); payloadErr != nil {
		return fmt.Errorf("invalid power approval payload: %w", payloadErr)
	}

	// The per-VM advisory lock may wait for a preceding submission. READ
	// COMMITTED ensures the active-event query below sees that submission after
	// the lock is acquired, even when the database default is stricter.
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin power approval request tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if lockErr := lockPowerVM(ctx, tx, vmID); lockErr != nil {
		return lockErr
	}
	active, err := findActivePowerEvent(ctx, tx, vmID, "")
	if err != nil {
		return err
	}
	if active != nil {
		return active
	}

	qtx := w.queries.WithTx(tx)
	if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
		ID:            eventID,
		EventType:     eventType,
		AggregateType: powerVMAggregateType,
		AggregateID:   vmID,
		Payload:       input.Payload,
		Status:        "PENDING",
		CreatedBy:     actor,
	}); err != nil {
		return fmt.Errorf("insert power approval event %s: %w", eventID, err)
	}
	if err := qtx.InsertTicket(ctx, sqlcrepo.InsertTicketParams{
		ID:             ticketID,
		EventID:        eventID,
		OperationType:  "POWER",
		Status:         "PENDING",
		Requester:      actor,
		Reason:         textOrNull(input.Reason),
		ParentTicketID: pgtype.Text{},
	}); err != nil {
		return fmt.Errorf("insert power approval ticket %s: %w", ticketID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit power approval request tx: %w", err)
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
		var duplicate *DuplicatePowerEventError
		var active *ActivePowerEventError
		if !errors.As(err, &duplicate) && !errors.As(err, &active) {
			observability.RecordSpanError(span, err)
		}
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if strings.TrimSpace(input.EventID) == "" ||
		strings.TrimSpace(input.EventType) == "" ||
		strings.TrimSpace(input.AggregateType) != powerVMAggregateType ||
		strings.TrimSpace(input.AggregateID) == "" ||
		strings.TrimSpace(input.CreatedBy) == "" ||
		len(input.Payload) == 0 {
		return fmt.Errorf("power event input is incomplete")
	}

	// The per-VM advisory lock may wait for a preceding submission. READ
	// COMMITTED ensures the active-event query below sees that submission after
	// the lock is acquired, even when the database default is stricter.
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin direct power tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	aggregateType := strings.TrimSpace(input.AggregateType)
	aggregateID := strings.TrimSpace(input.AggregateID)
	eventType := strings.TrimSpace(input.EventType)
	if !isPowerEventType(eventType) {
		return fmt.Errorf("unsupported power event type %q", eventType)
	}
	if payloadErr := validateVMPowerPayloadIdentity(
		input.Payload,
		eventType,
		aggregateID,
		strings.TrimSpace(input.CreatedBy),
		domain.VMPowerDispatchDirect,
	); payloadErr != nil {
		return fmt.Errorf("invalid direct power payload: %w", payloadErr)
	}
	if lockErr := lockPowerVM(ctx, tx, aggregateID); lockErr != nil {
		return lockErr
	}
	active, err := findActivePowerEvent(ctx, tx, aggregateID, "")
	if err != nil {
		return err
	}
	if active != nil {
		if active.ExistingTicketID == "" &&
			active.ExistingEventType == eventType &&
			active.ExistingEventStatus == "PENDING" {
			return &DuplicatePowerEventError{ExistingEventID: active.ExistingEventID}
		}
		return active
	}

	qtx := w.queries.WithTx(tx)
	if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
		ID:            input.EventID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
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
