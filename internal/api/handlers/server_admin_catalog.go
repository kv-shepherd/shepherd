package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/predicate"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	"kv-shepherd.io/shepherd/internal/service"
)

var (
	errAuthProviderInUse             = errors.New("auth provider in use")
	errAuthProviderNotFound          = errors.New("auth provider not found")
	errExternalCohortMappingNotFound = errors.New("external cohort mapping not found")
)

type templateCreateRequest struct {
	Name         string  `json:"name" binding:"required"`
	DisplayName  *string `json:"display_name"`
	Description  *string `json:"description"`
	CatalogScope *string `json:"catalog_scope"`
	// SourceType selects boot mode:
	// - "containerdisk"
	// - "cdi_image_import"
	// - "cdi_pvc_clone"
	SourceType *string `json:"source_type"`
	// ImageURL is the image / import URI.
	// Required for containerdisk and cdi_image_import.
	ImageURL *string `json:"image_url"`
	// PVCName is the source PersistentVolumeClaim name, required when source_type == cdi_pvc_clone.
	PVCName *string `json:"pvc_name"`
	// PVCNamespace is the Kubernetes namespace where the source PVC lives, required when source_type == cdi_pvc_clone.
	PVCNamespace *string `json:"pvc_namespace"`
	// CloudInit is optional cloud-init userdata YAML.
	CloudInit *string `json:"cloud_init"`
	OsFamily  *string `json:"os_family"`
	OsVersion *string `json:"os_version"`
	// SystemLabels are platform-defined compatibility labels. Empty means os:any.
	SystemLabels []string `json:"system_labels"`
	Enabled      *bool    `json:"enabled"`
}

type templateUpdateRequest struct {
	DisplayName  *string `json:"display_name"`
	Description  *string `json:"description"`
	CatalogScope *string `json:"catalog_scope"`
	SourceType   *string `json:"source_type"`
	ImageURL     *string `json:"image_url"`
	PVCName      *string `json:"pvc_name"`
	// PVCNamespace is the Kubernetes namespace where the PVC lives.
	PVCNamespace *string   `json:"pvc_namespace"`
	CloudInit    *string   `json:"cloud_init"`
	OsFamily     *string   `json:"os_family"`
	OsVersion    *string   `json:"os_version"`
	SystemLabels *[]string `json:"system_labels"`
	Enabled      *bool     `json:"enabled"`
}

type instanceSizeCreateRequest struct {
	Name              string                 `json:"name" binding:"required"`
	DisplayName       *string                `json:"display_name"`
	Description       *string                `json:"description"`
	CatalogScope      *string                `json:"catalog_scope"`
	CPUCores          float64                `json:"cpu_cores" binding:"required,min=0.5"`
	MemoryGi          float64                `json:"memory_gi" binding:"required,min=0.5"`
	DiskGb            *int                   `json:"disk_gb"`
	CPURequest        *float64               `json:"cpu_request"`
	MemoryRequestGi   *float64               `json:"memory_request_gi"`
	DedicatedCPU      *bool                  `json:"dedicated_cpu"`
	RequiresGpu       *bool                  `json:"requires_gpu"`
	RequiresSriov     *bool                  `json:"requires_sriov"`
	RequiresHugepages *bool                  `json:"requires_hugepages"`
	HugepagesSize     *string                `json:"hugepages_size"`
	DvAccessModes     []string               `json:"dv_access_modes"`
	DvVolumeMode      *string                `json:"dv_volume_mode"`
	SystemLabels      []string               `json:"system_labels"`
	SpecOverrides     map[string]interface{} `json:"spec_overrides"`
	SortOrder         *int                   `json:"sort_order"`
	Enabled           *bool                  `json:"enabled"`
}

const (
	envTest                   = "test"
	envProd                   = "prod"
	scopeTypeGlobal           = "global"
	scopeTypeSystem           = "system"
	scopeTypeService          = "service"
	templateCatalogScopeAll   = "all"
	templateSearchStateActive = "enabled"
)

type instanceSizeUpdateRequest struct {
	Name              *string                 `json:"name"`
	DisplayName       *string                 `json:"display_name"`
	Description       *string                 `json:"description"`
	CatalogScope      *string                 `json:"catalog_scope"`
	CPUCores          *float64                `json:"cpu_cores"`
	MemoryGi          *float64                `json:"memory_gi"`
	DiskGb            *int                    `json:"disk_gb"`
	CPURequest        *float64                `json:"cpu_request"`
	MemoryRequestGi   *float64                `json:"memory_request_gi"`
	DedicatedCPU      *bool                   `json:"dedicated_cpu"`
	RequiresGpu       *bool                   `json:"requires_gpu"`
	RequiresSriov     *bool                   `json:"requires_sriov"`
	RequiresHugepages *bool                   `json:"requires_hugepages"`
	HugepagesSize     *string                 `json:"hugepages_size"`
	DvAccessModes     *[]string               `json:"dv_access_modes"`
	DvVolumeMode      *string                 `json:"dv_volume_mode"`
	SystemLabels      *[]string               `json:"system_labels"`
	SpecOverrides     *map[string]interface{} `json:"spec_overrides"`
	SortOrder         *int                    `json:"sort_order"`
	Enabled           *bool                   `json:"enabled"`
}

type roleCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	DisplayName *string  `json:"display_name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions" binding:"required"`
	Enabled     *bool    `json:"enabled"`
}

type roleUpdateRequest struct {
	DisplayName *string   `json:"display_name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
	Enabled     *bool     `json:"enabled"`
}

type authProviderCreateRequest struct {
	Name      string                 `json:"name" binding:"required"`
	AuthType  string                 `json:"auth_type" binding:"required"`
	Config    map[string]interface{} `json:"config"`
	Enabled   *bool                  `json:"enabled"`
	SortOrder *int                   `json:"sort_order"`
}

type authProviderUpdateRequest struct {
	Name      *string                 `json:"name"`
	Config    *map[string]interface{} `json:"config"`
	Enabled   *bool                   `json:"enabled"`
	SortOrder *int                    `json:"sort_order"`
}

var permissionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*:[a-z][a-z0-9_]*$`)

const (
	sampleValueTypeUnknown = "unknown"
	sampleValueTypeArray   = "array"
)

var permissionCatalog = map[string]string{
	"builtin_approval:approve":     "Approve or reject built-in approval tasks",
	"builtin_approval:view":        "View built-in approval tasks",
	"audit:read":                   "Read audit logs",
	"auth_provider:configure":      "Create authentication providers",
	"auth_provider:delete":         "Delete authentication providers",
	"auth_provider:mapping_create": "Create external cohort mappings",
	"auth_provider:mapping_delete": "Delete external cohort mappings",
	"auth_provider:mapping_update": "Update external cohort mappings",
	"auth_provider:read":           "Read authentication provider configuration",
	"auth_provider:sync":           "Sync external cohorts for authentication providers",
	"auth_provider:update":         "Update authentication providers",
	"cluster:read":                 "Read clusters",
	"cluster:write":                "Create, update, or delete clusters",
	"instance_size:read":           "Read instance size catalog",
	"instance_size:write":          "Create/update/delete instance sizes",
	"namespace:read":               "Read namespace registry",
	"namespace:write":              "Create/update/delete namespace registry entries",
	"observability:read":           "Read administrator observability dashboards and signals",
	"platform:admin":               "Full platform management capability",
	"rate_limit:manage":            "Manage batch rate-limit policy overrides",
	"rbac:manage":                  "Manage RBAC roles and bindings",
	"rbac:read":                    "Read RBAC roles and permissions",
	"service:create":               "Create services",
	"service:delete":               "Delete services",
	"service:read":                 "Read service information",
	"system:delete":                "Delete systems",
	"system:read":                  "Read system information",
	"system:write":                 "Update system information",
	"ticket:view":                  "View platform work-order tickets",
	"template:read":                "Read template catalog",
	"template:write":               "Create/update/delete templates",
	"user:manage":                  "Manage local JWT users",
	"vm:create":                    "Submit VM creation requests",
	"vm:delete":                    "Submit VM deletion requests",
	"vm:operate":                   "Operate VM power actions",
	"vm:read":                      "Read VM information",
	"vnc:access":                   "Request interactive VM console access, including VNC and serial",
}

