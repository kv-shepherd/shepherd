package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/service"
)

type batchMutationSafetyFixture struct {
	server      *Server
	client      *ent.Client
	riverClient *river.Client[pgx.Tx]
}

func TestBatchHandler_DeleteBatchOwnerWithCreatePermissionCannotRetryOrCancel(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_mutation_operation_permission")
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)

	for _, action := range []string{"retry", "cancel"} {
		t.Run(action, func(t *testing.T) {
			ctx, response := newAuthedGinContext(
				t,
				http.MethodPost,
				"/vms/batch/"+batchID+"/"+action,
				"",
				"owner-1",
				[]string{"vm:create"},
			)
			if action == "retry" {
				fixture.server.RetryVMBatch(ctx, batchID)
			} else {
				fixture.server.CancelVMBatch(ctx, batchID)
			}
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s status = %d, want %d body=%s", action, response.Code, http.StatusForbidden, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), "FORBIDDEN")
		})
	}

	stored, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload delete child after forbidden actions: %v", err)
	}
	if stored.Status != entticket.StatusPENDING || stored.AttemptCount != 0 {
		t.Fatalf("delete child after forbidden actions = %s attempt %d, want PENDING/0", stored.Status, stored.AttemptCount)
	}
}

func TestBatchHandler_CancelVMBatch_TerminalParentDoesNotMutatePendingChild(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_cancel_terminal_parent")
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)
	parent, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load batch parent: %v", err)
	}
	if _, updateParentErr := fixture.client.Ticket.UpdateOneID(parent.ID).
		SetStatus(entticket.StatusSUCCESS).
		Save(t.Context()); updateParentErr != nil {
		t.Fatalf("seed completed batch parent: %v", updateParentErr)
	}
	if _, updateParentEventErr := fixture.client.DomainEvent.UpdateOneID(parent.EventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); updateParentEventErr != nil {
		t.Fatalf("seed completed batch parent event: %v", updateParentEventErr)
	}
	if _, updateProjectionErr := fixture.client.BatchTicket.UpdateOneID(batchID).
		SetChildCount(1).
		SetSuccessCount(0).
		SetFailedCount(0).
		SetPendingCount(1).
		SetStatus(entbatchticket.StatusIN_PROGRESS).
		Save(t.Context()); updateProjectionErr != nil {
		t.Fatalf("seed terminal-parent batch projection: %v", updateProjectionErr)
	}

	childBefore, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload child before terminal-parent cancel: %v", err)
	}
	childEventBefore, err := fixture.client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("load child event before terminal-parent cancel: %v", err)
	}
	parentBefore, err := fixture.client.Ticket.Get(t.Context(), parent.ID)
	if err != nil {
		t.Fatalf("reload parent before terminal-parent cancel: %v", err)
	}
	parentEventBefore, err := fixture.client.DomainEvent.Get(t.Context(), parent.EventID)
	if err != nil {
		t.Fatalf("reload parent event before terminal-parent cancel: %v", err)
	}
	projectionBefore, err := fixture.client.BatchTicket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load projection before terminal-parent cancel: %v", err)
	}

	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/cancel",
		"",
		"owner-1",
		[]string{"vm:delete"},
	)
	fixture.server.CancelVMBatch(ctx, batchID)
	if response.Code != http.StatusConflict {
		t.Fatalf("cancel status = %d, want %d body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	conflict := mustDecodeGeneratedError(t, response.Body.Bytes(), "BATCH_ACTION_NOT_APPLICABLE")
	if got, _ := conflict.Params["parent_status"].(string); got != string(entticket.StatusSUCCESS) {
		t.Fatalf("conflict parent_status = %q, want %q", got, entticket.StatusSUCCESS)
	}
	if got, _ := conflict.Params["event_status"].(string); got != string(domainevent.StatusCOMPLETED) {
		t.Fatalf("conflict event_status = %q, want %q", got, domainevent.StatusCOMPLETED)
	}

	childAfter, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload child after terminal-parent cancel: %v", err)
	}
	if childAfter.Status != childBefore.Status || childAfter.RejectReason != childBefore.RejectReason ||
		childAfter.AttemptCount != childBefore.AttemptCount || !sameOptionalTime(childAfter.LastAttemptAt, childBefore.LastAttemptAt) ||
		!childAfter.UpdatedAt.Equal(childBefore.UpdatedAt) {
		t.Fatalf("pending child changed after rejected cancel: before=%+v after=%+v", childBefore, childAfter)
	}
	childEventAfter, err := fixture.client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("reload child event after terminal-parent cancel: %v", err)
	}
	if childEventAfter.Status != childEventBefore.Status {
		t.Fatalf("pending child event changed after rejected cancel: before=%+v after=%+v", childEventBefore, childEventAfter)
	}
	parentAfter, err := fixture.client.Ticket.Get(t.Context(), parent.ID)
	if err != nil {
		t.Fatalf("reload parent after terminal-parent cancel: %v", err)
	}
	if parentAfter.Status != parentBefore.Status || parentAfter.RejectReason != parentBefore.RejectReason ||
		!parentAfter.UpdatedAt.Equal(parentBefore.UpdatedAt) {
		t.Fatalf("terminal parent changed after rejected cancel: before=%+v after=%+v", parentBefore, parentAfter)
	}
	parentEventAfter, err := fixture.client.DomainEvent.Get(t.Context(), parent.EventID)
	if err != nil {
		t.Fatalf("reload parent event after terminal-parent cancel: %v", err)
	}
	if parentEventAfter.Status != parentEventBefore.Status {
		t.Fatalf("terminal parent event changed after rejected cancel: before=%+v after=%+v", parentEventBefore, parentEventAfter)
	}
	projectionAfter, err := fixture.client.BatchTicket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload projection after terminal-parent cancel: %v", err)
	}
	if projectionAfter.Status != projectionBefore.Status ||
		projectionAfter.ChildCount != projectionBefore.ChildCount ||
		projectionAfter.SuccessCount != projectionBefore.SuccessCount ||
		projectionAfter.FailedCount != projectionBefore.FailedCount ||
		projectionAfter.PendingCount != projectionBefore.PendingCount {
		t.Fatalf("batch projection changed after rejected cancel: before=%+v after=%+v", projectionBefore, projectionAfter)
	}
}

