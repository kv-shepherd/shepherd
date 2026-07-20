package handlers

import (
	"context"
	"fmt"
	"strings"

	"kv-shepherd.io/shepherd/ent"
)

const (
	userMutationAdvisoryLockNamespace = "user-mutation"
	systemMembershipLockNamespace     = "system-membership"
)

func userMutationAdvisoryLockKey(userID string) string {
	normalizedUserID := strings.TrimSpace(userID)
	return fmt.Sprintf("%s:%d:%s", userMutationAdvisoryLockNamespace, len(normalizedUserID), normalizedUserID)
}

func lockUserMutation(ctx context.Context, tx *ent.Tx, userID string) error {
	normalizedUserID := strings.TrimSpace(userID)
	if normalizedUserID == "" {
		return fmt.Errorf("user id is required")
	}
	if tx == nil {
		return fmt.Errorf("ent transaction is required")
	}

	if err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(normalizedUserID),
	); err != nil {
		return fmt.Errorf("lock mutations for user %s: %w", normalizedUserID, err)
	}
	return nil
}

func systemMembershipAdvisoryLockKey(systemID string) string {
	normalizedSystemID := strings.TrimSpace(systemID)
	return fmt.Sprintf("%s:%d:%s", systemMembershipLockNamespace, len(normalizedSystemID), normalizedSystemID)
}

func lockSystemMembership(ctx context.Context, tx *ent.Tx, systemID string) error {
	normalizedSystemID := strings.TrimSpace(systemID)
	if normalizedSystemID == "" {
		return fmt.Errorf("system id is required")
	}
	if tx == nil {
		return fmt.Errorf("ent transaction is required")
	}

	if err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		systemMembershipAdvisoryLockKey(normalizedSystemID),
	); err != nil {
		return fmt.Errorf("lock membership for system %s: %w", normalizedSystemID, err)
	}
	return nil
}

func lockUserRow(ctx context.Context, tx *ent.Tx, userID string) error {
	if tx == nil {
		return fmt.Errorf("ent transaction is required")
	}
	if err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, strings.TrimSpace(userID)); err != nil {
		return fmt.Errorf("lock user row %s: %w", strings.TrimSpace(userID), err)
	}
	return nil
}

// lockUserRowForDeletion obtains a PostgreSQL row lock before associated-row
// cleanup. Foreign-key writers that started first commit before subsequent
// READ COMMITTED cleanup statements; writers that start later wait until the
// user deletion commits and then fail their FK check instead of racing cleanup.
func lockUserRowForDeletion(ctx context.Context, tx *ent.Tx, userID string) error {
	return lockUserRow(ctx, tx, userID)
}
