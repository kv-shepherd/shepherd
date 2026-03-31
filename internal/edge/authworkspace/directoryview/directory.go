package directoryview

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/internal/api/generated"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
)

var (
	ErrAuthProviderAdapterNotFound = errors.New("auth provider adapter not found")
	ErrDirectorySyncUnsupported    = errors.New("directory sync not supported")
)

func DirectorySyncDescriptorToAPI(desc directorycontract.DirectorySyncDescriptor) generated.DirectorySyncDescriptor {
	return generated.DirectorySyncDescriptor{
		Description:     strings.TrimSpace(desc.Description),
		DisplayName:     desc.DisplayName,
		RequestSchema:   desc.RequestSchema,
		SupportsPreview: desc.SupportsPreview,
	}
}

func DirectorySyncPreviewToAPI(preview *directorycontract.DirectorySyncPreview) generated.DirectorySyncPreview {
	if preview == nil {
		return generated.DirectorySyncPreview{}
	}
	items := make([]generated.DirectorySyncPreviewItem, 0, len(preview.Items))
	for i := range preview.Items {
		item := &preview.Items[i]
		conflicts := make([]generated.DirectorySyncConflict, 0, len(item.Conflicts))
		for _, conflict := range item.Conflicts {
			conflicts = append(conflicts, generated.DirectorySyncConflict{
				Code:           generated.DirectorySyncConflictCode(conflict.Code),
				ExistingUserId: strings.TrimSpace(conflict.ExistingUserID),
				Field:          strings.TrimSpace(conflict.Field),
				Message:        strings.TrimSpace(conflict.Message),
			})
		}

		items = append(items, generated.DirectorySyncPreviewItem{
			Conflicts: conflicts,
			Match: generated.DirectorySyncPreviewMatch{
				Action:         generated.DirectorySyncPreviewMatchAction(item.Match.Action),
				ExistingUserId: strings.TrimSpace(item.Match.ExistingUserID),
				MatchedBy:      generated.DirectorySyncPreviewMatchMatchedBy(item.Match.MatchedBy),
			},
			Record: generated.DirectoryUserRecord{
				Attributes:  directorycontract.CloneDirectoryAttributes(item.Record.Attributes),
				Cohorts:     directorySyncRecordCohortsToAPI(item.Record.Cohorts),
				DisplayName: item.Record.DisplayName,
				Email:       strings.TrimSpace(item.Record.Email),
				ExternalId:  item.Record.ExternalID,
				Username:    item.Record.Username,
			},
			Warnings: item.Warnings,
		})
	}
	return generated.DirectorySyncPreview{
		Items:      items,
		TotalCount: preview.TotalCount,
	}
}

func DirectorySyncJobToAPI(row *ent.DirectorySyncJob) generated.DirectorySyncJob {
	jobErrors := cloneStringsOrEmpty(row.Errors)
	return generated.DirectorySyncJob{
		CompletedAt:        derefTime(row.CompletedAt),
		ConflictResolution: generated.DirectorySyncJobConflictResolution(row.ConflictResolution),
		CreatedAt:          row.CreatedAt,
		ErrorCount:         row.ErrorCount,
		Errors:             jobErrors,
		Id:                 row.ID,
		JoinKeyType:        row.JoinKeyType,
		ProviderId:         row.AuthProviderID,
		ResultSummary: generated.DirectoryActionSummary{
			BlockedCount: row.BlockedCount,
			CreateCount:  row.CreateCount,
			UpdateCount:  row.UpdateCount,
		},
		SyncMode:     generated.DirectorySyncJobSyncMode(row.SyncMode),
		StartedAt:    derefTime(row.StartedAt),
		Status:       generated.DirectorySyncJobStatus(row.Status),
		TotalEntries: row.TotalEntries,
		TriggeredBy:  row.TriggeredBy,
		UpdatedAt:    row.UpdatedAt,
	}
}

