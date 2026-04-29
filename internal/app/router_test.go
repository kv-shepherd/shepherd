package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestSanitizeAllowedOrigins(t *testing.T) {
	got := sanitizeAllowedOrigins([]string{
		"  http://localhost:3000  ",
		"",
		"*",
		"http://localhost:3000",
		"https://example.com",
	})

	require.Equal(t, []string{
		"http://localhost:3000",
		"https://example.com",
	}, got)
}

func TestBuildCORSConfig_AllowAllForcesCredentialsOff(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			UnsafeAllowAllOrigins: true,
			AllowCredentials:      true,
		},
	}

	corsCfg := buildCORSConfig(cfg, nil)
	require.True(t, corsCfg.AllowAllOrigins)
	require.False(t, corsCfg.AllowCredentials)
}

func TestBuildCORSConfig_UsesDefaultOriginsWhenEmpty(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			UnsafeAllowAllOrigins: false,
			AllowedOrigins:        []string{"", "*", "   "},
			AllowCredentials:      true,
		},
	}

	corsCfg := buildCORSConfig(cfg, nil)
	require.False(t, corsCfg.AllowAllOrigins)
	require.Equal(t, []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
	}, corsCfg.AllowOrigins)
	require.True(t, corsCfg.AllowCredentials)
}

type stubCORSOriginChecker struct {
	allowed map[string]bool
}

func (s stubCORSOriginChecker) IsAllowedOrigin(_ context.Context, origin string) bool {
	return s.allowed[origin]
}

func TestBuildCORSConfig_UsesDynamicOriginCheckerWhenAvailable(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowCredentials: true,
		},
	}

	corsCfg := buildCORSConfig(cfg, stubCORSOriginChecker{
		allowed: map[string]bool{
			"https://console.example.com": true,
		},
	})

	require.NotNil(t, corsCfg.AllowOriginWithContextFunc)
	require.Empty(t, corsCfg.AllowOrigins)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/v1/auth/providers", http.NoBody)
	require.True(t, corsCfg.AllowOriginWithContextFunc(ctx, "https://console.example.com"))
	require.False(t, corsCfg.AllowOriginWithContextFunc(ctx, "https://denied.example.com"))
}

func TestBuildCORSConfig_AllowsExternalAuthCallbackAcrossOrigins(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AllowCredentials: true,
		},
	}

	corsCfg := buildCORSConfig(cfg, stubCORSOriginChecker{
		allowed: map[string]bool{},
	})

	require.NotNil(t, corsCfg.AllowOriginWithContextFunc)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/providers/provider-1/callback", http.NoBody)
	require.True(t, corsCfg.AllowOriginWithContextFunc(ctx, "https://test-kubevirt.example.com"))
}

func TestIsJWTOptionalPath_AllowsVMVNCBootstrap(t *testing.T) {
	require.True(t, isJWTOptionalPath("/api/v1/vms/vm-1/vnc"))
	require.True(t, isJWTOptionalPath("/api/v1/vms/vm-1/serial"))
	require.False(t, isJWTOptionalPath("/api/v1/vms/vm-1/console/status"))
}

func TestNewRouterAppliesMaxRequestBodyBytesBeforeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			MaxRequestBodyBytes: 4,
			AllowCredentials:    true,
		},
	}
	router := newRouter(cfg, nil, middleware.JWTConfig{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("12345"))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	require.Contains(t, rr.Body.String(), "REQUEST_TOO_LARGE")
}

func TestMapClusterHealthStatus(t *testing.T) {
	require.Equal(t, entcluster.StatusHEALTHY, mapClusterHealthStatus(provider.ClusterStatusHealthy))
	require.Equal(t, entcluster.StatusUNHEALTHY, mapClusterHealthStatus(provider.ClusterStatusUnhealthy))
	require.Equal(t, entcluster.StatusUNREACHABLE, mapClusterHealthStatus(provider.ClusterStatusUnreachable))
	require.Equal(t, entcluster.StatusUNKNOWN, mapClusterHealthStatus(provider.ClusterStatus("unexpected")))
}
