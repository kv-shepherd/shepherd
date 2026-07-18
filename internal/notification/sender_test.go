package notification

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type captureSender struct {
	lastSend       Params
	lastSendToMany Params
	recipients     []string
	sendErr        error
	sendToManyErr  error
}

func (s *captureSender) Send(_ context.Context, params Params) error {
	s.lastSend = params
	return s.sendErr
}

func (s *captureSender) SendToMany(_ context.Context, recipientIDs []string, params Params) error {
	s.recipients = append([]string(nil), recipientIDs...)
	s.lastSendToMany = params
	return s.sendToManyErr
}

func TestValidateParams_RequiresMandatoryFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{
			name: "recipient",
			params: Params{
				Title:      "t",
				TitleKey:   TitleKeyApprovalPending,
				Message:    "m",
				MessageKey: MessageKeyApprovalPending,
			},
			want: "recipient_id is required",
		},
		{
			name: "title",
			params: Params{
				RecipientID: "u-1",
				TitleKey:    TitleKeyApprovalPending,
				Message:     "m",
				MessageKey:  MessageKeyApprovalPending,
			},
			want: "title is required",
		},
		{
			name: "title_key",
			params: Params{
				RecipientID: "u-1",
				Title:       "t",
				Message:     "m",
				MessageKey:  MessageKeyApprovalPending,
			},
			want: "title_key is required",
		},
		{
			name: "message",
			params: Params{
				RecipientID: "u-1",
				Title:       "t",
				TitleKey:    TitleKeyApprovalPending,
				MessageKey:  MessageKeyApprovalPending,
			},
			want: "message is required",
		},
		{
			name: "message_key",
			params: Params{
				RecipientID: "u-1",
				Title:       "t",
				TitleKey:    TitleKeyApprovalPending,
				Message:     "m",
			},
			want: "message_key is required",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateParams(tc.params)
			if err == nil {
				t.Fatal("validateParams() expected error")
			}
			if err.Error() != tc.want {
				t.Fatalf("validateParams() error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestToEntType_MapsKnownTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want entnotification.Type
	}{
		{name: "pending", in: TypeApprovalPending, want: entnotification.TypeAPPROVAL_PENDING},
		{name: "completed", in: TypeApprovalCompleted, want: entnotification.TypeAPPROVAL_COMPLETED},
		{name: "rejected", in: TypeApprovalRejected, want: entnotification.TypeAPPROVAL_REJECTED},
		{name: "vm", in: TypeVMStatusChange, want: entnotification.TypeVM_STATUS_CHANGE},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := toEntType(tc.in)
			if err != nil {
				t.Fatalf("toEntType() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("toEntType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToEntType_RejectsUnknownType(t *testing.T) {
	t.Parallel()

	_, err := toEntType("UNKNOWN")
	if err == nil {
		t.Fatal("toEntType() expected error for unknown type")
	}
}

func TestCloneParams_ReturnsIndependentMap(t *testing.T) {
	t.Parallel()

	source := map[string]interface{}{
		"ticketId": "ticket-123",
		"count":    3,
	}
	cloned := cloneParams(source)
	if len(cloned) != len(source) {
		t.Fatalf("cloneParams() len = %d, want %d", len(cloned), len(source))
	}
	if cloned["ticketId"] != "ticket-123" || cloned["count"] != 3 {
		t.Fatalf("cloneParams() = %#v, want copied values", cloned)
	}

	cloned["ticketId"] = "mutated"
	if source["ticketId"] != "ticket-123" {
		t.Fatalf("cloneParams() returned aliased map; source = %#v", source)
	}
}

func TestCloneParams_EmptyInputReturnsWritableMap(t *testing.T) {
	t.Parallel()

	cloned := cloneParams(nil)
	if cloned == nil {
		t.Fatal("cloneParams(nil) = nil, want writable empty map")
	}
	cloned["key"] = "value"
	if cloned["key"] != "value" {
		t.Fatalf("cloneParams(nil) map write failed: %#v", cloned)
	}
}

func TestInboxSender_SendPersistsUnreadLocalizedNotification(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "notification_sender_persist")
	recipientID := mustCreateNotificationRecipient(t, client, "persist")
	params := notificationSenderTestParams(recipientID)

	if err := NewInboxSender(client).Send(t.Context(), params); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	notification, err := client.Notification.Query().WithUser().Only(t.Context())
	if err != nil {
		t.Fatalf("query persisted notification: %v", err)
	}
	if notification.Type != entnotification.TypeAPPROVAL_PENDING {
		t.Fatalf("type = %q, want %q", notification.Type, entnotification.TypeAPPROVAL_PENDING)
	}
	if notification.Title != params.Title || notification.TitleKey != params.TitleKey {
		t.Fatalf("title/key = %q/%q, want %q/%q", notification.Title, notification.TitleKey, params.Title, params.TitleKey)
	}
	if notification.Message != params.Message || notification.MessageKey != params.MessageKey {
		t.Fatalf("message/key = %q/%q, want %q/%q", notification.Message, notification.MessageKey, params.Message, params.MessageKey)
	}
	if !reflect.DeepEqual(notification.TitleParams, params.TitleParams) ||
		!reflect.DeepEqual(notification.MessageParams, params.MessageParams) {
		t.Fatalf("i18n params = %#v/%#v, want %#v/%#v", notification.TitleParams, notification.MessageParams, params.TitleParams, params.MessageParams)
	}
	if notification.ResourceType != params.ResourceType || notification.ResourceID != params.ResourceID {
		t.Fatalf("resource = %q/%q, want %q/%q", notification.ResourceType, notification.ResourceID, params.ResourceType, params.ResourceID)
	}
	if notification.Read || notification.ReadAt != nil {
		t.Fatalf("new notification read state = %v/%v, want false/nil", notification.Read, notification.ReadAt)
	}
	if notification.Edges.User == nil || notification.Edges.User.ID != recipientID {
		t.Fatalf("recipient edge = %#v, want %q", notification.Edges.User, recipientID)
	}
	if notification.ID == "" || notification.CreatedAt.IsZero() {
		t.Fatalf("notification identity/time = %q/%v, want populated", notification.ID, notification.CreatedAt)
	}
}

func TestInboxSender_SendRejectsInvalidInputWithoutWriting(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "notification_sender_invalid")
	recipientID := mustCreateNotificationRecipient(t, client, "invalid")
	sender := NewInboxSender(client)

	invalidParams := notificationSenderTestParams(recipientID)
	invalidParams.MessageKey = ""
	if err := sender.Send(t.Context(), invalidParams); err == nil || !strings.Contains(err.Error(), "message_key is required") {
		t.Fatalf("Send() invalid params error = %v, want message_key validation", err)
	}

	unknownType := notificationSenderTestParams(recipientID)
	unknownType.Type = "UNKNOWN"
	if err := sender.Send(t.Context(), unknownType); err == nil || !strings.Contains(err.Error(), "unknown notification type") {
		t.Fatalf("Send() unknown type error = %v, want type validation", err)
	}

	count, err := client.Notification.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 0 {
		t.Fatalf("notification count = %d, want 0 after rejected sends", count)
	}
}

func TestInboxSender_SendToManyContinuesAfterRecipientFailure(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "notification_sender_many")
	firstRecipient := mustCreateNotificationRecipient(t, client, "many-first")
	lastRecipient := mustCreateNotificationRecipient(t, client, "many-last")
	params := notificationSenderTestParams("original-recipient")

	err := NewInboxSender(client).SendToMany(t.Context(), []string{
		firstRecipient,
		"missing-recipient",
		lastRecipient,
	}, params)
	if err == nil || !strings.Contains(err.Error(), "1/3 recipients") {
		t.Fatalf("SendToMany() error = %v, want one failed recipient", err)
	}
	if params.RecipientID != "original-recipient" {
		t.Fatalf("SendToMany() mutated input recipient to %q", params.RecipientID)
	}

	notifications, err := client.Notification.Query().WithUser().All(t.Context())
	if err != nil {
		t.Fatalf("query delivered notifications: %v", err)
	}
	if len(notifications) != 2 {
		t.Fatalf("delivered notification count = %d, want 2", len(notifications))
	}
	recipients := map[string]bool{}
	for _, notification := range notifications {
		if notification.Edges.User == nil {
			t.Fatal("delivered notification is missing recipient edge")
		}
		recipients[notification.Edges.User.ID] = true
	}
	if !recipients[firstRecipient] || !recipients[lastRecipient] {
		t.Fatalf("delivered recipients = %#v, want %q and %q", recipients, firstRecipient, lastRecipient)
	}
}

func TestInboxSender_UsesCallerTransactionBoundary(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "notification_sender_transaction")
	recipientID := mustCreateNotificationRecipient(t, client, "transaction")
	params := notificationSenderTestParams(recipientID)

	rollbackTx, beginRollbackErr := client.Tx(t.Context())
	if beginRollbackErr != nil {
		t.Fatalf("begin rollback transaction: %v", beginRollbackErr)
	}
	if sendErr := NewInboxSender(rollbackTx.Client()).Send(t.Context(), params); sendErr != nil {
		t.Fatalf("send in rollback transaction: %v", sendErr)
	}
	if rollbackErr := rollbackTx.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback notification transaction: %v", rollbackErr)
	}
	assertNotificationCount(t, client, 0)

	commitTx, beginCommitErr := client.Tx(t.Context())
	if beginCommitErr != nil {
		t.Fatalf("begin commit transaction: %v", beginCommitErr)
	}
	if sendErr := NewInboxSender(commitTx.Client()).Send(t.Context(), params); sendErr != nil {
		_ = commitTx.Rollback()
		t.Fatalf("send in commit transaction: %v", sendErr)
	}
	if commitErr := commitTx.Commit(); commitErr != nil {
		t.Fatalf("commit notification transaction: %v", commitErr)
	}
	assertNotificationCount(t, client, 1)
}

