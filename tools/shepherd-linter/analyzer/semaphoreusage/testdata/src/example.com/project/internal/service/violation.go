// Package service contains a violation: Acquire without defer Release.
package service

import "context"

// BadFunc acquires but forgets to defer Release.
func BadFunc(ctx context.Context) error {
	if err := sem.Acquire(ctx, 1); err != nil { // want `calls Acquire\(\) without a paired defer Release\(\)`
		return err
	}
	// missing: defer sem.Release(1)
	return nil
}
