package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

type vmModifyContextUnavailableError struct {
	CPUReason    string
	MemoryReason string
	DiskReason   string
}

func newVMModifyContextUnavailableError(reason string) *vmModifyContextUnavailableError {
	return &vmModifyContextUnavailableError{
		CPUReason:    reason,
		MemoryReason: reason,
		DiskReason:   reason,
	}
}

func (e *vmModifyContextUnavailableError) Error() string {
	if e == nil {
		return "live VM modify context is unavailable"
	}
	return firstNonEmptyString(e.CPUReason, e.MemoryReason, e.DiskReason, "live VM modify context is unavailable")
}

func (e *vmModifyContextUnavailableError) Apply(resp *generated.VMModifyContext) {
	if e == nil || resp == nil {
		return
	}
	resp.CpuReason = firstNonEmptyString(e.CPUReason, resp.CpuReason)
	resp.MemoryReason = firstNonEmptyString(e.MemoryReason, resp.MemoryReason)
	resp.DiskReason = firstNonEmptyString(e.DiskReason, resp.DiskReason)
}

func (s *Server) GetVMModifyContext(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}

	vmRow, ok := s.loadVisibleVMForModify(ctx, c, vmID)
	if !ok {
		return
	}

	resp, _, _, err := s.resolveVMModifyContext(ctx, vmRow)
	if err != nil {
		var unavailableErr *vmModifyContextUnavailableError
		if errors.As(err, &unavailableErr) {
			unavailableErr.Apply(&resp)
			c.JSON(http.StatusOK, resp)
			return
		}
		logger.Error("failed to resolve vm modify context", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) CreateVMModifyRequest(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
	actor := strings.TrimSpace(middleware.GetUserID(ctx))
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMModifyRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	vmRow, ok := s.loadVisibleVMForModify(ctx, c, vmID)
	if !ok {
		return
	}

	payload, err := s.buildVMModifyPayload(ctx, vmRow, actor, req)
	if err != nil {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "VM_MODIFY_REQUEST_INVALID",
			Message: err.Error(),
		})
		return
	}

	existingTicket, err := s.findLatestActiveVMTicket(
		ctx,
		vmRow.ID,
		entticket.OperationTypeMODIFY,
		domain.EventVMModifyRequested,
	)
	if err != nil {
		logger.Error("failed to check duplicate VM modify approval request",
			zap.Error(err),
			zap.String("vm_id", vmID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if existingTicket != nil {
		writeDuplicatePendingVMOperation(c, existingTicket)
		return
	}

	ticketID, eventID, err := s.createVMModifyApprovalRequest(ctx, actor, strings.TrimSpace(req.Reason), payload)
	if err != nil {
		logger.Error("failed to create VM modify approval request",
			zap.Error(err),
			zap.String("vm_id", vmID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{
			EventID:   ticketID,
			Requester: actor,
			Action:    "modify",
			Reason:    strings.TrimSpace(req.Reason),
		}); routerErr != nil {
			logger.Warn("approval router SubmitForApproval failed for modify ticket (already PENDING in DB)",
				zap.String("ticket_id", ticketID),
				zap.Error(routerErr),
			)
		}
	}
	if s.notifier != nil {
		s.notifier.OnTicketSubmitted(ctx, ticketID, actor, payload.Namespace)
	}
	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm.modify_requested", "vm", payload.VMID, actor, map[string]interface{}{
			"ticket_id": ticketID,
			"event_id":  eventID,
		})
	}

	c.JSON(http.StatusAccepted, generated.TicketResponse{
		TicketId:      ticketID,
		Status:        generated.TicketResponseStatusPENDING,
		OperationType: generated.TicketResponseOperationTypeMODIFY,
	})
}

