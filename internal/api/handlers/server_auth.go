package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
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
	if !bindAndValidateJSON(c, &req) {
		return
	}
	if !s.enforceLoginRateLimit(c, req.Username) {
		return
	}

	user, err := s.client.User.Query().
		Where(entuser.UsernameEQ(req.Username)).
		Where(entuser.EnabledEQ(true)).
		Only(c.Request.Context())
	if err != nil {
		logger.Warn("login failed: invalid credentials")
		s.recordLoginFailure(c, req.Username)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}

	if compareErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); compareErr != nil {
		logger.Warn("login failed: invalid credentials")
		s.recordLoginFailure(c, req.Username)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}

	loginResp, err := s.issueLoginResponse(c.Request.Context(), user)
	if err != nil {
		logger.Error("failed to issue login response", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	s.recordLoginSuccess(c, req.Username)
	s.setAuthSessionCookie(c, loginResp.Token, loginResp.ExpiresAt)
	clientResp := loginResponseForClient(c, loginResp)

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

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, clientResp)
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

	userInfo, err := s.buildUserInfo(c.Request.Context(), user)
	if err != nil {
		logger.Error("failed to load roles for current user", zap.Error(err), zap.String("user_id", user.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, userInfo)
}

// ChangePassword handles POST /auth/change-password (Stage 1.5 forced password change).
func (s *Server) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}
	ctx := c.Request.Context()

	var req generated.ChangePasswordRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	user, err := s.client.User.Get(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
		return
	}

	if compareErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); compareErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CURRENT_PASSWORD"})
		return
	}

	if validationErr := s.validatePassword(req.NewPassword, user.Username, user.Email, user.DisplayName); validationErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: validationErr.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), passwordHashCost)
	if err != nil {
		logger.Error("failed to hash new password", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if err := tx.Client().User.UpdateOneID(userID).
			SetPasswordHash(string(hash)).
			SetForcePasswordChange(false).
			Exec(ctx); err != nil {
			return err
		}
		return s.revokeUserSessions(ctx, userID, "password_changed")
	}); err != nil {
		logger.Error("failed to update password", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	s.clearAuthSessionCookie(c)

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "user.password_change", "user", userID, userID,
			map[string]interface{}{"reason": "user_initiated"}); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "user.password_change"),
				zap.String("user_id", userID),
			)
		}
	}

	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

// Logout handles POST /auth/logout.
func (s *Server) Logout(c *gin.Context) {
	if err := s.revokeCurrentAuthToken(c, "logout"); err != nil {
		logger.Error("failed to revoke current auth token on logout", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	s.clearAuthSessionCookie(c)
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
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
		if rb.Edges.Role == nil || !rb.Edges.Role.Enabled {
			continue
		}
		role := rb.Edges.Role
		roles = append(roles, role)
		for _, p := range role.Permissions {
			permSet[p] = struct{}{}
		}
	}

	permissions := make([]string, 0, len(permSet))
	for p := range permSet {
		permissions = append(permissions, p)
	}
	sort.Strings(permissions)

	return roles, permissions, nil
}

func (s *Server) buildUserInfo(ctx context.Context, user *ent.User) (generated.UserInfo, error) {
	if s == nil || user == nil {
		return generated.UserInfo{}, fmt.Errorf("server and user are required")
	}

	roles, permissions, err := s.loadUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return generated.UserInfo{}, err
	}

	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	return generated.UserInfo{
		Id:                  user.ID,
		Username:            user.Username,
		Email:               user.Email,
		DisplayName:         user.DisplayName,
		Roles:               roleNames,
		Permissions:         permissions,
		ForcePasswordChange: user.ForcePasswordChange,
	}, nil
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

func (s *Server) validatePassword(password string, identityHints ...string) error {
	return s.passwordPolicy.ValidatePassword(password, identityHints...)
}

func (s *Server) issueLoginResponse(ctx context.Context, user *ent.User) (generated.LoginResponse, error) {
	if s == nil || user == nil {
		return generated.LoginResponse{}, fmt.Errorf("server and user are required")
	}

	roles, permissions, err := s.loadUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return generated.LoginResponse{}, err
	}

	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	sessionVersion, err := s.currentAuthSessionVersion(ctx, user.ID)
	if err != nil {
		return generated.LoginResponse{}, err
	}

	token, expiresAt, err := middleware.GenerateTokenWithSessionVersion(
		s.jwtCfg,
		user.ID,
		user.Username,
		roleNames,
		permissions,
		sessionVersion,
	)
	if err != nil {
		return generated.LoginResponse{}, err
	}
	return generated.LoginResponse{
		Token:               token,
		ExpiresAt:           expiresAt,
		ForcePasswordChange: user.ForcePasswordChange,
	}, nil
}
