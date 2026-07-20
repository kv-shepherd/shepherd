package ent

import (
	"context"
	"fmt"

	entsql "entgo.io/ent/dialect/sql"
)

// ExecContext executes raw SQL through the transaction's Ent driver. It is
// intentionally narrow so application code can include auxiliary tables that
// are not modeled by Ent in the same database transaction.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) error {
	if tx == nil || tx.driver == nil {
		return fmt.Errorf("ent transaction is required")
	}
	return tx.driver.Exec(ctx, query, args, nil)
}

// QueryIntContext executes a scalar integer query through the same Ent
// transaction. It is intentionally narrow for consistency checks against
// auxiliary tables (for example River job state) that are not Ent schemas.
func (tx *Tx) QueryIntContext(ctx context.Context, query string, args ...any) (int, error) {
	if tx == nil || tx.driver == nil {
		return 0, fmt.Errorf("ent transaction is required")
	}
	rows := &entsql.Rows{}
	if err := tx.driver.Query(ctx, query, args, rows); err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("scalar integer query returned no rows")
	}
	var value int
	if err := rows.Scan(&value); err != nil {
		return 0, err
	}
	if rows.Next() {
		return 0, fmt.Errorf("scalar integer query returned multiple rows")
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return value, nil
}
