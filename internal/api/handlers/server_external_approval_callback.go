package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	apivalidator "kv-shepherd.io/shepherd/internal/api/validator"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	approvalwebhook "kv-shepherd.io/shepherd/internal/governance/approval/webhook"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

const maxExternalApprovalCallbackBodyBytes int64 = 64 * 1024

// ReceiveExternalApprovalDecision handles POST /webhooks/approval-callback.
func (s *Server) ReceiveExternalApprovalDecision(c *gin.Context, params generated.ReceiveExternalApprovalDecisionParams) {
	ctx := c.Request.Context()
	if s.externalApprovalRegistry == nil || s.approvalRouter == nil {
		logger.Error("external approval callback dependencies are not configured")
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	systemID := strings.TrimSpace(params.XExternalApprovalSystemID)
	signature := strings.TrimSpace(params.XSignature256)
	headerTicketID := strings.TrimSpace(params.XTicketID)
	if systemID == "" || signature == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	rawBody, ok := readExternalApprovalCallbackBody(c)
	if !ok {
		return
	}

	_, signingKey, err := s.externalApprovalRegistry.CallbackSigningKey(ctx, systemID)
	if err != nil {
		writeExternalApprovalCallbackRegistryError(c, err, systemID)
		return
	}
	if !approvalwebhook.VerifySignature(rawBody, []byte(signingKey), signature) {
		logger.Warn("external approval callback signature verification failed",
			zap.String("external_approval_system_id", systemID),
			zap.String("ticket_id", headerTicketID),
		)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	req, ok := decodeExternalApprovalDecisionRequest(c, rawBody)
	if !ok {
		return
	}
	ticketID, approver, rejectReason, ok := validateExternalApprovalDecisionRequest(c, req, headerTicketID)
	if !ok {
		return
	}

	decision := approvalcontract.ApprovalDecision{
		Approved:     req.Approved,
		Approver:     approver,
		RejectReason: rejectReason,
		Execution:    approvalDecisionRequestToExecutionOptions(req.Execution),
	}
	if err := s.approvalRouter.ProcessApproval(ctx, ticketID, decision); err != nil {
		writeExternalApprovalDecisionError(c, err, ticketID, systemID, approver)
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "external_approval.callback.accepted", "ticket", ticketID, approver, map[string]interface{}{
			"external_approval_system_id": systemID,
			"provider_decision_id":        strings.TrimSpace(req.ProviderDecisionId),
			"approved":                    req.Approved,
		})
	}

	c.JSON(http.StatusOK, generated.ExternalApprovalDecisionResponse{
		Approved: req.Approved,
		Status:   generated.Accepted,
		TicketId: ticketID,
	})
}

func readExternalApprovalCallbackBody(c *gin.Context) ([]byte, bool) {
	if c.Request.Body == nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxExternalApprovalCallbackBodyBytes+1))
	if err != nil {
		logger.Warn("failed to read external approval callback body", zap.Error(err))
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return nil, false
	}
	if int64(len(body)) > maxExternalApprovalCallbackBodyBytes {
		c.JSON(http.StatusRequestEntityTooLarge, generated.Error{
			Code:    "REQUEST_TOO_LARGE",
			Message: "request body is too large",
		})
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return nil, false
	}
	return body, true
}

func decodeExternalApprovalDecisionRequest(c *gin.Context, rawBody []byte) (generated.ExternalApprovalDecisionRequest, bool) {
	if !jsonObjectHasField(rawBody, "approved") {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return generated.ExternalApprovalDecisionRequest{}, false
	}

	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()

	var req generated.ExternalApprovalDecisionRequest
	if err := decoder.Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return generated.ExternalApprovalDecisionRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return generated.ExternalApprovalDecisionRequest{}, false
	}
	if err := apivalidator.ValidateStruct(&req); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, toGeneratedError(appErr))
			return generated.ExternalApprovalDecisionRequest{}, false
		}
		logger.Error("external approval callback validation failed unexpectedly", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return generated.ExternalApprovalDecisionRequest{}, false
	}
	return req, true
}

func jsonObjectHasField(rawBody []byte, field string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &fields); err != nil {
		return false
	}
	_, ok := fields[field]
	return ok
}

func validateExternalApprovalDecisionRequest(
	c *gin.Context,
	req generated.ExternalApprovalDecisionRequest,
	headerTicketID string,
) (ticketID, approver, rejectReason string, ok bool) {
	ticketID = strings.TrimSpace(req.TicketId)
	approver = strings.TrimSpace(req.Approver)
	rejectReason = strings.TrimSpace(req.RejectReason)
	if ticketID == "" || approver == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return "", "", "", false
	}
	if headerTicketID != "" && headerTicketID != ticketID {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "TICKET_ID_MISMATCH"})
		return "", "", "", false
	}
	if !req.Approved && rejectReason == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "REJECT_REASON_REQUIRED"})
		return "", "", "", false
	}
	return ticketID, approver, rejectReason, true
}

func writeExternalApprovalCallbackRegistryError(c *gin.Context, err error, systemID string) {
	switch {
	case approvalregistry.IsValidationError(err):
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
	case errors.Is(err, approvalregistry.ErrCallbackSigningKeyUnavailable):
		logger.Warn("external approval callback signing key is unavailable",
			zap.String("external_approval_system_id", systemID),
		)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
	default:
		logger.Error("external approval callback registry lookup failed",
			zap.String("external_approval_system_id", systemID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
	}
}

func writeExternalApprovalDecisionError(c *gin.Context, err error, ticketID, systemID, approver string) {
	if appErr, ok := apperrors.IsAppError(err); ok {
		c.JSON(appErr.HTTPStatus, generated.Error{
			Code:        appErr.Code,
			Message:     appErr.Message,
			Params:      appErr.Params,
			FieldErrors: appFieldErrorsToAPI(appErr.FieldErrors),
		})
		return
	}
	logger.Error("external approval decision failed",
		zap.Error(err),
		zap.String("ticket_id", ticketID),
		zap.String("external_approval_system_id", systemID),
		zap.String("approver", approver),
	)
	c.JSON(http.StatusBadRequest, generated.Error{Code: "EXTERNAL_APPROVAL_DECISION_FAILED"})
}
