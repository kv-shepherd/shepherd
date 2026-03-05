package jobs

// vm_status_sync.go — ADR-0038 Phase 2: Adaptive K8s VM Status Polling Worker
//
// This River Worker periodically polls K8s for each managed VM's current status
// and persists it to the database. It implements:
//
//   - State-machine-driven polling tiers (high = ≤15s, low = ≥30min)
//   - Mandatory ResourceVersion caching to avoid etcd penetration
//   - Auto-downgrade: transitional VMs stuck >30min are downgraded to low tier
//   - Self-rescheduling: each execution inserts the next job with ScheduledAt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

// ---------------------------------------------------------------------------
// Constants (ADR-0038 §Polling frequency tiers)
// ---------------------------------------------------------------------------

const (
	// VMStatusSyncJobKind is the River kind/queue name for vm status sync jobs.
	VMStatusSyncJobKind = "vm_status_sync"

	// highTierIntervalSec is the poll interval for transitional VMs (CREATING, DELETING, etc.).
	highTierIntervalSec = 15
	// lowTierIntervalSec is the poll interval for stable VMs (RUNNING, STOPPED, FAILED).
	lowTierIntervalSec = 1800 // 30 minutes
	// createBootstrapGraceWindow is the maximum duration we treat STOPPED/UNKNOWN
	// as create-startup transient noise for newly approved VMs.
	//
	// Rationale:
	// - Stage 5.C mandates create flow convergence to RUNNING|FAILED.
	// - KubeVirt may briefly report printableStatus=Stopped right after create while
	//   VM/VMI startup is still converging.
	// - Downgrading to low-tier too early can freeze stale STOPPED for 30 minutes.
	createBootstrapGraceWindow = 2 * time.Minute
	// autoDowngradeThreshold is the duration after which a transitional VM stuck in
	// high-frequency polling is auto-downgraded to low-frequency (ADR-0038 §Auto-downgrade).
	autoDowngradeThreshold = 30 * time.Minute
)

// ---------------------------------------------------------------------------
// Job Args
// ---------------------------------------------------------------------------

// VMStatusSyncArgs carries only EventID (claim-check pattern, ADR-0009).
// VM identity is resolved from DB at runtime: EventID -> ApprovalTicket -> VM row.
type VMStatusSyncArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM status sync.
func (VMStatusSyncArgs) Kind() string { return VMStatusSyncJobKind }

// Note: VMStatusSyncArgs intentionally does NOT implement JobArgsWithInsertOpts.
// All insert options (Queue, MaxAttempts, UniqueOpts, ScheduledAt) are managed
// exclusively by scheduleNext() — the single source of truth — to avoid DRY
// violations between the method-level defaults and the per-insert overrides.
// See River docs: when both InsertOpts() and Insert() opts are present, the
// Insert() parameter silently overrides, which can mask configuration drift.

// ---------------------------------------------------------------------------
// Worker
// ---------------------------------------------------------------------------

// VMStatusSyncWorker polls K8s for a single VM's status and persists it.
//
// Execution flow (ADR-0038 Phase 2):
//  1. Resolve VM row from EventID (EventID -> ApprovalTicket -> VM)
//  2. Call K8s List (metadata.name filter + resourceVersion) via VMService
//  3. Map K8s status → DB status; detect tier transition
//  4. Persist: status, last_k8s_rv, last_polled_at, polling_tier, poll_interval_sec
//  5. Schedule next poll job with ScheduledAt = now + poll_interval_sec
type VMStatusSyncWorker struct {
	river.WorkerDefaults[VMStatusSyncArgs]
	entClient           *ent.Client
	vmService           *service.VMService
	riverClientProvider func() *river.Client[pgx.Tx] // for self-rescheduling
}

// NewVMStatusSyncWorker creates a new VMStatusSyncWorker with all dependencies.
func NewVMStatusSyncWorker(
	entClient *ent.Client,
	vmService *service.VMService,
	riverClientProvider func() *river.Client[pgx.Tx],
) *VMStatusSyncWorker {
	return &VMStatusSyncWorker{
		entClient:           entClient,
		vmService:           vmService,
		riverClientProvider: riverClientProvider,
	}
}

