package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func TestBatchMutationConflictResponsesKeepStableIdentifiers(t *testing.T) {
	tests := []struct {
		name     string
		invoke   func(*testing.T) (int, generated.Error)
		wantCode string
	}{
		{
			name: "retry attempts exhausted",
			invoke: func(t *testing.T) (int, generated.Error) {
				t.Helper()
				ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/retry", "", "owner-1", nil)
				writeBatchRetryAttemptsExhausted(ctx, "batch-1", &usecase.BatchChildAttemptsExhaustedError{
					TicketID:     "ticket-1",
					AttemptCount: domain.BatchChildMaxAttempts,
					MaxAttempts:  domain.BatchChildMaxAttempts,
				})
				return response.Code, decodeBatchCoverageError(t, response.Body.Bytes())
			},
			wantCode: "BATCH_RETRY_ATTEMPTS_EXHAUSTED",
		},
		{
			name: "retry state changed",
			invoke: func(t *testing.T) (int, generated.Error) {
				t.Helper()
				ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/retry", "", "owner-1", nil)
				writeBatchChildStateConflict(ctx, "batch-1", batchActionRetry, &batchChildStateConflictError{
					TicketID: "ticket-1",
					EventID:  "event-1",
				})
				return response.Code, decodeBatchCoverageError(t, response.Body.Bytes())
			},
			wantCode: batchNothingToRetryErrorCode,
		},
		{
			name: "cancel state changed",
			invoke: func(t *testing.T) (int, generated.Error) {
				t.Helper()
				ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/cancel", "", "owner-1", nil)
				writeBatchChildStateConflict(ctx, "batch-1", batchActionCancel, nil)
				return response.Code, decodeBatchCoverageError(t, response.Body.Bytes())
			},
			wantCode: "BATCH_NOTHING_TO_CANCEL",
		},
		{
			name: "unknown action fails closed",
			invoke: func(t *testing.T) (int, generated.Error) {
				t.Helper()
				ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/action", "", "owner-1", nil)
				writeBatchChildStateConflict(ctx, "batch-1", "replace", nil)
				return response.Code, decodeBatchCoverageError(t, response.Body.Bytes())
			},
			wantCode: "BATCH_ACTION_NOT_APPLICABLE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, response := test.invoke(t)
			if status != http.StatusConflict {
				t.Fatalf("status = %d, want %d", status, http.StatusConflict)
			}
			if response.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", response.Code, test.wantCode)
			}
			if got, _ := response.Params["batch_id"].(string); got != "batch-1" {
				t.Fatalf("batch_id = %q, want batch-1", got)
			}
		})
	}
}

