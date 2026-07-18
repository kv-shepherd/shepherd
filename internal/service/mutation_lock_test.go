package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordedMutationLockCall struct {
	args []any
}

type recordingMutationLockExecutor struct {
	calls     []recordedMutationLockCall
	failAt    int
	execError error
}

func (e *recordingMutationLockExecutor) ExecContext(_ context.Context, _ string, args ...any) error {
	e.calls = append(e.calls, recordedMutationLockCall{args: args})
	if e.execError != nil && len(e.calls) == e.failAt {
		return e.execError
	}
	return nil
}

func TestLockAuthProviderMutationValidatesAndWrapsExecution(t *testing.T) {
	t.Parallel()

	if err := LockAuthProviderMutation(t.Context(), nil, "provider-1"); err == nil || !strings.Contains(err.Error(), "transaction is required") {
		t.Fatalf("nil executor error = %v, want transaction validation error", err)
	}
	exec := &recordingMutationLockExecutor{}
	if err := LockAuthProviderMutation(t.Context(), exec, "  "); err == nil || !strings.Contains(err.Error(), "provider id is required") {
		t.Fatalf("blank provider error = %v, want provider validation error", err)
	}

	lockErr := errors.New("lock unavailable")
	exec = &recordingMutationLockExecutor{failAt: 1, execError: lockErr}
	if err := LockAuthProviderMutation(t.Context(), exec, " provider-1 "); !errors.Is(err, lockErr) || !strings.Contains(err.Error(), `provider-1`) {
		t.Fatalf("execution error = %v, want wrapped lock error", err)
	}
	if len(exec.calls) != 1 || !reflect.DeepEqual(exec.calls[0].args, []any{"provider-1"}) {
		t.Fatalf("lock call = %#v, want one call for trimmed provider", exec.calls)
	}

	exec = &recordingMutationLockExecutor{}
	if err := LockAuthProviderMutation(t.Context(), exec, "provider-2"); err != nil {
		t.Fatalf("LockAuthProviderMutation() error = %v", err)
	}
	if len(exec.calls) != 1 || !reflect.DeepEqual(exec.calls[0].args, []any{"provider-2"}) {
		t.Fatalf("successful lock call = %#v, want one call for provider-2", exec.calls)
	}
}

func TestLockRoleAssignmentValidatesAndWrapsExecution(t *testing.T) {
	t.Parallel()

	if err := LockRoleAssignment(t.Context(), nil, "role-1"); err == nil || !strings.Contains(err.Error(), "transaction is required") {
		t.Fatalf("nil executor error = %v, want transaction validation error", err)
	}
	exec := &recordingMutationLockExecutor{}
	if err := LockRoleAssignment(t.Context(), exec, "\t"); err == nil || !strings.Contains(err.Error(), "role id is required") {
		t.Fatalf("blank role error = %v, want role validation error", err)
	}

	lockErr := errors.New("row lock failed")
	exec = &recordingMutationLockExecutor{failAt: 1, execError: lockErr}
	if err := LockRoleAssignment(t.Context(), exec, " role-1 "); !errors.Is(err, lockErr) || !strings.Contains(err.Error(), "role-1") {
		t.Fatalf("execution error = %v, want wrapped row lock error", err)
	}
	if len(exec.calls) != 1 || !reflect.DeepEqual(exec.calls[0].args, []any{"role-1"}) {
		t.Fatalf("role lock call = %#v, want trimmed role row lock", exec.calls)
	}
}

func TestLockRoleAssignmentsUsesStableCompactOrderAndStopsOnFailure(t *testing.T) {
	t.Parallel()

	exec := &recordingMutationLockExecutor{}
	if err := LockRoleAssignments(t.Context(), exec, []string{" role-b ", "", "role-a", "role-b", " role-c"}); err != nil {
		t.Fatalf("LockRoleAssignments() error = %v", err)
	}
	got := make([]string, 0, len(exec.calls))
	for _, call := range exec.calls {
		got = append(got, call.args[0].(string))
	}
	if want := []string{"role-a", "role-b", "role-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lock order = %v, want %v", got, want)
	}

	lockErr := errors.New("second lock failed")
	exec = &recordingMutationLockExecutor{failAt: 2, execError: lockErr}
	if err := LockRoleAssignments(t.Context(), exec, []string{"role-c", "role-a", "role-b"}); !errors.Is(err, lockErr) {
		t.Fatalf("LockRoleAssignments() error = %v, want wrapped failure", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("calls after failure = %d, want 2", len(exec.calls))
	}
}

func TestLockRoleBindingUserValidatesAndWrapsExecution(t *testing.T) {
	t.Parallel()

	if err := LockRoleBindingUser(t.Context(), nil, "user-1"); err == nil || !strings.Contains(err.Error(), "transaction is required") {
		t.Fatalf("nil executor error = %v, want transaction validation error", err)
	}
	exec := &recordingMutationLockExecutor{}
	if err := LockRoleBindingUser(t.Context(), exec, " "); err == nil || !strings.Contains(err.Error(), "user id is required") {
		t.Fatalf("blank user error = %v, want user validation error", err)
	}

	lockErr := errors.New("user row lock failed")
	exec = &recordingMutationLockExecutor{failAt: 1, execError: lockErr}
	if err := LockRoleBindingUser(t.Context(), exec, " user-1 "); !errors.Is(err, lockErr) || !strings.Contains(err.Error(), "user-1") {
		t.Fatalf("execution error = %v, want wrapped user row lock error", err)
	}
	if len(exec.calls) != 1 || !reflect.DeepEqual(exec.calls[0].args, []any{"user-1"}) {
		t.Fatalf("user lock call = %#v, want trimmed user row lock", exec.calls)
	}
}

func TestLockRoleBindingUsersUsesStableCompactOrderAndStopsOnFailure(t *testing.T) {
	t.Parallel()

	exec := &recordingMutationLockExecutor{}
	if err := LockRoleBindingUsers(t.Context(), exec, []string{" user-b ", "", "user-a", "user-b", " user-c"}); err != nil {
		t.Fatalf("LockRoleBindingUsers() error = %v", err)
	}
	got := make([]string, 0, len(exec.calls))
	for _, call := range exec.calls {
		got = append(got, call.args[0].(string))
	}
	if want := []string{"user-a", "user-b", "user-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lock order = %v, want %v", got, want)
	}

	lockErr := errors.New("second user lock failed")
	exec = &recordingMutationLockExecutor{failAt: 2, execError: lockErr}
	if err := LockRoleBindingUsers(t.Context(), exec, []string{"user-c", "user-a", "user-b"}); !errors.Is(err, lockErr) {
		t.Fatalf("LockRoleBindingUsers() error = %v, want wrapped failure", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("calls after failure = %d, want 2", len(exec.calls))
	}
}
