package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ListVMs handles GET /vms.
func (s *Server) ListVMs(c *gin.Context, params generated.ListVMsParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:read") {
		return
	}

	query := s.client.VM.Query()
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.restricted {
		visibleNamespaces, err := s.listVisibleNamespaceNames(ctx, visibility)
		if err != nil {
			logger.Error("failed to load visible namespaces", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if len(visibleNamespaces) == 0 {
			page, perPage := defaultPagination(params.Page, params.PerPage)
			c.JSON(http.StatusOK, generated.VMList{
				Items: []generated.VM{},
				Pagination: generated.Pagination{
					Page:       page,
					PerPage:    perPage,
					Total:      0,
					TotalPages: 0,
				},
			})
			return
		}
		query = query.Where(entvm.NamespaceIn(visibleNamespaces...))
	}

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

	// Batch-fetch cluster environments to fill VM.environment (ADR-0015 §15).
	// Collect unique non-empty cluster IDs from this page.
	clusterIDs := make([]string, 0)
	clusterIDSet := make(map[string]struct{})
	for _, vm := range vms {
		if vm.ClusterID != "" {
			if _, seen := clusterIDSet[vm.ClusterID]; !seen {
				clusterIDSet[vm.ClusterID] = struct{}{}
				clusterIDs = append(clusterIDs, vm.ClusterID)
			}
		}
	}
	clusterEnvMap := make(map[string]string) // cluster_id → environment string
	if len(clusterIDs) > 0 {
		clusters, clusterErr := s.client.Cluster.Query().
			Where(entcluster.IDIn(clusterIDs...)).
			Select(entcluster.FieldID, entcluster.FieldEnvironment).
			All(ctx)
		if clusterErr != nil {
			// Non-fatal: log and continue without environment info.
			logger.Warn("failed to fetch cluster environments for VM list", zap.Error(clusterErr))
		} else {
			for _, cl := range clusters {
				clusterEnvMap[cl.ID] = string(cl.Environment)
			}
		}
	}

	items := make([]generated.VM, 0, len(vms))
	for _, vm := range vms {
		env := clusterEnvMap[vm.ClusterID]
		items = append(items, vmToAPI(vm, env))
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

// GetVMRequestContext handles GET /vms/request-context.
// Returns user-visible wizard context to avoid client-side fan-out and drift.
func (s *Server) GetVMRequestContext(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:create") {
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM request context namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	namespaceQuery := s.client.NamespaceRegistry.Query().
		Where(namespaceregistry.EnabledEQ(true))
	if visibility.restricted {
		if len(visibility.envs) == 0 {
			namespaceQuery = nil
		} else {
			namespaceQuery = namespaceQuery.Where(namespaceregistry.EnvironmentIn(visibility.envs...))
		}
	}

	namespaces := make([]string, 0)
	if namespaceQuery != nil {
		namespaces, err = namespaceQuery.
			Order(ent.Asc(namespaceregistry.FieldName)).
			Select(namespaceregistry.FieldName).
			Strings(ctx)
		if err != nil {
			logger.Error("failed to list request-context namespaces", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	templates, err := s.client.Template.Query().
		Where(enttemplate.EnabledEQ(true)).
		Order(ent.Asc(enttemplate.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list request-context templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	sizes, err := s.client.InstanceSize.Query().
		Where(instancesize.EnabledEQ(true)).
		Order(ent.Asc(instancesize.FieldSortOrder)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list request-context instance sizes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	templateItems := make([]generated.Template, 0, len(templates))
	for _, t := range templates {
		templateItems = append(templateItems, templateToAPI(t))
	}

	sizeItems := make([]generated.InstanceSize, 0, len(sizes))
	for _, sz := range sizes {
		sizeItems = append(sizeItems, instanceSizeToAPI(sz))
	}

	c.JSON(http.StatusOK, generated.VMRequestContext{
		Namespaces:    namespaces,
		Templates:     templateItems,
		InstanceSizes: sizeItems,
	})
}

// CreateVMRequest handles POST /vms/request (requires approval).
func (s *Server) CreateVMRequest(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:create") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM request namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	visible, err := s.isNamespaceVisible(ctx, req.Namespace, visibility)
	if err != nil {
		logger.Error("failed to check namespace visibility for VM request", zap.Error(err), zap.String("namespace", req.Namespace))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !visible {
		c.JSON(http.StatusForbidden, generated.Error{
			Code:    "NAMESPACE_ENV_FORBIDDEN",
			Message: "namespace is not visible under current environment permissions",
		})
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
		if appErr, ok := apperrors.IsAppError(err); ok {
			// Keep endpoint contract-compatible with current OpenAPI (400 on request failure),
			// while preserving machine-readable code/params.
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    appErr.Code,
				Message: appErr.Message,
				Params:  appErr.Params,
			})
			return
		}
		logger.Error("VM request failed",
			zap.Error(err),
			zap.String("actor", actor),
			zap.String("namespace", req.Namespace),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "VM_REQUEST_FAILED"})
		return
	}

	// Stage 2.E: Route the submission through the approval provider router.
	// V1: router always selects the built-in provider (no-op other than logging).
	// V2+: an external provider adapter can delegate to a third-party system here.
	// Non-fatal: ticket is already PENDING in DB; log the error but do not unwind the user response.
	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &provider.ApprovalRequest{
			EventID:   output.TicketID, // ticket_id echoed as event_id for provider correlation
			Requester: actor,
			Action:    "create",
			Reason:    req.Reason,
		}); routerErr != nil {
			logger.Warn("approval router SubmitForApproval failed (ticket already PENDING in DB)",
				zap.String("ticket_id", output.TicketID),
				zap.Error(routerErr),
			)
		}
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
	if !requireGlobalPermission(c, "vm:read") {
		return
	}

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
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	visible, err := s.isNamespaceVisible(ctx, vm.Namespace, visibility)
	if err != nil {
		logger.Error("failed to check VM namespace visibility", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !visible {
		c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
		return
	}

	// Fetch cluster environment for this VM (ADR-0015 §15).
	var clusterEnv string
	if vm.ClusterID != "" {
		cl, clErr := s.client.Cluster.Get(ctx, vm.ClusterID)
		if clErr != nil {
			// Non-fatal: log and continue without environment info.
			logger.Warn("failed to fetch cluster environment for VM",
				zap.Error(clErr),
				zap.String("cluster_id", vm.ClusterID),
			)
		} else {
			clusterEnv = string(cl.Environment)
		}
	}

	c.JSON(http.StatusOK, vmToAPI(vm, clusterEnv))
}

// DeleteVM handles DELETE /vms/{vm_id}.
// ADR-0015 §5.D: VM deletion requires approval ticket.
// Flow: confirmation gate → create DomainEvent + ApprovalTicket (operation_type=DELETE) → return 202.
// Admin approval triggers River job execution via Gateway.approveDelete.
func (s *Server) DeleteVM(c *gin.Context, vmId generated.VMID, params generated.DeleteVMParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:delete") {
		return
	}
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
				Params:  appErr.Params,
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

	// Stage 2.E: Route the delete submission through the approval provider router.
	// Same semantics as CreateVMRequest: ticket is already PENDING; router call is best-effort.
	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &provider.ApprovalRequest{
			EventID:   result.TicketID,
			Requester: actor,
			Action:    "delete",
		}); routerErr != nil {
			logger.Warn("approval router SubmitForApproval failed for delete ticket (already PENDING in DB)",
				zap.String("ticket_id", result.TicketID),
				zap.Error(routerErr),
			)
		}
	}

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
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
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
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
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
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
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

// vmToAPI converts an Ent VM entity to the generated API VM type.
// clusterEnv is the environment string ("test" or "prod") from the associated Cluster;
// pass an empty string when the cluster is not yet assigned.
func vmToAPI(vm *ent.VM, clusterEnv string) generated.VM {
	apiVM := generated.VM{
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
	// Populate environment from cluster (ADR-0015 §15).
	if clusterEnv != "" {
		apiVM.Environment = generated.VMEnvironment(clusterEnv)
	}
	return apiVM
}

// PowerVM handles POST /vms/{vm_id}/power.
// Unified power-action endpoint — routes to start/stop/restart via the `action` field.
// Follows oapi-codegen ServerInterface contract (ADR-0021).
func (s *Server) PowerVM(c *gin.Context, vmId generated.VMID) {
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}

	var req generated.VMPowerRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	vm, err := s.client.VM.Get(c.Request.Context(), vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for power action", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Route by action, applying the same state-machine guards as the dedicated endpoints.
	switch req.Action {
	case generated.Start:
		if vm.Status != entvm.StatusSTOPPED && vm.Status != entvm.StatusPAUSED {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot start VM in %s state, must be STOPPED or PAUSED", vm.Status),
			})
			return
		}
		s.enqueueVMPowerOp(c, vm, "start", domain.EventVMStartRequested)
	case generated.Stop:
		if vm.Status != entvm.StatusRUNNING {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot stop VM in %s state, must be RUNNING", vm.Status),
			})
			return
		}
		s.enqueueVMPowerOp(c, vm, "stop", domain.EventVMStopRequested)
	case generated.Restart:
		if vm.Status != entvm.StatusRUNNING {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot restart VM in %s state, must be RUNNING", vm.Status),
			})
			return
		}
		s.enqueueVMPowerOp(c, vm, "restart", domain.EventVMRestartRequested)
	default:
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_POWER_ACTION",
			Message: fmt.Sprintf("unknown power action %q, must be start, stop, or restart", req.Action),
		})
	}
}
