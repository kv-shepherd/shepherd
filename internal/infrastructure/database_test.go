package infrastructure

import (
	"testing"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/jobs"
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

func TestBuildRiverQueues_NormalizesVMQueuesToAtLeastOneWorker(t *testing.T) {
	t.Parallel()

	queues := buildRiverQueues(0)

	if got := queues["vm_operations"].MaxWorkers; got != 1 {
		t.Fatalf("vm_operations max workers = %d, want 1 when config is 0", got)
	}
	if got := queues[jobs.VMStatusSyncJobKind].MaxWorkers; got != 1 {
		t.Fatalf("%s max workers = %d, want 1 when config is 0", jobs.VMStatusSyncJobKind, got)
	}
}
