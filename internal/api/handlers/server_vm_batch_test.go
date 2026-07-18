package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
)

func TestNormalizeBatchOperation(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		op, eventType, err := normalizeBatchOperation(generated.VMBatchSubmitOperation("CREATE"))
		if err != nil {
			t.Fatalf("normalizeBatchOperation(CREATE) returned error: %v", err)
		}
		if op != string(generated.VMBatchSubmitOperation("CREATE")) {
			t.Fatalf("operation = %q, want %q", op, generated.VMBatchSubmitOperation("CREATE"))
		}
		if eventType != domain.EventBatchCreateRequested {
			t.Fatalf("eventType = %q, want %q", eventType, domain.EventBatchCreateRequested)
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		op, eventType, err := normalizeBatchOperation(generated.VMBatchSubmitOperation("DELETE"))
		if err != nil {
			t.Fatalf("normalizeBatchOperation(DELETE) returned error: %v", err)
		}
		if op != string(generated.VMBatchSubmitOperation("DELETE")) {
			t.Fatalf("operation = %q, want %q", op, generated.VMBatchSubmitOperation("DELETE"))
		}
		if eventType != domain.EventBatchDeleteRequested {
			t.Fatalf("eventType = %q, want %q", eventType, domain.EventBatchDeleteRequested)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		_, _, err := normalizeBatchOperation(generated.VMBatchSubmitOperation("POWER"))
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
		name          string
		total         int
		successCount  int
		failedCount   int
		pendingCount  int
		pendingOnly   int
		executing     int
		cancelled     int
		parentPending bool
		want          generated.VMBatchParentStatus
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
			name:          "all pending approval",
			total:         5,
			pendingOnly:   5,
			pendingCount:  5,
			parentPending: true,
			want:          generated.VMBatchParentStatusPENDINGAPPROVAL,
		},
		{
			name:         "dispatcher queued with pending children",
			total:        5,
			pendingOnly:  5,
			pendingCount: 5,
			want:         generated.VMBatchParentStatusINPROGRESS,
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
				tc.parentPending,
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
		want entbatchticket.Status
	}{
		{
			name: "pending approval",
			in:   generated.VMBatchParentStatusPENDINGAPPROVAL,
			want: entbatchticket.StatusPENDING_APPROVAL,
		},
		{
			name: "in progress",
			in:   generated.VMBatchParentStatusINPROGRESS,
			want: entbatchticket.StatusIN_PROGRESS,
		},
		{
			name: "completed",
			in:   generated.VMBatchParentStatusCOMPLETED,
			want: entbatchticket.StatusCOMPLETED,
		},
		{
			name: "partial success",
			in:   generated.VMBatchParentStatusPARTIALSUCCESS,
			want: entbatchticket.StatusPARTIAL_SUCCESS,
		},
		{
			name: "cancelled",
			in:   generated.VMBatchParentStatusCANCELLED,
			want: entbatchticket.StatusCANCELLED,
		},
		{
			name: "fallback to failed",
			in:   generated.VMBatchParentStatus("UNKNOWN"),
			want: entbatchticket.StatusFAILED,
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
		want entbatchticket.BatchType
	}{
		{
			name: "delete op",
			in:   string(generated.VMBatchSubmitOperation("DELETE")),
			want: entbatchticket.BatchTypeBATCH_DELETE,
		},
		{
			name: "power enum op",
			in:   string(generated.VMBatchSubmitOperation("POWER")),
			want: entbatchticket.BatchTypeBATCH_POWER,
		},
		{
			name: "power start key",
			in:   "POWER_START",
			want: entbatchticket.BatchTypeBATCH_POWER,
		},
		{
			name: "fallback create",
			in:   string(generated.VMBatchSubmitOperation("CREATE")),
			want: entbatchticket.BatchTypeBATCH_CREATE,
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
		return
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

		got := buildBatchPayloadItems(string(generated.VMBatchSubmitOperation("CREATE")), items)
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

		childPayload, err := json.Marshal(domain.VMDeletePayload{
			VMID:               "vm-1",
			VMName:             "vm-one",
			SystemID:           "system-1",
			SystemName:         "shop",
			ServiceID:          "service-1",
			ServiceName:        "redis",
			Namespace:          "prod",
			ClusterID:          "cluster-1",
			ClusterName:        "Cluster One",
			ClusterEnvironment: "test",
			OwnerID:            "user-1",
			OwnerDisplayName:   "Alice",
			OwnerUsername:      "alice",
			TemplateID:         "template-1",
			TemplateName:       "OpenEuler 22.03",
			InstanceSizeID:     "size-1",
			InstanceSizeName:   "M4 Large",
			RequestVMStatus:    "STOPPED",
			CurrentCPUCores:    4,
			CurrentMemoryGi:    8,
			CurrentDiskGB:      60,
		})
		if err != nil {
			t.Fatalf("marshal child payload: %v", err)
		}

		got := buildBatchPayloadItems(
			string(generated.VMBatchSubmitOperation("DELETE")),
			items,
			preparedBatchChild{payload: childPayload},
		)
		if len(got) != 1 {
			t.Fatalf("len(payload) = %d, want 1", len(got))
		}
		if got[0].VMID != "vm-1" {
			t.Fatalf("payload VMID = %q, want %q", got[0].VMID, "vm-1")
		}
		if got[0].ServiceName != "redis" || got[0].SystemName != "shop" {
			t.Fatalf("delete payload missing scope snapshot: %+v", got[0])
		}
		if got[0].TemplateName != "OpenEuler 22.03" || got[0].InstanceSizeName != "M4 Large" {
			t.Fatalf("delete payload missing config snapshot: %+v", got[0])
		}
		if got[0].OwnerDisplayName != "Alice" || got[0].OwnerUsername != "alice" {
			t.Fatalf("delete payload missing owner snapshot: %+v", got[0])
		}
		if got[0].Namespace != "prod" || got[0].ClusterName != "Cluster One" || got[0].ClusterEnvironment != "test" {
			t.Fatalf("delete payload missing target snapshot: %+v", got[0])
		}
		if got[0].CurrentCPUCores != 4 || got[0].CurrentMemoryGi != 8 || got[0].CurrentDiskGB != 60 {
			t.Fatalf("delete payload missing resource snapshot: %+v", got[0])
		}
	})

	t.Run("create operation keeps vm id cleared but preserves readable create snapshot", func(t *testing.T) {
		t.Parallel()

		childPayload, err := json.Marshal(domain.VMCreationPayload{
			RequesterID:      "user-1",
			OwnerID:          "user-1",
			OwnerDisplayName: "Alice",
			OwnerUsername:    "alice",
			ServiceID:        "service-1",
			ServiceName:      "redis",
			SystemID:         "system-1",
			SystemName:       "shop",
			TemplateID:       "template-1",
			TemplateName:     "OpenEuler 22.03",
			InstanceSizeID:   "size-1",
			InstanceSizeName: "M4 Large",
			Namespace:        "prod",
			TargetCPUCores:   4,
			TargetMemoryGi:   8,
			TargetDiskGB:     60,
		})
		if err != nil {
			t.Fatalf("marshal child payload: %v", err)
		}

		got := buildBatchPayloadItems(
			string(generated.VMBatchSubmitOperation("CREATE")),
			items,
			preparedBatchChild{payload: childPayload},
		)
		if len(got) != 1 {
			t.Fatalf("len(payload) = %d, want 1", len(got))
		}
		if got[0].VMID != "" {
			t.Fatalf("payload VMID = %q, want empty for create", got[0].VMID)
		}
		if got[0].SystemName != "shop" || got[0].ServiceName != "redis" {
			t.Fatalf("create payload missing scope snapshot: %+v", got[0])
		}
		if got[0].TemplateName != "OpenEuler 22.03" || got[0].InstanceSizeName != "M4 Large" {
			t.Fatalf("create payload missing config snapshot: %+v", got[0])
		}
		if got[0].OwnerDisplayName != "Alice" || got[0].OwnerUsername != "alice" {
			t.Fatalf("create payload missing owner snapshot: %+v", got[0])
		}
		if got[0].TargetCPUCores == nil || *got[0].TargetCPUCores != 4 || got[0].TargetMemoryGi == nil || *got[0].TargetMemoryGi != 8 || got[0].TargetDiskGB == nil || *got[0].TargetDiskGB != 60 {
			t.Fatalf("create payload missing requested resources: %+v", got[0])
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
	return id
}
