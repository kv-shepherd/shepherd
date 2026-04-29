package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
)

type corsOriginChecker interface {
	IsAllowedOrigin(ctx context.Context, origin string) bool
}

// Public routes that do NOT require JWT authentication.
var publicPrefixes = []string{
	"/api/v1/auth/login",
	"/api/v1/auth/providers",
	"/api/v1/health/",
	// Schema metadata: does not contain sensitive data; must be accessible
	// before login for bootstrap tooling (ADR-0023). Matches OpenAPI security:[].
	"/api/v1/schemas/",
}

func isJWTOptionalPath(path string) bool {
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return strings.HasPrefix(path, "/api/v1/vms/") &&
		(strings.HasSuffix(path, "/vnc") || strings.HasSuffix(path, "/serial"))
}

var defaultTrustedLoopbackProxies = []string{
	"127.0.0.1/8",
	"::1/128",
}

func newRouter(cfg *config.Config, server generated.ServerInterface, jwtCfg middleware.JWTConfig) *gin.Engine {
	router := gin.New()
	configureTrustedProxies(router, cfg.Server.TrustedProxies)
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.ErrorHandler())
	router.Use(middleware.MaxRequestBodyBytes(cfg.Server.MaxRequestBodyBytes))

	var originChecker corsOriginChecker
	if checker, ok := server.(corsOriginChecker); ok && checker != nil {
		originChecker = checker
	}

	router.Use(cors.New(buildCORSConfig(cfg, originChecker)))

	router.Use(jwtSkipPublic(jwtCfg))
	router.Use(middleware.MustOpenAPIValidator("/api/v1"))

	generated.RegisterHandlersWithOptions(router, server, generated.GinServerOptions{
		BaseURL: "/api/v1",
	})
	return router
}

func buildCORSConfig(cfg *config.Config, checker corsOriginChecker) cors.Config {
	allowAllOrigins := cfg.Server.UnsafeAllowAllOrigins
	allowedOrigins := sanitizeAllowedOrigins(cfg.Server.AllowedOrigins)

	corsCfg := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Request-ID", "X-Shepherd-Session-Mode"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: cfg.Server.AllowCredentials,
		MaxAge:           12 * time.Hour,
	}

	if allowAllOrigins {
		corsCfg.AllowAllOrigins = true
		// gin-contrib/cors docs: AllowAllOrigins cannot be used with credentials.
		corsCfg.AllowCredentials = false
		return corsCfg
	}

	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}

	if checker != nil {
		corsCfg.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
			if isExternalAuthCallbackRequest(c) {
				return true
			}
			return checker.IsAllowedOrigin(c.Request.Context(), origin)
		}
		return corsCfg
	}

	corsCfg.AllowOrigins = allowedOrigins
	return corsCfg
}

func sanitizeAllowedOrigins(origins []string) []string {
	cleaned := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "*" {
			continue
		}
		cleaned = append(cleaned, origin)
	}
	return slices.Compact(cleaned)
}

func isExternalAuthCallbackRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	switch c.Request.Method {
	case http.MethodGet, http.MethodPost:
	default:
		return false
	}
	return isExternalAuthCallbackPath(c.Request.URL.Path)
}

func isExternalAuthCallbackPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/auth/providers/") && strings.HasSuffix(path, "/callback")
}

func configureTrustedProxies(router *gin.Engine, trustedProxies []string) {
	if router == nil {
		return
	}

	proxies := sanitizeAllowedOrigins(trustedProxies)
	if len(proxies) == 0 {
		proxies = defaultTrustedLoopbackProxies
	}
	err := router.SetTrustedProxies(proxies)
	if err != nil {
		panic("configure trusted proxies: " + err.Error())
	}
}

// jwtSkipPublic returns middleware that applies JWT auth only on non-public routes.
func jwtSkipPublic(jwtCfg middleware.JWTConfig) gin.HandlerFunc {
	jwtMw := middleware.JWTAuthWithConfig(jwtCfg)
	return func(c *gin.Context) {
		if isJWTOptionalPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		jwtMw(c)
	}
}
