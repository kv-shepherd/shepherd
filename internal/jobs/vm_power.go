package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	powerOpStart                 = "start"
	powerOpStop                  = "stop"
	powerOpRestart               = "restart"
	restartProcessingFenceReason = "restart is already PROCESSING; read-only verification and escalation are required, and its ambiguous dispatch fence remains until a provider receipt, idempotency proof, or provable cancellation establishes a safe terminal outcome"

	finalPowerFailurePersistenceTimeout = 5 * time.Second
	powerFailureConvergenceSnooze       = 5 * time.Second
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
			// A terminal successful job must not prevent an explicit retry of a
			// failed durable event. Keep uniqueness only across runnable states.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
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
		if ent.IsNotFound(err) {
			return river.JobCancel(fmt.Errorf("power domain event %s no longer exists", eventID))
		}
		return finalizePowerPreDispatchFailureOnLastAttempt(
			ctx,
			job,
			eventID,
			fmt.Errorf("fetch domain event %s: %w", eventID, err),
		)
	}
	_, eventInitiallyTerminal := ticketStatusForTerminalDomainEvent(event.Status)

	// Step 2: Parse payload.
	var payload domain.VMPowerPayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		if eventInitiallyTerminal {
			return river.JobCancel(fmt.Errorf(
				"terminal power event %s has a malformed immutable payload; refusing projection repair: %w",
				eventID,
				unmarshalErr,
			))
		}
		return river.JobCancel(fmt.Errorf(
			"power event %s has a malformed immutable payload; refusing an unvalidated projection write: %w",
			eventID,
			unmarshalErr,
		))
	}
	switch payload.DispatchMode {
	case domain.VMPowerDispatchDirect:
	case domain.VMPowerDispatchTicket:
	default:
		return river.JobCancel(fmt.Errorf(
			"power event %s has missing or invalid immutable dispatch mode %q",
			eventID,
			payload.DispatchMode,
		))
	}
	operation := strings.ToLower(strings.TrimSpace(payload.Operation))
	switch operation {
	case powerOpStart, powerOpStop, powerOpRestart:
	default:
		if eventInitiallyTerminal {
			return river.JobCancel(fmt.Errorf(
				"terminal power event %s has unknown immutable operation %q; refusing projection repair",
				eventID,
				operation,
			))
		}
		return river.JobCancel(fmt.Errorf(
			"power event %s has unknown immutable operation %q; refusing an unvalidated projection write",
			eventID,
			operation,
		))
	}
	if eventInitiallyTerminal {
		logger.Info("vm power event already terminal, skipping duplicate execution",
			zap.String("event_id", eventID),
			zap.String("event_status", event.Status.String()),
		)
		return repairTerminalVMPowerProjection(ctx, w.entClient, job, eventID, payload, operation)
	}

	markFailed := func(err error, cancel bool) error {
		logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)

		if cancel {
			return cancelPowerWithoutRetry(ctx, w.entClient, eventID, payload, operation, err)
		}
		return finalizePowerFailureOnLastAttempt(ctx, w.entClient, job, eventID, payload, operation, err)
	}
	markDeterministicPreDispatchFailure := func(err error) error {
		// First settle a still-PENDING request. If another delivery already owns a
		// PROCESSING claim, RESTART keeps its fence; idempotent START/STOP consume
		// bounded attempts before an exact final provenance recheck may fail it.
		return resolveDeterministicPreDispatchFailure(
			ctx,
			w.entClient,
			w.auditLogger,
			job,
			eventID,
			payload,
			operation,
			err,
		)
	}

	if identityErr := validateVMPowerEventIdentity(event, payload, operation); identityErr != nil {
		return river.JobCancel(fmt.Errorf("refusing power dispatch with invalid immutable event identity: %w", identityErr))
	}
	if operation == powerOpRestart && event.Status == domainevent.StatusPROCESSING {
		return river.JobCancel(rejectPowerDispatch(eventID, restartProcessingFenceReason))
	}

	// Finish every deterministic database-side validation before claiming the
	// restart dispatch. Once a restart enters PROCESSING, any later uncertainty
	// must retain that fence because the provider may already have accepted it.
	currentVM, err := w.entClient.VM.Get(ctx, payload.VMID)
	switch {
	case err == nil:
		if identityErr := validateVMPowerVMIdentity(currentVM, payload); identityErr != nil {
			return river.JobCancel(fmt.Errorf("refusing power dispatch with invalid provider identity: %w", identityErr))
		}
		if currentVM.Status == vm.StatusDELETING {
			return markDeterministicPreDispatchFailure(fmt.Errorf("vm %s is deleting", payload.VMID))
		}
	case ent.IsNotFound(err):
		return markDeterministicPreDispatchFailure(fmt.Errorf("vm %s no longer exists", payload.VMID))
	default:
		queryErr := fmt.Errorf("query vm %s before power execution: %w", payload.VMID, err)
		return finalizePendingPowerFailureOnLastAttempt(
			ctx,
			w.entClient,
			w.auditLogger,
			job,
			eventID,
			payload,
			operation,
			queryErr,
		)
	}

	claimOutcome, claimErr := claimVMPowerDispatch(
		ctx,
		w.entClient,
		eventID,
		payload,
		operation,
	)
	if claimErr != nil {
		var rejected *powerDispatchRejectedError
		if errors.As(claimErr, &rejected) {
			if rejected.resolveProviderFreeFailure {
				// The rejected claim transaction rolled back before the provider
				// call. Revalidate PENDING first; an existing PROCESSING claim is
				// bounded only for START/STOP, while RESTART keeps its fence.
				return markDeterministicPreDispatchFailure(claimErr)
			}
			return river.JobCancel(claimErr)
		}
		if claimOutcome == powerDispatchRolledBack {
			return finalizePendingPowerFailureOnLastAttempt(
				ctx,
				w.entClient,
				w.auditLogger,
				job,
				eventID,
				payload,
				operation,
				fmt.Errorf("claim power dispatch for event %s: %w", eventID, claimErr),
			)
		}
		return finalizePowerPreDispatchFailureOnLastAttempt(
			ctx,
			job,
			eventID,
			fmt.Errorf("claim power dispatch for event %s: %w", eventID, claimErr),
		)
	}
	if claimOutcome == powerDispatchTerminal {
		logger.Info("VM power event became terminal before dispatch",
			zap.String("event_id", eventID),
		)
		return repairTerminalVMPowerProjection(ctx, w.entClient, job, eventID, payload, operation)
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
	}
	if ctxErr := jobContextErr(ctx, execErr); ctxErr != nil {
		failureErr := fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, ctxErr)
		if operation == powerOpRestart {
			// The provider contract cannot distinguish a request rejected before
			// submission from one accepted by KubeVirt whose response was lost.
			// Preserve PROCESSING/EXECUTING so neither an automatic delivery nor
			// the ordinary explicit-retry endpoint can repeat an ambiguous restart.
			logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
			return cancelAmbiguousRestartWithoutRetry(failureErr)
		}
		return finalizePowerFailureOnLastAttempt(ctx, w.entClient, job, eventID, payload, operation, failureErr)
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
		if operation == powerOpRestart {
			// Restart has no provider idempotency key or "definitely not applied"
			// error classification. Even transport/5xx and generic provider errors
			// may be post-commit response failures, so require an explicit retry.
			logger.Warn("VM restart returned an ambiguous provider error; cancelling automatic retries",
				zap.String("event_id", eventID),
				zap.String("vm_name", payload.VMName),
				zap.Error(execErr),
			)
			failureErr := fmt.Errorf("execute k8s %s for event %s: %w", operation, eventID, execErr)
			logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
			return cancelAmbiguousRestartWithoutRetry(failureErr)
		}
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
			if operation == powerOpRestart {
				failureErr := fmt.Errorf("refresh live VM after successful restart power event %s: %w", eventID, ctxErr)
				logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
				return cancelAmbiguousRestartWithoutRetry(failureErr)
			}
			return finalizePowerFailureOnLastAttempt(ctx, w.entClient, job, eventID, payload, operation, ctxErr)
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
	if persistErr := persistCompletedPowerState(
		ctx,
		w.entClient,
		eventID,
		payload,
		operation,
		targetStatus,
		targetTier,
		targetInterval,
		observedAt,
		targetRV,
	); persistErr != nil {
		failurePrefix := "persist terminal status"
		var vmStatusErr powerVMStatusPersistError
		if errors.As(persistErr, &vmStatusErr) {
			logger.Error("CRITICAL: K8s power op succeeded but VM status update failed",
				zap.String("event_id", eventID),
				zap.String("operation", operation),
				zap.String("vm_name", payload.VMName),
				zap.Error(persistErr))
			failurePrefix = "persist VM status"
		} else {
			logger.Error("CRITICAL: Power op completed but terminal status persistence failed",
				zap.String("event_id", eventID), zap.Error(persistErr))
		}
		failureErr := fmt.Errorf("%s after %s power event %s: %w", failurePrefix, operation, eventID, persistErr)
		if isRetryablePowerOperationAfterSuccess(operation) {
			return finalizePowerFailureOnLastAttempt(
				ctx,
				w.entClient,
				job,
				eventID,
				payload,
				operation,
				failureErr,
			)
		}

		// Restart is not safely repeatable: once the provider accepted it, neither
		// River nor the ordinary explicit-retry endpoint may dispatch it again.
		// The transaction that failed to persist COMPLETED rolled back, so the
		// durable PROCESSING/EXECUTING claim remains the reconciliation fence.
		logAuditVMOp(ctx, w.auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
		return cancelAmbiguousRestartWithoutRetry(failureErr)
	}
	logAuditVMOp(ctx, w.auditLogger, operation, payload.VMName, payload.Actor, eventID)

	logger.Info("VM power operation completed",
		zap.String("event_id", eventID),
		zap.String("operation", operation),
		zap.String("vm_name", payload.VMName),
	)
	return nil
}

