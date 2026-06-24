package handlers

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	authSessionModeHeader          = "X-Shepherd-Session-Mode"
	authSessionModeCookieOnlyValue = "cookie_only"
)

func (s *Server) currentAuthSessionVersion(ctx context.Context, userID string) (int64, error) {
	if s == nil || s.authSessions == nil {
		return 1, nil
	}
	version, err := s.authSessions.CurrentSessionVersion(ctx, userID)
	if err != nil {
		return 0, err
	}
	if version < 1 {
		return 1, nil
	}
	return version, nil
}

func (s *Server) revokeUserSessions(ctx context.Context, userID, reason string) error {
	if s == nil || s.authSessions == nil {
		return nil
	}
	return s.authSessions.RevokeUserSessions(ctx, userID, reason)
}

func (s *Server) revokeUsersSessions(ctx context.Context, userIDs []string, reason string) error {
	if s == nil || s.authSessions == nil {
		return nil
	}
	return s.authSessions.RevokeUsersSessions(ctx, userIDs, reason)
}

func userIDsForRoleWithClient(ctx context.Context, client *ent.Client, roleID string) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("server client is required")
	}

	users, err := client.User.Query().
		Where(entuser.HasRoleBindingsWith(rolebinding.HasRoleWith(role.IDEQ(roleID)))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(users))
	for _, userRow := range users {
		userID := strings.TrimSpace(userRow.ID)
		if userID == "" {
			continue
		}
		userIDs = append(userIDs, userID)
	}
	slices.Sort(userIDs)
	return slices.Compact(userIDs), nil
}

func userIDsForAuthProviderWithClient(ctx context.Context, client *ent.Client, providerID string) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("server client is required")
	}

	users, err := client.User.Query().
		Where(entuser.AuthProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(users))
	for _, userRow := range users {
		userID := strings.TrimSpace(userRow.ID)
		if userID == "" {
			continue
		}
		userIDs = append(userIDs, userID)
	}
	slices.Sort(userIDs)
	return slices.Compact(userIDs), nil
}

func (s *Server) revokeCurrentAuthToken(c *gin.Context, reason string) error {
	if s == nil || s.authSessions == nil {
		return nil
	}
	if c == nil {
		return fmt.Errorf("gin context is required")
	}

	tokenID := strings.TrimSpace(c.GetString("token_id"))
	if tokenID == "" {
		return service.ErrAuthSessionTokenIDMissing
	}
	userID := strings.TrimSpace(c.GetString("user_id"))
	if userID == "" {
		return service.ErrAuthSessionUserIDMissing
	}

	rawExpiresAt, ok := c.Get("token_expires_at")
	if !ok {
		return fmt.Errorf("token expiration is required for revocation")
	}
	expiresAt, ok := rawExpiresAt.(time.Time)
	if !ok || expiresAt.IsZero() {
		return fmt.Errorf("token expiration is invalid for revocation")
	}
	return s.authSessions.RevokeToken(c.Request.Context(), tokenID, userID, expiresAt, reason)
}

func loginResponseForClient(c *gin.Context, resp generated.LoginResponse) generated.LoginResponse {
	if !authSessionCookieOnlyMode(c) {
		return resp
	}
	resp.Token = ""
	return resp
}

func authSessionCookieOnlyMode(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader(authSessionModeHeader)), authSessionModeCookieOnlyValue)
}
