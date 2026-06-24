package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/enttest"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func TestBatchHandler_SubmitVMBatch_Unauthorized(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: "vm-1"},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "", nil)
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
}

func TestBatchHandler_SubmitApprovalBatch_AliasPath(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}
	var resp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.BatchId == "" {
		t.Fatal("batch_id is empty")
	}
}

func TestBatchHandler_SubmitVMBatch_InvalidBatchSize(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items:     []generated.VMBatchChildItem{},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "user-a", []string{"platform:admin"})
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_BATCH_SIZE")
}

func TestBatchHandler_SubmitVMBatch_DeleteRejectsRunningVM(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVMWithStatus(t, client, entvm.StatusRUNNING)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), usecase.VMDeleteInvalidStateCode)
}

func TestBatchHandler_SubmitVMBatch_ModifyCreatesModifyChildTickets(t *testing.T) {
	t.Parallel()

	srv, client, vmID := newBatchModifyTestServer(t)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("MODIFY"),
		Reason:    "scale resources",
		Items: []generated.VMBatchChildItem{
			{
				VmId:           vmID,
				Reason:         "scale memory",
				TargetMemoryGi: 8,
			},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"vm:operate", "platform:admin"})
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.BatchId == "" {
		t.Fatal("batch_id is empty")
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(resp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].OperationType != entticket.OperationTypeMODIFY {
		t.Fatalf("child operation_type = %q, want %q", children[0].OperationType, entticket.OperationTypeMODIFY)
	}
}

func TestBatchHandler_SubmitDelete_GetAndCancel(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Reason:    "bulk cleanup",
		Items: []generated.VMBatchChildItem{
			{VmId: vmID, Reason: "delete one"},
		},
	})

	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}

	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitResp.BatchId == "" {
		t.Fatal("submit response batch_id is empty")
	}
	if submitResp.Status != generated.VMBatchParentStatusPENDINGAPPROVAL {
		t.Fatalf("submit status = %q, want %q", submitResp.Status, generated.VMBatchParentStatusPENDINGAPPROVAL)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].Status != entticket.StatusPENDING {
		t.Fatalf("child status = %q, want %q", children[0].Status, entticket.StatusPENDING)
	}

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+submitResp.BatchId, "", "owner-1", []string{"vm:read"})
	srv.GetVMBatch(getCtx, submitResp.BatchId)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d body=%s", getW.Code, http.StatusOK, getW.Body.String())
	}
	var getResp generated.VMBatchStatusResponse
	if decodeErr := json.Unmarshal(getW.Body.Bytes(), &getResp); decodeErr != nil {
		t.Fatalf("decode get response: %v", decodeErr)
	}
	if getResp.ChildCount != 1 || getResp.PendingCount != 1 {
		t.Fatalf("unexpected get counters: child=%d pending=%d", getResp.ChildCount, getResp.PendingCount)
	}

	cancelCtx, cancelW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/cancel", "", "owner-1", []string{"vm:delete"})
	srv.CancelVMBatch(cancelCtx, submitResp.BatchId)
	if cancelW.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d body=%s", cancelW.Code, http.StatusOK, cancelW.Body.String())
	}
	var cancelResp generated.VMBatchActionResponse
	if decodeErr := json.Unmarshal(cancelW.Body.Bytes(), &cancelResp); decodeErr != nil {
		t.Fatalf("decode cancel response: %v", decodeErr)
	}
	if cancelResp.AffectedCount != 1 {
		t.Fatalf("affected_count = %d, want 1", cancelResp.AffectedCount)
	}
	if len(cancelResp.AffectedTicketIds) != 1 || cancelResp.AffectedTicketIds[0] != children[0].ID {
		t.Fatalf("affected_ticket_ids = %v, want [%s]", cancelResp.AffectedTicketIds, children[0].ID)
	}
	if cancelResp.Status != generated.VMBatchParentStatusCANCELLED {
		t.Fatalf("cancel status = %q, want %q", cancelResp.Status, generated.VMBatchParentStatusCANCELLED)
	}

	updatedChild, err := client.Ticket.Get(t.Context(), children[0].ID)
	if err != nil {
		t.Fatalf("query updated child ticket: %v", err)
	}
	if updatedChild.Status != entticket.StatusCANCELLED {
		t.Fatalf("child status after cancel = %q, want %q", updatedChild.Status, entticket.StatusCANCELLED)
	}
}

func TestBatchHandler_CancelVMBatch_RollsBackChildTicketsWhenEventCancelFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}
	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("child event cancel unavailable")),
		ent.OpUpdate,
	))

	cancelCtx, cancelW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/cancel", "", "owner-1", []string{"vm:delete"})
	srv.CancelVMBatch(cancelCtx, submitResp.BatchId)
	if cancelW.Code != http.StatusInternalServerError {
		t.Fatalf("cancel status = %d, want %d body=%s", cancelW.Code, http.StatusInternalServerError, cancelW.Body.String())
	}

	child, err := client.Ticket.Get(t.Context(), children[0].ID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusPENDING {
		t.Fatalf("child status after rollback = %q, want %q", child.Status, entticket.StatusPENDING)
	}
}

func TestBatchHandler_CancelVMBatch_ReturnsErrorWhenParentStatusSyncFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}
	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("parent batch projection unavailable")),
		ent.OpUpdateOne,
	))

	cancelCtx, cancelW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/cancel", "", "owner-1", []string{"vm:delete"})
	srv.CancelVMBatch(cancelCtx, submitResp.BatchId)
	if cancelW.Code != http.StatusInternalServerError {
		t.Fatalf("cancel status = %d, want %d body=%s", cancelW.Code, http.StatusInternalServerError, cancelW.Body.String())
	}
}

func TestBatchHandler_GetVMBatch_ReturnsErrorWhenProjectionSyncFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, _ := mustSeedPowerBatchForRetry(t, client, "START")
	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("batch projection sync unavailable")),
		ent.OpUpdateOne,
	))

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+batchID, "", "owner-1", []string{"vm:read"})
	srv.GetVMBatch(getCtx, batchID)

	if getW.Code != http.StatusInternalServerError {
		t.Fatalf("get status = %d, want %d body=%s", getW.Code, http.StatusInternalServerError, getW.Body.String())
	}
	assertErrorCode(t, getW.Body.Bytes(), "INTERNAL_ERROR")
}

func TestBatchHandler_GetVMBatch_ReturnsErrorWhenProjectionBackfillFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, _ := mustSeedPowerBatchForRetry(t, client, "START")
	if err := client.BatchTicket.DeleteOneID(batchID).Exec(t.Context()); err != nil {
		t.Fatalf("delete batch projection: %v", err)
	}
	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("batch projection backfill unavailable")),
		ent.OpCreate,
	))

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+batchID, "", "owner-1", []string{"vm:read"})
	srv.GetVMBatch(getCtx, batchID)

	if getW.Code != http.StatusInternalServerError {
		t.Fatalf("get status = %d, want %d body=%s", getW.Code, http.StatusInternalServerError, getW.Body.String())
	}
	assertErrorCode(t, getW.Body.Bytes(), "INTERNAL_ERROR")
}