func (s *Server) loadVisibleVMForModify(ctx context.Context, c *gin.Context, vmID generated.VMID) (*ent.VM, bool) {
	vmRow, err := s.client.VM.Get(ctx, vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return nil, false
		}
		logger.Error("failed to get VM for modify", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM namespace visibility for modify", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}
	visible, err := s.isNamespaceVisible(ctx, vmRow.Namespace, visibility)
	if err != nil {
		logger.Error("failed to check VM namespace visibility for modify", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}
	if !visible {
		c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
		return nil, false
	}
	return vmRow, true
}

func (s *Server) resolveVMModifyContext(
	ctx context.Context,
	vmRow *ent.VM,
) (generated.VMModifyContext, *domain.VM, *ent.Cluster, error) {
	resp := generated.VMModifyContext{
		VmId:            vmRow.ID,
		VmName:          vmRow.Name,
		Namespace:       vmRow.Namespace,
		ClusterId:       vmRow.ClusterID,
		CurrentCpuCores: 0,
		CurrentMemoryGi: 0,
		CurrentDiskGb:   0,
		CpuSupported:    false,
		MemorySupported: false,
		DiskSupported:   false,
	}

	clusterRow, err := s.client.Cluster.Get(ctx, vmRow.ClusterID)
	if err != nil {
		if ent.IsNotFound(err) {
			return resp, nil, nil, newVMModifyContextUnavailableError("selected cluster is not available")
		}
		return resp, nil, nil, err
	}
	resp.ClusterName = firstNonEmptyString(clusterRow.DisplayName, clusterRow.Name, clusterRow.ID)

	if !clusterRow.Enabled {
		return resp, nil, clusterRow, newVMModifyContextUnavailableError("cluster is disabled")
	}
	if clusterRow.Status != entcluster.StatusHEALTHY {
		return resp, nil, clusterRow, newVMModifyContextUnavailableError(
			fmt.Sprintf("cluster is not healthy (%s)", clusterRow.Status),
		)
	}

	if s.vmService == nil {
		return resp, nil, clusterRow, fmt.Errorf("vm service is not configured")
	}
	liveVM, err := s.vmService.GetVM(ctx, clusterRow.ID, vmRow.Namespace, vmRow.Name)
	if err != nil {
		return resp, nil, clusterRow, newVMModifyContextUnavailableError("live VM is not reachable")
	}

	resp.CurrentCpuCores = liveVM.Spec.CPU
	resp.CurrentMemoryGi = liveVM.Spec.MemoryGi
	resp.CurrentDiskGb = liveVM.Spec.DiskGB

	resp.CpuSupported = true
	resp.MemorySupported = true
	switch {
	case liveVM.Status == domain.VMStatusStopped:
	case provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"VMLiveUpdateFeatures"}):
	default:
		resp.CpuSupported = true
		resp.MemorySupported = true
		resp.CpuReason = "cpu changes will be saved now and take effect after the next restart"
		resp.MemoryReason = "memory changes will be saved now and take effect after the next restart"
	}

	if resp.CpuSupported && liveVM.Status != domain.VMStatusStopped {
		_, _, cpuErr := provider.ResolveVMLiveCPUHotplugSupport(liveVM)
		if cpuErr != nil {
			resp.CpuReason = "some cpu topology changes require a restart before they take effect"
		}
	}

	if provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"ExpandDisks"}) &&
		liveVM.Spec.RootDataVolumeName != "" &&
		liveVM.Spec.DiskHotplugSupported {
		resp.DiskSupported = true
	} else {
		switch {
		case !provider.HasAllCapabilities(clusterRow.EnabledFeatures, []string{"ExpandDisks"}):
			resp.DiskReason = "cluster does not support online disk expansion"
		case liveVM.Spec.RootDataVolumeName == "":
			resp.DiskReason = "vm does not use an expandable root data volume"
		default:
			resp.DiskReason = "disk live update is unavailable for this vm"
		}
	}

	return resp, liveVM, clusterRow, nil
}

