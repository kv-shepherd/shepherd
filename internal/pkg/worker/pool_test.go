package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	// Initialize logger for tests
	_ = logger.Init("error", "json")
}

func TestNewPools(t *testing.T) {
	ctx := context.Background()
	pools, err := NewPools(ctx, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPools() error = %v", err)
	}
	defer pools.Shutdown()

	if pools.General == nil {
		t.Error("General pool is nil")
	}
	if pools.K8s == nil {
		t.Error("K8s pool is nil")
	}
}

func TestPool_Submit(t *testing.T) {
	ctx := context.Background()
	pools, err := NewPools(ctx, PoolConfig{
		GeneralPoolSize: 10,
		K8sPoolSize:     5,
	})
	if err != nil {
		t.Fatalf("NewPools() error = %v", err)
	}
	defer pools.Shutdown()

	var executed atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	err = pools.General.Submit(ctx, func(ctx context.Context) {
		executed.Store(true)
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	wg.Wait()
	if !executed.Load() {
		t.Error("Task was not executed")
	}
}

func TestPool_Submit_CancelledContext(t *testing.T) {
	ctx := context.Background()
	pools, err := NewPools(ctx, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPools() error = %v", err)
	}
	defer pools.Shutdown()

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	err = pools.General.Submit(cancelledCtx, func(ctx context.Context) {
		t.Error("Task should not execute with cancelled context")
	})
	if err != context.Canceled {
		t.Errorf("Submit() error = %v, want context.Canceled", err)
	}
}

// TestPools_SubmitDetached uses table-driven tests (Go best practice from go.dev/doc).
func TestPools_SubmitDetached(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
	}{
		{"general pool", "general"},
		{"k8s pool", "k8s"},
		{"default fallback", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pools, err := NewPools(ctx, DefaultPoolConfig())
			if err != nil {
				t.Fatalf("NewPools() error = %v", err)
			}

			var executed atomic.Bool
			var wg sync.WaitGroup
			wg.Add(1)

			err = pools.SubmitDetached(tt.poolName, func(ctx context.Context) {
				executed.Store(true)
				wg.Done()
			})
			if err != nil {
				t.Fatalf("SubmitDetached(%q) error = %v", tt.poolName, err)
			}

			wg.Wait()
			pools.Shutdown()

			if !executed.Load() {
				t.Errorf("SubmitDetached(%q) task was not executed", tt.poolName)
			}
		})
	}
}

func TestPools_Metrics(t *testing.T) {
	ctx := context.Background()
	pools, err := NewPools(ctx, PoolConfig{
		GeneralPoolSize: 10,
		K8sPoolSize:     5,
	})
	if err != nil {
		t.Fatalf("NewPools() error = %v", err)
	}
	defer pools.Shutdown()

	metrics := pools.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() returned nil")
	}

	general, ok := metrics["general"].(map[string]int)
	if !ok {
		t.Fatal("general metrics not found or wrong type")
	}
	if general["cap"] != 10 {
		t.Errorf("general cap = %d, want 10", general["cap"])
	}

	k8s, ok := metrics["k8s"].(map[string]int)
	if !ok {
		t.Fatal("k8s metrics not found or wrong type")
	}
	if k8s["cap"] != 5 {
		t.Errorf("k8s cap = %d, want 5", k8s["cap"])
	}
}

func TestPool_Submit_ContextCancelledWhileQueued(t *testing.T) {
	ctx := context.Background()
	pools, err := NewPools(ctx, PoolConfig{
		GeneralPoolSize: 1,
		K8sPoolSize:     1,
	})
	if err != nil {
		t.Fatalf("NewPools() error = %v", err)
	}
	defer pools.Shutdown()

	// Fill the pool with a blocking task
	blockCh := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	_ = pools.General.Submit(ctx, func(ctx context.Context) {
		wg.Done()
		<-blockCh // Block until released
	})
	wg.Wait() // Wait for blocking task to start

	// Submit a task with a context that will be cancelled
	cancelCtx, cancel := context.WithCancel(ctx)

	var taskExecuted atomic.Bool
	var submitWg sync.WaitGroup
	submitWg.Add(1)
	go func() { //nolint:naked-goroutine // test helper
		defer submitWg.Done()
		_ = pools.General.Submit(cancelCtx, func(ctx context.Context) {
			taskExecuted.Store(true)
		})
	}()

	// Give the task time to be queued, then cancel context
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Release the blocking task
	close(blockCh)
	submitWg.Wait()

	// The task may or may not execute depending on timing,
	// but it should not panic
}
