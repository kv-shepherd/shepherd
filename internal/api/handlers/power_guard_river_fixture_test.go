package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/jobs"
)

func mustInsertRunnableHandlerPowerJob(t *testing.T, srv *Server, eventID string) int64 {
	t.Helper()
	if srv == nil || srv.riverClient == nil {
		t.Fatal("handler test server River client is not initialized")
	}
	inserted, err := srv.riverClient.Insert(t.Context(), jobs.VMPowerArgs{EventID: eventID}, nil)
	if err != nil {
		t.Fatalf("insert runnable vm_power job for event %s: %v", eventID, err)
	}
	if inserted == nil || inserted.Job == nil || inserted.UniqueSkippedAsDuplicate {
		t.Fatalf("runnable vm_power job insert result for event %s = %#v, want newly inserted job", eventID, inserted)
	}
	return inserted.Job.ID
}

func mustInsertRunnableHandlerPowerJobForTicket(
	t *testing.T,
	srv *Server,
	client *ent.Client,
	ticketID string,
) int64 {
	t.Helper()
	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	if err != nil {
		t.Fatalf("query power ticket %s before River job insert: %v", ticketID, err)
	}
	return mustInsertRunnableHandlerPowerJob(t, srv, ticket.EventID)
}
