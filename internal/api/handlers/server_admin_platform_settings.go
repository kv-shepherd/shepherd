package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/platformsetting"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const platformSettingKeyExternalAuth = "external_auth"

func (s *Server) GetExternalAuthPlatformSettings(c *gin.Context) {
	_, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	c.JSON(http.StatusOK, s.externalAuthPlatformSettings())
}

func (s *Server) UpdateExternalAuthPlatformSettings(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:update", "auth_provider:manage")
	if !ok {
		return
	}

	var req generated.ExternalAuthPlatformSettingsUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	publicBaseURL := strings.TrimSpace(req.PublicBaseUrl)
	if publicBaseURL != "" {
		if err := validatePublicBaseURL(publicBaseURL); err != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
	}

	existing, err := s.client.PlatformSetting.Query().
		Where(platformsetting.KeyEQ(platformSettingKeyExternalAuth)).
		Only(ctx)
	switch {
	case err == nil:
		if publicBaseURL == "" {
			if delErr := s.client.PlatformSetting.DeleteOne(existing).Exec(ctx); delErr != nil {
				logger.Error("failed to clear external auth platform setting", zap.Error(delErr))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			s.storeExternalAuthPlatformBaseURL("")
		} else {
			if _, saveErr := existing.Update().
				SetValue(map[string]interface{}{"public_base_url": publicBaseURL}).
				SetUpdatedBy(actor).
				Save(ctx); saveErr != nil {
				logger.Error("failed to update external auth platform setting", zap.Error(saveErr))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			s.storeExternalAuthPlatformBaseURL(publicBaseURL)
		}
	case ent.IsNotFound(err):
		if publicBaseURL != "" {
			if _, createErr := s.client.PlatformSetting.Create().
				SetID("platform-setting-" + uuid.NewString()).
				SetKey(platformSettingKeyExternalAuth).
				SetValue(map[string]interface{}{"public_base_url": publicBaseURL}).
				SetUpdatedBy(actor).
				Save(ctx); createErr != nil {
				logger.Error("failed to create external auth platform setting", zap.Error(createErr))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			s.storeExternalAuthPlatformBaseURL(publicBaseURL)
		}
	default:
		logger.Error("failed to query external auth platform setting", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, s.externalAuthPlatformSettings())
}

func (s *Server) externalAuthPlatformSettings() generated.ExternalAuthPlatformSettings {
	configured, source := s.effectiveExternalAuthPublicBaseURL()
	settings := generated.ExternalAuthPlatformSettings{
		Source:            generated.ExternalAuthPlatformSettingsSource(source),
		RuntimeLoginReady: strings.TrimSpace(configured) != "",
	}
	if configured != "" {
		settings.EffectivePublicBaseUrl = configured
	}
	settings.PublicBaseUrl = s.externalAuthPlatformBaseURL()
	return settings
}

func (s *Server) effectiveExternalAuthPublicBaseURL() (publicBaseURL, source string) {
	if value := s.externalAuthPlatformBaseURL(); value != "" {
		return value, "platform_setting"
	}
	if value := strings.TrimSpace(s.publicBaseURL); value != "" {
		return value, "server_config"
	}
	return "", "unset"
}

func (s *Server) externalAuthPlatformBaseURL() string {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return strings.TrimSpace(s.externalAuthBaseURL)
}

func (s *Server) storeExternalAuthPlatformBaseURL(raw string) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	s.externalAuthBaseURL = strings.TrimSpace(raw)
}

func (s *Server) loadExternalAuthPlatformSetting(ctx context.Context) {
	if s == nil || s.client == nil {
		return
	}

	row, err := s.client.PlatformSetting.Query().
		Where(platformsetting.KeyEQ(platformSettingKeyExternalAuth)).
		Only(ctx)
	switch {
	case err == nil:
		s.storeExternalAuthPlatformBaseURL(platformSettingPublicBaseURL(row.Value))
	case ent.IsNotFound(err):
		s.storeExternalAuthPlatformBaseURL("")
	default:
		logger.Warn("failed to load external auth platform setting at startup", zap.Error(err))
	}
}

func platformSettingPublicBaseURL(raw map[string]interface{}) string {
	if raw == nil {
		return ""
	}
	value, _ := raw["public_base_url"].(string)
	return strings.TrimSpace(value)
}

func validatePublicBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("public_base_url must be an absolute http or https URL")
	}
	if parsed.Scheme != externalAuthSchemeHTTP && parsed.Scheme != externalAuthSchemeHTTPS {
		return errors.New("public_base_url must use http or https")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("public_base_url must not include a path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("public_base_url must not include query or fragment")
	}
	return nil
}
