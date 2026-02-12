package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"

	"kv-shepherd.io/shepherd/ent"
)

const passwordHashCost = 12

// Login handles POST /auth/login (Stage 1.5).
func (s *Server) Login(c *gin.Context) {
	var req generated.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	user, err := s.client.User.Query().
		Where(entuser.UsernameEQ(req.Username)).
		Where(entuser.EnabledEQ(true)).
		Only(c.Request.Context())
	if err != nil {
		logger.Warn("login failed: invalid credentials")
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		logger.Warn("login failed: invalid credentials")
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}

	roles, permissions, err := s.loadUserRolesAndPermissions(c.Request.Context(), user.ID)
	if err != nil {
		logger.Error("failed to load roles", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}

	token, expiresAt, err := middleware.GenerateToken(s.jwtCfg, user.ID, user.Username, roleNames, permissions)
	if err != nil {
		logger.Error("failed to generate token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	now := time.Now()
	if err := s.client.User.UpdateOneID(user.ID).SetLastLoginAt(now).Exec(c.Request.Context()); err != nil {
		logger.Warn("failed to update last_login_at", zap.Error(err), zap.String("user_id", user.ID))
	}

	if s.audit != nil {
		if err := s.audit.LogAction(c.Request.Context(), "user.login", "user", user.ID, user.ID, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "user.login"),
				zap.String("user_id", user.ID),
			)
		}
	}

	c.JSON(http.StatusOK, generated.LoginResponse{
		Token:               token,
		ExpiresAt:           expiresAt,
		ForcePasswordChange: user.ForcePasswordChange,
	})
}

// GetCurrentUser handles GET /auth/me.
func (s *Server) GetCurrentUser(c *gin.Context) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	user, err := s.client.User.Get(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
		return
	}

	roles, _, err := s.loadUserRolesAndPermissions(c.Request.Context(), user.ID)
	if err != nil {
		logger.Error("failed to load roles for current user", zap.Error(err), zap.String("user_id", user.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}

	c.JSON(http.StatusOK, generated.UserInfo{
		Id:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       roleNames,
	})
}

// ChangePassword handles POST /auth/change-password (Stage 1.5 forced password change).
func (s *Server) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	user, err := s.client.User.Get(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CURRENT_PASSWORD"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), passwordHashCost)
	if err != nil {
		logger.Error("failed to hash new password", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	err = s.client.User.UpdateOneID(userID).
		SetPasswordHash(string(hash)).
		SetForcePasswordChange(false).
		Exec(c.Request.Context())
	if err != nil {
		logger.Error("failed to update password", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(c.Request.Context(), "user.password_change", "user", userID, userID,
			map[string]interface{}{"reason": "user_initiated"}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "user.password_change"),
				zap.String("user_id", userID),
			)
		}
	}

	c.Status(http.StatusNoContent)
}

// loadUserRolesAndPermissions fetches roles and flattened permissions for a user.
func (s *Server) loadUserRolesAndPermissions(ctx context.Context, userID string) ([]*ent.Role, []string, error) {
	user, err := s.client.User.Query().
		Where(entuser.IDEQ(userID)).
		WithRoleBindings(func(q *ent.RoleBindingQuery) {
			q.WithRole()
		}).
		Only(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("query user with roles: %w", err)
	}

	var roles []*ent.Role
	permSet := make(map[string]struct{})
	for _, rb := range user.Edges.RoleBindings {
		if rb.Edges.Role != nil {
			role := rb.Edges.Role
			roles = append(roles, role)
			for _, p := range role.Permissions {
				permSet[p] = struct{}{}
			}
		}
	}

	permissions := make([]string, 0, len(permSet))
	for p := range permSet {
		permissions = append(permissions, p)
	}

	return roles, permissions, nil
}

// HashPassword hashes a password using bcrypt (used by seed command).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// GenerateUserID creates a new user ID.
func GenerateUserID() string {
	id, _ := uuid.NewV7()
	return id.String()
}
