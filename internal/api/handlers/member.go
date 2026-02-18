package handlers

import (
	"net/http"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ListUsers handles GET /admin/users.
func (s *Server) ListUsers(c *gin.Context, params generated.ListUsersParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "user:manage", "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	query := s.client.User.Query()
	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count users", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	users, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Asc(entuser.FieldUsername)).
		WithRoleBindings(func(q *ent.RoleBindingQuery) {
			q.WithRole()
		}).
		All(ctx)
	if err != nil {
		logger.Error("failed to list users", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.User, 0, len(users))
	for _, u := range users {
		roleSet := make(map[string]struct{})
		for _, rb := range u.Edges.RoleBindings {
			if rb == nil || rb.Edges.Role == nil {
				continue
			}
			roleSet[rb.Edges.Role.Name] = struct{}{}
		}

		roles := make([]string, 0, len(roleSet))
		for roleName := range roleSet {
			roles = append(roles, roleName)
		}
		sort.Strings(roles)

		items = append(items, generated.User{
			Id:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Enabled:     u.Enabled,
			Roles:       roles,
			CreatedAt:   u.CreatedAt,
		})
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.UserList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ListSystemMembers handles GET /systems/{system_id}/members.
func (s *Server) ListSystemMembers(c *gin.Context, systemId generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "system:read") {
		return
	}
	if _, ok := s.requireSystemRole(c, systemId, "view"); !ok {
		return
	}

	bindings, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemId),
		).
		Order(ent.Asc(resourcerolebinding.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list system members", zap.Error(err), zap.String("system_id", systemId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	userIDs := make([]string, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if _, ok := seen[b.UserID]; ok {
			continue
		}
		seen[b.UserID] = struct{}{}
		userIDs = append(userIDs, b.UserID)
	}

	userByID := make(map[string]*ent.User, len(userIDs))
	if len(userIDs) > 0 {
		users, err := s.client.User.Query().Where(entuser.IDIn(userIDs...)).All(ctx)
		if err != nil {
			logger.Error("failed to query users for members", zap.Error(err), zap.String("system_id", systemId))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		for _, u := range users {
			userByID[u.ID] = u
		}
	}

	items := make([]generated.SystemMember, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, toSystemMember(b, userByID[b.UserID]))
	}

	c.JSON(http.StatusOK, generated.SystemMemberList{Items: items})
}

// AddSystemMember handles POST /systems/{system_id}/members.
func (s *Server) AddSystemMember(c *gin.Context, systemId generated.SystemID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "manage_members")
	if !ok {
		return
	}

	var req generated.SystemMemberCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	role := string(req.Role)
	if !isValidMemberRole(role) {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_ROLE"})
		return
	}

	userEnt, err := s.client.User.Get(ctx, req.UserId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get user for member add", zap.Error(err), zap.String("user_id", req.UserId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	id, _ := uuid.NewV7()
	member, err := s.client.ResourceRoleBinding.Create().
		SetID(id.String()).
		SetUserID(req.UserId).
		SetResourceType("system").
		SetResourceID(systemId).
		SetRole(resourcerolebinding.Role(role)).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "MEMBER_ALREADY_EXISTS"})
			return
		}
		logger.Error("failed to add system member",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("user_id", req.UserId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.add", "system", systemId, actor, map[string]interface{}{
			"user_id": req.UserId,
			"role":    role,
		})
	}

	c.JSON(http.StatusCreated, toSystemMember(member, userEnt))
}

