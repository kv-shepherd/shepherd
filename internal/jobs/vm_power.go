package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
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

	// Step 2: Parse payload.
	var payload domain.VMPowerPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return river.JobCancel(fmt.Errorf("unmarshal power payload for event %s: %w", eventID, err))
	}

	// Use operation from Args (authoritative) over payload (informational).
	operation := job.Args.Operation

	// Step 3: Execute K8s power operation (outside transaction per ADR-0012).
	var execErr error
	switch operation {
	case "start":
		execErr = w.vmService.StartVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	case "stop":
		execErr = w.vmService.StopVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	case "restart":
		execErr = w.vmService.RestartVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	default:
		return river.JobCancel(fmt.Errorf("unknown power operation: %s", operation))
	}

	if execErr != nil {
		// K8s operation failed — persist FAILED status (best-effort).
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for power event",
				zap.String("event_id", eventID), zap.Error(saveErr))
		}

		logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
		return fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, execErr)
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
	case "start":
		return vm.StatusRUNNING
	case "stop":
		return vm.StatusSTOPPED
	case "restart":
		return vm.StatusRUNNING
	default:
		return vm.StatusUNKNOWN
	}
}