func validateVMPowerEventIdentity(
	event *ent.DomainEvent,
	payload domain.VMPowerPayload,
	operation string,
) error {
	if event == nil {
		return fmt.Errorf("power event identity is missing")
	}
	var persistedPayload domain.VMPowerPayload
	if err := decodeVMPowerProvenanceJSON(event.Payload, &persistedPayload); err != nil {
		return fmt.Errorf("power event %s immutable payload is malformed: %w", event.ID, err)
	}
	if persistedPayload != payload {
		return fmt.Errorf("power event %s immutable payload changed after it was loaded", event.ID)
	}
	expectedEventType := ""
	switch operation {
	case powerOpStart:
		expectedEventType = string(domain.EventVMStartRequested)
	case powerOpStop:
		expectedEventType = string(domain.EventVMStopRequested)
	case powerOpRestart:
		expectedEventType = string(domain.EventVMRestartRequested)
	}
	if expectedEventType == "" ||
		strings.TrimSpace(event.EventType) != expectedEventType ||
		strings.TrimSpace(event.AggregateType) != "vm" ||
		strings.TrimSpace(event.AggregateID) == "" ||
		strings.TrimSpace(event.AggregateID) != strings.TrimSpace(payload.VMID) ||
		strings.TrimSpace(event.CreatedBy) == "" ||
		strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(payload.Actor) ||
		strings.TrimSpace(payload.VMName) == "" ||
		strings.TrimSpace(payload.ClusterID) == "" ||
		strings.TrimSpace(payload.Namespace) == "" {
		return fmt.Errorf("power event %s immutable identity is inconsistent with its payload", event.ID)
	}
	return nil
}

func validateVMPowerTicketIdentity(ticket *ent.Ticket, eventID string, payload domain.VMPowerPayload) error {
	if ticket == nil ||
		ticket.OperationType != entticket.OperationTypePOWER ||
		strings.TrimSpace(ticket.EventID) != strings.TrimSpace(eventID) ||
		strings.TrimSpace(ticket.Requester) == "" ||
		strings.TrimSpace(ticket.Requester) != strings.TrimSpace(payload.Actor) {
		return fmt.Errorf("power ticket identity is inconsistent with its event payload")
	}
	return nil
}

