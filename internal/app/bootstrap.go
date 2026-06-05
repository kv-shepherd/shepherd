// Package app — composition root. ADR-0022: bootstrap stays orchestration-only.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/app/modules"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/observability"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
	"kv-shepherd.io/shepherd/internal/provider"
	_ "kv-shepherd.io/shepherd/plugins/authprovider/autoreg" // Register built-in auth-provider plugins.
)

// Application holds composed application dependencies.
type Application struct {
	Config      *config.Config
	Router      *gin.Engine
	DB          *infrastructure.DatabaseClients
	Pools       *worker.Pools
	Modules     []modules.Module
	EntClient   *ent.Client
	HealthCheck *provider.ClusterHealthChecker
	Metrics     *observability.Metrics
	Tracing     *observability.Tracing
}

// Bootstrap initializes all dependencies using module-oriented manual DI.
func Bootstrap(ctx context.Context, cfg *config.Config) (*Application, error) {
	infra, err := modules.NewInfrastructure(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init infrastructure: %w", err)
	}

	vmModule, err := modules.NewVMModule(infra)
	if err != nil {
		infra.Close()
		return nil, fmt.Errorf("init vm module: %w", err)
	}

	baseModules := make([]modules.Module, 0, 4)
	baseModules = append(baseModules,
		vmModule,
		modules.NewGovernanceModule(infra),
		modules.NewAdminModule(infra),
	)

	workers := river.NewWorkers()
	for _, mod := range baseModules {
		registrar, ok := mod.(modules.WorkerRegistrar)
		if !ok {
			continue
		}
		registrar.RegisterWorkers(workers)
	}
	if initRiverErr := infra.InitRiver(workers); initRiverErr != nil {
		infra.Close()
		return nil, fmt.Errorf("init river workers: %w", initRiverErr)
	}
	if cfg.River.ConsumeJobs {
		registerPeriodicJobs(infra)
	}

	// P1-A: Pass vmModule's VMService to ApprovalModule to enable DryRun Pre-flight Gate (ADR-0006 Addendum).
	approvalModule, err := modules.NewApprovalModule(infra, vmModule.VMService())
	if err != nil {
		infra.Close()
		return nil, fmt.Errorf("init approval module: %w", err)
	}

	baseModules = append(baseModules, approvalModule)
	allModules := baseModules
	serverDeps := modules.NewServerDeps(cfg, infra, allModules)
	server := handlers.NewServer(serverDeps)
	metrics := newObservabilityMetrics(cfg, infra)
	tracing, err := newObservabilityTracing(ctx, cfg)
	if err != nil {
		infra.Close()
		return nil, fmt.Errorf("init tracing: %w", err)
	}

	return &Application{
		Config:      cfg,
		Router:      newRouter(cfg, server, serverDeps.JWTCfg, metrics, tracing),
		DB:          infra.DB,
		Pools:       infra.Pools,
		Modules:     allModules,
		EntClient:   infra.EntClient,
		HealthCheck: infra.HealthCheck,
		Metrics:     metrics,
		Tracing:     tracing,
	}, nil
}

func newObservabilityMetrics(cfg *config.Config, infra *modules.Infrastructure) *observability.Metrics {
	if cfg == nil || !cfg.Observability.MetricsEnabled {
		return nil
	}
	opts := make([]observability.Option, 0, 3)
	if cfg.Observability.DatabaseMetricsEnabled && infra != nil && infra.Pool != nil {
		opts = append(opts, observability.WithPostgresTableStats(
			infra.Pool,
			cfg.Observability.EffectiveDatabaseMetricsTimeout(),
		))
	}
	if cfg.Observability.RiverMetricsEnabled && infra != nil && infra.Pool != nil {
		opts = append(opts, observability.WithRiverQueueStats(
			infra.Pool,
			cfg.Observability.EffectiveRiverMetricsTimeout(),
		))
	}
	if cfg.Observability.BusinessMetricsEnabled && infra != nil && infra.Pool != nil {
		opts = append(opts, observability.WithBusinessMetrics(
			infra.Pool,
			cfg.Observability.EffectiveBusinessMetricsTimeout(),
		))
	}
	return observability.NewMetrics(opts...)
}

func newObservabilityTracing(ctx context.Context, cfg *config.Config) (*observability.Tracing, error) {
	if cfg == nil || !cfg.Observability.TracingEnabled {
		return nil, nil
	}
	return observability.NewTracing(ctx, observability.TracingOptions{
		Enabled:         true,
		ServiceName:     cfg.Observability.EffectiveTracingServiceName(),
		Exporter:        cfg.Observability.EffectiveTracingExporter(),
		SampleRatio:     cfg.Observability.EffectiveTracingSampleRatio(),
		ShutdownTimeout: cfg.Observability.EffectiveTracingShutdownTimeout(),
	})
}

// registerPeriodicJobs configures scheduled background tasks.
// Notification retention cleanup (master-flow Stage 5.F): run daily and once
// on startup to avoid long-lived inbox bloat.
func registerPeriodicJobs(infra *modules.Infrastructure) {
	if infra.RiverClient == nil {
		return
	}
	infra.RiverClient.PeriodicJobs().Add(
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.NotificationCleanupArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	)
	infra.RiverClient.PeriodicJobs().Add(
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.DomainEventArchiveArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	)
	infra.RiverClient.PeriodicJobs().Add(
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.VMTombstoneCleanupArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	)
	infra.RiverClient.PeriodicJobs().Add(
		river.NewPeriodicJob(
			river.PeriodicInterval(jobs.DefaultDirectoryEnrichmentScheduleScanInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.DirectoryEnrichmentScheduleScanArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	)
}
