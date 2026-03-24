package notificationcontract

import "testing"

func TestNotificationDataRoundTrip(t *testing.T) {
	notification := Notification{
		RecipientID: "user-1",
		Data:        map[string]interface{}{"ticket_id": "ticket-1"},
	}
	if notification.RecipientID != "user-1" {
		t.Fatalf("notification.RecipientID = %q, want user-1", notification.RecipientID)
	}
	if notification.Data["ticket_id"] != "ticket-1" {
		t.Fatalf("notification.Data = %#v, want ticket_id", notification.Data)
	}
}