func TestBatchHandler_GetVMBatch_HidesOtherUsersBatch(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}
	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+submitResp.BatchId, "", "other-user", []string{"vm:read"})
	srv.GetVMBatch(getCtx, submitResp.BatchId)
	if getW.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", getW.Code, http.StatusNotFound, getW.Body.String())
	}
	assertErrorCode(t, getW.Body.Bytes(), "BATCH_NOT_FOUND")
}

func TestBatchHandler_GetVMBatch_CreateChildIncludesProvisioningWhenVMExists(t *testing.T) {
	t.Parallel()

	baseSrv, client := newBatchBehaviorTestServer(t)
	serviceID, templateID, sizeID := mustCreateBatchCreatePrerequisites(t, client, "requester-1", "team-prod")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("CREATE"),
		Items: []generated.VMBatchChildItem{
			{
				ServiceId:      serviceID,
				TemplateId:     templateID,
				InstanceSizeId: sizeID,
				Namespace:      "team-prod",
				Reason:         "create one",
			},
		},
	})

	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"requester-1",
		[]string{"platform:admin"},
	)
	baseSrv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}

	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	child := children[0]

	vmID := "vm-" + uuid.NewString()
	vmName := "vm" + vmID[len(vmID)-4:]
	dvUID := "dv-" + uuid.NewString()
	svcID := mustCreateServiceForVM(t, client, "requester-1")
	if _, err := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusPENDING).
		SetCreatedBy("requester-1").
		SetClusterID("cluster-a").
		SetTicketID(child.ID).
		SetServiceID(svcID).
		Save(t.Context()); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.SeedDataVolumes([]*domain.DataVolume{
		{
			Name:         vmName + "-rootfs",
			Namespace:    "team-prod",
			UID:          dvUID,
			ClaimName:    vmName + "-rootfs",
			Phase:        "CloneInProgress",
			Progress:     "33.0%",
			RestartCount: 0,
		},
	})
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{
		{
			Name:      vmName + "-rootfs",
			Namespace: "team-prod",
			Phase:     "Bound",
		},
	})

	srv := NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	})

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+submitResp.BatchId, "", "requester-1", []string{"vm:read", "platform:admin"})
	srv.GetVMBatch(getCtx, submitResp.BatchId)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d body=%s", getW.Code, http.StatusOK, getW.Body.String())
	}

	var resp generated.VMBatchStatusResponse
	if err := json.Unmarshal(getW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if len(resp.Children) != 1 {
		t.Fatalf("children len = %d, want 1", len(resp.Children))
	}
	if resp.Children[0].Provisioning == nil {
		t.Fatal("child provisioning = nil, want non-nil")
	}
	if resp.Children[0].Provisioning.Phase != "CloneInProgress" {
		t.Fatalf("child provisioning phase = %q, want %q", resp.Children[0].Provisioning.Phase, "CloneInProgress")
	}
}

func TestBatchHandler_SubmitCreate_ForbiddenWhenNamespaceInvisible(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	serviceID, templateID, sizeID := mustCreateBatchCreatePrerequisites(t, client, "requester-1", "team-prod")
	serviceRow, err := client.Service.Get(t.Context(), serviceID.String())
	if err != nil {
		t.Fatalf("get create batch service: %v", err)
	}
	systemRow, err := serviceRow.QuerySystem().Only(t.Context())
	if err != nil {
		t.Fatalf("get create batch service system: %v", err)
	}
	mustCreateSystemBinding(t, client, "requester-1", systemRow.ID, "member")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("CREATE"),
		Items: []generated.VMBatchChildItem{
			{
				ServiceId:      serviceID,
				TemplateId:     templateID,
				InstanceSizeId: sizeID,
				Namespace:      "team-prod",
				Reason:         "create one",
			},
		},
	})

	// Non-admin without role bindings -> visibility resolves to fail-closed (no env visibility).
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"requester-1",
		[]string{"vm:create"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusForbidden {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusForbidden, submitW.Body.String())
	}
	assertErrorCode(t, submitW.Body.Bytes(), "NAMESPACE_ENV_FORBIDDEN")
}

func TestBatchHandler_RetryVMBatch_Errors(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)

	{
		c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/retry", "", "", nil)
		srv.RetryVMBatch(c, "batch-1")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
	}

	{
		c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/not-exist/retry", "", "user-a", []string{"vm:delete"})
		srv.RetryVMBatch(c, "not-exist")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "BATCH_NOT_FOUND")
	}

	{
		client := srv.client
		vmID := mustCreateBatchDeleteTargetVM(t, client)
		submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchOperation("DELETE"),
			Items: []generated.VMBatchChildItem{
				{VmId: vmID},
			},
		})
		submitCtx, submitW := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch",
			submitBody,
			"owner-1",
			[]string{"platform:admin"},
		)
		srv.SubmitVMBatch(submitCtx)
		if submitW.Code != http.StatusAccepted {
			t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
		}
		var submitResp generated.VMBatchSubmitResponse
		if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
			t.Fatalf("decode submit response: %v", err)
		}

		c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/retry", "", "owner-1", []string{"vm:delete"})
		srv.RetryVMBatch(c, submitResp.BatchId)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "BATCH_NOTHING_TO_RETRY")
	}
}

func TestBatchHandler_GetVMBatch_RequestContextCanceled(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/batch/batch-cancelled", "", "user-a", []string{"vm:read"})
	reqCtx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(reqCtx)

	srv.GetVMBatch(c, "batch-cancelled")

	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for canceled request, got %q", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for canceled request", w.Code, http.StatusOK)
	}
}

func TestBatchHandler_ListVMBatches_RequestContextCanceled(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/batch", "", "user-a", []string{"vm:read"})
	reqCtx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(reqCtx)

	srv.ListVMBatches(c, generated.ListVMBatchesParams{})

	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for canceled request, got %q", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for canceled request", w.Code, http.StatusOK)
	}
}

