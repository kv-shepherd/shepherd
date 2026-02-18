package app

import (
	"slices"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
)

// Public routes that do NOT require JWT authentication.
var publicPrefixes = []string{
	"/api/v1/auth/login",
	"/api/v1/health/",
}

func newRouter(cfg *config.Config, server generated.ServerInterface, jwtCfg middleware.JWTConfig) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.ErrorHandler())

	router.Use(cors.New(buildCORSConfig(cfg)))

	router.Use(jwtSkipPublic(jwtCfg))
	router.Use(middleware.MustOpenAPIValidator("/api/v1"))

	generated.RegisterHandlersWithOptions(router, server, generated.GinServerOptions{
		BaseURL: "/api/v1",
	})
	return router
}

func buildCORSConfig(cfg *config.Config) cors.Config {
	allowAllOrigins := cfg.Server.UnsafeAllowAllOrigins
	allowedOrigins := sanitizeAllowedOrigins(cfg.Server.AllowedOrigins)

	corsCfg := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Request-ID"},
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

// jwtSkipPublic returns middleware that applies JWT auth only on non-public routes.
func jwtSkipPublic(jwtCfg middleware.JWTConfig) gin.HandlerFunc {
	jwtMw := middleware.JWTAuthWithConfig(jwtCfg)
	return func(c *gin.Context) {
		for _, prefix := range publicPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}
		jwtMw(c)
	}
}
