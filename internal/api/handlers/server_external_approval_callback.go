package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	apivalidator "kv-shepherd.io/shepherd/internal/api/validator"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	approvalwebhook "kv-shepherd.io/shepherd/internal/governance/approval/webhook"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

const (
	maxExternalApprovalCallbackBodyBytes int64 = 64 * 1024
	externalApprovalSignatureTolerance         = 5 * time.Minute
	externalApprovalTimestampHeader            = "X-Shepherd-Timestamp"
)

// ListExternalApprovalPendingTickets handles GET /external-approval/pending.
func (s *Server) ListExternalApprovalPendingTickets(c *gin.Context, params generated.ListExternalApprovalPendingTicketsParams) {
	if s.client == nil || s.externalApprovalRegistry == nil {
		logger.Error("external approval polling dependencies are not configured")
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !s.verifyExternalApprovalPollingSignature(c, strings.TrimSpace(params.XExternalApprovalSystemID), strings.TrimSpace(params.XSignature256)) {
		return
	}

	s.writeTicketListResponse(c, "", ticketListOptions{
		page:    params.Page,
		perPage: params.PerPage,
		status:  string(entticket.StatusPENDING),
	})
}

// ReceiveExternalApprovalDecision handles POST /webhooks/approval-callback.
func (s *Server) ReceiveExternalApprovalDecision(c *gin.Context, params generated.ReceiveExternalApprovalDecisionParams) {
	s.receiveExternalApprovalDecision(
		c,
		strings.TrimSpace(params.XExternalApprovalSystemID),
		strings.TrimSpace(params.XSignature256),
		strings.TrimSpace(params.XTicketID),
		"",
	)
}

// ReceiveExternalApprovalTicketDecision handles POST /external-approval/tickets/{ticket_id}/decision.
func (s *Server) ReceiveExternalApprovalTicketDecision(c *gin.Context, ticketID generated.TicketID, params generated.ReceiveExternalApprovalTicketDecisionParams) {
	s.receiveExternalApprovalDecision(
		c,
		strings.TrimSpace(params.XExternalApprovalSystemID),
		strings.TrimSpace(params.XSignature256),
		"",
		strings.TrimSpace(ticketID),
	)
}

func (s *Server) receiveExternalApprovalDecision(c *gin.Context, systemID, signature, headerTicketID, pathTicketID string) {
	ctx := c.Request.Context()
	if s.externalApprovalRegistry == nil || s.approvalRouter == nil {
		logger.Error("external approval callback dependencies are not configured")
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

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
			zap.String("ticket_id", firstNonEmptyString(headerTicketID, pathTicketID)),
		)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	req, ok := decodeExternalApprovalDecisionRequest(c, rawBody)
	if !ok {
		return
	}
	ticketID, approver, rejectReason, ok := validateExternalApprovalDecisionRequest(c, req, headerTicketID, pathTicketID)
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

func (s *Server) verifyExternalApprovalPollingSignature(c *gin.Context, systemID, signature string) bool {
	if systemID == "" || signature == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return false
	}
	timestampHeader := strings.TrimSpace(c.GetHeader(externalApprovalTimestampHeader))
	if timestampHeader == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, timestampHeader)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return false
	}
	age := time.Since(timestamp)
	if age > externalApprovalSignatureTolerance || age < -externalApprovalSignatureTolerance {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return false
	}

	_, signingKey, err := s.externalApprovalRegistry.CallbackSigningKey(c.Request.Context(), systemID)
	if err != nil {
		writeExternalApprovalCallbackRegistryError(c, err, systemID)
		return false
	}
	payload := externalApprovalPollingSignaturePayload(c, timestampHeader)
	if !approvalwebhook.VerifySignature(payload, []byte(signingKey), signature) {
		logger.Warn("external approval polling signature verification failed",
			zap.String("external_approval_system_id", systemID),
		)
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return false
	}
	return true
}

func externalApprovalPollingSignaturePayload(c *gin.Context, timestampHeader string) []byte {
	method := ""
	path := ""
	rawQuery := ""
	if c != nil && c.Request != nil {
		method = c.Request.Method
		if c.Request.URL != nil {
			path = c.Request.URL.EscapedPath()
			if path == "" {
				path = c.Request.URL.Path
			}
			rawQuery = c.Request.URL.RawQuery
		}
	}
	return []byte(strings.Join([]string{
		strings.ToUpper(method),
		path,
		rawQuery,
		strings.TrimSpace(timestampHeader),
	}, "\n"))
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
	pathTicketID string,
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
	if pathTicketID != "" && pathTicketID != ticketID {
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
