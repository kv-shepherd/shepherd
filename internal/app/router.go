package app

import (
	"context"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/observability"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type corsOriginChecker interface {
	IsAllowedOrigin(ctx context.Context, origin string) bool
}

type corsRequestOriginChecker interface {
	IsAllowedRequestOrigin(ctx context.Context, path, origin string) bool
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

var publicExactPaths = []string{
	"/api/v1/external-approval/pending",
	"/api/v1/webhooks/approval-callback",
}

func isJWTOptionalPath(path string) bool {
	for _, publicPath := range publicExactPaths {
		if path == publicPath {
			return true
		}
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return strings.HasPrefix(path, "/api/v1/vms/") &&
		(strings.HasSuffix(path, "/vnc") || strings.HasSuffix(path, "/serial")) ||
		isExternalApprovalDecisionPath(path)
}

func isExternalApprovalDecisionPath(path string) bool {
	const prefix = "/api/v1/external-approval/tickets/"
	const suffix = "/decision"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return false
	}
	ticketID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return strings.TrimSpace(ticketID) != "" && !strings.Contains(ticketID, "/")
}

var defaultTrustedLoopbackProxies = []string{
	"127.0.0.1/8",
	"::1/128",
}

func newRouter(cfg *config.Config, server generated.ServerInterface, jwtCfg middleware.JWTConfig, metrics *observability.Metrics, tracing *observability.Tracing) *gin.Engine {
	router := gin.New()
	configureTrustedProxies(router, cfg.Server.TrustedProxies)
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.ErrorHandler())
	if tracing != nil {
		router.Use(tracing.Middleware())
	}
	router.Use(observability.HTTPRequestLogMiddleware(observability.HTTPRequestLogOptions{
		Logger:      logger.LOrNop(),
		MetricsPath: cfg.Observability.EffectiveMetricsPath(),
	}))
	if metrics != nil {
		router.Use(metrics.Middleware())
		router.GET(cfg.Observability.EffectiveMetricsPath(), gin.WrapH(metrics.Handler()))
	}
	router.Use(middleware.MaxRequestBodyBytes(cfg.Server.MaxRequestBodyBytes))

	var requestOriginChecker corsRequestOriginChecker
	if checker, ok := server.(corsRequestOriginChecker); ok && checker != nil {
		requestOriginChecker = checker
	}

	var originChecker corsOriginChecker
	if checker, ok := server.(corsOriginChecker); ok && checker != nil {
		originChecker = checker
	}

	router.Use(cors.New(buildCORSConfig(cfg, requestOriginChecker, originChecker)))

	router.Use(jwtSkipPublic(jwtCfg))
	openAPIValidatorOptions := make([]middleware.OpenAPIValidatorOption, 0, 1)
	if metrics != nil {
		openAPIValidatorOptions = append(openAPIValidatorOptions, middleware.WithOpenAPIValidationMetrics(metrics.OpenAPIValidationRecorder()))
	}
	router.Use(middleware.MustOpenAPIValidator("/api/v1", openAPIValidatorOptions...))

	generated.RegisterHandlersWithOptions(router, server, generated.GinServerOptions{
		BaseURL: "/api/v1",
	})
	return router
}

func buildCORSConfig(cfg *config.Config, requestChecker corsRequestOriginChecker, checker corsOriginChecker) cors.Config {
	allowAllOrigins := cfg.Server.UnsafeAllowAllOrigins
	allowedOrigins := sanitizeAllowedOrigins(cfg.Server.AllowedOrigins)

	corsCfg := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Request-ID", "X-Shepherd-Session-Mode", "X-External-Approval-System-ID", "X-Shepherd-Timestamp", "X-Signature-256", "X-Ticket-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID", observability.TraceIDHeader},
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

	if requestChecker != nil {
		corsCfg.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
			if c == nil || c.Request == nil {
				return false
			}
			path := ""
			if c.Request.URL != nil {
				path = c.Request.URL.Path
			}
			return requestChecker.IsAllowedRequestOrigin(c.Request.Context(), path, origin)
		}
		return corsCfg
	}

	if checker != nil {
		corsCfg.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
			return checker.IsAllowedOrigin(c.Request.Context(), origin)
		}
		return corsCfg
	}

	corsCfg.AllowOrigins = allowedOrigins
	return corsCfg
}

func sanitizeAllowedOrigins(origins []string) []string {
	cleaned := make([]string, 0, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "*" {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		cleaned = append(cleaned, origin)
	}
	return cleaned
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
