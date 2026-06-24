package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

const (
	powerOpStart   = "start"
	powerOpStop    = "stop"
	powerOpRestart = "restart"
)

// ---------------------------------------------------------------------------
// Job Args
// ---------------------------------------------------------------------------

// VMPowerArgs carries only EventID. Operation and VM identity are resolved from
// the immutable DomainEvent payload (Claim-check pattern, ADR-0009).
type VMPowerArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM power operations.
func (VMPowerArgs) Kind() string { return "vm_power" }

// InsertOpts returns default insert options for VM power jobs.
func (VMPowerArgs) InsertOpts() river.InsertOpts {
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

// VMPowerWorker processes VM power operation jobs (start/stop/restart).
//
// Execution flow:
//  1. Fetch DomainEvent by EventID (claim-check, ADR-0009)
//  2. Parse VMPowerPayload
//  3. Execute K8s power operation via VMService (outside transaction, ADR-0012)
//  4. Update VM status in DB
//  5. Update event status to COMPLETED or FAILED
type VMPowerWorker struct {
	river.WorkerDefaults[VMPowerArgs]
	entClient   *ent.Client
	vmService   *service.VMService
	auditLogger *audit.Logger
}

// NewVMPowerWorker creates a new VMPowerWorker with all dependencies (ADR-0013 manual DI).
func NewVMPowerWorker(entClient *ent.Client, vmService *service.VMService, auditLogger *audit.Logger) *VMPowerWorker {
	return &VMPowerWorker{entClient: entClient, vmService: vmService, auditLogger: auditLogger}
}

// Work executes the VM power operation.
func (w *VMPowerWorker) Work(ctx context.Context, job *river.Job[VMPowerArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM power operation",
		zap.String("event_id", eventID),
		zap.Int64("attempt", jobAttempt(job)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	requireTicket, err := eventHasTicket(ctx, w.entClient, eventID)
	if err != nil {
		return fmt.Errorf("query ticket binding for power event %s: %w", eventID, err)
	}
	if ticketStatus, ok := ticketStatusForTerminalDomainEvent(event.Status); ok {
		logger.Info("vm power event already terminal, skipping duplicate execution",
			zap.String("event_id", eventID),
			zap.String("event_status", event.Status.String()),
		)
		if ticketErr := updateTicketStatusByEventMaybe(ctx, w.entClient, eventID, ticketStatus, requireTicket); ticketErr != nil {
			if ctxErr := jobContextErr(ctx, ticketErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist %s ticket status for terminal power event %s: %w", ticketStatus, eventID, ticketErr)
		}
		return nil
	}
	if persistErr := persistProcessingEventAndMaybeExecutingTicketByEvent(ctx, w.entClient, eventID, requireTicket); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("persist PROCESSING/EXECUTING status for power event %s: %w", eventID, persistErr)
	}

	// Step 2: Parse payload.
	var payload domain.VMPowerPayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		if persistErr := persistFailedEventAndMaybeTicketByEvent(ctx, w.entClient, eventID, requireTicket); persistErr != nil {
			if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist FAILED status for malformed power event %s: %w", eventID, persistErr)
		}
		return river.JobCancel(fmt.Errorf("unmarshal power payload for event %s: %w", eventID, unmarshalErr))
	}

	operation := strings.ToLower(strings.TrimSpace(payload.Operation))
	markFailed := func(err error, cancel bool) error {
		logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)

		if cancel {
			if persistErr := persistFailedEventAndMaybeTicketByEvent(ctx, w.entClient, eventID, requireTicket); persistErr != nil {
				if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("persist FAILED status for power event %s before cancellation: %w", eventID, persistErr)
			}
			return river.JobCancel(err)
		}
		if isFinalJobAttempt(job) {
			if persistErr := persistFailedEventAndMaybeTicketByEvent(ctx, w.entClient, eventID, requireTicket); persistErr != nil {
				if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("persist final FAILED status for power event %s: %w", eventID, persistErr)
			}
		}
		return err
	}

	currentVM, err := w.entClient.VM.Get(ctx, payload.VMID)
	switch {
	case err == nil:
		if currentVM.Status == vm.StatusDELETING {
			return markFailed(fmt.Errorf("vm %s is deleting", payload.VMID), true)
		}
	case ent.IsNotFound(err):
		return markFailed(fmt.Errorf("vm %s no longer exists", payload.VMID), true)
	default:
		return fmt.Errorf("query vm %s before power execution: %w", payload.VMID, err)
	}

	// Step 3: Execute K8s power operation (outside transaction per ADR-0012).
	var execErr error
	switch operation {
	case powerOpStart:
		execErr = w.vmService.StartVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	case powerOpStop:
		execErr = w.vmService.StopVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	case powerOpRestart:
		execErr = w.vmService.RestartVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	default:
		if persistErr := persistFailedEventAndMaybeTicketByEvent(ctx, w.entClient, eventID, requireTicket); persistErr != nil {
			if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist FAILED status for unknown power event %s: %w", eventID, persistErr)
		}
		return river.JobCancel(fmt.Errorf("unknown power operation: %s", operation))
	}
	if ctxErr := jobContextErr(ctx, execErr); ctxErr != nil {
		return ctxErr
	}

	// Idempotent conflict handling (phase-4 checklist): start/stop may race with
	// stale status reads and return "already running/stopped". Treat these as
	// successful no-op transitions instead of job failure.
	if isIdempotentPowerConflict(operation, execErr) {
		logger.Warn("VM power operation treated as idempotent no-op",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.String("vm_name", payload.VMName),
			zap.Error(execErr),
		)
		execErr = nil
	}

	if isTerminalPowerError(operation, execErr) {
		logger.Warn("VM power operation failed with terminal error, cancelling retries",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.String("vm_name", payload.VMName),
			zap.Error(execErr),
		)
		return markFailed(fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, execErr), true)
	}

	if execErr != nil {
		if isClusterRuntimeUnavailable(execErr) {
			return snoozeClusterRuntimeUnavailable("vm_power", eventID, payload.ClusterID, "execute_power", execErr)
		}
		return markFailed(fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, execErr), false)
	}

	// Step 4: Refresh the VM status from K8s after the lifecycle operation.
	// Do not assume RUNNING/STOPPED immediately after start/stop/restart:
	// KubeVirt may still report transitional state for a while.
	observedAt := time.Now()
	targetStatus := fallbackPowerOperationStatus(operation)
	targetRV := ""
	if liveVM, liveErr := w.vmService.GetVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName); liveErr != nil {
		if ctxErr := jobContextErr(ctx, liveErr); ctxErr != nil {
			return ctxErr
		}
		logger.Warn("VM power operation succeeded but live status refresh failed; using transitional fallback",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.String("vm_name", payload.VMName),
			zap.Error(liveErr),
		)
	} else {
		targetStatus = mapDomainStatusToEntVM(liveVM.Status)
		targetRV = strings.TrimSpace(liveVM.ResourceVersion)
	}
	targetTier := tierForStatus(targetStatus)
	targetInterval := intervalForTier(targetTier)

	// Step 5: Update VM, event, and ticket terminal state together.
	if persistErr := persistCompletedPowerState(ctx, w.entClient, eventID, payload.VMID, targetStatus, targetTier, targetInterval, observedAt, targetRV, requireTicket); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		var vmStatusErr powerVMStatusPersistError
		if errors.As(persistErr, &vmStatusErr) {
			logger.Error("CRITICAL: K8s power op succeeded but VM status update failed",
				zap.String("event_id", eventID),
				zap.String("operation", operation),
				zap.String("vm_name", payload.VMName),
				zap.Error(persistErr))
			if isRetryablePowerOperationAfterSuccess(operation) {
				return fmt.Errorf("persist VM status after %s power event %s: %w", operation, eventID, persistErr)
			}
			return river.JobCancel(fmt.Errorf("persist VM status after %s power event %s: %w", operation, eventID, persistErr))
		}
		logger.Error("CRITICAL: Power op completed but terminal status persistence failed",
			zap.String("event_id", eventID), zap.Error(persistErr))
		if isRetryablePowerOperationAfterSuccess(operation) {
			return fmt.Errorf("persist terminal status after %s power event %s: %w", operation, eventID, persistErr)
		}
		return river.JobCancel(fmt.Errorf("persist terminal status after %s power event %s: %w", operation, eventID, persistErr))
	}
	logAuditVMOp(ctx, w.auditLogger, operation, payload.VMName, payload.Actor, eventID)

	logger.Info("VM power operation completed",
		zap.String("event_id", eventID),
		zap.String("operation", operation),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}

