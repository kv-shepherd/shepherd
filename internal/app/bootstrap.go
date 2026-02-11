// Package app — composition root. ADR-0022: bootstrap stays orchestration-only.
package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/app/modules"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/infrastructure"
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

	approvalModule, err := modules.NewApprovalModule(infra)
	if err != nil {
		infra.Close()
		return nil, fmt.Errorf("init approval module: %w", err)
	}

	allModules := append(baseModules, approvalModule)
	server := handlers.NewServer(modules.NewServerDeps(cfg, infra, allModules))

	return &Application{
		Config:  cfg,
		Router:  newRouter(server, []byte(cfg.Security.SessionSecret)),
		DB:      infra.DB,
		Pools:   infra.Pools,
		Modules: allModules,
	}, nil
}
