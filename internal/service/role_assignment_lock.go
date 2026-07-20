package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// RoleAssignmentExecutor executes role locks in the transaction that owns the
// dependent RoleBinding or ExternalCohortMapping mutation.
type RoleAssignmentExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) error
}

// LockRoleAssignment locks one role row until the caller's transaction ends.
// Writers must reload the role after this call because a missing row does not
// make SELECT ... FOR UPDATE fail when executed without scanning a result.
func LockRoleAssignment(ctx context.Context, exec RoleAssignmentExecutor, roleID string) error {
	roleID = strings.TrimSpace(roleID)
	if exec == nil {
		return fmt.Errorf("role assignment transaction is required")
	}
	if roleID == "" {
		return fmt.Errorf("role id is required")
	}
	if err := exec.ExecContext(ctx, `SELECT id FROM roles WHERE id = $1 FOR UPDATE`, roleID); err != nil {
		return fmt.Errorf("lock role row %s: %w", roleID, err)
	}
	return nil
}

// LockRoleAssignments locks a set of roles in stable order. Stable ordering is
// required because two provider transactions can reconcile overlapping role
// sets concurrently.
func LockRoleAssignments(ctx context.Context, exec RoleAssignmentExecutor, roleIDs []string) error {
	normalized := make([]string, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if roleID = strings.TrimSpace(roleID); roleID != "" {
			normalized = append(normalized, roleID)
		}
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	for _, roleID := range normalized {
		if err := LockRoleAssignment(ctx, exec, roleID); err != nil {
			return err
		}
	}
	return nil
}

// LockRoleBindingUser locks the user row before any role row used by a managed
// RoleBinding write. Keeping the global order user -> role prevents a manual
// binding writer and an external-cohort reconciliation from waiting on each
// other's foreign-key locks.
func LockRoleBindingUser(ctx context.Context, exec RoleAssignmentExecutor, userID string) error {
	userID = strings.TrimSpace(userID)
	if exec == nil {
		return fmt.Errorf("role assignment transaction is required")
	}
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if err := exec.ExecContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return fmt.Errorf("lock role binding user row %s: %w", userID, err)
	}
	return nil
}

// LockRoleBindingUsers locks user rows in stable order before role rows.
func LockRoleBindingUsers(ctx context.Context, exec RoleAssignmentExecutor, userIDs []string) error {
	normalized := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID = strings.TrimSpace(userID); userID != "" {
			normalized = append(normalized, userID)
		}
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	for _, userID := range normalized {
		if err := LockRoleBindingUser(ctx, exec, userID); err != nil {
			return err
		}
	}
	return nil
}
