package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/predicate"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// vmTargetInfo holds extracted VM information from a DELETE domain event payload.
type vmTargetInfo struct {
	VMID   string
	VMName string
}

// ListApprovals handles GET /approvals.
func (s *Server) ListApprovals(c *gin.Context, params generated.ListApprovalsParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "approval:view") {
		return
	}

	query := s.client.ApprovalTicket.Query()

	// Filter by status (omitzero: empty string = not specified).
	if params.Status != "" {
		query = query.Where(approvalticket.StatusEQ(approvalticket.Status(params.Status)))
	}
	if params.OperationType != "" {
		query = query.Where(approvalticket.OperationTypeEQ(approvalticket.OperationType(params.OperationType)))
	}
	if params.SelectedClusterId != "" {
		query = query.Where(approvalticket.SelectedClusterIDEQ(params.SelectedClusterId))
	}
	if params.PlacementAdvisoryCode != "" {
		query = query.Where(
			predicate.ApprovalTicket(func(s *entsql.Selector) {
				s.Where(sqljson.ValueEQ(
					approvalticket.FieldPlacementEvaluation,
					params.PlacementAdvisoryCode,
					sqljson.Path("advisory_code"),
				))
			}),
		)
	}
	switch params.PlacementSnapshot {
	case "":
	case generated.Present:
		query = query.Where(approvalticket.PlacementEvaluationNotNil())
	case generated.Missing:
		query = query.Where(approvalticket.PlacementEvaluationIsNil())
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting approval tickets", zap.Error(err))
			return
		}
		logger.Error("failed to count approval tickets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tickets, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Asc(approvalticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing approval tickets", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list approval tickets", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Collect all event IDs to batch-fetch domain events.
	// DELETE tickets: extract target VM info; all tickets: include raw payload.
	allEventIDs := make([]string, 0, len(tickets))
	deleteEventIDSet := make(map[string]struct{})
	createTicketIDs := make([]string, 0)
	for _, t := range tickets {
		allEventIDs = append(allEventIDs, t.EventID)
		if t.OperationType == approvalticket.OperationTypeDELETE {
			deleteEventIDSet[t.EventID] = struct{}{}
		}
		if t.OperationType == approvalticket.OperationTypeCREATE {
			createTicketIDs = append(createTicketIDs, t.ID)
		}
	}

	// Batch-fetch domain events for all tickets.
	vmInfoMap := make(map[string]vmTargetInfo) // keyed by event ID
	eventPayloadMap := make(map[string][]byte) // keyed by event ID; value is raw JSON payload
	if len(allEventIDs) > 0 {
		events, err := s.client.DomainEvent.Query().
			Where(domainevent.IDIn(allEventIDs...)).
			All(ctx)
		if err != nil {
			// Non-fatal: log and continue without event info.
			logger.Warn("failed to fetch domain events for approval tickets", zap.Error(err))
		} else {
			for _, ev := range events {
				// Store raw payload for all tickets.
				eventPayloadMap[ev.ID] = ev.Payload
				// Extract VM target info only for DELETE tickets.
				if _, isDelete := deleteEventIDSet[ev.ID]; !isDelete {
					continue
				}
				var vmPayload struct {
					VMID   string `json:"vm_id"`
					VMName string `json:"vm_name"`
				}
				if err := json.Unmarshal(ev.Payload, &vmPayload); err == nil && vmPayload.VMID != "" {
					vmInfoMap[ev.ID] = vmTargetInfo{
						VMID:   vmPayload.VMID,
						VMName: vmPayload.VMName,
					}
				}
			}
		}
	}

	createVMByTicketID := make(map[string]*ent.VM)
	if len(createTicketIDs) > 0 {
		vms, err := s.client.VM.Query().
			Where(entvm.TicketIDIn(createTicketIDs...)).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch VMs for create approval tickets", zap.Error(err))
		} else {
			for _, vm := range vms {
				if vm == nil || vm.TicketID == "" {
					continue
				}
				createVMByTicketID[vm.TicketID] = vm
			}
		}
	}

	items := make([]generated.ApprovalTicket, 0, len(tickets))
	for _, t := range tickets {
		// Deserialize raw event payload into map for ticket_payload field.
		var payloadMap map[string]interface{}
		if raw, ok := eventPayloadMap[t.EventID]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &payloadMap); err != nil {
				logger.Warn("failed to deserialize ticket payload",
					zap.Error(err),
					zap.String("event_id", t.EventID),
				)
			}
		}
		var provisioning *generated.ProvisioningStatus
		if t.OperationType == approvalticket.OperationTypeCREATE {
			provisioning = s.loadVMProvisioning(ctx, createVMByTicketID[t.ID])
		}
		item := ticketToAPI(t, payloadMap, provisioning)
		// Enrich DELETE tickets with target VM info.
		if t.OperationType == approvalticket.OperationTypeDELETE {
			if info, ok := vmInfoMap[t.EventID]; ok {
				item.TargetVmId = info.VMID
				item.TargetVmName = info.VMName
			}
		}
		items = append(items, item)
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.ApprovalTicketList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ApproveTicket handles POST /approvals/{ticket_id}/approve.
func (s *Server) ApproveTicket(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "approval:approve") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.ApprovalDecisionRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	opts := approval.ApproveOpts{
		ClusterID:       req.SelectedClusterId,
		StorageClass:    req.SelectedStorageClass,
		EnableOverride:  req.EnableOverride,
		CPURequest:      float64(req.CpuRequest),
		CPULimit:        float64(req.CpuLimit),
		MemoryRequestGi: float64(req.MemoryRequestGi),
		MemoryLimitGi:   float64(req.MemoryLimitGi),
		DiskGB:          req.DiskGb,
	}

	if err := s.gateway.Approve(ctx, ticketID, actor, opts); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		logger.Error("ticket approval failed",
			zap.Error(err),
			zap.String("ticket_id", ticketID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "APPROVAL_FAILED"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RejectTicket handles POST /approvals/{ticket_id}/reject.
func (s *Server) RejectTicket(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "approval:approve") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.RejectDecisionRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	if err := s.gateway.Reject(ctx, ticketID, actor, req.Reason); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		logger.Error("ticket rejection failed",
			zap.Error(err),
			zap.String("ticket_id", ticketID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "REJECT_FAILED"})
		return
	}

	c.Status(http.StatusNoContent)
}

