package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/batchapprovalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/ratelimitexemption"
	"kv-shepherd.io/shepherd/ent/ratelimituseroverride"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	maxBatchItems                   = 100
	maxPendingBatchParents          = 100
	maxPendingBatchParentsUser      = 3
	maxPendingBatchChildrenUser     = 30
	maxGlobalBatchRequestsPerMinute = 1000
	batchSubmitCooldown             = 2 * time.Minute
	batchRetryAfterSeconds          = 2
)

var errBatchNotFound = errors.New("batch not found")

type preparedBatchChild struct {
	eventType     domain.EventType
	aggregateID   string
	payload       []byte
	operationType approvalticket.OperationType
	reason        string
}

type batchValidationError struct {
	status int
	body   generated.Error
}

func (e *batchValidationError) Error() string {
	return e.body.Code + ": " + e.body.Message
}

// SubmitVMBatch handles POST /vms/batch.
func (s *Server) SubmitVMBatch(c *gin.Context) {
	s.submitBatch(c)
}

// SubmitApprovalBatch handles POST /approvals/batch compatibility endpoint.
func (s *Server) SubmitApprovalBatch(c *gin.Context) {
	s.submitBatch(c)
}

// SubmitVMBatchPower handles POST /vms/batch/power compatibility endpoint.
func (s *Server) SubmitVMBatchPower(c *gin.Context) {
	s.submitBatchPower(c)
}

