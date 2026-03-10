package testutil

import (
	"context"
	"fmt"
	"time"
)

func waitForPostgresReady(ctx context.Context, ping func(context.Context) error) error {
	const (
		deadline = 5 * time.Second
		interval = 100 * time.Millisecond
	)

	waitCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	var lastErr error
	for {
		lastErr = ping(waitCtx)
		if lastErr == nil {
			return nil
		}
		if waitCtx.Err() != nil {
			return fmt.Errorf("postgres not ready within %s: %w", deadline, lastErr)
		}
		select {
		case <-time.After(interval):
		case <-waitCtx.Done():
			return fmt.Errorf("postgres not ready within %s: %w", deadline, lastErr)
		}
	}
}
