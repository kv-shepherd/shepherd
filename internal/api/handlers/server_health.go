package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

// GetLiveness handles GET /health/live — Kubernetes liveness probe.
func (s *Server) GetLiveness(c *gin.Context) {
	c.JSON(http.StatusOK, generated.Health{
		Status: generated.HealthStatusOk,
	})
}

// GetReadiness handles GET /health/ready — Kubernetes readiness probe.
func (s *Server) GetReadiness(c *gin.Context) {
	checks := make(map[string]string)
	allHealthy := true

	// Database check.
	if err := s.pool.Ping(c.Request.Context()); err != nil {
		checks["database"] = "error"
		allHealthy = false
	} else {
		checks["database"] = "ok"
	}

	status := generated.HealthStatusOk
	httpStatus := http.StatusOK
	if !allHealthy {
		status = generated.HealthStatusDegraded
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, generated.Health{
		Status: status,
		Checks: checks,
	})
}
