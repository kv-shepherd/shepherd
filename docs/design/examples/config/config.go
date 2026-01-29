// Package config provides configuration management for KubeVirt Shepherd.
//
// Configuration is loaded from:
// 1. config.yaml file (optional)
// 2. Environment variables (ADR-0018: standard names like DATABASE_URL, SERVER_PORT)
// 3. Default values
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/config
package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the root configuration structure
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Session  SessionConfig  `mapstructure:"session"`
	K8s      K8sConfig      `mapstructure:"k8s"`
	Log      LogConfig      `mapstructure:"log"`
	River    RiverConfig    `mapstructure:"river"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig contains PostgreSQL connection settings
// ADR-0012: Shared connection pool for Ent + River + sqlc
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`

	// Pool configuration (shared by Ent, River, sqlc)
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`

	// Optional: PgBouncer dual-pool configuration
	WorkerHost string `mapstructure:"worker_host"`
	WorkerPort int    `mapstructure:"worker_port"`

	AutoMigrate bool `mapstructure:"auto_migrate"`
}

// SessionConfig contains session storage settings
// Sessions are stored in PostgreSQL (Redis removed)
type SessionConfig struct {
	Lifetime    time.Duration `mapstructure:"lifetime"`
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	Cookie      string        `mapstructure:"cookie"`
	Secure      bool          `mapstructure:"secure"`
	HttpOnly    bool          `mapstructure:"http_only"`
}

// K8sConfig contains Kubernetes operation settings
type K8sConfig struct {
	ClusterConcurrency int           `mapstructure:"cluster_concurrency"`
	OperationTimeout   time.Duration `mapstructure:"operation_timeout"`
}

// LogConfig contains logging settings
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json or console
}

// RiverConfig contains River Queue settings
type RiverConfig struct {
	MaxWorkers                  int           `mapstructure:"max_workers"`
	CompletedJobRetentionPeriod time.Duration `mapstructure:"completed_job_retention_period"`
}

// Load reads configuration from file and environment variables
// ADR-0018: Standard environment variables without prefix (DATABASE_URL, SERVER_PORT, etc.)
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/kubevirt-shepherd")

	// Environment variable override (ADR-0018)
	// No prefix: uses standard names like DATABASE_URL, SERVER_PORT, LOG_LEVEL
	// Maps nested config: database.max_conns → DATABASE_MAX_CONNS
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	// NOTE: No SetEnvPrefix - use standard env var names per ADR-0018

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file is optional, use defaults and env vars
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults() {
	// Server
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.shutdown_timeout", "30s")

	// Database (ADR-0012 shared pool)
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.max_conns", 50)
	viper.SetDefault("database.min_conns", 5)
	viper.SetDefault("database.max_conn_lifetime", "1h")
	viper.SetDefault("database.max_conn_idle_time", "10m")
	viper.SetDefault("database.auto_migrate", false)

	// Session (PostgreSQL-based, replaces Redis)
	viper.SetDefault("session.lifetime", "24h")
	viper.SetDefault("session.idle_timeout", "30m")
	viper.SetDefault("session.cookie", "session_id")
	viper.SetDefault("session.secure", true)
	viper.SetDefault("session.http_only", true)

	// K8s
	viper.SetDefault("k8s.cluster_concurrency", 20)
	viper.SetDefault("k8s.operation_timeout", "5m")

	// Log
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")

	// River
	viper.SetDefault("river.max_workers", 10)
	viper.SetDefault("river.completed_job_retention_period", "24h")
}
