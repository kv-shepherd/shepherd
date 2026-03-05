// Package service contains clean semaphore usage with defer Release.
package service

import "context"

type semaphore struct{}

func (s *semaphore) Acquire(ctx context.Context, n int64) error { return nil }
func (s *semaphore) Release(n int64)                            {}

var sem = &semaphore{}

// GoodFunc acquires and defers release correctly.
func GoodFunc(ctx context.Context) error {
	if err := sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer sem.Release(1)
	return nil
}
