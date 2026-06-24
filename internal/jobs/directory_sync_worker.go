package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	entdirectorysyncjob "kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	configcodec "kv-shepherd.io/shepherd/internal/provider/configcodec"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/service"
)

const DirectorySyncJobKind = "directory_sync"

// DirectorySyncArgs carries only the persisted job identifier. The provider
// identity and request snapshot are resolved from DirectorySyncJob at runtime.
type DirectorySyncArgs struct {
	JobID string `json:"job_id"`
}

func (DirectorySyncArgs) Kind() string { return DirectorySyncJobKind }

func (DirectorySyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
	}
}

// DirectorySyncWorker executes canonical directory imports.
type DirectorySyncWorker struct {
	river.WorkerDefaults[DirectorySyncArgs]
	entClient     *ent.Client
	directorySync *service.DirectorySyncService
	authSessions  *service.AuthSessionManager
	auditLogger   *audit.Logger
	configCodec   *configcodec.AuthProviderConfigCodec
}

func NewDirectorySyncWorker(
	entClient *ent.Client,
	directorySync *service.DirectorySyncService,
	auditLogger *audit.Logger,
	encryptionKey []byte,
	authSessions ...*service.AuthSessionManager,
) *DirectorySyncWorker {
	var authSessionManager *service.AuthSessionManager
	if len(authSessions) > 0 {
		authSessionManager = authSessions[0]
	}
	return &DirectorySyncWorker{
		entClient:     entClient,
		directorySync: directorySync,
		authSessions:  authSessionManager,
		auditLogger:   auditLogger,
		configCodec:   configcodec.NewAuthProviderConfigCodec(encryptionKey),
	}
}

func (w *DirectorySyncWorker) Work(ctx context.Context, job *river.Job[DirectorySyncArgs]) error {
	if w == nil || w.entClient == nil || w.directorySync == nil {
		return river.JobCancel(fmt.Errorf("directory_sync: worker dependencies are not initialized"))
	}

	jobID := strings.TrimSpace(job.Args.JobID)
	if jobID == "" {
		return river.JobCancel(fmt.Errorf("directory_sync: job_id is required"))
	}

	jobRow, err := w.entClient.DirectorySyncJob.Query().
		Where(entdirectorysyncjob.IDEQ(jobID)).
		Only(ctx)
	if err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		if ent.IsNotFound(err) {
			return river.JobCancel(fmt.Errorf("directory_sync: job %s not found", jobID))
		}
		return fmt.Errorf("load directory sync job %s: %w", jobID, err)
	}

	now := time.Now().UTC()
	if _, saveErr := w.entClient.DirectorySyncJob.UpdateOneID(jobRow.ID).
		SetStatus(entdirectorysyncjob.StatusRunning).
		SetStartedAt(now).
		ClearCompletedAt().
		Save(ctx); saveErr != nil {
		if ctxErr := jobContextErr(ctx, saveErr); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("mark directory sync job running: %w", saveErr)
	}

	authProviderRow, err := w.entClient.AuthProvider.Query().
		Where(authprovider.IDEQ(jobRow.AuthProviderID)).
		Only(ctx)
	if err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("load auth provider: %w", err))
	}
	if !authProviderRow.Enabled {
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("auth provider %s is disabled", authProviderRow.ID))
	}

	adapter := adminglobal.Resolve(authProviderRow.AuthType)
	if adapter == nil {
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("no auth provider adapter registered for type %q", authProviderRow.AuthType))
	}
	directoryCapability, ok := adapter.(directorycontract.DirectorySyncCapability)
	if !ok {
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("auth provider type %q does not support directory sync", authProviderRow.AuthType))
	}

	runtimeConfig, cfgErr := w.configCodec.DecryptForUse(authProviderRow.AuthType, authProviderRow.Config)
	if cfgErr != nil {
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("decrypt auth provider config: %w", cfgErr))
	}

	records, err := directoryCapability.ListDirectoryUsers(ctx, runtimeConfig, jobRow.RequestSnapshot)
	if err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		return w.failJob(ctx, jobRow.ID, jobRow.TriggeredBy, fmt.Errorf("list directory users: %w", err))
	}

	var (
		actionSummary directorycontract.DirectoryActionSummary
		errorCount    int
		jobErrors     []string
	)
	for _, record := range records {
		var (
			result   service.DirectorySyncApplyResult
			applyErr error
		)
		switch jobRow.SyncMode {
		case "", service.DirectoryExecutionModeManualImport:
			applyErr = withJobsTx(ctx, w.entClient, func(txClient *ent.Client) error {
				txDirectorySync := w.directorySync.WithClient(txClient)
				var err error
				result, _, err = txDirectorySync.ApplyRecord(ctx, jobRow.AuthProviderID, record, jobRow.ConflictResolution)
				if err != nil {
					return err
				}
				return w.revokeDirectorySyncRBACSessions(ctx, result)
			})
		case service.DirectoryExecutionModeScheduledEnrichment:
			applyErr = withJobsTx(ctx, w.entClient, func(txClient *ent.Client) error {
				txDirectorySync := w.directorySync.WithClient(txClient)
				var err error
				result, err = txDirectorySync.ApplyEnrichmentRecord(ctx, jobRow.AuthProviderID, directorycontract.DirectoryJoinKeyType(jobRow.JoinKeyType), record)
				if err != nil {
					return err
				}
				return w.revokeDirectorySyncRBACSessions(ctx, result)
			})
		default:
			applyErr = fmt.Errorf("unsupported directory sync mode %q", jobRow.SyncMode)
		}
		if applyErr != nil {
			if ctxErr := jobContextErr(ctx, applyErr); ctxErr != nil {
				return ctxErr
			}
			errorCount++
			jobErrors = append(jobErrors, applyErr.Error())
			continue
		}
		actionSummary.Add(result.Action)
	}

	completedAt := time.Now().UTC()
	update := w.entClient.DirectorySyncJob.UpdateOneID(jobRow.ID).
		SetStatus(entdirectorysyncjob.StatusCompleted).
		SetTotalEntries(len(records)).
		SetCreateCount(actionSummary.CreateCount).
		SetUpdateCount(actionSummary.UpdateCount).
		SetBlockedCount(actionSummary.BlockedCount).
		SetErrorCount(errorCount).
		SetCompletedAt(completedAt)
	if errorCount > 0 {
		update = update.SetErrors(jobErrors)
	} else {
		update = update.SetErrors([]string{})
	}
	if _, err := update.Save(ctx); err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("complete directory sync job: %w", err)
	}

	if w.auditLogger != nil {
		if auditErr := w.auditLogger.LogAction(ctx, "auth_provider.directory_sync", "directory_sync_job", jobRow.ID, jobRow.TriggeredBy, map[string]interface{}{
			"auth_provider_id": jobRow.AuthProviderID,
			"total_entries":    len(records),
			"create_count":     actionSummary.CreateCount,
			"update_count":     actionSummary.UpdateCount,
			"blocked_count":    actionSummary.BlockedCount,
			"error_count":      errorCount,
		}); auditErr != nil {
			logger.Warn("failed to write directory sync audit log",
				zap.String("job_id", jobRow.ID),
				zap.Error(auditErr),
			)
		}
	}

	return nil
}

