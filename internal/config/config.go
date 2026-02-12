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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config is the root configuration structure.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Session  SessionConfig  `mapstructure:"session"`
	K8s      K8sConfig      `mapstructure:"k8s"`
	Log      LogConfig      `mapstructure:"log"`
	River    RiverConfig    `mapstructure:"river"`
	Security SecurityConfig `mapstructure:"security"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig contains PostgreSQL connection settings.
// ADR-0012: Shared connection pool for Ent + River + sqlc.
type DatabaseConfig struct {
	URL string `mapstructure:"url"`

	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	SSLMode  string `mapstructure:"sslmode"`

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

// DSN returns the PostgreSQL connection string.
// Priority: DATABASE_URL > constructed from individual fields.
func (c DatabaseConfig) DSN() string {
	if c.URL != "" {
		return c.URL
	}
	sslmode := c.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, sslmode,
	)
}

// SessionConfig contains session storage settings.
// Sessions are stored in PostgreSQL (Redis removed).
type SessionConfig struct {
	Lifetime    time.Duration `mapstructure:"lifetime"`
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	Cookie      string        `mapstructure:"cookie"`
	Secure      bool          `mapstructure:"secure"`
	HttpOnly    bool          `mapstructure:"http_only"`
}

// K8sConfig contains Kubernetes operation settings.
type K8sConfig struct {
	ClusterConcurrency int           `mapstructure:"cluster_concurrency"`
	OperationTimeout   time.Duration `mapstructure:"operation_timeout"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json or console
}

// RiverConfig contains River Queue settings.
type RiverConfig struct {
	MaxWorkers                  int           `mapstructure:"max_workers"`
	CompletedJobRetentionPeriod time.Duration `mapstructure:"completed_job_retention_period"`
}

// SecurityConfig contains security-related settings.
// ADR-0025: Auto-generate secrets on first boot if missing.
type SecurityConfig struct {
	EncryptionKey       string         `mapstructure:"encryption_key"`
	SessionSecret       string         `mapstructure:"session_secret"`
	JWTVerificationKeys []string       `mapstructure:"jwt_verification_keys"`
	PasswordPolicy      PasswordPolicy `mapstructure:"password_policy"`
}

// PasswordPolicy defines password validation rules.
// Default mode is "nist" (NIST 800-63B compliant).
type PasswordPolicy struct {
	Mode             string `mapstructure:"mode"` // "nist" (default) or "legacy"
	RequireUppercase bool   `mapstructure:"require_uppercase"`
	RequireLowercase bool   `mapstructure:"require_lowercase"`
	RequireDigit     bool   `mapstructure:"require_digit"`
	RequireSpecial   bool   `mapstructure:"require_special"`
}

// WorkerConfig contains worker pool settings.
type WorkerConfig struct {
	GeneralPoolSize int `mapstructure:"general_pool_size"`
	K8sPoolSize     int `mapstructure:"k8s_pool_size"`
}

var (
	bootstrapLoggerOnce sync.Once
	bootstrapLogger     *zap.Logger
)

// Load reads configuration from file and environment variables.
// ADR-0018: Standard environment variables without prefix (DATABASE_URL, SERVER_PORT, etc.).
func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/kubevirt-shepherd")

	// Environment variable override (ADR-0018)
	// No prefix: uses standard names like DATABASE_URL, SERVER_PORT, LOG_LEVEL
	// Maps nested config: database.max_conns → DATABASE_MAX_CONNS
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Config file is optional, use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// ADR-0025: Auto-generate secrets on first boot if missing.
	if err := cfg.ensureSecrets(); err != nil {
		return nil, fmt.Errorf("ensure secrets: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// Validate checks for critical configuration errors.
func (c *Config) Validate() error {
	if c.Security.SessionSecret == "" {
		return fmt.Errorf("security.session_secret must not be empty")
	}
	if len(c.Security.SessionSecret) < 32 {
		return fmt.Errorf("security.session_secret must be at least 32 characters")
	}
	return nil
}

// ensureSecrets auto-generates missing secrets per ADR-0025.
func (c *Config) ensureSecrets() error {
	if c.Security.SessionSecret == "" {
		secret, err := generateSecureRandomHex(32)
		if err != nil {
			return fmt.Errorf("auto-generate session secret: %w", err)
		}
		c.Security.SessionSecret = secret
		logBootstrapWarn(
			"auto-generated session_secret (ADR-0025); set SECURITY_SESSION_SECRET env var for persistence",
			zap.Int("length", len(secret)),
		)
	}
	if c.Security.EncryptionKey == "" {
		key, err := generateSecureRandomHex(32)
		if err != nil {
			return fmt.Errorf("auto-generate encryption key: %w", err)
		}
		c.Security.EncryptionKey = key
		logBootstrapWarn(
			"auto-generated encryption_key (ADR-0025); set SECURITY_ENCRYPTION_KEY env var for persistence",
			zap.Int("length", len(key)),
		)
	}
	return nil
}

func logBootstrapWarn(msg string, fields ...zap.Field) {
	bootstrapLoggerOnce.Do(func() {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)

		l, err := cfg.Build()
		if err != nil {
			bootstrapLogger = zap.NewNop()
			return
		}
		bootstrapLogger = l
	})

	bootstrapLogger.Warn(msg, fields...)
}

// generateSecureRandomHex produces a hex-encoded string of n random bytes.
func generateSecureRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "30s")

	// Database (ADR-0012 shared pool)
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "shepherd")
	v.SetDefault("database.password", "")
	v.SetDefault("database.database", "shepherd")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_conns", 50)
	v.SetDefault("database.min_conns", 5)
	v.SetDefault("database.max_conn_lifetime", "1h")
	v.SetDefault("database.max_conn_idle_time", "10m")
	v.SetDefault("database.auto_migrate", false)

	// Session (PostgreSQL-based, replaces Redis)
	v.SetDefault("session.lifetime", "24h")
	v.SetDefault("session.idle_timeout", "30m")
	v.SetDefault("session.cookie", "session_id")
	v.SetDefault("session.secure", true)
	v.SetDefault("session.http_only", true)

	// K8s
	v.SetDefault("k8s.cluster_concurrency", 20)
	v.SetDefault("k8s.operation_timeout", "5m")

	// Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// River
	v.SetDefault("river.max_workers", 10)
	v.SetDefault("river.completed_job_retention_period", "24h")

	// Security (ADR-0025)
	v.SetDefault("security.password_policy.mode", "nist")
	v.SetDefault("security.jwt_verification_keys", []string{})

	// Worker Pool (ADR-0031)
	v.SetDefault("worker.general_pool_size", 100)
	v.SetDefault("worker.k8s_pool_size", 50)
}
