// Package infrastructure provides database and connection pool setup.
//
// ADR-0012: Uses shared pgxpool for Ent, River, and sqlc.
// This ensures atomic transactions across all three components.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/infrastructure
package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// DatabaseClients contains all database-related clients.
// All clients share a single pgxpool connection pool.
//
// Coding Standard: Use this struct to manage connection pools.
// Do not create separate sql.Open() and pgxpool.New() (doubles connections).
type DatabaseClients struct {
	// Pool is the shared connection pool (Ent + River + sqlc reuse)
	Pool *pgxpool.Pool

	// EntClient is the Ent ORM client
	EntClient *ent.Client

	// SqlcQueries is the sqlc query client (for core transactions)
	SqlcQueries *sqlc.Queries

	// WorkerPool is optional: separate pool for PgBouncer scenarios
	// nil means reuse Pool
	WorkerPool *pgxpool.Pool
}

// NewDatabaseClients creates database clients with shared connection pool.
func NewDatabaseClients(ctx context.Context, cfg config.DatabaseConfig) (*DatabaseClients, error) {
	// Build PostgreSQL DSN
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
	)

	// Parse pool configuration
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

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

	// Ent Client: reuse pgxpool via stdlib.OpenDBFromPool
	entDB := stdlib.OpenDBFromPool(pool)
	entDriver := entsql.OpenDB(dialect.Postgres, entDB)
	entClient := ent.NewClient(ent.Driver(entDriver))

	// sqlc Queries: use pgxpool directly
	sqlcQueries := sqlc.New(pool)

	// Optional: separate WorkerPool for PgBouncer
	var workerPool *pgxpool.Pool
	if cfg.WorkerHost != "" {
		workerDSN := fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
			cfg.User, cfg.Password, cfg.WorkerHost, cfg.WorkerPort, cfg.Database)
		workerPool, err = pgxpool.New(ctx, workerDSN)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("create worker pool: %w", err)
		}
	}

	return &DatabaseClients{
		Pool:        pool,
		EntClient:   entClient,
		SqlcQueries: sqlcQueries,
		WorkerPool:  workerPool,
	}, nil
}

// GetWorkerPool returns the worker connection pool.
// Returns WorkerPool if configured, otherwise returns shared Pool.
func (c *DatabaseClients) GetWorkerPool() *pgxpool.Pool {
	if c.WorkerPool != nil {
		return c.WorkerPool
	}
	return c.Pool
}

// NewRiverClient creates a River queue client.
func (c *DatabaseClients) NewRiverClient(workers *river.Workers, cfg config.RiverConfig) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(c.GetWorkerPool()), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.MaxWorkers},
		},
		Workers:                     workers,
		CompletedJobRetentionPeriod: cfg.CompletedJobRetentionPeriod,
	})
}

// Close closes all connection pools gracefully.
func (c *DatabaseClients) Close() {
	if c.EntClient != nil {
		c.EntClient.Close()
	}
	if c.WorkerPool != nil {
		c.WorkerPool.Close()
	}
	if c.Pool != nil {
		c.Pool.Close()
	}
}
