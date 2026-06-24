package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

// VMModifyArgs carries EventID for VM resource update jobs.
type VMModifyArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM resource updates.
func (VMModifyArgs) Kind() string { return "vm_modify" }

// InsertOpts returns default insert options for VM modify jobs.
func (VMModifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "vm_operations",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	}
}

// VMModifyWorker processes VM resource modification jobs after approval.
type VMModifyWorker struct {
	river.WorkerDefaults[VMModifyArgs]
	entClient   *ent.Client
	vmService   *service.VMService
	auditLogger *audit.Logger
}

// NewVMModifyWorker creates a new VMModifyWorker.
func NewVMModifyWorker(entClient *ent.Client, vmService *service.VMService, auditLogger *audit.Logger) *VMModifyWorker {
	return &VMModifyWorker{entClient: entClient, vmService: vmService, auditLogger: auditLogger}
}

// Work executes the VM live-update flow.
func (w *VMModifyWorker) Work(ctx context.Context, job *river.Job[VMModifyArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM modify job",
		zap.String("event_id", eventID),
		zap.Int64("attempt", jobAttempt(job)),
	)

	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	if ticketStatus, ok := ticketStatusForTerminalDomainEvent(event.Status); ok {
		logger.Info("vm modify event already terminal, skipping duplicate execution",
			zap.String("event_id", eventID),
			zap.String("event_status", event.Status.String()),
		)
		if ticketErr := updateTicketStatusByEvent(ctx, w.entClient, eventID, ticketStatus); ticketErr != nil {
			if ctxErr := jobContextErr(ctx, ticketErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist %s ticket status for terminal modify event %s: %w", ticketStatus, eventID, ticketErr)
		}
		return nil
	}
	if persistErr := persistProcessingEventAndExecutingTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("persist PROCESSING/EXECUTING status for modify event %s: %w", eventID, persistErr)
	}
	ticket, _ := w.entClient.Ticket.Query().
		Where(entticket.EventIDEQ(eventID), entticket.OperationTypeEQ(entticket.OperationTypeMODIFY)).
		Only(ctx)

	var payload domain.VMModifyPayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		if persistErr := persistFailedEventAndTicketByEvent(ctx, w.entClient, eventID); persistErr != nil {
			if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("persist FAILED status for malformed modify event %s: %w", eventID, persistErr)
		}
		return river.JobCancel(fmt.Errorf("unmarshal modify payload for event %s: %w", eventID, unmarshalErr))
	}

	markFailed := func(cause error, cancel bool) error {
		logAuditVMOp(ctx, w.auditLogger, "modify_failed", payload.VMName, payload.Actor, eventID)
		if cancel {
			if persistErr := persistFinalModifyFailure(ctx, w.entClient, eventID, cause); persistErr != nil {
				if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("persist FAILED status for modify event %s before cancellation: %w", eventID, persistErr)
			}
			return river.JobCancel(cause)
		}
		if isFinalJobAttempt(job) {
			if persistErr := persistFinalModifyFailure(ctx, w.entClient, eventID, cause); persistErr != nil {
				if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("persist final FAILED status for modify event %s: %w", eventID, persistErr)
			}
		}
		return cause
	}

	currentVM, err := w.entClient.VM.Get(ctx, payload.VMID)
	switch {
	case err == nil:
		if currentVM.Status == entvm.StatusDELETING {
			return markFailed(fmt.Errorf("vm %s is deleting", payload.VMID), true)
		}
	case ent.IsNotFound(err):
		return markFailed(fmt.Errorf("vm %s no longer exists", payload.VMID), true)
	default:
		return fmt.Errorf("query vm %s before modify execution: %w", payload.VMID, err)
	}

	clusterRow, err := w.entClient.Cluster.Get(ctx, payload.ClusterID)
	if err != nil {
		if ent.IsNotFound(err) {
			return markFailed(fmt.Errorf("cluster %s not found", payload.ClusterID), true)
		}
		return fmt.Errorf("query cluster %s: %w", payload.ClusterID, err)
	}
	if !clusterRow.Enabled {
		return markFailed(fmt.Errorf("cluster %s is disabled", payload.ClusterID), true)
	}
	if clusterRow.Status != entcluster.StatusHEALTHY {
		return snoozeClusterRuntimeUnavailable(
			"vm_modify",
			eventID,
			payload.ClusterID,
			"selected_cluster_status",
			fmt.Errorf("cluster %s is not healthy (status: %s)", payload.ClusterID, clusterRow.Status),
		)
	}

	liveVM, err := w.vmService.GetVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	if err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		if isClusterRuntimeUnavailable(err) {
			return snoozeClusterRuntimeUnavailable("vm_modify", eventID, payload.ClusterID, "load_live_vm", err)
		}
		return markFailed(fmt.Errorf("load live vm %s/%s: %w", payload.Namespace, payload.VMName, err), false)
	}
	if liveVM == nil {
		return markFailed(fmt.Errorf("live vm %s/%s is nil", payload.Namespace, payload.VMName), true)
	}
	if liveVM.Name != payload.VMName || liveVM.Namespace != payload.Namespace {
		return markFailed(fmt.Errorf("live vm identity mismatch for %s", payload.VMID), true)
	}
	if capabilityErr := validateVMModifyClusterCapabilities(clusterRow, liveVM, &payload); capabilityErr != nil {
		return markFailed(capabilityErr, true)
	}

	overrideCPURequest, overrideMemoryRequest := approvedModifyRequestOverrides(ticket)
	plan, err := approvedModifyMutationPlan(ticket)
	if err != nil {
		return markFailed(fmt.Errorf("load approved vm mutation: %w", err), true)
	}
	if plan == nil {
		plan, err = provider.PlanVMResourceUpdatePatch(payload.Namespace, liveVM, provider.VMLiveUpdateTargets{
			CPUCores:        payload.TargetCPUCores,
			MemoryGi:        payload.TargetMemoryGi,
			DiskGB:          payload.TargetDiskGB,
			CPURequest:      overrideCPURequest,
			MemoryRequestGi: overrideMemoryRequest,
		})
		if err != nil {
			return markFailed(fmt.Errorf("plan vm resource update patch: %w", err), true)
		}
	}

	updatedVM, err := w.vmService.ExecuteVMMutation(ctx, payload.ClusterID, payload.Namespace, payload.VMName, plan.Mutation)
	if err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		if isClusterRuntimeUnavailable(err) {
			return snoozeClusterRuntimeUnavailable("vm_modify", eventID, payload.ClusterID, "execute_mutation", err)
		}
		return markFailed(fmt.Errorf("execute vm mutation for event %s: %w", eventID, err), false)
	}

	if persistErr := w.persistCompletedModifyState(ctx, eventID, payload.VMID, updatedVM); persistErr != nil {
		if ctxErr := jobContextErr(ctx, persistErr); ctxErr != nil {
			return ctxErr
		}
		logger.Error("VM modify succeeded but terminal state persistence failed",
			zap.String("event_id", eventID),
			zap.String("vm_id", payload.VMID),
			zap.Error(persistErr),
		)
		return fmt.Errorf("persist completed modify state for event %s (vm_id=%s): %w", eventID, payload.VMID, persistErr)
	}
	logAuditVMOp(ctx, w.auditLogger, "modify", payload.VMName, payload.Actor, eventID)

	logger.Info("VM modify job completed",
		zap.String("event_id", eventID),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}