func (s *Server) deleteCatalogResourceWithActiveCreateGuard(
	c *gin.Context,
	actor string,
	resourceID string,
	resourceType string,
	logFieldName string,
	notFoundCode string,
	activeConflictCode string,
	activeConflictMessage string,
	auditAction string,
	loadFn func(context.Context) error,
	countActiveFn func(context.Context) (int, error),
	deleteFn func(context.Context) error,
) {
	ctx := c.Request.Context()

	if err := loadFn(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: notFoundCode})
			return
		}
		logger.Error(
			fmt.Sprintf("failed to load admin %s for delete", resourceType),
			zap.Error(err),
			zap.String(logFieldName, resourceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	activeCreateCount, err := countActiveFn(ctx)
	if err != nil {
		logger.Error(
			fmt.Sprintf("failed to check %s active requests", resourceType),
			zap.Error(err),
			zap.String(logFieldName, resourceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if activeCreateCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    activeConflictCode,
			Message: activeConflictMessage,
			Params: map[string]interface{}{
				"active_request_count": activeCreateCount,
			},
		})
		return
	}

	if err := deleteFn(ctx); err != nil {
		logger.Error(
			fmt.Sprintf("failed to delete admin %s", resourceType),
			zap.Error(err),
			zap.String(logFieldName, resourceID),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, auditAction, resourceType, resourceID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// ListAdminTemplates handles GET /admin/templates.
func (s *Server) ListAdminTemplates(c *gin.Context, params generated.ListAdminTemplatesParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "template:read", "template:write")
	if !ok {
		return
	}
	sourceTypeFilter := service.NormalizeTemplateSourceType(string(params.SourceType))
	if rejectInvalidEnumQuery(
		c,
		"source_type",
		sourceTypeFilter,
		service.TemplateSourceContainerDisk,
		service.TemplateSourceCDIImageImport,
		service.TemplateSourceCDIPVCClone,
	) {
		return
	}
	catalogScopeFilter := service.NormalizeCatalogScope(string(params.CatalogScope))
	if rejectInvalidEnumQuery(
		c,
		"catalog_scope",
		catalogScopeFilter,
		service.CatalogScopeUnclassified,
		service.CatalogScopeTest,
		service.CatalogScopeProd,
		service.CatalogScopeAll,
	) {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	query := s.client.Template.Query().
		Order(ent.Desc(enttemplate.FieldUpdatedAt))

	if search := strings.TrimSpace(params.Search); search != "" {
		normalizedSearch := strings.ToLower(search)
		predicates := []predicate.Template{
			enttemplate.IDContainsFold(search),
			enttemplate.NameContainsFold(search),
			enttemplate.DisplayNameContainsFold(search),
			enttemplate.DescriptionContainsFold(search),
			enttemplate.OsFamilyContainsFold(search),
			enttemplate.OsVersionContainsFold(search),
			enttemplate.SourceTypeContainsFold(search),
			enttemplate.ImageURLContainsFold(search),
			enttemplate.PvcNameContainsFold(search),
			enttemplate.PvcNamespaceContainsFold(search),
		}
		switch normalizedSearch {
		case envTest:
			predicates = append(predicates, enttemplate.CatalogScopeEQ(enttemplate.CatalogScopeTest))
		case envProd, "production":
			predicates = append(predicates, enttemplate.CatalogScopeEQ(enttemplate.CatalogScopeProd))
		case templateCatalogScopeAll:
			predicates = append(predicates, enttemplate.CatalogScopeEQ(enttemplate.CatalogScopeAll))
		case "unclassified", "hidden":
			predicates = append(predicates, enttemplate.CatalogScopeEQ(enttemplate.CatalogScopeUnclassified))
		case templateSearchStateActive, "active":
			predicates = append(predicates, enttemplate.EnabledEQ(true))
		case "disabled", "inactive":
			predicates = append(predicates, enttemplate.EnabledEQ(false))
		}
		query = query.Where(enttemplate.Or(predicates...))
	}
	if osFamily := strings.TrimSpace(params.OsFamily); osFamily != "" {
		query = query.Where(enttemplate.OsFamilyContainsFold(osFamily))
	}
	if sourceTypeFilter != "" {
		query = query.Where(enttemplate.SourceTypeEQ(sourceTypeFilter))
	}
	if catalogScopeFilter != "" {
		query = query.Where(enttemplate.CatalogScopeEQ(
			enttemplate.CatalogScope(catalogScopeFilter),
		))
	}
	if _, hasEnabledFilter := c.GetQuery("enabled"); hasEnabledFilter {
		query = query.Where(enttemplate.EnabledEQ(params.Enabled))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items, err := query.Offset(offset).Limit(perPage).All(ctx)
	if err != nil {
		logger.Error("failed to list admin templates", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	resp := make([]generated.Template, 0, len(items))
	for _, item := range items {
		resp = append(resp, templateToAPI(item))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.TemplateList{
		Items: resp,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateAdminTemplate handles POST /admin/templates.
func (s *Server) CreateAdminTemplate(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write")
	if !ok {
		return
	}

	var req templateCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name is required"})
		return
	}

	// ADR-0036: Validate source_type and its dependent fields (including pvc_namespace).
	if err := validateTemplateSource(req.SourceType, req.ImageURL, req.PVCName, req.PVCNamespace); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_TEMPLATE_SOURCE", Message: err.Error()})
		return
	}
	catalogScope, err := normalizeCatalogScopeInput(req.CatalogScope)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CATALOG_SCOPE", Message: err.Error()})
		return
	}
	effectiveCatalogScope := catalogScope
	if effectiveCatalogScope == "" {
		effectiveCatalogScope = service.CatalogScopeUnclassified
	}
	if scopeErr := validateTemplateSourceCatalogScope(req.SourceType, effectiveCatalogScope); scopeErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_TEMPLATE_SOURCE_SCOPE", Message: scopeErr.Error()})
		return
	}
	systemLabels, err := service.NormalizeTemplateSystemLabels(req.SystemLabels)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_SYSTEM_LABELS", Message: err.Error()})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.Template.Create().
		SetID(id.String()).
		SetName(name).
		SetCreatedBy(actor).
		SetSystemLabels(systemLabels)
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v != "" {
			create = create.SetDisplayName(v)
		}
	}
	if req.Description != nil {
		if v := strings.TrimSpace(*req.Description); v != "" {
			create = create.SetDescription(v)
		}
	}
	if catalogScope != "" {
		create = create.SetCatalogScope(enttemplate.CatalogScope(catalogScope))
	}
	if req.SourceType != nil {
		create = create.SetSourceType(service.NormalizeTemplateSourceType(*req.SourceType))
	}
	if req.ImageURL != nil {
		if v := strings.TrimSpace(*req.ImageURL); v != "" {
			create = create.SetImageURL(v)
		}
	}
	if req.PVCName != nil {
		if v := strings.TrimSpace(*req.PVCName); v != "" {
			create = create.SetPvcName(v)
		}
	}
	if req.PVCNamespace != nil {
		if v := strings.TrimSpace(*req.PVCNamespace); v != "" {
			create = create.SetPvcNamespace(v)
		}
	}
	if req.CloudInit != nil {
		// Store as-is; cloud-init YAML is user's responsibility
		create = create.SetCloudInit(*req.CloudInit)
	}
	if req.OsFamily != nil {
		if v := strings.TrimSpace(*req.OsFamily); v != "" {
			create = create.SetOsFamily(v)
		}
	}
	if req.OsVersion != nil {
		if v := strings.TrimSpace(*req.OsVersion); v != "" {
			create = create.SetOsVersion(v)
		}
	}
	if req.Enabled != nil {
		create = create.SetEnabled(*req.Enabled)
	}

	tpl, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "TEMPLATE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create admin template", zap.Error(err), zap.String("name", name))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "template.create", "template", tpl.ID, actor, map[string]interface{}{
			"name":        tpl.Name,
			"source_type": tpl.SourceType,
		})
	}

	c.JSON(http.StatusCreated, templateToAPI(tpl))
}

// UpdateAdminTemplate handles PATCH /admin/templates/{template_id}.
func (s *Server) UpdateAdminTemplate(c *gin.Context, templateID generated.TemplateID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write")
	if !ok {
		return
	}

	var req templateUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	// Finding 3 fix: resolve the effective source configuration by merging the
	// existing DB record with the incoming request fields.
	//
	// Problem: validateTemplateSource(req.SourceType, ...) short-circuits when
	// source_type is nil ("draft template" path), allowing a PATCH that sets
	// pvc_namespace="" without source_type to clear pvc_namespace on a record
	// that already has source_type="cdi_pvc_clone", leaving the row in an inconsistent state.
	//
	// Fix: always validate against the effective (merged) state. If the request
	// does not include a field, fall back to the stored value.
	existingTpl, err := s.client.Template.Get(ctx, templateID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "TEMPLATE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get admin template for update validation", zap.Error(err), zap.String("template_id", templateID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Build resolved (merged) source fields for validation.
	resolvedSourceType := resolveStringPtr(req.SourceType, existingTpl.SourceType)
	resolvedImageURL := resolveStringPtr(req.ImageURL, existingTpl.ImageURL)
	resolvedPVCName := resolveStringPtr(req.PVCName, existingTpl.PvcName)
	resolvedPVCNamespace := resolveStringPtr(req.PVCNamespace, existingTpl.PvcNamespace)
	resolvedCatalogScope := resolveStringPtr(req.CatalogScope, string(existingTpl.CatalogScope))

	// ADR-0036: Validate source_type consistency against the effective state.
	if validateErr := validateTemplateSource(resolvedSourceType, resolvedImageURL, resolvedPVCName, resolvedPVCNamespace); validateErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_TEMPLATE_SOURCE", Message: validateErr.Error()})
		return
	}
	catalogScope, err := normalizeCatalogScopeInput(req.CatalogScope)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CATALOG_SCOPE", Message: err.Error()})
		return
	}
	effectiveCatalogScope := string(existingTpl.CatalogScope)
	if resolvedCatalogScope != nil {
		effectiveCatalogScope = service.NormalizeCatalogScope(*resolvedCatalogScope)
	}
	if effectiveCatalogScope == "" {
		effectiveCatalogScope = service.CatalogScopeUnclassified
	}
	if validateErr := validateTemplateSourceCatalogScope(resolvedSourceType, effectiveCatalogScope); validateErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_TEMPLATE_SOURCE_SCOPE", Message: validateErr.Error()})
		return
	}
	var systemLabels []string
	if req.SystemLabels != nil {
		var labelErr error
		systemLabels, labelErr = service.NormalizeTemplateSystemLabels(*req.SystemLabels)
		if labelErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_SYSTEM_LABELS", Message: labelErr.Error()})
			return
		}
	}

	update := s.client.Template.UpdateOneID(templateID)
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v == "" {
			update = update.ClearDisplayName()
		} else {
			update = update.SetDisplayName(v)
		}
	}
	if req.Description != nil {
		if v := strings.TrimSpace(*req.Description); v == "" {
			update = update.ClearDescription()
		} else {
			update = update.SetDescription(v)
		}
	}
	if req.CatalogScope != nil {
		update = update.SetCatalogScope(enttemplate.CatalogScope(catalogScope))
	}
	if req.SourceType != nil {
		update = update.SetSourceType(service.NormalizeTemplateSourceType(*req.SourceType))
	}
	if req.ImageURL != nil {
		if v := strings.TrimSpace(*req.ImageURL); v == "" {
			update = update.ClearImageURL()
		} else {
			update = update.SetImageURL(v)
		}
	}
	if req.PVCName != nil {
		if v := strings.TrimSpace(*req.PVCName); v == "" {
			update = update.ClearPvcName()
		} else {
			update = update.SetPvcName(v)
		}
	}
	if req.PVCNamespace != nil {
		if v := strings.TrimSpace(*req.PVCNamespace); v == "" {
			update = update.ClearPvcNamespace()
		} else {
			update = update.SetPvcNamespace(v)
		}
	}
	if req.CloudInit != nil {
		if *req.CloudInit == "" {
			update = update.ClearCloudInit()
		} else {
			update = update.SetCloudInit(*req.CloudInit)
		}
	}
	if req.OsFamily != nil {
		if v := strings.TrimSpace(*req.OsFamily); v == "" {
			update = update.ClearOsFamily()
		} else {
			update = update.SetOsFamily(v)
		}
	}
	if req.OsVersion != nil {
		if v := strings.TrimSpace(*req.OsVersion); v == "" {
			update = update.ClearOsVersion()
		} else {
			update = update.SetOsVersion(v)
		}
	}
	if req.SystemLabels != nil {
		update = update.SetSystemLabels(systemLabels)
	}
	if req.Enabled != nil {
		update = update.SetEnabled(*req.Enabled)
	}

	tpl, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "TEMPLATE_NOT_FOUND"})
			return
		}
		logger.Error("failed to update admin template", zap.Error(err), zap.String("template_id", templateID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "template.update", "template", tpl.ID, actor, nil)
	}

	c.JSON(http.StatusOK, templateToAPI(tpl))
}

// DeleteAdminTemplate handles DELETE /admin/templates/{template_id}.
func (s *Server) DeleteAdminTemplate(c *gin.Context, templateID generated.TemplateID) {
	_, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write")
	if !ok {
		return
	}
	s.deleteCatalogResourceWithActiveCreateGuard(
		c,
		actor,
		templateID,
		"template",
		"template_id",
		"TEMPLATE_NOT_FOUND",
		"TEMPLATE_HAS_ACTIVE_REQUESTS",
		"template is referenced by active VM create requests",
		"template.delete",
		func(ctx context.Context) error {
			_, err := s.client.Template.Get(ctx, templateID)
			return err
		},
		func(ctx context.Context) (int, error) {
			return s.countActiveCreateTicketsForTemplate(ctx, templateID)
		},
		func(ctx context.Context) error {
			return s.client.Template.DeleteOneID(templateID).Exec(ctx)
		},
	)
}

// ListAdminInstanceSizes handles GET /admin/instance-sizes.
func (s *Server) ListAdminInstanceSizes(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "instance_size:read", "instance_size:write")
	if !ok {
		return
	}

	sizes, err := s.client.InstanceSize.Query().
		Order(ent.Asc(instancesize.FieldSortOrder), ent.Asc(instancesize.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list admin instance sizes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.InstanceSize, 0, len(sizes))
	for _, sz := range sizes {
		items = append(items, instanceSizeToAPI(sz))
	}

	c.JSON(http.StatusOK, generated.InstanceSizeList{Items: items})
}

