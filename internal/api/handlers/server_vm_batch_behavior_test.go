package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestBatchHandler_SubmitVMBatch_Unauthorized(t *testing.T) {
	t.Parallel()

	srv, _ := newBatchBehaviorTestServer(t)
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
		Items: []generated.VMBatchChildItem{
			{VmId: vmID},
		},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/approvals/batch", body, "owner-1", []string{"platform:admin"})
	srv.SubmitApprovalBatch(c)
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
		Operation: generated.VMBatchOperationDELETE,
		Items:     []generated.VMBatchChildItem{},
	})

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/batch", body, "user-a", []string{"platform:admin"})
	srv.SubmitVMBatch(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_BATCH_SIZE")
}

func TestBatchHandler_SubmitDelete_GetAndCancel(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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

	children, err := client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].Status != approvalticket.StatusPENDING {
		t.Fatalf("child status = %q, want %q", children[0].Status, approvalticket.StatusPENDING)
	}

	getCtx, getW := newAuthedGinContext(t, http.MethodGet, "/vms/batch/"+submitResp.BatchId, "", "owner-1", []string{"vm:read"})
	srv.GetVMBatch(getCtx, submitResp.BatchId)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d body=%s", getW.Code, http.StatusOK, getW.Body.String())
	}
	var getResp generated.VMBatchStatusResponse
	if err := json.Unmarshal(getW.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
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
	if err := json.Unmarshal(cancelW.Body.Bytes(), &cancelResp); err != nil {
		t.Fatalf("decode cancel response: %v", err)
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

	updatedChild, err := client.ApprovalTicket.Get(t.Context(), children[0].ID)
	if err != nil {
		t.Fatalf("query updated child ticket: %v", err)
	}
	if updatedChild.Status != approvalticket.StatusCANCELLED {
		t.Fatalf("child status after cancel = %q, want %q", updatedChild.Status, approvalticket.StatusCANCELLED)
	}
}

func TestBatchHandler_GetVMBatch_HidesOtherUsersBatch(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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

func TestBatchHandler_SubmitCreate_ForbiddenWhenNamespaceInvisible(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	serviceID, templateID, sizeID := mustCreateBatchCreatePrerequisites(t, client, "requester-1", "team-prod")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationCREATE,
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
}

func TestBatchHandler_SubmitVMBatch_IdempotentByRequestID(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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

	parentCount, err := client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDIsNil()).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count parent tickets: %v", err)
	}
	if parentCount != 1 {
		t.Fatalf("parent ticket count = %d, want 1", parentCount)
	}
}

func TestBatchHandler_SubmitVMBatch_RateLimitedByPendingParentCount(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")
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
		Operation: generated.VMBatchOperationDELETE,
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
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

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
		Operation: generated.VMBatchOperationDELETE,
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
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

	for i := range maxPendingBatchChildrenUser {
		_, err := client.ApprovalTicket.Create().
			SetID("child-pending-" + uuid.NewString()).
			SetEventID("event-" + uuid.NewString()).
			SetRequester("owner-1").
			SetStatus(approvalticket.StatusPENDING).
			SetParentTicketID("parent-seed").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed pending child ticket #%d: %v", i+1, err)
		}
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

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
		Operation: generated.VMBatchOperationDELETE,
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

func TestBatchHandler_RetryVMBatch_RetriesFailedDeleteChild(t *testing.T) {
	t.Parallel()

	writer := &fakeDeleteAtomicWriter{}
	srv, client := newBatchBehaviorTestServerWithGateway(t, writer)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchOperationDELETE,
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

	children, err := client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDEQ(submitResp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	child := children[0]

	if _, err := client.ApprovalTicket.UpdateOneID(child.ID).
		SetStatus(approvalticket.StatusFAILED).
		SetRejectReason("seed failure").
		Save(t.Context()); err != nil {
		t.Fatalf("seed child failed status: %v", err)
	}

	retryCtx, retryW := newAuthedGinContext(t, http.MethodPost, "/vms/batch/"+submitResp.BatchId+"/retry", "", "owner-1", []string{"vm:delete"})
	srv.RetryVMBatch(retryCtx, submitResp.BatchId)
	if retryW.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d body=%s", retryW.Code, http.StatusOK, retryW.Body.String())
	}

	var retryResp generated.VMBatchActionResponse
	if err := json.Unmarshal(retryW.Body.Bytes(), &retryResp); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retryResp.AffectedCount != 1 {
		t.Fatalf("affected_count = %d, want 1", retryResp.AffectedCount)
	}
	if len(retryResp.AffectedTicketIds) != 1 || retryResp.AffectedTicketIds[0] != child.ID {
		t.Fatalf("affected_ticket_ids = %v, want [%s]", retryResp.AffectedTicketIds, child.ID)
	}
	if writer.deleteCalls != 1 {
		t.Fatalf("delete atomic writer calls = %d, want 1", writer.deleteCalls)
	}
}

func TestBatchHandler_SubmitVMBatchPower_EnqueueFailureFallsBackToFailed(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client, "owner-1")

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
	if resp.Status != generated.VMBatchParentStatusFAILED {
		t.Fatalf("status = %q, want %q", resp.Status, generated.VMBatchParentStatusFAILED)
	}

	children, err := client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDEQ(resp.BatchId)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query child tickets: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("child ticket count = %d, want 1", len(children))
	}
	if children[0].Status != approvalticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", children[0].Status, approvalticket.StatusFAILED)
	}
	if !strings.Contains(children[0].RejectReason, "enqueue vm_power job failed") {
		t.Fatalf("child reject_reason = %q, want contains %q", children[0].RejectReason, "enqueue vm_power job failed")
	}
}

