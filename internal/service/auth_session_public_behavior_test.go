package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type failingAuthSessionExecutor struct {
	err error
}

func (e *failingAuthSessionExecutor) ExecContext(context.Context, string, ...any) error {
	return e.err
}

func TestAuthSessionManagerExportedMethodsFailClosedAndValidateInputs(t *testing.T) {
	t.Parallel()

	var unavailable *service.AuthSessionManager
	if err := unavailable.EnsureSchema(t.Context()); !errors.Is(err, service.ErrAuthSessionStoreUnavailable) {
		t.Fatalf("EnsureSchema() error = %v, want store unavailable", err)
	}
	if err := unavailable.ActivateUserSession(t.Context(), "user-1", 1); !errors.Is(err, service.ErrAuthSessionStoreUnavailable) {
		t.Fatalf("ActivateUserSession() error = %v, want store unavailable", err)
	}
	if err := unavailable.RevokeUsersSessionsTx(t.Context(), &failingAuthSessionExecutor{}, []string{"user-1"}, "test"); !errors.Is(err, service.ErrAuthSessionStoreUnavailable) {
		t.Fatalf("RevokeUsersSessionsTx() error = %v, want store unavailable", err)
	}
	if service.NewAuthSessionManager(nil, nil, 0) != nil {
		t.Fatal("NewAuthSessionManager(nil, nil) returned a usable manager")
	}

	client, pool := testutil.OpenEntPostgresWithPool(t, "auth_session_public_behavior")
	manager := service.NewAuthSessionManager(pool, client, 0)
	if manager == nil {
		t.Fatal("NewAuthSessionManager() returned nil for valid dependencies")
	}
	if err := manager.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema(nil) error = %v", err)
	}
	if err := manager.ActivateUserSession(t.Context(), " ", 1); !errors.Is(err, service.ErrAuthSessionUserIDMissing) {
		t.Fatalf("ActivateUserSession(blank user) error = %v, want missing user", err)
	}
	if err := manager.ActivateUserSession(t.Context(), "user-1", 0); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("ActivateUserSession(invalid version) error = %v", err)
	}
	if err := manager.RevokeUsersSessionsTx(t.Context(), nil, []string{"user-1"}, "test"); err == nil || !strings.Contains(err.Error(), "executor is required") {
		t.Fatalf("RevokeUsersSessionsTx(nil executor) error = %v", err)
	}
	if err := manager.RevokeUsersSessionsTx(t.Context(), &failingAuthSessionExecutor{}, []string{" ", ""}, "test"); err != nil {
		t.Fatalf("RevokeUsersSessionsTx(empty users) error = %v", err)
	}
	execErr := errors.New("executor unavailable")
	if err := manager.RevokeUsersSessionsTx(
		t.Context(),
		&failingAuthSessionExecutor{err: execErr},
		[]string{"user-2", "user-1"},
		"test",
	); !errors.Is(err, execErr) {
		t.Fatalf("RevokeUsersSessionsTx(executor failure) error = %v, want wrapped executor error", err)
	}

	uninitialized := service.NewAuthSessionManager(pool, client, 0)
	if err := uninitialized.RevokeUsersSessionsTx(t.Context(), &failingAuthSessionExecutor{}, []string{"user-1"}, "test"); err == nil || !strings.Contains(err.Error(), "schema is not initialized") {
		t.Fatalf("RevokeUsersSessionsTx(uninitialized schema) error = %v", err)
	}

	closedPool := testutil.OpenPGXPool(t, "auth_session_public_closed_pool")
	closedManager := service.NewAuthSessionManager(closedPool, client, 0)
	closedPool.Close()
	if err := closedManager.ActivateUserSession(t.Context(), "user-1", 1); err == nil || !strings.Contains(err.Error(), "closed pool") {
		t.Fatalf("ActivateUserSession(closed store) error = %v", err)
	}

	initializedPool := testutil.OpenPGXPool(t, "auth_session_public_initialized_closed_pool")
	initializedManager := service.NewAuthSessionManager(initializedPool, client, 0)
	if err := initializedManager.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("initialize manager before closing store: %v", err)
	}
	initializedPool.Close()
	if err := initializedManager.ActivateUserSession(t.Context(), "user-1", 1); err == nil || !strings.Contains(err.Error(), "activate auth session subject") {
		t.Fatalf("ActivateUserSession(closed initialized store) error = %v", err)
	}
}

func TestAuthSessionManagerConcurrentBatchRevocationsPreserveEveryVersion(t *testing.T) {
	t.Parallel()

	client, pool := testutil.OpenEntPostgresWithPool(t, "auth_session_public_concurrent")
	manager := service.NewAuthSessionManager(pool, client, 0)
	if manager == nil {
		t.Fatal("NewAuthSessionManager() returned nil for valid dependencies")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := manager.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	userIDs := []string{"auth-session-concurrent-first", "auth-session-concurrent-second"}
	initialVersions := make(map[string]int64, len(userIDs))
	for _, userID := range userIDs {
		version, err := manager.CurrentSessionVersion(ctx, userID)
		if err != nil {
			t.Fatalf("CurrentSessionVersion(%q) error = %v", userID, err)
		}
		initialVersions[userID] = version
	}

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	revoke := func(ids []string) error {
		ready <- struct{}{}
		select {
		case <-start:
		case <-ctx.Done():
			return ctx.Err()
		}
		return manager.RevokeUsersSessions(ctx, ids, "concurrent_batch")
	}
	var revocations errgroup.Group
	revocations.Go(func() error { return revoke([]string{userIDs[1], userIDs[0]}) })
	revocations.Go(func() error { return revoke([]string{userIDs[0], userIDs[1]}) })
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatalf("prepare concurrent revocations: %v", ctx.Err())
		}
	}
	close(start)
	if err := revocations.Wait(); err != nil {
		t.Fatalf("concurrent RevokeUsersSessions() error = %v", err)
	}

	for _, userID := range userIDs {
		version, err := manager.CurrentSessionVersion(ctx, userID)
		if err != nil {
			t.Fatalf("CurrentSessionVersion(%q) after concurrent revocations error = %v", userID, err)
		}
		if want := initialVersions[userID] + 2; version != want {
			t.Fatalf("CurrentSessionVersion(%q) = %d, want %d", userID, version, want)
		}
	}
}