func TestBatchHandler_ListVMBatches_RejectsInvalidSortOrder(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/batch?sort_order=sideways", "", "user-a", []string{"vm:read"})

	srv.ListVMBatches(c, generated.ListVMBatchesParams{
		SortOrder: generated.ListVMBatchesParamsSortOrder("sideways"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestBatchHandler_ListVMBatches_RejectsInvalidSortBy(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/batch?sort_by=namespace", "", "user-a", []string{"vm:read"})

	srv.ListVMBatches(c, generated.ListVMBatchesParams{
		SortBy: "namespace",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestBatchHandler_ListVMBatches_SortsByFailedCountWhenRequested(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	_, err := client.BatchTicket.Create().
		SetID("batch-low-fail-" + uuid.NewString()).
		SetBatchType(entbatchticket.BatchTypeBATCH_CREATE).
		SetChildCount(5).
		SetSuccessCount(4).
		SetFailedCount(1).
		SetPendingCount(0).
		SetStatus(entbatchticket.StatusPARTIAL_SUCCESS).
		SetCreatedBy("user-a").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create low-fail batch: %v", err)
	}
	highFail, err := client.BatchTicket.Create().
		SetID("batch-high-fail-" + uuid.NewString()).
		SetBatchType(entbatchticket.BatchTypeBATCH_CREATE).
		SetChildCount(5).
		SetSuccessCount(2).
		SetFailedCount(3).
		SetPendingCount(0).
		SetStatus(entbatchticket.StatusPARTIAL_SUCCESS).
		SetCreatedBy("user-a").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create high-fail batch: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/vms/batch?sort_by=failed_count&sort_order=desc", "", "user-a", []string{"vm:read"})
	srv.ListVMBatches(c, generated.ListVMBatchesParams{
		SortBy:    "failed_count",
		SortOrder: generated.ListVMBatchesParamsSortOrderDesc,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMBatchList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) < 2 {
		t.Fatalf("items len = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Id != highFail.ID {
		t.Fatalf("first batch id = %q, want %q", resp.Items[0].Id, highFail.ID)
	}
}

func TestBatchHandler_RetryVMBatch_RequestContextCanceled(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-cancelled/retry", "", "user-a", []string{"vm:delete"})
	reqCtx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(reqCtx)

	srv.RetryVMBatch(c, "batch-cancelled")

	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for canceled request, got %q", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for canceled request", w.Code, http.StatusOK)
	}
}

func TestBatchHandler_SubmitVMBatch_IdempotentByRequestID(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		RequestId: "req-123",
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c1, w1 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first submit status = %d, want %d body=%s", w1.Code, http.StatusAccepted, w1.Body.String())
	}
	var r1 generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("decode first submit: %v", err)
	}

	c2, w2 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c2)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second submit status = %d, want %d body=%s", w2.Code, http.StatusAccepted, w2.Body.String())
	}
	var r2 generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("decode second submit: %v", err)
	}
	if r2.BatchId != r1.BatchId {
		t.Fatalf("idempotent batch_id = %q, want %q", r2.BatchId, r1.BatchId)
	}

	parentCount, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDIsNil()).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count parent tickets: %v", err)
	}
	if parentCount != 1 {
		t.Fatalf("parent ticket count = %d, want 1", parentCount)
	}
}

func TestBatchHandler_SubmitVMBatch_IdempotentReturnsCurrentStatus(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		RequestId: "req-current-" + uuid.NewString(),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c1, w1 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first submit status = %d, want %d body=%s", w1.Code, http.StatusAccepted, w1.Body.String())
	}
	var firstResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w1.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(firstResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if _, err := client.Ticket.UpdateOneID(children[0].ID).
		SetStatus(entticket.StatusSUCCESS).
		Save(t.Context()); err != nil {
		t.Fatalf("seed child success: %v", err)
	}

	c2, w2 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c2)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second submit status = %d, want %d body=%s", w2.Code, http.StatusAccepted, w2.Body.String())
	}
	var secondResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if secondResp.BatchId != firstResp.BatchId {
		t.Fatalf("idempotent batch_id = %q, want %q", secondResp.BatchId, firstResp.BatchId)
	}
	if secondResp.Status != generated.VMBatchParentStatusCOMPLETED {
		t.Fatalf("idempotent status = %q, want %q", secondResp.Status, generated.VMBatchParentStatusCOMPLETED)
	}
}

func TestBatchHandler_SubmitVMBatch_IdempotentReturnsErrorWhenExistingViewFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		RequestId: "req-fail-" + uuid.NewString(),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c1, w1 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first submit status = %d, want %d body=%s", w1.Code, http.StatusAccepted, w1.Body.String())
	}

	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("existing batch projection unavailable")),
		ent.OpUpdateOne,
	))
	c2, w2 := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c2)

	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("second submit status = %d, want %d body=%s", w2.Code, http.StatusInternalServerError, w2.Body.String())
	}
	assertErrorCode(t, w2.Body.Bytes(), "INTERNAL_ERROR")
}

func TestBatchHandler_SubmitVMBatch_RateLimitedByPendingParentCount(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	for i := range maxPendingBatchParentsUser {
		_, err := client.DomainEvent.Create().
			SetID("ev-pending-" + uuid.NewString()).
			SetEventType(string(domain.EventBatchDeleteRequested)).
			SetAggregateType("batch").
			SetAggregateID("batch-pending-" + uuid.NewString()).
			SetPayload([]byte(`{"seed":true}`)).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy("owner-1").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed pending parent event #%d: %v", i+1, err)
		}
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "BATCH_RATE_LIMITED")
}

func TestBatchHandler_SubmitVMBatch_RateLimitedByGlobalRecentSubmitCount(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	for i := range maxGlobalBatchRequestsPerMinute {
		_, err := client.DomainEvent.Create().
			SetID("ev-global-" + uuid.NewString()).
			SetEventType(string(domain.EventBatchDeleteRequested)).
			SetAggregateType("batch").
			SetAggregateID("batch-global-" + uuid.NewString()).
			SetPayload([]byte(`{"seed":true}`)).
			SetStatus(domainevent.StatusCOMPLETED).
			SetCreatedBy("seed-user").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed global recent event #%d: %v", i+1, err)
		}
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "BATCH_RATE_LIMITED")
}

func TestBatchHandler_SubmitVMBatchPower_InvalidOperation(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("hibernate"),
		Items: []generated.VMBatchPowerItem{
			{VmId: "vm-any"},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_BATCH_OPERATION")
}

func TestBatchHandler_SubmitVMBatch_RateLimitedByPendingChildCount(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	for i := range maxPendingBatchChildrenUser {
		_, err := client.Ticket.Create().
			SetID("child-pending-" + uuid.NewString()).
			SetEventID("event-" + uuid.NewString()).
			SetRequester("owner-1").
			SetStatus(entticket.StatusPENDING).
			SetParentTicketID("parent-seed").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed pending child ticket #%d: %v", i+1, err)
		}
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "BATCH_RATE_LIMITED")
}

func TestBatchHandler_SubmitVMBatch_RateLimitedByCooldown(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	_, err := client.DomainEvent.Create().
		SetID("ev-cooldown-" + uuid.NewString()).
		SetEventType(string(domain.EventBatchDeleteRequested)).
		SetAggregateType("batch").
		SetAggregateID("batch-cooldown-" + uuid.NewString()).
		SetPayload([]byte(`{"request_id":"old","operation":"DELETE","items":[]}`)).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("owner-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed cooldown domain event: %v", err)
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "BATCH_RATE_LIMITED")
}

func TestBatchHandler_SubmitVMBatch_DeleteNotRateLimitedByRecentPowerBatch(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	_, err := client.DomainEvent.Create().
		SetID("ev-power-cooldown-" + uuid.NewString()).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID("batch-power-cooldown-" + uuid.NewString()).
		SetPayload([]byte(`{"request_id":"old-power","operation":"STOP","items":[]}`)).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("owner-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed power cooldown domain event: %v", err)
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}
}