func TestBatchHandler_CancelChildCASRejectsReparentedTicketWithoutMutation(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_cancel_reparented_child")
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)
	const otherBatchID = "corrupted-other-parent"

	if _, err := fixture.client.Ticket.UpdateOneID(child.ID).
		SetParentTicketID(otherBatchID).
		Save(t.Context()); err != nil {
		t.Fatalf("reparent child before cancel CAS: %v", err)
	}
	childBefore, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("load reparented child before cancel CAS: %v", err)
	}
	eventBefore, err := fixture.client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("load child event before cancel CAS: %v", err)
	}
	parentBefore, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load original parent before cancel CAS: %v", err)
	}
	parentEventBefore, err := fixture.client.DomainEvent.Get(t.Context(), parentBefore.EventID)
	if err != nil {
		t.Fatalf("load original parent event before cancel CAS: %v", err)
	}
	projectionBefore, err := fixture.client.BatchTicket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load original projection before cancel CAS: %v", err)
	}

	_, err = fixture.server.updateBatchChildTicketAndEventStatus(
		t.Context(),
		batchID,
		[]batchChildTicketEventRef{{TicketID: child.ID, EventID: child.EventID}},
		entticket.StatusCANCELLED,
		domainevent.StatusCANCELLED,
		false,
		[]entticket.Status{entticket.StatusPENDING},
		[]domainevent.Status{domainevent.StatusPENDING},
	)
	var conflict *batchChildStateConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("reparented child cancel error = %v, want *batchChildStateConflictError", err)
	}

	childAfter, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload reparented child after cancel CAS: %v", err)
	}
	if childAfter.Status != childBefore.Status || childAfter.EventID != childBefore.EventID ||
		childAfter.ParentTicketID != childBefore.ParentTicketID ||
		childAfter.RejectReason != childBefore.RejectReason ||
		childAfter.AttemptCount != childBefore.AttemptCount ||
		!sameOptionalTime(childAfter.LastAttemptAt, childBefore.LastAttemptAt) ||
		!childAfter.UpdatedAt.Equal(childBefore.UpdatedAt) {
		t.Fatalf("reparented child changed after rejected cancel: before=%+v after=%+v", childBefore, childAfter)
	}
	eventAfter, err := fixture.client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("reload child event after cancel CAS: %v", err)
	}
	if eventAfter.Status != eventBefore.Status {
		t.Fatalf("child event changed after rejected cancel: before=%s after=%s", eventBefore.Status, eventAfter.Status)
	}
	parentAfter, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload original parent after cancel CAS: %v", err)
	}
	if parentAfter.Status != parentBefore.Status || parentAfter.EventID != parentBefore.EventID ||
		parentAfter.RejectReason != parentBefore.RejectReason ||
		!parentAfter.UpdatedAt.Equal(parentBefore.UpdatedAt) {
		t.Fatalf("original parent changed after rejected cancel: before=%+v after=%+v", parentBefore, parentAfter)
	}
	parentEventAfter, err := fixture.client.DomainEvent.Get(t.Context(), parentBefore.EventID)
	if err != nil {
		t.Fatalf("reload original parent event after cancel CAS: %v", err)
	}
	if parentEventAfter.Status != parentEventBefore.Status {
		t.Fatalf("original parent event changed after rejected cancel: before=%s after=%s", parentEventBefore.Status, parentEventAfter.Status)
	}
	projectionAfter, err := fixture.client.BatchTicket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload original projection after cancel CAS: %v", err)
	}
	if projectionAfter.Status != projectionBefore.Status ||
		projectionAfter.ChildCount != projectionBefore.ChildCount ||
		projectionAfter.SuccessCount != projectionBefore.SuccessCount ||
		projectionAfter.FailedCount != projectionBefore.FailedCount ||
		projectionAfter.PendingCount != projectionBefore.PendingCount ||
		!projectionAfter.UpdatedAt.Equal(projectionBefore.UpdatedAt) {
		t.Fatalf("original projection changed after rejected cancel: before=%+v after=%+v", projectionBefore, projectionAfter)
	}
}

