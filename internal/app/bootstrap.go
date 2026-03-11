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
	registerPeriodicJobs(infra)

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

	return &Application{
		Config:      cfg,
		Router:      newRouter(cfg, server, serverDeps.JWTCfg),
		DB:          infra.DB,
		Pools:       infra.Pools,
		Modules:     allModules,
		EntClient:   infra.EntClient,
		HealthCheck: infra.HealthCheck,
	}, nil
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
}
