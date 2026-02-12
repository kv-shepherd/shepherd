// Package app — composition root. ADR-0022: bootstrap stays orchestration-only.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/app/modules"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
)

// Application holds composed application dependencies.
type Application struct {
	Config  *config.Config
	Router  *gin.Engine
	DB      *infrastructure.DatabaseClients
	Pools   *worker.Pools
	Modules []modules.Module
}

// Bootstrap initializes all dependencies using module-oriented manual DI.
func Bootstrap(ctx context.Context, cfg *config.Config) (*Application, error) {
	infra, err := modules.NewInfrastructure(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init infrastructure: %w", err)
	}

	baseModules := []modules.Module{
		modules.NewVMModule(infra),
		modules.NewGovernanceModule(infra),
		modules.NewAdminModule(infra),
	}

	workers := river.NewWorkers()
	for _, mod := range baseModules {
		mod.RegisterWorkers(workers)
	}
	if err := infra.InitRiver(workers); err != nil {
		infra.Close()
		return nil, fmt.Errorf("init river workers: %w", err)
	}
	// Notification retention cleanup (master-flow Stage 5.F): run daily and once
	// on startup to avoid long-lived inbox bloat.
	if infra.RiverClient != nil {
		infra.RiverClient.PeriodicJobs().Add(
			river.NewPeriodicJob(
				river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) {
					return jobs.NotificationCleanupArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		)
	}

	approvalModule, err := modules.NewApprovalModule(infra)
	if err != nil {
		infra.Close()
		return nil, fmt.Errorf("init approval module: %w", err)
	}

	allModules := append(baseModules, approvalModule)
	serverDeps := modules.NewServerDeps(cfg, infra, allModules)
	server := handlers.NewServer(serverDeps)

	return &Application{
		Config:  cfg,
		Router:  newRouter(cfg, server, serverDeps.JWTCfg),
		DB:      infra.DB,
		Pools:   infra.Pools,
		Modules: allModules,
	}, nil
}
