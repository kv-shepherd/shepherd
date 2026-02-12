package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Ensure no env vars interfere
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("DATABASE_URL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Server defaults
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 30s", cfg.Server.ReadTimeout)
	}
	if !cfg.Server.AllowCredentials {
		t.Errorf("Server.AllowCredentials = %v, want true", cfg.Server.AllowCredentials)
	}
	if cfg.Server.UnsafeAllowAllOrigins {
		t.Errorf("Server.UnsafeAllowAllOrigins = %v, want false", cfg.Server.UnsafeAllowAllOrigins)
	}

	// Database defaults
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want localhost", cfg.Database.Host)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want 5432", cfg.Database.Port)
	}
	if cfg.Database.MaxConns != 50 {
		t.Errorf("Database.MaxConns = %d, want 50", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 5 {
		t.Errorf("Database.MinConns = %d, want 5", cfg.Database.MinConns)
	}

	// K8s defaults
	if cfg.K8s.ClusterConcurrency != 20 {
		t.Errorf("K8s.ClusterConcurrency = %d, want 20", cfg.K8s.ClusterConcurrency)
	}

	// Log defaults
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want json", cfg.Log.Format)
	}

	// River defaults
	if cfg.River.MaxWorkers != 10 {
		t.Errorf("River.MaxWorkers = %d, want 10", cfg.River.MaxWorkers)
	}

	// Security defaults
	if cfg.Security.PasswordPolicy.Mode != "nist" {
		t.Errorf("PasswordPolicy.Mode = %q, want nist", cfg.Security.PasswordPolicy.Mode)
	}

	// Worker pool defaults
	if cfg.Worker.GeneralPoolSize != 100 {
		t.Errorf("Worker.GeneralPoolSize = %d, want 100", cfg.Worker.GeneralPoolSize)
	}
	if cfg.Worker.K8sPoolSize != 50 {
		t.Errorf("Worker.K8sPoolSize = %d, want 50", cfg.Worker.K8sPoolSize)
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  DatabaseConfig
		want string
	}{
		{
			name: "URL takes precedence",
			cfg: DatabaseConfig{
				URL:  "postgres://user:pass@host:5432/db",
				Host: "other",
			},
			want: "postgres://user:pass@host:5432/db",
		},
		{
			name: "construct from fields",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "shepherd",
				Password: "secret",
				Database: "shepherd",
				SSLMode:  "disable",
			},
			want: "postgres://shepherd:secret@localhost:5432/shepherd?sslmode=disable",
		},
		{
			name: "default sslmode when empty",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Password: "pass",
				Database: "db",
			},
			want: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_DatabaseURLFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://shepherd:shepherd_password@db:5432/shepherd_db?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := "postgres://shepherd:shepherd_password@db:5432/shepherd_db?sslmode=disable"
	if cfg.Database.URL != want {
		t.Fatalf("Database.URL = %q, want %q", cfg.Database.URL, want)
	}
	if cfg.Database.DSN() != want {
		t.Fatalf("Database.DSN() = %q, want %q", cfg.Database.DSN(), want)
	}
}

func TestLoad_ServerCORSFlagsFromEnv(t *testing.T) {
	t.Setenv("SERVER_ALLOWED_ORIGINS", "https://example.com")
	t.Setenv("SERVER_ALLOW_CREDENTIALS", "false")
	t.Setenv("SERVER_UNSAFE_ALLOW_ALL_ORIGINS", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := len(cfg.Server.AllowedOrigins); got != 1 {
		t.Fatalf("len(Server.AllowedOrigins) = %d, want 1", got)
	}
	if got := cfg.Server.AllowedOrigins[0]; got != "https://example.com" {
		t.Fatalf("Server.AllowedOrigins[0] = %q, want %q", got, "https://example.com")
	}
	if cfg.Server.AllowCredentials {
		t.Fatalf("Server.AllowCredentials = %v, want false", cfg.Server.AllowCredentials)
	}
	if !cfg.Server.UnsafeAllowAllOrigins {
		t.Fatalf("Server.UnsafeAllowAllOrigins = %v, want true", cfg.Server.UnsafeAllowAllOrigins)
	}
}
