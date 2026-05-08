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
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const riverMaxWorkersLimit = 10_000

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
	Port                int           `mapstructure:"port"`
	ReadTimeout         time.Duration `mapstructure:"read_timeout"`
	ReadHeaderTimeout   time.Duration `mapstructure:"read_header_timeout"`
	WriteTimeout        time.Duration `mapstructure:"write_timeout"`
	IdleTimeout         time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout     time.Duration `mapstructure:"shutdown_timeout"`
	PublicBaseURL       string        `mapstructure:"public_base_url"`
	MaxRequestBodyBytes int64         `mapstructure:"max_request_body_bytes"`
	AllowedOrigins      []string      `mapstructure:"allowed_origins"`
	TrustedProxies      []string      `mapstructure:"trusted_proxies"`
	AllowCredentials    bool          `mapstructure:"allow_credentials"`
	// UnsafeAllowAllOrigins disables origin allowlist checks and must only be used in trusted local development.
	UnsafeAllowAllOrigins    bool   `mapstructure:"unsafe_allow_all_origins"`
	UnsafeAllowAllOriginsAck string `mapstructure:"unsafe_allow_all_origins_ack"`
}

// DatabaseConfig contains PostgreSQL connection settings.
// ADR-0012: Shared connection pool for Ent + River + sqlc.
type DatabaseConfig struct {
	URL string `mapstructure:"url"`

	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"` //nolint:gosec // Configuration schema intentionally models a credential field.
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

	AutoMigrate                  bool `mapstructure:"auto_migrate"`
	AutoApplyVersionedMigrations bool `mapstructure:"auto_apply_versioned_migrations"`
}

// DSN returns the PostgreSQL connection string.
// Priority: DATABASE_URL > constructed from individual fields.
func (c DatabaseConfig) DSN() string {
	if c.URL != "" {
		return c.URL
	}
	sslmode := c.SSLMode
	if sslmode == "" {
		sslmode = "require"
	}
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port)),
		Path:     "/" + c.Database,
		RawQuery: url.Values{"sslmode": []string{sslmode}}.Encode(),
	}).String()
}

// SessionConfig contains session storage settings.
// Sessions are stored in PostgreSQL (Redis removed).
type SessionConfig struct {
	Lifetime    time.Duration `mapstructure:"lifetime"`
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	Cookie      string        `mapstructure:"cookie"`
	Secure      bool          `mapstructure:"secure"`
	HTTPOnly    bool          `mapstructure:"http_only"`
}

