package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/jobs"
)

type recordingBatchApprovalDispatcher struct {
	dispatchParent string
	finalizeParent string
	finalizeCause  error
}

func (d *recordingBatchApprovalDispatcher) DispatchBatchApproval(_ context.Context, parentID string) error {
	d.dispatchParent = parentID
	return nil
}

func (d *recordingBatchApprovalDispatcher) FailPendingBatchApprovalDispatch(
	_ context.Context,
	parentID string,
	cause error,
) error {
	d.finalizeParent = parentID
	d.finalizeCause = cause
	return nil
}

func TestDeferredBatchApprovalDispatcherFailsClosedUntilConfiguredAndRoutesAfterward(t *testing.T) {
	deferred := &deferredBatchApprovalDispatcher{}
	cause := errors.New("dispatch exhausted")

	if err := deferred.DispatchBatchApproval(t.Context(), "batch-before-target"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("DispatchBatchApproval() before target error = %v, want fail-closed", err)
	}
	if err := deferred.FailPendingBatchApprovalDispatch(t.Context(), "batch-before-target", cause); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("FailPendingBatchApprovalDispatch() before target error = %v, want fail-closed", err)
	}
	if err := deferred.setTarget(nil); err == nil {
		t.Fatal("setTarget(nil) error = nil, want fail-closed")
	}

	target := &recordingBatchApprovalDispatcher{}
	if err := deferred.setTarget(target); err != nil {
		t.Fatalf("setTarget() error = %v", err)
	}
	if err := deferred.setTarget(&recordingBatchApprovalDispatcher{}); err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("second setTarget() error = %v, want immutable target", err)
	}
	if err := deferred.DispatchBatchApproval(t.Context(), "batch-dispatch"); err != nil {
		t.Fatalf("DispatchBatchApproval() routed error = %v", err)
	}
	if err := deferred.FailPendingBatchApprovalDispatch(t.Context(), "batch-finalize", cause); err != nil {
		t.Fatalf("FailPendingBatchApprovalDispatch() routed error = %v", err)
	}
	if target.dispatchParent != "batch-dispatch" || target.finalizeParent != "batch-finalize" || !errors.Is(target.finalizeCause, cause) {
		t.Fatalf("routed dispatcher calls = %+v, want dispatch/finalize inputs", target)
	}
}

func TestBootstrapRegistersBatchApprovalWorkerBeforeRiverInitialization(t *testing.T) {
	source, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatalf("read bootstrap composition root: %v", err)
	}
	text := string(source)
	fragments := []string{
		"batchApprovalDispatcher := &deferredBatchApprovalDispatcher{}",
		"river.AddWorker(workers, jobs.NewBatchApprovalDispatchWorker(batchApprovalDispatcher))",
		"infra.InitRiver(workers)",
		"batchApprovalDispatcher.setTarget(approvalModule.TicketService())",
	}
	previous := -1
	for _, fragment := range fragments {
		position := strings.Index(text, fragment)
		if position < 0 {
			t.Fatalf("bootstrap batch dispatcher wiring missing %q", fragment)
		}
		if position <= previous {
			t.Fatalf("bootstrap batch dispatcher wiring out of order at %q", fragment)
		}
		previous = position
	}
}

var _ jobs.BatchApprovalDispatcher = (*recordingBatchApprovalDispatcher)(nil)
