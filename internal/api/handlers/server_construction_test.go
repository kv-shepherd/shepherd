package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestNewServer_DoesNotConstructApprovalRequirementsFallback(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "server_new_no_approval_fallback")
	srv := NewServer(ServerDeps{EntClient: client})

	if srv.approvalReqs != nil {
		t.Fatal("NewServer() should not construct ApprovalRequirementService implicitly")
	}
}

func TestNewServer_PreservesInjectedApprovalRequirements(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "server_new_preserve_approval_reqs")
	reqs := service.NewApprovalRequirementService(client)

	srv := NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: reqs,
	})

	if srv.approvalReqs != reqs {
		t.Fatal("NewServer() should preserve injected ApprovalRequirementService")
	}
}

func TestNewServer_DoesNotConstructDirectorySyncFallback(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "server_new_no_directory_sync_fallback")
	srv := NewServer(ServerDeps{EntClient: client})

	if srv.directorySync != nil {
		t.Fatal("NewServer() should not construct DirectorySyncService implicitly")
	}
}