func TestBatchHandler_RetryVMBatch_RetriesFailedDeleteChild(t *testing.T) {
	t.Parallel()

	writer := &fakeDeleteAtomicWriter{}
	srv, client := newBatchBehaviorTestServerWithTicketService(t, writer)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}
	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	child := children[0]

	if _, updateErr := client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("seed failure").
		Save(t.Context()); updateErr != nil {
		t.Fatalf("seed child failed status: %v", updateErr)
	}

	retryCtx, retryW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/retry", "", "owner-1", []string{"vm:delete"})
	srv.RetryVMBatch(retryCtx, submitResp.BatchId)
	if retryW.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusOK, retryW.Body.String())
	}

	var retryResp generated.VMBatchActionResponse
	if decodeErr := json.Unmarshal(retryW.Body.Bytes(), &retryResp); decodeErr != nil {
		t.Fatalf("decode retry response: %v", decodeErr)
	}
	if retryResp.AffectedCount != 1 {
		t.Fatalf("affected_count = %d, want 1", retryResp.AffectedCount)
	}
	if len(retryResp.AffectedTicketIds) != 1 || retryResp.AffectedTicketIds[0] != child.ID {
		t.Fatalf("affected_ticket_ids = %v, want [%s]", retryResp.AffectedTicketIds, child.ID)
	}
	if retryResp.Status == generated.VMBatchParentStatusFAILED || retryResp.Status == generated.VMBatchParentStatusCANCELLED {
		t.Fatalf("retry status = %q, want active status", retryResp.Status)
	}
	parentTicket, err := client.Ticket.Get(t.Context(), submitResp.BatchId)
	if err != nil {
		t.Fatalf("load parent ticket: %v", err)
	}
	if parentTicket.Status != entticket.StatusEXECUTING {
		t.Fatalf("parent ticket status = %q, want %q", parentTicket.Status, entticket.StatusEXECUTING)
	}
	parentEvent, err := client.DomainEvent.Get(t.Context(), parentTicket.EventID)
	if err != nil {
		t.Fatalf("load parent event: %v", err)
	}
	if parentEvent.Status != domainevent.StatusPROCESSING {
		t.Fatalf("parent event status = %q, want %q", parentEvent.Status, domainevent.StatusPROCESSING)
	}
	if writer.deleteCalls != 1 {
		t.Fatalf("delete atomic writer calls = %d, want 1", writer.deleteCalls)
	}
}

func TestBatchHandler_RetryVMBatch_UsesApprovalProviderSeam(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv, client := newBatchBehaviorTestServerWithApprovalRouter(t, capture)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}

	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	child := children[0]

	if _, err := client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("seed failure").
		Save(t.Context()); err != nil {
		t.Fatalf("seed child failed status: %v", err)
	}

	retryCtx, retryW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/retry", "", "owner-1", []string{"vm:delete"})
	srv.RetryVMBatch(retryCtx, submitResp.BatchId)
	if retryW.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusOK, retryW.Body.String())
	}

	if capture.processCalled != 1 {
		t.Fatalf("approval provider processCalled = %d, want 1", capture.processCalled)
	}
	if capture.lastTicketID != child.ID {
		t.Fatalf("approval provider lastTicketID = %q, want %q", capture.lastTicketID, child.ID)
	}
	if !capture.lastDecision.Approved {
		t.Fatal("approval provider lastDecision.Approved = false, want true")
	}
}

func TestBatchHandler_RetryVMBatch_RollsBackChildFailureMarkerWhenEventFailWriteFails(t *testing.T) {
	t.Parallel()

	approvalFailure := errors.New("approval retry failed")
	capture := &failingApprovalProvider{err: approvalFailure}
	srv, client := newBatchBehaviorTestServerWithApprovalProvider(t, capture)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("DELETE"),
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}
	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	child := children[0]
	if _, updateErr := client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("seed failure").
		Save(t.Context()); updateErr != nil {
		t.Fatalf("seed child failed status: %v", updateErr)
	}
	client.DomainEvent.Use(failDomainEventStatusUpdateHook(
		domainevent.StatusFAILED,
		errors.New("child event failure marker unavailable"),
	))

	retryCtx, retryW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/retry", "", "owner-1", []string{"vm:delete"})
	srv.RetryVMBatch(retryCtx, submitResp.BatchId)
	if retryW.Code != http.StatusInternalServerError {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusInternalServerError, retryW.Body.String())
	}
	if capture.processCalled != 1 {
		t.Fatalf("approval provider processCalled = %d, want 1", capture.processCalled)
	}

	updatedChild, err := client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if updatedChild.Status != entticket.StatusPENDING {
		t.Fatalf("child status after rollback = %q, want %q", updatedChild.Status, entticket.StatusPENDING)
	}
	if updatedChild.RejectReason != "" {
		t.Fatalf("child reject_reason after rollback = %q, want empty", updatedChild.RejectReason)
	}

	updatedEvent, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event: %v", err)
	}
	if updatedEvent.Status != domainevent.StatusPENDING {
		t.Fatalf("child event status after rollback = %q, want %q", updatedEvent.Status, domainevent.StatusPENDING)
	}
}

func TestBatchHandler_RetryVMBatch_UsesReviewBodyForFailedCreateBatch(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv, client := newBatchBehaviorTestServerWithApprovalRouter(t, capture)
	batchID, childID := mustSubmitFailedCreateBatchForRetry(t, srv, client, "owner-1")

	retryBody := mustJSON(t, generated.ApprovalDecisionRequest{
		SelectedClusterId:     "cluster-review",
		SelectedStorageClass:  "gold-sc",
		SelectedDvAccessModes: []string{"ReadWriteOnce"},
		SelectedDvVolumeMode:  generated.ApprovalDecisionRequestSelectedDvVolumeModeFilesystem,
		EnableOverride:        true,
		CpuRequest:            2,
		CpuLimit:              4,
		MemoryRequestGi:       8,
		MemoryLimitGi:         16,
		DiskGb:                120,
	})

	retryCtx, retryW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		retryBody,
		"admin-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	srv.RetryVMBatch(retryCtx, batchID)
	if retryW.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusOK, retryW.Body.String())
	}

	if capture.processCalled != 1 {
		t.Fatalf("approval provider processCalled = %d, want 1", capture.processCalled)
	}
	if capture.lastTicketID != childID {
		t.Fatalf("approval provider lastTicketID = %q, want %q", capture.lastTicketID, childID)
	}
	if capture.lastDecision.Execution.ClusterID != "cluster-review" {
		t.Fatalf("execution cluster_id = %q, want cluster-review", capture.lastDecision.Execution.ClusterID)
	}
	if capture.lastDecision.Execution.StorageClass != "gold-sc" {
		t.Fatalf("execution storage_class = %q, want gold-sc", capture.lastDecision.Execution.StorageClass)
	}
	if len(capture.lastDecision.Execution.DVAccessModes) != 1 || capture.lastDecision.Execution.DVAccessModes[0] != "ReadWriteOnce" {
		t.Fatalf("execution dv_access_modes = %v, want [ReadWriteOnce]", capture.lastDecision.Execution.DVAccessModes)
	}
	if capture.lastDecision.Execution.DVVolumeMode != "Filesystem" {
		t.Fatalf("execution dv_volume_mode = %q, want Filesystem", capture.lastDecision.Execution.DVVolumeMode)
	}
	if !capture.lastDecision.Execution.EnableOverride {
		t.Fatal("execution EnableOverride = false, want true")
	}
	if capture.lastDecision.Execution.CPURequest != 2 {
		t.Fatalf("execution cpu_request = %v, want 2", capture.lastDecision.Execution.CPURequest)
	}
	if capture.lastDecision.Execution.MemoryRequestGi != 8 {
		t.Fatalf("execution memory_request_gi = %v, want 8", capture.lastDecision.Execution.MemoryRequestGi)
	}
	if capture.lastDecision.Execution.DiskGB != 120 {
		t.Fatalf("execution disk_gb = %d, want 120", capture.lastDecision.Execution.DiskGB)
	}

	parentTicket, err := client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load parent ticket: %v", err)
	}
	if parentTicket.SelectedClusterID != "cluster-review" {
		t.Fatalf("parent selected_cluster_id = %q, want cluster-review", parentTicket.SelectedClusterID)
	}
	if parentTicket.SelectedStorageClass != "gold-sc" {
		t.Fatalf("parent selected_storage_class = %q, want gold-sc", parentTicket.SelectedStorageClass)
	}
}

