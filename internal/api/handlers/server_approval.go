package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/predicate"
	entservice "kv-shepherd.io/shepherd/ent/service"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

// vmTargetInfo holds extracted VM information from a DELETE domain event payload.
type vmTargetInfo struct {
	VMID   string
	VMName string
}

type ticketListOptions struct {
	mine                  bool
	page                  int
	perPage               int
	search                string
	status                string
	operationType         string
	selectedClusterID     string
	placementAdvisoryCode string
	placementSnapshot     string
}

type approvalActorLookup struct {
	DisplayName string
	Username    string
}

// ListTickets handles GET /tickets.
func (s *Server) ListTickets(c *gin.Context, params generated.ListTicketsParams) {
	ctx := c.Request.Context()
	actor := strings.TrimSpace(middleware.GetUserID(ctx))
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}
	if !params.Mine && !requireGlobalPermission(c, "ticket:view") {
		return
	}

	s.writeTicketListResponse(c, actor, ticketListOptions{
		mine:                  params.Mine,
		page:                  params.Page,
		perPage:               params.PerPage,
		search:                params.Search,
		status:                string(params.Status),
		operationType:         string(params.OperationType),
		selectedClusterID:     params.SelectedClusterId,
		placementAdvisoryCode: params.PlacementAdvisoryCode,
		placementSnapshot:     string(params.PlacementSnapshot),
	})
}

// ListBuiltinApprovalTasks handles GET /builtin-approval/tasks.
func (s *Server) ListBuiltinApprovalTasks(c *gin.Context, params generated.ListBuiltinApprovalTasksParams) {
	ctx := c.Request.Context()
	actor := strings.TrimSpace(middleware.GetUserID(ctx))
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}
	if !requireGlobalPermission(c, "builtin_approval:view") {
		return
	}

	s.writeTicketListResponse(c, actor, ticketListOptions{
		page:                  params.Page,
		perPage:               params.PerPage,
		search:                params.Search,
		status:                string(params.Status),
		operationType:         string(params.OperationType),
		selectedClusterID:     params.SelectedClusterId,
		placementAdvisoryCode: params.PlacementAdvisoryCode,
		placementSnapshot:     string(params.PlacementSnapshot),
	})
}

