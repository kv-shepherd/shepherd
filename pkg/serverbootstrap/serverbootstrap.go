package serverbootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/app"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type bootstrapFunc func(context.Context, *config.Config) (*app.Application, error)
type serveFunc func(*http.Server, chan<- error)

// Main runs the standard server entrypoint and exits non-zero on fatal errors.
func Main() {
	if err := Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// Run loads config, bootstraps the application, and serves until shutdown.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if initErr := logger.Init(cfg.Log.Level, cfg.Log.Format); initErr != nil {
		return fmt.Errorf("init logger: %w", initErr)
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warn: logger sync failed: %v\n", syncErr)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalizeStartupMigrationConfig(&cfg.Database)
	if migrateErr := infrastructure.EnsureStartupMigrations(ctx, cfg.Database); migrateErr != nil {
		return fmt.Errorf("prepare database schema: %w", migrateErr)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	return runWithConfig(ctx, cfg, signalCh, app.Bootstrap, defaultServe)
}

func normalizeStartupMigrationConfig(database *config.DatabaseConfig) {
	if database.AutoApplyVersionedMigrations && database.AutoMigrate {
		logger.Warn("Ignoring database.auto_migrate because versioned migrations are enabled")
		database.AutoMigrate = false
	}
}

func runWithConfig(
	ctx context.Context,
	cfg *config.Config,
	signalCh <-chan os.Signal,
	bootstrap bootstrapFunc,
	serve serveFunc,
) error {
	logger.Info("Starting KubeVirt Shepherd",
		zap.Int("port", cfg.Server.Port),
		zap.String("log_level", cfg.Log.Level),
	)

	application, err := bootstrap(ctx, cfg)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	defer application.Shutdown()

	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("start background services: %w", err)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           application.Router,
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	serve(srv, errCh)

	logger.Info("Server started", zap.String("addr", srv.Addr))

	select {
	case <-signalCh:
		logger.Info("Shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	logger.Info("Shutting down server...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("Server stopped gracefully")
	return nil
}

func defaultServe(srv *http.Server, errCh chan<- error) {
	// ADR-0031 exception: this entrypoint-owned goroutine only bridges
	// http.Server.ListenAndServe into shutdown-aware error handling.
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
}
