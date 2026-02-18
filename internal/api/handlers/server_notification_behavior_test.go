package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestNotificationHandler_ListNotifications_UserScopedAndUnreadFilter(t *testing.T) {
	t.Parallel()

	srv, client := newNotificationBehaviorTestServer(t)
	now := time.Now().UTC()

	mustCreateUser(t, client, "user-1", "user.one")
	mustCreateUser(t, client, "user-2", "user.two")
	mustCreateNotification(t, client, "n-1", "user-1", false, now.Add(-3*time.Hour))
	mustCreateNotification(t, client, "n-2", "user-1", true, now.Add(-2*time.Hour))
	mustCreateNotification(t, client, "n-3", "user-2", false, now.Add(-1*time.Hour))

	{
		c, w := newAuthedGinContext(t, http.MethodGet, "/notifications", "", "user-1", nil)
		srv.ListNotifications(c, generated.ListNotificationsParams{
			Page:       1,
			PerPage:    20,
			UnreadOnly: true,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp generated.NotificationList
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode unread-only response: %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("unread-only items len = %d, want 1", len(resp.Items))
		}
		if resp.Items[0].Id != "n-1" {
			t.Fatalf("unread-only item id = %q, want %q", resp.Items[0].Id, "n-1")
		}
		if resp.Pagination.Total != 1 {
			t.Fatalf("unread-only total = %d, want 1", resp.Pagination.Total)
		}
	}

	{
		c, w := newAuthedGinContext(t, http.MethodGet, "/notifications", "", "user-1", nil)
		srv.ListNotifications(c, generated.ListNotificationsParams{
			Page:    1,
			PerPage: 20,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp generated.NotificationList
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode all response: %v", err)
		}
		if len(resp.Items) != 2 {
			t.Fatalf("all items len = %d, want 2", len(resp.Items))
		}
		if resp.Pagination.Total != 2 {
			t.Fatalf("all total = %d, want 2", resp.Pagination.Total)
		}
		if resp.Items[0].Id != "n-2" || resp.Items[1].Id != "n-1" {
			t.Fatalf("unexpected order/items: got [%s, %s], want [n-2, n-1]", resp.Items[0].Id, resp.Items[1].Id)
		}
	}
}

func TestNotificationHandler_GetUnreadCount_UserScoped(t *testing.T) {
	t.Parallel()

	srv, client := newNotificationBehaviorTestServer(t)
	now := time.Now().UTC()

	mustCreateUser(t, client, "user-1", "user.one")
	mustCreateUser(t, client, "user-2", "user.two")
	mustCreateNotification(t, client, "n-1", "user-1", false, now.Add(-3*time.Hour))
	mustCreateNotification(t, client, "n-2", "user-1", true, now.Add(-2*time.Hour))
	mustCreateNotification(t, client, "n-3", "user-2", false, now.Add(-1*time.Hour))

	c, w := newAuthedGinContext(t, http.MethodGet, "/notifications/unread-count", "", "user-1", nil)
	srv.GetUnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.UnreadCount
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode unread count: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("unread count = %d, want 1", resp.Count)
	}
}

func TestNotificationHandler_MarkNotificationRead_UserScoped(t *testing.T) {
	t.Parallel()

	srv, client := newNotificationBehaviorTestServer(t)
	now := time.Now().UTC()

	mustCreateUser(t, client, "user-1", "user.one")
	mustCreateUser(t, client, "user-2", "user.two")
	mustCreateNotification(t, client, "n-own", "user-1", false, now.Add(-2*time.Hour))
	mustCreateNotification(t, client, "n-other", "user-2", false, now.Add(-1*time.Hour))

	{
		c, w := newAuthedGinContext(t, http.MethodPatch, "/notifications/n-own/read", "", "user-1", nil)
		srv.MarkNotificationRead(c, "n-own")
		if got := c.Writer.Status(); got != http.StatusNoContent {
			t.Fatalf("status = %d, want %d body=%s", got, http.StatusNoContent, w.Body.String())
		}

		obj, err := client.Notification.Get(t.Context(), "n-own")
		if err != nil {
			t.Fatalf("query notification: %v", err)
		}
		if !obj.Read {
			t.Fatal("notification read = false, want true")
		}
		if obj.ReadAt == nil {
			t.Fatal("notification read_at = nil, want non-nil")
		}
	}

	{
		c, w := newAuthedGinContext(t, http.MethodPatch, "/notifications/n-other/read", "", "user-1", nil)
		srv.MarkNotificationRead(c, "n-other")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
		}
	}
}

