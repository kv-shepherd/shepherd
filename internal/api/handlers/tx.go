package handlers

import (
	"context"
	"database/sql"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
)

// WithTx scopes an Ent transaction to the handler layer.
func WithTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	if client == nil {
		return fmt.Errorf("ent client is required")
	}

	// Lock-based handlers re-read authorization and referential state after
	// acquiring advisory or row locks. Pin READ COMMITTED explicitly so an
	// operator-level default such as REPEATABLE READ cannot preserve a snapshot
	// taken before a contended lock was acquired.
	tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
	}()

	if callbackErr := fn(tx); callbackErr != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w: rollback transaction: %w", callbackErr, rollbackErr)
		}
		return callbackErr
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
