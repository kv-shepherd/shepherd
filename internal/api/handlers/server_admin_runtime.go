package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/edge/authworkspace/runtimeview"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// GetAuthProviderRuntimeDescriptor handles GET /admin/auth-providers/{provider_id}/runtime.
func (s *Server) GetAuthProviderRuntimeDescriptor(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	providerRow, err := s.client.AuthProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for runtime descriptor", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	runtimeDescriptor, err := runtimeview.BuildRuntimeDescriptor(providerRow)
	if err != nil {
		if errors.Is(err, runtimeview.ErrAuthProviderAdapterNotFound) {
			logger.Error("failed to resolve auth provider adapter for runtime descriptor", zap.String("provider_id", providerID), zap.String("auth_type", providerRow.AuthType))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		logger.Error("failed to build auth provider runtime descriptor", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, runtimeDescriptor)
}