func DirectorySyncJobListToAPI(rows []*ent.DirectorySyncJob, page, perPage, total int) generated.DirectorySyncJobList {
	items := make([]generated.DirectorySyncJob, 0, len(rows))
	for _, row := range rows {
		items = append(items, DirectorySyncJobToAPI(row))
	}
	totalPages := 0
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	return generated.DirectorySyncJobList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

func DirectorySyncJobDetailToAPI(row *ent.DirectorySyncJob) generated.DirectorySyncJobDetail {
	return generated.DirectorySyncJobDetail{
		CompletedAt:        derefTime(row.CompletedAt),
		ConflictResolution: generated.DirectorySyncJobDetailConflictResolution(row.ConflictResolution),
		CreatedAt:          row.CreatedAt,
		ErrorCount:         row.ErrorCount,
		Errors:             cloneStringsOrEmpty(row.Errors),
		Id:                 row.ID,
		JoinKeyType:        row.JoinKeyType,
		ProviderId:         row.AuthProviderID,
		RequestSnapshot:    directorycontract.CloneDirectoryAttributes(row.RequestSnapshot),
		ResultSummary: generated.DirectoryActionSummary{
			BlockedCount: row.BlockedCount,
			CreateCount:  row.CreateCount,
			UpdateCount:  row.UpdateCount,
		},
		StartedAt:    derefTime(row.StartedAt),
		Status:       generated.DirectorySyncJobDetailStatus(row.Status),
		SyncMode:     generated.DirectorySyncJobDetailSyncMode(row.SyncMode),
		TotalEntries: row.TotalEntries,
		TriggeredBy:  row.TriggeredBy,
		UpdatedAt:    row.UpdatedAt,
	}
}

func cloneStringsOrEmpty(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func UnsupportedDirectoryScheduleStatus() generated.DirectoryEnrichmentScheduleStatus {
	return generated.DirectoryEnrichmentScheduleStatus{
		Supported: false,
		Enabled:   false,
	}
}

func ResolveDirectorySyncCapability(row *ent.AuthProvider) (directorycontract.DirectorySyncCapability, error) {
	adapter := adminglobal.Resolve(row.AuthType)
	if adapter == nil {
		return nil, ErrAuthProviderAdapterNotFound
	}
	capability, ok := adapter.(directorycontract.DirectorySyncCapability)
	if !ok {
		return nil, ErrDirectorySyncUnsupported
	}
	return capability, nil
}

func QueryLatestDirectoryScheduleJobs(
	ctx context.Context,
	client *ent.Client,
	providerID string,
	syncMode directorysyncjob.SyncMode,
) (pendingJob, latestJob *ent.DirectorySyncJob, err error) {
	pendingJob, err = client.DirectorySyncJob.Query().
		Where(
			directorysyncjob.AuthProviderIDEQ(providerID),
			directorysyncjob.SyncModeEQ(syncMode),
			directorysyncjob.StatusIn(directorysyncjob.StatusPending, directorysyncjob.StatusRunning),
		).
		Order(ent.Desc(directorysyncjob.FieldCreatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, nil, err
	}
	if ent.IsNotFound(err) {
		pendingJob = nil
	}

	latestJob, err = client.DirectorySyncJob.Query().
		Where(
			directorysyncjob.AuthProviderIDEQ(providerID),
			directorysyncjob.SyncModeEQ(syncMode),
		).
		Order(ent.Desc(directorysyncjob.FieldCreatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, nil, err
	}
	if ent.IsNotFound(err) {
		latestJob = nil
	}
	return pendingJob, latestJob, nil
}

func DirectoryScheduleStatusFromPlan(
	plan *directorycontract.ScheduledDirectoryEnrichmentPlan,
	pendingJob *ent.DirectorySyncJob,
	latestJob *ent.DirectorySyncJob,
	now time.Time,
) (generated.DirectoryEnrichmentScheduleStatus, error) {
	normalizedPlan, location, err := directorycontract.NormalizeScheduledDirectoryEnrichmentPlan(plan)
	if err != nil {
		return generated.DirectoryEnrichmentScheduleStatus{}, err
	}

	status := generated.DirectoryEnrichmentScheduleStatus{
		Supported: true,
		Enabled:   normalizedPlan.Enabled,
	}
	if !normalizedPlan.Enabled {
		return status, nil
	}

	status.Mode = generated.DirectoryEnrichmentScheduleStatusMode(normalizedPlan.Mode)
	status.JoinKeyType = generated.DirectoryEnrichmentScheduleStatusJoinKeyType(normalizedPlan.JoinKeyType)
	status.ScheduleCron = normalizedPlan.ScheduleCron
	status.ScheduleTimezone = normalizedPlan.ScheduleTimezone
	status.ProviderRequest = directorycontract.CloneDirectoryAttributes(normalizedPlan.ProviderRequest)

	if pendingJob != nil {
		status.PendingJobId = pendingJob.ID
		status.PendingJobStatus = generated.DirectoryEnrichmentScheduleStatusPendingJobStatus(pendingJob.Status)
	}

	if latestJob != nil {
		status.LastJobId = latestJob.ID
		status.LastJobStatus = generated.DirectoryEnrichmentScheduleStatusLastJobStatus(latestJob.Status)
		status.LastJobCreatedAt = latestJob.CreatedAt
		status.LastJobCompletedAt = derefTime(latestJob.CompletedAt)
	}

	if status.PendingJobId != "" {
		return status, nil
	}

	schedule, err := cron.ParseStandard(normalizedPlan.ScheduleCron)
	if err != nil {
		return generated.DirectoryEnrichmentScheduleStatus{}, err
	}
	if latestJob == nil {
		status.NextRunAt = schedule.Next(now.In(location)).UTC()
		return status, nil
	}
	status.NextRunAt = schedule.Next(latestJob.CreatedAt.In(location)).UTC()
	return status, nil
}

func directorySyncRecordCohortsToAPI(cohorts []directorycontract.ExternalCohort) []generated.DirectoryExternalCohort {
	if len(cohorts) == 0 {
		return nil
	}
	items := make([]generated.DirectoryExternalCohort, 0, len(cohorts))
	for _, cohort := range cohorts {
		items = append(items, generated.DirectoryExternalCohort{
			DisplayName: strings.TrimSpace(cohort.DisplayName),
			Key:         strings.TrimSpace(cohort.Key),
			Kind:        strings.TrimSpace(cohort.Kind),
		})
	}
	return items
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