func TestBatchMutationStateAndPermissionBoundaries(t *testing.T) {
	permissionTests := []struct {
		name        string
		operation   entticket.OperationType
		permissions []string
		wantAllowed bool
	}{
		{name: "create", operation: entticket.OperationTypeCREATE, permissions: []string{"vm:create"}, wantAllowed: true},
		{name: "delete", operation: entticket.OperationTypeDELETE, permissions: []string{"vm:delete"}, wantAllowed: true},
		{name: "modify", operation: entticket.OperationTypeMODIFY, permissions: []string{"vm:operate"}, wantAllowed: true},
		{name: "power", operation: entticket.OperationTypePOWER, permissions: []string{"vm:operate"}, wantAllowed: true},
		{name: "wrong permission", operation: entticket.OperationTypeDELETE, permissions: []string{"vm:create"}, wantAllowed: false},
		{name: "unknown operation", operation: entticket.OperationType("EXPORT"), permissions: []string{"vm:operate"}, wantAllowed: false},
		{name: "approval reviewer", operation: entticket.OperationType("EXPORT"), permissions: []string{"builtin_approval:approve"}, wantAllowed: true},
	}
	for _, test := range permissionTests {
		t.Run("permission "+test.name, func(t *testing.T) {
			ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/batch-1/retry", "", "owner-1", test.permissions)
			if got := requireBatchMutationPermission(ctx, test.operation); got != test.wantAllowed {
				t.Fatalf("allowed = %t, want %t", got, test.wantAllowed)
			}
			if !test.wantAllowed && response.Code != http.StatusForbidden {
				t.Fatalf("denied status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}

	eventTests := []struct {
		operation entticket.OperationType
		event     domain.EventType
		want      bool
	}{
		{operation: entticket.OperationTypeCREATE, event: domain.EventBatchCreateRequested, want: true},
		{operation: entticket.OperationTypeMODIFY, event: domain.EventBatchModifyRequested, want: true},
		{operation: entticket.OperationTypeDELETE, event: domain.EventBatchDeleteRequested, want: true},
		{operation: entticket.OperationTypePOWER, event: domain.EventBatchPowerRequested, want: true},
		{operation: entticket.OperationTypeCREATE, event: domain.EventBatchDeleteRequested, want: false},
		{operation: entticket.OperationType("EXPORT"), event: domain.EventBatchCreateRequested, want: false},
	}
	for _, test := range eventTests {
		if got := batchParentEventMatchesOperation(test.operation, test.event); got != test.want {
			t.Fatalf("operation/event %s/%s matched = %t, want %t", test.operation, test.event, got, test.want)
		}
	}

	projectionStatuses := []struct {
		stored entbatchticket.Status
		public generated.VMBatchParentStatus
	}{
		{stored: entbatchticket.StatusPENDING_APPROVAL, public: generated.VMBatchParentStatusPENDINGAPPROVAL},
		{stored: entbatchticket.StatusIN_PROGRESS, public: generated.VMBatchParentStatusINPROGRESS},
		{stored: entbatchticket.StatusCOMPLETED, public: generated.VMBatchParentStatusCOMPLETED},
		{stored: entbatchticket.StatusFAILED, public: generated.VMBatchParentStatusFAILED},
		{stored: entbatchticket.StatusPARTIAL_SUCCESS, public: generated.VMBatchParentStatusPARTIALSUCCESS},
		{stored: entbatchticket.StatusCANCELLED, public: generated.VMBatchParentStatusCANCELLED},
	}
	for _, test := range projectionStatuses {
		got, err := batchParentStatusFromProjection(test.stored)
		if err != nil {
			t.Fatalf("map projection status %q: %v", test.stored, err)
		}
		if got != test.public {
			t.Fatalf("projection status %q mapped to %q, want %q", test.stored, got, test.public)
		}
	}
	if _, err := batchParentStatusFromProjection(entbatchticket.Status("CORRUPT")); err == nil {
		t.Fatal("unsupported persisted projection status was accepted")
	}

	if !ticketStatusAllowed(entticket.StatusFAILED, nil) ||
		!ticketStatusAllowed(entticket.StatusFAILED, []entticket.Status{entticket.StatusPENDING, entticket.StatusFAILED}) ||
		ticketStatusAllowed(entticket.StatusFAILED, []entticket.Status{entticket.StatusPENDING}) {
		t.Fatal("ticket status allow-list did not preserve exact-state semantics")
	}
	if !domainEventStatusAllowed(domainevent.StatusFAILED, nil) ||
		!domainEventStatusAllowed(domainevent.StatusFAILED, []domainevent.Status{domainevent.StatusPENDING, domainevent.StatusFAILED}) ||
		domainEventStatusAllowed(domainevent.StatusFAILED, []domainevent.Status{domainevent.StatusPENDING}) {
		t.Fatal("event status allow-list did not preserve exact-state semantics")
	}
}

func TestLoadBatchView_BackfillsMixedProjectionOnceWithoutWorkflowDrift(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	items := make([]generated.VMBatchChildItem, 0, 5)
	for range 5 {
		items = append(items, generated.VMBatchChildItem{VmId: mustCreateBatchDeleteTargetVM(t, client)})
	}
	requestBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		Items:     items,
	})
	ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch", requestBody, "owner-1", []string{"platform:admin"})
	srv.SubmitVMBatch(ctx)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want %d body=%s", response.Code, http.StatusAccepted, response.Body.String())
	}
	var submitted generated.VMBatchSubmitResponse
	if err := json.Unmarshal(response.Body.Bytes(), &submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitted.BatchId)).
		Order(ent.Asc(entticket.FieldID)).
		All(t.Context())
	if err != nil {
		t.Fatalf("load submitted children: %v", err)
	}
	wantStatuses := []entticket.Status{
		entticket.StatusSUCCESS,
		entticket.StatusFAILED,
		entticket.StatusCANCELLED,
		entticket.StatusPENDING,
		entticket.StatusEXECUTING,
	}
	for index, child := range children {
		if _, updateErr := client.Ticket.UpdateOneID(child.ID).SetStatus(wantStatuses[index]).Save(t.Context()); updateErr != nil {
			t.Fatalf("seed child %s status %s: %v", child.ID, wantStatuses[index], updateErr)
		}
	}
	if deleteErr := client.BatchTicket.DeleteOneID(submitted.BatchId).Exec(t.Context()); deleteErr != nil {
		t.Fatalf("delete projection before backfill: %v", deleteErr)
	}
	workflowBefore := captureBatchCoverageWorkflow(t, srv, submitted.BatchId)

	view, loadedChildren, err := srv.loadBatchView(t.Context(), submitted.BatchId)
	if err != nil {
		t.Fatalf("load and backfill batch view: %v", err)
	}
	if len(loadedChildren) != len(wantStatuses) || view.ChildCount != len(wantStatuses) {
		t.Fatalf("loaded child counts = %d/%d, want %d", len(loadedChildren), view.ChildCount, len(wantStatuses))
	}
	if view.SuccessCount != 1 || view.FailedCount != 1 || view.PendingCount != 2 ||
		view.Status != generated.VMBatchParentStatusINPROGRESS {
		t.Fatalf("backfilled view = %+v, want success=1 failed=1 pending=2 IN_PROGRESS", view)
	}
	workflowAfter := captureBatchCoverageWorkflow(t, srv, submitted.BatchId)
	if !reflect.DeepEqual(workflowAfter, workflowBefore) {
		t.Fatalf("projection backfill changed workflow rows:\n before=%+v\n  after=%+v", workflowBefore, workflowAfter)
	}

	projectionBefore, err := client.BatchTicket.Get(t.Context(), submitted.BatchId)
	if err != nil {
		t.Fatalf("load backfilled projection: %v", err)
	}
	if projectionBefore.ChildCount != 5 || projectionBefore.SuccessCount != 1 || projectionBefore.FailedCount != 1 ||
		projectionBefore.PendingCount != 2 || projectionBefore.Status != entbatchticket.StatusIN_PROGRESS {
		t.Fatalf("backfilled projection = %+v", projectionBefore)
	}
	if backfillErr := srv.backfillMissingBatchProjection(t.Context(), submitted.BatchId); backfillErr != nil {
		t.Fatalf("repeat backfill: %v", backfillErr)
	}
	projectionAfter, err := client.BatchTicket.Get(t.Context(), submitted.BatchId)
	if err != nil {
		t.Fatalf("reload projection after idempotent backfill: %v", err)
	}
	if !projectionAfter.UpdatedAt.Equal(projectionBefore.UpdatedAt) ||
		projectionAfter.Status != projectionBefore.Status ||
		projectionAfter.ChildCount != projectionBefore.ChildCount ||
		projectionAfter.SuccessCount != projectionBefore.SuccessCount ||
		projectionAfter.FailedCount != projectionBefore.FailedCount ||
		projectionAfter.PendingCount != projectionBefore.PendingCount {
		t.Fatalf("repeat backfill rewrote projection: before=%+v after=%+v", projectionBefore, projectionAfter)
	}
}

func TestBackfillMissingBatchProjection_RejectsNonBatchIdentityWithoutWrite(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	batchID := "not-a-batch-" + uuid.NewString()
	eventID := "event-" + uuid.NewString()
	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMStartRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-1").
		SetPayload([]byte(`{}`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("owner-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed non-batch event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(batchID).
		SetEventID(eventID).
		SetRequester("owner-1").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypePOWER).
		SetReason("ordinary VM operation").
		Save(t.Context()); err != nil {
		t.Fatalf("seed non-batch ticket: %v", err)
	}

	if err := srv.backfillMissingBatchProjection(t.Context(), batchID); !errors.Is(err, errBatchNotFound) {
		t.Fatalf("non-batch backfill error = %v, want errBatchNotFound", err)
	}
	exists, err := client.BatchTicket.Query().Where(entbatchticket.IDEQ(batchID)).Exist(t.Context())
	if err != nil {
		t.Fatalf("query unexpected projection: %v", err)
	}
	if exists {
		t.Fatal("non-batch workflow received a public batch projection")
	}
}

func TestEvaluateBatchSubmissionRateLimits_FailsClosedAtEachReadBoundary(t *testing.T) {
	sentinel := errors.New("durable policy read unavailable")
	tests := []struct {
		name      string
		configure func(*batchCoverageLimitReader)
		wantError error
	}{
		{name: "active user read", configure: func(reader *batchCoverageLimitReader) { reader.activeErr = sentinel }, wantError: sentinel},
		{name: "actor missing", configure: func(reader *batchCoverageLimitReader) { reader.found = false }, wantError: errBatchSubmissionActorNotFound},
		{name: "actor disabled", configure: func(reader *batchCoverageLimitReader) { reader.enabled = false }, wantError: errBatchSubmissionActorNotAvailable},
		{name: "pending parent read", configure: func(reader *batchCoverageLimitReader) { reader.parentErr = sentinel }, wantError: sentinel},
		{name: "policy read", configure: func(reader *batchCoverageLimitReader) { reader.configErr = sentinel }, wantError: sentinel},
		{name: "global recent read", configure: func(reader *batchCoverageLimitReader) { reader.recentErr = sentinel }, wantError: sentinel},
		{name: "pending child read", configure: func(reader *batchCoverageLimitReader) { reader.childErr = sentinel }, wantError: sentinel},
		{name: "latest submission read", configure: func(reader *batchCoverageLimitReader) { reader.latestErr = sentinel }, wantError: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := batchCoverageLimitReader{found: true, enabled: true}
			test.configure(&reader)
			rateLimit, err := evaluateBatchSubmissionRateLimits(t.Context(), &reader, "owner-1", 1, time.Now().UTC())
			if rateLimit != nil {
				t.Fatalf("rate limit = %+v, want nil on read failure", rateLimit)
			}
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func TestWriteEarlyBatchReplayResponse_FailsClosedWithoutWorkflowWrites(t *testing.T) {
	t.Run("durable lookup error", func(t *testing.T) {
		srv, client := newBatchBehaviorTestServer(t)
		before := captureBatchCoverageCounts(t, srv)
		client.BatchTicket.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
				return nil, errors.New("batch replay projection unavailable")
			})
		}))

		ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch", "", "owner-1", nil)
		if handled := srv.writeEarlyBatchReplayResponse(ctx, "owner-1", "DELETE", "request-1", batchResourceType); !handled {
			t.Fatal("durable replay lookup error was not handled")
		}
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
		assertErrorCode(t, response.Body.Bytes(), "INTERNAL_ERROR")
		if after := captureBatchCoverageCounts(t, srv); after != before {
			t.Fatalf("failed replay lookup changed workflow counts: before=%+v after=%+v", before, after)
		}
	})

	t.Run("request cancellation", func(t *testing.T) {
		srv, _ := newBatchBehaviorTestServer(t)
		before := captureBatchCoverageCounts(t, srv)
		ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch", "", "owner-1", nil)
		requestContext, cancel := context.WithCancel(ctx.Request.Context())
		cancel()
		ctx.Request = ctx.Request.WithContext(requestContext)

		if handled := srv.writeEarlyBatchReplayResponse(ctx, "owner-1", "DELETE", "request-1", batchResourceType); !handled {
			t.Fatal("canceled replay lookup was not handled")
		}
		if response.Body.Len() != 0 {
			t.Fatalf("canceled lookup wrote a response body: %q", response.Body.String())
		}
		if after := captureBatchCoverageCounts(t, srv); after != before {
			t.Fatalf("canceled replay lookup changed workflow counts: before=%+v after=%+v", before, after)
		}
	})
}

func TestBatchSubmissionPersistenceReaders_RejectMissingDependencies(t *testing.T) {
	assertDependencyError := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s accepted a missing persistence dependency", name)
		}
	}

	entReader := entBatchSubmissionLimitReader{}
	_, _, err := entReader.activeUser(t.Context(), "owner-1")
	assertDependencyError("ent active user", err)
	_, _, err = entReader.pendingParentCounters(t.Context(), "owner-1")
	assertDependencyError("ent pending parents", err)
	_, err = entReader.userLimitConfig(t.Context(), "owner-1")
	assertDependencyError("ent user limit config", err)
	_, err = entReader.recentSubmissionCount(t.Context(), time.Now().UTC())
	assertDependencyError("ent recent submissions", err)
	_, err = entReader.pendingChildCount(t.Context(), "owner-1")
	assertDependencyError("ent pending children", err)
	_, _, err = entReader.latestSubmissionAt(t.Context(), "owner-1")
	assertDependencyError("ent latest submission", err)

	pgxReader := pgxBatchSubmissionLimitReader{}
	_, _, err = pgxReader.activeUser(t.Context(), "owner-1")
	assertDependencyError("pgx active user", err)
	_, _, err = pgxReader.pendingParentCounters(t.Context(), "owner-1")
	assertDependencyError("pgx pending parents", err)
	_, err = pgxReader.userLimitConfig(t.Context(), "owner-1")
	assertDependencyError("pgx user limit config", err)
	_, err = pgxReader.recentSubmissionCount(t.Context(), time.Now().UTC())
	assertDependencyError("pgx recent submissions", err)
	_, err = pgxReader.pendingChildCount(t.Context(), "owner-1")
	assertDependencyError("pgx pending children", err)
	_, _, err = pgxReader.latestSubmissionAt(t.Context(), "owner-1")
	assertDependencyError("pgx latest submission", err)
}