// CreateAdminInstanceSize handles POST /admin/instance-sizes.
func (s *Server) CreateAdminInstanceSize(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "instance_size:write")
	if !ok {
		return
	}

	var req instanceSizeCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	if err := validateInstanceSizeCreate(req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}
	catalogScope, err := normalizeCatalogScopeInput(req.CatalogScope)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CATALOG_SCOPE", Message: err.Error()})
		return
	}
	systemLabels, err := service.NormalizeInstanceSizeSystemLabels(req.SystemLabels)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_SYSTEM_LABELS", Message: err.Error()})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.InstanceSize.Create().
		SetID(id.String()).
		SetName(strings.TrimSpace(req.Name)).
		SetCPUCores(req.CPUCores).
		SetMemoryGi(req.MemoryGi).
		SetCreatedBy(actor).
		SetSystemLabels(systemLabels)
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v != "" {
			create = create.SetDisplayName(v)
		}
	}
	if req.Description != nil {
		if v := strings.TrimSpace(*req.Description); v != "" {
			create = create.SetDescription(v)
		}
	}
	if catalogScope != "" {
		create = create.SetCatalogScope(instancesize.CatalogScope(catalogScope))
	}
	if req.DiskGb != nil {
		create = create.SetDiskGB(*req.DiskGb)
	}
	if req.CPURequest != nil {
		create = create.SetCPURequest(*req.CPURequest)
	}
	if req.MemoryRequestGi != nil {
		create = create.SetMemoryRequestGi(*req.MemoryRequestGi)
	}
	if req.DedicatedCPU != nil {
		create = create.SetDedicatedCPU(*req.DedicatedCPU)
	}
	if req.RequiresGpu != nil {
		create = create.SetRequiresGpu(*req.RequiresGpu)
	}
	if req.RequiresSriov != nil {
		create = create.SetRequiresSriov(*req.RequiresSriov)
	}
	if req.RequiresHugepages != nil {
		create = create.SetRequiresHugepages(*req.RequiresHugepages)
	}
	if req.HugepagesSize != nil {
		if v := strings.TrimSpace(*req.HugepagesSize); v != "" {
			create = create.SetHugepagesSize(v)
		}
	}
	if len(req.DvAccessModes) > 0 {
		create = create.SetDvAccessModes(normalizeStringList(req.DvAccessModes))
	}
	if req.DvVolumeMode != nil {
		if v := strings.TrimSpace(*req.DvVolumeMode); v != "" {
			create = create.SetDvVolumeMode(v)
		}
	}
	if req.SpecOverrides != nil {
		// ADR-0036: Validate spec_overrides paths use spec.* prefix.
		if validateErr := service.ValidateSpecOverrides(req.SpecOverrides); validateErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "INVALID_SPEC_OVERRIDES",
				Message: validateErr.Error(),
			})
			return
		}
		// Detect conflicts between spec_overrides and indexed columns (advisory).
		if warnings := service.DetectSpecOverridesConflicts(
			req.CPUCores, req.MemoryGi,
			req.DedicatedCPU != nil && *req.DedicatedCPU,
			req.CPURequest, req.SpecOverrides,
		); len(warnings) > 0 {
			for _, w := range warnings {
				logger.Warn("instance size spec_overrides conflict",
					zap.String("name", req.Name), zap.String("warning", w))
			}
		}
		create = create.SetSpecOverrides(req.SpecOverrides)
	}
	if req.SpecOverrides != nil {
		hints := effectiveInstanceSizeCapabilityHintsFromSpec(
			req.SpecOverrides,
			req.RequiresGpu,
			req.RequiresHugepages,
			req.HugepagesSize,
		)
		create = create.SetRequiresGpu(hints.RequiresGPU)
		create = create.SetRequiresHugepages(hints.RequiresHugepages)
		if v := strings.TrimSpace(hints.HugepagesSize); v != "" {
			create = create.SetHugepagesSize(v)
		}
	}
	if req.SortOrder != nil {
		create = create.SetSortOrder(*req.SortOrder)
	}
	if req.Enabled != nil {
		create = create.SetEnabled(*req.Enabled)
	}

	sz, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "INSTANCE_SIZE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create admin instance size", zap.Error(err), zap.String("name", req.Name))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "instance_size.create", "instance_size", sz.ID, actor, map[string]interface{}{
			"name": sz.Name,
		})
	}

	c.JSON(http.StatusCreated, instanceSizeToAPI(sz))
}

// UpdateAdminInstanceSize handles PATCH /admin/instance-sizes/{instance_size_id}.
func (s *Server) UpdateAdminInstanceSize(c *gin.Context, instanceSizeID generated.InstanceSizeID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "instance_size:write")
	if !ok {
		return
	}

	var req instanceSizeUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	// Finding 2 fix: read the existing record BEFORE validation so that the effective
	// dedicated_cpu value is the merge of (existing DB value) + (request value).
	// Without this, a PATCH that changes only spec_overrides (not dedicated_cpu) would
	// use dedicatedFlag=false even when the stored record has dedicated_cpu=true,
	// causing false-positive rejection of a valid partial update.
	existingSize, err := s.client.InstanceSize.Get(ctx, instanceSizeID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "INSTANCE_SIZE_NOT_FOUND"})
			return
		}
		logger.Error("failed to get admin instance size for update validation", zap.Error(err), zap.String("instance_size_id", instanceSizeID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if validateErr := validateInstanceSizeUpdate(req, existingSize); validateErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: validateErr.Error()})
		return
	}
	catalogScope, err := normalizeCatalogScopeInput(req.CatalogScope)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CATALOG_SCOPE", Message: err.Error()})
		return
	}
	var systemLabels []string
	if req.SystemLabels != nil {
		var labelErr error
		systemLabels, labelErr = service.NormalizeInstanceSizeSystemLabels(*req.SystemLabels)
		if labelErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_SYSTEM_LABELS", Message: labelErr.Error()})
			return
		}
	}

	update := s.client.InstanceSize.UpdateOneID(instanceSizeID)
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name cannot be empty"})
			return
		}
		update = update.SetName(v)
	}
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v == "" {
			update = update.ClearDisplayName()
		} else {
			update = update.SetDisplayName(v)
		}
	}
	if req.Description != nil {
		if v := strings.TrimSpace(*req.Description); v == "" {
			update = update.ClearDescription()
		} else {
			update = update.SetDescription(v)
		}
	}
	if req.CatalogScope != nil {
		update = update.SetCatalogScope(instancesize.CatalogScope(catalogScope))
	}
	if req.CPUCores != nil {
		update = update.SetCPUCores(*req.CPUCores)
	}
	if req.MemoryGi != nil {
		update = update.SetMemoryGi(*req.MemoryGi)
	}
	if req.DiskGb != nil {
		if *req.DiskGb <= 0 {
			update = update.ClearDiskGB()
		} else {
			update = update.SetDiskGB(*req.DiskGb)
		}
	}
	if req.CPURequest != nil {
		if *req.CPURequest <= 0 {
			update = update.ClearCPURequest()
		} else {
			update = update.SetCPURequest(*req.CPURequest)
		}
	}
	if req.MemoryRequestGi != nil {
		if *req.MemoryRequestGi <= 0 {
			update = update.ClearMemoryRequestGi()
		} else {
			update = update.SetMemoryRequestGi(*req.MemoryRequestGi)
		}
	}
	if req.DedicatedCPU != nil {
		update = update.SetDedicatedCPU(*req.DedicatedCPU)
	}
	if req.RequiresGpu != nil {
		update = update.SetRequiresGpu(*req.RequiresGpu)
	}
	if req.RequiresSriov != nil {
		update = update.SetRequiresSriov(*req.RequiresSriov)
	}
	if req.RequiresHugepages != nil {
		update = update.SetRequiresHugepages(*req.RequiresHugepages)
	}
	if req.HugepagesSize != nil {
		if v := strings.TrimSpace(*req.HugepagesSize); v == "" {
			update = update.ClearHugepagesSize()
		} else {
			update = update.SetHugepagesSize(v)
		}
	}
	if req.DvAccessModes != nil {
		if len(*req.DvAccessModes) == 0 {
			update = update.ClearDvAccessModes()
			if req.DvVolumeMode == nil {
				update = update.ClearDvVolumeMode()
			}
		} else {
			update = update.SetDvAccessModes(normalizeStringList(*req.DvAccessModes))
		}
	}
	if req.DvVolumeMode != nil {
		if v := strings.TrimSpace(*req.DvVolumeMode); v == "" {
			update = update.ClearDvVolumeMode()
		} else {
			update = update.SetDvVolumeMode(v)
		}
	}
	if req.SystemLabels != nil {
		update = update.SetSystemLabels(systemLabels)
	}
	if req.SpecOverrides != nil {
		// ADR-0036: Validate spec_overrides paths use spec.* prefix.
		if validateErr := service.ValidateSpecOverrides(*req.SpecOverrides); validateErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "INVALID_SPEC_OVERRIDES",
				Message: validateErr.Error(),
			})
			return
		}
		update = update.SetSpecOverrides(*req.SpecOverrides)
		hints := effectiveInstanceSizeCapabilityHintsFromSpec(
			*req.SpecOverrides,
			req.RequiresGpu,
			req.RequiresHugepages,
			req.HugepagesSize,
		)
		update = update.SetRequiresGpu(hints.RequiresGPU)
		update = update.SetRequiresHugepages(hints.RequiresHugepages)
		if v := strings.TrimSpace(hints.HugepagesSize); v == "" {
			update = update.ClearHugepagesSize()
		} else {
			update = update.SetHugepagesSize(v)
		}
	}
	if req.SortOrder != nil {
		update = update.SetSortOrder(*req.SortOrder)
	}
	if req.Enabled != nil {
		update = update.SetEnabled(*req.Enabled)
	}

	sz, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "INSTANCE_SIZE_NOT_FOUND"})
			return
		}
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "INSTANCE_SIZE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to update admin instance size", zap.Error(err), zap.String("instance_size_id", instanceSizeID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "instance_size.update", "instance_size", sz.ID, actor, nil)
	}

	c.JSON(http.StatusOK, instanceSizeToAPI(sz))
}

// DeleteAdminInstanceSize handles DELETE /admin/instance-sizes/{instance_size_id}.
func (s *Server) DeleteAdminInstanceSize(c *gin.Context, instanceSizeID generated.InstanceSizeID) {
	_, actor, ok := requireActorWithAnyGlobalPermission(c, "instance_size:write")
	if !ok {
		return
	}
	s.deleteCatalogResourceWithActiveCreateGuard(
		c,
		actor,
		instanceSizeID,
		"instance_size",
		"instance_size_id",
		"INSTANCE_SIZE_NOT_FOUND",
		"INSTANCE_SIZE_HAS_ACTIVE_REQUESTS",
		"instance size is referenced by active VM create requests",
		"instance_size.delete",
		func(ctx context.Context) error {
			_, err := s.client.InstanceSize.Get(ctx, instanceSizeID)
			return err
		},
		func(ctx context.Context) (int, error) {
			return s.countActiveCreateTicketsForInstanceSize(ctx, instanceSizeID)
		},
		func(ctx context.Context) error {
			return s.client.InstanceSize.DeleteOneID(instanceSizeID).Exec(ctx)
		},
	)
}

// ListRoles handles GET /admin/roles.
func (s *Server) ListRoles(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	roles, err := s.client.Role.Query().
		Order(ent.Asc(role.FieldBuiltIn), ent.Asc(role.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list roles", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Role, 0, len(roles))
	for _, r := range roles {
		items = append(items, roleToAPI(r))
	}
	c.JSON(http.StatusOK, generated.RoleList{Items: items})
}

// CreateRole handles POST /admin/roles.
func (s *Server) CreateRole(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	var req roleCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name is required"})
		return
	}
	permissions, err := normalizePermissionKeys(req.Permissions)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.Role.Create().
		SetID(id.String()).
		SetName(name).
		SetPermissions(permissions).
		SetBuiltIn(false)
	if req.DisplayName != nil {
		if v := strings.TrimSpace(*req.DisplayName); v != "" {
			create = create.SetDisplayName(v)
		}
	}
	if req.Description != nil {
		if v := strings.TrimSpace(*req.Description); v != "" {
			create = create.SetDescription(v)
		}
	}
	if req.Enabled != nil {
		create = create.SetEnabled(*req.Enabled)
	}

	r, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "ROLE_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create role", zap.Error(err), zap.String("name", name))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.role.create", "role", r.ID, actor, map[string]interface{}{"name": r.Name})
	}

	c.JSON(http.StatusCreated, roleToAPI(r))
}