func notificationSenderTestParams(recipientID string) Params {
	return Params{
		RecipientID:   recipientID,
		Type:          TypeApprovalPending,
		Title:         "Approval pending",
		TitleKey:      TitleKeyApprovalPending,
		TitleParams:   map[string]interface{}{"ticketId": "ticket-1"},
		Message:       "A request is waiting for approval",
		MessageKey:    MessageKeyApprovalPending,
		MessageParams: map[string]interface{}{"requester": "alice", "namespace": "prod-a"},
		ResourceType:  "ticket",
		ResourceID:    "ticket-1",
	}
}

func mustCreateNotificationRecipient(t *testing.T, client *ent.Client, suffix string) string {
	t.Helper()

	id := "notification-recipient-" + suffix + "-" + uuid.NewString()
	if _, err := client.User.Create().
		SetID(id).
		SetUsername(id).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create notification recipient: %v", err)
	}
	return id
}

func assertNotificationCount(t *testing.T, client *ent.Client, want int) {
	t.Helper()

	count, err := client.Notification.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != want {
		t.Fatalf("notification count = %d, want %d", count, want)
	}
}

func TestApprovalPendingParams(t *testing.T) {
	t.Parallel()

	params := approvalPendingParams("ticket-123", "alice", "prod-ns")
	if params.Type != TypeApprovalPending {
		t.Fatalf("Type = %q, want %q", params.Type, TypeApprovalPending)
	}
	if params.TitleKey != TitleKeyApprovalPending || params.MessageKey != MessageKeyApprovalPending {
		t.Fatalf("i18n keys = %q/%q, want approval pending keys", params.TitleKey, params.MessageKey)
	}
	if params.ResourceType != "ticket" || params.ResourceID != "ticket-123" {
		t.Fatalf("resource = %q/%q, want ticket/ticket-123", params.ResourceType, params.ResourceID)
	}
	if params.MessageParams["requester"] != "alice" || params.MessageParams["namespace"] != "prod-ns" {
		t.Fatalf("message params = %#v, want requester and namespace", params.MessageParams)
	}
	if !strings.Contains(params.Message, "alice") || !strings.Contains(params.Message, "prod-ns") {
		t.Fatalf("message = %q, want requester and namespace", params.Message)
	}
}

