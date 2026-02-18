package usecase

import "testing"

func TestApprovalAtomicWriterValidateCreateInput(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}

	tests := []struct {
		name      string
		ticketID  string
		eventID   string
		approver  string
		clusterID string
		serviceID string
		namespace string
		requester string
		wantErr   bool
	}{
		{
			name:      "valid input",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "user-1",
			wantErr:   false,
		},
		{
			name:      "namespace required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "",
			requester: "user-1",
			wantErr:   true,
		},
		{
			name:      "cluster required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "user-1",
			wantErr:   true,
		},
		{
			name:      "requester required",
			ticketID:  "t-1",
			eventID:   "e-1",
			approver:  "admin-1",
			clusterID: "cluster-1",
			serviceID: "svc-1",
			namespace: "team-a",
			requester: "",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := w.validateCreateInput(
				tc.ticketID,
				tc.eventID,
				tc.approver,
				tc.clusterID,
				tc.serviceID,
				tc.namespace,
				tc.requester,
			)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMarshalJSONOrNull(t *testing.T) {
	t.Parallel()

	if b, err := marshalJSONOrNull(nil); err != nil || b != nil {
		t.Fatalf("marshalJSONOrNull(nil) = (%v, %v), want (nil, nil)", b, err)
	}
	if b, err := marshalJSONOrNull(map[string]interface{}{}); err != nil || b != nil {
		t.Fatalf("marshalJSONOrNull(empty) = (%v, %v), want (nil, nil)", b, err)
	}
	if b, err := marshalJSONOrNull(map[string]interface{}{"a": "b"}); err != nil || len(b) == 0 {
		t.Fatalf("marshalJSONOrNull(non-empty) unexpected: (%s, %v)", string(b), err)
	}
}
