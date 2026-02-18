// Package modules contains domain-oriented dependency modules for ADR-0022.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/app/modules
package modules

import (
	"context"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
)

// Module represents a domain-specific dependency unit in the composition root.
type Module interface {
	// Name returns a stable module identifier for logging/debugging.
	Name() string

	// Shutdown performs module-local graceful cleanup.
	Shutdown(context.Context) error
}

// ServerDepsContributor is implemented by modules that need to inject HTTP server deps.
type ServerDepsContributor interface {
	ContributeServerDeps(*handlers.ServerDeps)
}

// WorkerRegistrar is implemented by modules that register River workers.
type WorkerRegistrar interface {
	RegisterWorkers(*river.Workers)
}
