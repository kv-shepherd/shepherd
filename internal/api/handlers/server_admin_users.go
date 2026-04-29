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
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/service"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entuserpreference "kv-shepherd.io/shepherd/ent/userpreference"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type userCreateRequest struct {
	Username            string  `json:"username" binding:"required"`
	Password            string  `json:"password" binding:"required"` //nolint:gosec // API contract requires the canonical JSON field name.
	Email               *string `json:"email"`
	DisplayName         *string `json:"display_name"`
	Enabled             *bool   `json:"enabled"`
	ForcePasswordChange *bool   `json:"force_password_change"`
}

type userUpdateRequest struct {
	Email               *string `json:"email"`
	DisplayName         *string `json:"display_name"`
	Enabled             *bool   `json:"enabled"`
	Password            *string `json:"password"` //nolint:gosec // API contract requires the canonical JSON field name.
	ForcePasswordChange *bool   `json:"force_password_change"`
}

type userRoleBindingCreateRequest struct {
	RoleID              string   `json:"role_id" binding:"required"`
	ScopeType           *string  `json:"scope_type"`
	ScopeID             *string  `json:"scope_id"`
	AllowedEnvironments []string `json:"allowed_environments"`
}

// CreateUser handles POST /admin/users.
func (s *Server) CreateUser(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	var req userCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || strings.TrimSpace(password) == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "username and password are required"})
		return
	}
	if err := s.validatePassword(password, username, valueOrEmpty(req.Email), valueOrEmpty(req.DisplayName)); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
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
func (s *Server) UpdateUser(c *gin.Context, userID generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	var req userUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	existing, err := s.client.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	invalidateSessions := false
	emailValue := strings.TrimSpace(valueOrEmpty(req.Email))
	displayNameValue := strings.TrimSpace(valueOrEmpty(req.DisplayName))
	passwordHash := ""
	hasPasswordUpdate := false
	if req.Email != nil {
		emailValue = strings.TrimSpace(*req.Email)
	}
	if req.DisplayName != nil {
		displayNameValue = strings.TrimSpace(*req.DisplayName)
	}
	if req.Enabled != nil {
		invalidateSessions = invalidateSessions || existing.Enabled != *req.Enabled
	}
	if req.Password != nil {
		password := *req.Password
		if strings.TrimSpace(password) == "" {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "password cannot be empty"})
			return
		}
		emailHint := existing.Email
		if req.Email != nil {
			emailHint = strings.TrimSpace(*req.Email)
		}
		displayNameHint := existing.DisplayName
		if req.DisplayName != nil {
			displayNameHint = strings.TrimSpace(*req.DisplayName)
		}
		if validationErr := s.validatePassword(password, existing.Username, emailHint, displayNameHint); validationErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: validationErr.Error()})
			return
		}
		hash, hashErr := HashPassword(password)
		if hashErr != nil {
			logger.Error("failed to hash updated password", zap.Error(hashErr), zap.String("user_id", userID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		passwordHash = hash
		hasPasswordUpdate = true
		invalidateSessions = true
	}
	if req.ForcePasswordChange != nil {
		if !existing.ForcePasswordChange && *req.ForcePasswordChange {
			invalidateSessions = true
		}
	}

	var updated *ent.User
	if txErr := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		update := tx.Client().User.UpdateOneID(userID)
		if req.Email != nil {
			if emailValue == "" {
				update = update.ClearEmail()
			} else {
				update = update.SetEmail(emailValue)
			}
		}
		if req.DisplayName != nil {
			if displayNameValue == "" {
				update = update.ClearDisplayName()
			} else {
				update = update.SetDisplayName(displayNameValue)
			}
		}
		if req.Enabled != nil {
			update = update.SetEnabled(*req.Enabled)
		}
		if hasPasswordUpdate {
			update = update.SetPasswordHash(passwordHash)
			if req.ForcePasswordChange == nil {
				update = update.SetForcePasswordChange(true)
			}
		}
		if req.ForcePasswordChange != nil {
			update = update.SetForcePasswordChange(*req.ForcePasswordChange)
		}

		var saveErr error
		updated, saveErr = update.Save(ctx)
		if saveErr != nil {
			return saveErr
		}
		if invalidateSessions {
			return s.revokeUserSessions(ctx, userID, "user_updated")
		}
		return nil
	}); txErr != nil {
		if ent.IsConstraintError(txErr) {
			c.JSON(http.StatusConflict, generated.Error{Code: "USER_NAME_OR_EMAIL_EXISTS"})
			return
		}
		logger.Error("failed to update user", zap.Error(txErr), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roles, err := s.loadRoleNamesForUser(ctx, userID)
	if err != nil {
		logger.Error("failed to load role names for updated user", zap.Error(err), zap.String("user_id", userID))
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "user.update", "user", updated.ID, actor, nil)
	}

	c.JSON(http.StatusOK, userToAPI(updated, roles))
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// DeleteUser handles DELETE /admin/users/{user_id}.
func (s *Server) DeleteUser(c *gin.Context, userID generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage")
	if !ok {
		return
	}

	if userID == actor {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN", Message: "cannot delete current user"})
		return
	}

	if _, err := s.client.User.Get(ctx, userID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for delete", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := s.client.ExternalCohortGrant.Delete().Where(externalcohortgrant.UserIDEQ(userID)).Exec(ctx); err != nil {
		logger.Error("failed to delete external cohort grants for user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := s.client.RoleBinding.Delete().Where(rolebinding.HasUserWith(entuser.IDEQ(userID))).Exec(ctx); err != nil {
		logger.Error("failed to delete role bindings for user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := s.client.ResourceRoleBinding.Delete().Where(resourcerolebinding.UserIDEQ(userID)).Exec(ctx); err != nil {
		logger.Error("failed to delete resource role bindings for user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := s.client.UserPreference.Delete().Where(entuserpreference.UserIDEQ(userID)).Exec(ctx); err != nil {
		logger.Error("failed to delete user preferences for user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if err := s.client.User.DeleteOneID(userID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete user", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "user.delete", "user", userID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// ListUserRoleBindings handles GET /admin/users/{user_id}/role-bindings.
func (s *Server) ListUserRoleBindings(c *gin.Context, userID generated.UserID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	if _, err := s.client.User.Get(ctx, userID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for role bindings", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	bindings, err := s.client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(entuser.IDEQ(userID))).
		WithRole().
		Order(ent.Desc(rolebinding.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list user role bindings", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.GlobalRoleBinding, 0, len(bindings))
	for _, binding := range bindings {
		roleName := ""
		roleDisplayName := ""
		roleID := ""
		if binding.Edges.Role != nil {
			roleName = binding.Edges.Role.Name
			roleDisplayName = strings.TrimSpace(binding.Edges.Role.DisplayName)
			roleID = binding.Edges.Role.ID
		}
		items = append(items, s.roleBindingToAPI(ctx, binding, userID, roleID, roleName, roleDisplayName))
	}

	c.JSON(http.StatusOK, generated.GlobalRoleBindingList{Items: items})
}

// CreateUserRoleBinding handles POST /admin/users/{user_id}/role-bindings.
func (s *Server) CreateUserRoleBinding(c *gin.Context, userID generated.UserID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	if _, err := s.client.User.Get(ctx, userID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for role binding create", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req userRoleBindingCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	roleID := strings.TrimSpace(req.RoleID)
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
	if req.ScopeID != nil {
		scopeID = strings.TrimSpace(*req.ScopeID)
	}

	allowedEnvs, err := normalizeAllowedEnvironments(req.AllowedEnvironments)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}

	dupQuery := s.client.RoleBinding.Query().Where(
		rolebinding.HasUserWith(entuser.IDEQ(userID)),
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
		logger.Error("failed to check duplicate role binding", zap.Error(err), zap.String("user_id", userID), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, generated.Error{Code: "ROLE_BINDING_EXISTS"})
		return
	}

	id, _ := uuid.NewV7()
	var binding *ent.RoleBinding
	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		create := tx.Client().RoleBinding.Create().
			SetID(id.String()).
			SetUserID(userID).
			SetRoleID(roleID).
			SetScopeType(scopeType).
			SetCreatedBy(actor)
		if scopeID != "" {
			create = create.SetScopeID(scopeID)
		}
		if len(allowedEnvs) > 0 {
			create = create.SetAllowedEnvironments(allowedEnvs)
		}

		var saveErr error
		binding, saveErr = create.Save(ctx)
		if saveErr != nil {
			return saveErr
		}
		return s.revokeUserSessions(ctx, userID, "role_binding_created")
	}); err != nil {
		logger.Error("failed to create role binding", zap.Error(err), zap.String("user_id", userID), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.binding.create", "user", userID, actor, map[string]interface{}{
			"role_id":    roleID,
			"scope_type": scopeType,
			"scope_id":   scopeID,
		})
	}

	c.JSON(http.StatusCreated, s.roleBindingToAPI(
		ctx,
		binding,
		userID,
		roleEnt.ID,
		roleEnt.Name,
		strings.TrimSpace(roleEnt.DisplayName),
	))
}

// DeleteUserRoleBinding handles DELETE /admin/users/{user_id}/role-bindings/{binding_id}.
func (s *Server) DeleteUserRoleBinding(c *gin.Context, userID generated.UserID, bindingID generated.RoleBindingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	binding, err := s.client.RoleBinding.Query().
		Where(
			rolebinding.IDEQ(bindingID),
			rolebinding.HasUserWith(entuser.IDEQ(userID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_BINDING_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role binding for delete", zap.Error(err), zap.String("binding_id", bindingID), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if _, err := tx.Client().ExternalCohortGrant.Delete().Where(externalcohortgrant.RoleBindingIDEQ(binding.ID)).Exec(ctx); err != nil {
			return err
		}
		if err := tx.Client().RoleBinding.DeleteOneID(binding.ID).Exec(ctx); err != nil {
			return err
		}
		return s.revokeUserSessions(ctx, userID, "role_binding_deleted")
	}); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_BINDING_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete role binding", zap.Error(err), zap.String("binding_id", bindingID), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.binding.delete", "user", userID, actor, map[string]interface{}{
			"binding_id": bindingID,
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

func (s *Server) roleBindingToAPI(
	ctx context.Context,
	binding *ent.RoleBinding,
	userID, roleID, roleName, roleDisplayName string,
) generated.GlobalRoleBinding {
	allowed := make([]generated.GlobalRoleBindingAllowedEnvironments, 0, len(binding.AllowedEnvironments))
	for _, env := range binding.AllowedEnvironments {
		allowed = append(allowed, generated.GlobalRoleBindingAllowedEnvironments(env))
	}
	preferredRoleDisplay := strings.TrimSpace(roleDisplayName)
	if preferredRoleDisplay == "" {
		preferredRoleDisplay = roleName
	}
	managed, managedSource := s.roleBindingManagedState(ctx, binding.ID)
	return generated.GlobalRoleBinding{
		Id:                  binding.ID,
		UserId:              userID,
		RoleId:              roleID,
		RoleName:            roleName,
		RoleDisplayName:     preferredRoleDisplay,
		ScopeType:           binding.ScopeType,
		ScopeId:             binding.ScopeID,
		ScopeDisplayName:    s.resolveRoleBindingScopeDisplayName(ctx, binding),
		AllowedEnvironments: allowed,
		Managed:             managed,
		ManagedSource:       managedSource,
		CreatedBy:           binding.CreatedBy,
		CreatedAt:           binding.CreatedAt,
	}
}

func (s *Server) roleBindingManagedState(ctx context.Context, bindingID string) (managed bool, managedSource string) {
	_, err := s.client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.RoleBindingIDEQ(bindingID)).
		Only(ctx)
	if err == nil {
		return true, "external_cohort"
	}
	if ent.IsNotFound(err) {
		return false, ""
	}
	logger.Warn("failed to resolve managed role binding state", zap.Error(err), zap.String("binding_id", bindingID))
	return false, ""
}

func (s *Server) resolveRoleBindingScopeDisplayName(
	ctx context.Context,
	binding *ent.RoleBinding,
) string {
	scopeType := strings.TrimSpace(binding.ScopeType)
	scopeID := strings.TrimSpace(binding.ScopeID)
	if scopeID == "" {
		return ""
	}

	switch scopeType {
	case "system":
		systemEnt, err := s.client.System.Get(ctx, scopeID)
		if err == nil {
			return systemEnt.Name
		}
	case "service":
		serviceEnt, err := s.client.Service.Query().
			Where(service.IDEQ(scopeID)).
			WithSystem().
			Only(ctx)
		if err == nil {
			if serviceEnt.Edges.System != nil {
				return serviceEnt.Edges.System.Name + " / " + serviceEnt.Name
			}
			return serviceEnt.Name
		}
	case "vm":
		vmEnt, err := s.client.VM.Query().
			Where(entvm.IDEQ(scopeID)).
			WithService(func(q *ent.ServiceQuery) {
				q.WithSystem()
			}).
			Only(ctx)
		if err == nil {
			if vmEnt.Edges.Service != nil && vmEnt.Edges.Service.Edges.System != nil {
				return vmEnt.Edges.Service.Edges.System.Name + " / " + vmEnt.Edges.Service.Name + " / " + vmEnt.Name
			}
			if vmEnt.Edges.Service != nil {
				return vmEnt.Edges.Service.Name + " / " + vmEnt.Name
			}
			return vmEnt.Name
		}
	}

	return scopeID
}

func normalizeAllowedEnvironments(raw []string) ([]string, error) {
	const (
		envTest = "test"
		envProd = "prod"
	)

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
		if v != envTest && v != envProd {
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