func TestBatchHandler_RetryVMBatch_RollsBackReviewInputsWhenChildResetFails(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv, client := newBatchBehaviorTestServerWithApprovalRouter(t, capture)
	batchID, childID := mustSubmitFailedCreateBatchForRetry(t, srv, client, "owner-1")
	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("child event reset unavailable")),
		ent.OpUpdate,
	))

	retryBody := mustJSON(t, generated.ApprovalDecisionRequest{
		SelectedClusterId:    "cluster-review",
		SelectedStorageClass: "gold-sc",
	})
	retryCtx, retryW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		retryBody,
		"admin-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	srv.RetryVMBatch(retryCtx, batchID)
	if retryW.Code != http.StatusInternalServerError {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusInternalServerError, retryW.Body.String())
	}
	if capture.processCalled != 0 {
		t.Fatalf("approval provider processCalled = %d, want 0", capture.processCalled)
	}

	parentTicket, err := client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load parent ticket: %v", err)
	}
	if parentTicket.SelectedClusterID != "" {
		t.Fatalf("parent selected_cluster_id = %q, want empty after rollback", parentTicket.SelectedClusterID)
	}
	if parentTicket.SelectedStorageClass != "" {
		t.Fatalf("parent selected_storage_class = %q, want empty after rollback", parentTicket.SelectedStorageClass)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("load child ticket: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if child.RejectReason != "seed failure" {
		t.Fatalf("child reject_reason = %q, want seed failure", child.RejectReason)
	}
	childEvent, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("load child event: %v", err)
	}
	if childEvent.Status != domainevent.StatusFAILED {
		t.Fatalf("child event status = %q, want %q", childEvent.Status, domainevent.StatusFAILED)
	}
}

func TestBatchHandler_RetryVMBatch_RequiresReviewWhenFailedCreateBatchHasNoSelectedCluster(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv, client := newBatchBehaviorTestServerWithApprovalRouter(t, capture)
	batchID, childID := mustSubmitFailedCreateBatchForRetry(t, srv, client, "owner-1")

	retryCtx, retryW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"admin-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	srv.RetryVMBatch(retryCtx, batchID)
	if retryW.Code != http.StatusConflict {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusConflict, retryW.Body.String())
	}
	assertErrorCode(t, retryW.Body.Bytes(), "BATCH_RETRY_REVIEW_REQUIRED")

	var resp generated.Error
	if err := json.Unmarshal(retryW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if len(resp.FieldErrors) != 1 || resp.FieldErrors[0].Field != "selected_cluster_id" {
		t.Fatalf("field_errors = %+v, want selected_cluster_id", resp.FieldErrors)
	}
	if capture.processCalled != 0 {
		t.Fatalf("approval provider processCalled = %d, want 0", capture.processCalled)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("load child ticket: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
}

func TestBatchHandler_SubmitVMBatchPower_AtomicEnqueueFailureRollsBack(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		Items: []generated.VMBatchPowerItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	ticketCount, err := client.Ticket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if ticketCount != 0 {
		t.Fatalf("ticket count = %d, want 0", ticketCount)
	}
	eventCount, err := client.DomainEvent.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count domain events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("domain event count = %d, want 0", eventCount)
	}
	batchCount, err := client.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count batch tickets: %v", err)
	}
	if batchCount != 0 {
		t.Fatalf("batch ticket count = %d, want 0", batchCount)
	}
}

func TestBatchHandler_SubmitVMBatchPower_TestBatchAtomicallyEnqueues(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		Items: []generated.VMBatchPowerItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.BatchId == "" {
		t.Fatal("batch_id is empty")
	}
	if resp.Status != generated.VMBatchParentStatusINPROGRESS {
		t.Fatalf("status = %q, want %q", resp.Status, generated.VMBatchParentStatusINPROGRESS)
	}
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(resp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].Status != entticket.StatusEXECUTING {
		t.Fatalf("child status = %q, want %q", children[0].Status, entticket.StatusEXECUTING)
	}
}

func TestBatchHandler_SubmitVMBatchPower_IdempotentReturnsErrorWhenExistingViewFails(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		RequestId: "power-req-" + uuid.NewString(),
		Items: []generated.VMBatchPowerItem{
			{VmId: vmID},
		},
	})
	c1, w1 := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first submit status = %d, want %d body=%s", w1.Code, http.StatusAccepted, w1.Body.String())
	}

	client.BatchTicket.Use(enthook.On(
		enthook.FixedError(errors.New("existing power batch projection unavailable")),
		ent.OpUpdateOne,
	))
	c2, w2 := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c2)

	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("second submit status = %d, want %d body=%s", w2.Code, http.StatusInternalServerError, w2.Body.String())
	}
	assertErrorCode(t, w2.Body.Bytes(), "INTERNAL_ERROR")
}

func TestBatchHandler_RetryVMBatch_PowerChildUnknownOperation(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+batchID+"/retry", "", "owner-1", []string{"vm:operate"})
	srv.RetryVMBatch(c, batchID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMBatchActionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AffectedCount != 0 {
		t.Fatalf("affected_count = %d, want 0", resp.AffectedCount)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if !strings.Contains(child.RejectReason, "unknown power operation for retry") {
		t.Fatalf("child reject_reason = %q, want contains %q", child.RejectReason, "unknown power operation for retry")
	}
}

func TestBatchHandler_RetryVMBatch_PowerChildFailureMarkerRollsBackTicketOnEventPersistFailure(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")
	client.DomainEvent.Use(enthook.On(
		enthook.FixedError(errors.New("child event failure marker unavailable")),
		ent.OpUpdate,
	))

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+batchID+"/retry", "", "owner-1", []string{"vm:operate"})
	srv.RetryVMBatch(c, batchID)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if child.RejectReason != "seed failure" {
		t.Fatalf("child reject_reason = %q, want seed failure after rollback", child.RejectReason)
	}
}

