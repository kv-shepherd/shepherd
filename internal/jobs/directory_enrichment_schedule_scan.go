package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entdirectorysyncjob "kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	configcodec "kv-shepherd.io/shepherd/internal/provider/configcodec"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	DirectoryEnrichmentScheduleScanJobKind         = "directory_enrichment_schedule_scan"
	DefaultDirectoryEnrichmentScheduleScanInterval = 5 * time.Minute
	directoryEnrichmentSchedulerActor              = "system:directory-enrichment-scheduler"
)

// DirectoryEnrichmentScheduleScanArgs scans enabled provider plans and enqueues
// scheduled enrichment jobs when due.
type DirectoryEnrichmentScheduleScanArgs struct{}

func (DirectoryEnrichmentScheduleScanArgs) Kind() string {
	return DirectoryEnrichmentScheduleScanJobKind
}

func (DirectoryEnrichmentScheduleScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: DefaultDirectoryEnrichmentScheduleScanInterval,
			ByQueue:  true,
			ByArgs:   true,
		},
	}
}

// DirectoryEnrichmentScheduleScanWorker periodically scans provider-owned
// plans and enqueues canonical scheduled enrichment jobs.
type DirectoryEnrichmentScheduleScanWorker struct {
	river.WorkerDefaults[DirectoryEnrichmentScheduleScanArgs]
	entClient           *ent.Client
	riverClientProvider func() *river.Client[pgx.Tx]
	auditLogger         *audit.Logger
	configCodec         *configcodec.AuthProviderConfigCodec
}

func NewDirectoryEnrichmentScheduleScanWorker(
	entClient *ent.Client,
	riverClientProvider func() *river.Client[pgx.Tx],
	auditLogger *audit.Logger,
	encryptionKey []byte,
) *DirectoryEnrichmentScheduleScanWorker {
	return &DirectoryEnrichmentScheduleScanWorker{
		entClient:           entClient,
		riverClientProvider: riverClientProvider,
		auditLogger:         auditLogger,
		configCodec:         configcodec.NewAuthProviderConfigCodec(encryptionKey),
	}
}