func validateVMPowerVMIdentity(currentVM *ent.VM, payload domain.VMPowerPayload) error {
	if currentVM == nil ||
		strings.TrimSpace(currentVM.ID) != strings.TrimSpace(payload.VMID) ||
		strings.TrimSpace(currentVM.Name) != strings.TrimSpace(payload.VMName) ||
		strings.TrimSpace(currentVM.ClusterID) != strings.TrimSpace(payload.ClusterID) ||
		strings.TrimSpace(currentVM.Namespace) != strings.TrimSpace(payload.Namespace) {
		return fmt.Errorf("power event provider coordinates are inconsistent with VM %s", payload.VMID)
	}
	return nil
}

type powerDispatchClaimOutcome uint8

const (
	powerDispatchClaimed powerDispatchClaimOutcome = iota
	powerDispatchTerminal
	powerDispatchRolledBack
	powerDispatchUnknown
)

type powerDispatchRejectedError struct {
	eventID                    string
	reason                     string
	resolveProviderFreeFailure bool
}

func (e *powerDispatchRejectedError) Error() string {
	return fmt.Sprintf("power event %s dispatch rejected: %s", e.eventID, e.reason)
}

func rejectPowerDispatch(eventID, reason string) error {
	return &powerDispatchRejectedError{eventID: strings.TrimSpace(eventID), reason: reason}
}

// rejectProviderFreePowerDispatch marks a claim rejection that cannot have
// reached the provider. Resolution independently revalidates exact PENDING
// state first; only idempotent START/STOP may later converge PROCESSING after
// their bounded attempt budget is exhausted.
func rejectProviderFreePowerDispatch(eventID, reason string) error {
	return &powerDispatchRejectedError{
		eventID:                    strings.TrimSpace(eventID),
		reason:                     reason,
		resolveProviderFreeFailure: true,
	}
}

// claimVMPowerDispatch is the final transactional authorization gate before a
// provider side effect. It serializes with every legitimate power writer,
// locks the VM/ticket/event rows in the package-wide persistence order,
// validates immutable submission provenance and
// ticket state, and commits PENDING -> PROCESSING plus ticket EXECUTING as one
// durable claim. START/STOP may resume an already-PROCESSING claim because they
// are idempotent; RESTART preserves PROCESSING as an at-most-once fence.
func claimVMPowerDispatch(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) (powerDispatchClaimOutcome, error) {
	if client == nil || strings.TrimSpace(eventID) == "" {
		return powerDispatchClaimed, rejectPowerDispatch(eventID, "client and event id are required")
	}
	outcome := powerDispatchClaimed
	var callbackFailure error
	err := withJobsEntTx(ctx, client, func(tx *ent.Tx, txClient *ent.Client) (callbackErr error) {
		defer func() {
			if callbackErr == nil {
				return
			}
			callbackFailure = fmt.Errorf("power dispatch claim transaction failed before commit: %w", callbackErr)
			callbackErr = callbackFailure
		}()

		vmID := strings.TrimSpace(payload.VMID)
		if err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"power:vm:"+vmID,
		); err != nil {
			return fmt.Errorf("lock power dispatch for VM %s: %w", vmID, err)
		}

		lockedVM, err := txClient.VM.Query().
			Where(vm.IDEQ(vmID), lockVMRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			return rejectProviderFreePowerDispatch(eventID, fmt.Sprintf("VM %s no longer exists", vmID))
		}
		if err != nil {
			return fmt.Errorf("lock VM %s for power dispatch: %w", vmID, err)
		}
		if identityErr := validateVMPowerVMIdentity(lockedVM, payload); identityErr != nil {
			return rejectProviderFreePowerDispatch(eventID, identityErr.Error())
		}
		if lockedVM.Status == vm.StatusDELETING {
			return rejectProviderFreePowerDispatch(eventID, fmt.Sprintf("VM %s is deleting", vmID))
		}

		tickets, err := txClient.Ticket.Query().
			Where(entticket.EventIDEQ(strings.TrimSpace(eventID)), lockTicketRowForUpdate()).
			Order(entticket.ByID()).
			Limit(2).
			All(ctx)
		if err != nil {
			return fmt.Errorf("lock ticket binding for power event %s: %w", eventID, err)
		}

		lockedEvent, err := txClient.DomainEvent.Query().
			Where(domainevent.IDEQ(strings.TrimSpace(eventID)), lockDomainEventRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			return rejectPowerDispatch(eventID, "domain event no longer exists")
		}
		if err != nil {
			return fmt.Errorf("lock power domain event %s: %w", eventID, err)
		}
		if identityErr := validateVMPowerEventIdentity(lockedEvent, payload, operation); identityErr != nil {
			return rejectPowerDispatch(eventID, identityErr.Error())
		}

		var ticket *ent.Ticket
		switch payload.DispatchMode {
		case domain.VMPowerDispatchDirect:
			if len(tickets) != 0 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("direct mode requires zero tickets, found %d", len(tickets)))
			}
		case domain.VMPowerDispatchTicket:
			if len(tickets) != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("ticket mode requires exactly one ticket, found %d", len(tickets)))
			}
			ticket = tickets[0]
			if identityErr := validateVMPowerTicketIdentity(ticket, eventID, payload); identityErr != nil {
				return rejectPowerDispatch(eventID, identityErr.Error())
			}
		default:
			return rejectPowerDispatch(eventID, fmt.Sprintf("invalid dispatch mode %q", payload.DispatchMode))
		}

		switch lockedEvent.Status {
		case domainevent.StatusCOMPLETED, domainevent.StatusFAILED, domainevent.StatusCANCELLED:
			outcome = powerDispatchTerminal
			return nil
		case domainevent.StatusPROCESSING:
			if operation == powerOpRestart {
				return rejectPowerDispatch(eventID, restartProcessingFenceReason)
			}
			if ticket != nil {
				if authorizationErr := validateLockedVMPowerTicketAuthorization(ctx, txClient, ticket, lockedEvent.Status, payload); authorizationErr != nil {
					return authorizationErr
				}
			}
			return nil
		case domainevent.StatusPENDING:
			if ticket != nil {
				if authorizationErr := validateLockedVMPowerTicketAuthorization(ctx, txClient, ticket, lockedEvent.Status, payload); authorizationErr != nil {
					return authorizationErr
				}
			}
		default:
			return rejectPowerDispatch(eventID, fmt.Sprintf("event has unexpected status %s and is not dispatchable", lockedEvent.Status))
		}

		claimed, err := tryUpdateDomainEventStatusWithExpected(
			ctx,
			txClient,
			eventID,
			domainevent.StatusPROCESSING,
			domainevent.StatusPENDING,
		)
		if err != nil {
			return fmt.Errorf(
				"persist PROCESSING/EXECUTING status for power event %s: %w",
				eventID,
				err,
			)
		}
		if !claimed {
			return rejectProviderFreePowerDispatch(eventID, "PENDING dispatch claim was not acquired")
		}
		if ticket != nil {
			affected, err := txClient.Ticket.Update().
				Where(
					entticket.IDEQ(ticket.ID),
					entticket.EventIDEQ(eventID),
					entticket.StatusIn(entticket.StatusAPPROVED, entticket.StatusEXECUTING),
				).
				SetStatus(entticket.StatusEXECUTING).
				Save(ctx)
			if err != nil {
				return fmt.Errorf(
					"persist PROCESSING/EXECUTING status for power event %s: %w",
					eventID,
					err,
				)
			}
			if affected != 1 {
				return rejectProviderFreePowerDispatch(eventID, fmt.Sprintf("ticket %s lost its executable state", ticket.ID))
			}
		}
		if err := syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID); err != nil {
			return fmt.Errorf("persist PROCESSING/EXECUTING batch projection for power event %s: %w", eventID, err)
		}
		return nil
	})
	if err != nil {
		var rollbackErr *jobsTransactionRollbackError
		if callbackFailure != nil && errors.Is(err, callbackFailure) && !errors.As(err, &rollbackErr) {
			return powerDispatchRolledBack, err
		}
		return powerDispatchUnknown, err
	}
	return outcome, nil
}

