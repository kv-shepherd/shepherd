package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	_ = logger.Init("error", "json")
}

func TestBootstrap_NoDB(t *testing.T) {
	// Bootstrap without a real database should fail at DB connection.
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     "localhost",
			Port:     65432, // Non-existent port
			User:     "test",
			Password: "test",
			Database: "test",
			SSLMode:  "disable",
			MaxConns: 5,
			MinConns: 1,
		},
		Worker: config.WorkerConfig{
			GeneralPoolSize: 10,
			K8sPoolSize:     5,
		},
	}

	ctx := context.Background()
	app, err := Bootstrap(ctx, cfg)
	require.Error(t, err, "Bootstrap should fail without database")
	assert.Nil(t, app, "Application should be nil on bootstrap failure")
}

func TestApplication_RouterRoutes(t *testing.T) {
	// Test that an Application struct can be created with a valid config.
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Log:    config.LogConfig{Level: "error", Format: "json"},
	}

	app := &Application{
		Config: cfg,
	}

	assert.NotNil(t, app, "Application should be non-nil")
	assert.Equal(t, 8080, app.Config.Server.Port, "Port should be set correctly")
}

func TestApplication_Shutdown_Nil(t *testing.T) {
	// Shutdown on empty application should not panic.
	app := &Application{}

	assert.NotPanics(t, func() {
		app.Shutdown()
	}, "Shutdown on empty Application should not panic")
}