func (s *Server) submitBatch(c *gin.Context) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMBatchSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	if len(req.Items) == 0 || len(req.Items) > maxBatchItems {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_SIZE",
			Message: fmt.Sprintf("batch size must be between 1 and %d", maxBatchItems),
		})
		return
	}

	op, parentEventType, err := normalizeBatchOperation(req.Operation)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_OPERATION", Message: err.Error()})
		return
	}
	if op == string(generated.VMBatchOperationDELETE) {
		if !requireGlobalPermission(c, "vm:delete") {
			return
		}
	} else {
		if !requireGlobalPermission(c, "vm:create") {
			return
		}
	}

	if strings.TrimSpace(req.RequestId) != "" {
		if existingID, ok, err := s.findBatchByRequestID(ctx, actor, op, req.RequestId); err != nil {
			logger.Error("failed to query batch idempotency", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		} else if ok {
			c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
				BatchId:           existingID,
				Status:            generated.VMBatchParentStatusPENDINGAPPROVAL,
				StatusUrl:         "/api/v1/vms/batch/" + existingID,
				RetryAfterSeconds: batchRetryAfterSeconds,
			})
			return
		}
	}

	globalPending, userPending, err := s.pendingBatchParentCounters(ctx, actor)
	if err != nil {
		logger.Error("failed to evaluate batch submission limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	limitPolicy, err := s.resolveBatchUserLimitPolicy(ctx, actor)
	if err != nil {
		logger.Error("failed to resolve batch user limit policy", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	userPendingParentExceeded := !limitPolicy.Exempt && userPending >= limitPolicy.MaxPendingParents
	if globalPending >= maxPendingBatchParents || userPendingParentExceeded {
		c.Header("Retry-After", strconv.Itoa(batchRetryAfterSeconds))
		contactAdmin := !limitPolicy.Exempt && limitPolicy.UsesDefault
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch submission throttled by pending parent limits",
			Params: map[string]interface{}{
				"retry_after_seconds": batchRetryAfterSeconds,
				"global_pending":      globalPending,
				"user_pending":        userPending,
				"user_exempted":       limitPolicy.Exempt,
				"max_user_pending":    limitPolicy.MaxPendingParents,
				"contact_admin":       contactAdmin,
			},
		})
		return
	}
	if extraLimit, err := s.evaluateAdditionalBatchSubmissionLimits(ctx, actor, len(req.Items), limitPolicy); err != nil {
		logger.Error("failed to evaluate additional batch submission limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	} else if extraLimit != nil {
		retryAfter := extraLimit.RetryAfterSeconds
		if retryAfter <= 0 {
			retryAfter = batchRetryAfterSeconds
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch submission throttled by additional rate limits",
			Params: map[string]interface{}{
				"reason":                    extraLimit.Reason,
				"retry_after_seconds":       retryAfter,
				"global_recent_submits":     extraLimit.GlobalRecentSubmits,
				"user_pending_children":     extraLimit.UserPendingChildren,
				"user_cooldown_seconds":     extraLimit.UserCooldownSeconds,
				"requested_child_count":     len(req.Items),
				"max_global_per_minute":     maxGlobalBatchRequestsPerMinute,
				"max_user_pending_children": limitPolicy.MaxPendingChildren,
				"user_exempted":             limitPolicy.Exempt,
				"contact_admin":             !limitPolicy.Exempt && limitPolicy.UsesDefault,
			},
		})
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility for batch submit", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	children, err := s.prepareBatchChildren(ctx, actor, op, req, visibility)
	if err != nil {
		if appErr, ok := err.(*batchValidationError); ok {
			c.JSON(appErr.status, appErr.body)
			return
		}
		logger.Error("failed to prepare batch child tickets", zap.Error(err))
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_ITEMS", Message: err.Error()})
		return
	}

	parentID := generateIDV7()
	parentPayload := domain.BatchVMRequestPayload{
		Operation:   op,
		RequestID:   strings.TrimSpace(req.RequestId),
		Reason:      strings.TrimSpace(req.Reason),
		SubmittedBy: actor,
		SubmittedAt: time.Now().UTC(),
		Items:       buildBatchPayloadItems(op, req.Items),
	}
	parentPayloadBytes, err := parentPayload.ToJSON()
	if err != nil {
		logger.Error("failed to marshal parent batch payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to begin batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()

	parentEventID := generateIDV7()
	_, err = tx.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(parentEventType)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create parent batch domain event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	parentBuilder := tx.ApprovalTicket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetRequester(actor).
		SetStatus(approvalticket.StatusPENDING)
	if op == string(generated.VMBatchOperationDELETE) {
		parentBuilder = parentBuilder.SetOperationType(approvalticket.OperationTypeDELETE)
	} else {
		parentBuilder = parentBuilder.SetOperationType(approvalticket.OperationTypeCREATE)
	}
	parentReason := strings.TrimSpace(req.Reason)
	if parentReason == "" {
		parentReason = fmt.Sprintf("batch %s request (%d items)", strings.ToLower(op), len(children))
	}
	parentBuilder = parentBuilder.SetReason(parentReason)
	if _, err := parentBuilder.Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create parent batch approval ticket", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := tx.BatchApprovalTicket.Create().
		SetID(parentID).
		SetBatchType(toBatchProjectionType(op)).
		SetChildCount(len(children)).
		SetPendingCount(len(children)).
		SetStatus(batchapprovalticket.StatusPENDING_APPROVAL).
		SetCreatedBy(actor).
		SetReason(parentReason).
		SetNillableRequestID(nillableTrimmed(req.RequestId)).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create batch projection row", zap.Error(err), zap.String("batch_id", parentID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	for _, child := range children {
		childEventID := generateIDV7()
		_, err := tx.DomainEvent.Create().
			SetID(childEventID).
			SetEventType(string(child.eventType)).
			SetAggregateType("vm").
			SetAggregateID(child.aggregateID).
			SetPayload(child.payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy(actor).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create child domain event", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		_, err = tx.ApprovalTicket.Create().
			SetID(generateIDV7()).
			SetEventID(childEventID).
			SetOperationType(child.operationType).
			SetStatus(approvalticket.StatusPENDING).
			SetRequester(actor).
			SetReason(child.reason).
			SetParentTicketID(parentID).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create child approval ticket", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm.batch.submit", "approval_ticket", parentID, actor, map[string]interface{}{
			"operation":  op,
			"item_count": len(children),
		})
	}

	c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
		BatchId:           parentID,
		Status:            generated.VMBatchParentStatusPENDINGAPPROVAL,
		StatusUrl:         "/api/v1/vms/batch/" + parentID,
		RetryAfterSeconds: batchRetryAfterSeconds,
	})
}

func (s *Server) submitBatchPower(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMBatchPowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	if len(req.Items) == 0 || len(req.Items) > maxBatchItems {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_SIZE",
			Message: fmt.Sprintf("batch size must be between 1 and %d", maxBatchItems),
		})
		return
	}

	opKey, jobOperation, childEventType, err := normalizeBatchPowerOperation(req.Operation)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_OPERATION",
			Message: err.Error(),
		})
		return
	}

	if strings.TrimSpace(req.RequestId) != "" {
		if existingID, ok, err := s.findBatchByRequestID(ctx, actor, opKey, req.RequestId); err != nil {
			logger.Error("failed to query power-batch idempotency", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		} else if ok {
			c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
				BatchId:           existingID,
				Status:            generated.VMBatchParentStatusINPROGRESS,
				StatusUrl:         "/api/v1/vms/batch/" + existingID,
				RetryAfterSeconds: batchRetryAfterSeconds,
			})
			return
		}
	}

	globalPending, userPending, err := s.pendingBatchParentCounters(ctx, actor)
	if err != nil {
		logger.Error("failed to evaluate power-batch submission limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	limitPolicy, err := s.resolveBatchUserLimitPolicy(ctx, actor)
	if err != nil {
		logger.Error("failed to resolve power-batch user limit policy", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	userPendingParentExceeded := !limitPolicy.Exempt && userPending >= limitPolicy.MaxPendingParents
	if globalPending >= maxPendingBatchParents || userPendingParentExceeded {
		c.Header("Retry-After", strconv.Itoa(batchRetryAfterSeconds))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch power submission throttled by pending parent limits",
			Params: map[string]interface{}{
				"retry_after_seconds": batchRetryAfterSeconds,
				"global_pending":      globalPending,
				"user_pending":        userPending,
				"user_exempted":       limitPolicy.Exempt,
				"max_user_pending":    limitPolicy.MaxPendingParents,
			},
		})
		return
	}
	if extraLimit, err := s.evaluateAdditionalBatchSubmissionLimits(ctx, actor, len(req.Items), limitPolicy); err != nil {
		logger.Error("failed to evaluate power-batch additional limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	} else if extraLimit != nil {
		retryAfter := extraLimit.RetryAfterSeconds
		if retryAfter <= 0 {
			retryAfter = batchRetryAfterSeconds
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch power submission throttled by additional rate limits",
			Params: map[string]interface{}{
				"reason":                    extraLimit.Reason,
				"retry_after_seconds":       retryAfter,
				"global_recent_submits":     extraLimit.GlobalRecentSubmits,
				"user_pending_children":     extraLimit.UserPendingChildren,
				"user_cooldown_seconds":     extraLimit.UserCooldownSeconds,
				"requested_child_count":     len(req.Items),
				"max_global_per_minute":     maxGlobalBatchRequestsPerMinute,
				"max_user_pending_children": limitPolicy.MaxPendingChildren,
				"user_exempted":             limitPolicy.Exempt,
			},
		})
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility for power-batch submit", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	children, err := s.prepareBatchPowerChildren(ctx, actor, jobOperation, childEventType, req, visibility)
	if err != nil {
		if appErr, ok := err.(*batchValidationError); ok {
			c.JSON(appErr.status, appErr.body)
			return
		}
		logger.Error("failed to prepare power-batch child tickets", zap.Error(err))
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_ITEMS", Message: err.Error()})
		return
	}

	parentID := generateIDV7()
	parentPayload := domain.BatchVMRequestPayload{
		Operation:   opKey,
		RequestID:   strings.TrimSpace(req.RequestId),
		Reason:      strings.TrimSpace(req.Reason),
		SubmittedBy: actor,
		SubmittedAt: time.Now().UTC(),
		Items:       buildBatchPowerPayloadItems(req.Items),
	}
	parentPayloadBytes, err := parentPayload.ToJSON()
	if err != nil {
		logger.Error("failed to marshal power-batch parent payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to begin power-batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()

	parentEventID := generateIDV7()
	_, err = tx.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayloadBytes).
		SetStatus(domainevent.StatusPROCESSING).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch parent domain event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	parentReason := strings.TrimSpace(req.Reason)
	if parentReason == "" {
		parentReason = fmt.Sprintf("batch power %s request (%d items)", strings.ToLower(jobOperation), len(children))
	}
	if _, err := tx.ApprovalTicket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetOperationType(approvalticket.OperationTypeCREATE).
		SetStatus(approvalticket.StatusEXECUTING).
		SetRequester(actor).
		SetReason(parentReason).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch parent approval ticket", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := tx.BatchApprovalTicket.Create().
		SetID(parentID).
		SetBatchType(batchapprovalticket.BatchTypeBATCH_POWER).
		SetChildCount(len(children)).
		SetPendingCount(len(children)).
		SetStatus(batchapprovalticket.StatusIN_PROGRESS).
		SetCreatedBy(actor).
		SetReason(parentReason).
		SetNillableRequestID(nillableTrimmed(req.RequestId)).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch projection row", zap.Error(err), zap.String("batch_id", parentID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	childEventIDs := make([]string, 0, len(children))
	for _, child := range children {
		childEventID := generateIDV7()
		_, err := tx.DomainEvent.Create().
			SetID(childEventID).
			SetEventType(string(child.eventType)).
			SetAggregateType("vm").
			SetAggregateID(child.aggregateID).
			SetPayload(child.payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy(actor).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create power-batch child domain event", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if _, err := tx.ApprovalTicket.Create().
			SetID(generateIDV7()).
			SetEventID(childEventID).
			SetOperationType(approvalticket.OperationTypeCREATE).
			SetStatus(approvalticket.StatusEXECUTING).
			SetRequester(actor).
			SetReason(child.reason).
			SetParentTicketID(parentID).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create power-batch child approval ticket", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		childEventIDs = append(childEventIDs, childEventID)
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit power-batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	for _, eventID := range childEventIDs {
		if err := s.enqueueBatchPowerJob(ctx, eventID, strings.ToLower(jobOperation)); err != nil {
			logger.Warn("failed to enqueue power-batch child job",
				zap.String("event_id", eventID),
				zap.String("batch_id", parentID),
				zap.Error(err),
			)
			_, _ = s.client.ApprovalTicket.Update().
				Where(approvalticket.EventIDEQ(eventID)).
				SetStatus(approvalticket.StatusFAILED).
				SetRejectReason("enqueue vm_power job failed").
				Save(ctx)
			_, _ = s.client.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		}
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm.batch.power.submit", "approval_ticket", parentID, actor, map[string]interface{}{
			"operation":  strings.ToLower(jobOperation),
			"item_count": len(children),
		})
	}

	status := generated.VMBatchParentStatusINPROGRESS
	if view, _, err := s.loadBatchView(ctx, parentID); err == nil {
		status = view.Status
	}
	c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
		BatchId:           parentID,
		Status:            status,
		StatusUrl:         "/api/v1/vms/batch/" + parentID,
		RetryAfterSeconds: batchRetryAfterSeconds,
	})
}

// GetVMBatch handles GET /vms/batch/{batch_id}.
func (s *Server) GetVMBatch(c *gin.Context, batchId generated.BatchID) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:read", "vm:create", "vm:delete", "vm:operate") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	resp, _, err := s.loadBatchView(ctx, string(batchId))
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, errBatchNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		logger.Error("failed to load batch view", zap.Error(err), zap.String("batch_id", string(batchId)))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !hasPlatformAdmin(c) && resp.CreatedBy != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RetryVMBatch handles POST /vms/batch/{batch_id}/retry.
func (s *Server) RetryVMBatch(c *gin.Context, batchId generated.BatchID) {
	s.mutateBatchChildren(c, string(batchId), "retry")
}

// CancelVMBatch handles POST /vms/batch/{batch_id}/cancel.
func (s *Server) CancelVMBatch(c *gin.Context, batchId generated.BatchID) {
	s.mutateBatchChildren(c, string(batchId), "cancel")
}

func (s *Server) mutateBatchChildren(c *gin.Context, batchID string, action string) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:create", "vm:delete", "vm:operate") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	resp, children, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, errBatchNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		logger.Error("failed to load batch for action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !hasPlatformAdmin(c) && resp.CreatedBy != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
		return
	}
	parentTicket, err := s.client.ApprovalTicket.Get(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		logger.Error("failed to load parent batch ticket", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	parentEvent, err := s.client.DomainEvent.Get(ctx, parentTicket.EventID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		logger.Error("failed to load parent batch event", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	isPowerBatch := domain.EventType(parentEvent.EventType) == domain.EventBatchPowerRequested

	targetIDs := make([]string, 0)
	targetEventIDs := make([]string, 0)
	targetChildren := make([]*ent.ApprovalTicket, 0)
	for _, child := range children {
		switch action {
		case "retry":
			if child.Status == approvalticket.StatusFAILED || child.Status == approvalticket.StatusREJECTED {
				targetIDs = append(targetIDs, child.ID)
				targetEventIDs = append(targetEventIDs, child.EventID)
				targetChildren = append(targetChildren, child)
			}
		case "cancel":
			if child.Status == approvalticket.StatusPENDING {
				targetIDs = append(targetIDs, child.ID)
				targetEventIDs = append(targetEventIDs, child.EventID)
				targetChildren = append(targetChildren, child)
			}
		}
	}

	affectedCount := 0
	affectedTicketIDs := make([]string, 0)
	if len(targetIDs) > 0 {
		if action == "retry" {
			retryStatus := approvalticket.StatusPENDING
			if isPowerBatch {
				retryStatus = approvalticket.StatusEXECUTING
			}
			if _, err := s.client.ApprovalTicket.Update().
				Where(approvalticket.IDIn(targetIDs...)).
				SetStatus(retryStatus).
				ClearRejectReason().
				Save(ctx); err != nil {
				logger.Error("failed to reset child tickets for retry", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			if _, err := s.client.DomainEvent.Update().
				Where(domainevent.IDIn(targetEventIDs...)).
				SetStatus(domainevent.StatusPENDING).
				Save(ctx); err != nil {
				logger.Error("failed to reset child events for retry", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}

			for _, child := range targetChildren {
				if isPowerBatch {
					ev, err := s.client.DomainEvent.Get(ctx, child.EventID)
					if err != nil {
						_, _ = s.client.ApprovalTicket.UpdateOneID(child.ID).
							SetStatus(approvalticket.StatusFAILED).
							SetRejectReason("failed to load child event during power retry").
							Save(ctx)
						_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
							SetStatus(domainevent.StatusFAILED).
							Save(ctx)
						logger.Warn("failed to load child event during power-batch retry",
							zap.String("ticket_id", child.ID),
							zap.String("batch_id", batchID),
							zap.Error(err),
						)
						continue
					}
					var powerPayload domain.VMPowerPayload
					if err := json.Unmarshal(ev.Payload, &powerPayload); err != nil {
						_, _ = s.client.ApprovalTicket.UpdateOneID(child.ID).
							SetStatus(approvalticket.StatusFAILED).
							SetRejectReason("invalid power payload for retry").
							Save(ctx)
						_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
							SetStatus(domainevent.StatusFAILED).
							Save(ctx)
						logger.Warn("failed to parse child power payload during retry",
							zap.String("ticket_id", child.ID),
							zap.String("batch_id", batchID),
							zap.Error(err),
						)
						continue
					}
					op := strings.ToLower(strings.TrimSpace(powerPayload.Operation))
					if op != "start" && op != "stop" && op != "restart" {
						_, _ = s.client.ApprovalTicket.UpdateOneID(child.ID).
							SetStatus(approvalticket.StatusFAILED).
							SetRejectReason("unknown power operation for retry").
							Save(ctx)
						_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
							SetStatus(domainevent.StatusFAILED).
							Save(ctx)
						logger.Warn("unknown power operation in child payload during retry",
							zap.String("ticket_id", child.ID),
							zap.String("batch_id", batchID),
							zap.String("operation", powerPayload.Operation),
						)
						continue
					}
					if err := s.enqueueBatchPowerJob(ctx, child.EventID, op); err != nil {
						_, _ = s.client.ApprovalTicket.UpdateOneID(child.ID).
							SetStatus(approvalticket.StatusFAILED).
							SetRejectReason("failed to enqueue vm_power job").
							Save(ctx)
						_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
							SetStatus(domainevent.StatusFAILED).
							Save(ctx)
						logger.Warn("failed to enqueue power child during batch retry",
							zap.String("ticket_id", child.ID),
							zap.String("batch_id", batchID),
							zap.Error(err),
						)
						continue
					}
					affectedCount++
					affectedTicketIDs = append(affectedTicketIDs, child.ID)
					continue
				}
				if err := s.gateway.Approve(
					ctx,
					child.ID,
					actor,
					parentTicket.SelectedClusterID,
					parentTicket.SelectedStorageClass,
				); err != nil {
					message := err.Error()
					if len(message) > 512 {
						message = message[:512]
					}
					_, _ = s.client.ApprovalTicket.UpdateOneID(child.ID).
						SetStatus(approvalticket.StatusFAILED).
						SetRejectReason(message).
						Save(ctx)
					_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
						SetStatus(domainevent.StatusFAILED).
						Save(ctx)
					logger.Warn("failed to re-approve child ticket during batch retry",
						zap.String("ticket_id", child.ID),
						zap.String("batch_id", batchID),
						zap.Error(err),
					)
					continue
				}
				affectedCount++
				affectedTicketIDs = append(affectedTicketIDs, child.ID)
			}
		} else {
			if _, err := s.client.ApprovalTicket.Update().
				Where(approvalticket.IDIn(targetIDs...)).
				SetStatus(approvalticket.StatusCANCELLED).
				Save(ctx); err != nil {
				logger.Error("failed to mutate child tickets", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			if _, err := s.client.DomainEvent.Update().
				Where(domainevent.IDIn(targetEventIDs...)).
				SetStatus(domainevent.StatusCANCELLED).
				Save(ctx); err != nil {
				logger.Error("failed to mutate child events", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			affectedCount = len(targetIDs)
			affectedTicketIDs = append(affectedTicketIDs, targetIDs...)
		}
	}

	updated, _, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		logger.Error("failed to reload batch after action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, generated.VMBatchActionResponse{
		BatchId:           batchID,
		Status:            updated.Status,
		AffectedCount:     affectedCount,
		AffectedTicketIds: affectedTicketIDs,
	})
}

func (s *Server) prepareBatchChildren(
	ctx context.Context,
	actor string,
	op string,
	req generated.VMBatchSubmitRequest,
	visibility namespaceVisibility,
) ([]preparedBatchChild, error) {
	children := make([]preparedBatchChild, 0, len(req.Items))

	for idx, item := range req.Items {
		itemReason := strings.TrimSpace(item.Reason)
		if itemReason == "" {
			itemReason = strings.TrimSpace(req.Reason)
		}
		if itemReason == "" {
			itemReason = fmt.Sprintf("batch %s item #%d", strings.ToLower(op), idx+1)
		}

		switch op {
		case string(generated.VMBatchOperationCREATE):
			serviceID := strings.TrimSpace(item.ServiceId.String())
			templateID := strings.TrimSpace(item.TemplateId.String())
			instanceSizeID := strings.TrimSpace(item.InstanceSizeId.String())
			namespace := strings.TrimSpace(item.Namespace)
			if isZeroUUID(item.ServiceId) || isZeroUUID(item.TemplateId) || isZeroUUID(item.InstanceSizeId) || namespace == "" {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_BATCH_ITEM",
						Message: fmt.Sprintf("create item #%d requires service_id/template_id/instance_size_id/namespace", idx+1),
					},
				}
			}
			visible, err := s.isNamespaceVisible(ctx, namespace, visibility)
			if err != nil {
				return nil, err
			}
			if !visible {
				return nil, &batchValidationError{
					status: http.StatusForbidden,
					body: generated.Error{
						Code:    "NAMESPACE_ENV_FORBIDDEN",
						Message: fmt.Sprintf("namespace %q is outside allowed environment visibility", namespace),
					},
				}
			}
			payload := domain.VMCreationPayload{
				RequesterID:    actor,
				ServiceID:      serviceID,
				TemplateID:     templateID,
				InstanceSizeID: instanceSizeID,
				Namespace:      namespace,
				Reason:         itemReason,
			}
			payloadBytes, err := payload.ToJSON()
			if err != nil {
				return nil, err
			}
			children = append(children, preparedBatchChild{
				eventType:     domain.EventVMCreationRequested,
				aggregateID:   serviceID,
				payload:       payloadBytes,
				operationType: approvalticket.OperationTypeCREATE,
				reason:        itemReason,
			})

		case string(generated.VMBatchOperationDELETE):
			vmID := strings.TrimSpace(item.VmId)
			if vmID == "" {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_BATCH_ITEM",
						Message: fmt.Sprintf("delete item #%d requires vm_id", idx+1),
					},
				}
			}
			vmObj, err := s.client.VM.Get(ctx, vmID)
			if err != nil {
				if ent.IsNotFound(err) {
					return nil, &batchValidationError{
						status: http.StatusBadRequest,
						body: generated.Error{
							Code:    "VM_NOT_FOUND",
							Message: fmt.Sprintf("vm %q not found", vmID),
						},
					}
				}
				return nil, err
			}
			visible, err := s.isNamespaceVisible(ctx, vmObj.Namespace, visibility)
			if err != nil {
				return nil, err
			}
			if !visible {
				return nil, &batchValidationError{
					status: http.StatusForbidden,
					body: generated.Error{
						Code:    "NAMESPACE_ENV_FORBIDDEN",
						Message: fmt.Sprintf("vm namespace %q is outside allowed environment visibility", vmObj.Namespace),
					},
				}
			}
			payload := domain.VMDeletePayload{
				VMID:      vmObj.ID,
				VMName:    vmObj.Name,
				ClusterID: vmObj.ClusterID,
				Namespace: vmObj.Namespace,
				Actor:     actor,
			}
			payloadBytes, err := payload.ToJSON()
			if err != nil {
				return nil, err
			}
			children = append(children, preparedBatchChild{
				eventType:     domain.EventVMDeletionRequested,
				aggregateID:   vmObj.ID,
				payload:       payloadBytes,
				operationType: approvalticket.OperationTypeDELETE,
				reason:        itemReason,
			})
		}
	}

	return children, nil
}

func normalizeBatchOperation(op generated.VMBatchOperation) (string, domain.EventType, error) {
	switch op {
	case generated.VMBatchOperationCREATE:
		return string(op), domain.EventBatchCreateRequested, nil
	case generated.VMBatchOperationDELETE:
		return string(op), domain.EventBatchDeleteRequested, nil
	default:
		return "", "", fmt.Errorf("unsupported operation %q", op)
	}
}

func normalizeBatchPowerOperation(op generated.VMBatchPowerAction) (opKey string, jobOperation string, childEventType domain.EventType, err error) {
	switch strings.TrimSpace(strings.ToUpper(string(op))) {
	case "START":
		return "POWER_START", "START", domain.EventVMStartRequested, nil
	case "STOP":
		return "POWER_STOP", "STOP", domain.EventVMStopRequested, nil
	case "RESTART":
		return "POWER_RESTART", "RESTART", domain.EventVMRestartRequested, nil
	default:
		return "", "", "", fmt.Errorf("unsupported power operation %q", op)
	}
}

func (s *Server) prepareBatchPowerChildren(
	ctx context.Context,
	actor string,
	jobOperation string,
	childEventType domain.EventType,
	req generated.VMBatchPowerRequest,
	visibility namespaceVisibility,
) ([]preparedBatchChild, error) {
	children := make([]preparedBatchChild, 0, len(req.Items))
	for idx, item := range req.Items {
		vmID := strings.TrimSpace(item.VmId)
		if vmID == "" {
			return nil, &batchValidationError{
				status: http.StatusBadRequest,
				body: generated.Error{
					Code:    "INVALID_BATCH_ITEM",
					Message: fmt.Sprintf("power item #%d requires vm_id", idx+1),
				},
			}
		}
		vmObj, err := s.client.VM.Get(ctx, vmID)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "VM_NOT_FOUND",
						Message: fmt.Sprintf("vm %q not found", vmID),
					},
				}
			}
			return nil, err
		}
		visible, err := s.isNamespaceVisible(ctx, vmObj.Namespace, visibility)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, &batchValidationError{
				status: http.StatusForbidden,
				body: generated.Error{
					Code:    "NAMESPACE_ENV_FORBIDDEN",
					Message: fmt.Sprintf("vm namespace %q is outside allowed environment visibility", vmObj.Namespace),
				},
			}
		}

		itemReason := strings.TrimSpace(item.Reason)
		if itemReason == "" {
			itemReason = strings.TrimSpace(req.Reason)
		}
		if itemReason == "" {
			itemReason = fmt.Sprintf("batch power item #%d", idx+1)
		}

		payload := domain.VMPowerPayload{
			VMID:      vmObj.ID,
			VMName:    vmObj.Name,
			ClusterID: vmObj.ClusterID,
			Namespace: vmObj.Namespace,
			Operation: strings.ToLower(jobOperation),
			Actor:     actor,
		}
		payloadBytes, err := payload.ToJSON()
		if err != nil {
			return nil, err
		}
		children = append(children, preparedBatchChild{
			eventType:     childEventType,
			aggregateID:   vmObj.ID,
			payload:       payloadBytes,
			operationType: approvalticket.OperationTypeCREATE,
			reason:        itemReason,
		})
	}

	return children, nil
}

