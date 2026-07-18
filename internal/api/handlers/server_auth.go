package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"

	"kv-shepherd.io/shepherd/ent"
)

const (
	passwordHashCost                     = 12
	loginAuthorizationSnapshotMaxRetries = 3
)

var (
	errCurrentPasswordChanged       = errors.New("current password changed during request")
	errInvalidLocalLoginCredentials = errors.New("invalid local login credentials")
)

type passwordPolicyViolationError struct {
	cause error
}

func (e *passwordPolicyViolationError) Error() string {
	if e == nil || e.cause == nil {
		return "password does not satisfy the current policy"
	}
	return e.cause.Error()
}

func (e *passwordPolicyViolationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

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
		s.recordCredentialLoginFailure(c, req.Username, "invalid_credentials")
		s.recordLoginFailure(c, req.Username)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}

	loginResp, err := s.issueLocalLoginResponse(c.Request.Context(), user.ID, req.Password)
	if errors.Is(err, errInvalidLocalLoginCredentials) {
		s.recordCredentialLoginFailure(c, req.Username, "invalid_credentials")
		s.recordLoginFailure(c, req.Username)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_CREDENTIALS"})
		return
	}
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
		clientIP, requestID := loginAuditContext(c)
		if err := s.audit.LogAction(c.Request.Context(), "user.login", "user", user.ID, user.ID, map[string]interface{}{
			"username":   strings.TrimSpace(user.Username),
			"provider":   "local",
			"client_ip":  clientIP,
			"request_id": requestID,
		}); err != nil {
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

	hash, err := s.generatePasswordHash(req.NewPassword)
	if err != nil {
		logger.Error("failed to hash new password", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if err := s.ensureAuthSessionSchema(ctx); err != nil {
		logger.Error("failed to prepare auth session state for password change", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if lockErr := lockUserMutation(ctx, tx, userID); lockErr != nil {
			return lockErr
		}
		if lockErr := lockUserRow(ctx, tx, userID); lockErr != nil {
			return lockErr
		}
		currentUser, getErr := tx.Client().User.Get(ctx, userID)
		if getErr != nil {
			return getErr
		}
		if compareErr := bcrypt.CompareHashAndPassword([]byte(currentUser.PasswordHash), []byte(req.OldPassword)); compareErr != nil {
			return errCurrentPasswordChanged
		}
		if validationErr := s.validatePassword(
			req.NewPassword,
			currentUser.Username,
			currentUser.Email,
			currentUser.DisplayName,
		); validationErr != nil {
			return &passwordPolicyViolationError{cause: validationErr}
		}
		if err := tx.Client().User.UpdateOneID(userID).
			SetPasswordHash(hash).
			SetForcePasswordChange(false).
			Exec(ctx); err != nil {
			return err
		}
		return s.revokeUserSessionsTx(ctx, tx, userID, "password_changed")
	}); err != nil {
		if errors.Is(err, errCurrentPasswordChanged) {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CURRENT_PASSWORD"})
			return
		}
		var policyErr *passwordPolicyViolationError
		if errors.As(err, &policyErr) {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: policyErr.Error()})
			return
		}
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
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

// loadUserRolesAndPermissions fetches active role names and flattened permissions for a user.
func (s *Server) loadUserRolesAndPermissions(ctx context.Context, userID string) (roleNames, permissions []string, err error) {
	user, err := s.client.User.Query().
		Where(entuser.IDEQ(userID)).
		WithRoleBindings(func(q *ent.RoleBindingQuery) {
			q.WithRole()
		}).
		Only(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("query user with roles: %w", err)
	}

	roleSet := make(map[string]struct{})
	permSet := make(map[string]struct{})
	for _, rb := range user.Edges.RoleBindings {
		if rb.Edges.Role == nil || !rb.Edges.Role.Enabled {
			continue
		}
		role := rb.Edges.Role
		roleName := strings.TrimSpace(role.Name)
		if roleName != "" {
			roleSet[roleName] = struct{}{}
		}
		for _, p := range role.Permissions {
			key := strings.TrimSpace(p)
			if key == "" {
				continue
			}
			if _, supported := permissionCatalog[key]; !supported {
				continue
			}
			permSet[key] = struct{}{}
		}
	}

	roleNames = make([]string, 0, len(roleSet))
	for roleName := range roleSet {
		roleNames = append(roleNames, roleName)
	}
	sort.Strings(roleNames)

	permissions = make([]string, 0, len(permSet))
	for p := range permSet {
		permissions = append(permissions, p)
	}
	sort.Strings(permissions)

	return roleNames, permissions, nil
}

func (s *Server) buildUserInfo(ctx context.Context, user *ent.User) (generated.UserInfo, error) {
	if s == nil || user == nil {
		return generated.UserInfo{}, fmt.Errorf("server and user are required")
	}

	roleNames, permissions, err := s.loadUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return generated.UserInfo{}, err
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

func (s *Server) generatePasswordHash(password string) (string, error) {
	if s != nil && s.passwordHashGenerator != nil {
		return s.passwordHashGenerator(password)
	}
	return HashPassword(password)
}

// GenerateUserID creates a new user ID.
func GenerateUserID() string {
	id, _ := uuid.NewV7()
	return id.String()
}

func loginAuditContext(c *gin.Context) (clientIP, requestID string) {
	clientIP = loginAttemptClientIdentity(c)
	if c != nil && c.Request != nil {
		requestID = strings.TrimSpace(middleware.GetRequestID(c.Request.Context()))
	}
	return clientIP, requestID
}

func (s *Server) recordCredentialLoginFailure(c *gin.Context, username, reason string) {
	clientIP, requestID := loginAuditContext(c)
	logger.Warn(
		"login failed",
		zap.String("reason", strings.TrimSpace(reason)),
		zap.String("username", strings.TrimSpace(username)),
		zap.String("client_ip", clientIP),
		zap.String("provider", "local"),
		zap.String("request_id", requestID),
	)
	if s == nil || s.audit == nil || c == nil || c.Request == nil {
		return
	}
	_ = s.audit.LogAction(c.Request.Context(), "user.login_failed", "auth", strings.TrimSpace(username), "anonymous", map[string]interface{}{
		"username":   strings.TrimSpace(username),
		"client_ip":  clientIP,
		"provider":   "local",
		"reason":     strings.TrimSpace(reason),
		"request_id": requestID,
	})
}

func (s *Server) validatePassword(password string, identityHints ...string) error {
	return s.passwordPolicy.ValidatePassword(password, identityHints...)
}

func (s *Server) issueLocalLoginResponse(ctx context.Context, userID, password string) (generated.LoginResponse, error) {
	return s.issueLoginResponseWithUserValidation(ctx, userID, func(user *ent.User) error {
		if user == nil || !user.Enabled ||
			bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			return errInvalidLocalLoginCredentials
		}
		return nil
	})
}

func (s *Server) issueLoginResponseWithUserValidation(
	ctx context.Context,
	userID string,
	validateUser func(*ent.User) error,
) (generated.LoginResponse, error) {
	for attempt := 0; attempt < loginAuthorizationSnapshotMaxRetries; attempt++ {
		snapshot, err := s.loadLoginAuthorizationSnapshot(ctx, userID, validateUser)
		if err != nil {
			return generated.LoginResponse{}, err
		}
		loginResp, err := s.loginResponseFromAuthorizationSnapshot(snapshot)
		if err != nil {
			return generated.LoginResponse{}, err
		}
		if err := s.activateAuthSession(ctx, userID, snapshot.SessionVersion); err != nil {
			if errors.Is(err, service.ErrAuthSessionVersionChanged) {
				continue
			}
			return generated.LoginResponse{}, err
		}
		return loginResp, nil
	}
	return generated.LoginResponse{}, fmt.Errorf("authorization changed repeatedly while activating login session")
}

func (s *Server) loadLoginAuthorizationSnapshot(
	ctx context.Context,
	userID string,
	validateUser func(*ent.User) error,
) (*loginAuthorizationSnapshot, error) {
	if s == nil || strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("server and user id are required")
	}

	return loadStableLoginAuthorizationSnapshot(
		ctx,
		func(ctx context.Context) (int64, error) {
			return s.currentAuthSessionVersion(ctx, userID)
		},
		func(ctx context.Context) (*ent.User, []string, []string, error) {
			freshUser, loadErr := s.client.User.Get(ctx, userID)
			if loadErr != nil {
				if validateUser != nil && ent.IsNotFound(loadErr) {
					if validationErr := validateUser(nil); validationErr != nil {
						return nil, nil, nil, validationErr
					}
				}
				return nil, nil, nil, loadErr
			}
			if validateUser != nil {
				if validationErr := validateUser(freshUser); validationErr != nil {
					return nil, nil, nil, validationErr
				}
			}
			if !freshUser.Enabled {
				return nil, nil, nil, fmt.Errorf("user is disabled")
			}
			roleNames, permissions, loadErr := s.loadUserRolesAndPermissions(ctx, userID)
			return freshUser, roleNames, permissions, loadErr
		},
	)
}

func (s *Server) loginResponseFromAuthorizationSnapshot(
	snapshot *loginAuthorizationSnapshot,
) (generated.LoginResponse, error) {
	if s == nil || snapshot == nil || snapshot.User == nil {
		return generated.LoginResponse{}, fmt.Errorf("server and login authorization snapshot are required")
	}

	token, expiresAt, err := middleware.GenerateTokenWithSessionVersion(
		s.jwtCfg,
		snapshot.User.ID,
		snapshot.User.Username,
		snapshot.RoleNames,
		snapshot.Permissions,
		snapshot.SessionVersion,
	)
	if err != nil {
		return generated.LoginResponse{}, err
	}
	return generated.LoginResponse{
		Token:               token,
		ExpiresAt:           expiresAt,
		ForcePasswordChange: snapshot.User.ForcePasswordChange,
	}, nil
}

type loginAuthorizationSnapshot struct {
	User           *ent.User
	RoleNames      []string
	Permissions    []string
	SessionVersion int64
}

func loadStableLoginAuthorizationSnapshot(
	ctx context.Context,
	loadVersion func(context.Context) (int64, error),
	loadAuthorization func(context.Context) (*ent.User, []string, []string, error),
) (*loginAuthorizationSnapshot, error) {
	if loadVersion == nil || loadAuthorization == nil {
		return nil, fmt.Errorf("login authorization snapshot loaders are required")
	}
	for attempt := 0; attempt < loginAuthorizationSnapshotMaxRetries; attempt++ {
		beforeVersion, err := loadVersion(ctx)
		if err != nil {
			return nil, err
		}
		freshUser, roleNames, permissions, err := loadAuthorization(ctx)
		if err != nil {
			return nil, err
		}
		if freshUser == nil {
			return nil, fmt.Errorf("login authorization snapshot returned no user")
		}
		afterVersion, err := loadVersion(ctx)
		if err != nil {
			return nil, err
		}
		if beforeVersion == afterVersion {
			return &loginAuthorizationSnapshot{
				User:           freshUser,
				RoleNames:      roleNames,
				Permissions:    permissions,
				SessionVersion: afterVersion,
			}, nil
		}
	}
	return nil, fmt.Errorf("authorization changed repeatedly while issuing login token")
}
