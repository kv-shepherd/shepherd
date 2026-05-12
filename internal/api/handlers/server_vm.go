package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
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
	visibility, err := s.resolveVMQueryVisibility(ctx, c)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while resolving VM visibility", zap.Error(err))
			return
		}
		logger.Error("failed to resolve VM visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.empty() {
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
	query = visibility.apply(query)

	query = applyVMExactFilters(query, params)

	query = query.WithService(func(serviceQuery *ent.ServiceQuery) {
		serviceQuery.WithSystem()
	})

	page, perPage := defaultPagination(params.Page, params.PerPage)
	searchText := strings.TrimSpace(params.Search)
	osNameFilter := strings.TrimSpace(params.OsName)
	ipAddressFilter := strings.TrimSpace(params.IpAddress)
	requiresHydratedFiltering := searchText != "" || osNameFilter != "" || ipAddressFilter != ""

	if requiresHydratedFiltering {
		vms, listErr := query.
			Order(ent.Desc(entvm.FieldCreatedAt)).
			All(ctx)
		if listErr != nil {
			if isRequestContextCanceled(listErr) {
				logger.Debug("request canceled while listing filtered VMs", zap.Error(listErr))
				return
			}
			logger.Error("failed to list filtered VMs", zap.Error(listErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		items := s.hydrateVMListItems(ctx, vms)

		filteredItems := filterHydratedVMItems(items, searchText, osNameFilter, ipAddressFilter)
		pagedItems, total := paginateHydratedVMItems(filteredItems, page, perPage)
		totalPages := (total + perPage - 1) / perPage

		c.JSON(http.StatusOK, generated.VMList{
			Items: pagedItems,
			Pagination: generated.Pagination{
				Page:       page,
				PerPage:    perPage,
				Total:      total,
				TotalPages: totalPages,
			},
		})
		return
	}

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
	items := s.hydrateVMListItems(ctx, vms)

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

func (s *Server) GetVMFilterOptions(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:read") {
		return
	}

	query := s.client.VM.Query()
	visibility, err := s.resolveVMQueryVisibility(ctx, c)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while resolving VM filter visibility", zap.Error(err))
			return
		}
		logger.Error("failed to resolve VM filter visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if visibility.empty() {
		c.JSON(http.StatusOK, generated.VMFilterOptionsResponse{})
		return
	}
	query = visibility.apply(query)
	query = query.
		Where(entvm.StatusNEQ(entvm.StatusDELETING)).
		WithService(func(serviceQuery *ent.ServiceQuery) {
			serviceQuery.WithSystem()
		})

	vms, err := query.
		Order(ent.Desc(entvm.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing VM filter options", zap.Error(err))
			return
		}
		logger.Error("failed to list VM filter options", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := s.hydrateVMListItems(ctx, vms)

	statuses := make([]generated.FilterOption, 0, len(VMStatusValues()))
	for _, status := range VMStatusValues() {
		statuses = append(statuses, generated.FilterOption{
			Value: string(status),
			Label: string(status),
		})
	}

	filterOptions := buildVMFilterOptionsFromItems(items)

	c.JSON(http.StatusOK, generated.VMFilterOptionsResponse{
		Statuses:         statuses,
		Namespaces:       filterOptions.Namespaces,
		Clusters:         filterOptions.Clusters,
		Systems:          filterOptions.Systems,
		Services:         filterOptions.Services,
		OperatingSystems: filterOptions.OperatingSystems,
		IpAddresses:      filterOptions.IPAddresses,
	})
}

func applyVMExactFilters(query *ent.VMQuery, params generated.ListVMsParams) *ent.VMQuery {
	if params.Status != "" {
		query = query.Where(entvm.StatusEQ(entvm.Status(params.Status)))
	}
	if params.Namespace != "" {
		query = query.Where(entvm.NamespaceEQ(params.Namespace))
	}
	if params.ClusterId != "" {
		query = query.Where(entvm.ClusterIDEQ(params.ClusterId))
	}
	if params.SystemId != "" {
		query = query.Where(
			entvm.HasServiceWith(
				entservice.HasSystemWith(entsystem.IDEQ(params.SystemId)),
			),
		)
	}
	if params.ServiceId != "" {
		query = query.Where(
			entvm.HasServiceWith(entservice.IDEQ(params.ServiceId)),
		)
	}
	// Exclude VM tombstones (DELETING status = K8s deleted, DB hard-delete failed)
	// unless the user explicitly filters by DELETING status.
	if params.Status == "" || entvm.Status(params.Status) != entvm.StatusDELETING {
		query = query.Where(entvm.StatusNEQ(entvm.StatusDELETING))
	}
	return query
}

func (s *Server) hydrateVMListItems(ctx context.Context, vms []*ent.VM) []generated.VM {
	vms = s.refreshVMLiveStates(ctx, vms)
	liveVMByID := s.loadObservedLiveVMsByID(ctx, vms)

	clusterIDs := make([]string, 0)
	clusterIDSet := make(map[string]struct{})
	for _, vm := range vms {
		if vm.ClusterID == "" {
			continue
		}
		if _, seen := clusterIDSet[vm.ClusterID]; seen {
			continue
		}
		clusterIDSet[vm.ClusterID] = struct{}{}
		clusterIDs = append(clusterIDs, vm.ClusterID)
	}

	clusterEnvMap := make(map[string]string)
	clusterNameMap := make(map[string]string)
	if len(clusterIDs) > 0 {
		clusters, err := s.client.Cluster.Query().
			Where(entcluster.IDIn(clusterIDs...)).
			Select(entcluster.FieldID, entcluster.FieldEnvironment, entcluster.FieldDisplayName, entcluster.FieldName).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch cluster info for VM list", zap.Error(err))
		} else {
			for _, cl := range clusters {
				clusterEnvMap[cl.ID] = string(cl.Environment)
				clusterNameMap[cl.ID] = firstNonEmptyString(cl.DisplayName, cl.Name, cl.ID)
			}
		}
	}

	vmSnapshotInfoByTicketID, err := s.loadVMSnapshotInfoByTicketID(ctx, vms)
	if err != nil {
		logger.Warn("failed to load VM template snapshots for list response", zap.Error(err))
	}

	items := make([]generated.VM, 0, len(vms))
	for _, vm := range vms {
		env := clusterEnvMap[vm.ClusterID]
		name := clusterNameMap[vm.ClusterID]
		items = append(items, vmToAPI(vm, env, name, liveVMByID[vm.ID], vmSnapshotInfoByTicketID[vm.TicketID], nil))
	}
	return items
}

func filterHydratedVMItems(items []generated.VM, search, osName, ipAddress string) []generated.VM {
	trimmedSearch := strings.TrimSpace(search)
	trimmedOSName := strings.TrimSpace(osName)
	trimmedIPAddress := strings.TrimSpace(ipAddress)
	if trimmedSearch == "" && trimmedOSName == "" && trimmedIPAddress == "" {
		return items
	}

	filtered := make([]generated.VM, 0, len(items))
	for index := range items {
		item := &items[index]
		if trimmedOSName != "" && !strings.EqualFold(strings.TrimSpace(item.OsName), trimmedOSName) {
			continue
		}
		if trimmedIPAddress != "" && !strings.EqualFold(strings.TrimSpace(item.IpAddress), trimmedIPAddress) {
			continue
		}
		if trimmedSearch != "" && !vmMatchesQuickSearch(*item, trimmedSearch) {
			continue
		}
		filtered = append(filtered, *item)
	}
	return filtered
}

func vmMatchesQuickSearch(item generated.VM, search string) bool {
	search = normalizeQuickSearchText(search)
	if search == "" {
		return true
	}
	fields := []string{
		item.Name,
		item.Hostname,
		item.Namespace,
		item.ClusterName,
		item.ServiceName,
		item.SystemName,
		item.IpAddress,
		item.HostIp,
		item.OsName,
		string(item.Environment),
	}
	for _, field := range fields {
		if strings.Contains(normalizeQuickSearchText(field), search) {
			return true
		}
	}
	return false
}

func normalizeQuickSearchText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	normalized := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return unicode.ToLower(r)
		default:
			return ' '
		}
	}, value)
	return strings.Join(strings.Fields(normalized), " ")
}

