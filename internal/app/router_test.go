package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
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

func TestMapClusterHealthStatus(t *testing.T) {
	require.Equal(t, entcluster.StatusHEALTHY, mapClusterHealthStatus(provider.ClusterStatusHealthy))
	require.Equal(t, entcluster.StatusUNHEALTHY, mapClusterHealthStatus(provider.ClusterStatusUnhealthy))
	require.Equal(t, entcluster.StatusUNREACHABLE, mapClusterHealthStatus(provider.ClusterStatusUnreachable))
	require.Equal(t, entcluster.StatusUNKNOWN, mapClusterHealthStatus(provider.ClusterStatus("unexpected")))
}
