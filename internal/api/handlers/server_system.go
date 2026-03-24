package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	rrb "kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	serviceContextVMLimit      = 6
	serviceContextRequestLimit = 8
)

// ListSystems handles GET /systems.
func (s *Server) ListSystems(c *gin.Context, params generated.ListSystemsParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:read") {
		return
	}
	actor := middleware.GetUserID(ctx)

	query := s.client.System.Query()
	if !hasPlatformAdmin(c) {
		systemIDs, err := s.visibleSystemIDs(ctx, actor)
		if err != nil {
			if isRequestContextCanceled(err) {
				return
			}
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if len(systemIDs) == 0 {
			page, perPage := defaultPagination(params.Page, params.PerPage)
			c.JSON(http.StatusOK, generated.SystemList{
				Items: []generated.System{},
				Pagination: generated.Pagination{
					Page:       page,
					PerPage:    perPage,
					Total:      0,
					TotalPages: 0,
				},
			})
			return
		}

		query = query.Where(entsystem.IDIn(systemIDs...))
	}

	// Pagination.
	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting systems", zap.Error(err))
			return
		}
		logger.Error("failed to count systems", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	systems, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entsystem.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing systems", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list systems", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.System, 0, len(systems))
	for _, sys := range systems {
		items = append(items, systemToAPI(sys))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.SystemList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateSystem handles POST /systems (self-service, no approval).
func (s *Server) CreateSystem(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:write") {
		return
	}
	actor := middleware.GetUserID(ctx)

	var req generated.SystemCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	// Atomic: create System + ResourceRoleBinding.
	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to start transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	id, _ := uuid.NewV7()
	create := tx.System.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetCreatedBy(actor)
	if req.Description != "" {
		create = create.SetDescription(req.Description)
	}

	sys, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "SYSTEM_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create system", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Auto-assign creator as owner.
	rbID, _ := uuid.NewV7()
	_, err = tx.ResourceRoleBinding.Create().
		SetID(rbID.String()).
		SetUserID(actor).
		SetResourceType("system").
		SetResourceID(sys.ID).
		SetRole("owner").
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create resource role binding",
			zap.Error(err),
			zap.String("system_id", sys.ID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit system creation", zap.Error(err), zap.String("system_id", sys.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "system.create", "system", sys.ID, actor, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "system.create"),
				zap.String("resource_id", sys.ID),
			)
		}
	}

	c.JSON(http.StatusCreated, systemToAPI(sys))
}

// GetSystem handles GET /systems/{system_id}.
func (s *Server) GetSystem(c *gin.Context, systemID generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemID, "view"); !ok {
		return
	}

	sys, err := s.client.System.Get(ctx, systemID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, systemToAPI(sys))
}

// UpdateSystem handles PATCH /systems/{system_id}.
// Stage 4.C: only description is mutable; name is immutable.
func (s *Server) UpdateSystem(c *gin.Context, systemID generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:write") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "update")
	if !ok {
		return
	}

	var req generated.SystemUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	existing, err := s.client.System.Get(ctx, systemID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for update", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	updated, err := s.client.System.UpdateOneID(systemID).
		SetDescription(req.Description).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update system",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.update", "system", systemID, actor, map[string]interface{}{
			"field": "description",
			"old":   existing.Description,
			"new":   req.Description,
		})
	}

	c.JSON(http.StatusOK, systemToAPI(updated))
}

// DeleteSystem handles DELETE /systems/{system_id}.
// ADR-0015 §13 addendum: confirm_name query param required.
func (s *Server) DeleteSystem(c *gin.Context, systemID generated.SystemID, params generated.DeleteSystemParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:delete") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "delete")
	if !ok {
		return
	}

	// Check for child services via edge.
	count, err := s.client.System.Query().
		Where(entsystem.IDEQ(systemID)).
		QueryServices().
		Count(ctx)
	if err != nil {
		logger.Error("failed to count system services", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "SYSTEM_HAS_SERVICES",
			Message: "cannot delete system with existing services; delete all services first",
			Params: map[string]interface{}{
				"service_count": count,
			},
		})
		return
	}

	sys, err := s.client.System.Get(ctx, systemID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for delete", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Confirmation gate (ADR-0015 §13 addendum): confirm_name must match system name.
	if params.ConfirmName == "" || params.ConfirmName != sys.Name {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "DELETE_CONFIRMATION_REQUIRED",
			Message: "confirm_name query parameter must match system name exactly",
		})
		return
	}

	if err := s.client.System.DeleteOneID(systemID).Exec(ctx); err != nil {
		logger.Error("failed to delete system", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "system.delete", "system", systemID, actor, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "system.delete"),
				zap.String("resource_id", systemID),
			)
		}
	}

	c.Status(http.StatusNoContent)
}