func validateLockedVMPowerTicketAuthorization(
	ctx context.Context,
	txClient *ent.Client,
	ticket *ent.Ticket,
	eventStatus domainevent.Status,
	payload domain.VMPowerPayload,
) error {
	if ticket == nil || txClient == nil {
		return rejectPowerDispatch("", "ticket authorization context is incomplete")
	}
	eventID := strings.TrimSpace(ticket.EventID)
	parentID := strings.TrimSpace(ticket.ParentTicketID)
	if parentID == "" {
		expectedStatus := entticket.StatusAPPROVED
		if eventStatus == domainevent.StatusPROCESSING {
			expectedStatus = entticket.StatusEXECUTING
		}
		if ticket.Status != expectedStatus || strings.TrimSpace(ticket.Approver) == "" {
			return rejectPowerDispatch(
				eventID,
				fmt.Sprintf(
					"standalone ticket requires status %s and a non-empty approver, found status %s",
					expectedStatus,
					ticket.Status,
				),
			)
		}
		return nil
	}

	batchStatusAllowed := ticket.Status == entticket.StatusEXECUTING ||
		(eventStatus == domainevent.StatusPENDING && ticket.Status == entticket.StatusAPPROVED)
	if !batchStatusAllowed {
		return rejectPowerDispatch(
			eventID,
			fmt.Sprintf("batch child ticket has non-executable status %s", ticket.Status),
		)
	}
	actor := strings.TrimSpace(payload.Actor)
	parent, err := txClient.Ticket.Query().
		Where(
			entticket.IDEQ(parentID),
			entticket.ParentTicketIDIsNil(),
			entticket.OperationTypeEQ(entticket.OperationTypePOWER),
			entticket.StatusEQ(entticket.StatusEXECUTING),
			entticket.RequesterEQ(actor),
			lockTicketRowForUpdate(),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return rejectPowerDispatch(eventID, fmt.Sprintf("batch parent ticket %s is missing or inconsistent", parentID))
	}
	if err != nil {
		return fmt.Errorf("lock batch parent ticket %s for child event %s: %w", parentID, eventID, err)
	}
	parentEvent, err := txClient.DomainEvent.Query().
		Where(
			domainevent.IDEQ(strings.TrimSpace(parent.EventID)),
			domainevent.EventTypeEQ(string(domain.EventBatchPowerRequested)),
			domainevent.AggregateTypeEQ(batchAggregateType),
			domainevent.AggregateIDEQ(parentID),
			domainevent.StatusEQ(domainevent.StatusPROCESSING),
			domainevent.CreatedByEQ(actor),
			lockDomainEventRowForUpdate(),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return rejectPowerDispatch(eventID, fmt.Sprintf("batch parent event for ticket %s is missing or inconsistent", parentID))
	}
	if err != nil {
		return fmt.Errorf("lock batch parent event for ticket %s: %w", parentID, err)
	}
	projection, err := txClient.BatchTicket.Query().
		Where(
			batchticket.IDEQ(parentID),
			batchticket.BatchTypeEQ(batchticket.BatchTypeBATCH_POWER),
			batchticket.StatusEQ(batchticket.StatusIN_PROGRESS),
			batchticket.CreatedByEQ(actor),
			lockBatchTicketRowForUpdate(),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return rejectPowerDispatch(eventID, fmt.Sprintf("batch projection %s is missing or inconsistent", parentID))
	}
	if err != nil {
		return fmt.Errorf("lock batch projection %s for child event %s: %w", parentID, eventID, err)
	}
	return validateExactBatchPowerGraph(ctx, txClient, eventID, parent, parentEvent, projection)
}

func loadPowerEventStatusBounded(ctx context.Context, client *ent.Client, eventID string) (domainevent.Status, error) {
	if client == nil || eventID == "" {
		return "", fmt.Errorf("power event client and id are required")
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalPowerFailurePersistenceTimeout)
	defer cancel()
	event, err := client.DomainEvent.Get(readCtx, eventID)
	if err != nil {
		return "", err
	}
	return event.Status, nil
}