func paginateHydratedVMItems(items []generated.VM, page, perPage int) (paged []generated.VM, total int) {
	total = len(items)
	if total == 0 {
		return []generated.VM{}, 0
	}
	offset := (page - 1) * perPage
	if offset >= total {
		return []generated.VM{}, total
	}
	limit := offset + perPage
	if limit > total {
		limit = total
	}
	return items[offset:limit], total
}

type vmFilterOptionsCatalog struct {
	Namespaces       []generated.FilterOption
	Clusters         []generated.FilterOption
	Systems          []generated.FilterOption
	Services         []generated.FilterOption
	OperatingSystems []generated.FilterOption
	IPAddresses      []generated.FilterOption
}

func buildVMFilterOptionsFromItems(items []generated.VM) vmFilterOptionsCatalog {
	options := vmFilterOptionsCatalog{
		Namespaces:       make([]generated.FilterOption, 0),
		Clusters:         make([]generated.FilterOption, 0),
		Systems:          make([]generated.FilterOption, 0),
		Services:         make([]generated.FilterOption, 0),
		OperatingSystems: make([]generated.FilterOption, 0),
		IPAddresses:      make([]generated.FilterOption, 0),
	}

	seenNamespaces := make(map[string]struct{})
	seenClusters := make(map[string]struct{})
	seenSystems := make(map[string]struct{})
	seenServices := make(map[string]struct{})
	seenOperatingSystems := make(map[string]struct{})
	seenIPAddresses := make(map[string]struct{})

	for index := range items {
		item := &items[index]
		if value := strings.TrimSpace(item.Namespace); value != "" {
			if _, seen := seenNamespaces[value]; !seen {
				seenNamespaces[value] = struct{}{}
				options.Namespaces = append(options.Namespaces, generated.FilterOption{
					Value: value,
					Label: value,
					Group: string(item.Environment),
				})
			}
		}
		if value := strings.TrimSpace(item.ClusterId); value != "" {
			if _, seen := seenClusters[value]; !seen {
				seenClusters[value] = struct{}{}
				options.Clusters = append(options.Clusters, generated.FilterOption{
					Value: value,
					Label: firstNonEmptyString(item.ClusterName, value),
					Group: string(item.Environment),
				})
			}
		}
		if value := strings.TrimSpace(item.SystemId); value != "" {
			if _, seen := seenSystems[value]; !seen {
				seenSystems[value] = struct{}{}
				options.Systems = append(options.Systems, generated.FilterOption{
					Value: value,
					Label: firstNonEmptyString(item.SystemName, value),
				})
			}
		}
		if value := strings.TrimSpace(item.ServiceId); value != "" {
			if _, seen := seenServices[value]; !seen {
				seenServices[value] = struct{}{}
				serviceLabel := firstNonEmptyString(item.ServiceName, value)
				if systemLabel := strings.TrimSpace(item.SystemName); systemLabel != "" {
					serviceLabel = fmt.Sprintf("%s / %s", systemLabel, serviceLabel)
				}
				options.Services = append(options.Services, generated.FilterOption{
					Value: value,
					Label: serviceLabel,
					Group: item.SystemName,
				})
			}
		}
		if value := strings.TrimSpace(item.OsName); value != "" {
			normalized := strings.ToLower(value)
			if _, seen := seenOperatingSystems[normalized]; !seen {
				seenOperatingSystems[normalized] = struct{}{}
				options.OperatingSystems = append(options.OperatingSystems, generated.FilterOption{
					Value: value,
					Label: value,
				})
			}
		}
		if value := strings.TrimSpace(item.IpAddress); value != "" {
			if _, seen := seenIPAddresses[value]; !seen {
				seenIPAddresses[value] = struct{}{}
				options.IPAddresses = append(options.IPAddresses, generated.FilterOption{
					Value: value,
					Label: value,
					Group: string(item.Environment),
				})
			}
		}
	}

	sortFilterOptions(options.Namespaces)
	sortFilterOptions(options.Clusters)
	sortFilterOptions(options.Systems)
	sortFilterOptions(options.Services)
	sortFilterOptions(options.OperatingSystems)
	sortFilterOptions(options.IPAddresses)

	return options
}

