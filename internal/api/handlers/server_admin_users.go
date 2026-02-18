package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type userCreateRequest struct {
	Username            string  `json:"username" binding:"required"`
	Password            string  `json:"password" binding:"required"`
	Email               *string `json:"email"`
	DisplayName         *string `json:"display_name"`
	Enabled             *bool   `json:"enabled"`
	ForcePasswordChange *bool   `json:"force_password_change"`
}

type userUpdateRequest struct {
	Email               *string `json:"email"`
	DisplayName         *string `json:"display_name"`
	Enabled             *bool   `json:"enabled"`
	Password            *string `json:"password"`
	ForcePasswordChange *bool   `json:"force_password_change"`
}

type userRoleBindingCreateRequest struct {
	RoleId              string   `json:"role_id" binding:"required"`
	ScopeType           *string  `json:"scope_type"`
	ScopeId             *string  `json:"scope_id"`
	AllowedEnvironments []string `json:"allowed_environments"`
}

// CreateUser handles POST /admin/users.
func (s *Server) CreateUser(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "username and password are required"})
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		logger.Error("failed to hash user password", zap.Error(err), zap.String("username", username))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	create := s.client.User.Create().
		SetID(GenerateUserID()).
		SetUsername(username).
		SetPasswordHash(hash)
	if req.Email != nil {
		if v := strings.TrimSpace(*req.Email); v != "" {
			create = create.SetEmail(v)
		}
	}
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v != "" {
			create = create.SetDisplayName(v)
		}
	}
	if req.Enabled != nil {
		create = create.SetEnabled(*req.Enabled)
	}
	if req.ForcePasswordChange != nil {
		create = create.SetForcePasswordChange(*req.ForcePasswordChange)
	} else {
		create = create.SetForcePasswordChange(true)
	}

	userEnt, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "USER_NAME_OR_EMAIL_EXISTS"})
			return
		}
		logger.Error("failed to create local user", zap.Error(err), zap.String("username", username))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "user.create", "user", userEnt.ID, actor, map[string]interface{}{
			"username": userEnt.Username,
		})
	}

	c.JSON(http.StatusCreated, userToAPI(userEnt, nil))
}

// UpdateUser handles PATCH /admin/users/{user_id}.
func (s *Server) UpdateUser(c *gin.Context, userId generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	existing, err := s.client.User.Get(ctx, userId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	update := existing.Update()
	if req.Email != nil {
		if v := strings.TrimSpace(*req.Email); v == "" {
			update = update.ClearEmail()
		} else {
			update = update.SetEmail(v)
		}
	}
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v == "" {
			update = update.ClearDisplayName()
		} else {
			update = update.SetDisplayName(v)
		}
	}
	if req.Enabled != nil {
		update = update.SetEnabled(*req.Enabled)
	}
	if req.Password != nil {
		password := strings.TrimSpace(*req.Password)
		if password == "" {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "password cannot be empty"})
			return
		}
		hash, err := HashPassword(password)
		if err != nil {
			logger.Error("failed to hash updated password", zap.Error(err), zap.String("user_id", userId))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		update = update.SetPasswordHash(hash)
		if req.ForcePasswordChange == nil {
			update = update.SetForcePasswordChange(true)
		}
	}
	if req.ForcePasswordChange != nil {
		update = update.SetForcePasswordChange(*req.ForcePasswordChange)
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "USER_NAME_OR_EMAIL_EXISTS"})
			return
		}
		logger.Error("failed to update user", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roles, err := s.loadRoleNamesForUser(ctx, userId)
	if err != nil {
		logger.Error("failed to load role names for updated user", zap.Error(err), zap.String("user_id", userId))
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "user.update", "user", updated.ID, actor, nil)
	}

	c.JSON(http.StatusOK, userToAPI(updated, roles))
}

