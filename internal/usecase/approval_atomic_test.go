package usecase

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/jobs"
)

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

func TestMarshalJSONOrNull_NestedSnapshot(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"source_type": "containerdisk",
		"image_url":   "quay.io/containerdisks/ubuntu:22.04",
		"spec_overrides": map[string]interface{}{
			"spec.template.spec.domain.cpu.cores": float64(4),
		},
	}

	b, err := marshalJSONOrNull(payload)
	if err != nil {
		t.Fatalf("marshalJSONOrNull(nested) unexpected error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("marshalJSONOrNull(nested) should return non-empty json bytes")
	}
}

func TestApprovalAtomicWriterApprovePowerAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.ApprovePowerAndEnqueue(t.Context(), "ticket-1", "event-1", "admin-1", "start")
	if err == nil {
		t.Fatal("ApprovePowerAndEnqueue() expected initialization error, got nil")
	}
}

func TestVMStatusSyncInsertOpts(t *testing.T) {
	t.Parallel()

	opts := vmStatusSyncInsertOpts()
	if opts == nil {
		t.Fatal("vmStatusSyncInsertOpts() returned nil")
	}
	if opts.Queue != jobs.VMStatusSyncJobKind {
		t.Fatalf("queue = %q, want %q", opts.Queue, jobs.VMStatusSyncJobKind)
	}
	if opts.MaxAttempts != 3 {
		t.Fatalf("max attempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs || !opts.UniqueOpts.ByQueue {
		t.Fatalf("unique opts = %+v, want ByArgs=true and ByQueue=true", opts.UniqueOpts)
	}
}

func TestSnapshotRootVolumeHelpers(t *testing.T) {
	t.Parallel()

	values := map[string]interface{}{
		"dv_access_modes": []interface{}{"ReadWriteMany", "  ", "ReadWriteOnce"},
		"dv_volume_mode":  " Block ",
	}

	if got := snapshotString(values, "dv_volume_mode"); got != "Block" {
		t.Fatalf("snapshotString(volume_mode) = %q, want Block", got)
	}

	gotModes := snapshotStringSlice(values, "dv_access_modes")
	if len(gotModes) != 2 || gotModes[0] != "ReadWriteMany" || gotModes[1] != "ReadWriteOnce" {
		t.Fatalf("snapshotStringSlice(access_modes) = %#v, want [ReadWriteMany ReadWriteOnce]", gotModes)
	}
}

func TestMarshalJSONArrayOrNull(t *testing.T) {
	t.Parallel()

	data, err := marshalJSONArrayOrNull(nil)
	if err != nil {
		t.Fatalf("marshalJSONArrayOrNull(nil) error = %v", err)
	}
	if data != nil {
		t.Fatalf("marshalJSONArrayOrNull(nil) = %s, want nil", string(data))
	}

	data, err = marshalJSONArrayOrNull([]string{"ReadWriteMany"})
	if err != nil {
		t.Fatalf("marshalJSONArrayOrNull(non-empty) error = %v", err)
	}
	if string(data) != `["ReadWriteMany"]` {
		t.Fatalf("marshalJSONArrayOrNull(non-empty) = %s, want [\"ReadWriteMany\"]", string(data))
	}
}
