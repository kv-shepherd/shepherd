package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestSystemMemberMutationFailuresPreserveMembershipState(t *testing.T) {
	t.Parallel()

	t.Run("duplicate add", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		const (
			systemID = "system-duplicate-member"
			userID   = "user-duplicate-member"
		)
		mustCreateSystem(t, client, systemID, "dup-member", "seed")
		if _, err := client.User.Create().
			SetID(userID).
			SetUsername("duplicate.member").
			SetEnabled(true).
			Save(t.Context()); err != nil {
			t.Fatalf("seed duplicate member user: %v", err)
		}
		mustCreateSystemBinding(t, client, userID, systemID, resourcerolebinding.RoleAdmin.String())

		c, w := newAuthedGinContext(
			t,
			http.MethodPost,
			"/systems/"+systemID+"/members",
			`{"user_id":"`+userID+`","role":"viewer"}`,
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.AddSystemMember(c, systemID)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "MEMBER_ALREADY_EXISTS")
		assertSystemMemberRole(t, client, systemID, userID, resourcerolebinding.RoleAdmin)
	})

	t.Run("promote disabled user", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		const (
			systemID = "system-disabled-owner"
			userID   = "user-disabled-owner"
		)
		mustCreateSystem(t, client, systemID, "disabled", "seed")
		if _, err := client.User.Create().
			SetID(userID).
			SetUsername("disabled.owner").
			SetEnabled(false).
			Save(t.Context()); err != nil {
			t.Fatalf("seed disabled member user: %v", err)
		}
		mustCreateSystemBinding(t, client, userID, systemID, resourcerolebinding.RoleViewer.String())

		c, w := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/systems/"+systemID+"/members/"+userID,
			`{"role":"owner"}`,
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.UpdateSystemMemberRole(c, systemID, userID)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "USER_DISABLED")
		assertSystemMemberRole(t, client, systemID, userID, resourcerolebinding.RoleViewer)
	})

	t.Run("update absent member", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		const systemID = "system-update-absent-member"
		mustCreateSystem(t, client, systemID, "upd-missing", "seed")

		c, w := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/systems/"+systemID+"/members/user-absent",
			`{"role":"viewer"}`,
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.UpdateSystemMemberRole(c, systemID, "user-absent")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "MEMBER_NOT_FOUND")
		if count, err := client.ResourceRoleBinding.Query().Count(t.Context()); err != nil || count != 0 {
			t.Fatalf("membership count = %d/%v, want 0", count, err)
		}
	})

	t.Run("delete absent member", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		const systemID = "system-delete-absent-member"
		mustCreateSystem(t, client, systemID, "del-missing", "seed")

		c, w := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/systems/"+systemID+"/members/user-absent",
			"",
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.DeleteSystemMember(c, systemID, "user-absent")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "MEMBER_NOT_FOUND")
		if count, err := client.ResourceRoleBinding.Query().Count(t.Context()); err != nil || count != 0 {
			t.Fatalf("membership count = %d/%v, want 0", count, err)
		}
	})
}

func TestDeleteSystemMemberPersistsAuditAndFinalMembershipState(t *testing.T) {
	t.Parallel()

	client, pool := testutil.OpenEntPostgresWithPool(t, "delete_member_audit")
	srv := NewServer(ServerDeps{
		EntClient: client,
		Pool:      pool,
		Audit:     audit.NewLogger(client),
	})
	const (
		systemID = "system-delete-member-audit"
		userID   = "user-delete-member-audit"
	)
	mustCreateSystem(t, client, systemID, "member-audit", "seed")
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("delete.member.audit").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed audited member user: %v", err)
	}
	mustCreateSystemBinding(t, client, userID, systemID, resourcerolebinding.RoleViewer.String())

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemID+"/members/"+userID,
		"",
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	srv.DeleteSystemMember(c, systemID, userID)
	if got := c.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", got, http.StatusNoContent, w.Body.String())
	}
	if count, err := client.ResourceRoleBinding.Query().Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("membership count after delete = %d/%v, want 0", count, err)
	}
	auditRow, err := client.AuditLog.Query().
		Where(
			auditlog.ActionEQ("system.member.remove"),
			auditlog.ResourceTypeEQ("system"),
			auditlog.ResourceIDEQ(systemID),
			auditlog.ActorEQ("platform-admin"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("load member removal audit: %v", err)
	}
	if auditRow.Details["user_id"] != userID || auditRow.Details["role"] != resourcerolebinding.RoleViewer.String() {
		t.Fatalf("member removal audit details = %#v", auditRow.Details)
	}
}
