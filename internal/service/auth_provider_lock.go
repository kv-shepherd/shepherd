package service

import (
	"context"
	"fmt"
	"strings"
)

// AuthProviderMutationExecutor is the narrow transaction contract required to
// serialize writes that depend on an authentication provider. Implementations
// must execute the statement in the same PostgreSQL transaction as the
// dependent writes.
type AuthProviderMutationExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) error
}

// LockAuthProviderMutation serializes provider administration, JIT
// provisioning, cohort mapping reconciliation, and directory synchronization
// for one provider within the current PostgreSQL schema. The transaction-scoped
// advisory lock is released automatically on commit or rollback.
func LockAuthProviderMutation(ctx context.Context, exec AuthProviderMutationExecutor, providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if exec == nil {
		return fmt.Errorf("auth provider mutation transaction is required")
	}
	if providerID == "" {
		return fmt.Errorf("auth provider id is required")
	}
	if err := exec.ExecContext(ctx, `
SELECT pg_advisory_xact_lock(
  hashtextextended(current_schema() || ':auth_provider:' || $1, 0)
)
`, providerID); err != nil {
		return fmt.Errorf("lock auth provider mutation %q: %w", providerID, err)
	}
	return nil
}