func TestBatchHandler_RetryVMBatch_RejectedGenericChildDoesNotDispatchOrMutate(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_rejected_generic_retry")
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)
	attemptedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)

	if _, err := fixture.client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusREJECTED).
		SetRejectReason("policy rejected").
		SetAttemptCount(1).
		SetLastAttemptAt(attemptedAt).
		Save(t.Context()); err != nil {
		t.Fatalf("seed rejected generic child: %v", err)
	}
	if _, err := fixture.client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed cancelled generic child event: %v", err)
	}
	mustSetBatchParentFailedForMutationSafety(t, fixture.client, batchID, "original-approver")

	beforeDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)
	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"owner-1",
		[]string{"vm:delete"},
	)
	fixture.server.RetryVMBatch(ctx, batchID)
	if response.Code != http.StatusConflict {
		t.Fatalf("retry status = %d, want %d body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "BATCH_NOTHING_TO_RETRY")

	stored, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload rejected generic child: %v", err)
	}
	if stored.Status != entticket.StatusREJECTED || stored.RejectReason != "policy rejected" ||
		stored.AttemptCount != 1 || stored.LastAttemptAt == nil || !stored.LastAttemptAt.Equal(attemptedAt) {
		t.Fatalf(
			"rejected generic child changed: status=%s reason=%q attempt=%d at=%v",
			stored.Status,
			stored.RejectReason,
			stored.AttemptCount,
			stored.LastAttemptAt,
		)
	}
	storedEvent, err := fixture.client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("reload rejected generic child event: %v", err)
	}
	if storedEvent.Status != domainevent.StatusCANCELLED {
		t.Fatalf("rejected generic child event status = %s, want CANCELLED", storedEvent.Status)
	}
	afterDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)
	if afterDispatchers != beforeDispatchers {
		t.Fatalf("batch approval dispatcher count = %d, want unchanged %d", afterDispatchers, beforeDispatchers)
	}
}

