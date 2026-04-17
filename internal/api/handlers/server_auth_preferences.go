package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/platformsetting"
	entuserpreference "kv-shepherd.io/shepherd/ent/userpreference"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

var userPreferenceKeyPattern = regexp.MustCompile(`^[a-z0-9._-]{1,120}$`)

const sharedUserDirectoryDisplayPreferenceKey = "admin.users.columns.v4"

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

func isPlatformSharedPreferenceKey(key string) bool {
	return key == sharedUserDirectoryDisplayPreferenceKey
}

func platformSettingPreferenceToAPI(key string, setting *ent.PlatformSetting) generated.UserPreference {
	return generated.UserPreference{
		Key:       key,
		Value:     setting.Value,
		UpdatedAt: setting.UpdatedAt,
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
	if isPlatformSharedPreferenceKey(normalizedKey) {
		s.getPlatformSharedPreference(c, normalizedKey, userID)
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
	if isPlatformSharedPreferenceKey(normalizedKey) {
		s.updatePlatformSharedPreference(c, normalizedKey, req)
		return
	}

	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
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
	normalizedKey, ok := normalizeUserPreferenceKey(key)
	if !ok {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_PREFERENCE_KEY"})
		return
	}
	if isPlatformSharedPreferenceKey(normalizedKey) {
		s.deletePlatformSharedPreference(c, normalizedKey)
		return
	}

	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
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

func (s *Server) getPlatformSharedPreference(c *gin.Context, key, userID string) {
	setting, err := s.client.PlatformSetting.Query().
		Where(platformsetting.KeyEQ(key)).
		Only(c.Request.Context())
	switch {
	case err == nil:
		c.JSON(http.StatusOK, platformSettingPreferenceToAPI(key, setting))
		return
	case !ent.IsNotFound(err):
		logger.Error("failed to load platform-shared preference", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Backward compatibility: if the shared platform setting has not been created yet,
	// fall back to the current user's historical preference so the first admin does not lose
	// an existing layout immediately after upgrade.
	pref, prefErr := s.client.UserPreference.Query().
		Where(
			entuserpreference.UserIDEQ(userID),
			entuserpreference.KeyEQ(key),
		).
		Only(c.Request.Context())
	if prefErr != nil {
		if ent.IsNotFound(prefErr) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_PREFERENCE_NOT_FOUND"})
			return
		}
		logger.Error("failed to load legacy user preference fallback", zap.Error(prefErr), zap.String("user_id", userID), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, userPreferenceToAPI(pref))
}

func (s *Server) updatePlatformSharedPreference(c *gin.Context, key string, req generated.UserPreferenceUpdateRequest) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage", "rbac:manage")
	if !ok {
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to open transaction for platform-shared preference update", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existing, err := tx.PlatformSetting.Query().
		Where(platformsetting.KeyEQ(key)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to query platform-shared preference before save", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var saved *ent.PlatformSetting
	if ent.IsNotFound(err) {
		saved, err = tx.PlatformSetting.Create().
			SetID("platform-setting-" + uuid.NewString()).
			SetKey(key).
			SetValue(req.Value).
			SetUpdatedBy(actor).
			Save(ctx)
	} else {
		saved, err = existing.Update().
			SetValue(req.Value).
			SetUpdatedBy(actor).
			Save(ctx)
	}
	if err != nil {
		logger.Error("failed to save platform-shared preference", zap.Error(err), zap.String("key", key), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := tx.UserPreference.Delete().
		Where(entuserpreference.KeyEQ(key)).
		Exec(ctx); err != nil {
		logger.Error("failed to clean legacy user preferences after shared preference save", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit platform-shared preference update", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "platform.setting.update", "platform_setting", key, actor, map[string]interface{}{
			"key": key,
		})
	}

	c.JSON(http.StatusOK, platformSettingPreferenceToAPI(key, saved))
}

func (s *Server) deletePlatformSharedPreference(c *gin.Context, key string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "user:manage", "rbac:manage")
	if !ok {
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to open transaction for platform-shared preference delete", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.PlatformSetting.Delete().
		Where(platformsetting.KeyEQ(key)).
		Exec(ctx); err != nil {
		logger.Error("failed to delete platform-shared preference", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := tx.UserPreference.Delete().
		Where(entuserpreference.KeyEQ(key)).
		Exec(ctx); err != nil {
		logger.Error("failed to delete legacy user preferences for shared preference", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit platform-shared preference delete", zap.Error(err), zap.String("key", key))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "platform.setting.delete", "platform_setting", key, actor, map[string]interface{}{
			"key": key,
		})
	}

	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}
