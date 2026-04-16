package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
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

func vmDeleteExecutableStatus(status vm.Status) bool {
	switch status {
	case vm.StatusSTOPPED, vm.StatusFAILED, vm.StatusNOT_FOUND, vm.StatusUNKNOWN, vm.StatusDELETING:
		return true
	default:
		return false
	}
}

func shouldSkipK8sDelete(status vm.Status) bool {
	return status == vm.StatusNOT_FOUND
}

// Work executes the VM deletion.
func (w *VMDeleteWorker) Work(ctx context.Context, job *river.Job[VMDeleteArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM deletion job",
		zap.String("event_id", eventID),
		zap.Int64("attempt", jobAttempt(job)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusEXECUTING)

	// Step 2: Parse payload.
	var payload domain.VMDeletePayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		// Permanent failure — cancel job, don't retry corrupted data.
		_, _ = w.entClient.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal delete payload for event %s: %w", eventID, unmarshalErr))
	}

	// Step 3: Re-check persisted VM state before executing the destructive delete.
	skipK8sDelete := false
	currentVM, err := w.entClient.VM.Get(ctx, payload.VMID)
	switch {
	case err == nil:
		if !vmDeleteExecutableStatus(currentVM.Status) {
			if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
				SetStatus(domainevent.StatusFAILED).
				Save(ctx); saveErr != nil {
				logger.Error("failed to persist FAILED status for delete event rejected by runtime state guard",
					zap.String("event_id", eventID), zap.Error(saveErr))
			}
			setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusFAILED)
			logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
			return river.JobCancel(fmt.Errorf(
				"refuse delete for vm %s in %s state, must be STOPPED, FAILED, NOT_FOUND, UNKNOWN, or DELETING",
				payload.VMID,
				currentVM.Status,
			))
		}
		skipK8sDelete = shouldSkipK8sDelete(currentVM.Status)
		if _, updateErr := w.entClient.VM.UpdateOneID(payload.VMID).
			SetStatus(vm.StatusDELETING).
			Save(ctx); updateErr != nil {
			logger.Warn("failed to set VM status to DELETING (may already be deleted)",
				zap.String("vm_id", payload.VMID), zap.Error(updateErr))
		}
	case ent.IsNotFound(err):
		logger.Info("vm record already absent before delete execution, skipping status update",
			zap.String("vm_id", payload.VMID),
			zap.String("event_id", eventID),
		)
	default:
		return fmt.Errorf("fetch current vm %s before delete: %w", payload.VMID, err)
	}

	// Step 4: Execute K8s VM deletion (outside transaction per ADR-0012).
	if skipK8sDelete {
		logger.Info("Skipping K8s delete because VM is already absent",
			zap.String("event_id", eventID),
			zap.String("vm_id", payload.VMID),
			zap.String("vm_name", payload.VMName),
			zap.String("vm_status", string(currentVM.Status)),
		)
	} else {
		if deleteErr := w.vmService.DeleteVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName); deleteErr != nil {
			// If K8s reports NotFound, the resource is already gone — treat as success.
			if !k8serrors.IsNotFound(deleteErr) {
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
				setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusFAILED)

				logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
				return fmt.Errorf("execute k8s delete for event %s: %w", eventID, deleteErr)
			}
			logger.Info("K8s VM already deleted (NotFound), proceeding with DB cleanup",
				zap.String("event_id", eventID),
				zap.String("vm_name", payload.VMName),
			)
		}
	}

	// Step 5: K8s deletion succeeded — hard-delete VM record from DB.
	// CRITICAL: K8s resource is already deleted at this point.
	// If DB delete fails we MUST NOT return error (River retry would
	// re-execute K8s delete against a non-existent resource).
	if deleteErr := w.entClient.VM.DeleteOneID(payload.VMID).Exec(ctx); deleteErr != nil {
		// Fallback: if hard-delete fails (e.g. FK constraint), mark as DELETING tombstone.
		logger.Error("CRITICAL: VM deleted in K8s but DB hard-delete failed, falling back to tombstone",
			zap.String("event_id", eventID),
			zap.String("vm_name", payload.VMName),
			zap.Error(deleteErr))
		if _, saveErr := w.entClient.VM.UpdateOneID(payload.VMID).
			SetStatus(vm.StatusDELETING).
			Save(ctx); saveErr != nil {
			logger.Error("CRITICAL: VM tombstone fallback also failed",
				zap.String("event_id", eventID),
				zap.String("vm_name", payload.VMName),
				zap.Error(saveErr))
		}
	}

	// Step 6: Update event status to COMPLETED.
	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		logger.Error("CRITICAL: VM deleted but event status persistence failed",
			zap.String("event_id", eventID), zap.Error(saveErr))
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusSUCCESS)

	logAuditVMOp(ctx, w.auditLogger, "delete", payload.VMName, payload.Actor, eventID)

	logger.Info("VM deletion job completed",
		zap.String("event_id", eventID),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}