func (s *Server) buildVMModifyPayload(
	ctx context.Context,
	vmRow *ent.VM,
	actor string,
	req generated.VMModifyRequest,
) (domain.VMModifyPayload, error) {
	snapshotLoader := newBatchSnapshotLoader(s)
	snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vmRow)
	resp, liveVM, clusterRow, err := s.resolveVMModifyContext(ctx, vmRow)
	if err != nil {
		var unavailableErr *vmModifyContextUnavailableError
		if errors.As(err, &unavailableErr) {
			return domain.VMModifyPayload{}, errors.New(unavailableErr.Error())
		}
		return domain.VMModifyPayload{}, err
	}
	if liveVM == nil || clusterRow == nil {
		return domain.VMModifyPayload{}, fmt.Errorf("live VM modify context is unavailable")
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return domain.VMModifyPayload{}, fmt.Errorf("reason is required")
	}

	targetCPU := normalizeOptionalTargetFloat64(float64(req.TargetCpuCores))
	targetMemory := normalizeOptionalTargetFloat64(float64(req.TargetMemoryGi))
	targetDisk := normalizeOptionalTargetInt(req.TargetDiskGb)
	if targetCPU == nil && targetMemory == nil && targetDisk == nil {
		return domain.VMModifyPayload{}, fmt.Errorf("at least one target resource must be provided")
	}

	if targetCPU != nil && !resp.CpuSupported {
		return domain.VMModifyPayload{}, errors.New(firstNonEmptyString(resp.CpuReason, "cpu live update is unavailable"))
	}
	if targetMemory != nil && !resp.MemorySupported {
		return domain.VMModifyPayload{}, errors.New(firstNonEmptyString(resp.MemoryReason, "memory live update is unavailable"))
	}
	if targetDisk != nil && !resp.DiskSupported {
		return domain.VMModifyPayload{}, errors.New(firstNonEmptyString(resp.DiskReason, "disk live update is unavailable"))
	}

	plan, err := provider.PlanVMResourceUpdatePatch(vmRow.Namespace, liveVM, provider.VMLiveUpdateTargets{
		CPUCores:        targetCPU,
		MemoryGi:        targetMemory,
		DiskGB:          targetDisk,
		CPURequest:      nil,
		MemoryRequestGi: nil,
	})
	if err != nil {
		return domain.VMModifyPayload{}, err
	}

	return domain.VMModifyPayload{
		VMID:                   vmRow.ID,
		VMName:                 vmRow.Name,
		ClusterID:              clusterRow.ID,
		ClusterName:            firstNonEmptyString(clusterRow.DisplayName, clusterRow.Name, clusterRow.ID),
		ClusterEnvironment:     string(clusterRow.Environment),
		Namespace:              vmRow.Namespace,
		SystemID:               snapshot.SystemID,
		SystemName:             snapshot.SystemName,
		ServiceID:              snapshot.ServiceID,
		ServiceName:            snapshot.ServiceName,
		OwnerID:                snapshot.OwnerID,
		OwnerDisplayName:       snapshot.OwnerDisplayName,
		OwnerUsername:          snapshot.OwnerUsername,
		TemplateID:             snapshot.TemplateID,
		TemplateName:           snapshot.TemplateName,
		InstanceSizeID:         snapshot.InstanceSizeID,
		InstanceSizeName:       snapshot.InstanceSizeName,
		RequestVMStatus:        string(liveVM.Status),
		Actor:                  actor,
		CurrentCPUCores:        liveVM.Spec.CPU,
		CurrentMemoryGi:        liveVM.Spec.MemoryGi,
		CurrentDiskGB:          liveVM.Spec.DiskGB,
		CurrentCPURequest:      liveVM.Spec.CPURequest,
		CurrentMemoryRequestGi: liveVM.Spec.MemoryRequestGi,
		TargetCPUCores:         targetCPU,
		TargetMemoryGi:         targetMemory,
		TargetDiskGB:           targetDisk,
		RequiresRestart:        plan.RequiresRestart,
		ApplyMode:              plan.ApplyMode,
	}, nil
}

func (s *Server) createVMModifyApprovalRequest(
	ctx context.Context,
	actor string,
	reason string,
	payload domain.VMModifyPayload,
) (ticketID, eventID string, err error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()

	eventUUID, err := uuid.NewV7()
	if err != nil {
		return "", "", fmt.Errorf("generate event id: %w", err)
	}
	ticketUUID, err := uuid.NewV7()
	if err != nil {
		return "", "", fmt.Errorf("generate ticket id: %w", err)
	}

	payloadBytes, err := payload.ToJSON()
	if err != nil {
		return "", "", err
	}

	if _, err := tx.DomainEvent.Create().
		SetID(eventUUID.String()).
		SetEventType(string(domain.EventVMModifyRequested)).
		SetAggregateType("vm").
		SetAggregateID(payload.VMID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx); err != nil {
		return "", "", err
	}

	if _, err := tx.Ticket.Create().
		SetID(ticketUUID.String()).
		SetEventID(eventUUID.String()).
		SetOperationType(entticket.OperationTypeMODIFY).
		SetStatus(entticket.StatusPENDING).
		SetRequester(actor).
		SetReason(reason).
		Save(ctx); err != nil {
		return "", "", err
	}

	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return ticketUUID.String(), eventUUID.String(), nil
}

func normalizeOptionalTargetFloat64(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	normalized := value
	return &normalized
}

func normalizeOptionalTargetInt(value int) *int {
	if value <= 0 {
		return nil
	}
	normalized := value
	return &normalized
}

func derefFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