func TestTriggers_OnTicketApproved_UsesTicketResource(t *testing.T) {
	t.Parallel()

	sender := &captureSender{}
	triggers := NewTriggers(sender, nil)

	triggers.OnTicketApproved(context.Background(), "ticket-123", "user-1", "alice")

	if sender.lastSend.Type != TypeApprovalCompleted {
		t.Fatalf("type = %q, want %q", sender.lastSend.Type, TypeApprovalCompleted)
	}
	if sender.lastSend.ResourceType != "ticket" {
		t.Fatalf("resource_type = %q, want ticket", sender.lastSend.ResourceType)
	}
	if sender.lastSend.ResourceID != "ticket-123" {
		t.Fatalf("resource_id = %q, want ticket-123", sender.lastSend.ResourceID)
	}
	if sender.lastSend.MessageKey != MessageKeyApprovalCompleted {
		t.Fatalf("message_key = %q, want %q", sender.lastSend.MessageKey, MessageKeyApprovalCompleted)
	}
	if sender.lastSend.MessageParams["ticketId"] != "ticket-123" {
		t.Fatalf("message params = %#v, want ticket id", sender.lastSend.MessageParams)
	}
}

func TestTriggers_OnTicketRejected_IncludesReason(t *testing.T) {
	t.Parallel()

	sender := &captureSender{}
	triggers := NewTriggers(sender, nil)

	triggers.OnTicketRejected(context.Background(), "ticket-123", "user-1", "alice", "quota exceeded")

	if sender.lastSend.Type != TypeApprovalRejected {
		t.Fatalf("type = %q, want %q", sender.lastSend.Type, TypeApprovalRejected)
	}
	if !strings.Contains(sender.lastSend.Message, "quota exceeded") {
		t.Fatalf("message = %q, want rejection reason included", sender.lastSend.Message)
	}
	if sender.lastSend.MessageKey != MessageKeyApprovalRejectedWithReason {
		t.Fatalf("message_key = %q, want %q", sender.lastSend.MessageKey, MessageKeyApprovalRejectedWithReason)
	}
	if sender.lastSend.MessageParams["reason"] != "quota exceeded" {
		t.Fatalf("message params = %#v, want rejection reason", sender.lastSend.MessageParams)
	}
	if sender.lastSend.ResourceType != "ticket" {
		t.Fatalf("resource_type = %q, want ticket", sender.lastSend.ResourceType)
	}
}

