package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

// ---------------------------------------------------------------------------
// Job Args
// ---------------------------------------------------------------------------

// VMCreateArgs carries only EventID (Claim-check pattern, ADR-0009).
type VMCreateArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM creation.
func (VMCreateArgs) Kind() string { return "vm_create" }

// InsertOpts returns default insert options for VM creation jobs.
func (VMCreateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "vm_operations",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Worker
// ---------------------------------------------------------------------------

// VMCreateWorker processes VM creation jobs after approval.
//
// Execution flow (master-flow.md Stage 5.C):
//  1. Fetch DomainEvent by EventID (claim-check, ADR-0009)
//  2. Parse VMCreationPayload
//  3. Fetch ApprovalTicket for admin-determined fields (cluster, storage)
//  4. Build effective spec from payload + ticket modifications
//  5. Idempotency check: detect duplicate VM by event label
//  6. Execute K8s VM creation via VMService (outside transaction, ADR-0012)
//  7. Update event status to COMPLETED or FAILED
type VMCreateWorker struct {
	river.WorkerDefaults[VMCreateArgs]
	entClient   *ent.Client
	vmService   *service.VMService
	auditLogger *audit.Logger
}

// NewVMCreateWorker creates a new VMCreateWorker with all dependencies (ADR-0013 manual DI).
func NewVMCreateWorker(entClient *ent.Client, vmService *service.VMService, auditLogger *audit.Logger) *VMCreateWorker {
	return &VMCreateWorker{entClient: entClient, vmService: vmService, auditLogger: auditLogger}
}

// findCreatedVMByEvent performs an idempotency check by searching for an
// existing VM that was already created by a prior attempt with the same eventID.
func (w *VMCreateWorker) findCreatedVMByEvent(
	ctx context.Context,
	clusterID, namespace, eventID string,
) (*domain.VM, error) {
	list, err := w.vmService.ListVMs(ctx, clusterID, namespace, provider.ListOptions{
		LabelSelector: "shepherd.io/event-id=" + eventID,
		Limit:         1,
	})
	if err != nil {
		return nil, err
	}
	for _, candidate := range list.Items {
		if candidate == nil {
			continue
		}
		if candidate.Spec.Labels["shepherd.io/event-id"] == eventID {
			return candidate, nil
		}
	}
	return nil, nil
}

// Work executes the VM creation.
func (w *VMCreateWorker) Work(ctx context.Context, job *river.Job[VMCreateArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM creation job",
		zap.String("event_id", eventID),
		zap.Int64("attempt", int64(job.Attempt)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	if event.Status == domainevent.StatusCOMPLETED {
		logger.Info("vm create event already completed, skipping duplicate execution",
			zap.String("event_id", eventID),
		)
		return nil
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusEXECUTING)

	// Step 2: Parse payload.
	var payload domain.VMCreationPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		_, _ = w.entClient.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal payload for event %s: %w", eventID, err))
	}

	// Step 3: Fetch approval ticket for admin-determined fields.
	tickets, err := w.entClient.ApprovalTicket.Query().
		Where(approvalticket.EventIDEQ(eventID)).
		All(ctx)
	var clusterID, namespace string
	namespace = payload.Namespace
	if len(tickets) > 0 && err == nil {
		ticket := tickets[0]
		if ticket.SelectedClusterID != "" {
			clusterID = ticket.SelectedClusterID
		}
	}

	// Step 4: Build effective spec.
	spec := &domain.VMSpec{
		Labels: map[string]string{
			"shepherd.io/service-id":  payload.ServiceID,
			"shepherd.io/template-id": payload.TemplateID,
			"shepherd.io/event-id":    eventID,
		},
	}

	// Step 5: Idempotency check.
	// If a prior attempt already created this VM, detect it by event label and skip create.
	createdVM, err := w.findCreatedVMByEvent(ctx, clusterID, namespace, eventID)
	if err != nil {
		return fmt.Errorf("check vm create idempotency for event %s: %w", eventID, err)
	}

	var createdVMName string
	if createdVM != nil {
		createdVMName = createdVM.Name
		logger.Info("found existing VM for event, skipping duplicate create",
			zap.String("event_id", eventID),
			zap.String("vm_name", createdVMName),
		)
	} else {
		// Step 6: Execute K8s VM creation (outside transaction per ADR-0012).
		vmObj, err := w.vmService.ExecuteK8sCreate(ctx, clusterID, namespace, spec)
		if err != nil {
			// K8s VM was NOT created — safe to retry.
			// Persist FAILED status (best-effort; original error is returned regardless).
			if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
				SetStatus(domainevent.StatusFAILED).
				Save(ctx); saveErr != nil {
				logger.Error("failed to persist FAILED status for event",
					zap.String("event_id", eventID),
					zap.Error(saveErr),
				)
			}

			logAuditVMOp(ctx, w.auditLogger, "create_failed", eventID, "system", eventID)
			setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)

			return fmt.Errorf("execute k8s create for event %s: %w", eventID, err)
		}
		createdVMName = vmObj.Name
	}

	// Step 7: Update event status to COMPLETED.
	// We deliberately return error on persistence failure so River retries.
	// Retry is safe because idempotency check above prevents duplicate K8s create.
	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		return fmt.Errorf("persist COMPLETED status for event %s: %w", eventID, saveErr)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusSUCCESS)

	logAuditVMOp(ctx, w.auditLogger, "create", createdVMName, "system", eventID)

	logger.Info("VM creation job completed",
		zap.String("event_id", eventID),
		zap.String("vm_name", createdVMName),
		zap.String("cluster", clusterID),
	)

	return nil
}