// ListServicesOverview handles GET /services.
func (s *Server) ListServicesOverview(c *gin.Context, params generated.ListServicesOverviewParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}

	query := s.client.Service.Query()
	if params.SystemId != "" {
		query = query.Where(entservice.HasSystemWith(entsystem.IDEQ(params.SystemId)))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	if !hasPlatformAdmin(c) {
		actor := middleware.GetUserID(ctx)
		systemIDs, err := s.visibleSystemIDs(ctx, actor)
		if err != nil {
			if isRequestContextCanceled(err) {
				return
			}
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if len(systemIDs) == 0 {
			c.JSON(http.StatusOK, generated.ServiceList{
				Items: []generated.Service{},
				Pagination: generated.Pagination{
					Page:       page,
					PerPage:    perPage,
					Total:      0,
					TotalPages: 0,
				},
			})
			return
		}
		query = query.Where(entservice.HasSystemWith(entsystem.IDIn(systemIDs...)))
	}

	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting services overview", zap.Error(err))
			return
		}
		logger.Error("failed to count services overview", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	services, err := query.Clone().
		WithSystem().
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entservice.FieldCreatedAt), ent.Asc(entservice.FieldName)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing services overview", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list services overview", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Service, 0, len(services))
	for _, svc := range services {
		if svc.Edges.System == nil {
			logger.Error("service overview missing loaded system edge", zap.String("service_id", svc.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		items = append(items, serviceToAPI(svc, svc.Edges.System))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.ServiceList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ListServices handles GET /systems/{system_id}/services.
func (s *Server) ListServices(c *gin.Context, systemID generated.SystemID, params generated.ListServicesParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemID, "view"); !ok {
		return
	}

	query := s.client.Service.Query().
		Where(entservice.HasSystemWith(entsystem.IDEQ(systemID)))

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count services", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	services, err := query.Clone().
		WithSystem(func(q *ent.SystemQuery) {
			q.Where(entsystem.IDEQ(systemID))
		}).
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entservice.FieldCreatedAt), ent.Asc(entservice.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list services", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Service, 0, len(services))
	for _, svc := range services {
		if svc.Edges.System == nil {
			logger.Error("service list missing loaded system edge", zap.String("service_id", svc.ID), zap.String("system_id", systemID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		items = append(items, serviceToAPI(svc, svc.Edges.System))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.ServiceList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateService handles POST /systems/{system_id}/services.
func (s *Server) CreateService(c *gin.Context, systemID generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:create") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "create")
	if !ok {
		return
	}

	system, err := s.client.System.Get(ctx, systemID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for service creation", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.ServiceCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.Service.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetSystemID(systemID) // ent edge setter
	if req.Description != "" {
		create = create.SetDescription(req.Description)
	}

	svc, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "SERVICE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create service",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "service.create", "service", svc.ID, actor,
			map[string]interface{}{"system_id": systemID}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "service.create"),
				zap.String("resource_id", svc.ID),
			)
		}
	}

	c.JSON(http.StatusCreated, serviceToAPI(svc, system))
}

// GetService handles GET /systems/{system_id}/services/{service_id}.
func (s *Server) GetService(c *gin.Context, systemID generated.SystemID, serviceID generated.ServiceID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemID, "view"); !ok {
		return
	}

	// Verify the service exists and belongs to the given system.
	svc, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceID),
			entservice.HasSystemWith(entsystem.IDEQ(systemID)),
		).
		WithSystem(func(q *ent.SystemQuery) {
			q.Where(entsystem.IDEQ(systemID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if svc.Edges.System == nil {
		logger.Error("service get missing loaded system edge", zap.String("service_id", serviceID), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, serviceToAPI(svc, svc.Edges.System))
}

// GetServiceWorkspaceContext handles GET /systems/{system_id}/services/{service_id}/context.
func (s *Server) GetServiceWorkspaceContext(c *gin.Context, systemID generated.SystemID, serviceID generated.ServiceID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "view")
	if !ok {
		return
	}

	svc, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceID),
			entservice.HasSystemWith(entsystem.IDEQ(systemID)),
		).
		WithSystem(func(q *ent.SystemQuery) {
			q.Where(entsystem.IDEQ(systemID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service for workspace context",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if svc.Edges.System == nil {
		logger.Error("service context missing loaded system edge", zap.String("service_id", serviceID), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	resp := generated.ServiceWorkspaceContext{
		Service:        serviceToAPI(svc, svc.Edges.System),
		VisibleVms:     []generated.VM{},
		RecentRequests: []generated.Ticket{},
		Summary: generated.ServiceWorkspaceSummary{
			VisibleVmCount:     0,
			RecentRequestCount: 0,
		},
	}

	if hasGlobalPermission(c, "vm:read") {
		serviceVMs, totalVMs, loadErr := s.loadServiceContextVMs(ctx, c, serviceID)
		if loadErr != nil {
			if isRequestContextCanceled(loadErr) {
				return
			}
			logger.Error("failed to load service workspace VMs",
				zap.Error(loadErr),
				zap.String("service_id", serviceID),
			)
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		resp.VisibleVms = serviceVMs
		resp.Summary.VisibleVmCount = totalVMs
	}

	requests, totalRequests, err := s.loadServiceContextRecentRequests(ctx, actor, serviceID)
	if err != nil {
		if isRequestContextCanceled(err) {
			return
		}
		logger.Error("failed to load service workspace requests",
			zap.Error(err),
			zap.String("service_id", serviceID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	resp.RecentRequests = requests
	resp.Summary.RecentRequestCount = totalRequests

	c.JSON(http.StatusOK, resp)
}

// UpdateService handles PATCH /systems/{system_id}/services/{service_id}.
// Stage 4.C: only description is mutable; name is immutable.
func (s *Server) UpdateService(c *gin.Context, systemID generated.SystemID, serviceID generated.ServiceID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:create") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "update")
	if !ok {
		return
	}

	var req generated.ServiceUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	existing, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceID),
			entservice.HasSystemWith(entsystem.IDEQ(systemID)),
		).
		WithSystem(func(q *ent.SystemQuery) {
			q.Where(entsystem.IDEQ(systemID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service for update",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if existing.Edges.System == nil {
		logger.Error("service update missing loaded system edge", zap.String("service_id", serviceID), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	_, err = s.client.Service.UpdateOneID(serviceID).
		SetDescription(req.Description).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update service",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("service_id", serviceID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "service.update", "service", serviceID, actor, map[string]interface{}{
			"system_id": systemID,
			"field":     "description",
			"old":       existing.Description,
			"new":       req.Description,
		})
	}

	existing.Description = req.Description
	c.JSON(http.StatusOK, serviceToAPI(existing, existing.Edges.System))
}

// DeleteService handles DELETE /systems/{system_id}/services/{service_id}.
// ADR-0015 §13: requires confirm=true query param.
// Cascade constraint: must have zero child VMs.
func (s *Server) DeleteService(c *gin.Context, systemID generated.SystemID, serviceID generated.ServiceID, params generated.DeleteServiceParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:delete") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemID, "delete")
	if !ok {
		return
	}

	// Verify the service exists and belongs to the given system.
	svc, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceID),
			entservice.HasSystemWith(entsystem.IDEQ(systemID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service for delete",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Confirmation gate (ADR-0015 §13): confirm=true required.
	if !params.Confirm {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "DELETE_CONFIRMATION_REQUIRED",
			Message: "confirm=true query parameter is required",
		})
		return
	}

	// Cascade constraint: must have zero child VMs.
	vmCount, err := svc.QueryVms().Count(ctx)
	if err != nil {
		logger.Error("failed to count service VMs",
			zap.Error(err),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if vmCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "SERVICE_HAS_VMS",
			Message: "cannot delete service with existing VMs; delete all VMs first",
			Params: map[string]interface{}{
				"vm_count": vmCount,
			},
		})
		return
	}

	activeCreateCount, err := s.countActiveCreateTicketsForServiceIDs(ctx, []string{serviceID})
	if err != nil {
		logger.Error("failed to count active create requests for service delete",
			zap.Error(err),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if activeCreateCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "SERVICE_HAS_ACTIVE_REQUESTS",
			Message: "cannot delete service while unfinished VM create requests still reference it",
			Params: map[string]interface{}{
				"active_request_count": activeCreateCount,
			},
		})
		return
	}

	// Hard delete.
	if err := s.client.Service.DeleteOneID(serviceID).Exec(ctx); err != nil {
		logger.Error("failed to delete service",
			zap.Error(err),
			zap.String("service_id", serviceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "service.delete", "service", serviceID, actor,
			map[string]interface{}{"system_id": systemID}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "service.delete"),
				zap.String("resource_id", serviceID),
			)
		}
	}

	c.Status(http.StatusNoContent)
}

// ---- Converters ----

func (s *Server) loadServiceContextVMs(
	ctx context.Context,
	c *gin.Context,
	serviceID string,
) ([]generated.VM, int, error) {
	query := s.client.VM.Query().Where(
		entvm.HasServiceWith(entservice.IDEQ(serviceID)),
		entvm.StatusNEQ(entvm.StatusDELETING),
	)

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		return nil, 0, err
	}
	if visibility.restricted {
		visibleNamespaces, listErr := s.listVisibleNamespaceNames(ctx, visibility)
		if listErr != nil {
			return nil, 0, listErr
		}
		if len(visibleNamespaces) == 0 {
			return []generated.VM{}, 0, nil
		}
		query = query.Where(entvm.NamespaceIn(visibleNamespaces...))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []generated.VM{}, 0, nil
	}

	vms, err := query.Clone().
		WithService(func(q *ent.ServiceQuery) {
			q.Where(entservice.IDEQ(serviceID))
		}).
		Order(ent.Desc(entvm.FieldCreatedAt)).
		Limit(serviceContextVMLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	vms = s.refreshVMLiveStates(ctx, vms)

	clusterIDs := make([]string, 0, len(vms))
	seenClusterIDs := make(map[string]struct{}, len(vms))
	for _, vm := range vms {
		if vm == nil || vm.ClusterID == "" {
			continue
		}
		if _, seen := seenClusterIDs[vm.ClusterID]; seen {
			continue
		}
		seenClusterIDs[vm.ClusterID] = struct{}{}
		clusterIDs = append(clusterIDs, vm.ClusterID)
	}
	clusterEnvMap, clusterNameMap, err := s.loadClusterPresentation(ctx, clusterIDs)
	if err != nil {
		logger.Warn("failed to load cluster presentation for service context", zap.Error(err), zap.String("service_id", serviceID))
	}

	items := make([]generated.VM, 0, len(vms))
	for _, vm := range vms {
		env := clusterEnvMap[vm.ClusterID]
		name := clusterNameMap[vm.ClusterID]
		items = append(items, vmToAPI(vm, env, name, nil))
	}

	return items, total, nil
}

func (s *Server) loadServiceContextRecentRequests(
	ctx context.Context,
	actor string,
	serviceID string,
) ([]generated.Ticket, int, error) {
	eventIDs, err := s.client.DomainEvent.Query().
		Where(
			domainevent.EventTypeEQ(string(domain.EventVMCreationRequested)),
			domainevent.AggregateIDEQ(serviceID),
		).
		Select(domainevent.FieldID).
		Strings(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(eventIDs) == 0 {
		return []generated.Ticket{}, 0, nil
	}

	query := s.client.Ticket.Query().
		Where(
			entticket.ParentTicketIDIsNil(),
			entticket.OperationTypeEQ(entticket.OperationTypeCREATE),
			entticket.RequesterEQ(actor),
			entticket.EventIDIn(eventIDs...),
		)

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []generated.Ticket{}, 0, nil
	}

	tickets, err := query.Clone().
		Order(ent.Desc(entticket.FieldCreatedAt)).
		Limit(serviceContextRequestLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	requestEventIDs := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		requestEventIDs = append(requestEventIDs, ticket.EventID)
	}

	events, err := s.client.DomainEvent.Query().
		Where(domainevent.IDIn(requestEventIDs...)).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	eventPayloadMap := make(map[string][]byte, len(events))
	eventByID := make(map[string]*ent.DomainEvent, len(events))
	for _, event := range events {
		eventByID[event.ID] = event
		eventPayloadMap[event.ID] = event.Payload
	}

	templateIDs, instanceSizeIDs := collectApprovalCatalogLookupIDs(eventPayloadMap)
	vmIDs := collectApprovalSummaryVMIDs(eventPayloadMap)
	vmByID, extraTemplateIDs, extraInstanceSizeIDs := s.loadApprovalVMContexts(ctx, vmIDs)
	templateIDSet := sliceToStringSet(templateIDs)
	for _, templateID := range extraTemplateIDs {
		templateIDSet[templateID] = struct{}{}
	}
	instanceSizeIDSet := sliceToStringSet(instanceSizeIDs)
	for _, instanceSizeID := range extraInstanceSizeIDs {
		instanceSizeIDSet[instanceSizeID] = struct{}{}
	}
	templateByID, instanceSizeByID := s.loadApprovalCatalogLookups(
		ctx,
		sortedStringSet(templateIDSet),
		sortedStringSet(instanceSizeIDSet),
	)
	serviceIDs := collectApprovalPrefillServiceIDs(eventPayloadMap)
	serviceByID := s.loadApprovalServiceLookups(ctx, serviceIDs)
	systemIDByServiceID := s.loadApprovalPrefillSystemByServiceID(ctx, eventPayloadMap)
	batchProjectionByID := s.loadApprovalBatchProjections(ctx, tickets, eventByID)

	createVMByTicketID := make(map[string]*ent.VM, len(tickets))
	vms, err := s.client.VM.Query().
		Where(entvm.TicketIDIn(ticketIDs(tickets)...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch VMs for service context requests", zap.Error(err), zap.String("service_id", serviceID))
	} else {
		for _, vm := range vms {
			if vm == nil || vm.TicketID == "" {
				continue
			}
			createVMByTicketID[vm.TicketID] = vm
		}
	}

	items := make([]generated.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		var payloadMap map[string]interface{}
		if raw := eventPayloadMap[ticket.EventID]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &payloadMap); err != nil {
				logger.Warn("failed to deserialize service context ticket payload",
					zap.Error(err),
					zap.String("event_id", ticket.EventID),
				)
			}
		}
		enrichApprovalPayload(payloadMap, templateByID, instanceSizeByID, batchProjectionByID[ticket.ID])
		items = append(items, ticketToAPI(
			ticket,
			payloadMap,
			s.loadVMProvisioning(ctx, createVMByTicketID[ticket.ID]),
			buildTicketSummary(
				ticket,
				payloadMap,
				templateByID,
				instanceSizeByID,
				serviceByID,
				vmByID,
			),
			buildApprovalRequestPrefill(payloadMap, systemIDByServiceID),
		))
	}

	return items, total, nil
}

func (s *Server) loadClusterPresentation(
	ctx context.Context,
	clusterIDs []string,
) (clusterEnvMap, clusterNameMap map[string]string, err error) {
	clusterEnvMap = make(map[string]string, len(clusterIDs))
	clusterNameMap = make(map[string]string, len(clusterIDs))
	if len(clusterIDs) == 0 {
		return clusterEnvMap, clusterNameMap, nil
	}

	clusters, err := s.client.Cluster.Query().
		Where(entcluster.IDIn(clusterIDs...)).
		Select(entcluster.FieldID, entcluster.FieldEnvironment, entcluster.FieldDisplayName, entcluster.FieldName).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, cl := range clusters {
		clusterEnvMap[cl.ID] = string(cl.Environment)
		clusterNameMap[cl.ID] = firstNonEmptyString(cl.DisplayName, cl.Name, cl.ID)
	}
	return clusterEnvMap, clusterNameMap, nil
}

func ticketIDs(tickets []*ent.Ticket) []string {
	out := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket == nil || ticket.ID == "" {
			continue
		}
		out = append(out, ticket.ID)
	}
	return out
}

func systemToAPI(sys *ent.System) generated.System {
	return generated.System{
		Id:          sys.ID,
		Name:        sys.Name,
		Description: sys.Description,
		CreatedAt:   sys.CreatedAt,
		CreatedBy:   sys.CreatedBy,
		UpdatedAt:   sys.UpdatedAt,
	}
}

func (s *Server) visibleSystemIDs(ctx context.Context, actor string) ([]string, error) {
	bindings, err := s.client.ResourceRoleBinding.Query().
		Where(
			rrb.UserIDEQ(actor),
			rrb.ResourceTypeEQ("system"),
		).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while querying system bindings", zap.Error(err), zap.String("actor", actor))
			return nil, err
		}
		logger.Error("failed to query system bindings", zap.Error(err), zap.String("actor", actor))
		return nil, err
	}

	systemIDs := make([]string, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if _, ok := seen[b.ResourceID]; ok {
			continue
		}
		seen[b.ResourceID] = struct{}{}
		systemIDs = append(systemIDs, b.ResourceID)
	}

	return systemIDs, nil
}

// serviceToAPI converts ent Service to generated Service.
func serviceToAPI(svc *ent.Service, system *ent.System) generated.Service {
	return generated.Service{
		Id:                svc.ID,
		Name:              svc.Name,
		Description:       svc.Description,
		SystemId:          system.ID,
		SystemName:        system.Name,
		NextInstanceIndex: svc.NextInstanceIndex,
		CreatedAt:         svc.CreatedAt,
	}
}
