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

	// ContributeServerDeps injects module-owned dependencies into the HTTP server deps.
	ContributeServerDeps(*handlers.ServerDeps)

	// RegisterWorkers registers module workers into a shared River worker registry.
	RegisterWorkers(*river.Workers)

	// Shutdown performs module-local graceful cleanup.
	Shutdown(context.Context) error
}
