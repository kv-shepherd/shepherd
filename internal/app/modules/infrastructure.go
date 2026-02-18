package modules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
	"kv-shepherd.io/shepherd/internal/provider"
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
	VMProvider  provider.InfrastructureProvider
	HealthCheck *provider.ClusterHealthChecker
}

// NewInfrastructure initializes DB/pools and shared services.
func NewInfrastructure(ctx context.Context, cfg *config.Config) (*Infrastructure, error) {
	db, err := infrastructure.NewDatabaseClients(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// Dev-mode: auto-create Ent tables + River queue tables.
	if cfg.Database.AutoMigrate {
		if err := db.AutoMigrate(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("auto-migrate: %w", err)
		}
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
	clusterFactory := provider.NewClusterClientFactoryFromKubeconfigLoader(newClusterKubeconfigLoader(entClient))
	vmProvider := provider.NewKubeVirtProvider(
		clusterFactory,
		cfg.K8s.OperationTimeout,
	)
	healthChecker := provider.NewClusterHealthChecker(clusterFactory, 60*time.Second)

	return &Infrastructure{
		Config:      cfg,
		DB:          db,
		Pools:       pools,
		EntClient:   entClient,
		Pool:        db.Pool,
		RiverClient: db.RiverClient,
		AuditLogger: audit.NewLogger(entClient),
		VMProvider:  vmProvider,
		HealthCheck: healthChecker,
	}, nil
}

func newClusterKubeconfigLoader(client *ent.Client) provider.KubeconfigLoader {
	return func(clusterID string) ([]byte, error) {
		if client == nil {
			return nil, fmt.Errorf("ent client is not initialized")
		}
		clusterID = strings.TrimSpace(clusterID)
		if clusterID == "" {
			return nil, fmt.Errorf("cluster id is required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cl, err := client.Cluster.Get(ctx, clusterID)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("cluster %s not found", clusterID)
			}
			return nil, err
		}
		if !cl.Enabled {
			return nil, fmt.Errorf("cluster %s is disabled", clusterID)
		}
		if cl.Status != cluster.StatusHEALTHY {
			return nil, fmt.Errorf("cluster %s is not healthy (status: %s)", clusterID, cl.Status)
		}
		if len(cl.EncryptedKubeconfig) == 0 {
			return nil, fmt.Errorf("cluster %s kubeconfig is empty", clusterID)
		}
		return cl.EncryptedKubeconfig, nil
	}
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