func (s *Server) enqueueBatchPowerJob(ctx context.Context, eventID, operation string) error {
	if s.riverClient == nil {
		return fmt.Errorf("river client is not configured")
	}
	_, err := s.riverClient.Insert(ctx, jobs.VMPowerArgs{
		EventID:   eventID,
		Operation: operation,
	}, nil)
	return err
}

func batchParentEventTypes() []string {
	return []string{
		string(domain.EventBatchCreateRequested),
		string(domain.EventBatchDeleteRequested),
		string(domain.EventBatchPowerRequested),
	}
}

func (s *Server) pendingBatchParentCounters(ctx context.Context, actor string) (int, int, error) {
	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.StatusIn(domainevent.StatusPENDING, domainevent.StatusPROCESSING),
		).
		All(ctx)
	if err != nil {
		return 0, 0, err
	}

	global := len(events)
	user := 0
	for _, ev := range events {
		if ev.CreatedBy == actor {
			user++
		}
	}
	return global, user, nil
}

type batchSubmissionLimitViolation struct {
	Reason              string
	RetryAfterSeconds   int
	GlobalRecentSubmits int
	UserPendingChildren int
	UserCooldownSeconds int
}

type batchUserLimitPolicy struct {
	Exempt             bool
	UsesDefault        bool
	MaxPendingParents  int
	MaxPendingChildren int
	Cooldown           time.Duration
	ExemptionExpiresAt *time.Time
}