// UpdateRole handles PATCH /admin/roles/{role_id}.
func (s *Server) UpdateRole(c *gin.Context, roleID generated.RoleID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	var req roleUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	existing, err := s.client.Role.Get(ctx, roleID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if existing.BuiltIn {
		c.JSON(http.StatusForbidden, generated.Error{Code: "BUILTIN_ROLE_IMMUTABLE"})
		return
	}

	invalidateRoleSessions := false
	displayNameValue := strings.TrimSpace(valueOrEmpty(req.DisplayName))
	descriptionValue := strings.TrimSpace(valueOrEmpty(req.Description))
	var permissions []string
	hasPermissionsUpdate := false
	if req.DisplayName != nil {
		displayNameValue = strings.TrimSpace(*req.DisplayName)
	}
	if req.Description != nil {
		descriptionValue = strings.TrimSpace(*req.Description)
	}
	if req.Permissions != nil {
		var normalizeErr error
		permissions, normalizeErr = normalizePermissionKeys(*req.Permissions)
		if normalizeErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: normalizeErr.Error()})
			return
		}
		hasPermissionsUpdate = true
		invalidateRoleSessions = invalidateRoleSessions || !slices.Equal(existing.Permissions, permissions)
	}
	if req.Enabled != nil {
		invalidateRoleSessions = invalidateRoleSessions || existing.Enabled != *req.Enabled
	}

	var r *ent.Role
	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		update := tx.Client().Role.UpdateOneID(roleID)
		if req.DisplayName != nil {
			if displayNameValue == "" {
				update = update.ClearDisplayName()
			} else {
				update = update.SetDisplayName(displayNameValue)
			}
		}
		if req.Description != nil {
			if descriptionValue == "" {
				update = update.ClearDescription()
			} else {
				update = update.SetDescription(descriptionValue)
			}
		}
		if hasPermissionsUpdate {
			update = update.SetPermissions(permissions)
		}
		if req.Enabled != nil {
			update = update.SetEnabled(*req.Enabled)
		}

		var saveErr error
		r, saveErr = update.Save(ctx)
		if saveErr != nil {
			return saveErr
		}
		if !invalidateRoleSessions {
			return nil
		}

		userIDs, err := userIDsForRoleWithClient(ctx, tx.Client(), roleID)
		if err != nil {
			return err
		}
		return s.revokeUsersSessions(ctx, userIDs, "role_updated")
	}); err != nil {
		logger.Error("failed to update role", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.role.update", "role", r.ID, actor, nil)
	}

	c.JSON(http.StatusOK, roleToAPI(r))
}

