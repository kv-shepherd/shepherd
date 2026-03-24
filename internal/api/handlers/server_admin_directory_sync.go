package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/edge/authworkspace/directoryview"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/service"
)

// GetAuthProviderDirectoryDescriptor handles GET /admin/auth-providers/{provider_id}/directory/descriptor.
func (s *Server) GetAuthProviderDirectoryDescriptor(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	_, capability, supported := s.resolveDirectorySyncCapability(ctx, c, providerID)
	if !supported {
		return
	}

	c.JSON(http.StatusOK, directoryview.DirectorySyncDescriptorToAPI(capability.DescribeDirectorySync()))
}

// GetAuthProviderDirectorySchedule handles GET /admin/auth-providers/{provider_id}/directory/schedule.
func (s *Server) GetAuthProviderDirectorySchedule(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	providerRow, _, supported := s.resolveDirectorySyncCapability(ctx, c, providerID)
	if !supported {
		return
	}

	adapter := adminglobal.Resolve(providerRow.AuthType)
	scheduledCapability, ok := adapter.(directorycontract.ScheduledDirectoryEnrichmentCapability)
	if !ok {
		c.JSON(http.StatusOK, directoryview.UnsupportedDirectoryScheduleStatus())
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for directory schedule", zap.Error(cfgErr), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	plan, err := scheduledCapability.BuildScheduledDirectoryEnrichmentPlan(ctx, runtimeConfig)
	if err != nil {
		logger.Error("failed to build scheduled directory enrichment plan", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	status, err := s.buildDirectoryScheduleStatus(ctx, providerID, plan)
	if err != nil {
		logger.Error("failed to build scheduled directory enrichment status", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, status)
}

// PreviewAuthProviderDirectory handles POST /admin/auth-providers/{provider_id}/directory/preview.
func (s *Server) PreviewAuthProviderDirectory(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:sync", "auth_provider:manage")
	if !ok {
		return
	}

	providerRow, capability, supported := s.resolveDirectorySyncCapability(ctx, c, providerID)
	if !supported {
		return
	}
	descriptor := capability.DescribeDirectorySync()
	if !descriptor.SupportsPreview {
		c.JSON(http.StatusNotImplemented, generated.Error{Code: "DIRECTORY_SYNC_PREVIEW_NOT_SUPPORTED"})
		return
	}

	var req generated.DirectorySyncPreviewRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	if req.ProviderRequest == nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "provider_request is required"})
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for directory preview", zap.Error(cfgErr), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	preview, err := capability.PreviewDirectorySync(ctx, runtimeConfig, req.ProviderRequest)
	if err != nil {
		if isDirectorySyncRequestError(err) {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		logger.Error("failed to preview directory sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.directorySync == nil {
		logger.Error("directory sync service is not initialized")
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	canonicalPreview, err := s.directorySync.CanonicalizePreview(ctx, providerID, preview)
	if err != nil {
		logger.Error("failed to classify directory sync preview", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, directoryview.DirectorySyncPreviewToAPI(canonicalPreview))
}

// SyncAuthProviderDirectory handles POST /admin/auth-providers/{provider_id}/directory/sync.
func (s *Server) SyncAuthProviderDirectory(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:sync", "auth_provider:manage")
	if !ok {
		return
	}

	providerRow, capability, supported := s.resolveDirectorySyncCapability(ctx, c, providerID)
	if !supported {
		return
	}
	if s.riverClient == nil {
		logger.Error("river client is not initialized for directory sync enqueue", zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.DirectorySyncRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	if req.ProviderRequest == nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "provider_request is required"})
		return
	}
	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for directory sync", zap.Error(cfgErr), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if capability.DescribeDirectorySync().SupportsPreview {
		if _, previewErr := capability.PreviewDirectorySync(ctx, runtimeConfig, req.ProviderRequest); previewErr != nil && isDirectorySyncRequestError(previewErr) {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: previewErr.Error()})
			return
		}
	}

	conflictResolution := strings.TrimSpace(string(req.ConflictResolution))
	if conflictResolution == "" {
		conflictResolution = service.DirectoryConflictResolutionSkip
	}

	jobUUID, err := uuid.NewV7()
	if err != nil {
		logger.Error("failed to generate directory sync job id", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	jobID := jobUUID.String()

	jobRow, err := s.client.DirectorySyncJob.Create().
		SetID(jobID).
		SetAuthProviderID(providerID).
		SetRequestSnapshot(req.ProviderRequest).
		SetConflictResolution(conflictResolution).
		SetSyncMode(service.DirectoryExecutionModeManualImport).
		SetTriggeredBy(actor).
		Save(ctx)
	if err != nil {
		logger.Error("failed to create directory sync job", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := s.riverClient.Insert(ctx, jobs.DirectorySyncArgs{
		AuthProviderID: providerID,
		JobID:          jobRow.ID,
	}, nil); err != nil {
		logger.Error("failed to enqueue directory sync job", zap.Error(err), zap.String("provider_id", providerID), zap.String("job_id", jobRow.ID))
		_, _ = s.client.DirectorySyncJob.UpdateOneID(jobRow.ID).
			SetStatus(directorysyncjob.StatusFailed).
			SetErrorCount(1).
			SetErrors([]string{err.Error()}).
			SetCompletedAt(time.Now().UTC()).
			Save(ctx)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.directory_sync_requested", "directory_sync_job", jobRow.ID, actor, map[string]interface{}{
			"auth_provider_id": providerID,
		})
	}

	c.JSON(http.StatusAccepted, generated.DirectorySyncStartResponse{
		JobId:  jobRow.ID,
		Status: generated.DirectorySyncStartResponseStatus(jobRow.Status),
	})
}

// ListAuthProviderDirectorySyncJobs handles GET /admin/auth-providers/{provider_id}/directory/sync-jobs.
func (s *Server) ListAuthProviderDirectorySyncJobs(c *gin.Context, providerID generated.ProviderID, params generated.ListAuthProviderDirectorySyncJobsParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	if _, _, supported := s.resolveDirectorySyncCapability(ctx, c, providerID); !supported {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	query := s.client.DirectorySyncJob.Query().
		Where(directorysyncjob.AuthProviderIDEQ(providerID)).
		Order(ent.Desc(directorysyncjob.FieldCreatedAt))

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count directory sync jobs", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	rows, err := query.Offset(offset).Limit(perPage).All(ctx)
	if err != nil {
		logger.Error("failed to list directory sync jobs", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, directoryview.DirectorySyncJobListToAPI(rows, page, perPage, total))
}

// GetAuthProviderDirectorySyncJob handles GET /admin/auth-providers/{provider_id}/directory/sync-jobs/{job_id}.
func (s *Server) GetAuthProviderDirectorySyncJob(c *gin.Context, providerID generated.ProviderID, jobID generated.DirectorySyncJobID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	if _, _, supported := s.resolveDirectorySyncCapability(ctx, c, providerID); !supported {
		return
	}

	row, err := s.client.DirectorySyncJob.Query().
		Where(
			directorysyncjob.IDEQ(jobID),
			directorysyncjob.AuthProviderIDEQ(providerID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "DIRECTORY_SYNC_JOB_NOT_FOUND"})
			return
		}
		logger.Error("failed to get directory sync job", zap.Error(err), zap.String("provider_id", providerID), zap.String("job_id", jobID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, directoryview.DirectorySyncJobDetailToAPI(row))
}

func (s *Server) resolveDirectorySyncCapability(
	ctx context.Context,
	c *gin.Context,
	providerID generated.ProviderID,
) (*ent.AuthProvider, directorycontract.DirectorySyncCapability, bool) {
	providerRow, err := s.client.AuthProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return nil, nil, false
		}
		logger.Error("failed to get auth provider for directory sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, nil, false
	}

	capability, err := directoryview.ResolveDirectorySyncCapability(providerRow)
	if errors.Is(err, directoryview.ErrAuthProviderAdapterNotFound) {
		logger.Error("no auth provider adapter registered for directory sync", zap.String("provider_id", providerID), zap.String("auth_type", providerRow.AuthType))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, nil, false
	}
	if errors.Is(err, directoryview.ErrDirectorySyncUnsupported) {
		c.JSON(http.StatusNotImplemented, generated.Error{Code: "DIRECTORY_SYNC_NOT_SUPPORTED"})
		return nil, nil, false
	}
	if err != nil {
		logger.Error("failed to resolve directory sync capability", zap.Error(err), zap.String("provider_id", providerID), zap.String("auth_type", providerRow.AuthType))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, nil, false
	}
	return providerRow, capability, true
}

func isDirectorySyncRequestError(err error) bool {
	var requestErr *directorycontract.DirectorySyncRequestError
	return errors.As(err, &requestErr)
}

func (s *Server) buildDirectoryScheduleStatus(
	ctx context.Context,
	providerID string,
	plan *directorycontract.ScheduledDirectoryEnrichmentPlan,
) (generated.DirectoryEnrichmentScheduleStatus, error) {
	pendingJob, latestJob, err := directoryview.QueryLatestDirectoryScheduleJobs(
		ctx,
		s.client,
		providerID,
		service.DirectoryExecutionModeScheduledEnrichment,
	)
	if err != nil {
		return generated.DirectoryEnrichmentScheduleStatus{}, err
	}

	return directoryview.DirectoryScheduleStatusFromPlan(plan, pendingJob, latestJob, time.Now().UTC())
}
