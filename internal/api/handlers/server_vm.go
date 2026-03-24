package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

const (
	placementReasonCodeClusterPolicyDenied = "CLUSTER_POLICY_DENIED"
	placementAdvisoryCodeHostAssistedClone = "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY"
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
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while resolving VM namespace visibility", zap.Error(err))
			return
		}
		logger.Error("failed to resolve VM namespace visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.restricted {
		visibleNamespaces, listErr := s.listVisibleNamespaceNames(ctx, visibility)
		if listErr != nil {
			if isRequestContextCanceled(listErr) {
				logger.Debug("request canceled while listing visible namespaces", zap.Error(listErr))
				return
			}
			logger.Error("failed to load visible namespaces", zap.Error(listErr))
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
	// Exclude VM tombstones (DELETING status = K8s deleted, DB hard-delete failed).
	// Unless the user explicitly filters by DELETING status.
	if params.Status == "" || entvm.Status(params.Status) != entvm.StatusDELETING {
		query = query.Where(entvm.StatusNEQ(entvm.StatusDELETING))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting VMs", zap.Error(err))
			return
		}
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
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing VMs", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list VMs", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	vms = s.refreshVMLiveStates(ctx, vms)

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
	clusterEnvMap := make(map[string]string)  // cluster_id → environment string
	clusterNameMap := make(map[string]string) // cluster_id → display name fallback chain
	if len(clusterIDs) > 0 {
		clusters, clusterErr := s.client.Cluster.Query().
			Where(entcluster.IDIn(clusterIDs...)).
			Select(entcluster.FieldID, entcluster.FieldEnvironment, entcluster.FieldDisplayName, entcluster.FieldName).
			All(ctx)
		if clusterErr != nil {
			// Non-fatal: log and continue without environment info.
			logger.Warn("failed to fetch cluster info for VM list", zap.Error(clusterErr))
		} else {
			for _, cl := range clusters {
				clusterEnvMap[cl.ID] = string(cl.Environment)
				clusterNameMap[cl.ID] = firstNonEmptyString(cl.DisplayName, cl.Name, cl.ID)
			}
		}
	}

	items := make([]generated.VM, 0, len(vms))
	for _, vm := range vms {
		env := clusterEnvMap[vm.ClusterID]
		name := clusterNameMap[vm.ClusterID]
		items = append(items, vmToAPI(vm, env, name, nil))
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
func (s *Server) GetVMRequestContext(c *gin.Context, params generated.GetVMRequestContextParams) {
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

	templateScopes := visibleTemplateCatalogScopes(visibility)
	var templates []*ent.Template
	if len(templateScopes) == 0 {
		templates = []*ent.Template{}
	} else {
		templates, err = s.client.Template.Query().
			Where(
				enttemplate.EnabledEQ(true),
				enttemplate.CatalogScopeIn(templateScopes...),
			).
			Order(ent.Asc(enttemplate.FieldName)).
			All(ctx)
		if err != nil {
			logger.Error("failed to list request-context templates", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}
	templates = filterUserRequestableTemplates(templates)

	sizeScopes := visibleInstanceSizeCatalogScopes(visibility)
	var sizes []*ent.InstanceSize
	if len(sizeScopes) == 0 {
		sizes = []*ent.InstanceSize{}
	} else {
		sizes, err = s.client.InstanceSize.Query().
			Where(
				instancesize.EnabledEQ(true),
				instancesize.CatalogScopeIn(sizeScopes...),
			).
			Order(ent.Asc(instancesize.FieldSortOrder)).
			All(ctx)
		if err != nil {
			logger.Error("failed to list request-context instance sizes", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	templateItems := make([]generated.Template, 0, len(templates))
	for _, t := range templates {
		templateItems = append(templateItems, templateToAPI(t))
	}

	sizeItems := make([]generated.InstanceSize, 0, len(sizes))
	for _, sz := range sizes {
		sizeItems = append(sizeItems, instanceSizeToPublicAPI(sz))
	}

	resp := generated.VMRequestContext{
		Namespaces:    namespaces,
		Templates:     templateItems,
		InstanceSizes: sizeItems,
	}
	if shouldEvaluateUserPlacementHint(params) {
		hint, hintErr := s.buildUserPlacementHint(ctx, visibility, namespaces, templates, sizes, params)
		if hintErr != nil {
			if appErr, ok := apperrors.IsAppError(hintErr); ok {
				c.JSON(appErr.HTTPStatus, generated.Error{
					Code:    appErr.Code,
					Message: appErr.Message,
					Params:  appErr.Params,
				})
				return
			}
			logger.Error("failed to evaluate VM request placement hint", zap.Error(hintErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		resp.PlacementHint = *hint
	}

	c.JSON(http.StatusOK, resp)
}

func shouldEvaluateUserPlacementHint(params generated.GetVMRequestContextParams) bool {
	return params.Namespace != "" && params.TemplateId != uuid.Nil && params.InstanceSizeId != uuid.Nil
}

func (s *Server) buildUserPlacementHint(
	ctx context.Context,
	visibility namespaceVisibility,
	visibleNamespaces []string,
	visibleTemplates []*ent.Template,
	visibleSizes []*ent.InstanceSize,
	params generated.GetVMRequestContextParams,
) (*generated.VMPlacementHint, error) {
	namespaceName := params.Namespace
	templateID := params.TemplateId.String()
	instanceSizeID := params.InstanceSizeId.String()

	if !stringSliceContains(visibleNamespaces, namespaceName) {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "selected request context items are not available")
	}
	if !hasVisibleTemplate(visibleTemplates, templateID) || !hasVisibleInstanceSize(visibleSizes, instanceSizeID) {
		return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "selected request context items are not available")
	}

	namespaceEntity, err := s.client.NamespaceRegistry.Query().
		Where(namespaceregistry.NameEQ(namespaceName)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apperrors.BadRequest(apperrors.CodeValidationFailed, "selected request context items are not available")
		}
		return nil, fmt.Errorf("query namespace for placement hint: %w", err)
	}

	clusterQuery := s.client.Cluster.Query().
		Where(
			entcluster.EnabledEQ(true),
			entcluster.EnvironmentEQ(entcluster.Environment(namespaceEntity.Environment)),
		)
	if visibility.restricted && len(visibility.envs) > 0 {
		allowedClusterEnvs := make([]entcluster.Environment, 0, len(visibility.envs))
		for _, env := range visibility.envs {
			allowedClusterEnvs = append(allowedClusterEnvs, entcluster.Environment(env))
		}
		clusterQuery = clusterQuery.Where(entcluster.EnvironmentIn(allowedClusterEnvs...))
	}

	clusters, err := clusterQuery.
		Order(ent.Asc(entcluster.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query clusters for placement hint: %w", err)
	}
	if len(clusters) == 0 {
		return &generated.VMPlacementHint{
			Status:                 generated.UNAVAILABLE,
			CompatibleClusterCount: 0,
			EvaluatedClusterCount:  0,
			PrimaryReasonCode:      generated.VMPlacementHintPrimaryReasonCodeNoCandidateClusters,
		}, nil
	}

	validator := service.NewApprovalValidator(s.client).SetVMService(s.vmService)
	results, err := validator.EvaluateClusterCompatibility(ctx, clusters, service.ApprovalValidationInput{
		Namespace:      namespaceName,
		TemplateID:     templateID,
		InstanceSizeID: instanceSizeID,
	})
	if err != nil {
		return nil, err
	}
	return userPlacementHintFromResults(results), nil
}

func userPlacementHintFromResults(results []service.ClusterCompatibilityResult) *generated.VMPlacementHint {
	compatible := 0
	reasonCounts := make(map[generated.VMPlacementHintReasonCountCode]int)
	advisoryCounts := make(map[generated.VMPlacementHintAdvisoryCountCode]int)
	for _, result := range results {
		if result.Eligible {
			compatible++
			if result.AdvisoryCode != "" || result.AdvisoryMessage != "" {
				advisory := normalizeUserPlacementAdvisory(result.AdvisoryCode, result.AdvisoryMessage)
				advisoryCounts[advisory]++
			}
			continue
		}
		reason := normalizeUserPlacementReason(result.ReasonCode, result.ReasonMessage)
		reasonCounts[reason]++
	}

	status := generated.UNAVAILABLE
	if compatible > 0 {
		status = generated.AVAILABLE
	}

	items := make([]generated.VMPlacementHintReasonCount, 0, len(reasonCounts))
	var (
		primaryReason generated.VMPlacementHintPrimaryReasonCode
		primaryCount  int
	)
	for code, count := range reasonCounts {
		items = append(items, generated.VMPlacementHintReasonCount{
			Code:  code,
			Count: count,
		})
		candidatePrimary := generated.VMPlacementHintPrimaryReasonCode(code)
		if count > primaryCount || (count == primaryCount && string(candidatePrimary) < string(primaryReason)) {
			primaryReason = candidatePrimary
			primaryCount = count
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return string(items[i].Code) < string(items[j].Code)
		}
		return items[i].Count > items[j].Count
	})

	advisories := make([]generated.VMPlacementHintAdvisoryCount, 0, len(advisoryCounts))
	var (
		primaryAdvisory      generated.VMPlacementHintPrimaryAdvisoryCode
		primaryAdvisoryCount int
	)
	for code, count := range advisoryCounts {
		advisories = append(advisories, generated.VMPlacementHintAdvisoryCount{
			Code:  code,
			Count: count,
		})
		candidatePrimary := generated.VMPlacementHintPrimaryAdvisoryCode(code)
		if count > primaryAdvisoryCount || (count == primaryAdvisoryCount && string(candidatePrimary) < string(primaryAdvisory)) {
			primaryAdvisory = candidatePrimary
			primaryAdvisoryCount = count
		}
	}
	sort.Slice(advisories, func(i, j int) bool {
		if advisories[i].Count == advisories[j].Count {
			return string(advisories[i].Code) < string(advisories[j].Code)
		}
		return advisories[i].Count > advisories[j].Count
	})

	hint := &generated.VMPlacementHint{
		Status:                 status,
		CompatibleClusterCount: compatible,
		EvaluatedClusterCount:  len(results),
		ReasonCounts:           items,
		AdvisoryCounts:         advisories,
	}
	if primaryCount > 0 {
		hint.PrimaryReasonCode = primaryReason
	}
	if primaryAdvisoryCount > 0 {
		hint.PrimaryAdvisoryCode = primaryAdvisory
	}
	return hint
}

func normalizeUserPlacementReason(rawCode, rawMessage string) generated.VMPlacementHintReasonCountCode {
	switch rawCode {
	case "CLUSTER_POLICY_NOT_CONFIGURED":
		return generated.VMPlacementHintReasonCountCodePolicyNotConfigured
	case placementReasonCodeClusterPolicyDenied:
		return generated.VMPlacementHintReasonCountCodePolicyDenied
	case "OVERCOMMIT_INVALID", "DEDICATED_CPU_OVERCOMMIT_CONFLICT":
		return generated.VMPlacementHintReasonCountCodeRequestInvalid
	}

	message := strings.ToLower(strings.TrimSpace(rawMessage))
	switch {
	case strings.Contains(message, "missing required capabilities"):
		return generated.VMPlacementHintReasonCountCodeCapabilityMismatch
	case strings.Contains(message, "not healthy"):
		return generated.VMPlacementHintReasonCountCodeClusterUnavailable
	case strings.Contains(message, "requires guaranteed qos"):
		return generated.VMPlacementHintReasonCountCodeRequestInvalid
	case message != "":
		return generated.VMPlacementHintReasonCountCodeOther
	default:
		return generated.VMPlacementHintReasonCountCodeOther
	}
}

func normalizeUserPlacementAdvisory(rawCode, rawMessage string) generated.VMPlacementHintAdvisoryCountCode {
	if rawCode == placementAdvisoryCodeHostAssistedClone {
		return generated.VMPlacementHintAdvisoryCountCodeHostAssistedCloneLikely
	}

	message := strings.ToLower(strings.TrimSpace(rawMessage))
	switch {
	case strings.Contains(message, "host-assisted copy"), strings.Contains(message, "host assisted copy"):
		return generated.VMPlacementHintAdvisoryCountCodeHostAssistedCloneLikely
	case message != "":
		return generated.VMPlacementHintAdvisoryCountCodeOther
	default:
		return generated.VMPlacementHintAdvisoryCountCodeOther
	}
}

func hasVisibleTemplate(items []*ent.Template, templateID string) bool {
	for _, item := range items {
		if item != nil && item.ID == templateID {
			return true
		}
	}
	return false
}

func hasVisibleInstanceSize(items []*ent.InstanceSize, sizeID string) bool {
	for _, item := range items {
		if item != nil && item.ID == sizeID {
			return true
		}
	}
	return false
}

func stringSliceContains(items []string, candidate string) bool {
	for _, item := range items {
		if item == candidate {
			return true
		}
	}
	return false
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

	// Stage 2.E: route submission through the approval provider seam.
	// V1 uses the built-in provider implementation behind this router.
	// Non-fatal: ticket is already PENDING in DB; log the error but do not unwind the user response.
	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{
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

	// P2-B (ADR-0014 Layer 3): Non-blocking capability warning.
	// After accepting the request, check if the selected instance size requires
	// hardware features (GPU, SR-IOV, HugePages) that no HEALTHY cluster currently supports.
	// Emits X-Capability-Warning header. Request is NOT rejected — warning only.
	if warning := s.resolveCapabilityWarning(ctx, req.InstanceSizeId.String()); warning != "" {
		c.Header("X-Capability-Warning", warning)
		logger.Warn("capability warning set on VM request",
			zap.String("ticket_id", output.TicketID),
			zap.String("instance_size_id", req.InstanceSizeId.String()),
			zap.String("warning", warning),
		)
	}

	c.JSON(http.StatusAccepted, generated.TicketResponse{
		TicketId: output.TicketID,
		Status:   generated.TicketResponseStatusPENDING,
	})
}

// GetVM handles GET /vms/{vm_id}.
func (s *Server) GetVM(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:read") {
		return
	}

	vm, err := s.client.VM.Get(ctx, vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM", zap.Error(err), zap.String("vm_id", vmID))
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
		logger.Error("failed to check VM namespace visibility", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !visible {
		c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
		return
	}
	vm = s.refreshVMLiveState(ctx, vm)

	// Fetch cluster info for this VM (ADR-0015 §15).
	var clusterEnv, clusterName string
	if vm.ClusterID != "" {
		cl, clErr := s.client.Cluster.Get(ctx, vm.ClusterID)
		if clErr != nil {
			// Non-fatal: log and continue without cluster info.
			logger.Warn("failed to fetch cluster info for VM",
				zap.Error(clErr),
				zap.String("cluster_id", vm.ClusterID),
			)
		} else {
			clusterEnv = string(cl.Environment)
			clusterName = firstNonEmptyString(cl.DisplayName, cl.Name, cl.ID)
		}
	}

	c.JSON(http.StatusOK, vmToAPI(vm, clusterEnv, clusterName, s.loadVMProvisioning(ctx, vm)))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// GetVMRequestPrefill handles GET /vms/{vm_id}/request-prefill.
// Returns reusable CREATE-request context for "request similar VM" flows.
func (s *Server) GetVMRequestPrefill(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:create") {
		return
	}

	vm, err := s.client.VM.Get(ctx, vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for request prefill", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve VM namespace visibility for request prefill", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	visible, err := s.isNamespaceVisible(ctx, vm.Namespace, visibility)
	if err != nil {
		logger.Error("failed to check VM namespace visibility for request prefill", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !visible {
		c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
		return
	}

	if strings.TrimSpace(vm.TicketID) == "" {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "VM_REQUEST_PREFILL_UNAVAILABLE",
			Message: "vm does not carry reusable create-request context",
		})
		return
	}

	ticket, err := s.client.Ticket.Get(ctx, vm.TicketID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "VM_REQUEST_PREFILL_UNAVAILABLE",
				Message: "ticket backing this vm is not available",
			})
			return
		}
		logger.Error("failed to get ticket for vm request prefill", zap.Error(err), zap.String("vm_id", vmID), zap.String("ticket_id", vm.TicketID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	event, err := s.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "VM_REQUEST_PREFILL_UNAVAILABLE",
				Message: "domain event backing this vm is not available",
			})
			return
		}
		logger.Error("failed to get domain event for vm request prefill", zap.Error(err), zap.String("vm_id", vmID), zap.String("event_id", ticket.EventID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		logger.Warn("failed to decode vm request prefill payload", zap.Error(err), zap.String("vm_id", vmID), zap.String("event_id", event.ID))
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "VM_REQUEST_PREFILL_UNAVAILABLE",
			Message: "request payload is not reusable",
		})
		return
	}

	systemIDByServiceID := s.loadApprovalPrefillSystemByServiceID(ctx, map[string][]byte{
		event.ID: event.Payload,
	})
	prefill := buildApprovalRequestPrefill(payload, systemIDByServiceID)
	if prefill == nil {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "VM_REQUEST_PREFILL_UNAVAILABLE",
			Message: "request payload does not expose a reusable create-request shape",
		})
		return
	}

	c.JSON(http.StatusOK, prefill)
}

// DeleteVM handles DELETE /vms/{vm_id}.
// ADR-0015 §5.D: VM deletion requires a ticket.
// Flow: confirmation gate → create DomainEvent + Ticket (operation_type=DELETE) → return 202.
// Admin approval triggers River job execution via the core ticket service.
func (s *Server) DeleteVM(c *gin.Context, vmID generated.VMID, params generated.DeleteVMParams) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:delete") {
		return
	}
	actor := middleware.GetUserID(ctx)

	// Build use case input from params.
	input := usecase.DeleteVMInput{
		VMID:        vmID,
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
			zap.String("vm_id", vmID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	_ = result.Status // Keep use case output field for backward compatibility.

	// Stage 2.E: route delete submission through the approval provider seam.
	// Same semantics as CreateVMRequest: ticket is already PENDING; router call is best-effort.
	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{
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
func (s *Server) StartVM(c *gin.Context, vmID generated.VMID) {
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
	vm, err := s.client.VM.Get(c.Request.Context(), vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for start", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	vm = s.refreshVMLiveState(c.Request.Context(), vm)

	// State guard: only STOPPED or PAUSED VMs can be started.
	if vm.Status != entvm.StatusSTOPPED && vm.Status != entvm.StatusPAUSED {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot start VM in %s state, must be STOPPED or PAUSED", vm.Status),
		})
		return
	}

	s.handleVMPower(c, vm, "start", domain.EventVMStartRequested)
}

// StopVM handles POST /vms/{vm_id}/stop.
// ISSUE-001: Async via River (ADR-0006). Returns 202 Accepted.
func (s *Server) StopVM(c *gin.Context, vmID generated.VMID) {
	vm := s.mustGetRunningVMForPowerOp(c, vmID, "stop")
	if vm == nil {
		return
	}
	s.handleVMPower(c, vm, "stop", domain.EventVMStopRequested)
}

// RestartVM handles POST /vms/{vm_id}/restart.
// ISSUE-001: Async via River (ADR-0006). Returns 202 Accepted.
func (s *Server) RestartVM(c *gin.Context, vmID generated.VMID) {
	vm := s.mustGetRunningVMForPowerOp(c, vmID, "restart")
	if vm == nil {
		return
	}
	s.handleVMPower(c, vm, "restart", domain.EventVMRestartRequested)
}

func (s *Server) mustGetRunningVMForPowerOp(c *gin.Context, vmID generated.VMID, op string) *ent.VM {
	if !requireGlobalPermission(c, "vm:operate") {
		return nil
	}
	vm, err := s.client.VM.Get(c.Request.Context(), vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return nil
		}
		logger.Error(fmt.Sprintf("failed to get VM for %s", op), zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil
	}
	vm = s.refreshVMLiveState(c.Request.Context(), vm)

	if vm.Status != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot %s VM in %s state, must be RUNNING", op, vm.Status),
		})
		return nil
	}
	return vm
}

func (s *Server) handleVMPower(c *gin.Context, vm *ent.VM, operation string, eventType domain.EventType) {
	ctx := c.Request.Context()

	needsApproval, err := s.requiresPowerApproval(ctx, vm.Namespace, operation)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "NAMESPACE_NOT_REGISTERED",
				Message: "namespace is not registered in namespace_registry",
			})
			return
		}
		logger.Error("failed to evaluate power approval requirement",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("namespace", vm.Namespace),
			zap.String("operation", operation),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !needsApproval {
		s.enqueueVMPowerOp(c, vm, operation, eventType)
		return
	}

	actor := middleware.GetUserID(ctx)
	ticketID, eventID, err := s.createVMPowerApprovalRequest(ctx, vm, actor, operation, eventType)
	if err != nil {
		logger.Error("failed to create power approval request",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("operation", operation),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.approvalRouter != nil {
		if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{
			EventID:   ticketID,
			Requester: actor,
			Action:    "power_" + operation,
		}); routerErr != nil {
			logger.Warn("approval router SubmitForApproval failed for power ticket (already PENDING in DB)",
				zap.String("ticket_id", ticketID),
				zap.Error(routerErr),
			)
		}
	}
	if s.notifier != nil {
		s.notifier.OnTicketSubmitted(ctx, ticketID, actor, "")
	}
	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm."+operation+"_requested", "vm", vm.ID, actor, map[string]interface{}{
			"ticket_id": ticketID,
			"event_id":  eventID,
			"mode":      "approval_required",
		})
	}

	c.JSON(http.StatusAccepted, generated.VMPowerAcceptedResponse{
		EventId:  eventID,
		TicketId: ticketID,
		Status:   generated.VMPowerAcceptedResponseStatus("PENDING_APPROVAL"),
	})
}

