// Package worker provides goroutine pool management.
//
// Coding Standard (ADR-0031): Naked goroutines are forbidden.
// All concurrency must go through Worker Pool with context propagation.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/pkg/worker
package worker

import (
	"context"
	"errors"
	"time"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ErrPoolClosed is returned when submitting to a closed pool.
var ErrPoolClosed = errors.New("worker pool is closed")

// Task is a context-aware task function (ADR-0031 Rule 2).
type Task func(ctx context.Context)

// Pool wraps ants.Pool with context-aware submission (ADR-0031 Rule 2).
type Pool struct {
	pool *ants.Pool
	name string
}

// Pools is the Worker pool collection.
type Pools struct {
	General *Pool
	K8s     *Pool

	// serviceCtx is the service lifecycle context for detached tasks
	serviceCtx    context.Context
	serviceCancel context.CancelFunc
}

// PoolConfig contains Worker Pool configuration.
type PoolConfig struct {
	GeneralPoolSize int
	K8sPoolSize     int
}

// DefaultPoolConfig returns default configuration.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		GeneralPoolSize: 100,
		K8sPoolSize:     50,
	}
}

// NewPools creates Worker pool collection.
func NewPools(ctx context.Context, cfg PoolConfig) (*Pools, error) {
	// Create service lifecycle context for detached tasks
	serviceCtx, serviceCancel := context.WithCancel(ctx)

	// Unified panic recovery
	panicHandler := func(p interface{}) {
		logger.Error("Worker panic recovered",
			zap.Any("panic", p),
			zap.Stack("stack"),
		)
	}

	generalAnts, err := ants.NewPool(cfg.GeneralPoolSize,
		ants.WithPanicHandler(panicHandler),
		ants.WithNonblocking(false),
		ants.WithExpiryDuration(10*time.Second), // Purge idle workers (ants best practice)
	)
	if err != nil {
		serviceCancel()
		return nil, err
	}

	k8sAnts, err := ants.NewPool(cfg.K8sPoolSize,
		ants.WithPanicHandler(panicHandler),
		ants.WithNonblocking(false),
		ants.WithExpiryDuration(30*time.Second), // K8s tasks are longer-lived
	)
	if err != nil {
		generalAnts.Release()
		serviceCancel()
		return nil, err
	}

	return &Pools{
		General:       &Pool{pool: generalAnts, name: "general"},
		K8s:           &Pool{pool: k8sAnts, name: "k8s"},
		serviceCtx:    serviceCtx,
		serviceCancel: serviceCancel,
	}, nil
}

// Submit submits a context-aware task (ADR-0031 Rule 2).
// The task receives the caller's context and SHOULD check ctx.Done() at blocking points.
// If context is already cancelled, returns ctx.Err() immediately without submitting.
func (p *Pool) Submit(ctx context.Context, task Task) error {
	// Fast path: check if context is already cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return p.pool.Submit(func() {
		// Check context again inside worker (may have been cancelled while queued)
		select {
		case <-ctx.Done():
			logger.Debug("Task skipped: context cancelled",
				zap.String("pool", p.name),
				zap.Error(ctx.Err()),
			)
			return
		default:
		}
		task(ctx)
	})
}

// SubmitDetached submits a detached background task (ADR-0031 Rule 2).
// Detached tasks use the service lifecycle context instead of a request context.
// Use this for long-running background work that should survive request cancellation
// but still respect graceful shutdown.
func (p *Pools) SubmitDetached(poolName string, task Task) error {
	var pool *Pool
	switch poolName {
	case "general":
		pool = p.General
	case "k8s":
		pool = p.K8s
	default:
		pool = p.General
	}

	return pool.pool.Submit(func() {
		// Check service context
		select {
		case <-p.serviceCtx.Done():
			logger.Debug("Detached task skipped: service shutting down",
				zap.String("pool", poolName),
			)
			return
		default:
		}
		task(p.serviceCtx)
	})
}

// Shutdown gracefully shuts down all pools with a timeout.
// Cancels service context first, then waits for running tasks (max 30s).
func (p *Pools) Shutdown() {
	// Signal all detached tasks to stop
	p.serviceCancel()

	// Release pools with timeout (ants best practice: avoid infinite wait)
	const shutdownTimeout = 30 * time.Second
	if err := p.General.pool.ReleaseTimeout(shutdownTimeout); err != nil {
		logger.Warn("General pool shutdown timeout", zap.Error(err))
	}
	if err := p.K8s.pool.ReleaseTimeout(shutdownTimeout); err != nil {
		logger.Warn("K8s pool shutdown timeout", zap.Error(err))
	}
}

// Metrics returns pool metrics for observability.
func (p *Pools) Metrics() map[string]interface{} {
	return map[string]interface{}{
		"general": map[string]int{
			"running": p.General.pool.Running(),
			"free":    p.General.pool.Free(),
			"cap":     p.General.pool.Cap(),
		},
		"k8s": map[string]int{
			"running": p.K8s.pool.Running(),
			"free":    p.K8s.pool.Free(),
			"cap":     p.K8s.pool.Cap(),
		},
	}
}
