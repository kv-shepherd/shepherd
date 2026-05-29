package infrastructure

import (
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/observability"
)

func TestBuildRiverQueues_ConfiguredWorkers(t *testing.T) {
	t.Parallel()

	queues := buildRiverQueues(4)

	if got := queues[river.QueueDefault].MaxWorkers; got != 4 {
		t.Fatalf("default queue max workers = %d, want 4", got)
	}
	if got := queues["vm_operations"].MaxWorkers; got != 4 {
		t.Fatalf("vm_operations max workers = %d, want 4", got)
	}
	if got := queues[jobs.VMStatusSyncJobKind].MaxWorkers; got != 4 {
		t.Fatalf("%s max workers = %d, want 4", jobs.VMStatusSyncJobKind, got)
	}
}

func TestBuildRiverQueues_UsesConfiguredWorkersForAllQueues(t *testing.T) {
	t.Parallel()

	queues := buildRiverQueues(1)

	if got := queues[river.QueueDefault].MaxWorkers; got != 1 {
		t.Fatalf("default queue max workers = %d, want 1", got)
	}
	if got := queues["vm_operations"].MaxWorkers; got != 1 {
		t.Fatalf("vm_operations max workers = %d, want 1", got)
	}
	if got := queues[jobs.VMStatusSyncJobKind].MaxWorkers; got != 1 {
		t.Fatalf("%s max workers = %d, want 1", jobs.VMStatusSyncJobKind, got)
	}
}

func TestInitRiverClient_RejectsInvalidMaxWorkers(t *testing.T) {
	t.Parallel()

	var clients DatabaseClients
	err := clients.InitRiverClient(river.NewWorkers(), config.RiverConfig{MaxWorkers: 0})
	if err == nil {
		t.Fatalf("InitRiverClient() error = nil, want invalid max workers error")
	}
}

func TestBuildRiverMiddlewareIncludesWorkerLogMiddleware(t *testing.T) {
	t.Parallel()

	middleware := buildRiverMiddleware()
	if len(middleware) != 1 {
		t.Fatalf("middleware count = %d, want 1", len(middleware))
	}
	if _, ok := middleware[0].(*observability.RiverWorkerLogMiddleware); !ok {
		t.Fatalf("middleware[0] = %T, want RiverWorkerLogMiddleware", middleware[0])
	}
	if _, ok := middleware[0].(rivertype.JobInsertMiddleware); !ok {
		t.Fatalf("middleware[0] = %T, want JobInsertMiddleware", middleware[0])
	}
	if _, ok := middleware[0].(rivertype.WorkerMiddleware); !ok {
		t.Fatalf("middleware[0] = %T, want WorkerMiddleware", middleware[0])
	}
}