func TestBatchHandler_MarkBatchChildFailed_RejectsTicketEventMismatch(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	_, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	err = srv.markBatchChildFailed(t.Context(), child.ID, child.EventID+"-other", "new failure")
	if err == nil {
		t.Fatal("markBatchChildFailed() error = nil, want event mismatch error")
	}
	if !strings.Contains(err.Error(), "expected 1 row, got 0") {
		t.Fatalf("markBatchChildFailed() error = %v, want zero-row error", err)
	}

	child, err = client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket after mismatch: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if child.RejectReason != "seed failure" {
		t.Fatalf("child reject_reason = %q, want seed failure", child.RejectReason)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event: %v", err)
	}
	if event.Status != domainevent.StatusFAILED {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusFAILED)
	}
}

func TestBatchHandler_MarkBatchChildFailed_RejectsStaleTerminalTicket(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	_, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if _, updateErr := client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusSUCCESS).
		ClearRejectReason().
		Save(t.Context()); updateErr != nil {
		t.Fatalf("seed stale terminal child ticket: %v", updateErr)
	}

	err = srv.markBatchChildFailed(t.Context(), child.ID, child.EventID, "new failure")
	if err == nil {
		t.Fatal("markBatchChildFailed() error = nil, want stale ticket error")
	}
	if !strings.Contains(err.Error(), "expected 1 row, got 0") {
		t.Fatalf("markBatchChildFailed() error = %v, want zero-row error", err)
	}

	child, err = client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket after stale marker: %v", err)
	}
	if child.Status != entticket.StatusSUCCESS {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusSUCCESS)
	}
	if child.RejectReason != "" {
		t.Fatalf("child reject_reason = %q, want empty", child.RejectReason)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event: %v", err)
	}
	if event.Status != domainevent.StatusFAILED {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusFAILED)
	}
}

func TestBatchHandler_MarkBatchChildFailed_RollsBackTicketWhenEventTerminal(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	_, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if _, updateErr := client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); updateErr != nil {
		t.Fatalf("seed stale terminal child event: %v", updateErr)
	}

	err = srv.markBatchChildFailed(t.Context(), child.ID, child.EventID, "new failure")
	if err == nil {
		t.Fatal("markBatchChildFailed() error = nil, want stale event error")
	}
	if !strings.Contains(err.Error(), "expected 1 row, got 0") {
		t.Fatalf("markBatchChildFailed() error = %v, want zero-row error", err)
	}

	child, err = client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket after rollback: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if child.RejectReason != "seed failure" {
		t.Fatalf("child reject_reason = %q, want seed failure after rollback", child.RejectReason)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event after rollback: %v", err)
	}
	if event.Status != domainevent.StatusCOMPLETED {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusCOMPLETED)
	}
}

func TestBatchHandler_UpdateBatchChildStatus_RollsBackTicketWhenEventDoesNotMatchExpectedStatus(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	_, childID := mustSeedPowerBatchForRetry(t, client, "hibernate")

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	err = srv.updateBatchChildTicketAndEventStatus(
		t.Context(),
		[]batchChildTicketEventRef{{TicketID: child.ID, EventID: child.EventID}},
		entticket.StatusPENDING,
		domainevent.StatusPENDING,
		true,
		[]entticket.Status{entticket.StatusFAILED},
		[]domainevent.Status{domainevent.StatusCANCELLED},
	)
	if err == nil {
		t.Fatal("updateBatchChildTicketAndEventStatus() error = nil, want event status mismatch error")
	}
	if !strings.Contains(err.Error(), "expected 1 row, got 0") {
		t.Fatalf("updateBatchChildTicketAndEventStatus() error = %v, want zero-row error", err)
	}

	child, err = client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket after rollback: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if child.RejectReason != "seed failure" {
		t.Fatalf("child reject_reason = %q, want seed failure after rollback", child.RejectReason)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event after rollback: %v", err)
	}
	if event.Status != domainevent.StatusFAILED {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusFAILED)
	}
}

func TestBatchHandler_RetryVMBatch_PowerChildEnqueueFailure(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "start")

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+batchID+"/retry", "", "owner-1", []string{"vm:operate"})
	srv.RetryVMBatch(c, batchID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMBatchActionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AffectedCount != 0 {
		t.Fatalf("affected_count = %d, want 0", resp.AffectedCount)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusFAILED)
	}
	if !strings.Contains(child.RejectReason, "failed to enqueue vm_power job") {
		t.Fatalf("child reject_reason = %q, want contains %q", child.RejectReason, "failed to enqueue vm_power job")
	}
}

func TestBatchHandler_RetryVMBatch_PowerChildAtomicallyEnqueues(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServerWithRiver(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "start")

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+batchID+"/retry", "", "owner-1", []string{"vm:operate"})
	srv.RetryVMBatch(c, batchID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMBatchActionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AffectedCount != 1 {
		t.Fatalf("affected_count = %d, want 1", resp.AffectedCount)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusEXECUTING {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusEXECUTING)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event: %v", err)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusPENDING)
	}
}

func TestBatchHandler_RetryVMBatch_PowerRejectedChildAtomicallyEnqueues(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServerWithRiver(t)
	batchID, childID := mustSeedPowerBatchForRetryWithStatuses(
		t,
		client,
		"start",
		entticket.StatusREJECTED,
		domainevent.StatusCANCELLED,
	)

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+batchID+"/retry", "", "owner-1", []string{"vm:operate"})
	srv.RetryVMBatch(c, batchID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.VMBatchActionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AffectedCount != 1 {
		t.Fatalf("affected_count = %d, want 1", resp.AffectedCount)
	}

	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != entticket.StatusEXECUTING {
		t.Fatalf("child status = %q, want %q", child.Status, entticket.StatusEXECUTING)
	}
	if child.RejectReason != "" {
		t.Fatalf("child reject_reason = %q, want empty", child.RejectReason)
	}
	event, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("query child event: %v", err)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want %q", event.Status, domainevent.StatusPENDING)
	}
}