func (s *Server) requiresPowerApproval(ctx context.Context, namespace, operation string) (bool, error) {
	env, err := s.resolveNamespaceEnvironment(ctx, namespace)
	if err != nil {
		return false, err
	}
	if s.approvalReqs == nil {
		return false, nil
	}
	return s.approvalReqs.RequiresApproval(ctx, powerOperationToPolicyOperation(operation), env)
}

func (s *Server) createVMPowerApprovalRequest(
	ctx context.Context,
	vm *ent.VM,
	actor string,
	operation string,
	eventType domain.EventType,
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

	payloadBytes, err := json.Marshal(domain.VMPowerPayload{
		VMID:            vm.ID,
		VMName:          vm.Name,
		ClusterID:       vm.ClusterID,
		Namespace:       vm.Namespace,
		RequestVMStatus: string(vm.Status),
		Operation:       operation,
		Actor:           actor,
	})
	if err != nil {
		return "", "", err
	}

	if _, err := tx.DomainEvent.Create().
		SetID(eventUUID.String()).
		SetEventType(string(eventType)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx); err != nil {
		return "", "", err
	}

	if _, err := tx.Ticket.Create().
		SetID(ticketUUID.String()).
		SetEventID(eventUUID.String()).
		SetOperationType(entticket.OperationTypePOWER).
		SetStatus(entticket.StatusPENDING).
		SetRequester(actor).
		SetReason("vm " + operation + " request").
		Save(ctx); err != nil {
		return "", "", err
	}

	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return ticketUUID.String(), eventUUID.String(), nil
}