// DeleteRole handles DELETE /admin/roles/{role_id}.
func (s *Server) DeleteRole(c *gin.Context, roleID generated.RoleID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rbac:manage")
	if !ok {
		return
	}

	r, err := s.client.Role.Get(ctx, roleID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role for delete", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if r.BuiltIn {
		c.JSON(http.StatusForbidden, generated.Error{Code: "BUILTIN_ROLE_IMMUTABLE"})
		return
	}

	bindingCount, err := s.client.RoleBinding.Query().
		Where(rolebinding.HasRoleWith(role.IDEQ(roleID))).
		Count(ctx)
	if err != nil {
		logger.Error("failed to count role bindings", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if bindingCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{Code: "ROLE_IN_USE"})
		return
	}
	mappingExists, err := s.client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.RoleIDEQ(roleID)).
		Exist(ctx)
	if err != nil {
		logger.Error("failed to check external cohort mapping role usage", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if mappingExists {
		c.JSON(http.StatusConflict, generated.Error{Code: "ROLE_IN_USE"})
		return
	}

	if err := s.client.Role.DeleteOneID(roleID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete role", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "rbac.role.delete", "role", roleID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// ListPermissions handles GET /admin/permissions.
func (s *Server) ListPermissions(c *gin.Context) {
	_, _, ok := requireActorWithAnyGlobalPermission(c, "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	keys := make([]string, 0, len(permissionCatalog))
	for k := range permissionCatalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]generated.Permission, 0, len(keys))
	for _, key := range keys {
		items = append(items, generated.Permission{Key: key, Description: permissionCatalog[key]})
	}
	c.JSON(http.StatusOK, generated.PermissionList{Items: items})
}

// ListAuthProviderTypes handles GET /admin/auth-provider-types.
func (s *Server) ListAuthProviderTypes(c *gin.Context) {
	_, _, ok := requireActorWithAnyAuthProviderPermission(c)
	if !ok {
		return
	}

	types := adminglobal.List()
	items := make([]generated.AuthProviderType, 0, len(types))
	for _, tp := range types {
		items = append(items, generated.AuthProviderType{
			Type:         tp.Type,
			DisplayName:  tp.DisplayName,
			Description:  tp.Description,
			BuiltIn:      tp.BuiltIn,
			ConfigSchema: tp.ConfigSchema,
		})
	}

	c.JSON(http.StatusOK, generated.AuthProviderTypeList{Items: items})
}

// ListAuthProviders handles GET /admin/auth-providers.
func (s *Server) ListAuthProviders(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyAuthProviderPermission(c)
	if !ok {
		return
	}
	revealSensitive := hasGlobalPermission(c, "auth_provider:update") ||
		hasGlobalPermission(c, "auth_provider:configure")

	providers, err := s.client.AuthProvider.Query().
		Order(ent.Asc(authprovider.FieldSortOrder), ent.Asc(authprovider.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list auth providers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.AuthProvider, 0, len(providers))
	for _, provider := range providers {
		item, convErr := s.authProviderToAPI(provider, revealSensitive)
		if convErr != nil {
			logger.Error("failed to convert auth provider config", zap.Error(convErr), zap.String("provider_id", provider.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, generated.AuthProviderList{Items: items})
}

// CreateAuthProvider handles POST /admin/auth-providers.
func (s *Server) CreateAuthProvider(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:configure")
	if !ok {
		return
	}

	var req authProviderCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name is required"})
		return
	}
	authType, err := parseAuthProviderType(req.AuthType)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}
	if req.Config == nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "config is required"})
		return
	}
	if configErr := validateAuthProviderConfig(authType, req.Config); configErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: configErr.Error()})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.AuthProvider.Create().
		SetID(id.String()).
		SetName(name).
		SetAuthType(authType).
		SetCreatedBy(actor)
	if req.Config != nil {
		storedConfig, codecErr := s.authProviderConfig.EncryptForStorage(authType, req.Config)
		if codecErr != nil {
			logger.Error("failed to encrypt auth provider config", zap.Error(codecErr), zap.String("auth_type", authType))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		create = create.SetConfig(storedConfig)
	}
	if req.Enabled != nil {
		create = create.SetEnabled(*req.Enabled)
	}
	if req.SortOrder != nil {
		create = create.SetSortOrder(*req.SortOrder)
	}

	provider, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "AUTH_PROVIDER_NAME_EXISTS"})
			return
		}
		logger.Error("failed to create auth provider", zap.Error(err), zap.String("name", name))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.create", "auth_provider", provider.ID, actor, map[string]interface{}{
			"auth_type": provider.AuthType,
		})
	}

	resp, convErr := s.authProviderToAPI(provider, true)
	if convErr != nil {
		logger.Error("failed to convert created auth provider config", zap.Error(convErr), zap.String("provider_id", provider.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// UpdateAuthProvider handles PATCH /admin/auth-providers/{provider_id}.
func (s *Server) UpdateAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:update")
	if !ok {
		return
	}

	var req authProviderUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	existing, err := s.client.AuthProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query auth provider for update", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	nameValue := ""
	if req.Name != nil {
		nameValue = strings.TrimSpace(*req.Name)
		if nameValue == "" {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name cannot be empty"})
			return
		}
	}
	var mergedConfig map[string]interface{}
	if req.Config != nil {
		var mergeErr error
		mergedConfig, mergeErr = s.authProviderConfig.MergeForUpdate(existing.AuthType, existing.Config, *req.Config)
		if mergeErr != nil {
			logger.Error("failed to merge auth provider config update", zap.Error(mergeErr), zap.String("provider_id", providerID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		plainConfig, decryptErr := s.authProviderConfig.DecryptForUse(existing.AuthType, mergedConfig)
		if decryptErr != nil {
			logger.Error("failed to decrypt merged auth provider config", zap.Error(decryptErr), zap.String("provider_id", providerID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if validateErr := validateAuthProviderConfig(existing.AuthType, plainConfig); validateErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: validateErr.Error()})
			return
		}
	}

	revokeLinkedUserSessions := req.Enabled != nil && existing.Enabled && !*req.Enabled
	var provider *ent.AuthProvider
	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		txUpdate := tx.Client().AuthProvider.UpdateOneID(providerID)
		if req.Name != nil {
			txUpdate = txUpdate.SetName(nameValue)
		}
		if req.Config != nil {
			txUpdate = txUpdate.SetConfig(mergedConfig)
		}
		if req.Enabled != nil {
			txUpdate = txUpdate.SetEnabled(*req.Enabled)
		}
		if req.SortOrder != nil {
			txUpdate = txUpdate.SetSortOrder(*req.SortOrder)
		}

		var saveErr error
		provider, saveErr = txUpdate.Save(ctx)
		if saveErr != nil {
			return saveErr
		}
		if !revokeLinkedUserSessions {
			return nil
		}
		linkedUserIDs, err := userIDsForAuthProviderWithClient(ctx, tx.Client(), providerID)
		if err != nil {
			return err
		}
		return s.revokeUsersSessions(ctx, linkedUserIDs, "auth_provider_disabled")
	}); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "AUTH_PROVIDER_NAME_EXISTS"})
			return
		}
		logger.Error("failed to update auth provider", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.update", "auth_provider", provider.ID, actor, nil)
	}

	resp, convErr := s.authProviderToAPI(provider, true)
	if convErr != nil {
		logger.Error("failed to convert updated auth provider config", zap.Error(convErr), zap.String("provider_id", provider.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DeleteAuthProvider handles DELETE /admin/auth-providers/{provider_id}.
func (s *Server) DeleteAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:delete")
	if !ok {
		return
	}

	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		txClient := tx.Client()
		userCount, err := txClient.User.Query().Where(entuser.AuthProviderIDEQ(providerID)).Count(ctx)
		if err != nil {
			return fmt.Errorf("count provider-linked users: %w", err)
		}
		if userCount > 0 {
			return errAuthProviderInUse
		}
		affectedUserIDs, err := cleanupAuthProviderExternalCohortState(ctx, txClient, providerID)
		if err != nil {
			return err
		}
		if len(affectedUserIDs) > 0 {
			if err := s.revokeUsersSessions(ctx, affectedUserIDs, "auth_provider_deleted"); err != nil {
				return err
			}
		}
		if err := txClient.AuthProvider.DeleteOneID(providerID).Exec(ctx); err != nil {
			if ent.IsNotFound(err) {
				return errAuthProviderNotFound
			}
			return fmt.Errorf("delete auth provider: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, errAuthProviderInUse) {
			c.JSON(http.StatusConflict, generated.Error{Code: "AUTH_PROVIDER_IN_USE"})
			return
		}
		if errors.Is(err, errAuthProviderNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete auth provider", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.delete", "auth_provider", providerID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

func cleanupAuthProviderExternalCohortState(ctx context.Context, client *ent.Client, providerID string) ([]string, error) {
	grants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query external cohort grants for provider cleanup: %w", err)
	}
	affectedUserIDs := make([]string, 0, len(grants))
	for _, grant := range grants {
		affectedUserIDs = appendExternalCohortGrantUserID(affectedUserIDs, grant)
		if err := deleteExternalCohortGrantAndManagedRoleBinding(ctx, client, grant); err != nil {
			return nil, err
		}
	}
	if _, err := client.ExternalCohortMapping.Delete().
		Where(externalcohortmapping.ProviderIDEQ(providerID)).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("delete external cohort mappings for provider cleanup: %w", err)
	}
	if _, err := client.ExternalCohort.Delete().
		Where(externalcohort.ProviderIDEQ(providerID)).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("delete external cohorts for provider cleanup: %w", err)
	}
	return compactExternalCohortGrantUserIDs(affectedUserIDs), nil
}

// TestAuthProviderConnection handles POST /admin/auth-providers/{provider_id}/test-connection.
func (s *Server) TestAuthProviderConnection(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:configure")
	if !ok {
		return
	}

	provider, err := s.client.AuthProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for test connection", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(provider.AuthType, provider.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for test connection", zap.Error(cfgErr), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	okConn, message, err := testAuthProviderConnection(ctx, provider.AuthType, runtimeConfig)
	if err != nil {
		logger.Error("failed to test auth provider connection", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.test_connection", "auth_provider", provider.ID, actor, map[string]interface{}{
			"success": okConn,
		})
	}
	c.JSON(http.StatusOK, generated.AuthProviderConnectionTestResult{
		Success: okConn,
		Message: message,
	})
}

// GetAuthProviderSample handles GET /admin/auth-providers/{provider_id}/sample.
func (s *Server) GetAuthProviderSample(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyAuthProviderPermission(c)
	if !ok {
		return
	}

	provider, err := s.client.AuthProvider.Get(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for sample", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	syncedCohorts, err := s.client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ(providerID)).
		Order(ent.Asc(externalcohort.FieldCohortKind), ent.Asc(externalcohort.FieldDisplayName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to query synced cohorts for sample", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(provider.AuthType, provider.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for sample fields", zap.Error(cfgErr), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	observedUsers, err := s.client.User.Query().
		Where(entuser.AuthProviderIDEQ(providerID)).
		WithDirectoryProfile().
		Order(ent.Desc(entuser.FieldLastLoginAt), ent.Desc(entuser.FieldUpdatedAt)).
		Limit(50).
		All(ctx)
	if err != nil {
		logger.Error("failed to query observed auth provider users for sample fields", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	fields, err := buildAuthProviderSampleFields(ctx, provider.AuthType, runtimeConfig, syncedCohorts, observedUsers)
	if err != nil {
		logger.Error("failed to build auth provider sample fields", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, generated.AuthProviderSampleResponse{
		ProviderId: providerID,
		Fields:     fields,
	})
}

// ListAuthProviderCohorts handles GET /admin/auth-providers/{provider_id}/cohorts.
func (s *Server) ListAuthProviderCohorts(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyAuthProviderPermission(c)
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for cohort list", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	cohorts, err := s.listExternalCohorts(ctx, providerID)
	if err != nil {
		logger.Error("failed to list external cohorts", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.ExternalCohort, 0, len(cohorts))
	for _, cohort := range cohorts {
		items = append(items, externalCohortToAPI(cohort))
	}
	c.JSON(http.StatusOK, generated.ExternalCohortList{Items: items})
}

// SyncAuthProviderCohorts handles POST /admin/auth-providers/{provider_id}/cohorts/sync.
func (s *Server) SyncAuthProviderCohorts(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:sync")
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for cohort sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.ExternalCohortSyncRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	cohortKind := strings.TrimSpace(req.CohortKind)
	sourceField := strings.TrimSpace(req.SourceField)
	if cohortKind == "" || sourceField == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "cohort_kind and source_field are required"})
		return
	}
	cohortKeys := normalizeStringList(req.Cohorts)
	if len(cohortKeys) == 0 {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "cohorts must not be empty"})
		return
	}

	now := time.Now().UTC()
	for _, cohortKey := range cohortKeys {
		existing, err := s.client.ExternalCohort.Query().
			Where(
				externalcohort.ProviderIDEQ(providerID),
				externalcohort.CohortKindEQ(cohortKind),
				externalcohort.CohortKeyEQ(cohortKey),
			).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			logger.Error("failed to query synced cohort", zap.Error(err), zap.String("provider_id", providerID), zap.String("cohort_kind", cohortKind), zap.String("cohort_key", cohortKey))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if ent.IsNotFound(err) {
			id, _ := uuid.NewV7()
			_, err = s.client.ExternalCohort.Create().
				SetID(id.String()).
				SetProviderID(providerID).
				SetCohortKind(cohortKind).
				SetCohortKey(cohortKey).
				SetDisplayName(cohortKey).
				SetSourceField(sourceField).
				SetLastSyncedAt(now).
				Save(ctx)
			if err != nil {
				logger.Error("failed to create synced cohort", zap.Error(err), zap.String("provider_id", providerID), zap.String("cohort_kind", cohortKind), zap.String("cohort_key", cohortKey))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			continue
		}

		if _, err := existing.Update().
			SetDisplayName(cohortKey).
			SetSourceField(sourceField).
			SetLastSyncedAt(now).
			Save(ctx); err != nil {
			logger.Error("failed to update synced cohort", zap.Error(err), zap.String("provider_id", providerID), zap.String("cohort_kind", cohortKind), zap.String("cohort_key", cohortKey))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	syncedCohorts, err := s.listExternalCohorts(ctx, providerID)
	if err != nil {
		logger.Error("failed to list synced cohorts after sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.sync", "auth_provider", providerID, actor, map[string]interface{}{
			"cohort_kind":  cohortKind,
			"source_field": sourceField,
			"cohort_count": len(cohortKeys),
		})
	}

	items := make([]generated.ExternalCohort, 0, len(syncedCohorts))
	for _, cohort := range syncedCohorts {
		items = append(items, externalCohortToAPI(cohort))
	}
	c.JSON(http.StatusOK, generated.ExternalCohortSyncResponse{Items: items})
}

// ListAuthProviderCohortMappings handles GET /admin/auth-providers/{provider_id}/cohort-mappings.
func (s *Server) ListAuthProviderCohortMappings(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyAuthProviderPermission(c)
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for cohort mapping list", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	mappings, err := s.client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.ProviderIDEQ(providerID)).
		Order(ent.Asc(externalcohortmapping.FieldCohortKind), ent.Asc(externalcohortmapping.FieldCohortKey)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list external cohort mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roleNameByID, err := s.roleNameMapByMappings(ctx, mappings)
	if err != nil {
		logger.Error("failed to resolve role names for mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	cohortDisplayNameByRef, err := s.externalCohortDisplayNameMapByProvider(ctx, providerID)
	if err != nil {
		logger.Error("failed to resolve cohort display names for mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.ExternalCohortMapping, 0, len(mappings))
	for _, m := range mappings {
		items = append(items, externalCohortMappingToAPI(m, roleNameByID[m.RoleID], cohortDisplayNameByRef[externalCohortRefKey(m.CohortKind, m.CohortKey)]))
	}
	c.JSON(http.StatusOK, generated.ExternalCohortMappingList{Items: items})
}

// CreateAuthProviderCohortMapping handles POST /admin/auth-providers/{provider_id}/cohort-mappings.
func (s *Server) CreateAuthProviderCohortMapping(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_create")
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for cohort mapping create", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.ExternalCohortMappingCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	cohortKind := strings.TrimSpace(req.CohortKind)
	cohortKey := strings.TrimSpace(req.CohortKey)
	roleID := strings.TrimSpace(req.RoleId)
	if cohortKind == "" || cohortKey == "" || roleID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "cohort_kind, cohort_key, and role_id are required"})
		return
	}
	roleEnt, err := s.client.Role.Get(ctx, roleID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role for cohort mapping create", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !roleEnt.Enabled {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "ROLE_DISABLED",
			Message: "disabled roles cannot be used for external cohort mappings",
		})
		return
	}

	scopeType := strings.TrimSpace(req.ScopeType)
	if scopeType == "" {
		scopeType = scopeTypeGlobal
	}
	scopeID := strings.TrimSpace(req.ScopeId)
	allowedEnvs := normalizeExternalCohortAllowedEnvironmentsCreate(req.AllowedEnvironments)

	cohortDisplayName := strings.TrimSpace(req.CohortDisplayName)
	if cohortDisplayName == "" {
		cohortDisplayName = cohortKey
	}
	if syncErr := s.ensureExternalCohort(ctx, providerID, cohortKind, cohortKey, cohortDisplayName); syncErr != nil {
		logger.Error("failed to ensure external cohort before mapping create", zap.Error(syncErr), zap.String("provider_id", providerID), zap.String("cohort_kind", cohortKind), zap.String("cohort_key", cohortKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	id, _ := uuid.NewV7()
	mapping, err := s.client.ExternalCohortMapping.Create().
		SetID(id.String()).
		SetProviderID(providerID).
		SetCohortKind(cohortKind).
		SetCohortKey(cohortKey).
		SetRoleID(roleID).
		SetScopeType(scopeType).
		SetScopeID(scopeID).
		SetAllowedEnvironments(allowedEnvs).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "EXTERNAL_COHORT_MAPPING_EXISTS"})
			return
		}
		logger.Error("failed to create external cohort mapping", zap.Error(err), zap.String("provider_id", providerID), zap.String("cohort_kind", cohortKind), zap.String("cohort_key", cohortKey))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_create", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": mapping.ID,
		})
	}

	c.JSON(http.StatusCreated, externalCohortMappingToAPI(mapping, roleEnt.Name, cohortDisplayName))
}

// UpdateAuthProviderCohortMapping handles PATCH /admin/auth-providers/{provider_id}/cohort-mappings/{mapping_id}.
func (s *Server) UpdateAuthProviderCohortMapping(c *gin.Context, providerID generated.ProviderID, mappingID generated.MappingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_update")
	if !ok {
		return
	}

	req, fields, ok := bindExternalCohortMappingUpdateJSON(c)
	if !ok {
		return
	}

	mapping, err := s.client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.IDEQ(mappingID), externalcohortmapping.ProviderIDEQ(providerID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "EXTERNAL_COHORT_MAPPING_NOT_FOUND"})
			return
		}
		logger.Error("failed to query external cohort mapping for update", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roleName := ""
	roleID := strings.TrimSpace(req.RoleId)
	if roleID != "" {
		roleEnt, roleErr := s.client.Role.Get(ctx, roleID)
		if roleErr != nil {
			if ent.IsNotFound(roleErr) {
				c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
				return
			}
			logger.Error("failed to query role for external cohort mapping update", zap.Error(roleErr), zap.String("role_id", roleID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if !roleEnt.Enabled {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "ROLE_DISABLED",
				Message: "disabled roles cannot be used for external cohort mappings",
			})
			return
		}
		roleName = roleEnt.Name
	}

	var updated *ent.ExternalCohortMapping
	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		txClient := tx.Client()
		update := txClient.ExternalCohortMapping.UpdateOneID(mapping.ID).
			Where(externalcohortmapping.ProviderIDEQ(providerID))
		if roleID != "" {
			update = update.SetRoleID(roleID)
		}

		if _, exists := fields["scope_type"]; exists {
			scopeType := strings.TrimSpace(req.ScopeType)
			if scopeType == "" {
				scopeType = scopeTypeGlobal
			}
			update = update.SetScopeType(scopeType)
			if _, scopeIDProvided := fields["scope_id"]; !scopeIDProvided && scopeType == scopeTypeGlobal {
				update = update.ClearScopeID()
			}
		}
		if _, exists := fields["scope_id"]; exists {
			if scopeID := strings.TrimSpace(req.ScopeId); scopeID != "" {
				update = update.SetScopeID(scopeID)
			} else {
				update = update.ClearScopeID()
			}
		}
		if _, exists := fields["allowed_environments"]; exists {
			update = update.SetAllowedEnvironments(normalizeExternalCohortAllowedEnvironmentsUpdate(req.AllowedEnvironments))
		}

		saved, err := update.Save(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return errExternalCohortMappingNotFound
			}
			return err
		}
		updated = saved
		affectedUserIDs, err := reconcileExternalCohortGrantsForUpdatedMapping(ctx, txClient, providerID, mapping, updated)
		if err != nil {
			return err
		}
		if len(affectedUserIDs) > 0 {
			return s.revokeUsersSessions(ctx, affectedUserIDs, "external_cohort_mapping_updated")
		}
		return nil
	}); err != nil {
		if errors.Is(err, errExternalCohortMappingNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "EXTERNAL_COHORT_MAPPING_NOT_FOUND"})
			return
		}
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "EXTERNAL_COHORT_MAPPING_EXISTS"})
			return
		}
		logger.Error("failed to update external cohort mapping", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if roleName == "" {
		roleName = roleNameByID(ctx, s.client, updated.RoleID)
	}
	cohortDisplayName := externalCohortDisplayNameByRef(ctx, s.client, providerID, updated.CohortKind, updated.CohortKey)
	if cohortDisplayName == "" {
		cohortDisplayName = updated.CohortKey
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_update", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": updated.ID,
		})
	}

	c.JSON(http.StatusOK, externalCohortMappingToAPI(updated, roleName, cohortDisplayName))
}