func TestNotificationHandler_MarkAllNotificationsRead_UserScoped(t *testing.T) {
	t.Parallel()

	srv, client := newNotificationBehaviorTestServer(t)
	now := time.Now().UTC()

	mustCreateUser(t, client, "user-1", "user.one")
	mustCreateUser(t, client, "user-2", "user.two")
	mustCreateNotification(t, client, "n-1", "user-1", false, now.Add(-3*time.Hour))
	mustCreateNotification(t, client, "n-2", "user-1", false, now.Add(-2*time.Hour))
	mustCreateNotification(t, client, "n-3", "user-2", false, now.Add(-1*time.Hour))

	c, w := newAuthedGinContext(t, http.MethodPost, "/notifications/mark-all-read", "", "user-1", nil)
	srv.MarkAllNotificationsRead(c)
	if got := c.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", got, http.StatusNoContent, w.Body.String())
	}

	user1Unread, err := client.Notification.Query().
		Where(
			entnotification.HasUserWith(entuser.IDEQ("user-1")),
			entnotification.ReadEQ(false),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count user-1 unread: %v", err)
	}
	if user1Unread != 0 {
		t.Fatalf("user-1 unread = %d, want 0", user1Unread)
	}

	user2Unread, err := client.Notification.Query().
		Where(
			entnotification.HasUserWith(entuser.IDEQ("user-2")),
			entnotification.ReadEQ(false),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count user-2 unread: %v", err)
	}
	if user2Unread != 1 {
		t.Fatalf("user-2 unread = %d, want 1", user2Unread)
	}
}

func TestNotificationHandler_Unauthorized(t *testing.T) {
	t.Parallel()

	srv, _ := newNotificationBehaviorTestServer(t)

	tests := []struct {
		name   string
		method string
		path   string
		run    func(c *gin.Context)
	}{
		{
			name:   "list notifications",
			method: http.MethodGet,
			path:   "/notifications",
			run: func(c *gin.Context) {
				srv.ListNotifications(c, generated.ListNotificationsParams{})
			},
		},
		{
			name:   "unread count",
			method: http.MethodGet,
			path:   "/notifications/unread-count",
			run: func(c *gin.Context) {
				srv.GetUnreadCount(c)
			},
		},
		{
			name:   "mark one read",
			method: http.MethodPatch,
			path:   "/notifications/n-1/read",
			run: func(c *gin.Context) {
				srv.MarkNotificationRead(c, "n-1")
			},
		},
		{
			name:   "mark all read",
			method: http.MethodPost,
			path:   "/notifications/mark-all-read",
			run: func(c *gin.Context) {
				srv.MarkAllNotificationsRead(c)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, w := newAuthedGinContext(t, tc.method, tc.path, "", "", nil)
			tc.run(c)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
			}
			assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
		})
	}
}

func newNotificationBehaviorTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	client := testutil.OpenEntPostgres(t, "notification_handler_behavior")
	return NewServer(ServerDeps{EntClient: client}), client
}

func mustCreateUser(t *testing.T, client *ent.Client, id, username string) *ent.User {
	t.Helper()
	obj, err := client.User.Create().
		SetID(id).
		SetUsername(username).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return obj
}

func mustCreateNotification(t *testing.T, client *ent.Client, id, userID string, read bool, createdAt time.Time) *ent.Notification {
	t.Helper()
	builder := client.Notification.Create().
		SetID(id).
		SetType(entnotification.TypeAPPROVAL_PENDING).
		SetTitle("title-" + id).
		SetMessage("message-" + id).
		SetUserID(userID).
		SetCreatedAt(createdAt).
		SetRead(read)
	if read {
		readAt := createdAt.Add(5 * time.Minute)
		builder = builder.SetReadAt(readAt)
	}
	obj, err := builder.Save(t.Context())
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	return obj
}