func TestBatchHandler_RetryVMBatch_RejectedPowerChildDoesNotEnqueueOrMutate(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_rejected_power_retry")
	batchID, childID := mustSeedPowerBatchForRetryWithStatuses(
		t,
		fixture.client,
		"start",
		entticket.StatusREJECTED,
		domainevent.StatusCANCELLED,
	)
	mustSetBatchParentFailedForMutationSafety(t, fixture.client, batchID, "original-approver")
	before, err := fixture.client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("load rejected power child before retry: %v", err)
	}
	beforePowerJobs := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.VMPowerArgs{}.Kind())

	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"owner-1",
		[]string{"vm:operate"},
	)
	fixture.server.RetryVMBatch(ctx, batchID)
	if response.Code != http.StatusConflict {
		t.Fatalf("retry status = %d, want %d body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "BATCH_NOTHING_TO_RETRY")

	after, err := fixture.client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("reload rejected power child after retry: %v", err)
	}
	if after.Status != before.Status || after.RejectReason != before.RejectReason ||
		after.AttemptCount != before.AttemptCount || !sameOptionalTime(after.LastAttemptAt, before.LastAttemptAt) {
		t.Fatalf(
			"rejected power child changed: before=(%s,%q,%d,%v) after=(%s,%q,%d,%v)",
			before.Status,
			before.RejectReason,
			before.AttemptCount,
			before.LastAttemptAt,
			after.Status,
			after.RejectReason,
			after.AttemptCount,
			after.LastAttemptAt,
		)
	}
	afterPowerJobs := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.VMPowerArgs{}.Kind())
	if afterPowerJobs != beforePowerJobs {
		t.Fatalf("vm_power job count = %d, want unchanged %d", afterPowerJobs, beforePowerJobs)
	}
}

func TestBatchHandler_RetryVMBatch_GenericRetryApproverAttribution(t *testing.T) {
	const originalApprover = "original-approver"
	fixture := newBatchMutationSafetyFixture(t, "batch_retry_approver_attribution")
	batchID := mustSeedFailedDeleteBatchForApproverSafety(t, fixture, originalApprover)

	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"owner-1",
		[]string{"vm:delete"},
	)
	fixture.server.RetryVMBatch(ctx, batchID)
	if response.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	parent, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload generic retry parent: %v", err)
	}
	if parent.Approver != originalApprover {
		t.Fatalf("parent approver = %q, want %q", parent.Approver, originalApprover)
	}
}

func TestBatchHandler_RetryVMBatch_NonCreateReviewBodyFailsClosed(t *testing.T) {
	const originalApprover = "original-approver"
	fixture := newBatchMutationSafetyFixture(t, "batch_retry_non_create_review")
	batchID := mustSeedFailedDeleteBatchForApproverSafety(t, fixture, originalApprover)
	beforeDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)

	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		`{}`,
		"reviewer-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	fixture.server.RetryVMBatch(ctx, batchID)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("retry status = %d, want %d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "BATCH_RETRY_REVIEW_NOT_APPLICABLE")

	parent, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload parent after rejected review body: %v", err)
	}
	if parent.Status != entticket.StatusFAILED || parent.Approver != originalApprover {
		t.Fatalf("parent after rejected review body = %s/%q, want FAILED/%q", parent.Status, parent.Approver, originalApprover)
	}
	afterDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)
	if afterDispatchers != beforeDispatchers {
		t.Fatalf("dispatcher count = %d, want unchanged %d", afterDispatchers, beforeDispatchers)
	}
}

func TestBatchHandler_RetryVMBatch_ReviewCannotReplaceMissingApprovalProvenance(t *testing.T) {
	fixture := newBatchMutationSafetyFixture(t, "batch_retry_missing_approval_provenance")
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)
	if _, err := fixture.client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("retryable failure").
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed generic child: %v", err)
	}
	if _, err := fixture.client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed generic child event: %v", err)
	}
	mustSetBatchParentFailedForMutationSafety(t, fixture.client, batchID, "")
	beforeDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)

	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		`{}`,
		"reviewer-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	fixture.server.RetryVMBatch(ctx, batchID)
	if response.Code != http.StatusConflict {
		t.Fatalf("retry status = %d, want %d body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	conflict := mustDecodeGeneratedError(t, response.Body.Bytes(), "BATCH_ACTION_NOT_APPLICABLE")
	if got, _ := conflict.Params["reason"].(string); got != "missing_approval_provenance" {
		t.Fatalf("conflict reason = %q, want missing_approval_provenance", got)
	}
	if !strings.Contains(conflict.Message, "create a new approval batch") {
		t.Fatalf("conflict message = %q, want new approval batch guidance", conflict.Message)
	}

	storedChild, err := fixture.client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload child after provenance conflict: %v", err)
	}
	if storedChild.Status != entticket.StatusFAILED || storedChild.AttemptCount != 0 {
		t.Fatalf("child after provenance conflict = %s attempt %d, want FAILED/0", storedChild.Status, storedChild.AttemptCount)
	}
	storedParent, err := fixture.client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("reload parent after provenance conflict: %v", err)
	}
	if storedParent.Approver != "" {
		t.Fatalf("parent approver = %q, want empty", storedParent.Approver)
	}
	afterDispatchers := countBatchMutationSafetyJobs(t, fixture.riverClient, jobs.BatchApprovalDispatchJobKind)
	if afterDispatchers != beforeDispatchers {
		t.Fatalf("batch approval dispatcher count = %d, want unchanged %d", afterDispatchers, beforeDispatchers)
	}
}