type powerVMStatusPersistError struct {
	err error
}

func (e powerVMStatusPersistError) Error() string {
	return e.err.Error()
}

func (e powerVMStatusPersistError) Unwrap() error {
	return e.err
}

func persistCompletedPowerState(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	vmID string,
	targetStatus vm.Status,
	targetTier vm.PollingTier,
	targetInterval int,
	observedAt time.Time,
	resourceVersion string,
	requireTicket bool,
) error {
	if client == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, client, func(txClient *ent.Client) error {
		if strings.TrimSpace(vmID) != "" {
			update := txClient.VM.UpdateOneID(vmID).
				Where(vm.StatusNEQ(vm.StatusDELETING)).
				SetStatus(targetStatus).
				SetPollingTier(targetTier).
				SetPollIntervalSec(targetInterval)
			if targetTier == vm.PollingTierHigh {
				update = update.SetHighTierSince(observedAt)
			} else {
				update = update.ClearHighTierSince()
			}
			if strings.TrimSpace(resourceVersion) != "" {
				update = update.SetLastK8sRv(resourceVersion).SetLastPolledAt(observedAt)
			}
			if _, err := update.Save(ctx); err != nil {
				if ent.IsNotFound(err) {
					logger.Info("skipped completed power VM status persistence because row is delete-owned or absent",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
					)
				} else {
					return powerVMStatusPersistError{
						err: fmt.Errorf("update VM status during completed power persistence for event %s: %w", eventID, err),
					}
				}
			}
		}
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusCOMPLETED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClientMaybe(ctx, txClient, eventID, entticket.StatusSUCCESS, requireTicket); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

// fallbackPowerOperationStatus maps a power operation to a safe transitional
// status when a post-operation live refresh is unavailable.
func fallbackPowerOperationStatus(operation string) vm.Status {
	switch operation {
	case powerOpStart:
		return vm.StatusSTARTING
	case powerOpStop:
		return vm.StatusSTOPPING
	case powerOpRestart:
		return vm.StatusSTOPPING
	default:
		return vm.StatusUNKNOWN
	}
}

// isIdempotentPowerConflict reports whether a provider error is a benign
// idempotency conflict for start/stop operations.
func isIdempotentPowerConflict(operation string, err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch operation {
	case powerOpStart:
		return strings.Contains(msg, "already running")
	case powerOpStop:
		return strings.Contains(msg, "already stopped") ||
			strings.Contains(msg, "not running")
	default:
		return false
	}
}

func isRetryablePowerOperationAfterSuccess(operation string) bool {
	switch operation {
	case powerOpStart, powerOpStop:
		return true
	default:
		return false
	}
}

// isTerminalPowerError reports whether a provider error is deterministic and
// should not be retried by River.
func isTerminalPowerError(operation string, err error) bool {
	if isPowerTargetNotFound(err) {
		return true
	}
	// Restart on a non-running VM is a deterministic invalid-state conflict.
	// Retrying does not help and only adds noisy retries/errors in logs.
	if operation == powerOpRestart && isRestartStateConflict(err) {
		return true
	}
	if operation == powerOpStart && isManualPowerRequestUnsupported(err, "start") {
		return true
	}
	if operation == powerOpStop && isManualPowerRequestUnsupported(err, "stop") {
		return true
	}
	return false
}

// isPowerTargetNotFound reports whether a power-op target VM is missing on K8s.
func isPowerTargetNotFound(err error) bool {
	if err == nil {
		return false
	}
	if k8serrors.IsNotFound(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "not found") {
		return false
	}
	return strings.Contains(msg, "virtualmachine") || strings.Contains(msg, " vm ")
}

// isRestartStateConflict reports whether restart failed because the VM is not
// in a restartable running state.
func isRestartStateConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "does not support manual restart requests") {
		return true
	}
	if strings.Contains(msg, "vm is not running") {
		return true
	}
	return strings.Contains(msg, "virtualmachine") && strings.Contains(msg, "not running")
}

func isManualPowerRequestUnsupported(err error, operation string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(
		strings.ToLower(err.Error()),
		fmt.Sprintf("does not support manual %s requests", operation),
	)
}