func powerOperationToPolicyOperation(operation string) approvalpolicy.Operation {
	switch operation {
	case "start":
		return approvalpolicy.OperationSTART_VM
	case "stop":
		return approvalpolicy.OperationSTOP_VM
	case "restart":
		return approvalpolicy.OperationRESTART_VM
	default:
		return approvalpolicy.OperationSTART_VM
	}
}

// enqueueVMPowerOp creates a DomainEvent, enqueues a River job, and returns 202 Accepted.
// Shared by StartVM, StopVM, RestartVM to reduce duplication.
func (s *Server) enqueueVMPowerOp(c *gin.Context, vm *ent.VM, operation string, eventType domain.EventType) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)

	payload := domain.VMPowerPayload{
		VMID:            vm.ID,
		VMName:          vm.Name,
		ClusterID:       vm.ClusterID,
		Namespace:       vm.Namespace,
		RequestVMStatus: string(vm.Status),
		Operation:       operation,
		Actor:           actor,
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
	if s.riverClient == nil {
		logger.Error("riverClient is nil — cannot enqueue VM power job (composition root misconfigured?)", zap.String("vm_id", vm.ID))
		_, _ = s.client.DomainEvent.UpdateOneID(eventID.String()).SetStatus(domainevent.StatusFAILED).Save(ctx)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
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

	c.JSON(http.StatusAccepted, generated.VMPowerAcceptedResponse{
		EventId: eventID.String(),
		Status:  generated.VMPowerAcceptedResponseStatus("ACCEPTED"),
	})
}

// ---- Converter ----

// vmToAPI converts an Ent VM entity to the generated API VM type.
// clusterEnv is the environment string ("test" or "prod") from the associated Cluster;
// pass an empty string when the cluster is not yet assigned.
func vmToAPI(vm *ent.VM, clusterEnv, clusterName string, provisioning *generated.ProvisioningStatus) generated.VM {
	apiVM := generated.VM{
		Id:          vm.ID,
		Name:        vm.Name,
		Namespace:   vm.Namespace,
		Status:      generated.VMStatus(vm.Status),
		ClusterId:   vm.ClusterID,
		ClusterName: clusterName,
		Hostname:    vm.Hostname,
		Instance:    vm.Instance,
		// ServiceId: not directly available (FK edge), omitted if not eagerly loaded
		TicketId:  vm.TicketID,
		CreatedBy: vm.CreatedBy,
		CreatedAt: vm.CreatedAt,
	}
	if vm.Edges.Service != nil {
		apiVM.ServiceId = vm.Edges.Service.ID
	}
	// Populate environment from cluster (ADR-0015 §15).
	if clusterEnv != "" {
		apiVM.Environment = generated.VMEnvironment(clusterEnv)
	}
	apiVM.Provisioning = provisioning
	return apiVM
}

// PowerVM handles POST /vms/{vm_id}/power.
// Unified power-action endpoint — routes to start/stop/restart via the `action` field.
// Follows oapi-codegen ServerInterface contract (ADR-0021).
func (s *Server) PowerVM(c *gin.Context, vmID generated.VMID) {
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}

	var req generated.VMPowerRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	vm, err := s.client.VM.Get(c.Request.Context(), vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for power action", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	vm = s.refreshVMLiveState(c.Request.Context(), vm)

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
		s.handleVMPower(c, vm, "start", domain.EventVMStartRequested)
	case generated.Stop:
		if vm.Status != entvm.StatusRUNNING {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot stop VM in %s state, must be RUNNING", vm.Status),
			})
			return
		}
		s.handleVMPower(c, vm, "stop", domain.EventVMStopRequested)
	case generated.Restart:
		if vm.Status != entvm.StatusRUNNING {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot restart VM in %s state, must be RUNNING", vm.Status),
			})
			return
		}
		s.handleVMPower(c, vm, "restart", domain.EventVMRestartRequested)
	default:
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_POWER_ACTION",
			Message: fmt.Sprintf("unknown power action %q, must be start, stop, or restart", req.Action),
		})
	}
}