// resolveDeterministicPreDispatchFailure handles a proven provider-free
// anomaly. It fails an exact PENDING request immediately. An idempotent
// START/STOP that was already claimed consumes ordinary attempts before an
// exact final PROCESSING -> FAILED transition; RESTART never releases its
// ambiguity fence.
func resolveDeterministicPreDispatchFailure(
	ctx context.Context,
	client *ent.Client,
	auditLogger *audit.Logger,
	job *river.Job[VMPowerArgs],
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	cause error,
) error {
	if persistErr := persistPendingPowerFailureBounded(ctx, client, eventID, payload, operation); persistErr != nil {
		return resolvePreDispatchPowerFailurePersistence(
			ctx,
			client,
			auditLogger,
			job,
			true,
			eventID,
			payload,
			operation,
			cause,
			persistErr,
		)
	}
	// Emit a terminal failure audit only after the exact PENDING -> FAILED
	// transaction, including ticket and batch projection convergence, committed.
	// A competing PROCESSING restart fence therefore never produces a misleading
	// terminal failure record.
	logAuditVMOp(ctx, auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
	return river.JobCancel(cause)
}

// cancelPowerWithoutRetry persists the durable FAILED fence after this delivery
// has acquired the dispatch claim and the provider returned a deterministic,
// known-not-applied error.
func cancelPowerWithoutRetry(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	cause error,
) error {
	if persistErr := persistFinalPowerFailureBounded(ctx, client, eventID, payload, operation); persistErr != nil {
		joinedErr := errors.Join(
			cause,
			fmt.Errorf("persist FAILED status before cancelling power job: %w", persistErr),
		)
		var rejected *powerDispatchRejectedError
		if errors.As(persistErr, &rejected) {
			return river.JobCancel(joinedErr)
		}
		observedStatus, statusErr := loadPowerEventStatusBounded(ctx, client, eventID)
		if statusErr == nil {
			if terminal, terminalErr := cancelTerminalPowerPersistenceConflict(eventID, observedStatus, joinedErr); terminal {
				return terminalErr
			}
		}
		if operation == powerOpRestart {
			// The provider reported a deterministic known-not-applied restart, but
			// that classification cannot survive this worker without the FAILED
			// write. Conservatively treat it as ambiguous: keep the existing
			// PROCESSING/EXECUTING fence and cancel so River can never redispatch.
			logger.Error("restart failure classification could not be persisted; preserving ambiguous dispatch fence",
				zap.String("event_id", eventID),
				zap.String("operation", operation),
				zap.Error(joinedErr),
			)
			return river.JobCancel(joinedErr)
		}
		// START and STOP are safely repeatable. Snoozing does not consume an
		// attempt, so a later delivery can retry the durable failure write rather
		// than letting River discard an unpersisted terminal classification.
		logger.Error("deterministic power failure could not be persisted; snoozing for durable convergence",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.Error(joinedErr),
		)
		return river.JobSnooze(powerFailureConvergenceSnooze)
	}
	return river.JobCancel(cause)
}

// cancelAmbiguousRestartWithoutRetry leaves the claimed
// PROCESSING/EXECUTING state untouched. That state is the durable fence that
// blocks ordinary new submissions and explicit retries. It remains fenced
// unless a future provider receipt, idempotency, or provable-cancellation
// protocol can establish a safe terminal outcome.
func cancelAmbiguousRestartWithoutRetry(cause error) error {
	return river.JobCancel(cause)
}

// finalizePowerFailureOnLastAttempt preserves River's ordinary retry behavior
// until the last attempt. River discards an ordinary error when
// Attempt >= MaxAttempts, so the last invocation must first converge the
// recoverable durable state instead of leaving PROCESSING/EXECUTING rows.
func finalizePowerFailureOnLastAttempt(
	ctx context.Context,
	client *ent.Client,
	job *river.Job[VMPowerArgs],
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	cause error,
) error {
	ctxErr := jobContextErr(ctx, cause)
	if !isFinalJobAttempt(job) {
		if ctxErr != nil {
			return ctxErr
		}
		return cause
	}
	if persistErr := persistFinalPowerFailureBounded(ctx, client, eventID, payload, operation); persistErr != nil {
		joinedErr := errors.Join(
			cause,
			fmt.Errorf("persist final FAILED status for power event %s: %w", eventID, persistErr),
		)
		var rejected *powerDispatchRejectedError
		if errors.As(persistErr, &rejected) {
			return river.JobCancel(joinedErr)
		}
		observedStatus, statusErr := loadPowerEventStatusBounded(ctx, client, eventID)
		if statusErr == nil {
			if terminal, terminalErr := cancelTerminalPowerPersistenceConflict(eventID, observedStatus, joinedErr); terminal {
				return terminalErr
			}
		}
		if operation == powerOpRestart {
			logger.Error("restart final failure could not be persisted; preserving ambiguous dispatch fence",
				zap.String("event_id", eventID),
				zap.Error(joinedErr),
			)
			return river.JobCancel(joinedErr)
		}
		logger.Error("final power failure could not be persisted; snoozing for durable convergence",
			zap.String("event_id", eventID),
			zap.String("operation", operation),
			zap.Error(joinedErr),
		)
		return river.JobSnooze(powerFailureConvergenceSnooze)
	}
	if ctxErr != nil {
		return ctxErr
	}
	return cause
}

// finalizePendingPowerFailureOnLastAttempt converges a final pre-claim failure
// only while the event is still PENDING. A PROCESSING event may belong to a
// concurrent provider call, so this helper never authorizes PROCESSING ->
// FAILED and instead preserves that durable fence.
func finalizePendingPowerFailureOnLastAttempt(
	ctx context.Context,
	client *ent.Client,
	auditLogger *audit.Logger,
	job *river.Job[VMPowerArgs],
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	cause error,
) error {
	ctxErr := jobContextErr(ctx, cause)
	if !isFinalJobAttempt(job) {
		if ctxErr != nil {
			return ctxErr
		}
		return cause
	}
	if persistErr := persistPendingPowerFailureBounded(ctx, client, eventID, payload, operation); persistErr != nil {
		return resolvePreDispatchPowerFailurePersistence(
			ctx,
			client,
			auditLogger,
			job,
			false,
			eventID,
			payload,
			operation,
			cause,
			persistErr,
		)
	}
	logAuditVMOp(ctx, auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
	if ctxErr != nil {
		return ctxErr
	}
	return cause
}

// finalizePowerPreDispatchFailureOnLastAttempt handles every failure before a
// worker has parsed and transactionally proved immutable dispatch provenance.
// It never guesses a ticket binding. A final transient error is snoozed so a
// later delivery can perform the exact claim or failure transition safely.
func finalizePowerPreDispatchFailureOnLastAttempt(
	ctx context.Context,
	job *river.Job[VMPowerArgs],
	eventID string,
	cause error,
) error {
	ctxErr := jobContextErr(ctx, cause)
	if !isFinalJobAttempt(job) {
		if ctxErr != nil {
			return ctxErr
		}
		return cause
	}
	logger.Error("final pre-dispatch power failure lacks validated provenance; snoozing without projection writes",
		zap.String("event_id", eventID),
		zap.Error(cause),
	)
	return river.JobSnooze(powerFailureConvergenceSnooze)
}

// repairTerminalPowerTicketOnLastAttempt uses an uncancelled bounded context
// to finish an already-terminal event's ticket/batch projection. It never
// changes the terminal event itself.
func repairTerminalVMPowerProjection(
	ctx context.Context,
	client *ent.Client,
	job *river.Job[VMPowerArgs],
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) error {
	cause := repairTerminalVMPowerProjectionOnce(ctx, client, eventID, payload, operation)
	if cause == nil {
		return nil
	}
	var rejected *powerDispatchRejectedError
	if errors.As(cause, &rejected) {
		return river.JobCancel(cause)
	}
	if !isFinalJobAttempt(job) {
		return cause
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalPowerFailurePersistenceTimeout)
	defer cancel()
	if persistErr := repairTerminalVMPowerProjectionOnce(persistCtx, client, eventID, payload, operation); persistErr != nil {
		if errors.As(persistErr, &rejected) {
			return river.JobCancel(errors.Join(cause, persistErr))
		}
		joinedErr := errors.Join(
			cause,
			fmt.Errorf("repair exact terminal projection for power event %s: %w", eventID, persistErr),
		)
		logger.Error("terminal power projection repair failed; snoozing for durable convergence",
			zap.String("event_id", eventID),
			zap.Error(joinedErr),
		)
		return river.JobSnooze(powerFailureConvergenceSnooze)
	}
	return cause
}

func repairTerminalVMPowerProjectionOnce(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) error {
	if client == nil || strings.TrimSpace(eventID) == "" || strings.TrimSpace(payload.VMID) == "" {
		return rejectPowerDispatch(eventID, "terminal repair requires client, event id, and VM id")
	}
	return withJobsEntTx(ctx, client, func(tx *ent.Tx, txClient *ent.Client) error {
		if err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"power:vm:"+strings.TrimSpace(payload.VMID),
		); err != nil {
			return fmt.Errorf("lock terminal power projection for VM %s: %w", payload.VMID, err)
		}
		tickets, err := txClient.Ticket.Query().
			Where(entticket.EventIDEQ(strings.TrimSpace(eventID)), lockTicketRowForUpdate()).
			Order(entticket.ByID()).
			Limit(2).
			All(ctx)
		if err != nil {
			return fmt.Errorf("lock terminal ticket binding for power event %s: %w", eventID, err)
		}
		lockedEvent, err := txClient.DomainEvent.Query().
			Where(domainevent.IDEQ(strings.TrimSpace(eventID)), lockDomainEventRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			return rejectPowerDispatch(eventID, "terminal domain event no longer exists")
		}
		if err != nil {
			return fmt.Errorf("lock terminal power event %s: %w", eventID, err)
		}
		if err := validateVMPowerEventIdentity(lockedEvent, payload, operation); err != nil {
			return rejectPowerDispatch(eventID, err.Error())
		}
		targetStatus, terminal := ticketStatusForTerminalDomainEvent(lockedEvent.Status)
		if !terminal {
			return rejectPowerDispatch(eventID, fmt.Sprintf("event is no longer terminal (status %s)", lockedEvent.Status))
		}
		switch payload.DispatchMode {
		case domain.VMPowerDispatchDirect:
			if len(tickets) != 0 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("terminal direct mode requires zero tickets, found %d", len(tickets)))
			}
			return nil
		case domain.VMPowerDispatchTicket:
			if len(tickets) != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("terminal ticket mode requires exactly one ticket, found %d", len(tickets)))
			}
		default:
			return rejectPowerDispatch(eventID, fmt.Sprintf("invalid terminal dispatch mode %q", payload.DispatchMode))
		}

		ticket := tickets[0]
		if err := validateVMPowerTicketIdentity(ticket, eventID, payload); err != nil {
			return rejectPowerDispatch(eventID, err.Error())
		}
		if err := validateTerminalVMPowerTicketProvenance(ctx, txClient, ticket, lockedEvent.Status, payload); err != nil {
			return err
		}
		update, compatible := terminalPowerTicketRepairDecision(ticket.Status, targetStatus)
		if !compatible {
			return rejectPowerDispatch(
				eventID,
				fmt.Sprintf("terminal event status %s conflicts with ticket status %s", lockedEvent.Status, ticket.Status),
			)
		}
		if ticket.Status == targetStatus {
			// Provenance validation already proved the standalone approval or the
			// complete batch graph, counters, and parent state. Avoid same-value writes.
			return nil
		}
		if update {
			affected, err := txClient.Ticket.Update().
				Where(
					entticket.IDEQ(ticket.ID),
					entticket.EventIDEQ(eventID),
					entticket.StatusEQ(ticket.Status),
				).
				SetStatus(targetStatus).
				Save(ctx)
			if err != nil {
				return err
			}
			if affected != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("terminal ticket %s changed during repair", ticket.ID))
			}
		}
		return syncParentBatchStatusByChildEventWithClient(ctx, txClient, eventID)
	})
}

