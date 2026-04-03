package notification

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestTriggers_OnTicketSubmitted_NotifiesPlatformAdminsAndApprovalAdmins(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	client := testutil.OpenEntPostgres(t, "notification_triggers_ticket_submitted")
	ctx := t.Context()

	platformUser, err := client.User.Create().
		SetID("user-platform-" + uuid.NewString()).
		SetUsername("platform-admin").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed platform user: %v", err)
	}
	approvalUser, err := client.User.Create().
		SetID("user-approval-" + uuid.NewString()).
		SetUsername("approval-admin").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed approval user: %v", err)
	}
	viewerUser, err := client.User.Create().
		SetID("user-viewer-" + uuid.NewString()).
		SetUsername("viewer").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed viewer user: %v", err)
	}

	platformRole, err := client.Role.Create().
		SetID("role-platform-" + uuid.NewString()).
		SetName("PlatformAdmin").
		SetPermissions([]string{"platform:admin"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed platform role: %v", err)
	}
	approvalRole, err := client.Role.Create().
		SetID("role-approval-" + uuid.NewString()).
		SetName("ApprovalAdmin").
		SetPermissions([]string{"builtin_approval:approve"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed approval role: %v", err)
	}
	viewerRole, err := client.Role.Create().
		SetID("role-viewer-" + uuid.NewString()).
		SetName("Viewer").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed viewer role: %v", err)
	}

	for _, binding := range []struct {
		userID string
		roleID string
	}{
		{userID: platformUser.ID, roleID: platformRole.ID},
		{userID: approvalUser.ID, roleID: approvalRole.ID},
		{userID: viewerUser.ID, roleID: viewerRole.ID},
	} {
		if _, err := client.RoleBinding.Create().
			SetID("rb-" + uuid.NewString()).
			SetUserID(binding.userID).
			SetRoleID(binding.roleID).
			SetScopeType("global").
			SetCreatedBy("seed").
			Save(ctx); err != nil {
			t.Fatalf("seed role binding for %s: %v", binding.userID, err)
		}
	}

	sender := &captureSender{}
	triggers := NewTriggers(sender, client)
	triggers.OnTicketSubmitted(ctx, "ticket-1", "alice", "gtest1")

	if len(sender.recipients) != 2 {
		t.Fatalf("recipient count = %d, want 2", len(sender.recipients))
	}

	got := map[string]struct{}{}
	for _, recipient := range sender.recipients {
		got[recipient] = struct{}{}
	}
	if _, ok := got[platformUser.ID]; !ok {
		t.Fatalf("missing platform admin recipient: %+v", sender.recipients)
	}
	if _, ok := got[approvalUser.ID]; !ok {
		t.Fatalf("missing approval admin recipient: %+v", sender.recipients)
	}
	if _, ok := got[viewerUser.ID]; ok {
		t.Fatalf("viewer should not receive approval notification: %+v", sender.recipients)
	}
}