func persistFinalModifyFailure(ctx context.Context, client *ent.Client, eventID string, cause error) error {
	if client == nil {
		return nil
	}
	return persistFailedEventAndTicketByEventWithRejectReason(ctx, client, eventID, strings.TrimSpace(cause.Error()))
}

func validateVMModifyClusterCapabilities(clusterRow *ent.Cluster, liveVM *domain.VM, payload *domain.VMModifyPayload) error {
	if clusterRow == nil {
		return fmt.Errorf("cluster row is nil")
	}
	if liveVM == nil {
		return fmt.Errorf("live vm is nil")
	}
	if payload == nil {
		return fmt.Errorf("modify payload is nil")
	}
	if payload.TargetDiskGB != nil &&
		!provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"ExpandDisks"}) {
		return fmt.Errorf("cluster %s does not support online disk expansion", clusterRow.Name)
	}
	return nil
}

func approvedModifyRequestOverrides(ticket *ent.Ticket) (cpuRequest, memoryRequest *float64) {
	if ticket == nil || len(ticket.ModifiedSpec) == 0 {
		return nil, nil
	}
	if value, ok := ticket.ModifiedSpec["cpu_request"].(float64); ok && value > 0 {
		cpuRequest = &value
	}
	if value, ok := ticket.ModifiedSpec["memory_request_gi"].(float64); ok && value > 0 {
		memoryRequest = &value
	}
	return cpuRequest, memoryRequest
}