func terminalPowerTicketRepairDecision(current, target entticket.Status) (update, compatible bool) {
	if current == target {
		return false, true
	}
	allowed := false
	switch target {
	case entticket.StatusSUCCESS:
		allowed = current == entticket.StatusAPPROVED || current == entticket.StatusEXECUTING
	case entticket.StatusFAILED:
		allowed = current == entticket.StatusPENDING || current == entticket.StatusAPPROVED || current == entticket.StatusEXECUTING
	case entticket.StatusCANCELLED:
		if current == entticket.StatusREJECTED {
			return false, true
		}
		allowed = current == entticket.StatusPENDING || current == entticket.StatusAPPROVED || current == entticket.StatusEXECUTING
	default:
		return false, false
	}
	return allowed, allowed
}

// resolvePreDispatchPowerFailurePersistence distinguishes a transient PENDING
// persistence failure from an already-PROCESSING dispatch claim. RESTART keeps
// its at-most-once fence. In the proven deterministic/provider-free path,
// idempotent START/STOP consume ordinary River attempts and converge an exact
// PROCESSING claim on the final attempt. Other pre-claim failures retain the
// existing snooze-based orphan recovery behavior.
func resolvePreDispatchPowerFailurePersistence(
	ctx context.Context,
	client *ent.Client,
	auditLogger *audit.Logger,
	job *river.Job[VMPowerArgs],
	finalizeIdempotentProcessing bool,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
	cause error,
	persistErr error,
) error {
	joinedErr := errors.Join(
		cause,
		fmt.Errorf("persist pre-dispatch FAILED status for power event %s: %w", eventID, persistErr),
	)
	status, statusErr := loadPowerEventStatusBounded(ctx, client, eventID)
	if ent.IsNotFound(statusErr) {
		// There is no durable event left to converge, and no provider call was
		// made by this rejected delivery. Retrying cannot repair a projection.
		return river.JobCancel(joinedErr)
	}
	if statusErr == nil {
		if terminal, terminalErr := cancelTerminalPowerPersistenceConflict(eventID, status, joinedErr); terminal {
			return terminalErr
		}
		if status == domainevent.StatusPROCESSING {
			if operation == powerOpRestart {
				logger.Error("pre-dispatch failure persistence encountered a restart PROCESSING fence; cancelling without redispatch",
					zap.String("event_id", eventID),
					zap.String("event_status", status.String()),
					zap.Error(joinedErr),
				)
				return river.JobCancel(joinedErr)
			}
			if !finalizeIdempotentProcessing {
				logger.Error("pre-dispatch failure persistence encountered a resumable PROCESSING fence; snoozing for idempotent recovery",
					zap.String("event_id", eventID),
					zap.String("event_status", status.String()),
					zap.String("operation", operation),
					zap.Error(joinedErr),
				)
				return river.JobSnooze(powerFailureConvergenceSnooze)
			}
			if operation != powerOpStart && operation != powerOpStop {
				return river.JobCancel(joinedErr)
			}
			if !isFinalJobAttempt(job) {
				logger.Warn("pre-dispatch failure encountered an idempotent PROCESSING claim; consuming a River attempt",
					zap.String("event_id", eventID),
					zap.String("event_status", status.String()),
					zap.String("operation", operation),
					zap.Int64("attempt", jobAttempt(job)),
					zap.Error(joinedErr),
				)
				if ctxErr := jobContextErr(ctx, cause); ctxErr != nil {
					return ctxErr
				}
				return joinedErr
			}

			finalPersistErr := persistProcessingPowerFailureBounded(
				ctx,
				client,
				eventID,
				payload,
				operation,
			)
			if finalPersistErr == nil {
				logAuditVMOp(ctx, auditLogger, operation+"_failed", payload.VMName, payload.Actor, eventID)
				return river.JobCancel(cause)
			}
			finalJoinedErr := errors.Join(
				joinedErr,
				fmt.Errorf(
					"persist final PROCESSING -> FAILED status for power event %s: %w",
					eventID,
					finalPersistErr,
				),
			)
			var rejected *powerDispatchRejectedError
			if errors.As(finalPersistErr, &rejected) {
				return river.JobCancel(finalJoinedErr)
			}

			finalStatus, finalStatusErr := loadPowerEventStatusBounded(ctx, client, eventID)
			if ent.IsNotFound(finalStatusErr) {
				return river.JobCancel(finalJoinedErr)
			}
			if finalStatusErr == nil {
				if terminal, terminalErr := cancelTerminalPowerPersistenceConflict(
					eventID,
					finalStatus,
					finalJoinedErr,
				); terminal {
					return terminalErr
				}
			}
			logger.Error("final idempotent PROCESSING failure could not be persisted; snoozing for durable convergence",
				zap.String("event_id", eventID),
				zap.String("event_status", finalStatus.String()),
				zap.String("operation", operation),
				zap.NamedError("status_read_error", finalStatusErr),
				zap.Error(finalJoinedErr),
			)
			return river.JobSnooze(powerFailureConvergenceSnooze)
		}
		var rejected *powerDispatchRejectedError
		if status == domainevent.StatusPENDING && errors.As(persistErr, &rejected) {
			return river.JobCancel(joinedErr)
		}
	}
	logger.Error("pre-dispatch failure could not be persisted; snoozing for durable convergence",
		zap.String("event_id", eventID),
		zap.String("event_status", status.String()),
		zap.NamedError("status_read_error", statusErr),
		zap.Error(joinedErr),
	)
	return river.JobSnooze(powerFailureConvergenceSnooze)
}

