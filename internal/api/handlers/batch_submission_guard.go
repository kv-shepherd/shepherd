package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func lockBatchSubmissionTransaction(ctx context.Context, tx *ent.Tx) error {
	if tx == nil {
		return fmt.Errorf("batch submission transaction is required")
	}
	if err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		usecase.BatchSubmissionAdvisoryLockKey,
	); err != nil {
		return fmt.Errorf("lock batch submissions: %w", err)
	}
	return nil
}

func lockBatchSubmissionActor(ctx context.Context, tx pgx.Tx, actor string) error {
	if tx == nil {
		return fmt.Errorf("batch submission pgx transaction is required")
	}
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(actor),
	); err != nil {
		return fmt.Errorf("lock batch submission actor %s: %w", actor, err)
	}
	return nil
}

// acquireBatchSubmissionGuard is intentionally process-local. It keeps
// same-process waiters out of the database pool while the leader acquires the
// transaction-scoped global lock in its business transaction. Keeping the
// database lock in that transaction is required for atomic quota checks and
// avoids consuming a second backend under PgBouncer transaction pooling.
func (s *Server) acquireBatchSubmissionGuard(ctx context.Context) (release func(), err error) {
	if s == nil || s.batchSubmissionGate == nil {
		return nil, fmt.Errorf("batch submission gate is not initialized")
	}
	select {
	case s.batchSubmissionGate <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for local batch submission gate: %w", ctx.Err())
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			<-s.batchSubmissionGate
		})
	}, nil
}