// CancelTicket handles POST /approvals/{ticket_id}/cancel.
func (s *Server) CancelTicket(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "approval:approve", "vm:create", "vm:delete", "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	if err := s.gateway.Cancel(ctx, ticketID, actor); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		logger.Error("ticket cancellation failed",
			zap.Error(err),
			zap.String("ticket_id", ticketID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ---- Converter ----

// ticketToAPI converts an Ent ApprovalTicket to the generated API type.
// ticketPayload is the deserialized DomainEvent payload (may be nil for older/missing events).
func ticketToAPI(
	t *ent.ApprovalTicket,
	ticketPayload map[string]interface{},
	provisioning *generated.ProvisioningStatus,
) generated.ApprovalTicket {
	var placementEvaluation *generated.PlacementEvaluation
	if len(t.PlacementEvaluation) > 0 {
		placementEvaluation = placementEvaluationToAPI(t.PlacementEvaluation)
	}
	return generated.ApprovalTicket{
		Id:                  t.ID,
		EventId:             t.EventID,
		OperationType:       generated.ApprovalTicketOperationType(t.OperationType),
		Requester:           t.Requester,
		Status:              generated.ApprovalTicketStatus(t.Status),
		Approver:            t.Approver,
		Reason:              t.Reason,
		RejectReason:        t.RejectReason,
		TicketPayload:       ticketPayload,
		Provisioning:        provisioning,
		PlacementEvaluation: placementEvaluation,
		CreatedAt:           t.CreatedAt,
	}
}

func placementEvaluationToAPI(snapshot map[string]interface{}) *generated.PlacementEvaluation {
	if len(snapshot) == 0 {
		return nil
	}
	result := &generated.PlacementEvaluation{}
	if value, ok := snapshot["selected_cluster_id"].(string); ok {
		result.SelectedClusterId = value
	}
	if value, ok := snapshot["selected_cluster_name"].(string); ok {
		result.SelectedClusterName = value
	}
	if value, ok := snapshot["selected_cluster_environment"].(string); ok {
		result.SelectedClusterEnvironment = generated.PlacementEvaluationSelectedClusterEnvironment(value)
	}
	if value, ok := snapshot["requested_storage_class"].(string); ok {
		result.RequestedStorageClass = value
	}
	if value, ok := snapshot["effective_storage_class"].(string); ok {
		result.EffectiveStorageClass = value
	}
	if value, ok := snapshot["eligible"].(bool); ok {
		result.Eligible = value
	}
	if value, ok := snapshot["reason_code"].(string); ok {
		result.ReasonCode = value
	}
	if value, ok := snapshot["reason_message"].(string); ok {
		result.ReasonMessage = value
	}
	if value, ok := snapshot["advisory_code"].(string); ok {
		result.AdvisoryCode = value
	}
	if value, ok := snapshot["advisory_message"].(string); ok {
		result.AdvisoryMessage = value
	}
	if value, ok := snapshot["override"].(map[string]interface{}); ok {
		result.Override = value
	}
	if value, ok := snapshot["evaluated_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			result.EvaluatedAt = parsed
		}
	}
	return result
}
