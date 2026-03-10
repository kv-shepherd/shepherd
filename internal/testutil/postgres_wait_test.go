package testutil

import (
	"context"
	"errors"
	"testing"
)

func TestWaitForPostgresReady_ReturnsImmediatelyOnSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	err := waitForPostgresReady(context.Background(), func(context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("waitForPostgresReady() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("waitForPostgresReady() calls = %d, want 1", calls)
	}
}

func TestWaitForPostgresReady_RespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForPostgresReady(ctx, func(context.Context) error {
		return errors.New("still booting")
	})
	if err == nil {
		t.Fatal("waitForPostgresReady() error = nil, want timeout/cancel error")
	}
}
