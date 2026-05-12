package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/testutil"
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

func TestApprovalAtomicWriterCreatePowerEventAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.CreatePowerEventAndEnqueue(t.Context(), PowerEventInput{
		EventID:       "event-1",
		EventType:     "VM_START_REQUESTED",
		AggregateType: "vm",
		AggregateID:   "vm-1",
		Payload:       []byte(`{"operation":"start"}`),
		CreatedBy:     "user-1",
	})
	if err == nil {
		t.Fatal("CreatePowerEventAndEnqueue() expected initialization error, got nil")
	}
}

func TestApprovalAtomicWriterCreateBatchPowerAndMaybeEnqueue_CommitsRowsAndJobsAtomically(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := testutil.OpenPGXPool(t, "r")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	w := NewApprovalAtomicWriter(pool, riverClient)
	err = w.CreateBatchPowerAndMaybeEnqueue(ctx, BatchPowerSubmissionInput{
		ParentID:      "batch-atomic-1",
		Actor:         "user-1",
		RequestID:     "request-1",
		Reason:        "batch power start",
		ParentPayload: []byte(`{"operation":"POWER_START","items":[]}`),
		Children: []BatchPowerChildInput{
			{
				EventType:   "VM_START_REQUESTED",
				AggregateID: "vm-1",
				Payload:     []byte(`{"vm_id":"vm-1","operation":"start"}`),
				Reason:      "start vm",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatchPowerAndMaybeEnqueue() unexpected error: %v", err)
	}

	var parentStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, "batch-atomic-1").Scan(&parentStatus); err != nil {
		t.Fatalf("query parent ticket: %v", err)
	}
	if parentStatus != "EXECUTING" {
		t.Fatalf("parent status = %q, want EXECUTING", parentStatus)
	}

	var childStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE parent_ticket_id=$1`, "batch-atomic-1").Scan(&childStatus); err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if childStatus != "EXECUTING" {
		t.Fatalf("child status = %q, want EXECUTING", childStatus)
	}

	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, "vm_power").Scan(&jobCount); err != nil {
		t.Fatalf("query river jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("river vm_power job count = %d, want 1", jobCount)
	}
}

func TestApprovalAtomicWriterApproveModifyAndEnqueue_RequiresInitializedWriter(t *testing.T) {
	t.Parallel()

	w := &ApprovalAtomicWriter{}
	err := w.ApproveModifyAndEnqueue(t.Context(), "ticket-1", "event-1", "admin-1", nil)
	if err == nil {
		t.Fatal("ApproveModifyAndEnqueue() expected initialization error, got nil")
	}
}

func TestVMStatusSyncInsertOpts(t *testing.T) {
	t.Parallel()

	opts := vmStatusSyncInsertOpts()
	if opts == nil {
		t.Fatal("vmStatusSyncInsertOpts() returned nil")
		return
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