func bindExternalCohortMappingUpdateJSON(c *gin.Context) (generated.ExternalCohortMappingUpdateRequest, map[string]json.RawMessage, bool) {
	var req generated.ExternalCohortMappingUpdateRequest
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return req, nil, false
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return req, nil, false
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &fields); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return req, nil, false
	}
	return req, fields, true
}

// DeleteAuthProviderCohortMapping handles DELETE /admin/auth-providers/{provider_id}/cohort-mappings/{mapping_id}.
func (s *Server) DeleteAuthProviderCohortMapping(c *gin.Context, providerID generated.ProviderID, mappingID generated.MappingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_delete")
	if !ok {
		return
	}

	if err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		txClient := tx.Client()
		count, deleteErr := txClient.ExternalCohortMapping.Delete().
			Where(externalcohortmapping.IDEQ(mappingID), externalcohortmapping.ProviderIDEQ(providerID)).
			Exec(ctx)
		if deleteErr != nil {
			return fmt.Errorf("delete external cohort mapping: %w", deleteErr)
		}
		if count == 0 {
			return errExternalCohortMappingNotFound
		}
		affectedUserIDs, err := cleanupExternalCohortGrantsForDeletedMapping(ctx, txClient, providerID, mappingID)
		if err != nil {
			return err
		}
		if len(affectedUserIDs) > 0 {
			return s.revokeUsersSessions(ctx, affectedUserIDs, "external_cohort_mapping_deleted")
		}
		return nil
	}); err != nil {
		if errors.Is(err, errExternalCohortMappingNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "EXTERNAL_COHORT_MAPPING_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete external cohort mapping", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_delete", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": mappingID,
		})
	}

	c.Status(http.StatusNoContent)
}

func cleanupExternalCohortGrantsForDeletedMapping(ctx context.Context, client *ent.Client, providerID, mappingID string) ([]string, error) {
	grants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query external cohort grants for mapping cleanup: %w", err)
	}

	affectedUserIDs := make([]string, 0, len(grants))
	for _, grant := range grants {
		remainingSourceMappingIDs, affected := externalCohortGrantSourceMappingIDsAfterDelete(grant.SourceMappingIds, mappingID)
		if !affected {
			continue
		}
		affectedUserIDs = appendExternalCohortGrantUserID(affectedUserIDs, grant)
		if len(remainingSourceMappingIDs) > 0 {
			if _, err := client.ExternalCohortGrant.UpdateOneID(grant.ID).
				SetSourceMappingIds(remainingSourceMappingIDs).
				Save(ctx); err != nil {
				return nil, fmt.Errorf("update external cohort grant %s source mappings: %w", grant.ID, err)
			}
			continue
		}

		if err := deleteExternalCohortGrantAndManagedRoleBinding(ctx, client, grant); err != nil {
			return nil, err
		}
	}

	return compactExternalCohortGrantUserIDs(affectedUserIDs), nil
}

func reconcileExternalCohortGrantsForUpdatedMapping(
	ctx context.Context,
	client *ent.Client,
	providerID string,
	before, after *ent.ExternalCohortMapping,
) ([]string, error) {
	oldBindingKey := externalCohortMappingBindingKey(before)
	newBindingKey := externalCohortMappingBindingKey(after)
	if oldBindingKey == newBindingKey {
		return nil, nil
	}

	grants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query external cohort grants for mapping update: %w", err)
	}

	affectedUserIDs := make([]string, 0, len(grants))
	now := time.Now().UTC()
	for _, grant := range grants {
		remainingSourceMappingIDs, affected := externalCohortGrantSourceMappingIDsAfterDelete(grant.SourceMappingIds, after.ID)
		if !affected || grant.BindingKey == newBindingKey {
			continue
		}

		affectedUserIDs = appendExternalCohortGrantUserID(affectedUserIDs, grant)
		if err := ensureExternalCohortGrantForUpdatedMapping(ctx, client, grant.UserID, providerID, after, newBindingKey, now); err != nil {
			return nil, err
		}
		if len(remainingSourceMappingIDs) > 0 {
			if _, err := client.ExternalCohortGrant.UpdateOneID(grant.ID).
				SetSourceMappingIds(remainingSourceMappingIDs).
				SetLastAppliedAt(now).
				Save(ctx); err != nil {
				return nil, fmt.Errorf("update external cohort grant %s source mappings after mapping update: %w", grant.ID, err)
			}
			continue
		}

		if err := deleteExternalCohortGrantAndManagedRoleBinding(ctx, client, grant); err != nil {
			return nil, err
		}
	}

	return compactExternalCohortGrantUserIDs(affectedUserIDs), nil
}

func ensureExternalCohortGrantForUpdatedMapping(
	ctx context.Context,
	client *ent.Client,
	userID, providerID string,
	mapping *ent.ExternalCohortMapping,
	bindingKey string,
	now time.Time,
) error {
	target, err := client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userID),
			externalcohortgrant.ProviderIDEQ(providerID),
			externalcohortgrant.BindingKeyEQ(bindingKey),
		).
		Only(ctx)
	if err == nil {
		sourceMappingIDs, changed := externalCohortGrantSourceMappingIDsWithMapping(target.SourceMappingIds, mapping.ID)
		if !changed {
			return nil
		}
		if _, updateErr := client.ExternalCohortGrant.UpdateOneID(target.ID).
			SetSourceMappingIds(sourceMappingIDs).
			SetLastAppliedAt(now).
			Save(ctx); updateErr != nil {
			return fmt.Errorf("merge external cohort grant %s source mappings after mapping update: %w", target.ID, updateErr)
		}
		return nil
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("query target external cohort grant after mapping update: %w", err)
	}

	roleBindingID, err := createExternalCohortManagedRoleBindingForMapping(ctx, client, userID, mapping)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate external cohort grant id: %w", err)
	}
	if _, err := client.ExternalCohortGrant.Create().
		SetID(id.String()).
		SetUserID(userID).
		SetProviderID(providerID).
		SetBindingKey(bindingKey).
		SetRoleBindingID(roleBindingID).
		SetSourceMappingIds([]string{mapping.ID}).
		SetLastAppliedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("create external cohort grant after mapping update: %w", err)
	}
	return nil
}

func createExternalCohortManagedRoleBindingForMapping(
	ctx context.Context,
	client *ent.Client,
	userID string,
	mapping *ent.ExternalCohortMapping,
) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate role binding id: %w", err)
	}

	scopeType := strings.TrimSpace(mapping.ScopeType)
	if scopeType == "" {
		scopeType = scopeTypeGlobal
	}
	create := client.RoleBinding.Create().
		SetID(id.String()).
		SetUserID(userID).
		SetRoleID(mapping.RoleID).
		SetScopeType(scopeType).
		SetCreatedBy(externalCohortRoleBindingActor)
	if scopeID := strings.TrimSpace(mapping.ScopeID); scopeID != "" {
		create = create.SetScopeID(scopeID)
	}
	if allowedEnvironments := normalizedExternalCohortRoleBindingEnvironments(mapping.AllowedEnvironments); len(allowedEnvironments) > 0 {
		create = create.SetAllowedEnvironments(allowedEnvironments)
	}

	binding, err := create.Save(ctx)
	if err != nil {
		return "", fmt.Errorf("create managed role binding after mapping update: %w", err)
	}
	return binding.ID, nil
}

func deleteExternalCohortGrantAndManagedRoleBinding(ctx context.Context, client *ent.Client, grant *ent.ExternalCohortGrant) error {
	if grant == nil {
		return nil
	}
	if err := client.ExternalCohortGrant.DeleteOneID(grant.ID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("delete external cohort grant %s: %w", grant.ID, err)
	}
	if grant.RoleBindingID == "" {
		return nil
	}
	if err := client.RoleBinding.DeleteOneID(grant.RoleBindingID).
		Where(
			rolebinding.CreatedByEQ(externalCohortRoleBindingActor),
			rolebinding.HasUserWith(entuser.IDEQ(grant.UserID)),
		).
		Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("delete managed role binding %s: %w", grant.RoleBindingID, err)
	}
	return nil
}

func appendExternalCohortGrantUserID(userIDs []string, grant *ent.ExternalCohortGrant) []string {
	if grant == nil {
		return userIDs
	}
	userID := strings.TrimSpace(grant.UserID)
	if userID == "" {
		return userIDs
	}
	return append(userIDs, userID)
}