func TestBatchHandler_SubmitVMBatchPower_ProdBatchStaysPendingApproval(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentProd)

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		Items: []generated.VMBatchPowerItem{
			{VmId: vmID},
		},
	})
	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", body, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatchPower(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != generated.VMBatchParentStatusPENDINGAPPROVAL {
		t.Fatalf("status = %q, want %q", resp.Status, generated.VMBatchParentStatusPENDINGAPPROVAL)
	}

	parent, err := client.Ticket.Get(t.Context(), resp.BatchId)
	if err != nil {
		t.Fatalf("query parent ticket: %v", err)
	}
	if parent.OperationType != entticket.OperationTypePOWER {
		t.Fatalf("parent operation_type = %q, want %q", parent.OperationType, entticket.OperationTypePOWER)
	}
	if parent.Status != entticket.StatusPENDING {
		t.Fatalf("parent status = %q, want %q", parent.Status, entticket.StatusPENDING)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(resp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].OperationType != entticket.OperationTypePOWER {
		t.Fatalf("child operation_type = %q, want %q", children[0].OperationType, entticket.OperationTypePOWER)
	}
	if children[0].Status != entticket.StatusPENDING {
		t.Fatalf("child status = %q, want %q", children[0].Status, entticket.StatusPENDING)
	}
}

func newBatchBehaviorTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client, pool := newBatchBehaviorTestStore(t, "batch_handler_behavior")
	return NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		ApprovalReqs: service.NewApprovalRequirementService(client),
	}), client
}

func newBatchBehaviorTestServerWithRiver(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client, pool := newBatchBehaviorTestStore(t, "r")
	return NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		RiverClient:  newBatchBehaviorTestRiverClient(t, pool),
		ApprovalReqs: service.NewApprovalRequirementService(client),
	}), client
}

func newBatchBehaviorTestServerWithTicketService(t *testing.T, writer *fakeDeleteAtomicWriter) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client, pool := newBatchBehaviorTestStore(t, "batch_handler_behavior_with_ticket_service")
	ticketService := ticketing.NewService(client, nil, writer)
	return NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		ApprovalReqs:  service.NewApprovalRequirementService(client),
		TicketService: ticketService,
	}), client
}

func newBatchBehaviorTestServerWithApprovalRouter(t *testing.T, capture *captureApprovalProvider) (*Server, *ent.Client) {
	return newBatchBehaviorTestServerWithApprovalProvider(t, capture)
}

func newBatchBehaviorTestServerWithApprovalProvider(t *testing.T, approvalProvider provider.ApprovalProvider) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client, pool := newBatchBehaviorTestStore(t, "batch_handler_behavior_with_provider_router")
	return NewServer(ServerDeps{
		EntClient:      client,
		Pool:           pool,
		ApprovalReqs:   service.NewApprovalRequirementService(client),
		ApprovalRouter: approval.NewApprovalProviderRouter(approvalProvider),
	}), client
}

type failingApprovalProvider struct {
	captureApprovalProvider
	err error
}

func (p *failingApprovalProvider) ProcessApproval(ctx context.Context, ticketID string, decision provider.ApprovalDecision) error {
	_ = p.captureApprovalProvider.ProcessApproval(ctx, ticketID, decision)
	if p.err != nil {
		return p.err
	}
	return errors.New("approval failed")
}

func failDomainEventStatusUpdateHook(status domainevent.Status, err error) func(ent.Mutator) ent.Mutator {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			domainEventMutation, ok := mutation.(*ent.DomainEventMutation)
			if ok {
				nextStatus, exists := domainEventMutation.Status()
				if exists && nextStatus == status {
					return nil, err
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func newBatchBehaviorTestStore(t *testing.T, prefix string) (*ent.Client, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.OpenPGXPool(t, prefix)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	return client, pool
}

func newBatchBehaviorTestRiverClient(t *testing.T, pool *pgxpool.Pool) *river.Client[pgx.Tx] {
	t.Helper()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	return riverClient
}

func newBatchModifyTestServer(t *testing.T) (*Server, *ent.Client, string) {
	t.Helper()

	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "batch_handler_modify")

	clusterID := "cluster-" + uuid.NewString()
	_, err := client.Cluster.Create().
		SetID(clusterID).
		SetName("cluster-" + clusterID[len(clusterID)-4:]).
		SetDisplayName("Cluster Modify").
		SetAPIServerURL("https://k8s.example.com").
		SetEncryptedKubeconfig([]byte("fake-kubeconfig")).
		SetCreatedBy("seed").
		SetEnvironment(entcluster.EnvironmentProd).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnabled(true).
		SetEnabledFeatures([]string{"VMLiveUpdateFeatures", "ExpandDisks"}).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	systemID := "sys-" + uuid.NewString()
	serviceID := "svc-" + uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	vmName := "vmname" + vmID[len(vmID)-4:]

	sys := mustCreateSystem(t, client, systemID, "shop"+systemID[len(systemID)-4:], "owner-1")
	svc := mustCreateService(t, client, serviceID, "redis"+serviceID[len(serviceID)-4:], sys.ID, "svc")
	_, err = client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace("prod-shop").
		SetClusterID(clusterID).
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-1").
		SetServiceID(svc.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		ID:        vmID,
		Name:      vmName,
		Namespace: "prod-shop",
		Cluster:   clusterID,
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			DiskGB:                   20,
			RootDataVolumeName:       "rootdisk",
			DiskHotplugSupported:     true,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
		ResourceVersion: "rv-batch-modify-1",
	}})

	return NewServer(ServerDeps{
		EntClient: client,
		VMService: service.NewVMService(mock),
	}), client, vmID
}

func mustCreateBatchDeleteTargetVM(t *testing.T, client *ent.Client) string {
	t.Helper()
	return mustCreateBatchDeleteTargetVMWithStatus(t, client, entvm.StatusSTOPPED)
}

