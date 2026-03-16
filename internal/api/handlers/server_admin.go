package handlers

import (
	"fmt"
	"net/http"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/clusterpolicy"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/predicate"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/api/generated"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

// kubeConfig is a minimal struct for parsing kubeconfig YAML.
// Only the fields we need (clusters[].cluster.server) are defined.
type kubeConfig struct {
	Clusters []struct {
		Cluster struct {
			Server string `yaml:"server"`
		} `yaml:"cluster"`
		Name string `yaml:"name"`
	} `yaml:"clusters"`
}

// parseAPIServerURL extracts the first cluster's server URL from kubeconfig YAML bytes.
func parseAPIServerURL(data []byte) (string, error) {
	var kc kubeConfig
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return "", fmt.Errorf("invalid kubeconfig YAML: %w", err)
	}
	if len(kc.Clusters) == 0 || kc.Clusters[0].Cluster.Server == "" {
		return "", fmt.Errorf("kubeconfig contains no cluster server URL")
	}
	return kc.Clusters[0].Cluster.Server, nil
}

// ListClusters handles GET /admin/clusters.
// Supports coarse feature filtering via ?requires=Feature1,Feature2 and optional
// CREATE-placement compatibility filtering when request context query params are supplied.
// Filtering is done in-memory after DB query: Ent's JSON array column cannot use SQL CONTAINS
// without a jsonb cast + GIN index, which is not yet provisioned (acceptable at current cluster count).
func (s *Server) ListClusters(c *gin.Context, params generated.ListClustersParams) {
	if !requireAnyGlobalPermission(c, "cluster:read", "cluster:manage") {
		return
	}
	ctx := c.Request.Context()

	// Parse required features from ?requires=Feature1,Feature2 (case-insensitive, comma-separated)
	var requiredFeatures []string
	if params.Requires != "" {
		for _, f := range strings.Split(params.Requires, ",") {
			if trimmed := strings.TrimSpace(f); trimmed != "" {
				requiredFeatures = append(requiredFeatures, trimmed)
			}
		}
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage
	query := s.client.Cluster.Query().Order(ent.Desc(cluster.FieldCreatedAt))
	compatibilityInput, hasCompatibilityFilter := buildClusterCompatibilityFilter(params)
	includeIncompatible := params.IncludeIncompatible
	compatibilityByClusterID := make(map[string]generated.ClusterCompatibility)

	var (
		clusters []*ent.Cluster
		total    int
		err      error
	)
	if len(requiredFeatures) == 0 && !hasCompatibilityFilter {
		total, err = query.Clone().Count(ctx)
		if err != nil {
			logger.Error("failed to count clusters", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		clusters, err = query.
			Offset(offset).
			Limit(perPage).
			All(ctx)
		if err != nil {
			logger.Error("failed to list clusters", zap.Error(err), zap.Int("page", page))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	} else {
		// Filter semantics: apply all compatibility/capability filtering before pagination so
		// pagination.total/total_pages reflect the filtered result set.
		allClusters, allErr := query.All(ctx)
		if allErr != nil {
			logger.Error("failed to list clusters with requires filter",
				zap.Error(allErr),
				zap.Strings("requires", requiredFeatures),
			)
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		filtered := make([]*ent.Cluster, 0, len(allClusters))
		for _, cl := range allClusters {
			if provider.HasAllCapabilities(cl.EnabledFeatures, requiredFeatures) {
				filtered = append(filtered, cl)
			}
		}
		if hasCompatibilityFilter {
			validator := service.NewApprovalValidator(s.client).SetVMService(s.vmService)
			results, evalErr := validator.EvaluateClusterCompatibility(ctx, filtered, compatibilityInput)
			if evalErr != nil {
				if appErr, ok := apperrors.IsAppError(evalErr); ok {
					c.JSON(http.StatusBadRequest, generated.Error{
						Code:    appErr.Code,
						Message: appErr.Message,
						Params:  appErr.Params,
					})
					return
				}
				logger.Error("failed to evaluate cluster compatibility filter", zap.Error(evalErr))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			nextFiltered := make([]*ent.Cluster, 0, len(results))
			for _, result := range results {
				if result.Cluster == nil {
					continue
				}
				compatibilityByClusterID[result.Cluster.ID] = generated.ClusterCompatibility{
					Eligible:             result.Eligible,
					ReasonCode:           result.ReasonCode,
					ReasonMessage:        result.ReasonMessage,
					AdvisoryCode:         result.AdvisoryCode,
					AdvisoryMessage:      result.AdvisoryMessage,
					RootVolumeResolution: rootVolumeResolutionToAPI(result.RootVolumeResolution),
				}
				if includeIncompatible || result.Eligible {
					nextFiltered = append(nextFiltered, result.Cluster)
				}
			}
			filtered = nextFiltered
		}
		total = len(filtered)
		if offset >= total {
			clusters = nil
		} else {
			end := offset + perPage
			if end > total {
				end = total
			}
			clusters = filtered[offset:end]
		}
	}

	items := make([]generated.Cluster, 0, len(clusters))
	policyByClusterID := make(map[string]*ent.ClusterPolicy, len(clusters))
	if len(clusters) > 0 {
		clusterIDs := make([]string, 0, len(clusters))
		for _, cl := range clusters {
			if cl != nil && cl.ID != "" {
				clusterIDs = append(clusterIDs, cl.ID)
			}
		}
		if len(clusterIDs) > 0 {
			policies, policyErr := s.client.ClusterPolicy.Query().
				Where(clusterpolicy.ClusterIDIn(clusterIDs...)).
				All(ctx)
			if policyErr != nil {
				logger.Warn("failed to list cluster policy state", zap.Error(policyErr))
			} else {
				for _, policy := range policies {
					if policy != nil && policy.ClusterID != "" {
						policyByClusterID[policy.ClusterID] = policy
					}
				}
			}
		}
	}
	for _, cl := range clusters {
		items = append(items, clusterToAPI(cl, policyByClusterID[cl.ID], compatibilityByClusterID[cl.ID]))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.ClusterList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateCluster handles POST /admin/clusters.
func (s *Server) CreateCluster(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	var req generated.ClusterCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	// Parse api_server_url from kubeconfig (V1: full kubeconfig only).
	apiServerURL, err := parseAPIServerURL(req.Kubeconfig)
	if err != nil {
		logger.Warn("failed to parse kubeconfig", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_KUBECONFIG",
			Message: err.Error(),
		})
		return
	}

	id, _ := uuid.NewV7()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to begin cluster create transaction", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	create := tx.Cluster.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetAPIServerURL(apiServerURL).
		SetEncryptedKubeconfig(req.Kubeconfig). // V1: stored as plaintext; Phase 2 adds AES-256-GCM
		SetStatus(cluster.StatusUNKNOWN).
		SetCreatedBy(actor)
	if req.DisplayName != "" {
		create = create.SetDisplayName(req.DisplayName)
	}
	if req.Environment != "" {
		create = create.SetEnvironment(cluster.Environment(req.Environment))
	}

	cl, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "CLUSTER_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create cluster", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var policy *ent.ClusterPolicy
	if s.clusterPolicy != nil {
		policy, err = s.clusterPolicy.WithClient(tx.Client()).Upsert(
			ctx,
			cl.ID,
			defaultClusterPolicyInput(),
			actor,
		)
		if err != nil {
			logger.Error("failed to create default cluster policy",
				zap.Error(err),
				zap.String("cluster_id", cl.ID),
				zap.String("actor", actor),
			)
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit cluster create transaction",
			zap.Error(err),
			zap.String("cluster_id", cl.ID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "cluster.create", "cluster", cl.ID, actor, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "cluster.create"),
				zap.String("resource_id", cl.ID),
			)
		}
	}

	if s.refreshClusterHealth != nil {
		if err := s.refreshClusterHealth(ctx, cl.ID); err != nil {
			logger.Warn("initial cluster health refresh failed",
				zap.String("cluster_id", cl.ID),
				zap.Error(err),
			)
		} else {
			if refreshed, getErr := s.client.Cluster.Get(ctx, cl.ID); getErr == nil {
				cl = refreshed
			}
		}
	}

	c.JSON(http.StatusCreated, clusterToAPI(cl, policy, generated.ClusterCompatibility{}))
}

// UpdateClusterEnvironment handles PUT /admin/clusters/{cluster_id}/environment.
func (s *Server) UpdateClusterEnvironment(c *gin.Context, clusterID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	var req generated.ClusterEnvironmentUpdate
	if !bindAndValidateJSON(c, &req) {
		return
	}

	cl, err := s.client.Cluster.UpdateOneID(clusterID).
		SetEnvironment(cluster.Environment(req.Environment)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "CLUSTER_NOT_FOUND"})
			return
		}
		logger.Error("failed to update cluster environment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "cluster.update_environment", "cluster", cl.ID, actor, map[string]interface{}{
			"environment": string(req.Environment),
		})
	}

	c.JSON(http.StatusOK, clusterToAPI(cl, nil, generated.ClusterCompatibility{}))
}

// GetClusterPolicy handles GET /admin/clusters/{cluster_id}/policy.
func (s *Server) GetClusterPolicy(c *gin.Context, clusterID string) {
	if !requireAnyGlobalPermission(c, "cluster:read", "cluster:manage") {
		return
	}
	ctx := c.Request.Context()

	if _, err := s.client.Cluster.Get(ctx, clusterID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "CLUSTER_NOT_FOUND"})
			return
		}
		logger.Error("failed to load cluster for policy read", zap.Error(err), zap.String("cluster_id", clusterID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	policy, err := s.client.ClusterPolicy.Query().
		Where(clusterpolicy.ClusterIDEQ(clusterID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "CLUSTER_POLICY_NOT_FOUND"})
			return
		}
		logger.Error("failed to load cluster policy", zap.Error(err), zap.String("cluster_id", clusterID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, clusterPolicyToAPI(policy))
}

// UpsertClusterPolicy handles PUT /admin/clusters/{cluster_id}/policy.
func (s *Server) UpsertClusterPolicy(c *gin.Context, clusterID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "cluster:write", "cluster:manage")
	if !ok {
		return
	}

	var req generated.ClusterPolicyUpsertRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	policy, err := s.clusterPolicy.Upsert(ctx, clusterID, service.ClusterPolicyInput{
		AllowCPUOvercommit:           req.AllowCpuOvercommit,
		AllowMemoryOvercommit:        req.AllowMemoryOvercommit,
		AllowDedicatedCPU:            req.AllowDedicatedCpu,
		AllowGPU:                     req.AllowGpu,
		AllowSRIOV:                   req.AllowSriov,
		AllowHugepages:               req.AllowHugepages,
		AllowedHugepagesSizes:        req.AllowedHugepagesSizes,
		AllowCDIClone:                req.AllowCdiClone,
		AllowedCloneSourceNamespaces: req.AllowedCloneSourceNamespaces,
		AllowedStorageClasses:        req.AllowedStorageClasses,
	}, actor)
	if err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			status := appErr.HTTPStatus
			if appErr.Code == "CLUSTER_NOT_FOUND" {
				status = http.StatusNotFound
			}
			c.JSON(status, generated.Error{Code: appErr.Code, Message: appErr.Message})
			return
		}
		logger.Error("failed to upsert cluster policy",
			zap.Error(err),
			zap.String("cluster_id", clusterID),
			zap.String("actor", actor),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "cluster.upsert_policy", "cluster_policy", policy.ID, actor, map[string]interface{}{
			"cluster_id": clusterID,
		})
	}

	c.JSON(http.StatusOK, clusterPolicyToAPI(policy))
}