func (s *Server) writeTicketListResponse(c *gin.Context, actor string, options ticketListOptions) {
	ctx := c.Request.Context()
	query := s.client.Ticket.Query()
	query = query.Where(entticket.ParentTicketIDIsNil())
	if options.mine {
		query = query.Where(entticket.RequesterEQ(actor))
	}
	if search := strings.TrimSpace(options.search); search != "" {
		query = query.Where(
			entticket.Or(
				entticket.IDContainsFold(search),
				entticket.RequesterContainsFold(search),
				entticket.ApproverContainsFold(search),
				entticket.ReasonContainsFold(search),
				entticket.RejectReasonContainsFold(search),
				entticket.SelectedClusterIDContainsFold(search),
				predicate.Ticket(func(s *entsql.Selector) {
					s.Where(sqljson.StringContains(
						entticket.FieldPlacementEvaluation,
						search,
						sqljson.Path("selected_cluster_name"),
					))
				}),
			),
		)
	}

	// Filter by status (omitzero: empty string = not specified).
	if options.status != "" {
		query = query.Where(entticket.StatusEQ(entticket.Status(options.status)))
	}
	if options.operationType != "" {
		query = query.Where(entticket.OperationTypeEQ(entticket.OperationType(options.operationType)))
	}
	if options.selectedClusterID != "" {
		query = query.Where(entticket.SelectedClusterIDEQ(options.selectedClusterID))
	}
	if options.placementAdvisoryCode != "" {
		query = query.Where(
			predicate.Ticket(func(s *entsql.Selector) {
				s.Where(sqljson.ValueEQ(
					entticket.FieldPlacementEvaluation,
					options.placementAdvisoryCode,
					sqljson.Path("advisory_code"),
				))
			}),
		)
	}
	switch options.placementSnapshot {
	case "":
	case "present":
		query = query.Where(entticket.PlacementEvaluationNotNil())
	case "missing":
		query = query.Where(entticket.PlacementEvaluationIsNil())
	}

	page, perPage := defaultPagination(options.page, options.perPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting approval tasks", zap.Error(err))
			return
		}
		logger.Error("failed to count approval tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tickets, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(entticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing approval tasks", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list approval tasks", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Collect all event IDs to batch-fetch domain events.
	// DELETE tickets: extract target VM info; all tickets: include raw payload.
	allEventIDs := make([]string, 0, len(tickets))
	vmTargetEventIDSet := make(map[string]struct{})
	createTicketIDs := make([]string, 0)
	for _, t := range tickets {
		allEventIDs = append(allEventIDs, t.EventID)
		if t.OperationType == entticket.OperationTypeDELETE || t.OperationType == entticket.OperationTypeMODIFY {
			vmTargetEventIDSet[t.EventID] = struct{}{}
		}
		if t.OperationType == entticket.OperationTypeCREATE {
			createTicketIDs = append(createTicketIDs, t.ID)
		}
	}

	// Batch-fetch domain events for all tickets.
	vmInfoMap := make(map[string]vmTargetInfo) // keyed by event ID
	eventPayloadMap := make(map[string][]byte) // keyed by event ID; value is raw JSON payload
	eventByID := make(map[string]*ent.DomainEvent)
	if len(allEventIDs) > 0 {
		events, err := s.client.DomainEvent.Query().
			Where(domainevent.IDIn(allEventIDs...)).
			All(ctx)
		if err != nil {
			// Non-fatal: log and continue without event info.
			logger.Warn("failed to fetch domain events for approval tasks", zap.Error(err))
		} else {
			for _, ev := range events {
				eventByID[ev.ID] = ev
				// Store raw payload for all tickets.
				eventPayloadMap[ev.ID] = ev.Payload
				// Extract VM target info for VM-targeting tickets (DELETE / MODIFY).
				if _, isVMTarget := vmTargetEventIDSet[ev.ID]; !isVMTarget {
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

	templateIDs, instanceSizeIDs := collectApprovalCatalogLookupIDs(eventPayloadMap)
	serviceByID := s.loadApprovalServiceLookups(
		ctx,
		collectApprovalPrefillServiceIDs(eventPayloadMap),
	)
	vmByID, vmTemplateIDs, vmInstanceSizeIDs := s.loadApprovalVMContexts(
		ctx,
		collectApprovalSummaryVMIDs(eventPayloadMap),
		serviceByID,
	)
	templateIDs = append(templateIDs, vmTemplateIDs...)
	instanceSizeIDs = append(instanceSizeIDs, vmInstanceSizeIDs...)
	templateByID, instanceSizeByID := s.loadApprovalCatalogLookups(
		ctx,
		sortedStringSet(sliceToStringSet(templateIDs)),
		sortedStringSet(sliceToStringSet(instanceSizeIDs)),
	)
	systemIDByServiceID := buildApprovalSystemIDByServiceID(serviceByID)
	batchProjectionByID := s.loadApprovalBatchProjections(ctx, tickets, eventByID)

	createVMByTicketID := make(map[string]*ent.VM)
	if len(createTicketIDs) > 0 {
		vms, err := s.client.VM.Query().
			Where(entvm.TicketIDIn(createTicketIDs...)).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch VMs for create approval tasks", zap.Error(err))
		} else {
			for _, vm := range vms {
				if vm == nil || vm.TicketID == "" {
					continue
				}
				createVMByTicketID[vm.TicketID] = vm
			}
		}
	}

	actorByID := s.loadApprovalActorLookups(ctx, tickets)
	batchFallbackItemsByParentID := s.loadApprovalBatchChildFallbackItems(
		ctx,
		tickets,
		templateByID,
		instanceSizeByID,
		serviceByID,
		vmByID,
		actorByID,
	)

	items := make([]generated.Ticket, 0, len(tickets))
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
		enrichApprovalPayload(payloadMap, templateByID, instanceSizeByID, batchProjectionByID[t.ID])
		var provisioning *generated.ProvisioningStatus
		if t.OperationType == entticket.OperationTypeCREATE {
			provisioning = s.loadVMProvisioning(ctx, createVMByTicketID[t.ID])
		}
		item := ticketToAPI(
			t,
			payloadMap,
			provisioning,
			buildTicketSummary(
				t,
				payloadMap,
				templateByID,
				instanceSizeByID,
				serviceByID,
				vmByID,
				actorByID,
				batchFallbackItemsByParentID[t.ID],
			),
			buildApprovalRequestPrefill(payloadMap, systemIDByServiceID),
			actorByID[t.Requester],
			actorByID[t.Approver],
		)
		// Enrich VM-targeting tickets with target VM info.
		if t.OperationType == entticket.OperationTypeDELETE || t.OperationType == entticket.OperationTypeMODIFY {
			if info, ok := vmInfoMap[t.EventID]; ok {
				item.TargetVmId = info.VMID
				item.TargetVmName = info.VMName
			}
		}
		items = append(items, item)
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.TicketList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ApproveBuiltinApprovalTask handles POST /builtin-approval/tasks/{ticket_id}/approve.
func (s *Server) ApproveBuiltinApprovalTask(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "builtin_approval:approve") {
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

	if s.approvalRouter == nil {
		logger.Error("approval provider is not configured", zap.String("ticket_id", ticketID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := s.approvalRouter.ProcessApproval(ctx, ticketID, approvalcontract.ApprovalDecision{
		Approved: true,
		Approver: actor,
		Execution: approvalcontract.ApprovalExecutionOptions{
			ClusterID:       req.SelectedClusterId,
			StorageClass:    req.SelectedStorageClass,
			DVAccessModes:   req.SelectedDvAccessModes,
			DVVolumeMode:    string(req.SelectedDvVolumeMode),
			EnableOverride:  req.EnableOverride,
			CPURequest:      float64(req.CpuRequest),
			CPULimit:        float64(req.CpuLimit),
			MemoryRequestGi: float64(req.MemoryRequestGi),
			MemoryLimitGi:   float64(req.MemoryLimitGi),
			DiskGB:          req.DiskGb,
		},
	}); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:        appErr.Code,
				Message:     appErr.Message,
				Params:      appErr.Params,
				FieldErrors: appFieldErrorsToAPI(appErr.FieldErrors),
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

// RejectBuiltinApprovalTask handles POST /builtin-approval/tasks/{ticket_id}/reject.
func (s *Server) RejectBuiltinApprovalTask(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "builtin_approval:approve") {
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

	if s.approvalRouter == nil {
		logger.Error("approval provider is not configured", zap.String("ticket_id", ticketID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := s.approvalRouter.ProcessApproval(ctx, ticketID, approvalcontract.ApprovalDecision{
		Approved:     false,
		Approver:     actor,
		RejectReason: req.Reason,
	}); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:        appErr.Code,
				Message:     appErr.Message,
				Params:      appErr.Params,
				FieldErrors: appFieldErrorsToAPI(appErr.FieldErrors),
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

// CancelTicket handles POST /tickets/{ticket_id}/cancel.
func (s *Server) CancelTicket(c *gin.Context, ticketID generated.TicketID) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	if s.ticketService == nil {
		logger.Error("ticket service is not configured", zap.String("ticket_id", ticketID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if err := s.ticketService.Cancel(ctx, ticketID, actor); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.JSON(appErr.HTTPStatus, generated.Error{
				Code:        appErr.Code,
				Message:     appErr.Message,
				Params:      appErr.Params,
				FieldErrors: appFieldErrorsToAPI(appErr.FieldErrors),
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

// ticketToAPI converts an Ent Ticket to the generated ticket API type.
// ticketPayload is the deserialized DomainEvent payload (may be nil for older/missing events).
func ticketToAPI(
	t *ent.Ticket,
	ticketPayload map[string]interface{},
	provisioning *generated.ProvisioningStatus,
	summary *generated.TicketSummary,
	requestPrefill *generated.VMRequestPrefill,
	requester approvalActorLookup,
	approver approvalActorLookup,
) generated.Ticket {
	var placementEvaluation *generated.PlacementEvaluation
	if len(t.PlacementEvaluation) > 0 {
		placementEvaluation = placementEvaluationToAPI(t.PlacementEvaluation)
	}
	return generated.Ticket{
		Id:            t.ID,
		EventId:       t.EventID,
		OperationType: generated.TicketOperationType(t.OperationType),
		Requester:     t.Requester,
		RequesterDisplayName: firstNonEmptyString(
			strings.TrimSpace(requester.DisplayName),
			strings.TrimSpace(requester.Username),
		),
		RequesterUsername: strings.TrimSpace(requester.Username),
		Status:            generated.TicketStatus(t.Status),
		Approver:          t.Approver,
		ApproverDisplayName: firstNonEmptyString(
			strings.TrimSpace(approver.DisplayName),
			strings.TrimSpace(approver.Username),
		),
		ApproverUsername:    strings.TrimSpace(approver.Username),
		Reason:              t.Reason,
		RejectReason:        t.RejectReason,
		Summary:             summary,
		RequestPrefill:      derefApprovalRequestPrefill(requestPrefill),
		TicketPayload:       ticketPayload,
		Provisioning:        provisioning,
		PlacementEvaluation: placementEvaluation,
		CreatedAt:           t.CreatedAt,
	}
}

func (s *Server) loadApprovalActorLookups(
	ctx context.Context,
	tickets []*ent.Ticket,
) map[string]approvalActorLookup {
	idSet := make(map[string]struct{})
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		if requester := strings.TrimSpace(ticket.Requester); requester != "" {
			idSet[requester] = struct{}{}
		}
		if approver := strings.TrimSpace(ticket.Approver); approver != "" {
			idSet[approver] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return map[string]approvalActorLookup{}
	}

	userIDs := make([]string, 0, len(idSet))
	for userID := range idSet {
		userIDs = append(userIDs, userID)
	}

	return s.loadApprovalActorLookupsByIDs(ctx, userIDs)
}

func (s *Server) loadApprovalActorLookupsByIDs(
	ctx context.Context,
	userIDs []string,
) map[string]approvalActorLookup {
	if len(userIDs) == 0 {
		return map[string]approvalActorLookup{}
	}

	users, err := s.client.User.Query().
		Where(entuser.IDIn(userIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch users for approval task actors", zap.Error(err))
		return map[string]approvalActorLookup{}
	}

	byID := make(map[string]approvalActorLookup, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		byID[user.ID] = approvalActorLookup{
			DisplayName: strings.TrimSpace(user.DisplayName),
			Username:    strings.TrimSpace(user.Username),
		}
	}
	return byID
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
	if value, ok := snapshot["requested_dv_access_modes"].([]interface{}); ok {
		result.RequestedDvAccessModes = interfaceSliceToStringSlice(value)
	}
	if value, ok := snapshot["effective_dv_access_modes"].([]interface{}); ok {
		result.EffectiveDvAccessModes = interfaceSliceToStringSlice(value)
	}
	if value, ok := snapshot["requested_dv_volume_mode"].(string); ok {
		result.RequestedDvVolumeMode = generated.PlacementEvaluationRequestedDvVolumeMode(value)
	}
	if value, ok := snapshot["effective_dv_volume_mode"].(string); ok {
		result.EffectiveDvVolumeMode = generated.PlacementEvaluationEffectiveDvVolumeMode(value)
	}
	if value, ok := snapshot["root_volume_resolution_state"].(string); ok {
		result.RootVolumeResolutionState = generated.PlacementEvaluationRootVolumeResolutionState(value)
	}
	if value, ok := snapshot["root_volume_resolution_message"].(string); ok {
		result.RootVolumeResolutionMessage = value
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
		if override := placementOverrideToAPI(value); override != nil {
			result.Override = *override
		}
	}
	if value, ok := snapshot["evaluated_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			result.EvaluatedAt = parsed
		}
	}
	return result
}

func appFieldErrorsToAPI(fieldErrors []apperrors.FieldError) []generated.FieldError {
	if len(fieldErrors) == 0 {
		return nil
	}
	result := make([]generated.FieldError, 0, len(fieldErrors))
	for _, fieldError := range fieldErrors {
		result = append(result, generated.FieldError{
			Code:    fieldError.Code,
			Field:   fieldError.Field,
			Message: fieldError.Message,
		})
	}
	return result
}

func placementOverrideToAPI(snapshot map[string]interface{}) *generated.PlacementResourceOverride {
	if len(snapshot) == 0 {
		return nil
	}
	result := &generated.PlacementResourceOverride{}
	if value, ok := snapshot["enabled"].(bool); ok {
		result.Enabled = value
	}
	if value, ok := snapshot["cpu_request"].(float64); ok {
		result.CpuRequest = value
	}
	if value, ok := snapshot["cpu_limit"].(float64); ok {
		result.CpuLimit = value
	}
	if value, ok := snapshot["memory_request_gi"].(float64); ok {
		result.MemoryRequestGi = value
	}
	if value, ok := snapshot["memory_limit_gi"].(float64); ok {
		result.MemoryLimitGi = value
	}
	switch value := snapshot["disk_gb"].(type) {
	case int:
		result.DiskGb = value
	case float64:
		result.DiskGb = int(value)
	}
	if *result == (generated.PlacementResourceOverride{}) {
		return nil
	}
	return result
}

func interfaceSliceToStringSlice(values []interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		text, ok := raw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			out = append(out, text)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectApprovalCatalogLookupIDs(eventPayloadMap map[string][]byte) (templateIDs, instanceSizeIDs []string) {
	templateIDSet := make(map[string]struct{})
	instanceSizeIDSet := make(map[string]struct{})
	for _, raw := range eventPayloadMap {
		if len(raw) == 0 {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		collectCatalogIDsFromPayload(payload, templateIDSet, instanceSizeIDSet)
	}
	return sortedStringSet(templateIDSet), sortedStringSet(instanceSizeIDSet)
}

func collectCatalogIDsFromPayload(
	payload map[string]interface{},
	templateIDs map[string]struct{},
	instanceSizeIDs map[string]struct{},
) {
	if len(payload) == 0 {
		return
	}
	if templateID := trimPayloadString(payload["template_id"]); templateID != "" {
		templateIDs[templateID] = struct{}{}
	}
	if instanceSizeID := trimPayloadString(payload["instance_size_id"]); instanceSizeID != "" {
		instanceSizeIDs[instanceSizeID] = struct{}{}
	}
	items, ok := payload["items"].([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if templateID := trimPayloadString(itemMap["template_id"]); templateID != "" {
			templateIDs[templateID] = struct{}{}
		}
		if instanceSizeID := trimPayloadString(itemMap["instance_size_id"]); instanceSizeID != "" {
			instanceSizeIDs[instanceSizeID] = struct{}{}
		}
	}
}

func collectApprovalPrefillServiceIDs(eventPayloadMap map[string][]byte) []string {
	serviceIDSet := make(map[string]struct{})
	for _, raw := range eventPayloadMap {
		if len(raw) == 0 {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		collectServiceIDsFromPayload(payload, serviceIDSet)
	}
	return sortedStringSet(serviceIDSet)
}

func collectServiceIDsFromPayload(
	payload map[string]interface{},
	serviceIDs map[string]struct{},
) {
	if len(payload) == 0 {
		return
	}
	if serviceID := trimPayloadString(payload["service_id"]); serviceID != "" {
		serviceIDs[serviceID] = struct{}{}
	}
	items, ok := payload["items"].([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if serviceID := trimPayloadString(itemMap["service_id"]); serviceID != "" {
			serviceIDs[serviceID] = struct{}{}
		}
	}
}

func collectApprovalActorIDsFromPayload(
	payload map[string]interface{},
	actorIDs map[string]struct{},
) {
	if len(payload) == 0 {
		return
	}
	for _, key := range []string{"owner_id", "requester_id", "created_by", "actor"} {
		if actorID := trimPayloadString(payload[key]); actorID != "" {
			actorIDs[actorID] = struct{}{}
		}
	}
	items, ok := payload["items"].([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		collectApprovalActorIDsFromPayload(itemMap, actorIDs)
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func trimPayloadString(value interface{}) string {
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func trimPayloadPositiveInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case float64:
		if v >= 1 {
			return int(v)
		}
	}
	return 0
}

func parsePrefillUUID(value string) (openapi_types.UUID, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, false
	}
	return parsed, true
}

func approvalPrefillItemFromPayload(
	item map[string]interface{},
	systemIDByServiceID map[string]string,
) (*generated.VMRequestPrefill, bool) {
	serviceID := trimPayloadString(item["service_id"])
	templateID := trimPayloadString(item["template_id"])
	instanceSizeID := trimPayloadString(item["instance_size_id"])
	namespace := trimPayloadString(item["namespace"])
	reason := trimPayloadString(item["reason"])
	systemID := systemIDByServiceID[serviceID]
	if serviceID == "" || systemID == "" || templateID == "" || instanceSizeID == "" || namespace == "" || reason == "" {
		return nil, false
	}
	systemUUID, ok := parsePrefillUUID(systemID)
	if !ok {
		return nil, false
	}
	serviceUUID, ok := parsePrefillUUID(serviceID)
	if !ok {
		return nil, false
	}
	templateUUID, ok := parsePrefillUUID(templateID)
	if !ok {
		return nil, false
	}
	instanceSizeUUID, ok := parsePrefillUUID(instanceSizeID)
	if !ok {
		return nil, false
	}
	return &generated.VMRequestPrefill{
		SystemId:       systemUUID,
		ServiceId:      serviceUUID,
		TemplateId:     templateUUID,
		InstanceSizeId: instanceSizeUUID,
		Namespace:      namespace,
		Reason:         reason,
		BatchCount:     1,
	}, true
}

func sameApprovalPrefillShape(a, b *generated.VMRequestPrefill) bool {
	if a == nil || b == nil {
		return false
	}
	return a.SystemId == b.SystemId &&
		a.ServiceId == b.ServiceId &&
		a.TemplateId == b.TemplateId &&
		a.InstanceSizeId == b.InstanceSizeId &&
		a.Namespace == b.Namespace &&
		a.Reason == b.Reason
}

func buildApprovalRequestPrefill(
	payload map[string]interface{},
	systemIDByServiceID map[string]string,
) *generated.VMRequestPrefill {
	if len(payload) == 0 {
		return nil
	}
	if direct, ok := approvalPrefillItemFromPayload(payload, systemIDByServiceID); ok {
		return direct
	}

	items, ok := payload["items"].([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}

	firstItem, ok := items[0].(map[string]interface{})
	if !ok {
		return nil
	}
	prefill, ok := approvalPrefillItemFromPayload(firstItem, systemIDByServiceID)
	if !ok {
		return nil
	}

	for _, raw := range items[1:] {
		itemMap, ok := raw.(map[string]interface{})
		if !ok {
			return nil
		}
		nextPrefill, ok := approvalPrefillItemFromPayload(itemMap, systemIDByServiceID)
		if !ok || !sameApprovalPrefillShape(prefill, nextPrefill) {
			return nil
		}
	}

	if topLevelReason := trimPayloadString(payload["reason"]); topLevelReason != "" {
		prefill.Reason = topLevelReason
	}
	if batchCount := trimPayloadPositiveInt(payload["batch_item_count"]); batchCount > 0 {
		prefill.BatchCount = batchCount
	} else {
		prefill.BatchCount = len(items)
	}
	return prefill
}

func derefApprovalRequestPrefill(prefill *generated.VMRequestPrefill) generated.VMRequestPrefill {
	if prefill == nil {
		return generated.VMRequestPrefill{}
	}
	return *prefill
}

func (s *Server) loadApprovalCatalogLookups(
	ctx context.Context,
	templateIDs []string,
	instanceSizeIDs []string,
) (templateByID map[string]*ent.Template, instanceSizeByID map[string]*ent.InstanceSize) {
	templateByID = make(map[string]*ent.Template, len(templateIDs))
	if len(templateIDs) > 0 {
		templates, err := s.client.Template.Query().
			Where(enttemplate.IDIn(templateIDs...)).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch templates for approval payload enrichment", zap.Error(err))
		} else {
			for _, tpl := range templates {
				templateByID[tpl.ID] = tpl
			}
		}
	}

	instanceSizeByID = make(map[string]*ent.InstanceSize, len(instanceSizeIDs))
	if len(instanceSizeIDs) > 0 {
		sizes, err := s.client.InstanceSize.Query().
			Where(entinstancesize.IDIn(instanceSizeIDs...)).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch instance sizes for approval payload enrichment", zap.Error(err))
		} else {
			for _, size := range sizes {
				instanceSizeByID[size.ID] = size
			}
		}
	}
	return templateByID, instanceSizeByID
}

func (s *Server) loadApprovalPrefillSystemByServiceID(
	ctx context.Context,
	eventPayloadMap map[string][]byte,
) map[string]string {
	serviceIDs := collectApprovalPrefillServiceIDs(eventPayloadMap)
	byServiceID := make(map[string]string, len(serviceIDs))
	if len(serviceIDs) == 0 {
		return byServiceID
	}

	services, err := s.client.Service.Query().
		Where(entservice.IDIn(serviceIDs...)).
		WithSystem().
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch services for approval request prefill", zap.Error(err))
		return byServiceID
	}

	for _, svc := range services {
		if svc == nil || svc.ID == "" || svc.Edges.System == nil {
			continue
		}
		byServiceID[svc.ID] = svc.Edges.System.ID
	}
	return byServiceID
}

func (s *Server) loadApprovalBatchProjections(
	ctx context.Context,
	tickets []*ent.Ticket,
	eventByID map[string]*ent.DomainEvent,
) map[string]*ent.BatchTicket {
	parentIDs := make([]string, 0)
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		event := eventByID[ticket.EventID]
		if event == nil || strings.TrimSpace(event.AggregateType) != "batch" {
			continue
		}
		parentIDs = append(parentIDs, ticket.ID)
	}
	if len(parentIDs) == 0 {
		return map[string]*ent.BatchTicket{}
	}
	rows, err := s.client.BatchTicket.Query().
		Where(entbatchticket.IDIn(parentIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch batch projections for approval list", zap.Error(err))
		return map[string]*ent.BatchTicket{}
	}
	byID := make(map[string]*ent.BatchTicket, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	return byID
}

func enrichApprovalPayload(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	batchProjection *ent.BatchTicket,
) {
	if len(payload) == 0 {
		return
	}
	enrichApprovalPayloadItem(payload, templateByID, instanceSizeByID)
	if items, ok := payload["items"].([]interface{}); ok {
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			enrichApprovalPayloadItem(item, templateByID, instanceSizeByID)
		}
		payload["batch_item_count"] = len(items)
	}
	if batchProjection != nil {
		payload["batch_summary"] = map[string]interface{}{
			"status":        string(batchProjection.Status),
			"child_count":   batchProjection.ChildCount,
			"success_count": batchProjection.SuccessCount,
			"failed_count":  batchProjection.FailedCount,
			"pending_count": batchProjection.PendingCount,
		}
	}
}

func enrichApprovalPayloadItem(
	item map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
) {
	if len(item) == 0 {
		return
	}
	if templateID := trimPayloadString(item["template_id"]); templateID != "" {
		if tpl := templateByID[templateID]; tpl != nil {
			item["template_name"] = tpl.Name
			if strings.TrimSpace(tpl.DisplayName) != "" {
				item["template_display_name"] = tpl.DisplayName
				item["template_label"] = tpl.DisplayName
			} else {
				item["template_label"] = tpl.Name
			}
			if strings.TrimSpace(tpl.OsFamily) != "" {
				item["template_os_family"] = tpl.OsFamily
			}
			if strings.TrimSpace(tpl.OsVersion) != "" {
				item["template_os_version"] = tpl.OsVersion
			}
		}
	}
	if instanceSizeID := trimPayloadString(item["instance_size_id"]); instanceSizeID != "" {
		if size := instanceSizeByID[instanceSizeID]; size != nil {
			item["instance_size_name"] = size.Name
			if strings.TrimSpace(size.DisplayName) != "" {
				item["instance_size_display_name"] = size.DisplayName
				item["instance_size_label"] = size.DisplayName
			} else {
				item["instance_size_label"] = size.Name
			}
			item["instance_size_disk_gb"] = size.DiskGB
			item["instance_size_dedicated_cpu"] = size.DedicatedCPU
			item["instance_size_cpu_cores"] = size.CPUCores
			item["instance_size_memory_gi"] = size.MemoryGi
			item["instance_size_catalog_scope"] = string(size.CatalogScope)
		}
	}
}
