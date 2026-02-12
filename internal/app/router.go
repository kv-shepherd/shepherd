package app

import (
	"strings"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

// Public routes that do NOT require JWT authentication.
var publicPrefixes = []string{
	"/api/v1/auth/login",
	"/api/v1/health/",
}

// adminPrefixes are routes that require platform:admin role.
var adminPrefixes = []string{
	"/api/v1/admin/",
	"/api/v1/audit-logs",
}

func newRouter(server generated.ServerInterface, jwtCfg middleware.JWTConfig) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.ErrorHandler())
	router.Use(jwtSkipPublic(jwtCfg))
	router.Use(rbacAdminRoutes())

	generated.RegisterHandlersWithOptions(router, server, generated.GinServerOptions{
		BaseURL: "/api/v1",
	})
	return router
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

// rbacAdminRoutes returns middleware enforcing platform:admin on admin endpoints.
func rbacAdminRoutes() gin.HandlerFunc {
	adminMw := middleware.RequirePermission("platform:admin")
	return func(c *gin.Context) {
		for _, prefix := range adminPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				adminMw(c)
				return
			}
		}
		c.Next()
	}
}
