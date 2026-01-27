// Package worker provides goroutine pool management.
//
// Coding Standard (Required): Naked goroutines are forbidden.
// All concurrency must go through Worker Pool.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/pkg/worker
package worker

import (
	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// PoolConfig contains Worker Pool configuration.
type PoolConfig struct {
	// GeneralPoolSize is the size of the general task pool
	GeneralPoolSize int `mapstructure:"general_pool_size"`

	// K8sPoolSize is the size of the K8s operation pool (additional semaphore limiting)
	K8sPoolSize int `mapstructure:"k8s_pool_size"`
}

// DefaultPoolConfig returns default configuration.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		GeneralPoolSize: 100,
		K8sPoolSize:     50,
	}
}

// Pools is the Worker pool collection.
type Pools struct {
	General *ants.Pool
	K8s     *ants.Pool
}

// NewPools creates Worker pool collection.
func NewPools(cfg PoolConfig) (*Pools, error) {
	// Unified panic recovery
	panicHandler := func(p interface{}) {
		logger.Error("Worker panic recovered",
			zap.Any("panic", p),
			zap.Stack("stack"),
		)
	}

	general, err := ants.NewPool(cfg.GeneralPoolSize,
		ants.WithPanicHandler(panicHandler),
		ants.WithNonblocking(false),
	)
	if err != nil {
		return nil, err
	}

	k8sPool, err := ants.NewPool(cfg.K8sPoolSize,
		ants.WithPanicHandler(panicHandler),
		ants.WithNonblocking(false),
	)
	if err != nil {
		general.Release()
		return nil, err
	}

	return &Pools{
		General: general,
		K8s:     k8sPool,
	}, nil
}

// Shutdown gracefully shuts down all pools.
func (p *Pools) Shutdown() {
	p.General.Release()
	p.K8s.Release()
}

// Metrics returns pool metrics for observability.
func (p *Pools) Metrics() map[string]interface{} {
	return map[string]interface{}{
		"general": map[string]int{
			"running": p.General.Running(),
			"free":    p.General.Free(),
			"cap":     p.General.Cap(),
		},
		"k8s": map[string]int{
			"running": p.K8s.Running(),
			"free":    p.K8s.Free(),
			"cap":     p.K8s.Cap(),
		},
	}
}

// Usage Examples:
//
// ❌ Forbidden: naked goroutine
// go func() {
//     result, err := someOperation()
//     // No panic recovery
//     // No concurrency control
//     // No metrics
// }()
//
// ✅ Correct: use Worker Pool
// pools.General.Submit(func() {
//     result, err := someOperation()
//     if err != nil {
//         logger.Error("Operation failed", zap.Error(err))
//     }
//     // Automatic panic recovery
//     // Controlled concurrency
//     // Observable via Metrics()
// })