// Work executes a single VM status sync poll.
func (w *VMStatusSyncWorker) Work(ctx context.Context, job *river.Job[VMStatusSyncArgs]) error {
	eventID := strings.TrimSpace(job.Args.EventID)
	if eventID == "" {
		return river.JobCancel(fmt.Errorf("vm_status_sync: empty event_id"))
	}

	// Step 1: Resolve VM row from EventID (claim-check).
	vmRow, err := w.resolveVMByEventID(ctx, eventID)
	if err != nil {
		if ent.IsNotFound(err) {
			// Ticket/VM deleted from DB — stop polling (cancel, do not retry).
			logger.Info("vm_status_sync: event has no active VM binding, cancelling poll chain",
				zap.String("event_id", eventID),
			)
			return river.JobCancel(fmt.Errorf("event %s has no active vm", eventID))
		}
		return fmt.Errorf("vm_status_sync: resolve vm by event %s: %w", eventID, err)
	}
	vmID := vmRow.ID

	clusterID := strings.TrimSpace(vmRow.ClusterID)
	if clusterID == "" {
		// VM not yet assigned to a cluster — skip, reschedule at low frequency.
		logger.Debug("vm_status_sync: VM has no cluster_id, skipping poll",
			zap.String("vm_id", vmID),
		)
		return w.scheduleNext(ctx, eventID, lowTierIntervalSec)
	}

	// Terminal states: stop the poll chain entirely.
	if vmRow.Status == vm.StatusDELETING {
		// DELETING VMs are handled by the VMDeleteWorker; stop sync polling.
		logger.Debug("vm_status_sync: VM is DELETING, stopping poll chain",
			zap.String("vm_id", vmID),
		)
		return nil // no reschedule
	}

	// Step 2: Call K8s List with metadata.name selector and cached resourceVersion.
	// This is ADR-0038's required watch-cache-friendly polling pattern.
	//
	// Note on Limit+ResourceVersion interaction (K8s API semantics):
	//   When Limit and ResourceVersion are both set, K8s may return more results
	//   than Limit if the watch cache has already received updates beyond the
	//   requested ResourceVersion. In practice this is a no-op for us because
	//   FieldSelector="metadata.name=<name>" constrains the result to at most 1
	//   item. The Limit=1 is therefore a defensive guard, not a pagination
	//   mechanism.
	lastRV := ""
	if vmRow.LastK8sRv != nil {
		lastRV = strings.TrimSpace(*vmRow.LastK8sRv)
	}
	vmList, err := w.vmService.ListVMs(ctx, clusterID, vmRow.Namespace, provider.ListOptions{
		FieldSelector:     "metadata.name=" + vmRow.Name,
		Limit:             1,
		ResourceVersion:   lastRV,
		SkipVMIEnrichment: true, // status sync polls only VM status; avoid extra VMI List load.
	})
	if err != nil {
		// ADR-0038: if resourceVersion is expired (410 Gone), clear the cached RV so
		// next poll re-establishes the baseline with resourceVersion="".
		if k8serrors.IsResourceExpired(err) || k8serrors.IsGone(err) {
			logger.Warn("vm_status_sync: cached resourceVersion expired, resetting baseline",
				zap.String("vm_id", vmID),
				zap.String("event_id", eventID),
				zap.String("stale_rv", lastRV),
				zap.Error(err),
			)
			if _, saveErr := w.entClient.VM.UpdateOneID(vmID).
				ClearLastK8sRv().
				SetLastPolledAt(time.Now()).
				Save(ctx); saveErr != nil {
				return fmt.Errorf("vm_status_sync: clear stale resourceVersion for vm %s: %w", vmID, saveErr)
			}
			return w.scheduleNext(ctx, eventID, vmRow.PollIntervalSec)
		}

		// K8s unreachable or API failure — log and retry.
		logger.Warn("vm_status_sync: K8s ListVMs failed, will retry",
			zap.String("vm_id", vmID),
			zap.String("cluster", clusterID),
			zap.Error(err),
		)
		// Reschedule at the same interval — transient failures should not change tier.
		return w.scheduleNext(ctx, eventID, vmRow.PollIntervalSec)
	}
	if vmList == nil || len(vmList.Items) == 0 || vmList.Items[0] == nil {
		// Create flow race is expected: VM row exists but K8s object not yet visible.
		logger.Warn("vm_status_sync: VM not visible on cluster yet, will retry",
			zap.String("vm_id", vmID),
			zap.String("cluster", clusterID),
			zap.String("namespace", vmRow.Namespace),
			zap.String("name", vmRow.Name),
		)
		return w.scheduleNext(ctx, eventID, vmRow.PollIntervalSec)
	}
	domainVM := vmList.Items[0]

	// Step 3: Determine new DB status and polling tier.
	now := time.Now()
	observedStatus := mapDomainStatusToEntVM(domainVM.Status)
	newStatus := reconcileCreateBootstrapStatus(vmRow, observedStatus, now)
	newTier := tierForStatus(newStatus)
	newInterval := intervalForTier(newTier)
	highTierSince := deriveHighTierSince(vmRow, newTier, now)

	// Auto-downgrade check (ADR-0038 §Auto-downgrade):
	// If VM remains in high tier >30min, downgrade to low frequency.
	if shouldAutoDowngrade(newTier, highTierSince, now) {
		stuckDuration := now.Sub(*highTierSince)
		logger.Warn("vm_status_sync: auto-downgrading stuck transitional VM to low-frequency",
			zap.String("vm_id", vmID),
			zap.String("status", string(newStatus)),
			zap.Duration("stuck_duration", stuckDuration),
		)
		newTier = vm.PollingTierLow
		newInterval = lowTierIntervalSec
		highTierSince = nil
	}

	// Step 4: Persist updates to DB.
	update := w.entClient.VM.UpdateOneID(vmID).
		SetStatus(newStatus).
		SetPollingTier(newTier).
		SetPollIntervalSec(newInterval).
		SetLastPolledAt(now)

	if highTierSince == nil {
		update = update.ClearHighTierSince()
	} else {
		update = update.SetHighTierSince(*highTierSince)
	}

	// ADR-0038 §ResourceVersion mandatory requirement:
	// Persist the new ResourceVersion from the K8s response.
	if domainVM.ResourceVersion != "" {
		update = update.SetLastK8sRv(domainVM.ResourceVersion)
	}

	if _, err := update.Save(ctx); err != nil {
		return fmt.Errorf("vm_status_sync: persist status for vm %s: %w", vmID, err)
	}

	logger.Debug("vm_status_sync: status synced",
		zap.String("vm_id", vmID),
		zap.String("event_id", eventID),
		zap.String("old_status", string(vmRow.Status)),
		zap.String("observed_status", string(observedStatus)),
		zap.String("new_status", string(newStatus)),
		zap.String("tier", string(newTier)),
		zap.Int("interval_sec", newInterval),
	)

	// Step 5: Schedule next poll.
	return w.scheduleNext(ctx, eventID, newInterval)
}