// DeleteUser handles DELETE /admin/users/{user_id}.
func (s *Server) DeleteUser(c *gin.Context, userId generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	if userId == actor {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN", Message: "cannot delete current user"})
		return
	}

	if _, err := s.client.User.Get(ctx, userId); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for delete", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := s.client.RoleBinding.Delete().Where(rolebinding.HasUserWith(entuser.IDEQ(userId))).Exec(ctx); err != nil {
		logger.Error("failed to delete role bindings for user", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := s.client.ResourceRoleBinding.Delete().Where(resourcerolebinding.UserIDEQ(userId)).Exec(ctx); err != nil {
		logger.Error("failed to delete resource role bindings for user", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if err := s.client.User.DeleteOneID(userId).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete user", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "user.delete", "user", userId, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// ListUserRoleBindings handles GET /admin/users/{user_id}/role-bindings.
func (s *Server) ListUserRoleBindings(c *gin.Context, userId generated.UserID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	if _, err := s.client.User.Get(ctx, userId); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for role bindings", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	bindings, err := s.client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(entuser.IDEQ(userId))).
		WithRole().
		Order(ent.Desc(rolebinding.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list user role bindings", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.GlobalRoleBinding, 0, len(bindings))
	for _, binding := range bindings {
		roleName := ""
		roleID := ""
		if binding.Edges.Role != nil {
			roleName = binding.Edges.Role.Name
			roleID = binding.Edges.Role.ID
		}
		items = append(items, roleBindingToAPI(binding, userId, roleID, roleName))
	}

	c.JSON(http.StatusOK, generated.GlobalRoleBindingList{Items: items})
}

// CreateUserRoleBinding handles POST /admin/users/{user_id}/role-bindings.
func (s *Server) CreateUserRoleBinding(c *gin.Context, userId generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	if _, err := s.client.User.Get(ctx, userId); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for role binding create", zap.Error(err), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req userRoleBindingCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	roleID := strings.TrimSpace(req.RoleId)
	if roleID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "role_id is required"})
		return
	}
	roleEnt, err := s.client.Role.Get(ctx, roleID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role for role binding create", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	scopeType := "global"
	if req.ScopeType != nil {
		if v := strings.TrimSpace(*req.ScopeType); v != "" {
			scopeType = v
		}
	}
	scopeID := ""
	if req.ScopeId != nil {
		scopeID = strings.TrimSpace(*req.ScopeId)
	}

	allowedEnvs, err := normalizeAllowedEnvironments(req.AllowedEnvironments)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}

	dupQuery := s.client.RoleBinding.Query().Where(
		rolebinding.HasUserWith(entuser.IDEQ(userId)),
		rolebinding.HasRoleWith(role.IDEQ(roleID)),
		rolebinding.ScopeTypeEQ(scopeType),
	)
	if scopeID == "" {
		dupQuery = dupQuery.Where(rolebinding.ScopeIDIsNil())
	} else {
		dupQuery = dupQuery.Where(rolebinding.ScopeIDEQ(scopeID))
	}
	exists, err := dupQuery.Exist(ctx)
	if err != nil {
		logger.Error("failed to check duplicate role binding", zap.Error(err), zap.String("user_id", userId), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, generated.Error{Code: "ROLE_BINDING_EXISTS"})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.RoleBinding.Create().
		SetID(id.String()).
		SetUserID(userId).
		SetRoleID(roleID).
		SetScopeType(scopeType).
		SetCreatedBy(actor)
	if scopeID != "" {
		create = create.SetScopeID(scopeID)
	}
	if len(allowedEnvs) > 0 {
		create = create.SetAllowedEnvironments(allowedEnvs)
	}

	binding, err := create.Save(ctx)
	if err != nil {
		logger.Error("failed to create role binding", zap.Error(err), zap.String("user_id", userId), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.binding.create", "user", userId, actor, map[string]interface{}{
			"role_id":    roleID,
			"scope_type": scopeType,
			"scope_id":   scopeID,
		})
	}

	c.JSON(http.StatusCreated, roleBindingToAPI(binding, userId, roleEnt.ID, roleEnt.Name))
}

// DeleteUserRoleBinding handles DELETE /admin/users/{user_id}/role-bindings/{binding_id}.
func (s *Server) DeleteUserRoleBinding(c *gin.Context, userId generated.UserID, bindingId generated.RoleBindingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	binding, err := s.client.RoleBinding.Query().
		Where(
			rolebinding.IDEQ(bindingId),
			rolebinding.HasUserWith(entuser.IDEQ(userId)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_BINDING_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role binding for delete", zap.Error(err), zap.String("binding_id", bindingId), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := s.client.RoleBinding.DeleteOneID(binding.ID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_BINDING_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete role binding", zap.Error(err), zap.String("binding_id", bindingId), zap.String("user_id", userId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.binding.delete", "user", userId, actor, map[string]interface{}{
			"binding_id": bindingId,
		})
	}

	c.Status(http.StatusNoContent)
}

func loadRoleNames(bindings []*ent.RoleBinding) []string {
	set := make(map[string]struct{})
	for _, rb := range bindings {
		if rb == nil || rb.Edges.Role == nil {
			continue
		}
		set[rb.Edges.Role.Name] = struct{}{}
	}
	roles := make([]string, 0, len(set))
	for r := range set {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	return roles
}

func (s *Server) loadRoleNamesForUser(ctx context.Context, userID string) ([]string, error) {
	u, err := s.client.User.Query().
		Where(entuser.IDEQ(userID)).
		WithRoleBindings(func(q *ent.RoleBindingQuery) { q.WithRole() }).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return loadRoleNames(u.Edges.RoleBindings), nil
}

func userToAPI(u *ent.User, roles []string) generated.User {
	return generated.User{
		Id:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Enabled:     u.Enabled,
		Roles:       roles,
		CreatedAt:   u.CreatedAt,
	}
}

func roleBindingToAPI(binding *ent.RoleBinding, userID, roleID, roleName string) generated.GlobalRoleBinding {
	allowed := make([]generated.GlobalRoleBindingAllowedEnvironments, 0, len(binding.AllowedEnvironments))
	for _, env := range binding.AllowedEnvironments {
		allowed = append(allowed, generated.GlobalRoleBindingAllowedEnvironments(env))
	}
	return generated.GlobalRoleBinding{
		Id:                  binding.ID,
		UserId:              userID,
		RoleId:              roleID,
		RoleName:            roleName,
		ScopeType:           binding.ScopeType,
		ScopeId:             binding.ScopeID,
		AllowedEnvironments: allowed,
		CreatedBy:           binding.CreatedBy,
		CreatedAt:           binding.CreatedAt,
	}
}

func normalizeAllowedEnvironments(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	set := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, env := range raw {
		v := strings.TrimSpace(strings.ToLower(env))
		if v == "" {
			continue
		}
		if v != "test" && v != "prod" {
			return nil, fmt.Errorf("allowed_environments must be test/prod")
		}
		if _, exists := set[v]; exists {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}