func defaultBatchUserLimitPolicy() batchUserLimitPolicy {
	return batchUserLimitPolicy{
		Exempt:             false,
		UsesDefault:        true,
		MaxPendingParents:  maxPendingBatchParentsUser,
		MaxPendingChildren: maxPendingBatchChildrenUser,
		Cooldown:           batchSubmitCooldown,
	}
}

func (s *Server) resolveBatchUserLimitPolicy(ctx context.Context, actor string) (batchUserLimitPolicy, error) {
	policy := defaultBatchUserLimitPolicy()

	exemption, err := s.client.RateLimitExemption.Query().
		Where(ratelimitexemption.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return policy, err
	}
	if err == nil {
		if exemption.ExpiresAt != nil && exemption.ExpiresAt.Before(time.Now().UTC()) {
			if delErr := s.client.RateLimitExemption.DeleteOneID(actor).Exec(ctx); delErr != nil {
				logger.Warn("failed to purge expired rate-limit exemption",
					zap.String("user_id", actor),
					zap.Error(delErr),
				)
			}
		} else {
			policy.Exempt = true
			policy.UsesDefault = false
			if exemption.ExpiresAt != nil {
				exp := *exemption.ExpiresAt
				policy.ExemptionExpiresAt = &exp
			}
		}
	}

	override, err := s.client.RateLimitUserOverride.Query().
		Where(ratelimituseroverride.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return policy, err
	}
	if err == nil {
		policy.UsesDefault = false
		if override.MaxPendingParents != nil && *override.MaxPendingParents > 0 {
			policy.MaxPendingParents = *override.MaxPendingParents
		}
		if override.MaxPendingChildren != nil && *override.MaxPendingChildren > 0 {
			policy.MaxPendingChildren = *override.MaxPendingChildren
		}
		if override.CooldownSeconds != nil && *override.CooldownSeconds >= 0 {
			policy.Cooldown = time.Duration(*override.CooldownSeconds) * time.Second
		}
	}

	return policy, nil
}

