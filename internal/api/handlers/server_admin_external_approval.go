package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	approvalbuiltin "kv-shepherd.io/shepherd/internal/governance/approval/builtin"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type externalApprovalSystemCreateRequest struct {
	Name                string            `json:"name" binding:"required"`
	Type                string            `json:"type"`
	Enabled             *bool             `json:"enabled"`
	WebhookURL          string            `json:"webhook_url" binding:"required"`
	WebhookHeaders      map[string]string `json:"webhook_headers"`
	TimeoutSeconds      *int              `json:"timeout_seconds"`
	RetryCount          *int              `json:"retry_count"`
	RetryBackoffSeconds *int              `json:"retry_backoff_seconds"`
	SigningKey          string            `json:"signing_key" binding:"required"`
	SortOrder           *int              `json:"sort_order"`
}

type externalApprovalSystemUpdateRequest struct {
	Name                *string            `json:"name"`
	Enabled             *bool              `json:"enabled"`
	WebhookURL          *string            `json:"webhook_url"`
	WebhookHeaders      *map[string]string `json:"webhook_headers"`
	TimeoutSeconds      *int               `json:"timeout_seconds"`
	RetryCount          *int               `json:"retry_count"`
	RetryBackoffSeconds *int               `json:"retry_backoff_seconds"`
	SigningKey          *string            `json:"signing_key"`
	SortOrder           *int               `json:"sort_order"`
}

// ListExternalApprovalSystems handles GET /admin/external-approval-systems.
func (s *Server) ListExternalApprovalSystems(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	if s.externalApprovalRegistry == nil {
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	systems, err := s.externalApprovalRegistry.List(ctx)
	if err != nil {
		logger.Error("failed to list external approval systems", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	items := make([]generated.ExternalApprovalSystem, 0, len(systems))
	for i := range systems {
		items = append(items, externalApprovalSystemToAPI(systems[i]))
	}
	c.JSON(http.StatusOK, generated.ExternalApprovalSystemList{Items: items})
}

// CreateExternalApprovalSystem handles POST /admin/external-approval-systems.
func (s *Server) CreateExternalApprovalSystem(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	if s.externalApprovalRegistry == nil {
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req externalApprovalSystemCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	system, err := s.externalApprovalRegistry.Create(ctx, approvalregistry.CreateInput{
		Name:                req.Name,
		ProviderType:        req.Type,
		Enabled:             req.Enabled,
		WebhookURL:          req.WebhookURL,
		WebhookHeaders:      req.WebhookHeaders,
		TimeoutSeconds:      req.TimeoutSeconds,
		RetryCount:          req.RetryCount,
		RetryBackoffSeconds: req.RetryBackoffSeconds,
		SigningKey:          req.SigningKey,
		SortOrder:           req.SortOrder,
		CreatedBy:           actor,
	})
	if err != nil {
		writeExternalApprovalSystemError(c, err, "create external approval system")
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "external_approval_system.create", "external_approval_system", system.ID, actor, map[string]interface{}{
			"type": system.ProviderType,
		})
	}
	s.refreshExternalApprovalProvider(ctx)
	c.JSON(http.StatusCreated, externalApprovalSystemToAPI(*system))
}

// UpdateExternalApprovalSystem handles PATCH /admin/external-approval-systems/{system_id}.
func (s *Server) UpdateExternalApprovalSystem(c *gin.Context, systemID generated.ExternalApprovalSystemID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	if s.externalApprovalRegistry == nil {
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req externalApprovalSystemUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	system, err := s.externalApprovalRegistry.Update(ctx, systemID, approvalregistry.UpdateInput{
		Name:                req.Name,
		Enabled:             req.Enabled,
		WebhookURL:          req.WebhookURL,
		WebhookHeaders:      req.WebhookHeaders,
		TimeoutSeconds:      req.TimeoutSeconds,
		RetryCount:          req.RetryCount,
		RetryBackoffSeconds: req.RetryBackoffSeconds,
		SigningKey:          req.SigningKey,
		SortOrder:           req.SortOrder,
	})
	if err != nil {
		writeExternalApprovalSystemError(c, err, "update external approval system")
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "external_approval_system.update", "external_approval_system", system.ID, actor, nil)
	}
	s.refreshExternalApprovalProvider(ctx)
	c.JSON(http.StatusOK, externalApprovalSystemToAPI(*system))
}

// DeleteExternalApprovalSystem handles DELETE /admin/external-approval-systems/{system_id}.
func (s *Server) DeleteExternalApprovalSystem(c *gin.Context, systemID generated.ExternalApprovalSystemID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	if s.externalApprovalRegistry == nil {
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if err := s.externalApprovalRegistry.Delete(ctx, systemID); err != nil {
		writeExternalApprovalSystemError(c, err, "delete external approval system")
		return
	}
	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "external_approval_system.delete", "external_approval_system", systemID, actor, nil)
	}
	s.refreshExternalApprovalProvider(ctx)
	c.Status(http.StatusNoContent)
}

func writeExternalApprovalSystemError(c *gin.Context, err error, operation string) {
	switch {
	case approvalregistry.IsValidationError(err):
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
	case ent.IsNotFound(err):
		c.JSON(http.StatusNotFound, generated.Error{Code: "EXTERNAL_APPROVAL_SYSTEM_NOT_FOUND"})
	case ent.IsConstraintError(err):
		c.JSON(http.StatusConflict, generated.Error{Code: "EXTERNAL_APPROVAL_SYSTEM_NAME_EXISTS"})
	default:
		logger.Error("external approval system operation failed", zap.String("operation", operation), zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
	}
}

func externalApprovalSystemToAPI(system approvalregistry.System) generated.ExternalApprovalSystem {
	headers := make(map[string]string, len(system.WebhookHeaders))
	for key, value := range system.WebhookHeaders {
		headers[key] = value
	}
	return generated.ExternalApprovalSystem{
		Id:                  system.ID,
		Name:                system.Name,
		Type:                generated.ExternalApprovalSystemType(system.ProviderType),
		Enabled:             system.Enabled,
		WebhookUrl:          system.WebhookURL,
		WebhookHeaders:      headers,
		TimeoutSeconds:      system.TimeoutSeconds,
		RetryCount:          system.RetryCount,
		RetryBackoffSeconds: system.RetryBackoffSeconds,
		SigningKeySet:       system.SigningKeySet,
		SortOrder:           system.SortOrder,
		CreatedBy:           system.CreatedBy,
		CreatedAt:           system.CreatedAt,
		UpdatedAt:           system.UpdatedAt,
	}
}

func (s *Server) refreshExternalApprovalProvider(ctx context.Context) {
	if s == nil || s.approvalRouter == nil || s.externalApprovalRegistry == nil || s.ticketService == nil {
		return
	}
	provider, err := s.externalApprovalRegistry.ActiveProvider(ctx, approvalbuiltin.NewProvider(s.ticketService))
	if err != nil {
		logger.Warn("failed to refresh external approval provider", zap.Error(err))
		return
	}
	if err := s.approvalRouter.SetActiveProvider(provider); err != nil {
		logger.Warn("failed to install refreshed external approval provider", zap.Error(err))
	}
}