// K8sConfig contains Kubernetes operation settings.
type K8sConfig struct {
	OperationTimeout time.Duration `mapstructure:"operation_timeout"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json or console
}

// RiverConfig contains River Queue settings.
type RiverConfig struct {
	ConsumeJobs                 bool          `mapstructure:"consume_jobs"`
	MaxWorkers                  int           `mapstructure:"max_workers"`
	CompletedJobRetentionPeriod time.Duration `mapstructure:"completed_job_retention_period"`
}

// SecurityConfig contains security-related settings.
// ADR-0025: explicit values may come from config/env; unresolved values are
// completed later by runtime bootstrap using env > DB > generated persistence.
type SecurityConfig struct {
	EncryptionKey       string         `mapstructure:"encryption_key"`
	SessionSecret       string         `mapstructure:"session_secret"` //nolint:gosec // Configuration schema intentionally models a secret field.
	JWTVerificationKeys []string       `mapstructure:"jwt_verification_keys"`
	PasswordPolicy      PasswordPolicy `mapstructure:"password_policy"`
	LoginRateLimit      LoginRateLimit `mapstructure:"login_rate_limit"`
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

// LoginRateLimit defines application-layer throttling for login attempts.
type LoginRateLimit struct {
	Enabled       bool          `mapstructure:"enabled"`
	MaxFailures   int           `mapstructure:"max_failures"`
	Window        time.Duration `mapstructure:"window"`
	BlockDuration time.Duration `mapstructure:"block_duration"`
}

// WorkerConfig contains worker pool settings.
type WorkerConfig struct {
	GeneralPoolSize int `mapstructure:"general_pool_size"`
	K8sPoolSize     int `mapstructure:"k8s_pool_size"`
}

const (
	defaultMaxRequestBodyBytes    int64 = 10 << 20 // 10 MiB
	unsafeAllowAllOriginsAckValue       = "I_UNDERSTAND_THIS_IS_UNSAFE"
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
	bindEnvKeys(v)

	if err := v.ReadInConfig(); err != nil {
		var notFoundErr viper.ConfigFileNotFoundError
		if !errors.As(err, &notFoundErr) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Config file is optional, use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	applyExplicitEnvOverrides(v, &cfg)
	cfg.Server.AllowedOrigins = mergeAllowedOrigins(cfg.Server.AllowedOrigins, cfg.Server.PublicBaseURL)
	cfg.Server.TrustedProxies = sanitizeStringSlice(cfg.Server.TrustedProxies)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	if err := validateLoadedRiverConfig(cfg.River); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// Validate checks for critical configuration errors.
func (c *Config) Validate() error {
	if secret := strings.TrimSpace(c.Security.SessionSecret); secret != "" && len(secret) < 32 {
		return fmt.Errorf("security.session_secret must be at least 32 characters")
	}
	if baseURL := strings.TrimSpace(c.Server.PublicBaseURL); baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("server.public_base_url must be an absolute http or https URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("server.public_base_url must use http or https")
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return fmt.Errorf("server.public_base_url must not include a path")
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("server.public_base_url must not include query or fragment")
		}
	}
	if c.Server.MaxRequestBodyBytes < 0 {
		return fmt.Errorf("server.max_request_body_bytes must be >= 0")
	}
	if c.Server.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("server.read_header_timeout must be > 0")
	}
	if c.Server.IdleTimeout <= 0 {
		return fmt.Errorf("server.idle_timeout must be > 0")
	}
	if isReleaseMode() {
		if c.Server.MaxRequestBodyBytes == 0 {
			return fmt.Errorf("server.max_request_body_bytes must be > 0 when GIN_MODE=release")
		}
		if !c.Session.HTTPOnly {
			return fmt.Errorf("session.http_only must be true when GIN_MODE=release")
		}
		if !c.Session.Secure {
			return fmt.Errorf("session.secure must be true when GIN_MODE=release")
		}
	}
	for _, proxy := range sanitizeStringSlice(c.Server.TrustedProxies) {
		if _, _, err := net.ParseCIDR(proxy); err == nil {
			continue
		}
		if ip := net.ParseIP(proxy); ip != nil {
			continue
		}
		return fmt.Errorf("server.trusted_proxies contains invalid IP or CIDR %q", proxy)
	}
	if c.Server.UnsafeAllowAllOrigins {
		if strings.TrimSpace(c.Server.UnsafeAllowAllOriginsAck) != unsafeAllowAllOriginsAckValue {
			return fmt.Errorf(
				"server.unsafe_allow_all_origins requires server.unsafe_allow_all_origins_ack=%q",
				unsafeAllowAllOriginsAckValue,
			)
		}
		if c.Server.AllowCredentials {
			return fmt.Errorf("server.allow_credentials must be false when server.unsafe_allow_all_origins is enabled")
		}
		if isReleaseMode() {
			return fmt.Errorf("server.unsafe_allow_all_origins must remain false when GIN_MODE=release")
		}
	}
	if c.Security.LoginRateLimit.Enabled {
		if c.Security.LoginRateLimit.MaxFailures < 1 {
			return fmt.Errorf("security.login_rate_limit.max_failures must be >= 1")
		}
		if c.Security.LoginRateLimit.Window <= 0 {
			return fmt.Errorf("security.login_rate_limit.window must be > 0")
		}
		if c.Security.LoginRateLimit.BlockDuration <= 0 {
			return fmt.Errorf("security.login_rate_limit.block_duration must be > 0")
		}
	}
	if c.River.MaxWorkers != 0 {
		if err := validateLoadedRiverConfig(c.River); err != nil {
			return err
		}
	} else if c.River.CompletedJobRetentionPeriod < -1 {
		return fmt.Errorf("river.completed_job_retention_period must be >= -1")
	}
	if key := strings.TrimSpace(c.Security.EncryptionKey); key != "" {
		if _, err := c.Security.DecodeEncryptionKey(); err != nil {
			return err
		}
	}
	return nil
}

func validateLoadedRiverConfig(cfg RiverConfig) error {
	if cfg.MaxWorkers < 1 || cfg.MaxWorkers > riverMaxWorkersLimit {
		return fmt.Errorf("river.max_workers must be between 1 and %d", riverMaxWorkersLimit)
	}
	if cfg.CompletedJobRetentionPeriod < -1 {
		return fmt.Errorf("river.completed_job_retention_period must be >= -1")
	}
	return nil
}

func isReleaseMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release")
}

// ValidateResolvedSecuritySecrets validates the runtime-required security
// values after bootstrap secret resolution has completed.
func (c *Config) ValidateResolvedSecuritySecrets() error {
	if strings.TrimSpace(c.Security.SessionSecret) == "" {
		return fmt.Errorf("security.session_secret must not be empty")
	}
	if strings.TrimSpace(c.Security.EncryptionKey) == "" {
		return fmt.Errorf("security.encryption_key must not be empty")
	}
	return c.Validate()
}

// DecodeEncryptionKey decodes the configured AES-256-GCM key.
func (c SecurityConfig) DecodeEncryptionKey() ([]byte, error) {
	key := strings.TrimSpace(c.EncryptionKey)
	if key == "" {
		return nil, fmt.Errorf("security.encryption_key must not be empty")
	}

	raw, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("security.encryption_key must be hex-encoded: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("security.encryption_key must decode to 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.read_header_timeout", "10s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "2m")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("server.public_base_url", "")
	v.SetDefault("server.max_request_body_bytes", defaultMaxRequestBodyBytes)
	v.SetDefault("server.allowed_origins", []string{"http://localhost:3000", "http://127.0.0.1:3000"})
	v.SetDefault("server.trusted_proxies", []string{})
	v.SetDefault("server.allow_credentials", true)
	v.SetDefault("server.unsafe_allow_all_origins", false)
	v.SetDefault("server.unsafe_allow_all_origins_ack", "")

	// Database (ADR-0012 shared pool)
	v.SetDefault("database.url", "")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "shepherd")
	v.SetDefault("database.password", "")
	v.SetDefault("database.database", "shepherd")
	v.SetDefault("database.sslmode", "require")
	v.SetDefault("database.max_conns", 50)
	v.SetDefault("database.min_conns", 5)
	v.SetDefault("database.max_conn_lifetime", "1h")
	v.SetDefault("database.max_conn_idle_time", "10m")
	v.SetDefault("database.auto_migrate", false)
	v.SetDefault("database.auto_apply_versioned_migrations", false)

	// Session (PostgreSQL-based, replaces Redis)
	v.SetDefault("session.lifetime", "24h")
	v.SetDefault("session.idle_timeout", "30m")
	v.SetDefault("session.cookie", "shepherd_session")
	v.SetDefault("session.secure", true)
	v.SetDefault("session.http_only", true)

	// K8s
	v.SetDefault("k8s.operation_timeout", "5m")

	// Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// River
	v.SetDefault("river.consume_jobs", true)
	v.SetDefault("river.max_workers", 10)
	v.SetDefault("river.completed_job_retention_period", "24h")

	// Security (ADR-0025)
	v.SetDefault("security.password_policy.mode", "nist")
	v.SetDefault("security.jwt_verification_keys", []string{})
	v.SetDefault("security.login_rate_limit.enabled", true)
	v.SetDefault("security.login_rate_limit.max_failures", 5)
	v.SetDefault("security.login_rate_limit.window", "15m")
	v.SetDefault("security.login_rate_limit.block_duration", "15m")

	// Worker Pool (ADR-0031)
	v.SetDefault("worker.general_pool_size", 100)
	v.SetDefault("worker.k8s_pool_size", 50)
}

func bindEnvKeys(v *viper.Viper) {
	for _, key := range []string{
		"server.port",
		"server.read_timeout",
		"server.read_header_timeout",
		"server.write_timeout",
		"server.idle_timeout",
		"server.shutdown_timeout",
		"server.public_base_url",
		"server.max_request_body_bytes",
		"server.allowed_origins",
		"server.trusted_proxies",
		"server.allow_credentials",
		"server.unsafe_allow_all_origins",
		"server.unsafe_allow_all_origins_ack",
		"database.url",
		"database.host",
		"database.port",
		"database.user",
		"database.password",
		"database.database",
		"database.sslmode",
		"database.max_conns",
		"database.min_conns",
		"database.max_conn_lifetime",
		"database.max_conn_idle_time",
		"database.worker_host",
		"database.worker_port",
		"database.auto_migrate",
		"database.auto_apply_versioned_migrations",
		"session.lifetime",
		"session.idle_timeout",
		"session.cookie",
		"session.secure",
		"session.http_only",
		"k8s.operation_timeout",
		"log.level",
		"log.format",
		"river.consume_jobs",
		"river.max_workers",
		"river.completed_job_retention_period",
		"security.encryption_key",
		"security.session_secret",
		"security.jwt_verification_keys",
		"security.password_policy.mode",
		"security.password_policy.require_uppercase",
		"security.password_policy.require_lowercase",
		"security.password_policy.require_digit",
		"security.password_policy.require_special",
		"security.login_rate_limit.enabled",
		"security.login_rate_limit.max_failures",
		"security.login_rate_limit.window",
		"security.login_rate_limit.block_duration",
		"worker.general_pool_size",
		"worker.k8s_pool_size",
	} {
		if err := v.BindEnv(key); err != nil {
			panic(fmt.Sprintf("bind env for %s: %v", key, err))
		}
	}
}

func applyExplicitEnvOverrides(v *viper.Viper, cfg *Config) {
	if cfg == nil {
		return
	}
	if value := strings.TrimSpace(v.GetString("server.public_base_url")); value != "" {
		cfg.Server.PublicBaseURL = value
	}
	if value := strings.TrimSpace(v.GetString("security.session_secret")); value != "" {
		cfg.Security.SessionSecret = value
	}
	if value := strings.TrimSpace(v.GetString("security.encryption_key")); value != "" {
		cfg.Security.EncryptionKey = value
	}
	if v.IsSet("server.trusted_proxies") {
		cfg.Server.TrustedProxies = sanitizeStringSlice(v.GetStringSlice("server.trusted_proxies"))
	}
	if v.IsSet("security.jwt_verification_keys") {
		cfg.Security.JWTVerificationKeys = sanitizeStringSlice(v.GetStringSlice("security.jwt_verification_keys"))
	}
}

func mergeAllowedOrigins(origins []string, publicBaseURL string) []string {
	items := sanitizeStringSlice(origins)
	origin := publicBaseOrigin(publicBaseURL)
	if origin == "" {
		return items
	}
	for _, item := range items {
		if strings.EqualFold(item, origin) {
			return items
		}
	}
	return append(items, origin)
}

func publicBaseOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func sanitizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			items = append(items, part)
		}
	}
	return items
}
