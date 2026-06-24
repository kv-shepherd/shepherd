package jobs

import (
	"context"
	"encoding/json"
	"errors"
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
	for _, executable := range vmDeleteExecutableStatuses() {
		if status == executable {
			return true
		}
	}
	return false
}

func vmDeleteExecutableStatuses() []vm.Status {
	return []vm.Status{
		vm.StatusSTOPPED,
		vm.StatusFAILED,
		vm.StatusNOT_FOUND,
		vm.StatusUNKNOWN,
		vm.StatusDELETING,
	}
}

func shouldSkipK8sDelete(status vm.Status) bool {
	return status == vm.StatusNOT_FOUND
}

func persistFinalDeleteFailure(ctx context.Context, client *ent.Client, eventID, vmID string) error {
	if client == nil {
		return nil
	}
	return persistFailedEventTicketAndVMByEvent(ctx, client, eventID, vmID)
}

type vmDeleteHardDeleteError struct {
	err error
}

func (e vmDeleteHardDeleteError) Error() string {
	return e.err.Error()
}

func (e vmDeleteHardDeleteError) Unwrap() error {
	return e.err
}

func persistCompletedDeleteState(ctx context.Context, client *ent.Client, eventID, vmID, vmName string) error {
	if client == nil || eventID == "" {
		return nil
	}
	err := withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if vmID != "" {
			if deleteErr := txClient.VM.DeleteOneID(vmID).Exec(ctx); deleteErr != nil {
				if ctxErr := jobContextErr(ctx, deleteErr); ctxErr != nil {
					return ctxErr
				}
				if ent.IsNotFound(deleteErr) {
					logger.Info("VM row already absent during completed delete persistence",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
					)
				} else {
					return vmDeleteHardDeleteError{err: fmt.Errorf("hard-delete VM row %s after delete event %s: %w", vmID, eventID, deleteErr)}
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
	if err == nil {
		return nil
	}
	var hardDeleteErr vmDeleteHardDeleteError
	if !errors.As(err, &hardDeleteErr) {
		return err
	}
	// K8s deletion already succeeded and the VM row was moved to DELETING before
	// the provider call. A real PostgreSQL delete error aborts its transaction,
	// so complete the Event/Ticket in a fresh transaction and keep the tombstone.
	logger.Error("CRITICAL: VM deleted in K8s but DB hard-delete failed, leaving DELETING tombstone",
		zap.String("event_id", eventID),
		zap.String("vm_id", vmID),
		zap.String("vm_name", vmName),
		zap.Error(hardDeleteErr.err))
	return persistCompletedEventAndTicketByEvent(ctx, client, eventID)
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
	if ticketStatus, ok := ticketStatusForTerminalDomainEvent(event.Status); ok {
		logger.Info("vm delete event already terminal, skipping duplicate execution",
			zap.String("event_id", eventID),
			zap.String("event_status", event.Status.String()),
		)
		if ticketErr := updateTicketStatusByEvent(ctx, w.entClient, eventID, ticketStatus); ticketErr != nil {
			if ctxErr := jobContextErr(ctx, ticketErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist %s ticket status for terminal delete event %s: %w", ticketStatus, eventID, ticketErr)
		}
		return nil
	}
	if persistErr := persistProcessingEventAndExecutingTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("persist PROCESSING/EXECUTING status for delete event %s: %w", eventID, persistErr)
	}

	// Step 2: Parse payload.
	var payload domain.VMDeletePayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		// Permanent failure — cancel job, don't retry corrupted data.
		if persistErr := persistFailedEventAndTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
			if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist FAILED status for malformed delete event %s: %w", eventID, persistErr)
		}
		return river.JobCancel(fmt.Errorf("unmarshal delete payload for event %s: %w", eventID, unmarshalErr))
	}

	// Step 3: Re-check persisted VM state before executing the destructive delete.
	skipK8sDelete := false
	currentVM, err := w.entClient.VM.Get(ctx, payload.VMID)
	switch {
	case err == nil:
		if !vmDeleteExecutableStatus(currentVM.Status) {
			if persistErr := persistFailedEventAndTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
				if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("persist FAILED status for rejected delete event %s before cancellation: %w", eventID, persistErr)
			}
			logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
			return river.JobCancel(fmt.Errorf(
				"refuse delete for vm %s in %s state, must be STOPPED, FAILED, NOT_FOUND, UNKNOWN, or DELETING",
				payload.VMID,
				currentVM.Status,
			))
		}
		skipK8sDelete = shouldSkipK8sDelete(currentVM.Status)
		if _, updateErr := w.entClient.VM.UpdateOneID(payload.VMID).
			Where(vm.StatusIn(vmDeleteExecutableStatuses()...)).
			SetStatus(vm.StatusDELETING).
			Save(ctx); updateErr != nil {
			if ctxErr := jobContextErr(ctx, updateErr); ctxErr != nil {
				return ctxErr
			}
			if ent.IsNotFound(updateErr) {
				refreshedVM, refreshErr := w.entClient.VM.Get(ctx, payload.VMID)
				switch {
				case ent.IsNotFound(refreshErr):
					logger.Info("vm record disappeared while claiming delete execution; continuing idempotent provider delete",
						zap.String("vm_id", payload.VMID),
						zap.String("event_id", eventID),
					)
				case refreshErr != nil:
					return fmt.Errorf("reload current vm %s after stale delete claim: %w", payload.VMID, refreshErr)
				default:
					if persistErr := persistFailedEventAndTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
						if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
							return ctxErr
						}
						return fmt.Errorf("persist FAILED status for stale delete event %s before cancellation: %w", eventID, persistErr)
					}
					logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
					return river.JobCancel(fmt.Errorf(
						"refuse delete for vm %s because status changed from %s to %s before delete execution",
						payload.VMID,
						currentVM.Status,
						refreshedVM.Status,
					))
				}
			} else {
				return fmt.Errorf("persist DELETING status for delete event %s vm %s: %w", eventID, payload.VMID, updateErr)
			}
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
			if ctxErr := jobContextErr(ctx, deleteErr); ctxErr != nil {
				return ctxErr
			}
			// If K8s reports NotFound, the resource is already gone — treat as success.
			if !k8serrors.IsNotFound(deleteErr) {
				if isClusterRuntimeUnavailable(deleteErr) {
					return snoozeClusterRuntimeUnavailable("vm_delete", eventID, payload.ClusterID, "execute_delete", deleteErr)
				}
				logAuditVMOp(ctx, w.auditLogger, "delete_failed", payload.VMName, payload.Actor, eventID)
				failureErr := fmt.Errorf("execute k8s delete for event %s: %w", eventID, deleteErr)
				if isFinalJobAttempt(job) {
					if persistErr := persistFinalDeleteFailure(ctx, w.entClient, eventID, payload.VMID); persistErr != nil {
						if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
							return ctxErr
						}
						return fmt.Errorf("persist final FAILED status for delete event %s: %w", eventID, persistErr)
					}
				}
				return failureErr
			}
			logger.Info("K8s VM already deleted (NotFound), proceeding with DB cleanup",
				zap.String("event_id", eventID),
				zap.String("vm_name", payload.VMName),
			)
		}
	}

	// Step 5: K8s deletion succeeded — persist VM cleanup plus terminal
	// event, ticket, and parent batch state together.
	if persistErr := persistCompletedDeleteState(ctx, w.entClient, eventID, payload.VMID, payload.VMName); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		logger.Error("CRITICAL: VM deleted but terminal state persistence failed",
			zap.String("event_id", eventID),
			zap.Error(persistErr),
		)
		return fmt.Errorf("persist completed delete state for event %s: %w", eventID, persistErr)
	}

	logAuditVMOp(ctx, w.auditLogger, "delete", payload.VMName, payload.Actor, eventID)

	logger.Info("VM deletion job completed",
		zap.String("event_id", eventID),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}