// scheduleNext inserts the next VMStatusSyncArgs job with ScheduledAt.
func (w *VMStatusSyncWorker) scheduleNext(ctx context.Context, eventID string, intervalSec int) error {
	if w == nil || w.riverClientProvider == nil {
		// Graceful degradation: if River client provider is not wired, skip rescheduling.
		logger.Warn("vm_status_sync: river client provider is nil, cannot schedule next poll",
			zap.String("event_id", eventID),
		)
		return nil
	}
	riverClient := w.riverClientProvider()
	if riverClient == nil {
		// Graceful degradation: if River client is not wired, skip rescheduling.
		logger.Warn("vm_status_sync: river client is nil, cannot schedule next poll",
			zap.String("event_id", eventID),
		)
		return nil
	}

	scheduledAt := time.Now().Add(time.Duration(intervalSec) * time.Second)
	_, err := riverClient.Insert(ctx, VMStatusSyncArgs{EventID: eventID}, &river.InsertOpts{
		Queue:       VMStatusSyncJobKind,
		MaxAttempts: 3,
		ScheduledAt: scheduledAt,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
			// Explicit ByState per River best practice: prevent implicit default
			// confusion. Include all active states so a scheduled/pending/running
			// job for the same EventID blocks duplicate inserts. Completed jobs
			// are excluded so the next poll cycle can always be scheduled after
			// the current one finishes.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	})
	if err != nil {
		logger.Warn("vm_status_sync: failed to schedule next poll",
			zap.String("event_id", eventID),
			zap.Error(err),
		)
		return fmt.Errorf("vm_status_sync: schedule next poll for event %s: %w", eventID, err)
	}
	return nil
}

// resolveVMByEventID resolves the current VM row associated with an EventID.
// Lookup path (claim-check): EventID -> ApprovalTicket -> VM(ticket_id).
func (w *VMStatusSyncWorker) resolveVMByEventID(ctx context.Context, eventID string) (*ent.VM, error) {
	ticket, err := w.entClient.ApprovalTicket.Query().
		Where(approvalticket.EventIDEQ(eventID)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return w.entClient.VM.Query().
		Where(vm.TicketIDEQ(ticket.ID)).
		Only(ctx)
}

// ---------------------------------------------------------------------------
// Tier / Status mapping helpers
// ---------------------------------------------------------------------------

// tierForStatus returns the appropriate polling tier for a given VM status.
// Transitional states → high-frequency; stable states → low-frequency.
func tierForStatus(status vm.Status) vm.PollingTier {
	switch status {
	case vm.StatusCREATING, vm.StatusDELETING, vm.StatusSTOPPING, vm.StatusMIGRATING, vm.StatusPENDING:
		return vm.PollingTierHigh
	case vm.StatusRUNNING, vm.StatusSTOPPED, vm.StatusFAILED, vm.StatusPAUSED, vm.StatusUNKNOWN:
		return vm.PollingTierLow
	default:
		return vm.PollingTierLow
	}
}

// intervalForTier returns the concrete poll interval in seconds for a tier.
func intervalForTier(tier vm.PollingTier) int {
	switch tier {
	case vm.PollingTierHigh:
		return highTierIntervalSec
	case vm.PollingTierLow:
		return lowTierIntervalSec
	default:
		return lowTierIntervalSec
	}
}

// mapDomainStatusToEntVM converts domain.VMStatus to the Ent vm.Status enum.
func mapDomainStatusToEntVM(status domain.VMStatus) vm.Status {
	switch status {
	case domain.VMStatusCreating:
		return vm.StatusCREATING
	case domain.VMStatusRunning:
		return vm.StatusRUNNING
	case domain.VMStatusStopping:
		return vm.StatusSTOPPING
	case domain.VMStatusStopped:
		return vm.StatusSTOPPED
	case domain.VMStatusDeleting:
		return vm.StatusDELETING
	case domain.VMStatusFailed:
		return vm.StatusFAILED
	case domain.VMStatusPending:
		return vm.StatusPENDING
	case domain.VMStatusMigrating:
		return vm.StatusMIGRATING
	case domain.VMStatusPaused:
		return vm.StatusPAUSED
	case domain.VMStatusUnknown:
		return vm.StatusUNKNOWN
	default:
		return vm.StatusUNKNOWN
	}
}

// reconcileCreateBootstrapStatus prevents premature CREATING→STOPPED/UNKNOWN
// downgrade during the initial create bootstrap window.
func reconcileCreateBootstrapStatus(vmRow *ent.VM, observed vm.Status, now time.Time) vm.Status {
	if shouldHoldCreateBootstrapStatus(vmRow, observed, now) {
		return vm.StatusCREATING
	}
	return observed
}

func shouldHoldCreateBootstrapStatus(vmRow *ent.VM, observed vm.Status, now time.Time) bool {
	if vmRow == nil {
		return false
	}
	if vmRow.PollingTier != vm.PollingTierHigh {
		return false
	}
	if vmRow.Status != vm.StatusCREATING && vmRow.Status != vm.StatusRUNNING {
		return false
	}
	if observed != vm.StatusSTOPPED && observed != vm.StatusUNKNOWN {
		return false
	}
	if vmRow.CreatedAt.IsZero() {
		return false
	}
	return now.Sub(vmRow.CreatedAt) <= createBootstrapGraceWindow
}

// deriveHighTierSince computes the persisted high-tier entry timestamp.
//
// Rules:
//  1. Non-high tier: clear timestamp (nil)
//  2. Entering high tier (or legacy row without timestamp): set to now
//  3. Staying in high tier: keep existing timestamp
func deriveHighTierSince(vmRow *ent.VM, newTier vm.PollingTier, now time.Time) *time.Time {
	if vmRow == nil || newTier != vm.PollingTierHigh {
		return nil
	}
	if vmRow.PollingTier != vm.PollingTierHigh || vmRow.HighTierSince == nil {
		t := now
		return &t
	}
	t := *vmRow.HighTierSince
	return &t
}

func shouldAutoDowngrade(newTier vm.PollingTier, highTierSince *time.Time, now time.Time) bool {
	if newTier != vm.PollingTierHigh || highTierSince == nil {
		return false
	}
	return now.Sub(*highTierSince) > autoDowngradeThreshold
}