func TestBatchSubmissionPolicyAndHTTPFailureBoundaries(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
	maxParents := 8
	maxChildren := 80
	cooldownSeconds := 0
	reader := batchCoverageLimitReader{
		found:   true,
		enabled: true,
		config: batchUserLimitConfig{
			ExemptionFound:     true,
			ExemptionExpiresAt: &expiresAt,
			MaxPendingParents:  &maxParents,
			MaxPendingChildren: &maxChildren,
			CooldownSeconds:    &cooldownSeconds,
		},
	}
	policy, err := resolveBatchUserLimitPolicyWithReader(t.Context(), &reader, "owner-1", now)
	if err != nil {
		t.Fatalf("resolve explicit user policy: %v", err)
	}
	if !policy.Exempt || policy.ExemptionExpiresAt == nil || !policy.ExemptionExpiresAt.Equal(expiresAt) ||
		policy.MaxPendingParents != maxParents || policy.MaxPendingChildren != maxChildren || policy.Cooldown != 0 ||
		policy.UsesDefaultParents || policy.UsesDefaultChildren || policy.UsesDefaultCooldown {
		t.Fatalf("resolved explicit policy = %+v", policy)
	}
	expiresAt = time.Time{}
	if policy.ExemptionExpiresAt.IsZero() {
		t.Fatal("resolved policy retained the mutable reader expiration pointer")
	}

	actorErrors := []struct {
		err         error
		wantStatus  int
		wantCode    string
		wantHandled bool
	}{
		{err: errBatchSubmissionActorNotFound, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED", wantHandled: true},
		{err: errBatchSubmissionActorNotAvailable, wantStatus: http.StatusForbidden, wantCode: "FORBIDDEN", wantHandled: true},
		{err: errors.New("unrelated"), wantStatus: http.StatusOK, wantHandled: false},
	}
	for _, test := range actorErrors {
		ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch", "", "owner-1", nil)
		if handled := writeBatchSubmissionActorStateError(ctx, test.err); handled != test.wantHandled {
			t.Fatalf("actor error %v handled = %t, want %t", test.err, handled, test.wantHandled)
		}
		if response.Code != test.wantStatus {
			t.Fatalf("actor error %v status = %d, want %d", test.err, response.Code, test.wantStatus)
		}
		if test.wantCode != "" {
			assertErrorCode(t, response.Body.Bytes(), test.wantCode)
		}
	}

	var nilRateLimit *batchSubmissionRateLimitError
	if got := nilRateLimit.Error(); got != batchRateLimitedMessage {
		t.Fatalf("nil rate-limit error = %q", got)
	}
	if got := (&batchSubmissionRateLimitError{PendingParentLimit: true}).Error(); !strings.Contains(got, "pending parent") {
		t.Fatalf("pending-parent error = %q", got)
	}
	if got := (&batchSubmissionRateLimitError{Additional: &batchSubmissionLimitViolation{Reason: "cooldown"}}).Error(); !strings.Contains(got, "cooldown") {
		t.Fatalf("additional limit error = %q", got)
	}
	if got := (&batchSubmissionRateLimitError{}).Error(); got != batchRateLimitedMessage {
		t.Fatalf("empty rate-limit error = %q", got)
	}

	emptyContext, emptyResponse := newAuthedGinContext(t, http.MethodPost, "/vms/batch", "", "owner-1", nil)
	writeBatchSubmissionRateLimitResponse(emptyContext, nil, false)
	if emptyResponse.Body.Len() != 0 {
		t.Fatalf("nil rate limit wrote response body %q", emptyResponse.Body.String())
	}

	ctx, response := newAuthedGinContext(t, http.MethodPost, "/vms/batch/power", "", "owner-1", nil)
	writeBatchSubmissionRateLimitResponse(ctx, &batchSubmissionRateLimitError{
		Additional: &batchSubmissionLimitViolation{Reason: "user_submit_cooldown"},
		Policy: batchUserLimitPolicy{
			MaxPendingChildren: maxPendingBatchChildrenUser,
		},
		RequestedChildren: 1,
	}, true)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "2" {
		t.Fatalf("power limit response = status %d retry-after %q body=%s", response.Code, response.Header().Get("Retry-After"), response.Body.String())
	}
	limitResponse := decodeBatchCoverageError(t, response.Body.Bytes())
	if limitResponse.Code != "BATCH_RATE_LIMITED" || !strings.Contains(limitResponse.Message, "batch power") {
		t.Fatalf("power limit response = %+v", limitResponse)
	}
}

func TestBackfillMissingBatchProjection_RejectsUninitializedOrBlankInput(t *testing.T) {
	if err := (&Server{}).backfillMissingBatchProjection(t.Context(), "batch-1"); err == nil {
		t.Fatal("uninitialized server accepted projection backfill")
	}
	srv, _ := newBatchBehaviorTestServer(t)
	if err := srv.backfillMissingBatchProjection(t.Context(), "  "); err == nil {
		t.Fatal("blank parent id accepted projection backfill")
	}
}

func TestBatchPersistenceIdentityMappingsRejectUnknownValues(t *testing.T) {
	replayOperations := []struct {
		operation string
		wantOK    bool
	}{
		{operation: "create", wantOK: true},
		{operation: "modify", wantOK: true},
		{operation: "delete", wantOK: true},
		{operation: batchPowerOperationStart, wantOK: true},
		{operation: batchPowerOperationStop, wantOK: true},
		{operation: batchPowerOperationRestart, wantOK: true},
		{operation: "EXPORT", wantOK: false},
	}
	for _, test := range replayOperations {
		_, _, ok := batchReplayIdentityForOperation(test.operation)
		if ok != test.wantOK {
			t.Fatalf("replay operation %q valid = %t, want %t", test.operation, ok, test.wantOK)
		}
	}
	if !isBatchPowerReplayOperation(" power_start ") || isBatchPowerReplayOperation("DELETE") {
		t.Fatal("power replay scope accepted an invalid sibling operation")
	}

	eventTypes := []struct {
		eventType string
		wantOK    bool
	}{
		{eventType: string(domain.EventBatchCreateRequested), wantOK: true},
		{eventType: string(domain.EventBatchModifyRequested), wantOK: true},
		{eventType: string(domain.EventBatchDeleteRequested), wantOK: true},
		{eventType: string(domain.EventBatchPowerRequested), wantOK: true},
		{eventType: string(domain.EventVMStartRequested), wantOK: false},
	}
	for _, test := range eventTypes {
		_, ok := batchViewOperationForEvent(test.eventType)
		if ok != test.wantOK {
			t.Fatalf("event type %q public = %t, want %t", test.eventType, ok, test.wantOK)
		}
	}

	publicTypes := []struct {
		batchType entbatchticket.BatchType
		wantOK    bool
	}{
		{batchType: entbatchticket.BatchTypeBATCH_CREATE, wantOK: true},
		{batchType: entbatchticket.BatchTypeBATCH_MODIFY, wantOK: true},
		{batchType: entbatchticket.BatchTypeBATCH_DELETE, wantOK: true},
		{batchType: entbatchticket.BatchTypeBATCH_POWER, wantOK: true},
		{batchType: entbatchticket.BatchTypeBATCH_APPROVE, wantOK: false},
	}
	for _, test := range publicTypes {
		_, ok := publicBatchOperation(test.batchType)
		if ok != test.wantOK {
			t.Fatalf("batch type %q public = %t, want %t", test.batchType, ok, test.wantOK)
		}
	}
}

type batchCoverageWorkflowState struct {
	TicketStatus string
	EventStatus  string
	RejectReason string
	AttemptCount int
	LastAttempt  time.Time
	UpdatedAt    time.Time
}

type batchCoverageCounts struct {
	Tickets     int
	Events      int
	Projections int
}

func captureBatchCoverageCounts(t *testing.T, srv *Server) batchCoverageCounts {
	t.Helper()
	var counts batchCoverageCounts
	if err := srv.pool.QueryRow(t.Context(), `
SELECT
  (SELECT count(*) FROM tickets),
  (SELECT count(*) FROM domain_events),
  (SELECT count(*) FROM batch_tickets)
`).Scan(&counts.Tickets, &counts.Events, &counts.Projections); err != nil {
		t.Fatalf("capture batch workflow counts: %v", err)
	}
	return counts
}

func captureBatchCoverageWorkflow(t *testing.T, srv *Server, batchID string) map[string]batchCoverageWorkflowState {
	t.Helper()
	rows, err := srv.pool.Query(t.Context(), `
SELECT ticket.id, ticket.status, event.status, COALESCE(ticket.reject_reason, ''),
       ticket.attempt_count, COALESCE(ticket.last_attempt_at, 'epoch'::timestamptz), ticket.updated_at
FROM tickets AS ticket
JOIN domain_events AS event ON event.id = ticket.event_id
WHERE ticket.id = $1 OR ticket.parent_ticket_id = $1
ORDER BY ticket.id
`, batchID)
	if err != nil {
		t.Fatalf("query workflow state: %v", err)
	}
	defer rows.Close()

	states := make(map[string]batchCoverageWorkflowState)
	for rows.Next() {
		var id string
		var state batchCoverageWorkflowState
		if err := rows.Scan(
			&id,
			&state.TicketStatus,
			&state.EventStatus,
			&state.RejectReason,
			&state.AttemptCount,
			&state.LastAttempt,
			&state.UpdatedAt,
		); err != nil {
			t.Fatalf("scan workflow state: %v", err)
		}
		states[id] = state
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate workflow state: %v", err)
	}
	return states
}

func decodeBatchCoverageError(t *testing.T, body []byte) generated.Error {
	t.Helper()
	var response generated.Error
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, strings.TrimSpace(string(body)))
	}
	return response
}

type batchCoverageLimitReader struct {
	found     bool
	enabled   bool
	config    batchUserLimitConfig
	activeErr error
	parentErr error
	configErr error
	recentErr error
	childErr  error
	latestErr error
}

func (r *batchCoverageLimitReader) activeUser(context.Context, string) (found, enabled bool, err error) {
	return r.found, r.enabled, r.activeErr
}

func (r *batchCoverageLimitReader) pendingParentCounters(context.Context, string) (global, user int, err error) {
	return 0, 0, r.parentErr
}

func (r *batchCoverageLimitReader) userLimitConfig(context.Context, string) (batchUserLimitConfig, error) {
	return r.config, r.configErr
}

func (r *batchCoverageLimitReader) recentSubmissionCount(context.Context, time.Time) (int, error) {
	return 0, r.recentErr
}

func (r *batchCoverageLimitReader) pendingChildCount(context.Context, string) (int, error) {
	return 0, r.childErr
}

func (r *batchCoverageLimitReader) latestSubmissionAt(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, r.latestErr
}
