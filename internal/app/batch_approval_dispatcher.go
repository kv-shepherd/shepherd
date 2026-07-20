package app

import (
	"context"
	"fmt"
	"sync"

	"kv-shepherd.io/shepherd/internal/jobs"
)

// deferredBatchApprovalDispatcher breaks the composition-time cycle between
// River client creation and the ticket service's transactional River writer.
// The worker is registered before NewClient, and Bootstrap installs the target
// before Application.Start can consume jobs.
type deferredBatchApprovalDispatcher struct {
	mu     sync.RWMutex
	target jobs.BatchApprovalDispatcher
}

func (d *deferredBatchApprovalDispatcher) setTarget(target jobs.BatchApprovalDispatcher) error {
	if target == nil {
		return fmt.Errorf("batch approval dispatcher target is nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.target != nil {
		return fmt.Errorf("batch approval dispatcher target is already configured")
	}
	d.target = target
	return nil
}

func (d *deferredBatchApprovalDispatcher) loadTarget() (jobs.BatchApprovalDispatcher, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.target == nil {
		return nil, fmt.Errorf("batch approval dispatcher target is not configured")
	}
	return d.target, nil
}

func (d *deferredBatchApprovalDispatcher) DispatchBatchApproval(ctx context.Context, parentID string) error {
	target, err := d.loadTarget()
	if err != nil {
		return err
	}
	return target.DispatchBatchApproval(ctx, parentID)
}

func (d *deferredBatchApprovalDispatcher) FailPendingBatchApprovalDispatch(
	ctx context.Context,
	parentID string,
	cause error,
) error {
	target, err := d.loadTarget()
	if err != nil {
		return err
	}
	return target.FailPendingBatchApprovalDispatch(ctx, parentID, cause)
}
