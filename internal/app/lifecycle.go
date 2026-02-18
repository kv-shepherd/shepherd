package app

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

// Start starts all background services (River workers, health checker).
func (a *Application) Start(ctx context.Context) error {
	if a.DB != nil && a.DB.RiverClient != nil {
		if err := a.DB.RiverClient.Start(ctx); err != nil {
			return fmt.Errorf("start river client: %w", err)
		}
		logger.Info("River client started, jobs will now be consumed")
	}

	if a.HealthCheck != nil && a.EntClient != nil {
		if err := a.refreshClusterHealth(ctx); err != nil {
			return fmt.Errorf("initial cluster health refresh: %w", err)
		}
		go a.runClusterHealthLoop(ctx) //nolint:naked-goroutine // dedicated background lifecycle loop.
		logger.Info("Cluster health checker started")
	}

	return nil
}

// Shutdown gracefully shuts down all application components.
func (a *Application) Shutdown() {
	shutdownCtx := context.Background()

	if a.HealthCheck != nil {
		a.HealthCheck.Stop()
	}

	if a.DB != nil && a.DB.RiverClient != nil {
		if err := a.DB.RiverClient.Stop(shutdownCtx); err != nil {
			logger.Error("failed to stop river client", zap.Error(err))
		}
		logger.Info("River client stopped")
	}

	for _, mod := range a.Modules {
		if mod == nil {
			continue
		}
		if err := mod.Shutdown(shutdownCtx); err != nil {
			logger.Warn("module shutdown returned error",
				zap.String("module", mod.Name()),
				zap.Error(err),
			)
		}
	}

	if a.Pools != nil {
		a.Pools.Shutdown()
	}
	if a.DB != nil {
		a.DB.Close()
	}
}

func (a *Application) runClusterHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.refreshClusterHealth(ctx); err != nil {
				logger.Warn("cluster health refresh failed", zap.Error(err))
			}
		}
	}
}

func (a *Application) refreshClusterHealth(ctx context.Context) error {
	clusters, err := a.EntClient.Cluster.Query().
		Where(entcluster.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query enabled clusters: %w", err)
	}

	for _, cl := range clusters {
		health := a.HealthCheck.CheckCluster(ctx, cl.ID)
		a.HealthCheck.UpdateHealth(health)

		nextStatus := mapClusterHealthStatus(health.Status)
		update := a.EntClient.Cluster.UpdateOneID(cl.ID).SetStatus(nextStatus)
		if health.KubeVirtVersion != "" {
			update = update.SetKubevirtVersion(health.KubeVirtVersion)
		}
		if _, err := update.Save(ctx); err != nil {
			logger.Warn("persist cluster health failed",
				zap.String("cluster_id", cl.ID),
				zap.String("cluster_name", cl.Name),
				zap.String("status", nextStatus.String()),
				zap.Error(err),
			)
			continue
		}
	}
	return nil
}

func mapClusterHealthStatus(status provider.ClusterStatus) entcluster.Status {
	switch status {
	case provider.ClusterStatusHealthy:
		return entcluster.StatusHEALTHY
	case provider.ClusterStatusUnhealthy:
		return entcluster.StatusUNHEALTHY
	case provider.ClusterStatusUnreachable:
		return entcluster.StatusUNREACHABLE
	default:
		return entcluster.StatusUNKNOWN
	}
}
