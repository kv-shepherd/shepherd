package modules

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/infrastructure"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
	"kv-shepherd.io/shepherd/internal/provider"
	infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"
	kubeconfigcodec "kv-shepherd.io/shepherd/internal/provider/kubeconfigcodec"
	"kv-shepherd.io/shepherd/internal/service"
)

// Infrastructure holds shared cross-cutting dependencies for all modules.
// It is a provider, not a Module.
type Infrastructure struct {
	Config        *config.Config
	EncryptionKey []byte
	DB            *infrastructure.DatabaseClients
	Pools         *worker.Pools
	EntClient     *ent.Client
	Pool          *pgxpool.Pool
	RiverClient   *river.Client[pgx.Tx]
	AuditLogger   *audit.Logger
	AuthSessions  *service.AuthSessionManager
	VMProvider    infracontract.InfrastructureProvider
	HealthCheck   *provider.ClusterHealthChecker
}

// NewInfrastructure initializes DB/pools and shared services.
func NewInfrastructure(ctx context.Context, cfg *config.Config) (*Infrastructure, error) {
	db, err := infrastructure.NewDatabaseClients(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// Dev-mode: auto-create Ent tables + River queue tables.
	if cfg.Database.AutoMigrate {
		if migrateErr := db.AutoMigrate(ctx); migrateErr != nil {
			db.Close()
			return nil, fmt.Errorf("auto-migrate: %w", migrateErr)
		}
	}

	resolvedSecurity, err := infrastructure.ResolveBootstrapSecuritySecrets(ctx, db.Pool, cfg.Security)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("resolve bootstrap security secrets: %w", err)
	}
	cfg.Security = resolvedSecurity
	if validateErr := cfg.ValidateResolvedSecuritySecrets(); validateErr != nil {
		db.Close()
		return nil, fmt.Errorf("validate resolved security secrets: %w", validateErr)
	}

	encryptionKey, err := cfg.Security.DecodeEncryptionKey()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("decode encryption key: %w", err)
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
	authSessions := service.NewAuthSessionManager(db.Pool, entClient, cfg.Session.IdleTimeout)
	clusterKubeconfigCodec := kubeconfigcodec.NewClusterKubeconfigCodec(encryptionKey)
	migrateLegacyClusterKubeconfigsOnStartup(entClient, clusterKubeconfigCodec)
	vmClusterFactory := provider.NewClusterClientFactoryFromKubeconfigLoader(newClusterKubeconfigLoader(entClient, clusterKubeconfigCodec, true))
	healthClusterFactory := provider.NewClusterClientFactoryFromKubeconfigLoader(newClusterKubeconfigLoader(entClient, clusterKubeconfigCodec, false))
	vmProvider := provider.NewKubeVirtProvider(
		vmClusterFactory,
		cfg.K8s.OperationTimeout,
	)
	healthChecker := provider.NewClusterHealthCheckerWithTimeout(
		healthClusterFactory,
		60*time.Second,
		cfg.K8s.OperationTimeout,
	)

	return &Infrastructure{
		Config:        cfg,
		EncryptionKey: encryptionKey,
		DB:            db,
		Pools:         pools,
		EntClient:     entClient,
		Pool:          db.Pool,
		RiverClient:   db.RiverClient,
		AuditLogger:   audit.NewLogger(entClient),
		AuthSessions:  authSessions,
		VMProvider:    vmProvider,
		HealthCheck:   healthChecker,
	}, nil
}

func newClusterKubeconfigLoader(client *ent.Client, codec *kubeconfigcodec.ClusterKubeconfigCodec, requireHealthy bool) provider.KubeconfigLoader {
	return func(clusterID string) ([]byte, error) {
		return loadClusterKubeconfigForRuntime(client, codec, clusterID, requireHealthy)
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
