package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ClusterStatus represents cluster health status.
type ClusterStatus string

const (
	ClusterStatusUnknown     ClusterStatus = "UNKNOWN"
	ClusterStatusHealthy     ClusterStatus = "HEALTHY"
	ClusterStatusUnhealthy   ClusterStatus = "UNHEALTHY"
	ClusterStatusUnreachable ClusterStatus = "UNREACHABLE"
)

// ClusterHealth contains health check results.
type ClusterHealth struct {
	ClusterName     string        `json:"cluster_name"`
	Status          ClusterStatus `json:"status"`
	KubeVirtVersion string        `json:"kubevirt_version,omitempty"`
	LastChecked     time.Time     `json:"last_checked"`
	Error           string        `json:"error,omitempty"`
}

// ClusterHealthChecker performs periodic health checks on registered clusters.
type ClusterHealthChecker struct {
	clientFactory ClusterClientFactory
	interval      time.Duration
	results       map[string]*ClusterHealth
	mu            sync.RWMutex
	stopCh        chan struct{}
	stopOnce      sync.Once // ISSUE-010: prevent double-close panic
}

// NewClusterHealthChecker creates a new ClusterHealthChecker.
func NewClusterHealthChecker(clientFactory ClusterClientFactory, interval time.Duration) *ClusterHealthChecker {
	return &ClusterHealthChecker{
		clientFactory: clientFactory,
		interval:      interval,
		results:       make(map[string]*ClusterHealth),
		stopCh:        make(chan struct{}),
	}
}

// CheckCluster performs a single health check for a cluster.
func (c *ClusterHealthChecker) CheckCluster(ctx context.Context, clusterName string) *ClusterHealth {
	health := &ClusterHealth{
		ClusterName: clusterName,
		LastChecked: time.Now(),
	}

	client, err := c.clientFactory(clusterName)
	if err != nil {
		health.Status = ClusterStatusUnreachable
		health.Error = fmt.Sprintf("connection failed: %v", err)
		logger.Error("Cluster health check failed",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return health
	}

	// Verify API connectivity by listing VMs (lightweight check)
	_, err = client.VM().List(ctx, "default", defaultListOpts())
	if err != nil {
		health.Status = ClusterStatusUnhealthy
		health.Error = fmt.Sprintf("kubevirt api error: %v", err)
		return health
	}

	health.Status = ClusterStatusHealthy
	return health
}

// GetHealth returns the cached health status for a cluster.
func (c *ClusterHealthChecker) GetHealth(clusterName string) *ClusterHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if h, ok := c.results[clusterName]; ok {
		return h
	}
	return &ClusterHealth{
		ClusterName: clusterName,
		Status:      ClusterStatusUnknown,
	}
}

// UpdateHealth stores a health check result.
func (c *ClusterHealthChecker) UpdateHealth(health *ClusterHealth) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[health.ClusterName] = health
}

// Start begins periodic health checking for the given clusters.
// nolint:naked-goroutine // health checker ticker loop; doesn't fit worker pool pattern.
func (c *ClusterHealthChecker) Start(ctx context.Context, clusterNames []string) {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Initial check
		c.checkAll(ctx, clusterNames)

		for {
			select {
			case <-ticker.C:
				c.checkAll(ctx, clusterNames)
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop halts periodic health checking.
// ISSUE-010 FIX: Uses sync.Once to prevent double-close panic.
func (c *ClusterHealthChecker) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *ClusterHealthChecker) checkAll(ctx context.Context, clusterNames []string) {
	for _, name := range clusterNames {
		health := c.CheckCluster(ctx, name)
		c.UpdateHealth(health)
	}
}
