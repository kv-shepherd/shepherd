package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	EnabledFeatures []string      `json:"enabled_features,omitempty"` // ADR-0014: merged GA + explicit featureGates
	StorageClasses  []string      `json:"storage_classes,omitempty"`  // ADR-0015: auto-detected cluster StorageClasses
	// StorageClassesDetected distinguishes a successful empty detection result from
	// a degraded health check where StorageClass listing was skipped or failed.
	StorageClassesDetected bool      `json:"storage_classes_detected,omitempty"`
	LastChecked            time.Time `json:"last_checked"`
	Error                  string    `json:"error,omitempty"`
}

// ClusterHealthChecker performs periodic health checks on registered clusters.
type ClusterHealthChecker struct {
	clientFactory      ClusterClientFactory
	capabilityDetector *CapabilityDetector // ADR-0014: nil-safe, optional
	interval           time.Duration
	operationTimeout   time.Duration
	results            map[string]*ClusterHealth
	mu                 sync.RWMutex
}

const defaultClusterHealthOperationTimeout = 5 * time.Minute

// NewClusterHealthChecker creates a new ClusterHealthChecker.
func NewClusterHealthChecker(clientFactory ClusterClientFactory, interval time.Duration) *ClusterHealthChecker {
	return NewClusterHealthCheckerWithTimeout(clientFactory, interval, defaultClusterHealthOperationTimeout)
}

// NewClusterHealthCheckerWithTimeout creates a new ClusterHealthChecker with a
// bounded timeout for each Kubernetes health probe.
func NewClusterHealthCheckerWithTimeout(
	clientFactory ClusterClientFactory,
	interval time.Duration,
	operationTimeout time.Duration,
) *ClusterHealthChecker {
	if operationTimeout <= 0 {
		operationTimeout = defaultClusterHealthOperationTimeout
	}
	return &ClusterHealthChecker{
		clientFactory:      clientFactory,
		capabilityDetector: NewCapabilityDetector(), // always enabled
		interval:           interval,
		operationTimeout:   operationTimeout,
		results:            make(map[string]*ClusterHealth),
	}
}

// CheckCluster performs a single health check for a cluster.
//
// Connectivity probe: calls client.KubeVirt().GetVersion() which does a GET on the
// cluster-scoped KubeVirt CR singleton. This is namespace-independent and always exists
// on correctly-installed KubeVirt clusters — unlike the former VM list probe which
// required VMs to exist in the "default" namespace.
//
// Capability detection: runs CapabilityDetector.Detect() after connectivity is confirmed.
// Detection failure is non-fatal (RBAC may restrict featureGates access).
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

	opCtx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	// Connectivity probe: GET KubeVirt CR (cluster-scoped singleton, no namespace dependency).
	// If this fails, the cluster is genuinely unreachable or RBAC is fully denied.
	// GetVersion() returns ("", nil) if the field is not yet populated — that is not an error.
	version, probeErr := client.KubeVirt().GetVersion(opCtx)
	if probeErr != nil {
		health.Status = ClusterStatusUnhealthy
		health.Error = fmt.Sprintf("kubevirt api probe failed: %v", probeErr)
		logger.Warn("Cluster health check probe failed",
			zap.String("cluster", clusterName),
			zap.Error(probeErr),
		)
		return health
	}

	health.Status = ClusterStatusHealthy
	if version != "" {
		health.KubeVirtVersion = version
	}

	// Capability detection: non-fatal, runs after connectivity confirmed.
	// capabilityDetector is always non-nil (initialized in NewClusterHealthChecker).
	if c.capabilityDetector != nil {
		caps, detectErr := c.capabilityDetector.Detect(opCtx, client)
		if detectErr != nil {
			// Non-fatal: log and continue with empty EnabledFeatures.
			logger.Warn("capability detection failed, using GA table only",
				zap.String("cluster", clusterName),
				zap.Error(detectErr),
			)
		} else {
			health.EnabledFeatures = caps.EnabledFeatures
			// Prefer version from caps (may override the probe result with more detail).
			if caps.KubeVirtVersion != "" {
				health.KubeVirtVersion = caps.KubeVirtVersion
			}
		}
	}

	if storageClient := client.StorageClass(); storageClient != nil {
		storageClasses, detectErr := detectStorageClasses(opCtx, storageClient)
		if detectErr != nil {
			logger.Warn("storage class detection failed",
				zap.String("cluster", clusterName),
				zap.Error(detectErr),
			)
		} else {
			health.StorageClasses = storageClasses
			health.StorageClassesDetected = true
		}
	}

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
func (c *ClusterHealthChecker) Start(ctx context.Context, clusterNames []string) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial check
	c.checkAll(ctx, clusterNames)

	for {
		select {
		case <-ticker.C:
			c.checkAll(ctx, clusterNames)
		case <-ctx.Done():
			return
		}
	}
}

func (c *ClusterHealthChecker) checkAll(ctx context.Context, clusterNames []string) {
	for _, name := range clusterNames {
		health := c.CheckCluster(ctx, name)
		c.UpdateHealth(health)
	}
}

func detectStorageClasses(opCtx context.Context, storageClient StorageClassClient) ([]string, error) {
	list, err := storageClient.List(opCtx, k8smetav1.ListOptions{ResourceVersion: ""})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		if item.Name == "" {
			continue
		}
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names, nil
}
