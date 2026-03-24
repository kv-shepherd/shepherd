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
	setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusEXECUTING)

	var payload domain.VMModifyPayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		_, _ = w.entClient.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal modify payload for event %s: %w", eventID, unmarshalErr))
	}

	markFailed := func(cause error, cancel bool) error {
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for modify event",
				zap.String("event_id", eventID),
				zap.Error(saveErr),
			)
		}
		setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusFAILED)
		logAuditVMOp(ctx, w.auditLogger, "modify_failed", payload.VMName, payload.Actor, eventID)
		if cancel {
			return river.JobCancel(cause)
		}
		return cause
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
		return markFailed(fmt.Errorf("cluster %s is not healthy", payload.ClusterID), true)
	}

	if capabilityErr := validateVMModifyClusterCapabilities(clusterRow, &payload); capabilityErr != nil {
		return markFailed(capabilityErr, true)
	}

	liveVM, err := w.vmService.GetVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	if err != nil {
		return markFailed(fmt.Errorf("load live vm %s/%s: %w", payload.Namespace, payload.VMName, err), false)
	}
	if liveVM == nil {
		return markFailed(fmt.Errorf("live vm %s/%s is nil", payload.Namespace, payload.VMName), true)
	}
	if liveVM.Name != payload.VMName || liveVM.Namespace != payload.Namespace {
		return markFailed(fmt.Errorf("live vm identity mismatch for %s", payload.VMID), true)
	}

	renderedPatch, err := provider.RenderVMLiveUpdatePatch(payload.Namespace, liveVM, provider.VMLiveUpdateTargets{
		CPUCores: payload.TargetCPUCores,
		MemoryGi: payload.TargetMemoryGi,
		DiskGB:   payload.TargetDiskGB,
	})
	if err != nil {
		return markFailed(fmt.Errorf("render live update patch: %w", err), true)
	}

	updatedVM, err := w.vmService.ExecuteK8sUpdate(ctx, payload.ClusterID, payload.Namespace, payload.VMName, &domain.VMSpec{
		Name:         payload.VMName,
		RenderedYAML: renderedPatch,
	})
	if err != nil {
		return markFailed(fmt.Errorf("execute k8s update for event %s: %w", eventID, err), false)
	}

	if persistErr := w.persistModifiedVMStatus(ctx, payload.VMID, updatedVM); persistErr != nil {
		logger.Error("VM modify succeeded but status sync persistence failed",
			zap.String("event_id", eventID),
			zap.String("vm_id", payload.VMID),
			zap.Error(persistErr),
		)
	}

	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		logger.Error("failed to persist COMPLETED status for modify event",
			zap.String("event_id", eventID),
			zap.Error(saveErr),
		)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, entticket.StatusSUCCESS)
	logAuditVMOp(ctx, w.auditLogger, "modify", payload.VMName, payload.Actor, eventID)

	logger.Info("VM modify job completed",
		zap.String("event_id", eventID),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}

func validateVMModifyClusterCapabilities(clusterRow *ent.Cluster, payload *domain.VMModifyPayload) error {
	if clusterRow == nil {
		return fmt.Errorf("cluster row is nil")
	}
	if payload == nil {
		return fmt.Errorf("modify payload is nil")
	}
	if (payload.TargetCPUCores != nil || payload.TargetMemoryGi != nil) &&
		!provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"VMLiveUpdateFeatures"}) {
		return fmt.Errorf("cluster %s does not support live CPU/memory updates", clusterRow.Name)
	}
	if payload.TargetDiskGB != nil &&
		!provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"ExpandDisks"}) {
		return fmt.Errorf("cluster %s does not support online disk expansion", clusterRow.Name)
	}
	return nil
}

func (w *VMModifyWorker) persistModifiedVMStatus(ctx context.Context, vmID string, updatedVM *domain.VM) error {
	if strings.TrimSpace(vmID) == "" || updatedVM == nil {
		return nil
	}
	observedAt := time.Now().UTC()
	targetStatus := mapDomainStatusToEntVM(updatedVM.Status)
	targetTier := tierForStatus(targetStatus)
	targetInterval := intervalForTier(targetTier)
	update := w.entClient.VM.UpdateOneID(vmID).
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
	return err
}
