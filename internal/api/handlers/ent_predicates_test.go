package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/predicate"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestEntJSONPredicateHelpersMatchPostgresRows(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "ent_json_predicates_"+uuid.NewString()[:8])
	ctx := t.Context()

	userID := "user-" + uuid.NewString()
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("json-predicate-user-" + uuid.NewString()[:8]).
		SetDisplayName("JSON Predicate User").
		SetEmail("json-predicate-" + uuid.NewString()[:8] + "@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("create user: %v", err)
	}
	quotedField := "quoted'field"
	if _, err := client.UserDirectoryProfile.Create().
		SetID("profile-" + uuid.NewString()).
		SetUserID(userID).
		SetAttributes(map[string]interface{}{
			"department": "Platform Engineering",
			quotedField:  "Escaped Value",
		}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		t.Fatalf("create user directory profile: %v", err)
	}

	users, err := client.User.Query().Where(userProfileAttributeContains("department", "platform")).All(ctx)
	if err != nil {
		t.Fatalf("query user profile contains: %v", err)
	}
	if len(users) != 1 || users[0].ID != userID {
		t.Fatalf("user profile contains returned %+v, want only %s", users, userID)
	}

	users, err = client.User.Query().Where(userProfileAttributeEqualFold("department", "platform engineering")).All(ctx)
	if err != nil {
		t.Fatalf("query user profile equal fold: %v", err)
	}
	if len(users) != 1 || users[0].ID != userID {
		t.Fatalf("user profile equal fold returned %+v, want only %s", users, userID)
	}

	users, err = client.User.Query().Where(userProfileAttributeEqualFold(quotedField, "escaped value")).All(ctx)
	if err != nil {
		t.Fatalf("query user profile quoted field: %v", err)
	}
	if len(users) != 1 || users[0].ID != userID {
		t.Fatalf("user profile quoted field returned %+v, want only %s", users, userID)
	}

	count, err := client.User.Query().Where(impossibleUserSearchPredicate()).Count(ctx)
	if err != nil {
		t.Fatalf("query impossible predicate: %v", err)
	}
	if count != 0 {
		t.Fatalf("impossible predicate count = %d, want 0", count)
	}

	ticketID := "ticket-" + uuid.NewString()
	if _, createErr := client.Ticket.Create().
		SetID(ticketID).
		SetEventID("event-" + uuid.NewString()).
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("requester-1").
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_name": "Production East",
			"advisory_code":         "POLICY_REVIEW",
		}).
		Save(ctx); createErr != nil {
		t.Fatalf("create ticket: %v", createErr)
	}

	tickets, err := client.Ticket.Query().Where(ticketPlacementSelectedClusterNameContains("East")).All(ctx)
	if err != nil {
		t.Fatalf("query ticket selected cluster name: %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != ticketID {
		t.Fatalf("ticket cluster-name predicate returned %+v, want only %s", tickets, ticketID)
	}

	tickets, err = client.Ticket.Query().Where(ticketPlacementAdvisoryCodeEquals("POLICY_REVIEW")).All(ctx)
	if err != nil {
		t.Fatalf("query ticket advisory code: %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != ticketID {
		t.Fatalf("ticket advisory-code predicate returned %+v, want only %s", tickets, ticketID)
	}

	auditID := "audit-" + uuid.NewString()
	if _, createErr := client.AuditLog.Create().
		SetID(auditID).
		SetAction("approval.validation_failed").
		SetResourceType("ticket").
		SetResourceID(ticketID).
		SetActor("admin-1").
		SetDetails(map[string]interface{}{
			"decision": "validation_failed",
			"placement_evaluation": map[string]interface{}{
				"reason_code":   "VALIDATION_FAILED",
				"advisory_code": "POLICY_REVIEW",
			},
		}).
		Save(ctx); createErr != nil {
		t.Fatalf("create audit log: %v", createErr)
	}

	assertAuditPredicateMatchesOnly(t, client, auditDetailsDecisionEquals("validation_failed"), auditID)
	assertAuditPredicateMatchesOnly(t, client, auditDetailsPlacementReasonCodeEquals("VALIDATION_FAILED"), auditID)
	assertAuditPredicateMatchesOnly(t, client, auditDetailsPlacementAdvisoryCodeEquals("POLICY_REVIEW"), auditID)
}

func assertAuditPredicateMatchesOnly(t *testing.T, client *ent.Client, pred predicate.AuditLog, wantID string) {
	t.Helper()

	rows, err := client.AuditLog.Query().Where(pred).All(t.Context())
	if err != nil {
		t.Fatalf("query audit predicate: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != wantID {
		t.Fatalf("audit predicate returned %+v, want only %s", rows, wantID)
	}
}