// cancelTerminalPowerPersistenceConflict handles a permanent conditional-write
// conflict after another worker has already made the event terminal. This
// path may not have parsed and transactionally validated immutable dispatch
// provenance, so it quarantines the delivery without rewriting either the
// event or a potentially unrelated ticket binding. The ordinary terminal
// entry path performs the exact projection repair.
func cancelTerminalPowerPersistenceConflict(
	eventID string,
	status domainevent.Status,
	cause error,
) (terminal bool, err error) {
	_, terminal = ticketStatusForTerminalDomainEvent(status)
	if !terminal {
		return false, nil
	}
	terminalErr := errors.Join(
		cause,
		fmt.Errorf(
			"power event %s is already terminal with status %s; refusing an unvalidated event or ticket projection write",
			eventID,
			status,
		),
	)
	logger.Info("power event reached a competing terminal state; cancelling without retry",
		zap.String("event_id", eventID),
		zap.String("event_status", status.String()),
	)
	return true, river.JobCancel(terminalErr)
}

func persistFinalPowerFailureBounded(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalPowerFailurePersistenceTimeout)
	defer cancel()
	return persistVMPowerFailureExact(
		persistCtx,
		client,
		eventID,
		payload,
		operation,
		domainevent.StatusPENDING,
		domainevent.StatusPROCESSING,
		domainevent.StatusFAILED,
	)
}