func (s *Server) evaluateAdditionalBatchSubmissionLimits(
	ctx context.Context,
	actor string,
	requestedChildCount int,
	policy batchUserLimitPolicy,
) (*batchSubmissionLimitViolation, error) {
	recentSince := time.Now().UTC().Add(-time.Minute)
	globalRecentSubmits, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedAtGTE(recentSince),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if globalRecentSubmits >= maxGlobalBatchRequestsPerMinute {
		return &batchSubmissionLimitViolation{
			Reason:              "global_request_rate_limit",
			RetryAfterSeconds:   60,
			GlobalRecentSubmits: globalRecentSubmits,
		}, nil
	}

	if policy.Exempt {
		return nil, nil
	}

	userPendingChildren, err := s.client.ApprovalTicket.Query().
		Where(
			approvalticket.RequesterEQ(actor),
			approvalticket.ParentTicketIDNotNil(),
			approvalticket.StatusIn(
				approvalticket.StatusPENDING,
				approvalticket.StatusAPPROVED,
				approvalticket.StatusEXECUTING,
			),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if userPendingChildren+requestedChildCount > policy.MaxPendingChildren {
		return &batchSubmissionLimitViolation{
			Reason:              "user_pending_child_limit",
			RetryAfterSeconds:   batchRetryAfterSeconds,
			GlobalRecentSubmits: globalRecentSubmits,
			UserPendingChildren: userPendingChildren,
		}, nil
	}

	lastEvent, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedByEQ(actor),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		cooldownRemaining := time.Until(lastEvent.CreatedAt.Add(policy.Cooldown))
		if cooldownRemaining > 0 {
			cooldownSeconds := int(math.Ceil(cooldownRemaining.Seconds()))
			return &batchSubmissionLimitViolation{
				Reason:              "user_submit_cooldown",
				RetryAfterSeconds:   cooldownSeconds,
				GlobalRecentSubmits: globalRecentSubmits,
				UserPendingChildren: userPendingChildren,
				UserCooldownSeconds: cooldownSeconds,
			}, nil
		}
	}

	return nil, nil
}