func compactExternalCohortGrantUserIDs(userIDs []string) []string {
	if len(userIDs) == 0 {
		return nil
	}
	normalized := userIDs[:0]
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		normalized = append(normalized, userID)
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func externalCohortMappingBindingKey(mapping *ent.ExternalCohortMapping) string {
	if mapping == nil {
		return ""
	}
	normalizedEnvironments := normalizedExternalCohortRoleBindingEnvironments(mapping.AllowedEnvironments)
	return strings.Join([]string{
		strings.TrimSpace(mapping.RoleID),
		strings.TrimSpace(mapping.ScopeType),
		strings.TrimSpace(mapping.ScopeID),
		strings.Join(normalizedEnvironments, ","),
	}, "|")
}

func normalizedExternalCohortRoleBindingEnvironments(environments []string) []string {
	normalized := make([]string, 0, len(environments))
	for _, environment := range environments {
		environment = strings.TrimSpace(environment)
		if environment == "" {
			continue
		}
		normalized = append(normalized, environment)
	}
	sort.Strings(normalized)
	return normalized
}

func externalCohortGrantSourceMappingIDsAfterDelete(sourceMappingIDs []string, deletedMappingID string) ([]string, bool) {
	remaining := make([]string, 0, len(sourceMappingIDs))
	affected := false
	for _, sourceMappingID := range sourceMappingIDs {
		if sourceMappingID == deletedMappingID {
			affected = true
			continue
		}
		remaining = append(remaining, sourceMappingID)
	}
	return remaining, affected
}

func externalCohortGrantSourceMappingIDsWithMapping(sourceMappingIDs []string, mappingID string) ([]string, bool) {
	seen := make(map[string]struct{}, len(sourceMappingIDs)+1)
	merged := make([]string, 0, len(sourceMappingIDs)+1)
	for _, sourceMappingID := range sourceMappingIDs {
		if sourceMappingID == "" {
			continue
		}
		if _, exists := seen[sourceMappingID]; exists {
			continue
		}
		seen[sourceMappingID] = struct{}{}
		merged = append(merged, sourceMappingID)
	}
	if _, exists := seen[mappingID]; !exists && mappingID != "" {
		merged = append(merged, mappingID)
	}
	sort.Strings(merged)
	return merged, !slices.Equal(sourceMappingIDs, merged)
}

func testAuthProviderConnection(ctx context.Context, authType string, config map[string]interface{}) (ok bool, message string, err error) {
	adapter := adminglobal.Resolve(authType)
	if adapter == nil {
		return false, "no adapter registered", nil
	}
	return adapter.TestConnection(ctx, config)
}

type sampleFieldAccumulator struct {
	valueType string
	values    map[string]struct{}
	uniqueCnt int
}

func buildAuthProviderSampleFields(
	ctx context.Context,
	authType string,
	config map[string]interface{},
	syncedCohorts []*ent.ExternalCohort,
	observedUsers []*ent.User,
) ([]generated.AuthProviderSampleField, error) {
	acc := map[string]*sampleFieldAccumulator{}

	if adapter := adminglobal.Resolve(authType); adapter != nil {
		pluginFields, err := adapter.SampleFields(ctx, config)
		if err != nil {
			return nil, err
		}
		for _, field := range pluginFields {
			slot := &sampleFieldAccumulator{
				valueType: strings.TrimSpace(strings.ToLower(field.ValueType)),
				values:    map[string]struct{}{},
			}
			if slot.valueType == "" {
				slot.valueType = sampleValueTypeUnknown
			}
			for _, val := range field.Sample {
				v := strings.TrimSpace(val)
				if v != "" {
					slot.values[v] = struct{}{}
				}
			}
			slot.uniqueCnt = len(slot.values)
			if field.UniqueCount > slot.uniqueCnt {
				slot.uniqueCnt = field.UniqueCount
			}
			acc[field.Field] = slot
		}
	}
	if claimsMap, ok := config["claims_mapping"].(map[string]interface{}); ok {
		for field := range claimsMap {
			if _, exists := acc[field]; !exists {
				acc[field] = &sampleFieldAccumulator{valueType: "string", values: map[string]struct{}{}}
			}
		}
	}
	if len(syncedCohorts) > 0 {
		cohorts := make([]interface{}, 0, len(syncedCohorts))
		for _, cohort := range syncedCohorts {
			cohorts = append(cohorts, cohort.CohortKind+":"+cohort.CohortKey)
		}
		addSampleValue(acc, "cohorts", cohorts)
	}
	for _, observed := range observedUsers {
		if observed == nil {
			continue
		}
		addSampleValue(acc, "external_id", observed.ExternalID)
		addSampleValue(acc, "username", observed.Username)
		addSampleValue(acc, "display_name", observed.DisplayName)
		addSampleValue(acc, "email", observed.Email)
		addSampleValue(acc, "enabled", observed.Enabled)
		if profile := observed.Edges.DirectoryProfile; profile != nil {
			for fieldName, raw := range profile.Attributes {
				if strings.EqualFold(strings.TrimSpace(fieldName), "external_cohorts") {
					continue
				}
				addSampleValue(acc, fieldName, raw)
			}
		}
	}

	fields := make([]generated.AuthProviderSampleField, 0, len(acc))
	for fieldName, v := range acc {
		values := make([]string, 0, len(v.values))
		for val := range v.values {
			values = append(values, val)
		}
		sort.Strings(values)
		if len(values) > 10 {
			values = values[:10]
		}
		fields = append(fields, generated.AuthProviderSampleField{
			Field:       fieldName,
			ValueType:   generated.AuthProviderSampleFieldValueType(v.valueType),
			UniqueCount: max(v.uniqueCnt, len(v.values)),
			Sample:      values,
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return fields, nil
}

func addSampleValue(acc map[string]*sampleFieldAccumulator, field string, raw interface{}) {
	field = strings.TrimSpace(field)
	if field == "" {
		return
	}
	entry, ok := acc[field]
	if !ok {
		entry = &sampleFieldAccumulator{valueType: detectSampleValueType(raw), values: map[string]struct{}{}}
		acc[field] = entry
	}

	switch typed := raw.(type) {
	case []interface{}:
		if entry.valueType == sampleValueTypeUnknown {
			entry.valueType = sampleValueTypeArray
		}
		for _, item := range typed {
			val := strings.TrimSpace(fmt.Sprint(item))
			if val != "" {
				entry.values[val] = struct{}{}
			}
		}
	case []string:
		if entry.valueType == sampleValueTypeUnknown {
			entry.valueType = sampleValueTypeArray
		}
		for _, item := range typed {
			val := strings.TrimSpace(item)
			if val != "" {
				entry.values[val] = struct{}{}
			}
		}
	case nil:
		return
	default:
		val := strings.TrimSpace(fmt.Sprint(typed))
		if val != "" {
			entry.values[val] = struct{}{}
		}
	}
	entry.uniqueCnt = max(entry.uniqueCnt, len(entry.values))
}

func detectSampleValueType(raw interface{}) string {
	switch raw.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	case map[string]interface{}:
		return "object"
	case []interface{}, []string:
		return sampleValueTypeArray
	default:
		return sampleValueTypeUnknown
	}
}

func normalizeStringList(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}

func normalizeExternalCohortAllowedEnvironmentsCreate(raw []generated.ExternalCohortMappingCreateRequestAllowedEnvironments) []string {
	plain := make([]string, 0, len(raw))
	for _, env := range raw {
		plain = append(plain, string(env))
	}
	return normalizeExternalCohortAllowedEnvironments(plain)
}

func normalizeExternalCohortAllowedEnvironmentsUpdate(raw []generated.ExternalCohortMappingUpdateRequestAllowedEnvironments) []string {
	plain := make([]string, 0, len(raw))
	for _, env := range raw {
		plain = append(plain, string(env))
	}
	return normalizeExternalCohortAllowedEnvironments(plain)
}

func normalizeExternalCohortAllowedEnvironments(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, env := range raw {
		v := strings.ToLower(strings.TrimSpace(env))
		if v != envTest && v != envProd {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (s *Server) ensureExternalCohort(ctx context.Context, providerID, cohortKind, cohortKey, displayName string) error {
	_, err := s.client.ExternalCohort.Query().
		Where(
			externalcohort.ProviderIDEQ(providerID),
			externalcohort.CohortKindEQ(cohortKind),
			externalcohort.CohortKeyEQ(cohortKey),
		).
		Only(ctx)
	if err == nil {
		return nil
	}
	if !ent.IsNotFound(err) {
		return err
	}
	id, _ := uuid.NewV7()
	_, err = s.client.ExternalCohort.Create().
		SetID(id.String()).
		SetProviderID(providerID).
		SetCohortKind(cohortKind).
		SetCohortKey(cohortKey).
		SetDisplayName(displayName).
		SetLastSyncedAt(time.Now().UTC()).
		Save(ctx)
	return err
}

func (s *Server) roleNameMapByMappings(ctx context.Context, mappings []*ent.ExternalCohortMapping) (map[string]string, error) {
	roleIDs := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, m := range mappings {
		if _, exists := seen[m.RoleID]; exists {
			continue
		}
		seen[m.RoleID] = struct{}{}
		roleIDs = append(roleIDs, m.RoleID)
	}
	if len(roleIDs) == 0 {
		return map[string]string{}, nil
	}
	roles, err := s.client.Role.Query().Where(role.IDIn(roleIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(roles))
	for _, r := range roles {
		out[r.ID] = r.Name
	}
	return out, nil
}

func (s *Server) externalCohortDisplayNameMapByProvider(ctx context.Context, providerID string) (map[string]string, error) {
	cohorts, err := s.client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cohorts))
	for _, cohort := range cohorts {
		out[externalCohortRefKey(cohort.CohortKind, cohort.CohortKey)] = cohort.DisplayName
	}
	return out, nil
}

func roleNameByID(ctx context.Context, client *ent.Client, roleID string) string {
	r, err := client.Role.Get(ctx, roleID)
	if err != nil {
		return ""
	}
	return r.Name
}

func externalCohortDisplayNameByRef(ctx context.Context, client *ent.Client, providerID, cohortKind, cohortKey string) string {
	cohort, err := client.ExternalCohort.Query().
		Where(
			externalcohort.ProviderIDEQ(providerID),
			externalcohort.CohortKindEQ(cohortKind),
			externalcohort.CohortKeyEQ(cohortKey),
		).
		Only(ctx)
	if err != nil {
		return ""
	}
	return cohort.DisplayName
}

func externalCohortRefKey(kind, key string) string {
	return strings.TrimSpace(strings.ToLower(kind)) + "|" + strings.TrimSpace(key)
}

func (s *Server) listExternalCohorts(ctx context.Context, providerID string) ([]*ent.ExternalCohort, error) {
	return s.client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ(providerID)).
		Order(ent.Asc(externalcohort.FieldCohortKind), ent.Asc(externalcohort.FieldDisplayName)).
		All(ctx)
}

func externalCohortToAPI(item *ent.ExternalCohort) generated.ExternalCohort {
	last := time.Time{}
	if item.LastSyncedAt != nil {
		last = *item.LastSyncedAt
	}
	return generated.ExternalCohort{
		Id:           item.ID,
		ProviderId:   item.ProviderID,
		CohortKind:   item.CohortKind,
		CohortKey:    item.CohortKey,
		DisplayName:  item.DisplayName,
		SourceField:  item.SourceField,
		LastSyncedAt: last,
	}
}

func externalCohortMappingToAPI(
	m *ent.ExternalCohortMapping,
	roleName, cohortDisplayName string,
) generated.ExternalCohortMapping {
	allowed := make([]generated.ExternalCohortMappingAllowedEnvironments, 0, len(m.AllowedEnvironments))
	for _, env := range m.AllowedEnvironments {
		allowed = append(allowed, generated.ExternalCohortMappingAllowedEnvironments(env))
	}
	return generated.ExternalCohortMapping{
		Id:                  m.ID,
		ProviderId:          m.ProviderID,
		CohortKind:          m.CohortKind,
		CohortKey:           m.CohortKey,
		CohortDisplayName:   cohortDisplayName,
		RoleId:              m.RoleID,
		RoleName:            roleName,
		ScopeType:           m.ScopeType,
		ScopeId:             m.ScopeID,
		AllowedEnvironments: allowed,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
}

func validateInstanceSizeCreate(req instanceSizeCreateRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if req.CPUCores < 0.5 {
		return fmt.Errorf("cpu_cores must be >= 0.5")
	}
	if !service.IsHalfStep(req.CPUCores) {
		return fmt.Errorf("cpu_cores must use 0.5-step values (0.5, 1.0, 1.5, ...)")
	}
	if req.MemoryGi < 0.5 {
		return fmt.Errorf("memory_gi must be >= 0.5")
	}
	if !service.IsHalfStep(req.MemoryGi) {
		return fmt.Errorf("memory_gi must use 0.5-step values (0.5, 1.0, 1.5, ...)")
	}
	if req.DiskGb != nil && *req.DiskGb < 1 {
		return fmt.Errorf("disk_gb must be >= 1")
	}
	if req.CPURequest != nil {
		if *req.CPURequest < 0.5 {
			return fmt.Errorf("cpu_request must be >= 0.5")
		}
		if !service.IsHalfStep(*req.CPURequest) {
			return fmt.Errorf("cpu_request must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if req.MemoryRequestGi != nil {
		if *req.MemoryRequestGi < 0.5 {
			return fmt.Errorf("memory_request_gi must be >= 0.5")
		}
		if !service.IsHalfStep(*req.MemoryRequestGi) {
			return fmt.Errorf("memory_request_gi must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	dedicated := req.DedicatedCPU != nil && *req.DedicatedCPU
	cpuRequest := 0.0
	if req.CPURequest != nil {
		cpuRequest = *req.CPURequest
	}
	memoryRequestGi := 0.0
	if req.MemoryRequestGi != nil {
		memoryRequestGi = *req.MemoryRequestGi
	}
	if err := service.ValidateOvercommit(
		req.CPUCores,
		cpuRequest,
		req.MemoryGi,
		memoryRequestGi,
		dedicated,
	); err != nil {
		return fmt.Errorf("%s", err.Error())
	}
	hints := effectiveInstanceSizeCapabilityHintsFromSpec(
		req.SpecOverrides,
		req.RequiresGpu,
		req.RequiresHugepages,
		req.HugepagesSize,
	)
	requiresHugepages := hints.RequiresHugepages
	hasHugepagesSize := strings.TrimSpace(hints.HugepagesSize) != ""
	if requiresHugepages && !hasHugepagesSize {
		return fmt.Errorf("hugepages_size is required when requires_hugepages is true")
	}
	if err := validateDVStorageMode(req.DvAccessModes, derefString(req.DvVolumeMode)); err != nil {
		return err
	}

	// ADR-0036 constraint: dedicated_cpu indexed field must agree with spec_overrides.
	// Reject mismatches at write time to prevent data inconsistency that would be
	// caught (confusingly) only at approval time.
	if req.SpecOverrides != nil {
		specHasDedicated := service.HasDedicatedCPUInSpecOverrides(req.SpecOverrides)
		if specHasDedicated && !dedicated {
			return fmt.Errorf(
				"spec_overrides sets dedicatedCpuPlacement=true but dedicated_cpu is false; " +
					"set dedicated_cpu=true to keep indexed field consistent with the spec override")
		}
		if dedicated && !specHasDedicated {
			// spec_overrides explicitly sets dedicatedCpuPlacement to false — also an inconsistency.
			// Use the canonical service function that checks BOTH flat dot-notation key AND nested
			// map format (DynamicSchemaForm / Ant Design output).
			if conflictPath, hasConflict := service.SpecOverrideSetsExplicitFalseForDedicatedCPU(req.SpecOverrides); hasConflict {
				return fmt.Errorf(
					"spec_overrides path %q is false but dedicated_cpu is true; "+
						"remove the override or set dedicated_cpu=false", conflictPath)
			}
		}
	}

	return nil
}

// validateInstanceSizeUpdate validates a PATCH request for an InstanceSize.
//
// existingSize is the current database row. Partial updates are validated against
// the effective post-merge values instead of the sparse PATCH body alone.
func validateInstanceSizeUpdate(req instanceSizeUpdateRequest, existingSize *ent.InstanceSize) error {
	if existingSize == nil {
		return fmt.Errorf("existing instance size is required")
	}
	if req.CPUCores != nil {
		if *req.CPUCores < 0.5 {
			return fmt.Errorf("cpu_cores must be >= 0.5")
		}
		if !service.IsHalfStep(*req.CPUCores) {
			return fmt.Errorf("cpu_cores must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if req.MemoryGi != nil {
		if *req.MemoryGi < 0.5 {
			return fmt.Errorf("memory_gi must be >= 0.5")
		}
		if !service.IsHalfStep(*req.MemoryGi) {
			return fmt.Errorf("memory_gi must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if req.CPURequest != nil {
		if *req.CPURequest < 0 {
			return fmt.Errorf("cpu_request must be >= 0")
		}
		if *req.CPURequest > 0 && !service.IsHalfStep(*req.CPURequest) {
			return fmt.Errorf("cpu_request must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if req.MemoryRequestGi != nil {
		if *req.MemoryRequestGi < 0 {
			return fmt.Errorf("memory_request_gi must be >= 0")
		}
		if *req.MemoryRequestGi > 0 && !service.IsHalfStep(*req.MemoryRequestGi) {
			return fmt.Errorf("memory_request_gi must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if req.DiskGb != nil && *req.DiskGb < 0 {
		return fmt.Errorf("disk_gb must be >= 0")
	}
	if err := validateDVStorageMode(derefStringSlice(req.DvAccessModes), derefString(req.DvVolumeMode)); err != nil {
		return err
	}

	effectiveCPUCores := existingSize.CPUCores
	if req.CPUCores != nil {
		effectiveCPUCores = *req.CPUCores
	}
	effectiveMemoryGi := existingSize.MemoryGi
	if req.MemoryGi != nil {
		effectiveMemoryGi = *req.MemoryGi
	}
	effectiveCPURequest := existingSize.CPURequest
	if req.CPURequest != nil {
		if *req.CPURequest <= 0 {
			effectiveCPURequest = 0
		} else {
			effectiveCPURequest = *req.CPURequest
		}
	}
	effectiveMemoryRequestGi := existingSize.MemoryRequestGi
	if req.MemoryRequestGi != nil {
		if *req.MemoryRequestGi <= 0 {
			effectiveMemoryRequestGi = 0
		} else {
			effectiveMemoryRequestGi = *req.MemoryRequestGi
		}
	}
	effectiveDedicated := existingSize.DedicatedCPU
	if req.DedicatedCPU != nil {
		effectiveDedicated = *req.DedicatedCPU
	}
	if err := service.ValidateOvercommit(
		effectiveCPUCores,
		effectiveCPURequest,
		effectiveMemoryGi,
		effectiveMemoryRequestGi,
		effectiveDedicated,
	); err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	// ADR-0036 constraint: dedicated_cpu indexed field must agree with spec_overrides.
	// Use the effective dedicated_cpu: request value if provided, else the existing DB value.
	if req.SpecOverrides != nil {
		specHasDedicated := service.HasDedicatedCPUInSpecOverrides(*req.SpecOverrides)
		if specHasDedicated && !effectiveDedicated {
			return fmt.Errorf(
				"spec_overrides sets dedicatedCpuPlacement=true but dedicated_cpu is false; " +
					"set dedicated_cpu=true to keep indexed field consistent with the spec override")
		}
		if effectiveDedicated && !specHasDedicated {
			// spec_overrides explicitly sets dedicatedCpuPlacement to false — also an inconsistency.
			// Use the canonical service function that checks BOTH flat dot-notation key AND nested
			// map format (DynamicSchemaForm / Ant Design output).
			if conflictPath, hasConflict := service.SpecOverrideSetsExplicitFalseForDedicatedCPU(*req.SpecOverrides); hasConflict {
				return fmt.Errorf(
					"spec_overrides path %q is false but dedicated_cpu is true; "+
						"remove the override or set dedicated_cpu=false", conflictPath)
			}
		}
	}

	return nil
}

func validateDVStorageMode(accessModes []string, volumeMode string) error {
	normalizedAccessModes := normalizeStringList(accessModes)
	normalizedVolumeMode := strings.TrimSpace(volumeMode)
	if len(normalizedAccessModes) == 0 && normalizedVolumeMode == "" {
		return nil
	}
	if len(normalizedAccessModes) == 0 || normalizedVolumeMode == "" {
		return fmt.Errorf("dv_access_modes and dv_volume_mode must be set together")
	}
	switch normalizedVolumeMode {
	case "Block", "Filesystem":
		return nil
	default:
		return fmt.Errorf("dv_volume_mode must be Block or Filesystem")
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

// resolveStringPtr merges a PATCH request field with an existing database value for
// template source validation.
//
//   - If reqField is non-nil, the request is explicitly providing a value (possibly empty
//     to clear), so we use it unchanged.
//   - If reqField is nil, the field is absent from the PATCH request (no change intended),
//     so we fall back to the current stored value wrapped as *string.
//
// This lets validateTemplateSource see the effective (post-merge) state rather than
// the partial view from the request alone.
func resolveStringPtr(reqField *string, existingValue string) *string {
	if reqField != nil {
		return reqField
	}
	if existingValue == "" {
		return nil
	}
	return &existingValue
}

func normalizePermissionKeys(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		key := strings.TrimSpace(p)
		if key == "" {
			continue
		}
		if strings.Contains(key, "*") {
			return nil, fmt.Errorf("wildcard permissions are not allowed: %s", key)
		}
		if !permissionKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid permission key format: %s", key)
		}
		if _, supported := permissionCatalog[key]; !supported {
			return nil, fmt.Errorf("unsupported permission key: %s", key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one permission is required")
	}
	sort.Strings(out)
	return out, nil
}

func parseAuthProviderType(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return "", fmt.Errorf("invalid auth_type")
	}
	return v, nil
}

func validateAuthProviderConfig(authType string, config map[string]interface{}) error {
	adapter := adminglobal.Resolve(authType)
	if adapter == nil {
		return fmt.Errorf("no adapter registered for auth_type=%s", authType)
	}
	return adapter.ValidateConfig(config)
}

func roleToAPI(r *ent.Role) generated.Role {
	permissions := make([]string, 0, len(r.Permissions))
	permissions = append(permissions, r.Permissions...)
	sort.Strings(permissions)
	return generated.Role{
		Id:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Permissions: permissions,
		BuiltIn:     r.BuiltIn,
		Enabled:     r.Enabled,
		CreatedAt:   r.CreatedAt,
	}
}

func (s *Server) authProviderToAPI(p *ent.AuthProvider, revealSensitive bool) (generated.AuthProvider, error) {
	config := p.Config
	if s != nil && s.authProviderConfig != nil {
		var err error
		if revealSensitive {
			config, err = s.authProviderConfig.DecryptForUse(p.AuthType, p.Config)
		} else {
			config, err = s.authProviderConfig.SanitizeForAPI(p.AuthType, p.Config)
		}
		if err != nil {
			return generated.AuthProvider{}, err
		}
	}
	return generated.AuthProvider{
		Id:        p.ID,
		Name:      p.Name,
		AuthType:  p.AuthType,
		Config:    config,
		Enabled:   p.Enabled,
		SortOrder: p.SortOrder,
		CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}, nil
}

// validateTemplateSource enforces the canonical template boot-source taxonomy.
// Supported source_type values are:
//   - "containerdisk"
//   - "cdi_image_import"
//   - "cdi_pvc_clone"
//
// Draft templates may omit source_type entirely.
func validateTemplateSource(sourceType, imageURL, pvcName, pvcNamespace *string) error {
	if sourceType == nil {
		// No source configured yet — allowed (draft template).
		return nil
	}
	normalized := service.NormalizeTemplateSourceType(*sourceType)
	switch normalized {
	case service.TemplateSourceContainerDisk:
		if imageURL == nil || strings.TrimSpace(*imageURL) == "" {
			return fmt.Errorf("image_url is required when source_type is %q", service.TemplateSourceContainerDisk)
		}
	case service.TemplateSourceCDIImageImport:
		if imageURL == nil || strings.TrimSpace(*imageURL) == "" {
			return fmt.Errorf("image_url is required when source_type is %q", service.TemplateSourceCDIImageImport)
		}
	case service.TemplateSourceCDIPVCClone:
		if pvcName == nil || strings.TrimSpace(*pvcName) == "" {
			return fmt.Errorf("pvc_name is required when source_type is %q", service.TemplateSourceCDIPVCClone)
		}
		if pvcNamespace == nil || strings.TrimSpace(*pvcNamespace) == "" {
			return fmt.Errorf("pvc_namespace is required when source_type is %q", service.TemplateSourceCDIPVCClone)
		}
	default:
		return fmt.Errorf(
			"source_type must be one of %q, %q, %q; got %q",
			service.TemplateSourceContainerDisk,
			service.TemplateSourceCDIImageImport,
			service.TemplateSourceCDIPVCClone,
			*sourceType,
		)
	}

	// Detect accidental dual-mode configuration.
	hasImage := imageURL != nil && strings.TrimSpace(*imageURL) != ""
	hasPVC := pvcName != nil && strings.TrimSpace(*pvcName) != ""
	if hasImage && hasPVC {
		return fmt.Errorf("image_url and pvc_name are mutually exclusive; set only one")
	}
	if normalized == service.TemplateSourceCDIPVCClone && imageURL != nil && strings.TrimSpace(*imageURL) != "" {
		return fmt.Errorf("image_url must be empty when source_type is %q", service.TemplateSourceCDIPVCClone)
	}
	if (normalized == service.TemplateSourceContainerDisk || normalized == service.TemplateSourceCDIImageImport) && hasPVC {
		return fmt.Errorf("pvc_name must be empty when source_type is %q", normalized)
	}

	return nil
}

func validateTemplateSourceCatalogScope(sourceType *string, catalogScope string) error {
	if sourceType == nil {
		return nil
	}
	if service.NormalizeTemplateSourceType(*sourceType) != service.TemplateSourceContainerDisk {
		return nil
	}
	switch service.NormalizeCatalogScope(catalogScope) {
	case service.CatalogScopeProd, service.CatalogScopeAll:
		return fmt.Errorf(
			"source_type %q cannot use catalog_scope %q; use %q or %q for ephemeral container disks",
			service.TemplateSourceContainerDisk,
			service.NormalizeCatalogScope(catalogScope),
			service.CatalogScopeTest,
			service.CatalogScopeUnclassified,
		)
	default:
		return nil
	}
}
