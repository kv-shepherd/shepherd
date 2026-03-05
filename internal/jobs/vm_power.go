package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riverqueue/river"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
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

// VMPowerArgs carries EventID and operation type for VM power jobs (Claim-check, ADR-0009).
type VMPowerArgs struct {
	EventID   string `json:"event_id"`
	Operation string `json:"operation"` // start, stop, restart
}

// Kind returns the job kind identifier for VM power operations.
func (VMPowerArgs) Kind() string { return "vm_power" }

// InsertOpts returns default insert options for VM power jobs.
func (VMPowerArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "vm_operations",
		MaxAttempts: 3,
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
		zap.String("operation", job.Args.Operation),
		zap.Int64("attempt", int64(job.Attempt)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusEXECUTING)

	// Step 2: Parse payload.
	var payload domain.VMPowerPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for malformed power payload",
				zap.String("event_id", eventID), zap.Error(saveErr))
		}
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal power payload for event %s: %w", eventID, err))
	}

	// Use operation from Args (authoritative) over payload (informational).
	operation := job.Args.Operation
	markFailed := func(err error, cancel bool) error {
		// K8s operation failed — persist FAILED status (best-effort).
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for power event",
				zap.String("event_id", eventID), zap.Error(saveErr))
		}

		logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)

		if cancel {
			return river.JobCancel(err)
		}
		return err
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
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for unknown power operation",
				zap.String("event_id", eventID), zap.Error(saveErr))
		}
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unknown power operation: %s", operation))
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
		return markFailed(fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, execErr), false)
	}

	// Step 4: Update VM status in DB based on operation.
	// CRITICAL: K8s operation already executed.
	targetStatus := operationToStatus(operation)
	if _, saveErr := w.entClient.VM.UpdateOneID(payload.VMID).
		SetStatus(targetStatus).
		Save(ctx); saveErr != nil {
		logger.Error("CRITICAL: K8s power op succeeded but VM status update failed",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.String("vm_name", payload.VMName),
			zap.Error(saveErr))
	}

	// Step 5: Update event status to COMPLETED.
	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		logger.Error("CRITICAL: Power op completed but event status persistence failed",
			zap.String("event_id", eventID), zap.Error(saveErr))
	}

	logAuditVMOp(ctx, w.auditLogger, operation, payload.VMName, payload.Actor, eventID)
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusSUCCESS)

	logger.Info("VM power operation completed",
		zap.String("event_id", eventID),
		zap.String("operation", operation),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}

// operationToStatus maps a power operation to the expected VM status after execution.
func operationToStatus(operation string) vm.Status {
	switch operation {
	case powerOpStart:
		return vm.StatusRUNNING
	case powerOpStop:
		return vm.StatusSTOPPED
	case powerOpRestart:
		return vm.StatusRUNNING
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
		return strings.Contains(msg, "already running") ||
			strings.Contains(msg, "does not support manual start requests")
	case powerOpStop:
		return strings.Contains(msg, "already stopped") ||
			strings.Contains(msg, "not running") ||
			strings.Contains(msg, "does not support manual stop requests")
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
