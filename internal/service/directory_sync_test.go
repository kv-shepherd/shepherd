package service

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func newDirectorySyncTestClient(t *testing.T) *ent.Client {
	t.Helper()
	return testutil.OpenEntPostgres(t, "directory_sync_service")
}

func TestDirectorySyncServicePreviewMatchCreate(t *testing.T) {
	t.Parallel()

	client := newDirectorySyncTestClient(t)
	svc := NewDirectorySyncService(client)

	preview, err := svc.Preview(t.Context(), "provider-1", []directorycontract.DirectoryUserRecord{
		{
			ExternalID:  "ext-create-1",
			Username:    "fresh-user",
			DisplayName: "Fresh User",
			Email:       "fresh@example.com",
		},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(preview.Items))
	}
	if preview.Items[0].Match.Action != directorycontract.DirectoryActionCreate {
		t.Fatalf("match.action = %q, want %q", preview.Items[0].Match.Action, directorycontract.DirectoryActionCreate)
	}
}

func TestDirectorySyncServicePreviewMatchUpdate(t *testing.T) {
	t.Parallel()

	client := newDirectorySyncTestClient(t)
	svc := NewDirectorySyncService(client)

	existingUser, err := client.User.Create().
		SetID("existing-update-user").
		SetUsername("managed-user").
		SetDisplayName("Managed User").
		SetEmail("managed@example.com").
		SetAuthProviderID("provider-1").
		SetExternalID("ext-update-1").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	preview, err := svc.Preview(t.Context(), "provider-1", []directorycontract.DirectoryUserRecord{
		{
			ExternalID:  "ext-update-1",
			Username:    "managed-user",
			DisplayName: "Managed User",
			Email:       "managed@example.com",
		},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(preview.Items))
	}
	if preview.Items[0].Match.Action != directorycontract.DirectoryActionUpdate {
		t.Fatalf("match.action = %q, want %q", preview.Items[0].Match.Action, directorycontract.DirectoryActionUpdate)
	}
	if preview.Items[0].Match.ExistingUserID != existingUser.ID {
		t.Fatalf("existing_user_id = %q, want %q", preview.Items[0].Match.ExistingUserID, existingUser.ID)
	}
	if preview.Items[0].Match.MatchedBy != directorycontract.DirectoryPreviewMatchByExternalID {
		t.Fatalf("matched_by = %q, want %q", preview.Items[0].Match.MatchedBy, directorycontract.DirectoryPreviewMatchByExternalID)
	}
}

func TestDirectorySyncServicePreviewMatchBlocked(t *testing.T) {
	t.Parallel()

	client := newDirectorySyncTestClient(t)
	svc := NewDirectorySyncService(client)

	if _, err := client.User.Create().
		SetID("existing-blocked-user").
		SetUsername("blocked-user").
		SetDisplayName("Blocked User").
		SetEmail("blocked@example.com").
		SetAuthProviderID("other-provider").
		SetExternalID("ext-other-1").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	preview, err := svc.Preview(t.Context(), "provider-1", []directorycontract.DirectoryUserRecord{
		{
			ExternalID:  "ext-blocked-1",
			Username:    "blocked-user",
			DisplayName: "Blocked User",
			Email:       "blocked@example.com",
		},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(preview.Items))
	}
	if preview.Items[0].Match.Action != directorycontract.DirectoryActionBlocked {
		t.Fatalf("match.action = %q, want %q", preview.Items[0].Match.Action, directorycontract.DirectoryActionBlocked)
	}
}
