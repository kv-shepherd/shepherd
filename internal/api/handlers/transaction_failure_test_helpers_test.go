package handlers

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/internal/service"
)

func installAuthSessionVersionBumpFailure(
	t *testing.T,
	srv *Server,
	authSessions *service.AuthSessionManager,
	userIDs ...string,
) map[string]int64 {
	t.Helper()
	if srv.pool == nil {
		t.Fatal("test server pool is nil")
	}
	if authSessions == nil {
		t.Fatal("test auth session manager is nil")
	}

	versions := make(map[string]int64, len(userIDs))
	for _, userID := range userIDs {
		version, err := authSessions.CurrentSessionVersion(t.Context(), userID)
		if err != nil {
			t.Fatalf("seed auth session version for %q: %v", userID, err)
		}
		if err := authSessions.ActivateUserSession(t.Context(), userID, version); err != nil {
			t.Fatalf("seed auth session subject for %q: %v", userID, err)
		}
		versions[userID] = version
	}

	// Sequence increments are not rolled back, so is_called proves the handler
	// reached the trigger even though the surrounding transaction is aborted.
	if _, err := srv.pool.Exec(t.Context(), `
CREATE SEQUENCE auth_session_version_failure_witness;
CREATE FUNCTION reject_auth_session_version_bump() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM nextval('auth_session_version_failure_witness');
  RAISE EXCEPTION 'forced auth session version bump failure';
END;
$$;
CREATE TRIGGER reject_auth_session_version_bump
BEFORE UPDATE OF session_version ON auth_session_subjects
FOR EACH ROW
EXECUTE FUNCTION reject_auth_session_version_bump();
`); err != nil {
		t.Fatalf("install auth session version failure trigger: %v", err)
	}
	return versions
}

func assertAuthSessionVersionBumpFailureTriggered(t *testing.T, srv *Server) {
	t.Helper()
	var triggered bool
	if err := srv.pool.QueryRow(t.Context(), `
SELECT is_called FROM auth_session_version_failure_witness
`).Scan(&triggered); err != nil {
		t.Fatalf("read auth session version failure witness: %v", err)
	}
	if !triggered {
		t.Fatal("auth session version failure trigger was not reached")
	}
}

func assertAuthSessionVersionsUnchanged(
	t *testing.T,
	authSessions *service.AuthSessionManager,
	want map[string]int64,
) {
	t.Helper()
	for userID, wantVersion := range want {
		gotVersion, err := authSessions.CurrentSessionVersion(t.Context(), userID)
		if err != nil {
			t.Fatalf("read auth session version for %q after rollback: %v", userID, err)
		}
		if gotVersion != wantVersion {
			t.Fatalf(
				"auth session version for %q after rollback = %d, want unchanged at %d",
				userID,
				gotVersion,
				wantVersion,
			)
		}
	}
}

func installVMPowerRiverInsertFailure(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if pool == nil {
		t.Fatal("test pool is nil")
	}
	// Keep a non-transactional witness so an unrelated earlier 500 cannot make
	// the rollback test pass without exercising River's transactional insert.
	if _, err := pool.Exec(t.Context(), `
CREATE SEQUENCE vm_power_river_failure_witness;
CREATE FUNCTION reject_vm_power_river_job_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM nextval('vm_power_river_failure_witness');
  RAISE EXCEPTION 'forced vm_power River insert failure';
END;
$$;
CREATE TRIGGER reject_vm_power_river_job_insert
BEFORE INSERT ON river_job
FOR EACH ROW
WHEN (NEW.kind = 'vm_power')
EXECUTE FUNCTION reject_vm_power_river_job_insert();
`); err != nil {
		t.Fatalf("install vm_power River insert failure trigger: %v", err)
	}
}

func assertVMPowerRiverInsertFailureTriggered(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var triggered bool
	if err := pool.QueryRow(t.Context(), `
SELECT is_called FROM vm_power_river_failure_witness
`).Scan(&triggered); err != nil {
		t.Fatalf("read vm_power River failure witness: %v", err)
	}
	if !triggered {
		t.Fatal("vm_power River insert failure trigger was not reached")
	}
}