func (s *Server) findBatchByRequestID(ctx context.Context, actor, op, requestID string) (string, bool, error) {
	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedByEQ(actor),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return "", false, err
	}

	for _, ev := range events {
		var payload domain.BatchVMRequestPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.RequestID) != requestID || strings.TrimSpace(payload.Operation) != op {
			continue
		}
		parentExists, err := s.client.ApprovalTicket.Query().
			Where(
				approvalticket.IDEQ(ev.AggregateID),
				approvalticket.EventIDEQ(ev.ID),
				approvalticket.ParentTicketIDIsNil(),
			).
			Exist(ctx)
		if err != nil {
			return "", false, err
		}
		if parentExists {
			return ev.AggregateID, true, nil
		}
	}

	return "", false, nil
}

func (s *Server) loadBatchView(ctx context.Context, batchID string) (generated.VMBatchStatusResponse, []*ent.ApprovalTicket, error) {
	parent, err := s.client.ApprovalTicket.Query().
		Where(
			approvalticket.IDEQ(batchID),
			approvalticket.ParentTicketIDIsNil(),
		).
		Only(ctx)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}

	parentEvent, err := s.client.DomainEvent.Get(ctx, parent.EventID)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}
	projection, err := s.client.BatchApprovalTicket.Get(ctx, parent.ID)
	if err != nil && !ent.IsNotFound(err) {
		return generated.VMBatchStatusResponse{}, nil, err
	}

	operation := generated.VMBatchOperationCREATE
	switch domain.EventType(parentEvent.EventType) {
	case domain.EventBatchDeleteRequested:
		operation = generated.VMBatchOperationDELETE
	case domain.EventBatchCreateRequested:
		operation = generated.VMBatchOperationCREATE
	case domain.EventBatchPowerRequested:
		operation = generated.VMBatchOperationPOWER
	default:
		return generated.VMBatchStatusResponse{}, nil, errBatchNotFound
	}

	children, err := s.client.ApprovalTicket.Query().
		Where(approvalticket.ParentTicketIDEQ(parent.ID)).
		Order(ent.Asc(approvalticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}

	eventIDs := make([]string, 0, len(children))
	for _, child := range children {
		eventIDs = append(eventIDs, child.EventID)
	}
	eventByID := map[string]*ent.DomainEvent{}
	if len(eventIDs) > 0 {
		events, err := s.client.DomainEvent.Query().Where(domainevent.IDIn(eventIDs...)).All(ctx)
		if err != nil {
			return generated.VMBatchStatusResponse{}, nil, err
		}
		for _, ev := range events {
			eventByID[ev.ID] = ev
		}
	}

	var (
		successCount int
		failedCount  int
		pendingCount int
		cancelled    int
		pendingOnly  int
		executing    int
	)
	childStatuses := make([]generated.VMBatchChildStatus, 0, len(children))
	for _, child := range children {
		switch child.Status {
		case approvalticket.StatusSUCCESS:
			successCount++
		case approvalticket.StatusFAILED, approvalticket.StatusREJECTED:
			failedCount++
		case approvalticket.StatusCANCELLED:
			cancelled++
		case approvalticket.StatusPENDING:
			pendingCount++
			pendingOnly++
		default:
			pendingCount++
			executing++
		}

		resourceID := ""
		resourceName := ""
		lastError := strings.TrimSpace(child.RejectReason)
		if ev := eventByID[child.EventID]; ev != nil {
			resourceID = strings.TrimSpace(ev.AggregateID)
			switch domain.EventType(ev.EventType) {
			case domain.EventVMDeletionRequested:
				var payload domain.VMDeletePayload
				if err := json.Unmarshal(ev.Payload, &payload); err == nil {
					if strings.TrimSpace(payload.VMName) != "" {
						resourceName = payload.VMName
					}
				}
			case domain.EventVMCreationRequested:
				var payload domain.VMCreationPayload
				if err := json.Unmarshal(ev.Payload, &payload); err == nil {
					if resourceID == "" {
						resourceID = strings.TrimSpace(payload.ServiceID)
					}
				}
			}
		}

		attemptCount := 0
		if child.Status != approvalticket.StatusPENDING {
			attemptCount = 1
		}

		childStatuses = append(childStatuses, generated.VMBatchChildStatus{
			TicketId:     child.ID,
			EventId:      child.EventID,
			Status:       generated.VMBatchChildStatusStatus(child.Status),
			ResourceId:   resourceID,
			ResourceName: resourceName,
			LastError:    lastError,
			AttemptCount: attemptCount,
		})
	}

	status := aggregateBatchParentStatus(len(children), successCount, failedCount, pendingCount, pendingOnly, executing, cancelled)
	projectionStatus := mapProjectionStatus(status)
	if projection == nil {
		createBuilder := s.client.BatchApprovalTicket.Create().
			SetID(parent.ID).
			SetBatchType(toBatchProjectionType(string(operation))).
			SetChildCount(len(children)).
			SetSuccessCount(successCount).
			SetFailedCount(failedCount).
			SetPendingCount(pendingCount).
			SetStatus(projectionStatus).
			SetCreatedBy(parent.Requester).
			SetReason(parent.Reason)
		if _, err := createBuilder.Save(ctx); err != nil && !ent.IsConstraintError(err) {
			logger.Warn("failed to backfill batch projection row", zap.String("batch_id", parent.ID), zap.Error(err))
		}
	} else {
		_, err = s.client.BatchApprovalTicket.UpdateOneID(parent.ID).
			SetChildCount(len(children)).
			SetSuccessCount(successCount).
			SetFailedCount(failedCount).
			SetPendingCount(pendingCount).
			SetStatus(projectionStatus).
			Save(ctx)
		if err != nil {
			logger.Warn("failed to sync batch projection counters", zap.String("batch_id", parent.ID), zap.Error(err))
		}
	}

	response := generated.VMBatchStatusResponse{
		BatchId:      parent.ID,
		Operation:    operation,
		Status:       status,
		ChildCount:   len(children),
		SuccessCount: successCount,
		FailedCount:  failedCount,
		PendingCount: pendingCount,
		Children:     childStatuses,
		CreatedBy:    parent.Requester,
		CreatedAt:    parent.CreatedAt,
		UpdatedAt:    parent.UpdatedAt,
	}
	return response, children, nil
}

