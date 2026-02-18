package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	rrb "kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
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
		bindings, err := s.client.ResourceRoleBinding.Query().
			Where(
				rrb.UserIDEQ(actor),
				rrb.ResourceTypeEQ("system"),
			).
			All(ctx)
		if err != nil {
			logger.Error("failed to query system bindings", zap.Error(err), zap.String("actor", actor))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if len(bindings) == 0 {
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

		systemIDs := make([]string, 0, len(bindings))
		seen := make(map[string]struct{}, len(bindings))
		for _, b := range bindings {
			if _, ok := seen[b.ResourceID]; ok {
				continue
			}
			seen[b.ResourceID] = struct{}{}
			systemIDs = append(systemIDs, b.ResourceID)
		}
		query = query.Where(entsystem.IDIn(systemIDs...))
	}

	// Pagination.
	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
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
func (s *Server) GetSystem(c *gin.Context, systemId generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemId, "view"); !ok {
		return
	}

	sys, err := s.client.System.Get(ctx, systemId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, systemToAPI(sys))
}

// UpdateSystem handles PATCH /systems/{system_id}.
// Stage 4.C: only description is mutable; name is immutable.
func (s *Server) UpdateSystem(c *gin.Context, systemId generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:write") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "update")
	if !ok {
		return
	}

	var req generated.SystemUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	existing, err := s.client.System.Get(ctx, systemId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for update", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	updated, err := s.client.System.UpdateOneID(systemId).
		SetDescription(req.Description).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update system",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.update", "system", systemId, actor, map[string]interface{}{
			"field": "description",
			"old":   existing.Description,
			"new":   req.Description,
		})
	}

	c.JSON(http.StatusOK, systemToAPI(updated))
}

// DeleteSystem handles DELETE /systems/{system_id}.
// ADR-0015 §13 addendum: confirm_name query param required.
func (s *Server) DeleteSystem(c *gin.Context, systemId generated.SystemID, params generated.DeleteSystemParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:delete") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "delete")
	if !ok {
		return
	}

	// Check for child services via edge.
	count, err := s.client.System.Query().
		Where(entsystem.IDEQ(systemId)).
		QueryServices().
		Count(ctx)
	if err != nil {
		logger.Error("failed to count system services", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, generated.Error{Code: "SYSTEM_HAS_SERVICES"})
		return
	}

	sys, err := s.client.System.Get(ctx, systemId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for delete", zap.Error(err), zap.String("system_id", systemId))
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

	if err := s.client.System.DeleteOneID(systemId).Exec(ctx); err != nil {
		logger.Error("failed to delete system", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "system.delete", "system", systemId, actor, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "system.delete"),
				zap.String("resource_id", systemId),
			)
		}
	}

	c.Status(http.StatusNoContent)
}

// ListServices handles GET /systems/{system_id}/services.
func (s *Server) ListServices(c *gin.Context, systemId generated.SystemID, params generated.ListServicesParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemId, "view"); !ok {
		return
	}

	// Query services via system edge.
	query := s.client.System.Query().
		Where(entsystem.IDEQ(systemId)).
		QueryServices()

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count services", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	services, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entservice.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list services", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Service, 0, len(services))
	for _, svc := range services {
		items = append(items, serviceToAPI(svc, systemId))
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
func (s *Server) CreateService(c *gin.Context, systemId generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:create") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "create")
	if !ok {
		return
	}

	// Verify system exists.
	_, err := s.client.System.Get(ctx, systemId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get system for service creation", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.ServiceCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.Service.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetSystemID(systemId) // ent edge setter
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
			zap.String("system_id", systemId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "service.create", "service", svc.ID, actor,
			map[string]interface{}{"system_id": systemId}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "service.create"),
				zap.String("resource_id", svc.ID),
			)
		}
	}

	c.JSON(http.StatusCreated, serviceToAPI(svc, systemId))
}

// GetService handles GET /systems/{system_id}/services/{service_id}.
func (s *Server) GetService(c *gin.Context, systemId generated.SystemID, serviceId generated.ServiceID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemId, "view"); !ok {
		return
	}

	// Verify the service exists and belongs to the given system.
	svc, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceId),
			entservice.HasSystemWith(entsystem.IDEQ(systemId)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("service_id", serviceId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, serviceToAPI(svc, systemId))
}

// UpdateService handles PATCH /systems/{system_id}/services/{service_id}.
// Stage 4.C: only description is mutable; name is immutable.
func (s *Server) UpdateService(c *gin.Context, systemId generated.SystemID, serviceId generated.ServiceID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:create") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "update")
	if !ok {
		return
	}

	var req generated.ServiceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	existing, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceId),
			entservice.HasSystemWith(entsystem.IDEQ(systemId)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service for update",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("service_id", serviceId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	updated, err := s.client.Service.UpdateOneID(serviceId).
		SetDescription(req.Description).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update service",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("service_id", serviceId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "service.update", "service", serviceId, actor, map[string]interface{}{
			"system_id": systemId,
			"field":     "description",
			"old":       existing.Description,
			"new":       req.Description,
		})
	}

	c.JSON(http.StatusOK, serviceToAPI(updated, systemId))
}

// DeleteService handles DELETE /systems/{system_id}/services/{service_id}.
// ADR-0015 §13: requires confirm=true query param.
// Cascade constraint: must have zero child VMs.
func (s *Server) DeleteService(c *gin.Context, systemId generated.SystemID, serviceId generated.ServiceID, params generated.DeleteServiceParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "service:delete") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "delete")
	if !ok {
		return
	}

	// Verify the service exists and belongs to the given system.
	svc, err := s.client.Service.Query().
		Where(
			entservice.IDEQ(serviceId),
			entservice.HasSystemWith(entsystem.IDEQ(systemId)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get service for delete",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("service_id", serviceId),
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
			zap.String("service_id", serviceId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if vmCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "SERVICE_HAS_VMS",
			Message: "cannot delete service with existing VMs; delete all VMs first",
		})
		return
	}

	// Hard delete.
	if err := s.client.Service.DeleteOneID(serviceId).Exec(ctx); err != nil {
		logger.Error("failed to delete service",
			zap.Error(err),
			zap.String("service_id", serviceId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "service.delete", "service", serviceId, actor,
			map[string]interface{}{"system_id": systemId}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "service.delete"),
				zap.String("resource_id", serviceId),
			)
		}
	}

	c.Status(http.StatusNoContent)
}

// ---- Converters ----

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

// serviceToAPI converts ent Service to generated Service.
// systemId is passed because Service stores FK in unexported field.
func serviceToAPI(svc *ent.Service, systemId string) generated.Service {
	return generated.Service{
		Id:                svc.ID,
		Name:              svc.Name,
		Description:       svc.Description,
		SystemId:          systemId,
		NextInstanceIndex: svc.NextInstanceIndex,
		CreatedAt:         svc.CreatedAt,
	}
}
