package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
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

	query := s.client.ApprovalTicket.Query()

	// Filter by status (omitzero: empty string = not specified).
	if params.Status != "" {
		query = query.Where(approvalticket.StatusEQ(approvalticket.Status(params.Status)))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
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
		logger.Error("failed to list approval tickets", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Collect event IDs for DELETE tickets to batch-fetch target VM info.
	deleteEventIDs := make([]string, 0)
	for _, t := range tickets {
		if t.OperationType == approvalticket.OperationTypeDELETE {
			deleteEventIDs = append(deleteEventIDs, t.EventID)
		}
	}

	// Batch-fetch domain events for DELETE tickets and extract VM info from payload.
	vmInfoMap := make(map[string]vmTargetInfo) // key = event_id
	if len(deleteEventIDs) > 0 {
		events, err := s.client.DomainEvent.Query().
			Where(domainevent.IDIn(deleteEventIDs...)).
			All(ctx)
		if err != nil {
			// Non-fatal: log and continue without VM info.
			logger.Warn("failed to fetch domain events for delete tickets", zap.Error(err))
		} else {
			for _, ev := range events {
				var payload struct {
					VMID   string `json:"vm_id"`
					VMName string `json:"vm_name"`
				}
				if err := json.Unmarshal(ev.Payload, &payload); err == nil {
					vmInfoMap[ev.ID] = vmTargetInfo{
						VMID:   payload.VMID,
						VMName: payload.VMName,
					}
				}
			}
		}
	}

	items := make([]generated.ApprovalTicket, 0, len(tickets))
	for _, t := range tickets {
		item := ticketToAPI(t)
		// Enrich DELETE tickets with target VM info.
		if info, ok := vmInfoMap[t.EventID]; ok {
			item.TargetVmId = info.VMID
			item.TargetVmName = info.VMName
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
func (s *Server) ApproveTicket(c *gin.Context, ticketId generated.TicketID) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.ApprovalDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	if err := s.gateway.Approve(ctx, ticketId, actor, req.SelectedClusterId, req.SelectedStorageClass); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		logger.Error("ticket approval failed",
			zap.Error(err),
			zap.String("ticket_id", ticketId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "APPROVAL_FAILED"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RejectTicket handles POST /approvals/{ticket_id}/reject.
func (s *Server) RejectTicket(c *gin.Context, ticketId generated.TicketID) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.RejectDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	if err := s.gateway.Reject(ctx, ticketId, actor, req.Reason); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		logger.Error("ticket rejection failed",
			zap.Error(err),
			zap.String("ticket_id", ticketId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "REJECT_FAILED"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ---- Converter ----

func ticketToAPI(t *ent.ApprovalTicket) generated.ApprovalTicket {
	return generated.ApprovalTicket{
		Id:            t.ID,
		EventId:       t.EventID,
		OperationType: generated.ApprovalTicketOperationType(t.OperationType),
		Requester:     t.Requester,
		Status:        generated.ApprovalTicketStatus(t.Status),
		Approver:      t.Approver,
		Reason:        t.Reason,
		RejectReason:  t.RejectReason,
		CreatedAt:     t.CreatedAt,
	}
}
