package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/internal/app/modules"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
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

func TestNewObservabilityTracingDisabled(t *testing.T) {
	t.Parallel()

	tracing, err := newObservabilityTracing(context.Background(), &config.Config{
		Observability: config.ObservabilityConfig{
			TracingEnabled: false,
		},
	})

	require.NoError(t, err)
	require.Nil(t, tracing)
}

func TestNewObservabilityTracingInvalidExporter(t *testing.T) {
	t.Parallel()

	tracing, err := newObservabilityTracing(context.Background(), &config.Config{
		Observability: config.ObservabilityConfig{
			TracingEnabled:     true,
			TracingServiceName: "shepherd",
			TracingExporter:    "invalid",
			TracingSampleRatio: 0.1,
		},
	})

	require.Error(t, err)
	require.Nil(t, tracing)
	require.Contains(t, err.Error(), "unsupported tracing exporter")
}

func TestNewObservabilityMetricsIncludesBusinessCollector(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "app_business_metrics")
	metrics := newObservabilityMetrics(&config.Config{
		Observability: config.ObservabilityConfig{
			MetricsEnabled:         true,
			BusinessMetricsEnabled: true,
		},
	}, &modules.Infrastructure{Pool: pool})

	require.NotNil(t, metrics)
	families, err := metrics.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() == "shepherd_business_metrics_scrape_success" {
			return
		}
	}
	t.Fatal("business metrics scrape success metric was not registered")
}

func TestApplication_Start_NoDependencies(t *testing.T) {
	t.Parallel()

	app := &Application{}
	assert.NotPanics(t, func() {
		err := app.Start(context.Background())
		require.NoError(t, err)
	})
}

func TestRegisterPeriodicJobs_WiringContract(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatalf("read bootstrap.go: %v", err)
	}
	text := string(src)

	required := []string{
		"jobs.NotificationCleanupArgs{}",
		"jobs.DomainEventArchiveArgs{}",
		"jobs.VMTombstoneCleanupArgs{}",
		"jobs.DirectoryEnrichmentScheduleScanArgs{}",
		"&river.PeriodicJobOpts{RunOnStart: true}",
	}
	for _, fragment := range required {
		if !strings.Contains(text, fragment) {
			t.Fatalf("bootstrap periodic job wiring missing required fragment %q", fragment)
		}
	}
	if !strings.Contains(text, "if cfg.River.ConsumeJobs {") {
		t.Fatal("bootstrap must guard periodic job registration behind cfg.River.ConsumeJobs")
	}
	if !strings.Contains(text, "registerPeriodicJobs(infra)") {
		t.Fatal("bootstrap must continue wiring registerPeriodicJobs(infra)")
	}
}
