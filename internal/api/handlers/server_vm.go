package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ListVMs handles GET /vms.
func (s *Server) ListVMs(c *gin.Context, params generated.ListVMsParams) {
	ctx := c.Request.Context()

	query := s.client.VM.Query()

	// Filter by status.
	if params.Status != "" {
		query = query.Where(entvm.StatusEQ(entvm.Status(params.Status)))
	}
	// Filter by namespace.
	if params.Namespace != "" {
		query = query.Where(entvm.NamespaceEQ(params.Namespace))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count VMs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	vms, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entvm.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list VMs", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.VM, 0, len(vms))
	for _, vm := range vms {
		items = append(items, vmToAPI(vm))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.VMList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateVMRequest handles POST /vms/request (requires approval).
func (s *Server) CreateVMRequest(c *gin.Context) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	output, err := s.createVMUC.Execute(ctx, usecase.CreateVMInput{
		ServiceID:      req.ServiceId.String(),
		TemplateID:     req.TemplateId.String(),
		InstanceSizeID: req.InstanceSizeId.String(),
		Namespace:      req.Namespace,
		Reason:         req.Reason,
		RequestedBy:    actor,
	})
	if err != nil {
		logger.Error("VM request failed",
			zap.Error(err),
			zap.String("actor", actor),
			zap.String("namespace", req.Namespace),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "VM_REQUEST_FAILED"})
		return
	}

	// Notification trigger: APPROVAL_PENDING → notify approvers (master-flow.md Stage 5.F).
	if s.notifier != nil {
		s.notifier.OnTicketSubmitted(ctx, output.TicketID, actor, req.Namespace)
	}

	c.JSON(http.StatusAccepted, generated.ApprovalTicketResponse{
		TicketId: output.TicketID,
		Status:   generated.ApprovalTicketResponseStatusPENDING,
	})
}

// GetVM handles GET /vms/{vm_id}.
func (s *Server) GetVM(c *gin.Context, vmId generated.VMID) {
	ctx := c.Request.Context()

	vm, err := s.client.VM.Get(ctx, vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, vmToAPI(vm))
}

// DeleteVM handles DELETE /vms/{vm_id}.
// ADR-0015 §5.D: VM deletion requires approval ticket.
// Flow: confirmation gate → create DomainEvent + ApprovalTicket (operation_type=DELETE) → return 202.
// Admin approval triggers River job execution via Gateway.approveDelete.
func (s *Server) DeleteVM(c *gin.Context, vmId generated.VMID, params generated.DeleteVMParams) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)

	// Build use case input from params.
	input := usecase.DeleteVMInput{
		VMID:        vmId,
		RequestedBy: actor,
		Confirm:     params.Confirm,
		ConfirmName: params.ConfirmName,
	}

	result, err := s.deleteVMUC.Execute(ctx, input)
	if err != nil {
		// Use apperrors.IsAppError to extract structured error info.
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
			})
			return
		}
		// Fallback for non-AppError errors.
		logger.Error("VM delete request failed",
			zap.Error(err),
			zap.String("vm_id", vmId),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	_ = result.Status // Keep use case output field for backward compatibility.

	// Notification trigger: APPROVAL_PENDING → notify approvers for delete request.
	if s.notifier != nil {
		s.notifier.OnTicketSubmitted(ctx, result.TicketID, actor, "")
	}

	c.JSON(http.StatusAccepted, generated.DeleteVMResponse{
		TicketId: result.TicketID,
		EventId:  result.EventID,
		Status:   generated.DeleteVMResponseStatusPENDING,
	})
}

// StartVM handles POST /vms/{vm_id}/start.
// ISSUE-001: Async via River (ADR-0006). Returns 202 Accepted.
func (s *Server) StartVM(c *gin.Context, vmId generated.VMID) {
	vm, err := s.client.VM.Get(c.Request.Context(), vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for start", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// State guard: only STOPPED or PAUSED VMs can be started.
	if vm.Status != entvm.StatusSTOPPED && vm.Status != entvm.StatusPAUSED {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot start VM in %s state, must be STOPPED or PAUSED", vm.Status),
		})
		return
	}

	s.enqueueVMPowerOp(c, vm, "start", domain.EventVMStartRequested)
}

// StopVM handles POST /vms/{vm_id}/stop.
// ISSUE-001: Async via River (ADR-0006). Returns 202 Accepted.
func (s *Server) StopVM(c *gin.Context, vmId generated.VMID) {
	vm, err := s.client.VM.Get(c.Request.Context(), vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for stop", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if vm.Status != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot stop VM in %s state, must be RUNNING", vm.Status),
		})
		return
	}

	s.enqueueVMPowerOp(c, vm, "stop", domain.EventVMStopRequested)
}

// RestartVM handles POST /vms/{vm_id}/restart.
// ISSUE-001: Async via River (ADR-0006). Returns 202 Accepted.
func (s *Server) RestartVM(c *gin.Context, vmId generated.VMID) {
	vm, err := s.client.VM.Get(c.Request.Context(), vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for restart", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if vm.Status != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot restart VM in %s state, must be RUNNING", vm.Status),
		})
		return
	}

	s.enqueueVMPowerOp(c, vm, "restart", domain.EventVMRestartRequested)
}

// enqueueVMPowerOp creates a DomainEvent, enqueues a River job, and returns 202 Accepted.
// Shared by StartVM, StopVM, RestartVM to reduce duplication.
func (s *Server) enqueueVMPowerOp(c *gin.Context, vm *ent.VM, operation string, eventType domain.EventType) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)

	payload := domain.VMPowerPayload{
		VMID:      vm.ID,
		VMName:    vm.Name,
		ClusterID: vm.ClusterID,
		Namespace: vm.Namespace,
		Operation: operation,
		Actor:     actor,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal power payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	eventID, _ := uuid.NewV7()
	_, err = s.client.DomainEvent.Create().
		SetID(eventID.String()).
		SetEventType(string(eventType)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		logger.Error("failed to create power domain event", zap.Error(err), zap.String("vm_id", vm.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Enqueue River job (ADR-0006).
	if _, err := s.riverClient.Insert(ctx, jobs.VMPowerArgs{
		EventID:   eventID.String(),
		Operation: operation,
	}, nil); err != nil {
		logger.Error("failed to enqueue VM power job", zap.Error(err), zap.String("event_id", eventID.String()))
		_, _ = s.client.DomainEvent.UpdateOneID(eventID.String()).SetStatus(domainevent.StatusFAILED).Save(ctx)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm."+operation+"_requested", "vm", vm.ID, actor, nil)
	}

	c.JSON(http.StatusAccepted, gin.H{"event_id": eventID.String(), "status": "ACCEPTED"})
}

// ---- Converter ----

func vmToAPI(vm *ent.VM) generated.VM {
	return generated.VM{
		Id:        vm.ID,
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Status:    generated.VMStatus(vm.Status),
		ClusterId: vm.ClusterID,
		Hostname:  vm.Hostname,
		Instance:  vm.Instance,
		// ServiceId: not directly available (FK edge), omitted if not eagerly loaded
		TicketId:  vm.TicketID,
		CreatedBy: vm.CreatedBy,
		CreatedAt: vm.CreatedAt,
	}
}