func TestBatchHandler_RetryVMBatch_PowerChildUnknownOperation(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "owner-1", "hibernate")

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

	child, err := client.ApprovalTicket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != approvalticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, approvalticket.StatusFAILED)
	}
	if !strings.Contains(child.RejectReason, "unknown power operation for retry") {
		t.Fatalf("child reject_reason = %q, want contains %q", child.RejectReason, "unknown power operation for retry")
	}
}

func TestBatchHandler_RetryVMBatch_PowerChildEnqueueFailure(t *testing.T) {
	t.Parallel()

	srv, client := newBatchBehaviorTestServer(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "owner-1", "start")

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

	child, err := client.ApprovalTicket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query child ticket: %v", err)
	}
	if child.Status != approvalticket.StatusFAILED {
		t.Fatalf("child status = %q, want %q", child.Status, approvalticket.StatusFAILED)
	}
	if !strings.Contains(child.RejectReason, "failed to enqueue vm_power job") {
		t.Fatalf("child reject_reason = %q, want contains %q", child.RejectReason, "failed to enqueue vm_power job")
	}
}

func newBatchBehaviorTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "batch_handler_behavior")
	return NewServer(ServerDeps{EntClient: client}), client
}

func newBatchBehaviorTestServerWithGateway(t *testing.T, writer *fakeDeleteAtomicWriter) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "batch_handler_behavior_with_gateway")
	gw := approval.NewGateway(client, nil, writer)
	return NewServer(ServerDeps{
		EntClient: client,
		Gateway:   gw,
	}), client
}

func mustCreateBatchDeleteTargetVM(t *testing.T, client *ent.Client, actor string) string {
	t.Helper()

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
		SetStatus("RUNNING").
		SetCreatedBy(actor).
		SetServiceID(svc.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return vmID
}

func mustSeedPowerBatchForRetry(t *testing.T, client *ent.Client, actor, operation string) (batchID, childID string) {
	t.Helper()

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
	if _, err := client.ApprovalTicket.Create().
		SetID(batchID).
		SetEventID(parentEventID).
		SetRequester(actor).
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
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
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy(actor).
		Save(t.Context()); err != nil {
		t.Fatalf("create child event: %v", err)
	}
	if _, err := client.ApprovalTicket.Create().
		SetID(childID).
		SetEventID(childEventID).
		SetRequester(actor).
		SetStatus(approvalticket.StatusFAILED).
		SetOperationType(approvalticket.OperationTypeCREATE).
		SetParentTicketID(batchID).
		SetRejectReason("seed failure").
		Save(t.Context()); err != nil {
		t.Fatalf("create child ticket: %v", err)
	}
	if _, err := client.BatchApprovalTicket.Create().
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
		SetVersion(1).
		SetCreatedBy(actor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID(sizeRawID).
		SetName("size-" + sizeRawID[len(sizeRawID)-4:]).
		SetCPUCores(2).
		SetMemoryMB(2048).
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
	_ int,
	_ map[string]interface{},
	_ map[string]interface{},
	_ map[string]interface{},
) (string, string, error) {
	return "vm-fake", "vm-fake", nil
}

func (f *fakeDeleteAtomicWriter) ApproveDeleteAndEnqueue(_ context.Context, _, _, _, _ string) error {
	f.deleteCalls++
	return nil
}
