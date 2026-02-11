package modules

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
)

// Infrastructure holds shared cross-cutting dependencies for all modules.
// It is a provider, not a Module.
type Infrastructure struct {
	Config      *config.Config
	DB          *infrastructure.DatabaseClients
	Pools       *worker.Pools
	EntClient   *ent.Client
	Pool        *pgxpool.Pool
	RiverClient *river.Client[pgx.Tx]
	AuditLogger *audit.Logger
}

// NewInfrastructure initializes DB/pools and shared services.
func NewInfrastructure(ctx context.Context, cfg *config.Config) (*Infrastructure, error) {
	db, err := infrastructure.NewDatabaseClients(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	pools, err := worker.NewPools(ctx, worker.PoolConfig{
		GeneralPoolSize: cfg.Worker.GeneralPoolSize,
		K8sPoolSize:     cfg.Worker.K8sPoolSize,
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init worker pools: %w", err)
	}

	entClient := db.EntClient

	return &Infrastructure{
		Config:      cfg,
		DB:          db,
		Pools:       pools,
		EntClient:   entClient,
		Pool:        db.Pool,
		RiverClient: db.RiverClient,
		AuditLogger: audit.NewLogger(entClient),
	}, nil
}

// InitRiver initializes River client on top of a prepared worker registry.
func (i *Infrastructure) InitRiver(workers *river.Workers) error {
	if i == nil || i.DB == nil || i.Config == nil {
		return fmt.Errorf("infrastructure is not initialized")
	}
	if err := i.DB.InitRiverClient(workers, i.Config.River); err != nil {
		return fmt.Errorf("init river: %w", err)
	}
	i.RiverClient = i.DB.RiverClient
	return nil
}

// Close releases infra resources in reverse dependency order.
func (i *Infrastructure) Close() {
	if i == nil {
		return
	}
	if i.Pools != nil {
		i.Pools.Shutdown()
	}
	if i.DB != nil {
		i.DB.Close()
	}
}