func aggregateBatchParentStatus(
	total int,
	successCount int,
	failedCount int,
	pendingCount int,
	pendingOnly int,
	executingCount int,
	cancelledCount int,
) generated.VMBatchParentStatus {
	if total == 0 {
		return generated.VMBatchParentStatusFAILED
	}
	if cancelledCount == total {
		return generated.VMBatchParentStatusCANCELLED
	}
	if successCount == total {
		return generated.VMBatchParentStatusCOMPLETED
	}
	if failedCount+cancelledCount == total {
		return generated.VMBatchParentStatusFAILED
	}
	if pendingOnly == total {
		return generated.VMBatchParentStatusPENDINGAPPROVAL
	}
	if pendingCount > 0 || executingCount > 0 {
		return generated.VMBatchParentStatusINPROGRESS
	}
	if successCount > 0 && failedCount+cancelledCount > 0 {
		return generated.VMBatchParentStatusPARTIALSUCCESS
	}
	return generated.VMBatchParentStatusINPROGRESS
}

func mapProjectionStatus(status generated.VMBatchParentStatus) batchapprovalticket.Status {
	switch status {
	case generated.VMBatchParentStatusPENDINGAPPROVAL:
		return batchapprovalticket.StatusPENDING_APPROVAL
	case generated.VMBatchParentStatusINPROGRESS:
		return batchapprovalticket.StatusIN_PROGRESS
	case generated.VMBatchParentStatusCOMPLETED:
		return batchapprovalticket.StatusCOMPLETED
	case generated.VMBatchParentStatusPARTIALSUCCESS:
		return batchapprovalticket.StatusPARTIAL_SUCCESS
	case generated.VMBatchParentStatusCANCELLED:
		return batchapprovalticket.StatusCANCELLED
	default:
		return batchapprovalticket.StatusFAILED
	}
}