func mustCreateBatchDeleteTargetVMWithStatus(t *testing.T, client *ent.Client, status entvm.Status) string {
	t.Helper()
	actor := "owner-1"

	systemID := "sys-" + uuid.NewString()
	serviceID := "svc-" + uuid.NewString()
	vmID := "vm-" + uuid.NewString()

	sys := mustCreateSystem(t, client, systemID, "shop"+systemID[len(systemID)-4:], actor)
	svc := mustCreateService(t, client, serviceID, "redis"+serviceID[len(serviceID)-4:], sys.ID, "svc")
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vmname" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("prod-shop").
		SetClusterID("cluster-a").
		SetStatus(status).
		SetCreatedBy(actor).
		SetServiceID(svc.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return vmID
}

func mustCreateBatchPowerTargetVM(t *testing.T, client *ent.Client, env namespaceregistry.Environment) string {
	t.Helper()
	actor := "owner-1"

	systemID := "sys-" + uuid.NewString()
	serviceID := "svc-" + uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	namespace := "team-test"
	if env == namespaceregistry.EnvironmentProd {
		namespace = "team-prod"
	}

	sys := mustCreateSystem(t, client, systemID, "shop"+systemID[len(systemID)-4:], actor)
	svc := mustCreateService(t, client, serviceID, "redis"+serviceID[len(serviceID)-4:], sys.ID, "svc")
	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(env).
		SetCreatedBy(actor).
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vmname" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace(namespace).
		SetClusterID("cluster-a").
		SetStatus("RUNNING").
		SetCreatedBy(actor).
		SetServiceID(svc.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return vmID
}

func mustSeedPowerBatchForRetry(t *testing.T, client *ent.Client, operation string) (batchID, childID string) {
	t.Helper()
	return mustSeedPowerBatchForRetryWithStatuses(t, client, operation, entticket.StatusFAILED, domainevent.StatusFAILED)
}

func mustSeedPowerBatchForRetryWithStatuses(
	t *testing.T,
	client *ent.Client,
	operation string,
	ticketStatus entticket.Status,
	eventStatus domainevent.Status,
) (batchID, childID string) {
	t.Helper()

	const actor = "owner-1"

	batchID = "batch-" + uuid.NewString()
	parentEventID := "ev-parent-" + uuid.NewString()
	childEventID := "ev-child-" + uuid.NewString()
	childID = "ticket-child-" + uuid.NewString()

	parentPayload := []byte(`{"operation":"POWER_START","items":[]}`)
	if _, err := client.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(batchID).
		SetPayload(parentPayload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(t.Context()); err != nil {
		t.Fatalf("create parent event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(batchID).
		SetEventID(parentEventID).
		SetRequester(actor).
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("power batch").
		Save(t.Context()); err != nil {
		t.Fatalf("create parent ticket: %v", err)
	}

	payload := mustJSON(t, domain.VMPowerPayload{
		VMID:      "vm-1",
		VMName:    "vm-1",
		ClusterID: "cluster-a",
		Namespace: "prod-shop",
		Operation: operation,
		Actor:     actor,
	})
	if _, err := client.DomainEvent.Create().
		SetID(childEventID).
		SetEventType(string(domain.EventVMStartRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-1").
		SetPayload([]byte(payload)).
		SetStatus(eventStatus).
		SetCreatedBy(actor).
		Save(t.Context()); err != nil {
		t.Fatalf("create child event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(childID).
		SetEventID(childEventID).
		SetRequester(actor).
		SetStatus(ticketStatus).
		SetOperationType(entticket.OperationTypePOWER).
		SetParentTicketID(batchID).
		SetRejectReason("seed failure").
		Save(t.Context()); err != nil {
		t.Fatalf("create child ticket: %v", err)
	}
	if _, err := client.BatchTicket.Create().
		SetID(batchID).
		SetBatchType("BATCH_POWER").
		SetChildCount(1).
		SetFailedCount(1).
		SetStatus("FAILED").
		SetCreatedBy(actor).
		SetReason("power batch").
		Save(t.Context()); err != nil {
		t.Fatalf("create batch projection: %v", err)
	}

	return batchID, childID
}

func mustSubmitFailedCreateBatchForRetry(
	t *testing.T,
	srv *Server,
	client *ent.Client,
	actor string,
) (batchID, childID string) {
	t.Helper()

	serviceID, templateID, sizeID := mustCreateBatchCreatePrerequisites(
		t,
		client,
		actor,
		"team-prod",
	)
	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperation("CREATE"),
		Items: []generated.VMBatchChildItem{
			{
				ServiceId:      serviceID,
				TemplateId:     templateID,
				InstanceSizeId: sizeID,
				Namespace:      "team-prod",
				Reason:         "create one",
			},
		},
	})
	submitCtx, submitW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		actor,
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitW.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", submitW.Code, http.StatusAccepted, submitW.Body.String())
	}

	var submitResp generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	childID = children[0].ID

	if _, updateChildErr := client.Ticket.UpdateOneID(childID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("seed failure").
		Save(t.Context()); updateChildErr != nil {
		t.Fatalf("seed child failed status: %v", updateChildErr)
	}
	if _, updateChildEventErr := client.DomainEvent.UpdateOneID(children[0].EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); updateChildEventErr != nil {
		t.Fatalf("seed child event failed status: %v", updateChildEventErr)
	}
	if _, updateParentErr := client.Ticket.UpdateOneID(submitResp.BatchId).
		SetStatus(entticket.StatusFAILED).
		ClearSelectedClusterID().
		ClearSelectedStorageClass().
		SetRejectReason("seed batch failure").
		Save(t.Context()); updateParentErr != nil {
		t.Fatalf("seed parent failed status: %v", updateParentErr)
	}
	parentTicket, err := client.Ticket.Get(t.Context(), submitResp.BatchId)
	if err != nil {
		t.Fatalf("load parent ticket: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(parentTicket.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed parent event failed status: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(submitResp.BatchId).
		SetChildCount(1).
		SetFailedCount(1).
		SetSuccessCount(0).
		SetPendingCount(0).
		SetStatus(entbatchticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed batch projection failed status: %v", err)
	}

	return submitResp.BatchId, childID
}

func mustCreateBatchCreatePrerequisites(
	t *testing.T,
	client *ent.Client,
	actor string,
	namespace string,
) (serviceID, templateID, sizeID openapi_types.UUID) {
	t.Helper()

	systemID := "sys-" + uuid.NewString()
	serviceRawID := uuid.NewString()
	templateRawID := uuid.NewString()
	sizeRawID := uuid.NewString()

	sys := mustCreateSystem(t, client, systemID, "shop"+systemID[len(systemID)-4:], actor)
	_ = mustCreateService(t, client, serviceRawID, "api"+serviceRawID[len(serviceRawID)-4:], sys.ID, "svc")

	_, err := client.Template.Create().
		SetID(templateRawID).
		SetName("tpl-" + templateRawID[len(templateRawID)-4:]).
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/ubuntu:22.04").
		SetCatalogScope(enttemplate.CatalogScopeProd).
		SetCreatedBy(actor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID(sizeRawID).
		SetName("size-" + sizeRawID[len(sizeRawID)-4:]).
		SetCPUCores(2).
		SetMemoryGi(2).
		SetCatalogScope(instancesize.CatalogScopeProd).
		SetCreatedBy(actor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}
	_, err = client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy(actor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	return mustOpenAPIUUID(t, serviceRawID), mustOpenAPIUUID(t, templateRawID), mustOpenAPIUUID(t, sizeRawID)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return string(b)
}

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp generated.Error
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Code != want {
		t.Fatalf("error code = %q, want %q", resp.Code, want)
	}
}

type fakeDeleteAtomicWriter struct {
	deleteCalls int
}

func (f *fakeDeleteAtomicWriter) ApproveCreateAndEnqueue(
	_ context.Context,
	_, _, _, _, _, _, _, _ string,
	_ map[string]interface{},
	_ map[string]interface{},
	_ map[string]interface{},
	_ map[string]interface{},
) (vmID, vmName string, err error) {
	return "vm-fake", "vm-fake", nil
}

func (f *fakeDeleteAtomicWriter) ApproveDeleteAndEnqueue(_ context.Context, _, _, _, _ string) error {
	f.deleteCalls++
	return nil
}

func (f *fakeDeleteAtomicWriter) ApproveModifyAndEnqueue(_ context.Context, _, _, _ string, _ map[string]interface{}) error {
	return nil
}

func (f *fakeDeleteAtomicWriter) ApprovePowerAndEnqueue(_ context.Context, _, _, _, _ string) error {
	return nil
}
