package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entuserpreference "kv-shepherd.io/shepherd/ent/userpreference"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

var userPreferenceKeyPattern = regexp.MustCompile(`^[a-z0-9._-]{1,120}$`)

func normalizeUserPreferenceKey(raw string) (string, bool) {
	key := strings.TrimSpace(raw)
	if !userPreferenceKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

func userPreferenceToAPI(pref *ent.UserPreference) generated.UserPreference {
	return generated.UserPreference{
		Key:       pref.Key,
		Value:     pref.Value,
		UpdatedAt: pref.UpdatedAt,
	}
}

// GetCurrentUserPreference handles GET /auth/preferences/{key}.
func (s *Server) GetCurrentUserPreference(c *gin.Context, key string) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	normalizedKey, ok := normalizeUserPreferenceKey(key)
	if !ok {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_PREFERENCE_KEY"})
		return
	}

	pref, err := s.client.UserPreference.Query().
		Where(
			entuserpreference.UserIDEQ(userID),
			entuserpreference.KeyEQ(normalizedKey),
		).
		Only(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_PREFERENCE_NOT_FOUND"})
			return
		}
		logger.Error("failed to load current user preference", zap.Error(err), zap.String("user_id", userID), zap.String("key", normalizedKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, userPreferenceToAPI(pref))
}

// UpdateCurrentUserPreference handles PUT /auth/preferences/{key}.
func (s *Server) UpdateCurrentUserPreference(c *gin.Context, key string) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	normalizedKey, ok := normalizeUserPreferenceKey(key)
	if !ok {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_PREFERENCE_KEY"})
		return
	}

	var req generated.UserPreferenceUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	if req.Value == nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "value is required"})
		return
	}

	existing, err := s.client.UserPreference.Query().
		Where(
			entuserpreference.UserIDEQ(userID),
			entuserpreference.KeyEQ(normalizedKey),
		).
		Only(c.Request.Context())
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to query current user preference before save", zap.Error(err), zap.String("user_id", userID), zap.String("key", normalizedKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var pref *ent.UserPreference
	if ent.IsNotFound(err) {
		pref, err = s.client.UserPreference.Create().
			SetID(uuid.Must(uuid.NewV7()).String()).
			SetUserID(userID).
			SetKey(normalizedKey).
			SetValue(req.Value).
			Save(c.Request.Context())
	} else {
		pref, err = existing.Update().
			SetValue(req.Value).
			Save(c.Request.Context())
	}
	if err != nil {
		logger.Error("failed to save current user preference", zap.Error(err), zap.String("user_id", userID), zap.String("key", normalizedKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(c.Request.Context(), "user.preference.update", "user", userID, userID, map[string]interface{}{
			"key": normalizedKey,
		})
	}

	c.JSON(http.StatusOK, userPreferenceToAPI(pref))
}

// DeleteCurrentUserPreference handles DELETE /auth/preferences/{key}.
func (s *Server) DeleteCurrentUserPreference(c *gin.Context, key string) {
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	normalizedKey, ok := normalizeUserPreferenceKey(key)
	if !ok {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_PREFERENCE_KEY"})
		return
	}

	if _, err := s.client.UserPreference.Delete().
		Where(
			entuserpreference.UserIDEQ(userID),
			entuserpreference.KeyEQ(normalizedKey),
		).
		Exec(c.Request.Context()); err != nil {
		logger.Error("failed to delete current user preference", zap.Error(err), zap.String("user_id", userID), zap.String("key", normalizedKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(c.Request.Context(), "user.preference.delete", "user", userID, userID, map[string]interface{}{
			"key": normalizedKey,
		})
	}

	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}