// ListTemplates handles GET /templates.
func (s *Server) ListTemplates(c *gin.Context, params generated.ListTemplatesParams) {
	if !requireAnyGlobalPermission(c, "vm:create", "template:read", "template:manage") {
		return
	}
	ctx := c.Request.Context()
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve template visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	query := s.client.Template.Query().
		Where(enttemplate.EnabledEQ(true))
	visibleScopes := visibleTemplateCatalogScopes(visibility)
	if len(visibleScopes) == 0 {
		page, perPage := defaultPagination(params.Page, params.PerPage)
		c.JSON(http.StatusOK, generated.TemplateList{
			Items: []generated.Template{},
			Pagination: generated.Pagination{
				Page:       page,
				PerPage:    perPage,
				Total:      0,
				TotalPages: 0,
			},
		})
		return
	}
	query = query.Where(enttemplate.CatalogScopeIn(visibleScopes...))

	templates, err := query.
		Order(ent.Asc(enttemplate.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	filtered := filterUserRequestableTemplates(templates)

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + perPage
	if end > total {
		end = total
	}

	items := make([]generated.Template, 0, end-offset)
	for _, t := range filtered[offset:end] {
		items = append(items, templateToAPI(t))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.TemplateList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ListInstanceSizes handles GET /instance-sizes.
func (s *Server) ListInstanceSizes(c *gin.Context) {
	if !requireAnyGlobalPermission(c, "vm:create", "instance_size:read", "instance_size:write") {
		return
	}
	ctx := c.Request.Context()
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve instance size visibility", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	visibleScopes := visibleInstanceSizeCatalogScopes(visibility)
	if len(visibleScopes) == 0 {
		c.JSON(http.StatusOK, generated.InstanceSizeList{Items: []generated.InstanceSize{}})
		return
	}

	sizes, err := s.client.InstanceSize.Query().
		Where(
			instancesize.EnabledEQ(true),
			instancesize.CatalogScopeIn(visibleScopes...),
		).
		Order(ent.Asc(instancesize.FieldSortOrder)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list instance sizes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.InstanceSize, 0, len(sizes))
	for _, sz := range sizes {
		items = append(items, instanceSizeToPublicAPI(sz))
	}

	c.JSON(http.StatusOK, generated.InstanceSizeList{
		Items: items,
	})
}

// ListAuditLogs handles GET /audit-logs.
func (s *Server) ListAuditLogs(c *gin.Context, params generated.ListAuditLogsParams) {
	if !requireGlobalPermission(c, "audit:read") {
		return
	}
	ctx := c.Request.Context()

	query := s.client.AuditLog.Query()

	if params.Action != "" {
		query = query.Where(auditlog.ActionEQ(params.Action))
	}
	if params.Actor != "" {
		query = query.Where(auditlog.ActorEQ(params.Actor))
	}
	if params.ResourceType != "" {
		query = query.Where(auditlog.ResourceTypeEQ(params.ResourceType))
	}
	if params.ResourceId != "" {
		query = query.Where(auditlog.ResourceIDEQ(params.ResourceId))
	}
	if decision := strings.TrimSpace(params.ApprovalDecision); decision != "" {
		query = query.Where(
			auditlog.ResourceTypeEQ("approval_ticket"),
			predicate.AuditLog(func(s *entsql.Selector) {
				s.Where(sqljson.ValueEQ(auditlog.FieldDetails, decision, sqljson.Path("decision")))
			}),
		)
	}
	if reasonCode := strings.TrimSpace(params.PlacementReasonCode); reasonCode != "" {
		query = query.Where(
			auditlog.ResourceTypeEQ("approval_ticket"),
			predicate.AuditLog(func(s *entsql.Selector) {
				s.Where(sqljson.ValueEQ(
					auditlog.FieldDetails,
					reasonCode,
					sqljson.Path("placement_evaluation", "reason_code"),
				))
			}),
		)
	}
	if advisoryCode := strings.TrimSpace(params.PlacementAdvisoryCode); advisoryCode != "" {
		query = query.Where(
			auditlog.ResourceTypeEQ("approval_ticket"),
			predicate.AuditLog(func(s *entsql.Selector) {
				s.Where(sqljson.ValueEQ(
					auditlog.FieldDetails,
					advisoryCode,
					sqljson.Path("placement_evaluation", "advisory_code"),
				))
			}),
		)
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	logs, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(auditlog.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list audit logs", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.AuditLog, 0, len(logs))
	for _, l := range logs {
		items = append(items, generated.AuditLog{
			Id:           l.ID,
			Action:       l.Action,
			Actor:        l.Actor,
			ResourceType: l.ResourceType,
			ResourceId:   l.ResourceID,
			Details:      l.Details,
			CreatedAt:    l.CreatedAt,
		})
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.AuditLogList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ---- Converters ----

func clusterToAPI(cl *ent.Cluster, policy *ent.ClusterPolicy, compatibility generated.ClusterCompatibility) generated.Cluster {
	return generated.Cluster{
		Id:                  cl.ID,
		Name:                cl.Name,
		DisplayName:         cl.DisplayName,
		ApiServerUrl:        cl.APIServerURL,
		Status:              generated.ClusterStatus(cl.Status),
		Environment:         generated.ClusterEnvironment(cl.Environment),
		KubevirtVersion:     cl.KubevirtVersion,
		EnabledFeatures:     cl.EnabledFeatures, // P2-A (ADR-0014): expose detected capability features
		StorageClasses:      cl.StorageClasses,
		DefaultStorageClass: cl.DefaultStorageClass,
		PolicyConfigured:    policy != nil,
		PolicySummary:       buildClusterPolicySummary(policy),
		Compatibility:       compatibility,
		Enabled:             cl.Enabled,
		CreatedAt:           cl.CreatedAt,
	}
}

func buildClusterPolicySummary(policy *ent.ClusterPolicy) generated.ClusterPolicySummary {
	if policy == nil {
		return generated.ClusterPolicySummary{Mode: generated.MISSING}
	}

	deniedControls := make([]generated.ClusterPolicySummaryDeniedControls, 0, 7)
	if !policy.AllowCPUOvercommit {
		deniedControls = append(deniedControls, generated.CpuOvercommit)
	}
	if !policy.AllowMemoryOvercommit {
		deniedControls = append(deniedControls, generated.MemoryOvercommit)
	}
	if !policy.AllowDedicatedCPU {
		deniedControls = append(deniedControls, generated.DedicatedCpu)
	}
	if !policy.AllowGpu {
		deniedControls = append(deniedControls, generated.Gpu)
	}
	if !policy.AllowSriov {
		deniedControls = append(deniedControls, generated.Sriov)
	}
	if !policy.AllowHugepages {
		deniedControls = append(deniedControls, generated.Hugepages)
	}
	if !policy.AllowCdiClone {
		deniedControls = append(deniedControls, generated.CdiClone)
	}

	scopedControls := make([]generated.ClusterPolicySummaryScopedControls, 0, 3)
	if len(policy.AllowedHugepagesSizes) > 0 {
		scopedControls = append(scopedControls, generated.HugepagesSizes)
	}
	if len(policy.AllowedCloneSourceNamespaces) > 0 {
		scopedControls = append(scopedControls, generated.CloneSourceNamespaces)
	}
	if len(policy.AllowedStorageClasses) > 0 {
		scopedControls = append(scopedControls, generated.StorageClasses)
	}

	mode := generated.OPEN
	if len(deniedControls) > 0 || len(scopedControls) > 0 {
		mode = generated.GUARDED
	}

	return generated.ClusterPolicySummary{
		Mode:                             mode,
		DeniedControls:                   deniedControls,
		ScopedControls:                   scopedControls,
		AllowedStorageClassCount:         len(policy.AllowedStorageClasses),
		AllowedCloneSourceNamespaceCount: len(policy.AllowedCloneSourceNamespaces),
		AllowedHugepagesSizeCount:        len(policy.AllowedHugepagesSizes),
	}
}

func buildClusterCompatibilityFilter(params generated.ListClustersParams) (service.ApprovalValidationInput, bool) {
	input := service.ApprovalValidationInput{
		TemplateID:     strings.TrimSpace(params.TemplateId),
		InstanceSizeID: strings.TrimSpace(params.InstanceSizeId),
		Namespace:      strings.TrimSpace(params.Namespace),
		StorageClass:   strings.TrimSpace(params.SelectedStorageClass),
		DVAccessModes:  cloneStringSlice(params.SelectedDvAccessModes),
		DVVolumeMode:   strings.TrimSpace(string(params.SelectedDvVolumeMode)),
	}

	if params.CpuRequest != 0 || params.CpuLimit != 0 || params.MemoryRequestGi != 0 || params.MemoryLimitGi != 0 {
		override := service.ApprovalResourceOverride{
			CPURequest:      float64(params.CpuRequest),
			CPULimit:        float64(params.CpuLimit),
			MemoryRequestGi: float64(params.MemoryRequestGi),
			MemoryLimitGi:   float64(params.MemoryLimitGi),
		}
		input.Override = &override
	}

	hasFilter := input.TemplateID != "" ||
		input.InstanceSizeID != "" ||
		input.Namespace != "" ||
		input.StorageClass != "" ||
		len(input.DVAccessModes) > 0 ||
		input.DVVolumeMode != "" ||
		input.Override != nil

	return input, hasFilter
}

func rootVolumeResolutionToAPI(resolution *service.RootVolumeResolution) generated.RootVolumeResolution {
	if resolution == nil {
		return generated.RootVolumeResolution{}
	}

	modeOptions := make([]generated.StorageClaimPropertySet, 0, len(resolution.ModeOptions))
	for _, option := range resolution.ModeOptions {
		modeOptions = append(modeOptions, generated.StorageClaimPropertySet{
			AccessModes: cloneStringSlice(option.AccessModes),
			VolumeMode:  generated.StorageClaimPropertySetVolumeMode(option.VolumeMode),
		})
	}

	return generated.RootVolumeResolution{
		IntentMode:            generated.RootVolumeResolutionIntentMode(resolution.IntentMode),
		State:                 generated.RootVolumeResolutionState(resolution.State),
		Message:               resolution.Message,
		RequestedStorageClass: resolution.RequestedStorageClass,
		EffectiveStorageClass: resolution.EffectiveStorageClass,
		RequestedAccessModes:  cloneStringSlice(resolution.RequestedAccessModes),
		RequestedVolumeMode:   generated.RootVolumeResolutionRequestedVolumeMode(resolution.RequestedVolumeMode),
		EffectiveAccessModes:  cloneStringSlice(resolution.EffectiveAccessModes),
		EffectiveVolumeMode:   generated.RootVolumeResolutionEffectiveVolumeMode(resolution.EffectiveVolumeMode),
		ModeOptions:           modeOptions,
	}
}

func defaultClusterPolicyInput() service.ClusterPolicyInput {
	return service.ClusterPolicyInput{
		AllowCPUOvercommit:    true,
		AllowMemoryOvercommit: true,
		AllowDedicatedCPU:     false,
		AllowGPU:              false,
		AllowSRIOV:            false,
		AllowHugepages:        false,
		AllowCDIClone:         true,
	}
}

func clusterPolicyToAPI(policy *ent.ClusterPolicy) generated.ClusterPolicy {
	return generated.ClusterPolicy{
		Id:                           policy.ID,
		ClusterId:                    policy.ClusterID,
		AllowCpuOvercommit:           policy.AllowCPUOvercommit,
		AllowMemoryOvercommit:        policy.AllowMemoryOvercommit,
		AllowDedicatedCpu:            policy.AllowDedicatedCPU,
		AllowGpu:                     policy.AllowGpu,
		AllowSriov:                   policy.AllowSriov,
		AllowHugepages:               policy.AllowHugepages,
		AllowedHugepagesSizes:        policy.AllowedHugepagesSizes,
		AllowCdiClone:                policy.AllowCdiClone,
		AllowedCloneSourceNamespaces: policy.AllowedCloneSourceNamespaces,
		AllowedStorageClasses:        policy.AllowedStorageClasses,
		CreatedBy:                    policy.CreatedBy,
		UpdatedBy:                    policy.UpdatedBy,
		CreatedAt:                    policy.CreatedAt,
		UpdatedAt:                    policy.UpdatedAt,
	}
}

func templateToAPI(t *ent.Template) generated.Template {
	return generated.Template{
		Id:           t.ID,
		Name:         t.Name,
		DisplayName:  t.DisplayName,
		Description:  t.Description,
		CatalogScope: generated.TemplateCatalogScope(t.CatalogScope),
		SourceType: generated.TemplateSourceType(
			service.EffectiveTemplateSourceType(t.SourceType, t.ImageURL, t.PvcName),
		),
		ImageUrl:     t.ImageURL,
		PvcName:      t.PvcName,
		PvcNamespace: t.PvcNamespace,
		CloudInit:    t.CloudInit,
		OsFamily:     t.OsFamily,
		OsVersion:    t.OsVersion,
		Enabled:      t.Enabled,
	}
}

func filterUserRequestableTemplates(templates []*ent.Template) []*ent.Template {
	if len(templates) == 0 {
		return templates
	}
	filtered := make([]*ent.Template, 0, len(templates))
	for _, tpl := range templates {
		if !service.IsUserRequestableTemplateSource(tpl.SourceType, tpl.ImageURL, tpl.PvcName) {
			continue
		}
		filtered = append(filtered, tpl)
	}
	return filtered
}

func instanceSizeToAPI(sz *ent.InstanceSize) generated.InstanceSize {
	return generated.InstanceSize{
		Id:                sz.ID,
		Name:              sz.Name,
		DisplayName:       sz.DisplayName,
		Description:       sz.Description,
		CatalogScope:      generated.InstanceSizeCatalogScope(sz.CatalogScope),
		CpuCores:          float32(sz.CPUCores),
		CpuRequest:        float32(sz.CPURequest),
		MemoryGi:          float32(sz.MemoryGi),
		MemoryRequestGi:   float32(sz.MemoryRequestGi),
		DiskGb:            sz.DiskGB,
		DedicatedCpu:      sz.DedicatedCPU,
		RequiresGpu:       sz.RequiresGpu,
		RequiresSriov:     sz.RequiresSriov,
		RequiresHugepages: sz.RequiresHugepages,
		HugepagesSize:     sz.HugepagesSize,
		DvAccessModes:     cloneStringSlice(sz.DvAccessModes),
		DvVolumeMode:      generated.InstanceSizeDvVolumeMode(strings.TrimSpace(sz.DvVolumeMode)),
		SortOrder:         sz.SortOrder,
		SpecOverrides:     sz.SpecOverrides,
		Enabled:           sz.Enabled,
	}
}

// instanceSizeToPublicAPI converts an InstanceSize for user-facing endpoints.
// It is identical to instanceSizeToAPI but strips fields that are admin-only
// internals (spec_overrides). The generated InstanceSize type uses
// omitempty+omitzero, so a nil SpecOverrides is omitted from JSON output.
func instanceSizeToPublicAPI(sz *ent.InstanceSize) generated.InstanceSize {
	return generated.InstanceSize{
		Id:                sz.ID,
		Name:              sz.Name,
		DisplayName:       sz.DisplayName,
		Description:       sz.Description,
		CatalogScope:      generated.InstanceSizeCatalogScope(sz.CatalogScope),
		CpuCores:          float32(sz.CPUCores),
		CpuRequest:        float32(sz.CPURequest),
		MemoryGi:          float32(sz.MemoryGi),
		MemoryRequestGi:   float32(sz.MemoryRequestGi),
		DiskGb:            sz.DiskGB,
		DedicatedCpu:      sz.DedicatedCPU,
		RequiresGpu:       sz.RequiresGpu,
		RequiresSriov:     sz.RequiresSriov,
		RequiresHugepages: sz.RequiresHugepages,
		HugepagesSize:     sz.HugepagesSize,
		DvAccessModes:     cloneStringSlice(sz.DvAccessModes),
		DvVolumeMode:      generated.InstanceSizeDvVolumeMode(strings.TrimSpace(sz.DvVolumeMode)),
		SortOrder:         sz.SortOrder,
		// SpecOverrides intentionally omitted: admin-only internal detail.
		Enabled: sz.Enabled,
	}
}