func sortFilterOptions(options []generated.FilterOption) {
	sort.Slice(options, func(i, j int) bool {
		if options[i].Group != options[j].Group {
			return strings.ToLower(options[i].Group) < strings.ToLower(options[j].Group)
		}
		return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
	})
}

func VMStatusValues() []entvm.Status {
	return []entvm.Status{
		entvm.StatusRUNNING,
		entvm.StatusSTOPPED,
		entvm.StatusSTARTING,
		entvm.StatusSTOPPING,
		entvm.StatusCREATING,
		entvm.StatusPENDING,
		entvm.StatusMIGRATING,
		entvm.StatusPAUSED,
		entvm.StatusFAILED,
		entvm.StatusUNKNOWN,
		entvm.StatusNOT_FOUND,
		entvm.StatusDELETING,
	}
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
	if params.TemplateId != uuid.Nil {
		if selectedTemplate := visibleTemplateByID(templates, params.TemplateId.String()); selectedTemplate != nil {
			sizes = filterInstanceSizesCompatibleWithTemplate(sizes, selectedTemplate)
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
	return visibleTemplateByID(items, templateID) != nil
}

func visibleTemplateByID(items []*ent.Template, templateID string) *ent.Template {
	for _, item := range items {
		if item != nil && item.ID == templateID {
			return item
		}
	}
	return nil
}

func hasVisibleInstanceSize(items []*ent.InstanceSize, sizeID string) bool {
	for _, item := range items {
		if item != nil && item.ID == sizeID {
			return true
		}
	}
	return false
}

func filterInstanceSizesCompatibleWithTemplate(items []*ent.InstanceSize, tpl *ent.Template) []*ent.InstanceSize {
	if tpl == nil || len(items) == 0 {
		return items
	}
	filtered := make([]*ent.InstanceSize, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if service.TemplateInstanceSizeCompatible(tpl.SystemLabels, item.SystemLabels) {
			filtered = append(filtered, item)
		}
	}
	return filtered
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
	if _, ok := s.loadAccessibleService(ctx, c, req.ServiceId.String(), "create"); !ok {
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
		TargetCPUCores: normalizeOptionalTargetFloat64(float64(req.TargetCpuCores)),
		TargetMemoryGi: normalizeOptionalTargetFloat64(float64(req.TargetMemoryGi)),
		TargetDiskGB:   normalizeOptionalTargetInt(req.TargetDiskGb),
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
	// The router owns built-in fallback and any active external provider.
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

	vm, ok := s.loadAccessibleVM(ctx, c, vmID, "view")
	if !ok {
		return
	}
	vm = s.refreshVMLiveState(ctx, vm)
	liveVMByID := s.loadObservedLiveVMsByID(ctx, []*ent.VM{vm})

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

	vmSnapshotInfoByTicketID, snapshotErr := s.loadVMSnapshotInfoByTicketID(ctx, []*ent.VM{vm})
	if snapshotErr != nil {
		logger.Warn("failed to load VM template snapshot for detail response",
			zap.Error(snapshotErr),
			zap.String("vm_id", vm.ID),
		)
	}

	c.JSON(http.StatusOK, vmToAPI(vm, clusterEnv, clusterName, liveVMByID[vm.ID], vmSnapshotInfoByTicketID[vm.TicketID], s.loadVMProvisioning(ctx, vm)))
}

func (s *Server) GetVMManifest(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "platform:admin") {
		return
	}

	vm, err := s.client.VM.Query().
		Where(entvm.IDEQ(vmID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for manifest", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.vmService == nil {
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "MANIFEST_UNAVAILABLE",
			Message: "vm manifest service is unavailable",
		})
		return
	}

	manifestYAML, err := s.vmService.GetVMManifestYAML(ctx, vm.ClusterID, vm.Namespace, vm.Name)
	if err != nil {
		logger.Error("failed to fetch VM manifest yaml", zap.Error(err), zap.String("vm_id", vmID), zap.String("cluster_id", vm.ClusterID))
		c.JSON(http.StatusBadGateway, generated.Error{
			Code:    "MANIFEST_UNAVAILABLE",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, generated.VMManifestResponse{
		VmId:      vm.ID,
		Name:      vm.Name,
		Namespace: vm.Namespace,
		ClusterId: vm.ClusterID,
		Yaml:      manifestYAML,
	})
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

	vm, ok := s.loadAccessibleVM(ctx, c, vmID, "view")
	if !ok {
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
	if _, ok := s.loadAccessibleVM(ctx, c, vmID, "create"); !ok {
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
	vm, ok := s.loadAccessibleVM(c.Request.Context(), c, vmID, "create")
	if !ok {
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
	vm := s.mustGetStoppableVMForPowerOp(c, vmID, "stop")
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
	vm, ok := s.loadAccessibleVM(c.Request.Context(), c, vmID, "create")
	if !ok {
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

func (s *Server) mustGetStoppableVMForPowerOp(c *gin.Context, vmID generated.VMID, op string) *ent.VM {
	if !requireGlobalPermission(c, "vm:operate") {
		return nil
	}
	vm, ok := s.loadAccessibleVM(c.Request.Context(), c, vmID, "create")
	if !ok {
		return nil
	}
	vm = s.refreshVMLiveState(c.Request.Context(), vm)

	if vm.Status != entvm.StatusRUNNING && vm.Status != entvm.StatusSTARTING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "INVALID_STATE_TRANSITION",
			Message: fmt.Sprintf("cannot %s VM in %s state, must be RUNNING or STARTING", op, vm.Status),
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

	existingTicket, err := s.findLatestActiveVMTicket(
		ctx,
		vm.ID,
		entticket.OperationTypePOWER,
		domain.EventVMStartRequested,
		domain.EventVMStopRequested,
		domain.EventVMRestartRequested,
	)
	if err != nil {
		logger.Error("failed to check duplicate power approval request",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("operation", operation),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if existingTicket != nil {
		writeDuplicatePendingVMOperation(c, existingTicket)
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
	snapshotLoader := newBatchSnapshotLoader(s)
	snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vm)
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
		VMID:               vm.ID,
		VMName:             vm.Name,
		ClusterID:          vm.ClusterID,
		ClusterName:        snapshot.ClusterName,
		ClusterEnvironment: snapshot.ClusterEnvironment,
		Namespace:          vm.Namespace,
		SystemID:           snapshot.SystemID,
		SystemName:         snapshot.SystemName,
		ServiceID:          snapshot.ServiceID,
		ServiceName:        snapshot.ServiceName,
		OwnerID:            snapshot.OwnerID,
		OwnerDisplayName:   snapshot.OwnerDisplayName,
		OwnerUsername:      snapshot.OwnerUsername,
		TemplateID:         snapshot.TemplateID,
		TemplateName:       snapshot.TemplateName,
		InstanceSizeID:     snapshot.InstanceSizeID,
		InstanceSizeName:   snapshot.InstanceSizeName,
		RequestVMStatus:    string(vm.Status),
		CurrentCPUCores:    snapshot.CurrentCPUCores,
		CurrentMemoryGi:    snapshot.CurrentMemoryGi,
		CurrentDiskGB:      snapshot.CurrentDiskGB,
		Operation:          operation,
		Actor:              actor,
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
	snapshotLoader := newBatchSnapshotLoader(s)
	snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vm)

	payload := domain.VMPowerPayload{
		VMID:               vm.ID,
		VMName:             vm.Name,
		ClusterID:          vm.ClusterID,
		ClusterName:        snapshot.ClusterName,
		ClusterEnvironment: snapshot.ClusterEnvironment,
		Namespace:          vm.Namespace,
		SystemID:           snapshot.SystemID,
		SystemName:         snapshot.SystemName,
		ServiceID:          snapshot.ServiceID,
		ServiceName:        snapshot.ServiceName,
		OwnerID:            snapshot.OwnerID,
		OwnerDisplayName:   snapshot.OwnerDisplayName,
		OwnerUsername:      snapshot.OwnerUsername,
		TemplateID:         snapshot.TemplateID,
		TemplateName:       snapshot.TemplateName,
		InstanceSizeID:     snapshot.InstanceSizeID,
		InstanceSizeName:   snapshot.InstanceSizeName,
		RequestVMStatus:    string(vm.Status),
		CurrentCPUCores:    snapshot.CurrentCPUCores,
		CurrentMemoryGi:    snapshot.CurrentMemoryGi,
		CurrentDiskGB:      snapshot.CurrentDiskGB,
		Operation:          operation,
		Actor:              actor,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal power payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	eventID, _ := uuid.NewV7()
	atomicWriter := usecase.NewApprovalAtomicWriter(s.pool, s.riverClient)
	if err := atomicWriter.CreatePowerEventAndEnqueue(ctx, usecase.PowerEventInput{
		EventID:       eventID.String(),
		EventType:     string(eventType),
		AggregateType: "vm",
		AggregateID:   vm.ID,
		Payload:       payloadBytes,
		CreatedBy:     actor,
	}); err != nil {
		logger.Error("failed to create and enqueue power domain event", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("event_id", eventID.String()))
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
type vmSnapshotInfo struct {
	OSName    string
	OSVersion string
	OSFamily  string
}

func vmToAPI(vm *ent.VM, clusterEnv, clusterName string, liveVM *domain.VM, snapshot vmSnapshotInfo, provisioning *generated.ProvisioningStatus) generated.VM {
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
		apiVM.ServiceName = vm.Edges.Service.Name
		if vm.Edges.Service.Edges.System != nil {
			apiVM.SystemId = vm.Edges.Service.Edges.System.ID
			apiVM.SystemName = vm.Edges.Service.Edges.System.Name
		}
	}
	if liveVM != nil {
		caps := generated.VMConsoleCapabilities{
			SerialAvailable: liveVM.Spec.AutoattachSerialConsole,
			VncAvailable:    liveVM.Spec.AutoattachGraphicsDevice,
		}
		switch {
		case caps.SerialAvailable:
			preferred := generated.SERIAL
			caps.PreferredConsoleType = &preferred
		case caps.VncAvailable:
			preferred := generated.VNC
			caps.PreferredConsoleType = &preferred
		}
		apiVM.ConsoleCapabilities = &caps
		apiVM.NodeName = liveVM.NodeName
		apiVM.HostIp = liveVM.HostIP
		apiVM.IpAddress = liveVM.IPAddress
		apiVM.CpuCores = float32(liveVM.Spec.CPU)
		apiVM.MemoryGi = float32(liveVM.Spec.MemoryGi)
		apiVM.DiskGb = liveVM.Spec.DiskGB
		apiVM.OsName = liveVM.OSName
		apiVM.OsVersion = liveVM.OSVersion
		apiVM.OsFamily = liveVM.OSFamily
	}
	if apiVM.OsName == "" {
		apiVM.OsName = snapshot.OSName
	}
	if apiVM.OsVersion == "" {
		apiVM.OsVersion = snapshot.OSVersion
	}
	if apiVM.OsFamily == "" {
		apiVM.OsFamily = snapshot.OSFamily
	}
	// Populate environment from cluster (ADR-0015 §15).
	if clusterEnv != "" {
		apiVM.Environment = generated.VMEnvironment(clusterEnv)
	}
	apiVM.Provisioning = provisioning
	return apiVM
}

func (s *Server) loadVMSnapshotInfoByTicketID(ctx context.Context, vms []*ent.VM) (map[string]vmSnapshotInfo, error) {
	ticketIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, vm := range vms {
		if vm == nil || strings.TrimSpace(vm.TicketID) == "" {
			continue
		}
		if _, ok := seen[vm.TicketID]; ok {
			continue
		}
		seen[vm.TicketID] = struct{}{}
		ticketIDs = append(ticketIDs, vm.TicketID)
	}
	if len(ticketIDs) == 0 {
		return map[string]vmSnapshotInfo{}, nil
	}

	tickets, err := s.client.Ticket.Query().
		Where(entticket.IDIn(ticketIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	infoByTicketID := make(map[string]vmSnapshotInfo, len(tickets))
	for _, ticket := range tickets {
		infoByTicketID[ticket.ID] = extractVMSnapshotInfo(ticket.TemplateSnapshot)
	}
	return infoByTicketID, nil
}

func extractVMSnapshotInfo(snapshot map[string]interface{}) vmSnapshotInfo {
	if len(snapshot) == 0 {
		return vmSnapshotInfo{}
	}
	osFamily := lookupJSONString(snapshot, "os_family")
	osVersion := lookupJSONString(snapshot, "os_version")
	return vmSnapshotInfo{
		OSName:    humanizeOSFamily(osFamily),
		OSVersion: osVersion,
		OSFamily:  osFamily,
	}
}

func lookupJSONString(raw map[string]interface{}, paths ...string) string {
	for _, path := range paths {
		segments := strings.Split(path, ".")
		current := interface{}(raw)
		ok := true
		for _, segment := range segments {
			next, match := current.(map[string]interface{})
			if !match {
				ok = false
				break
			}
			current, match = next[segment]
			if !match {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if value, match := current.(string); match {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func humanizeOSFamily(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return strings.TrimSpace(value)
	}
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

	vm, ok := s.loadAccessibleVM(c.Request.Context(), c, vmID, "create")
	if !ok {
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
		if vm.Status != entvm.StatusRUNNING && vm.Status != entvm.StatusSTARTING {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "INVALID_STATE_TRANSITION",
				Message: fmt.Sprintf("cannot stop VM in %s state, must be RUNNING or STARTING", vm.Status),
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