func newBatchMutationSafetyFixture(t *testing.T, prefix string) batchMutationSafetyFixture {
	t.Helper()
	client, pool := newBatchBehaviorTestStore(t, prefix)
	riverClient := newBatchBehaviorTestRiverClient(t, pool)
	return batchMutationSafetyFixture{
		server: NewServer(ServerDeps{
			EntClient:    client,
			Pool:         pool,
			RiverClient:  riverClient,
			ApprovalReqs: service.NewApprovalRequirementService(client),
		}),
		client:      client,
		riverClient: riverClient,
	}
}

func mustSubmitDeleteBatchForMutationSafety(
	t *testing.T,
	fixture batchMutationSafetyFixture,
) (string, *ent.Ticket) {
	t.Helper()
	vmID := mustCreateBatchDeleteTargetVM(t, fixture.client)
	body, err := json.Marshal(generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})
	if err != nil {
		t.Fatalf("marshal delete batch request: %v", err)
	}
	ctx, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		string(body),
		"owner-1",
		[]string{"platform:admin"},
	)
	fixture.server.SubmitVMBatch(ctx)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", response.Code, http.StatusAccepted, response.Body.String())
	}
	var submitted generated.VMBatchSubmitResponse
	if decodeErr := json.Unmarshal(response.Body.Bytes(), &submitted); decodeErr != nil {
		t.Fatalf("decode delete batch response: %v", decodeErr)
	}
	child, err := fixture.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitted.BatchId)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("load delete batch child: %v", err)
	}
	return submitted.BatchId, child
}

func mustSeedFailedDeleteBatchForApproverSafety(
	t *testing.T,
	fixture batchMutationSafetyFixture,
	approver string,
) string {
	t.Helper()
	batchID, child := mustSubmitDeleteBatchForMutationSafety(t, fixture)
	if _, err := fixture.client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("retryable failure").
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed generic child: %v", err)
	}
	if _, err := fixture.client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed generic child event: %v", err)
	}

	mustSetBatchParentFailedForMutationSafety(t, fixture.client, batchID, approver)
	return batchID
}

func mustSetBatchParentFailedForMutationSafety(
	t *testing.T,
	client *ent.Client,
	batchID string,
	approver string,
) {
	t.Helper()
	parent, err := client.Ticket.Get(t.Context(), batchID)
	if err != nil {
		t.Fatalf("load batch parent: %v", err)
	}
	update := client.Ticket.UpdateOneID(batchID).
		SetStatus(entticket.StatusFAILED).
		SetApprover(approver).
		SetRejectReason("retryable batch failure")
	if _, err := update.Save(t.Context()); err != nil {
		t.Fatalf("seed failed batch parent: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(parent.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed batch parent event: %v", err)
	}
	if _, err := client.BatchTicket.UpdateOneID(batchID).
		SetChildCount(1).
		SetSuccessCount(0).
		SetFailedCount(1).
		SetPendingCount(0).
		SetStatus(entbatchticket.StatusFAILED).
		Save(t.Context()); err != nil {
		t.Fatalf("seed failed generic batch projection: %v", err)
	}
}

func countBatchMutationSafetyJobs(t *testing.T, client *river.Client[pgx.Tx], kind string) int {
	t.Helper()
	result, err := client.JobList(t.Context(), river.NewJobListParams().Kinds(kind))
	if err != nil {
		t.Fatalf("list %s jobs: %v", kind, err)
	}
	return len(result.Jobs)
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
