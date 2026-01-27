// Package handlers provides HTTP request handlers.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/handler
package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
)

// WorkerStatus is an interface for checking worker health.
type WorkerStatus interface {
	IsHealthy() bool
	LastHeartbeat() time.Time
}

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	client           *ent.Client
	pool             *pgxpool.Pool
	riverWorker      WorkerStatus   // Injected in Phase 4
	resourceWatchers []WorkerStatus // One per cluster
}

// NewHealthHandler creates a new health check handler.
// pool is used for database ping (more reliable than Ent query).
func NewHealthHandler(client *ent.Client, pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{
		client: client,
		pool:   pool,
	}
}

// SetRiverWorker sets the River Worker reference (called in Phase 4).
func (h *HealthHandler) SetRiverWorker(w WorkerStatus) {
	h.riverWorker = w
}

// AddResourceWatcher adds a ResourceWatcher reference (called in Phase 2).
func (h *HealthHandler) AddResourceWatcher(w WorkerStatus) {
	h.resourceWatchers = append(h.resourceWatchers, w)
}

// Live is the liveness probe - checks if process is responsive.
// Kubernetes uses this to determine if pod should be restarted.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// Ready is the readiness probe - checks if dependencies are ready.
// Kubernetes uses this to determine if pod should receive traffic.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx := c.Request.Context()

	checks := make(map[string]interface{})
	allHealthy := true

	// ========== Database Check ==========
	if err := h.pool.Ping(ctx); err != nil {
		checks["database"] = map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		}
		allHealthy = false
	} else {
		checks["database"] = map[string]interface{}{
			"status": "ok",
		}
	}

	// ========== River Worker Check ==========
	if h.riverWorker != nil {
		workerHealthy := h.riverWorker.IsHealthy()
		lastHeartbeat := h.riverWorker.LastHeartbeat()
		heartbeatAge := time.Since(lastHeartbeat)

		// Heartbeat > 60s is considered unhealthy
		if heartbeatAge > 60*time.Second {
			workerHealthy = false
		}

		checks["river_worker"] = map[string]interface{}{
			"status":           boolToStatus(workerHealthy),
			"last_heartbeat":   lastHeartbeat.Format(time.RFC3339),
			"heartbeat_age_ms": heartbeatAge.Milliseconds(),
		}

		if !workerHealthy {
			allHealthy = false
		}
	}

	// ========== Resource Watchers Check ==========
	if len(h.resourceWatchers) > 0 {
		watchersStatus := make([]map[string]interface{}, 0, len(h.resourceWatchers))
		watchersHealthy := true

		for i, watcher := range h.resourceWatchers {
			healthy := watcher.IsHealthy()
			lastHeartbeat := watcher.LastHeartbeat()
			heartbeatAge := time.Since(lastHeartbeat)

			// Heartbeat > 120s is considered unhealthy (watchers may need more time to reconnect)
			if heartbeatAge > 120*time.Second {
				healthy = false
			}

			watchersStatus = append(watchersStatus, map[string]interface{}{
				"index":            i,
				"status":           boolToStatus(healthy),
				"last_heartbeat":   lastHeartbeat.Format(time.RFC3339),
				"heartbeat_age_ms": heartbeatAge.Milliseconds(),
			})

			if !healthy {
				watchersHealthy = false
			}
		}

		checks["resource_watchers"] = map[string]interface{}{
			"status":   boolToStatus(watchersHealthy),
			"count":    len(h.resourceWatchers),
			"watchers": watchersStatus,
		}

		if !watchersHealthy {
			allHealthy = false
		}
	}

	status := http.StatusOK
	if !allHealthy {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{
		"status": boolToHealthStatus(allHealthy),
		"checks": checks,
	})
}

func boolToStatus(b bool) string {
	if b {
		return "ok"
	}
	return "error"
}

func boolToHealthStatus(b bool) string {
	if b {
		return "ok"
	}
	return "degraded"
}