func TestTriggers_OnVMStatusChanged_UsesVMResource(t *testing.T) {
	t.Parallel()

	sender := &captureSender{}
	triggers := NewTriggers(sender, nil)

	triggers.OnVMStatusChanged(context.Background(), "vm-1", "vm-name", "user-1", "RUNNING")

	if sender.lastSend.Type != TypeVMStatusChange {
		t.Fatalf("type = %q, want %q", sender.lastSend.Type, TypeVMStatusChange)
	}
	if sender.lastSend.ResourceType != "vm" {
		t.Fatalf("resource_type = %q, want vm", sender.lastSend.ResourceType)
	}
	if sender.lastSend.ResourceID != "vm-1" {
		t.Fatalf("resource_id = %q, want vm-1", sender.lastSend.ResourceID)
	}
	if sender.lastSend.TitleKey != TitleKeyVMStatusChange || sender.lastSend.MessageKey != MessageKeyVMStatusChange {
		t.Fatalf("i18n keys = %q/%q, want VM status keys", sender.lastSend.TitleKey, sender.lastSend.MessageKey)
	}
	if sender.lastSend.MessageParams["vmName"] != "vm-name" || sender.lastSend.MessageParams["state"] != "RUNNING" {
		t.Fatalf("message params = %#v, want VM name/state", sender.lastSend.MessageParams)
	}
}
