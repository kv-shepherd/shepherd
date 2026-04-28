package serverbootstrap

import (
	"context"
	"errors"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/app"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	_ = logger.Init("error", "json")
}

func TestRunWithConfig_BootstrapError(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	signalCh := make(chan os.Signal, 1)

	err := runWithConfig(context.Background(), cfg, signalCh, func(context.Context, *config.Config) (*app.Application, error) {
		return nil, errors.New("boom")
	}, func(*http.Server, chan<- error) {
		t.Fatal("serve should not be called on bootstrap error")
	})

	if err == nil || err.Error() != "bootstrap: boom" {
		t.Fatalf("expected bootstrap error, got %v", err)
	}
}

func TestRunWithConfig_ServerError(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	signalCh := make(chan os.Signal, 1)

	err := runWithConfig(context.Background(), cfg, signalCh, func(context.Context, *config.Config) (*app.Application, error) {
		return &app.Application{Config: cfg, Router: gin.New()}, nil
	}, func(_ *http.Server, errCh chan<- error) {
		errCh <- errors.New("listen boom")
		close(errCh)
	})

	if err == nil || err.Error() != "server error: listen boom" {
		t.Fatalf("expected server error, got %v", err)
	}
}

func TestRunWithConfig_ShutdownSignal(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	signalCh := make(chan os.Signal, 1)
	signalCh <- syscall.SIGTERM

	err := runWithConfig(context.Background(), cfg, signalCh, func(context.Context, *config.Config) (*app.Application, error) {
		return &app.Application{Config: cfg, Router: gin.New()}, nil
	}, func(_ *http.Server, errCh chan<- error) {
	})

	if err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestNormalizeStartupMigrationConfig_PrefersVersionedMigrations(t *testing.T) {
	t.Parallel()

	database := config.DatabaseConfig{
		AutoMigrate:                  true,
		AutoApplyVersionedMigrations: true,
	}
	normalizeStartupMigrationConfig(&database)

	if database.AutoMigrate {
		t.Fatal("AutoMigrate = true, want false when versioned migrations are enabled")
	}
	if !database.AutoApplyVersionedMigrations {
		t.Fatal("AutoApplyVersionedMigrations = false, want true")
	}
}

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     50 * time.Millisecond,
			WriteTimeout:    50 * time.Millisecond,
			ShutdownTimeout: 50 * time.Millisecond,
		},
		Log: config.LogConfig{
			Level:  "error",
			Format: "json",
		},
	}
}
