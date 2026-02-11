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