func toBatchProjectionType(op string) batchapprovalticket.BatchType {
	switch strings.TrimSpace(strings.ToUpper(op)) {
	case string(generated.VMBatchOperationDELETE):
		return batchapprovalticket.BatchTypeBATCH_DELETE
	case string(generated.VMBatchOperationPOWER), "POWER_START", "POWER_STOP", "POWER_RESTART":
		return batchapprovalticket.BatchTypeBATCH_POWER
	default:
		return batchapprovalticket.BatchTypeBATCH_CREATE
	}
}

func nillableTrimmed(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func buildBatchPayloadItems(op string, items []generated.VMBatchChildItem) []domain.BatchVMItemPayload {
	out := make([]domain.BatchVMItemPayload, 0, len(items))
	for _, item := range items {
		payloadItem := domain.BatchVMItemPayload{
			VMID:           strings.TrimSpace(item.VmId),
			ServiceID:      strings.TrimSpace(item.ServiceId.String()),
			TemplateID:     strings.TrimSpace(item.TemplateId.String()),
			InstanceSizeID: strings.TrimSpace(item.InstanceSizeId.String()),
			Namespace:      strings.TrimSpace(item.Namespace),
			Reason:         strings.TrimSpace(item.Reason),
		}
		if op == string(generated.VMBatchOperationDELETE) {
			payloadItem.ServiceID = ""
			payloadItem.TemplateID = ""
			payloadItem.InstanceSizeID = ""
			payloadItem.Namespace = ""
		} else {
			payloadItem.VMID = ""
		}
		out = append(out, payloadItem)
	}
	return out
}

func buildBatchPowerPayloadItems(items []generated.VMBatchPowerItem) []domain.BatchVMItemPayload {
	out := make([]domain.BatchVMItemPayload, 0, len(items))
	for _, item := range items {
		out = append(out, domain.BatchVMItemPayload{
			VMID:   strings.TrimSpace(item.VmId),
			Reason: strings.TrimSpace(item.Reason),
		})
	}
	return out
}

func isZeroUUID(id openapi_types.UUID) bool {
	s := strings.TrimSpace(id.String())
	return s == "" || s == "00000000-0000-0000-0000-000000000000"
}

func generateIDV7() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return id.String()
}
