package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// Start starts all background services (River workers, health checker).
func (a *Application) Start(ctx context.Context) error {
	if a.DB != nil && a.DB.RiverClient != nil {
		if err := a.DB.RiverClient.Start(ctx); err != nil {
			return fmt.Errorf("start river client: %w", err)
		}
		logger.Info("River client started, jobs will now be consumed")
	}
	return nil
}

// Shutdown gracefully shuts down all application components.
func (a *Application) Shutdown() {
	shutdownCtx := context.Background()

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
