// Package infrastructure provides database and connection pool setup.
//
// ADR-0012: Uses shared pgxpool for Ent, River, and sqlc.
// This ensures atomic transactions across all three components.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/infrastructure
package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entmigrate "kv-shepherd.io/shepherd/ent/migrate"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// DatabaseClients contains all database-related clients.
// All clients share a single pgxpool connection pool.
//
// Coding Standard: Use this struct to manage connection pools.
// Do not create separate sql.Open() and pgxpool.New() (doubles connections).
type DatabaseClients struct {
	// Pool is the shared connection pool (Ent + River + sqlc).
	Pool *pgxpool.Pool

	// DB is the *sql.DB wrapper around Pool for Ent ORM (ADR-0012).
	// Created via stdlib.OpenDBFromPool to reuse pgxpool connections.
	DB *sql.DB

	// EntClient is the Ent ORM client backed by the shared pool.
	EntClient *ent.Client

	// RiverClient is the River job queue client backed by the shared pool.
	RiverClient *river.Client[pgx.Tx]

	// WorkerPool is optional: separate pool for PgBouncer scenarios.
	// nil means reuse Pool.
	WorkerPool *pgxpool.Pool
}

// NewDatabaseClients creates database clients with shared connection pool.
func NewDatabaseClients(ctx context.Context, cfg config.DatabaseConfig) (*DatabaseClients, error) {
	dsn := cfg.DSN()

	// Parse pool configuration
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = time.Minute

	// Set UTC timezone on each new connection (pgxpool best practice)
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET timezone = 'UTC'")
		return err
	}

	// Create shared connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Create *sql.DB from pool for Ent ORM (ADR-0012: stdlib.OpenDBFromPool)
	// This reuses the pgxpool connections instead of creating a separate pool.
	db := stdlib.OpenDBFromPool(pool)

	// Create Ent client backed by shared pool
	entDriver := entsql.OpenDB(dialect.Postgres, db)
	entClient := ent.NewClient(ent.Driver(entDriver))

	logger.Info("Database connection pool created",
		zap.Int32("max_conns", cfg.MaxConns),
		zap.Int32("min_conns", cfg.MinConns),
	)

	// Optional: separate WorkerPool for PgBouncer
	var workerPool *pgxpool.Pool
	if cfg.WorkerHost != "" {
		workerDSN := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			cfg.User, cfg.Password, cfg.WorkerHost, cfg.WorkerPort, cfg.Database, cfg.SSLMode)
		workerPool, err = pgxpool.New(ctx, workerDSN)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("create worker pool: %w", err)
		}
	}

	return &DatabaseClients{
		Pool:       pool,
		DB:         db,
		EntClient:  entClient,
		WorkerPool: workerPool,
	}, nil
}

// AutoMigrate runs Ent schema migration and River queue table migration.
// Only use in development; production should use Atlas-managed migrations.
func (c *DatabaseClients) AutoMigrate(ctx context.Context) error {
	// 1. Ent schema creation (creates all tables defined in ent/schema)
	logger.Info("Running Ent auto-migration...")
	if err := c.EntClient.Schema.Create(ctx,
		entmigrate.WithDropIndex(true),
		entmigrate.WithDropColumn(true),
		entmigrate.WithForeignKeys(true),
	); err != nil {
		return fmt.Errorf("ent auto-migrate: %w", err)
	}
	logger.Info("Ent auto-migration completed")

	// 2. River queue table migration (creates river_job, river_queue, etc.)
	logger.Info("Running River migration...")
	migrator, err := rivermigrate.New(riverpgxv5.New(c.Pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("river migrate up: %w", err)
	}
	if len(res.Versions) > 0 {
		logger.Info("River migration completed",
			zap.Int("versions_applied", len(res.Versions)),
		)
	} else {
		logger.Info("River migration: already up-to-date")
	}

	return nil
}

// InitRiverClient creates a River client with registered workers.
// Called after NewDatabaseClients; workers param comes from bootstrap.
func (c *DatabaseClients) InitRiverClient(workers *river.Workers, cfg config.RiverConfig) error {
	riverClient, err := river.NewClient(riverpgxv5.New(c.Pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.MaxWorkers},
		},
		Workers:                     workers,
		CompletedJobRetentionPeriod: cfg.CompletedJobRetentionPeriod,
	})
	if err != nil {
		return fmt.Errorf("create river client: %w", err)
	}
	c.RiverClient = riverClient
	logger.Info("River client initialized", zap.Int("max_workers", cfg.MaxWorkers))
	return nil
}

// GetWorkerPool returns the worker connection pool.
// Returns WorkerPool if configured, otherwise returns shared Pool.
func (c *DatabaseClients) GetWorkerPool() *pgxpool.Pool {
	if c.WorkerPool != nil {
		return c.WorkerPool
	}
	return c.Pool
}

// Close closes all connection pools gracefully.
func (c *DatabaseClients) Close() {
	if c.EntClient != nil {
		c.EntClient.Close()
	}
	if c.DB != nil {
		c.DB.Close()
	}
	if c.WorkerPool != nil {
		c.WorkerPool.Close()
	}
	if c.Pool != nil {
		c.Pool.Close()
	}
}
