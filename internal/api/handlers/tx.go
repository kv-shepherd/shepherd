package handlers

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
)

// WithTx scopes an Ent transaction to the handler layer.
func WithTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	if client == nil {
		return fmt.Errorf("ent client is required")
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}
