package approvalcontract

import "testing"

func TestApprovalRequestMetadataRoundTrip(t *testing.T) {
	request := ApprovalRequest{
		EventID:  "evt-1",
		Metadata: map[string]interface{}{"scope": "prod"},
	}
	if request.EventID != "evt-1" {
		t.Fatalf("request.EventID = %q, want evt-1", request.EventID)
	}
	if request.Metadata["scope"] != "prod" {
		t.Fatalf("request.Metadata = %#v, want prod scope", request.Metadata)
	}
}
