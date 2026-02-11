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
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

// ---------------------------------------------------------------------------
// Job Args
// ---------------------------------------------------------------------------

// VMDeleteArgs carries EventID for VM deletion jobs (Claim-check, ADR-0009).
type VMDeleteArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM deletion.
func (VMDeleteArgs) Kind() string { return "vm_delete" }

// InsertOpts returns default insert options for VM deletion jobs.
func (VMDeleteArgs) InsertOpts() river.InsertOpts {
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

// VMDeleteWorker processes VM deletion jobs after approval.
//
// Execution flow (master-flow.md Stage 5.D):
//  1. Fetch DomainEvent by EventID (claim-check, ADR-0009)
//  2. Parse VMDeletePayload
//  3. Update VM status to DELETING
//  4. Execute K8s VM deletion via VMService (outside transaction, ADR-0012)
//  5. Persist terminal status (tombstone or FAILED)
//  6. Update event status to COMPLETED or FAILED
type VMDeleteWorker struct {
	river.WorkerDefaults[VMDeleteArgs]
	entClient   *ent.Client
	vmService   *service.VMService
	auditLogger *audit.Logger
}

// NewVMDeleteWorker creates a new VMDeleteWorker with all dependencies (ADR-0013 manual DI).
func NewVMDeleteWorker(entClient *ent.Client, vmService *service.VMService, auditLogger *audit.Logger) *VMDeleteWorker {
	return &VMDeleteWorker{entClient: entClient, vmService: vmService, auditLogger: auditLogger}
}

// Work executes the VM deletion.
func (w *VMDeleteWorker) Work(ctx context.Context, job *river.Job[VMDeleteArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM deletion job",
		zap.String("event_id", eventID),
		zap.Int64("attempt", int64(job.Attempt)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusEXECUTING)

	// Step 2: Parse payload.
	var payload domain.VMDeletePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		// Permanent failure — cancel job, don't retry corrupted data.
		_, _ = w.entClient.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal delete payload for event %s: %w", eventID, err))
	}

	// Step 3: Update VM status to DELETING.
	if _, err := w.entClient.VM.UpdateOneID(payload.VMID).
		SetStatus(vm.StatusDELETING).
		Save(ctx); err != nil {
		logger.Warn("failed to set VM status to DELETING (may already be deleted)",
			zap.String("vm_id", payload.VMID), zap.Error(err))
	}

	// Step 4: Execute K8s VM deletion (outside transaction per ADR-0012).
	if err := w.vmService.DeleteVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName); err != nil {
		// K8s deletion failed — persist FAILED status (best-effort).
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for delete event",
				zap.String("event_id", eventID), zap.Error(saveErr))
		}

		// Update VM status to FAILED.
		if _, saveErr := w.entClient.VM.UpdateOneID(payload.VMID).
			SetStatus(vm.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist VM FAILED status",
				zap.String("vm_id", payload.VMID), zap.Error(saveErr))
		}
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)

		logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
		return fmt.Errorf("execute k8s delete for event %s: %w", eventID, err)
	}

	// Step 5: K8s deletion succeeded — mark VM as tombstone.
	// CRITICAL: K8s resource is already deleted at this point.
	// If DB update fails we MUST NOT return error (River retry would
	// re-execute K8s delete against a non-existent resource).
	if _, saveErr := w.entClient.VM.UpdateOneID(payload.VMID).
		SetStatus(vm.StatusDELETING). // Tombstone; can be hard-deleted by background job
		Save(ctx); saveErr != nil {
		logger.Error("CRITICAL: VM deleted in K8s but DB status update failed",
			zap.String("event_id", eventID),
			zap.String("vm_name", payload.VMName),
			zap.Error(saveErr))
	}

	// Step 6: Update event status to COMPLETED.
	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		logger.Error("CRITICAL: VM deleted but event status persistence failed",
			zap.String("event_id", eventID), zap.Error(saveErr))
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusSUCCESS)

	logAuditVMOp(ctx, w.auditLogger, "delete", payload.VMName, payload.Actor, eventID)

	logger.Info("VM deletion job completed",
		zap.String("event_id", eventID),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}