// UpdateSystemMemberRole handles PATCH /systems/{system_id}/members/{user_id}.
func (s *Server) UpdateSystemMemberRole(c *gin.Context, systemId generated.SystemID, userId generated.UserID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "manage_members")
	if !ok {
		return
	}

	var req generated.SystemMemberRoleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	role := string(req.Role)
	if !isValidMemberRole(role) {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_ROLE"})
		return
	}

	existing, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ(userId),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemId),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "MEMBER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query member for role update",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("user_id", userId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	updated, err := s.client.ResourceRoleBinding.UpdateOneID(existing.ID).
		SetRole(resourcerolebinding.Role(role)).
		Save(ctx)
	if err != nil {
		logger.Error("failed to update member role",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("user_id", userId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	userEnt, err := s.client.User.Get(ctx, userId)
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to get user after role update", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.update_role", "system", systemId, actor, map[string]interface{}{
			"user_id":  userId,
			"old_role": existing.Role.String(),
			"new_role": role,
		})
	}

	c.JSON(http.StatusOK, toSystemMember(updated, userEnt))
}

// DeleteSystemMember handles DELETE /systems/{system_id}/members/{user_id}.
func (s *Server) DeleteSystemMember(c *gin.Context, systemId generated.SystemID, userId generated.UserID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "rbac:manage") {
		return
	}
	actor, ok := s.requireSystemRole(c, systemId, "manage_members")
	if !ok {
		return
	}

	member, err := s.client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ(userId),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemId),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "MEMBER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query member for delete",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("user_id", userId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if member.Role == resourcerolebinding.RoleOwner {
		ownerCount, err := s.client.ResourceRoleBinding.Query().
			Where(
				resourcerolebinding.ResourceTypeEQ("system"),
				resourcerolebinding.ResourceIDEQ(systemId),
				resourcerolebinding.RoleEQ(resourcerolebinding.RoleOwner),
			).
			Count(ctx)
		if err != nil {
			logger.Error("failed to count system owners", zap.Error(err), zap.String("system_id", systemId))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if ownerCount <= 1 {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "LAST_OWNER_CANNOT_BE_REMOVED",
				Message: "system must have at least one owner",
			})
			return
		}
	}

	if err := s.client.ResourceRoleBinding.DeleteOneID(member.ID).Exec(ctx); err != nil {
		logger.Error("failed to delete member",
			zap.Error(err),
			zap.String("system_id", systemId),
			zap.String("user_id", userId),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "system.member.remove", "system", systemId, actor, map[string]interface{}{
			"user_id": userId,
			"role":    member.Role.String(),
		})
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) requireSystemRole(c *gin.Context, systemID, action string) (string, bool) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return "", false
	}

	if _, err := s.client.System.Get(ctx, systemID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "SYSTEM_NOT_FOUND"})
			return "", false
		}
		logger.Error("failed to get system for member operation", zap.Error(err), zap.String("system_id", systemID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return "", false
	}

	if hasPlatformAdmin(c) {
		return actor, true
	}

	checker := middleware.NewResourceRoleChecker(s.client)
	role, found, err := checker.CheckResourceRole(ctx, actor, "system", systemID)
	if err != nil {
		logger.Error("failed to check system role",
			zap.Error(err),
			zap.String("system_id", systemID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return "", false
	}
	if !found || !middleware.RoleCanPerform(role, action) {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return "", false
	}

	return actor, true
}

func hasPlatformAdmin(c *gin.Context) bool {
	perms, exists := c.Get("permissions")
	if !exists {
		return false
	}
	permList, ok := perms.([]string)
	if !ok {
		return false
	}
	return slices.Contains(permList, "platform:admin")
}

func isValidMemberRole(role string) bool {
	switch role {
	case resourcerolebinding.RoleOwner.String(),
		resourcerolebinding.RoleAdmin.String(),
		resourcerolebinding.RoleMember.String(),
		resourcerolebinding.RoleViewer.String():
		return true
	default:
		return false
	}
}

func toSystemMember(binding *ent.ResourceRoleBinding, user *ent.User) generated.SystemMember {
	member := generated.SystemMember{
		UserId:    binding.UserID,
		Username:  binding.UserID,
		Role:      generated.SystemMemberRole(binding.Role.String()),
		CreatedAt: binding.CreatedAt,
	}
	if user == nil {
		return member
	}
	member.Username = user.Username
	member.Email = user.Email
	member.DisplayName = user.DisplayName
	return member
}