func persistProcessingPowerFailureBounded(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalPowerFailurePersistenceTimeout)
	defer cancel()
	return persistVMPowerFailureExact(
		persistCtx,
		client,
		eventID,
		payload,
		operation,
		domainevent.StatusPROCESSING,
	)
}

func persistPendingPowerFailureBounded(
	ctx context.Context,
	client *ent.Client,
	eventID string,
	payload domain.VMPowerPayload,
	operation string,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalPowerFailurePersistenceTimeout)
	defer cancel()
	return persistVMPowerFailureExact(
		persistCtx,
		client,
		eventID,
		payload,
		operation,
		domainevent.StatusPENDING,
	)
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
	payload domain.VMPowerPayload,
	operation string,
	targetStatus vm.Status,
	targetTier vm.PollingTier,
	targetInterval int,
	observedAt time.Time,
	resourceVersion string,
) error {
	eventID = strings.TrimSpace(eventID)
	vmID := strings.TrimSpace(payload.VMID)
	if client == nil || eventID == "" || vmID == "" {
		return rejectPowerDispatch(eventID, "exact completed power persistence context is incomplete")
	}
	return withJobsEntTx(ctx, client, func(tx *ent.Tx, txClient *ent.Client) error {
		if err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"power:vm:"+vmID,
		); err != nil {
			return fmt.Errorf("lock completed power persistence for VM %s: %w", vmID, err)
		}

		lockedVM, err := txClient.VM.Query().
			Where(vm.IDEQ(vmID), lockVMRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			lockedVM = nil
		} else if err != nil {
			return fmt.Errorf("lock VM %s for completed power persistence: %w", vmID, err)
		} else if identityErr := validateVMPowerVMIdentity(lockedVM, payload); identityErr != nil {
			return rejectPowerDispatch(eventID, identityErr.Error())
		}

		tickets, err := txClient.Ticket.Query().
			Where(entticket.EventIDEQ(eventID), lockTicketRowForUpdate()).
			Order(entticket.ByID()).
			Limit(2).
			All(ctx)
		if err != nil {
			return fmt.Errorf("lock ticket binding for completed power event %s: %w", eventID, err)
		}
		lockedEvent, err := txClient.DomainEvent.Query().
			Where(domainevent.IDEQ(eventID), lockDomainEventRowForUpdate()).
			Only(ctx)
		if ent.IsNotFound(err) {
			return rejectPowerDispatch(eventID, "completed power event no longer exists")
		}
		if err != nil {
			return fmt.Errorf("lock completed power event %s: %w", eventID, err)
		}
		if identityErr := validateVMPowerEventIdentity(lockedEvent, payload, operation); identityErr != nil {
			return rejectPowerDispatch(eventID, identityErr.Error())
		}
		if lockedEvent.Status != domainevent.StatusPROCESSING {
			return rejectPowerDispatch(
				eventID,
				fmt.Sprintf("completed power event requires PROCESSING, found %s", lockedEvent.Status),
			)
		}

		var ticket *ent.Ticket
		switch payload.DispatchMode {
		case domain.VMPowerDispatchDirect:
			if len(tickets) != 0 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("completed direct mode requires zero tickets, found %d", len(tickets)))
			}
		case domain.VMPowerDispatchTicket:
			if len(tickets) != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("completed ticket mode requires exactly one ticket, found %d", len(tickets)))
			}
			ticket = tickets[0]
			if identityErr := validateVMPowerTicketIdentity(ticket, eventID, payload); identityErr != nil {
				return rejectPowerDispatch(eventID, identityErr.Error())
			}
			if authErr := validateLockedVMPowerTicketAuthorization(ctx, txClient, ticket, lockedEvent.Status, payload); authErr != nil {
				return authErr
			}
		default:
			return rejectPowerDispatch(eventID, fmt.Sprintf("invalid completed dispatch mode %q", payload.DispatchMode))
		}

		if lockedVM != nil {
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
			if _, saveErr := update.Save(ctx); saveErr != nil {
				if ent.IsNotFound(saveErr) {
					logger.Info("skipped completed power VM status persistence because row is delete-owned or absent",
						zap.String("event_id", eventID),
						zap.String("vm_id", vmID),
					)
				} else {
					return powerVMStatusPersistError{
						err: fmt.Errorf("update VM status during completed power persistence for event %s: %w", eventID, saveErr),
					}
				}
			}
		}
		updated, err := tryUpdateDomainEventStatusWithExpected(
			ctx,
			txClient,
			eventID,
			domainevent.StatusCOMPLETED,
			domainevent.StatusPROCESSING,
		)
		if err != nil {
			return err
		}
		if !updated {
			return rejectPowerDispatch(eventID, "exact COMPLETED event transition was not acquired")
		}
		if ticket != nil {
			affected, err := txClient.Ticket.Update().
				Where(
					entticket.IDEQ(ticket.ID),
					entticket.EventIDEQ(eventID),
					entticket.StatusEQ(entticket.StatusEXECUTING),
				).
				SetStatus(entticket.StatusSUCCESS).
				Save(ctx)
			if err != nil {
				return err
			}
			if affected != 1 {
				return rejectPowerDispatch(eventID, fmt.Sprintf("ticket %s lost its exact completed state", ticket.ID))
			}
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

// isTerminalPowerError reports whether a provider error is known to be a
// deterministic terminal failure. Other restart errors are handled separately
// as ambiguous at-most-once outcomes and are also never retried automatically.
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