func (w *DirectorySyncWorker) revokeDirectorySyncRBACSessions(ctx context.Context, result service.DirectorySyncApplyResult) error {
	if w == nil || w.authSessions == nil || !result.RBACChanged || strings.TrimSpace(result.UserID) == "" {
		return nil
	}
	return w.authSessions.RevokeUserSessions(ctx, result.UserID, "directory_sync_rbac_changed")
}

func (w *DirectorySyncWorker) failJob(ctx context.Context, jobID, actor string, cause error) error {
	if ctxErr := jobContextErr(ctx, cause); ctxErr != nil {
		return ctxErr
	}
	if w == nil || w.entClient == nil {
		return cause
	}

	message := cause.Error()
	completedAt := time.Now().UTC()
	update := w.entClient.DirectorySyncJob.UpdateOneID(jobID).
		SetStatus(entdirectorysyncjob.StatusFailed).
		SetCompletedAt(completedAt).
		AddErrorCount(1).
		SetErrors([]string{message})
	if _, err := update.Save(ctx); err != nil {
		if ctxErr := jobContextErr(ctx, err); ctxErr != nil {
			return ctxErr
		}
		return errors.Join(cause, fmt.Errorf("persist directory sync job failure: %w", err))
	}

	if w.auditLogger != nil && actor != "" {
		if auditErr := w.auditLogger.LogAction(ctx, "auth_provider.directory_sync_failed", "directory_sync_job", jobID, actor, map[string]interface{}{
			"error": message,
		}); auditErr != nil {
			logger.Warn("failed to write directory sync failure audit log",
				zap.String("job_id", jobID),
				zap.Error(auditErr),
			)
		}
	}
	return nil
}
