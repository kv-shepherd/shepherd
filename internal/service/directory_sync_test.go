package service

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
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

func TestDirectorySyncServiceApplyRecord_ReconcilesObservedCohortsAndBindings(t *testing.T) {
	t.Parallel()

	client := newDirectorySyncTestClient(t)
	svc := NewDirectorySyncService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-approver-1").
		SetName("Approver").
		SetPermissions([]string{"ticket:approve"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if _, createMappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-directory-1").
		SetProviderID("provider-directory").
		SetCohortKind("department").
		SetCohortKey("2955").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); createMappingErr != nil {
		t.Fatalf("create cohort mapping: %v", createMappingErr)
	}

	result, conflicts, err := svc.ApplyRecord(t.Context(), "provider-directory", directorycontract.DirectoryUserRecord{
		ExternalID:  "alice@example.com",
		Username:    "alice@example.com",
		DisplayName: "Alice Example",
		Email:       "alice@example.com",
		Attributes: map[string]interface{}{
			"department": "Finance",
			"section":    "Planning",
		},
		Cohorts: []directorycontract.ExternalCohort{
			{Kind: "department", Key: "2955", DisplayName: "Finance"},
		},
	}, DirectoryConflictResolutionSkip)
	if err != nil {
		t.Fatalf("ApplyRecord() error = %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts len = %d, want 0", len(conflicts))
	}
	if result.Action != directorycontract.DirectoryActionCreate {
		t.Fatalf("action = %q, want %q", result.Action, directorycontract.DirectoryActionCreate)
	}

	createdUser, err := client.User.Query().
		Where(user.UsernameEQ("alice@example.com")).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query created user: %v", err)
	}

	bindings, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(user.IDEQ(createdUser.ID)),
			rolebinding.HasRoleWith(role.IDEQ(roleEnt.ID)),
		).
		All(t.Context())
	if err != nil {
		t.Fatalf("query role bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("role binding count = %d, want 1", len(bindings))
	}

	grants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(createdUser.ID)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query external cohort grants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("external cohort grant count = %d, want 1", len(grants))
	}

	observed, err := client.ExternalCohort.Query().
		Where(
			externalcohort.ProviderIDEQ("provider-directory"),
			externalcohort.CohortKindEQ("department"),
			externalcohort.CohortKeyEQ("2955"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query observed external cohort: %v", err)
	}
	if observed.DisplayName != "Finance" {
		t.Fatalf("observed cohort display_name = %q, want %q", observed.DisplayName, "Finance")
	}

	profile, err := client.UserDirectoryProfile.Query().
		Where(userdirectoryprofile.UserIDEQ(createdUser.ID)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query directory profile: %v", err)
	}
	if got := profile.Attributes["department"]; got != "Finance" {
		t.Fatalf("profile department = %#v, want %q", got, "Finance")
	}
	if _, ok := profile.Attributes["external_cohorts"]; !ok {
		t.Fatal("profile external_cohorts missing")
	}
}