func (w *DirectoryEnrichmentScheduleScanWorker) Work(ctx context.Context, _ *river.Job[DirectoryEnrichmentScheduleScanArgs]) error {
	if w == nil || w.entClient == nil || w.riverClientProvider == nil {
		return fmt.Errorf("directory enrichment schedule scan worker is not initialized")
	}

	providers, err := w.entClient.AuthProvider.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("list auth providers for scheduled enrichment: %w", err)
	}

	now := time.Now().UTC()
	for _, authProviderRow := range providers {
		if err := w.enqueueScheduledEnrichmentIfDue(ctx, authProviderRow, now); err != nil {
			logger.Warn("failed to evaluate scheduled directory enrichment plan",
				zap.String("provider_id", authProviderRow.ID),
				zap.String("auth_type", authProviderRow.AuthType),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (w *DirectoryEnrichmentScheduleScanWorker) enqueueScheduledEnrichmentIfDue(
	ctx context.Context,
	authProviderRow *ent.AuthProvider,
	now time.Time,
) error {
	if authProviderRow == nil {
		return nil
	}
	riverClient := w.riverClientProvider()
	if riverClient == nil {
		return fmt.Errorf("directory enrichment schedule scan worker river client is not initialized")
	}

	adapter := adminglobal.Resolve(authProviderRow.AuthType)
	if adapter == nil {
		return nil
	}
	if _, ok := adapter.(directorycontract.DirectorySyncCapability); !ok {
		return nil
	}
	scheduledCapability, ok := adapter.(directorycontract.ScheduledDirectoryEnrichmentCapability)
	if !ok {
		return nil
	}

	runtimeConfig, err := w.configCodec.DecryptForUse(authProviderRow.AuthType, authProviderRow.Config)
	if err != nil {
		return fmt.Errorf("decrypt auth provider config: %w", err)
	}

	plan, err := scheduledCapability.BuildScheduledDirectoryEnrichmentPlan(ctx, runtimeConfig)
	if err != nil {
		return fmt.Errorf("build scheduled enrichment plan: %w", err)
	}
	plan, location, err := directorycontract.NormalizeScheduledDirectoryEnrichmentPlan(plan)
	if err != nil {
		return err
	}
	if !plan.Enabled {
		return nil
	}

	due, err := w.scheduledDirectoryEnrichmentDue(ctx, authProviderRow.ID, plan.ScheduleCron, location, now)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}

	jobID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate directory enrichment job id: %w", err)
	}

	jobRow, err := w.entClient.DirectorySyncJob.Create().
		SetID(jobID.String()).
		SetAuthProviderID(authProviderRow.ID).
		SetRequestSnapshot(cloneJSONMap(plan.ProviderRequest)).
		SetConflictResolution(service.DirectoryConflictResolutionSkip).
		SetSyncMode(service.DirectoryExecutionModeScheduledEnrichment).
		SetJoinKeyType(string(plan.JoinKeyType)).
		SetTriggeredBy(directoryEnrichmentSchedulerActor).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create scheduled directory enrichment job: %w", err)
	}

	if _, err := riverClient.Insert(ctx, DirectorySyncArgs{
		JobID: jobRow.ID,
	}, nil); err != nil {
		_, _ = w.entClient.DirectorySyncJob.UpdateOneID(jobRow.ID).
			SetStatus(entdirectorysyncjob.StatusFailed).
			SetErrorCount(1).
			SetErrors([]string{err.Error()}).
			SetCompletedAt(time.Now().UTC()).
			Save(ctx)
		return fmt.Errorf("enqueue scheduled directory enrichment job: %w", err)
	}

	if w.auditLogger != nil {
		if auditErr := w.auditLogger.LogAction(ctx, "auth_provider.directory_enrichment_scheduled", "directory_sync_job", jobRow.ID, directoryEnrichmentSchedulerActor, map[string]interface{}{
			"auth_provider_id": authProviderRow.ID,
			"sync_mode":        service.DirectoryExecutionModeScheduledEnrichment,
			"join_key_type":    jobRow.JoinKeyType,
		}); auditErr != nil {
			logger.Warn("failed to write directory enrichment scheduler audit log",
				zap.String("provider_id", authProviderRow.ID),
				zap.String("job_id", jobRow.ID),
				zap.Error(auditErr),
			)
		}
	}

	return nil
}

func (w *DirectoryEnrichmentScheduleScanWorker) scheduledDirectoryEnrichmentDue(
	ctx context.Context,
	authProviderID string,
	scheduleExpr string,
	location *time.Location,
	now time.Time,
) (bool, error) {
	pendingCount, err := w.entClient.DirectorySyncJob.Query().
		Where(
			entdirectorysyncjob.AuthProviderIDEQ(authProviderID),
			entdirectorysyncjob.SyncModeEQ(service.DirectoryExecutionModeScheduledEnrichment),
			entdirectorysyncjob.StatusIn(
				entdirectorysyncjob.StatusPending,
				entdirectorysyncjob.StatusRunning,
			),
		).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count pending scheduled enrichment jobs: %w", err)
	}
	if pendingCount > 0 {
		return false, nil
	}

	latestJob, err := w.entClient.DirectorySyncJob.Query().
		Where(
			entdirectorysyncjob.AuthProviderIDEQ(authProviderID),
			entdirectorysyncjob.SyncModeEQ(service.DirectoryExecutionModeScheduledEnrichment),
		).
		Order(ent.Desc(entdirectorysyncjob.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("query latest scheduled enrichment job: %w", err)
	}

	schedule, err := cron.ParseStandard(strings.TrimSpace(scheduleExpr))
	if err != nil {
		return false, fmt.Errorf("parse schedule cron %q: %w", scheduleExpr, err)
	}

	nextRun := schedule.Next(latestJob.CreatedAt.In(location))
	return !nextRun.After(now.In(location)), nil
}

func cloneJSONMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