func approvedModifyMutationPlan(ticket *ent.Ticket) (*provider.VMResourceUpdatePlan, error) {
	if ticket == nil || len(ticket.ModifiedSpec) == 0 {
		return nil, nil
	}
	raw, ok := ticket.ModifiedSpec["vm_mutation"]
	if !ok {
		return nil, nil
	}
	snapshot, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("vm_mutation snapshot has unexpected type %T", raw)
	}
	mutation, err := domain.VMMutationFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	plan := &provider.VMResourceUpdatePlan{
		Mutation: mutation,
	}
	if applyMode, ok := ticket.ModifiedSpec["apply_mode"].(string); ok {
		plan.ApplyMode = applyMode
	}
	if requiresRestart, ok := ticket.ModifiedSpec["requires_restart"].(bool); ok {
		plan.RequiresRestart = requiresRestart
	}
	return plan, nil
}

func (w *VMModifyWorker) persistCompletedModifyState(ctx context.Context, eventID, vmID string, updatedVM *domain.VM) error {
	if w.entClient == nil || eventID == "" {
		return nil
	}
	return withJobsTx(ctx, w.entClient, func(txClient *ent.Client) error {
		if err := persistModifiedVMStatusWithClient(ctx, txClient, vmID, updatedVM); err != nil {
			return err
		}
		if err := updateDomainEventStatusWithExpected(ctx, txClient, eventID, domainevent.StatusCOMPLETED, domainevent.StatusPROCESSING); err != nil {
			return err
		}
		if err := updateTicketStatusByEventWithClient(ctx, txClient, eventID, entticket.StatusSUCCESS); err != nil {
			return err
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func persistModifiedVMStatusWithClient(ctx context.Context, client *ent.Client, vmID string, updatedVM *domain.VM) error {
	if client == nil {
		return nil
	}
	if strings.TrimSpace(vmID) == "" || updatedVM == nil {
		return nil
	}
	observedAt := time.Now().UTC()
	targetStatus := mapDomainStatusToEntVM(updatedVM.Status)
	targetTier := tierForStatus(targetStatus)
	targetInterval := intervalForTier(targetTier)
	update := client.VM.UpdateOneID(vmID).
		Where(entvm.StatusNEQ(entvm.StatusDELETING)).
		SetStatus(targetStatus).
		SetPollingTier(targetTier).
		SetPollIntervalSec(targetInterval)
	if targetTier == entvm.PollingTierHigh {
		update = update.SetHighTierSince(observedAt)
	} else {
		update = update.ClearHighTierSince()
	}
	if strings.TrimSpace(updatedVM.ResourceVersion) != "" {
		update = update.SetLastK8sRv(updatedVM.ResourceVersion).SetLastPolledAt(observedAt)
	}
	_, err := update.Save(ctx)
	if ent.IsNotFound(err) {
		logger.Info("skipped completed modify VM status persistence because row is delete-owned or absent",
			zap.String("vm_id", vmID),
		)
		return nil
	}
	return err
}
