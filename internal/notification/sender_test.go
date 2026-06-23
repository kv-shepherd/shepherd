package notification

import (
	"context"
	"strings"
	"testing"

	entnotification "kv-shepherd.io/shepherd/ent/notification"
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
