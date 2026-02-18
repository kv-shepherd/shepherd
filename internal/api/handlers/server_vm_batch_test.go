package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"kv-shepherd.io/shepherd/ent/batchapprovalticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
)

func TestNormalizeBatchOperation(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		op, eventType, err := normalizeBatchOperation(generated.VMBatchOperationCREATE)
		if err != nil {
			t.Fatalf("normalizeBatchOperation(CREATE) returned error: %v", err)
		}
		if op != string(generated.VMBatchOperationCREATE) {
			t.Fatalf("operation = %q, want %q", op, generated.VMBatchOperationCREATE)
		}
		if eventType != domain.EventBatchCreateRequested {
			t.Fatalf("eventType = %q, want %q", eventType, domain.EventBatchCreateRequested)
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		op, eventType, err := normalizeBatchOperation(generated.VMBatchOperationDELETE)
		if err != nil {
			t.Fatalf("normalizeBatchOperation(DELETE) returned error: %v", err)
		}
		if op != string(generated.VMBatchOperationDELETE) {
			t.Fatalf("operation = %q, want %q", op, generated.VMBatchOperationDELETE)
		}
		if eventType != domain.EventBatchDeleteRequested {
			t.Fatalf("eventType = %q, want %q", eventType, domain.EventBatchDeleteRequested)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		_, _, err := normalizeBatchOperation(generated.VMBatchOperationPOWER)
		if err == nil || !strings.Contains(err.Error(), "unsupported operation") {
			t.Fatalf("normalizeBatchOperation(POWER) error = %v, want unsupported operation", err)
		}
	})
}

func TestNormalizeBatchPowerOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     generated.VMBatchPowerAction
		wantKey   string
		wantJob   string
		wantEvent domain.EventType
	}{
		{
			name:      "start",
			input:     generated.VMBatchPowerAction("start"),
			wantKey:   "POWER_START",
			wantJob:   "START",
			wantEvent: domain.EventVMStartRequested,
		},
		{
			name:      "stop with spaces",
			input:     generated.VMBatchPowerAction(" stop "),
			wantKey:   "POWER_STOP",
			wantJob:   "STOP",
			wantEvent: domain.EventVMStopRequested,
		},
		{
			name:      "restart",
			input:     generated.VMBatchPowerAction("RESTART"),
			wantKey:   "POWER_RESTART",
			wantJob:   "RESTART",
			wantEvent: domain.EventVMRestartRequested,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotKey, gotJob, gotEvent, err := normalizeBatchPowerOperation(tc.input)
			if err != nil {
				t.Fatalf("normalizeBatchPowerOperation(%q) returned error: %v", tc.input, err)
			}
			if gotKey != tc.wantKey {
				t.Fatalf("opKey = %q, want %q", gotKey, tc.wantKey)
			}
			if gotJob != tc.wantJob {
				t.Fatalf("jobOperation = %q, want %q", gotJob, tc.wantJob)
			}
			if gotEvent != tc.wantEvent {
				t.Fatalf("childEventType = %q, want %q", gotEvent, tc.wantEvent)
			}
		})
	}

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		_, _, _, err := normalizeBatchPowerOperation(generated.VMBatchPowerAction("hibernate"))
		if err == nil || !strings.Contains(err.Error(), "unsupported power operation") {
			t.Fatalf("normalizeBatchPowerOperation error = %v, want unsupported power operation", err)
		}
	})
}

func TestAggregateBatchParentStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		total        int
		successCount int
		failedCount  int
		pendingCount int
		pendingOnly  int
		executing    int
		cancelled    int
		want         generated.VMBatchParentStatus
	}{
		{
			name: "zero total failed",
			want: generated.VMBatchParentStatusFAILED,
		},
		{
			name:      "all cancelled",
			total:     3,
			cancelled: 3,
			want:      generated.VMBatchParentStatusCANCELLED,
		},
		{
			name:         "all completed",
			total:        2,
			successCount: 2,
			want:         generated.VMBatchParentStatusCOMPLETED,
		},
		{
			name:        "all failed and cancelled",
			total:       4,
			failedCount: 2,
			cancelled:   2,
			want:        generated.VMBatchParentStatusFAILED,
		},
		{
			name:         "all pending approval",
			total:        5,
			pendingOnly:  5,
			pendingCount: 5,
			want:         generated.VMBatchParentStatusPENDINGAPPROVAL,
		},
		{
			name:         "in progress with executing children",
			total:        3,
			successCount: 1,
			pendingCount: 1,
			executing:    1,
			want:         generated.VMBatchParentStatusINPROGRESS,
		},
		{
			name:         "partial success",
			total:        3,
			successCount: 2,
			failedCount:  1,
			want:         generated.VMBatchParentStatusPARTIALSUCCESS,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := aggregateBatchParentStatus(
				tc.total,
				tc.successCount,
				tc.failedCount,
				tc.pendingCount,
				tc.pendingOnly,
				tc.executing,
				tc.cancelled,
			)
			if got != tc.want {
				t.Fatalf("aggregateBatchParentStatus(...) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMapProjectionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   generated.VMBatchParentStatus
		want batchapprovalticket.Status
	}{
		{
			name: "pending approval",
			in:   generated.VMBatchParentStatusPENDINGAPPROVAL,
			want: batchapprovalticket.StatusPENDING_APPROVAL,
		},
		{
			name: "in progress",
			in:   generated.VMBatchParentStatusINPROGRESS,
			want: batchapprovalticket.StatusIN_PROGRESS,
		},
		{
			name: "completed",
			in:   generated.VMBatchParentStatusCOMPLETED,
			want: batchapprovalticket.StatusCOMPLETED,
		},
		{
			name: "partial success",
			in:   generated.VMBatchParentStatusPARTIALSUCCESS,
			want: batchapprovalticket.StatusPARTIAL_SUCCESS,
		},
		{
			name: "cancelled",
			in:   generated.VMBatchParentStatusCANCELLED,
			want: batchapprovalticket.StatusCANCELLED,
		},
		{
			name: "fallback to failed",
			in:   generated.VMBatchParentStatus("UNKNOWN"),
			want: batchapprovalticket.StatusFAILED,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mapProjectionStatus(tc.in); got != tc.want {
				t.Fatalf("mapProjectionStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestToBatchProjectionType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want batchapprovalticket.BatchType
	}{
		{
			name: "delete op",
			in:   string(generated.VMBatchOperationDELETE),
			want: batchapprovalticket.BatchTypeBATCH_DELETE,
		},
		{
			name: "power enum op",
			in:   string(generated.VMBatchOperationPOWER),
			want: batchapprovalticket.BatchTypeBATCH_POWER,
		},
		{
			name: "power start key",
			in:   "POWER_START",
			want: batchapprovalticket.BatchTypeBATCH_POWER,
		},
		{
			name: "fallback create",
			in:   string(generated.VMBatchOperationCREATE),
			want: batchapprovalticket.BatchTypeBATCH_CREATE,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := toBatchProjectionType(tc.in); got != tc.want {
				t.Fatalf("toBatchProjectionType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNillableTrimmed(t *testing.T) {
	t.Parallel()

	if got := nillableTrimmed("   "); got != nil {
		t.Fatalf("nillableTrimmed(empty) = %v, want nil", *got)
	}

	got := nillableTrimmed("  abc  ")
	if got == nil {
		t.Fatal("nillableTrimmed(non-empty) = nil, want non-nil")
	}
	if *got != "abc" {
		t.Fatalf("nillableTrimmed(non-empty) = %q, want %q", *got, "abc")
	}
}

func TestBuildBatchPayloadItems(t *testing.T) {
	t.Parallel()

	serviceID := mustOpenAPIUUID(t, "11111111-1111-1111-1111-111111111111")
	templateID := mustOpenAPIUUID(t, "22222222-2222-2222-2222-222222222222")
	sizeID := mustOpenAPIUUID(t, "33333333-3333-3333-3333-333333333333")

	items := []generated.VMBatchChildItem{
		{
			VmId:           " vm-1 ",
			ServiceId:      serviceID,
			TemplateId:     templateID,
			InstanceSizeId: sizeID,
			Namespace:      " prod ",
			Reason:         "  reason-one  ",
		},
	}

	t.Run("create operation clears vm id and keeps create fields", func(t *testing.T) {
		t.Parallel()

		got := buildBatchPayloadItems(string(generated.VMBatchOperationCREATE), items)
		if len(got) != 1 {
			t.Fatalf("len(payload) = %d, want 1", len(got))
		}
		if got[0].VMID != "" {
			t.Fatalf("payload VMID = %q, want empty for create", got[0].VMID)
		}
		if got[0].ServiceID == "" || got[0].TemplateID == "" || got[0].InstanceSizeID == "" {
			t.Fatal("create payload missing required IDs")
		}
		if got[0].Namespace != "prod" {
			t.Fatalf("payload Namespace = %q, want %q", got[0].Namespace, "prod")
		}
		if got[0].Reason != "reason-one" {
			t.Fatalf("payload Reason = %q, want %q", got[0].Reason, "reason-one")
		}
	})

	t.Run("delete operation keeps vm id and clears create fields", func(t *testing.T) {
		t.Parallel()

		got := buildBatchPayloadItems(string(generated.VMBatchOperationDELETE), items)
		if len(got) != 1 {
			t.Fatalf("len(payload) = %d, want 1", len(got))
		}
		if got[0].VMID != "vm-1" {
			t.Fatalf("payload VMID = %q, want %q", got[0].VMID, "vm-1")
		}
		if got[0].ServiceID != "" || got[0].TemplateID != "" || got[0].InstanceSizeID != "" || got[0].Namespace != "" {
			t.Fatalf("delete payload must clear create fields, got %+v", got[0])
		}
	})
}

func TestBuildBatchPowerPayloadItems(t *testing.T) {
	t.Parallel()

	got := buildBatchPowerPayloadItems([]generated.VMBatchPowerItem{
		{VmId: " vm-a ", Reason: "  keep  "},
	})
	if len(got) != 1 {
		t.Fatalf("len(payload) = %d, want 1", len(got))
	}
	if got[0].VMID != "vm-a" {
		t.Fatalf("payload VMID = %q, want %q", got[0].VMID, "vm-a")
	}
	if got[0].Reason != "keep" {
		t.Fatalf("payload Reason = %q, want %q", got[0].Reason, "keep")
	}
}

func TestIsZeroUUID(t *testing.T) {
	t.Parallel()

	var zero openapi_types.UUID
	if !isZeroUUID(zero) {
		t.Fatal("isZeroUUID(zero) = false, want true")
	}

	nonZero := mustOpenAPIUUID(t, "44444444-4444-4444-4444-444444444444")
	if isZeroUUID(nonZero) {
		t.Fatal("isZeroUUID(nonZero) = true, want false")
	}
}

func mustOpenAPIUUID(t *testing.T, raw string) openapi_types.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("uuid.Parse(%q) failed: %v", raw, err)
	}
	return openapi_types.UUID(id)
}
