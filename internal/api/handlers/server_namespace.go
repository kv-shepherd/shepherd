package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ListNamespaces handles GET /admin/namespaces.
func (s *Server) ListNamespaces(c *gin.Context, params generated.ListNamespacesParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "cluster:read", "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	query := s.client.NamespaceRegistry.Query()
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.restricted {
		if len(visibility.envs) == 0 {
			c.JSON(http.StatusOK, generated.NamespaceRegistryList{
				Items: []generated.NamespaceRegistry{},
				Pagination: generated.Pagination{
					Page:       1,
					PerPage:    20,
					Total:      0,
					TotalPages: 0,
				},
			})
			return
		}
		query = query.Where(namespaceregistry.EnvironmentIn(visibility.envs...))
	}

	// Filter by environment (omitzero: empty string = not specified).
	if params.Environment != "" {
		query = query.Where(namespaceregistry.EnvironmentEQ(
			namespaceregistry.Environment(params.Environment),
		))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count namespaces", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	namespaces, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(namespaceregistry.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list namespaces", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.NamespaceRegistry, 0, len(namespaces))
	for _, ns := range namespaces {
		items = append(items, namespaceToAPI(ns))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.NamespaceRegistryList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateNamespace handles POST /admin/namespaces.
func (s *Server) CreateNamespace(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	var req generated.NamespaceCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.NamespaceRegistry.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetEnvironment(namespaceregistry.Environment(req.Environment)).
		SetCreatedBy(actor)

	if req.Description != "" {
		create = create.SetDescription(req.Description)
	}

	ns, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "NAMESPACE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create namespace", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "namespace.create", "namespace", ns.ID, actor, map[string]interface{}{
			"name":        ns.Name,
			"environment": string(ns.Environment),
		})
	}

	c.JSON(http.StatusCreated, namespaceToAPI(ns))
}

// GetNamespace handles GET /admin/namespaces/{namespace_id}.
func (s *Server) GetNamespace(c *gin.Context, namespaceId generated.NamespaceID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "cluster:read", "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	ns, err := s.client.NamespaceRegistry.Get(ctx, namespaceId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "NAMESPACE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get namespace", zap.Error(err), zap.String("namespace_id", namespaceId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.restricted {
		visible := false
		for _, env := range visibility.envs {
			if ns.Environment == env {
				visible = true
				break
			}
		}
		if !visible {
			c.JSON(http.StatusNotFound, generated.Error{Code: "NAMESPACE_NOT_FOUND"})
			return
		}
	}

	c.JSON(http.StatusOK, namespaceToAPI(ns))
}

// UpdateNamespace handles PUT /admin/namespaces/{namespace_id}.
func (s *Server) UpdateNamespace(c *gin.Context, namespaceId generated.NamespaceID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	var req generated.NamespaceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	update := s.client.NamespaceRegistry.UpdateOneID(namespaceId)
	if req.Description != "" {
		update = update.SetDescription(req.Description)
	}
	update = update.SetEnabled(req.Enabled)

	ns, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "NAMESPACE_NOT_FOUND"})
			return
		}
		logger.Error("failed to update namespace", zap.Error(err), zap.String("namespace_id", namespaceId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "namespace.update", "namespace", ns.ID, actor, nil)
	}

	c.JSON(http.StatusOK, namespaceToAPI(ns))
}

// DeleteNamespace handles DELETE /admin/namespaces/{namespace_id}.
// ADR-0015 §13 addendum: confirm_name query param required.
func (s *Server) DeleteNamespace(c *gin.Context, namespaceId generated.NamespaceID, params generated.DeleteNamespaceParams) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	ns, err := s.client.NamespaceRegistry.Get(ctx, namespaceId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "NAMESPACE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get namespace for delete", zap.Error(err), zap.String("namespace_id", namespaceId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Confirmation gate: confirm_name must match namespace name.
	if params.ConfirmName == "" || params.ConfirmName != ns.Name {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "DELETE_CONFIRMATION_REQUIRED",
			Message: "confirm_name query parameter must match namespace name exactly",
		})
		return
	}

	// Prevent namespace deletion when live VM records still reference it.
	vmCount, err := s.client.VM.Query().Where(vm.NamespaceEQ(ns.Name)).Count(ctx)
	if err != nil {
		logger.Error("failed to check namespace usage", zap.Error(err), zap.String("namespace", ns.Name))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if vmCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "NAMESPACE_IN_USE",
			Message: "namespace is referenced by existing VM records",
		})
		return
	}

	if err := s.client.NamespaceRegistry.DeleteOneID(namespaceId).Exec(ctx); err != nil {
		logger.Error("failed to delete namespace", zap.Error(err), zap.String("namespace_id", namespaceId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "namespace.delete", "namespace", namespaceId, actor, map[string]interface{}{
			"name": ns.Name,
		})
	}

	c.Status(http.StatusNoContent)
}

// ---- Converter ----

func namespaceToAPI(ns *ent.NamespaceRegistry) generated.NamespaceRegistry {
	return generated.NamespaceRegistry{
		Id:          ns.ID,
		Name:        ns.Name,
		Environment: generated.NamespaceRegistryEnvironment(ns.Environment),
		Description: ns.Description,
		Enabled:     ns.Enabled,
		CreatedBy:   ns.CreatedBy,
		CreatedAt:   ns.CreatedAt,
		UpdatedAt:   ns.UpdatedAt,
	}
}
