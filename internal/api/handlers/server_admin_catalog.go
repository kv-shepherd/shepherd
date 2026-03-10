package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	"kv-shepherd.io/shepherd/ent/idpgroupmapping"
	"kv-shepherd.io/shepherd/ent/idpsyncedgroup"
	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	providerregistry "kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
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
	Enabled   *bool   `json:"enabled"`
}

type templateUpdateRequest struct {
	DisplayName  *string `json:"display_name"`
	Description  *string `json:"description"`
	CatalogScope *string `json:"catalog_scope"`
	SourceType   *string `json:"source_type"`
	ImageURL     *string `json:"image_url"`
	PVCName      *string `json:"pvc_name"`
	// PVCNamespace is the Kubernetes namespace where the PVC lives.
	PVCNamespace *string `json:"pvc_namespace"`
	CloudInit    *string `json:"cloud_init"`
	OsFamily     *string `json:"os_family"`
	OsVersion    *string `json:"os_version"`
	Enabled      *bool   `json:"enabled"`
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
	SpecOverrides     map[string]interface{} `json:"spec_overrides"`
	SortOrder         *int                   `json:"sort_order"`
	Enabled           *bool                  `json:"enabled"`
}

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
	"approval:approve":             "Approve or reject approval tickets",
	"approval:view":                "View approval tickets",
	"audit:read":                   "Read audit logs",
	"auth_provider:configure":      "Create authentication providers",
	"auth_provider:delete":         "Delete authentication providers",
	"auth_provider:manage":         "Manage authentication providers (compat)",
	"auth_provider:mapping_create": "Create IdP group mappings",
	"auth_provider:mapping_delete": "Delete IdP group mappings",
	"auth_provider:mapping_update": "Update IdP group mappings",
	"auth_provider:read":           "Read authentication provider configuration",
	"auth_provider:sync":           "Sync external groups for authentication providers",
	"auth_provider:update":         "Update authentication providers",
	"cluster:manage":               "Manage clusters (compat)",
	"cluster:read":                 "Read clusters",
	"cluster:write":                "Create or update clusters",
	"instance_size:read":           "Read instance size catalog",
	"instance_size:write":          "Create/update/delete instance sizes",
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
	"template:manage":              "Manage templates (compat)",
	"template:read":                "Read template catalog",
	"template:write":               "Create/update/delete templates",
	"user:manage":                  "Manage local JWT users",
	"vm:create":                    "Submit VM creation requests",
	"vm:delete":                    "Submit VM deletion requests",
	"vm:operate":                   "Operate VM power actions",
	"vm:read":                      "Read VM information",
	"vnc:access":                   "Request VNC console access",
}

// ListAdminTemplates handles GET /admin/templates.
func (s *Server) ListAdminTemplates(c *gin.Context, params generated.ListAdminTemplatesParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "template:read", "template:manage")
	if !ok {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	query := s.client.Template.Query().
		Order(ent.Desc(enttemplate.FieldUpdatedAt))

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
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write", "template:manage")
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

	id, _ := uuid.NewV7()
	create := s.client.Template.Create().
		SetID(id.String()).
		SetName(name).
		SetCreatedBy(actor)
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
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write", "template:manage")
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
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "template:write", "template:manage")
	if !ok {
		return
	}

	if err := s.client.Template.DeleteOneID(templateID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "TEMPLATE_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete admin template", zap.Error(err), zap.String("template_id", templateID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "template.delete", "template", templateID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// ListAdminInstanceSizes handles GET /admin/instance-sizes.
func (s *Server) ListAdminInstanceSizes(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "instance_size:read")
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

	id, _ := uuid.NewV7()
	create := s.client.InstanceSize.Create().
		SetID(id.String()).
		SetName(strings.TrimSpace(req.Name)).
		SetCPUCores(req.CPUCores).
		SetMemoryGi(req.MemoryGi).
		SetCreatedBy(actor)
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

	if validateErr := validateInstanceSizeUpdate(req, existingSize.DedicatedCPU); validateErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: validateErr.Error()})
		return
	}
	catalogScope, err := normalizeCatalogScopeInput(req.CatalogScope)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_CATALOG_SCOPE", Message: err.Error()})
		return
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
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "instance_size:write")
	if !ok {
		return
	}

	if err := s.client.InstanceSize.DeleteOneID(instanceSizeID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "INSTANCE_SIZE_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete admin instance size", zap.Error(err), zap.String("instance_size_id", instanceSizeID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "instance_size.delete", "instance_size", instanceSizeID, actor, nil)
	}

	c.Status(http.StatusNoContent)
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

	update := existing.Update()
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
	if req.Permissions != nil {
		permissions, normalizeErr := normalizePermissionKeys(*req.Permissions)
		if normalizeErr != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: normalizeErr.Error()})
			return
		}
		update = update.SetPermissions(permissions)
	}
	if req.Enabled != nil {
		update = update.SetEnabled(*req.Enabled)
	}

	r, err := update.Save(ctx)
	if err != nil {
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
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rbac:read", "rbac:manage")
	if !ok {
		return
	}

	catalog := make(map[string]string, len(permissionCatalog))
	for k, v := range permissionCatalog {
		catalog[k] = v
	}

	roles, err := s.client.Role.Query().All(ctx)
	if err != nil {
		logger.Error("failed to query roles for permission catalog", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, r := range roles {
		for _, p := range r.Permissions {
			if _, exists := catalog[p]; !exists {
				catalog[p] = ""
			}
		}
	}

	keys := make([]string, 0, len(catalog))
	for k := range catalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]generated.Permission, 0, len(keys))
	for _, key := range keys {
		items = append(items, generated.Permission{Key: key, Description: catalog[key]})
	}
	c.JSON(http.StatusOK, generated.PermissionList{Items: items})
}

// ListAuthProviderTypes handles GET /admin/auth-provider-types.
func (s *Server) ListAuthProviderTypes(c *gin.Context) {
	_, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	types := providerregistry.ListAuthProviderAdminAdapterTypes()
	items := make([]generated.AuthProviderType, 0, len(types))
	for _, tp := range types {
		items = append(items, generated.AuthProviderType{
			Type:        tp.Type,
			DisplayName: tp.DisplayName,
			Description: tp.Description,
			BuiltIn:     tp.BuiltIn,
			ConfigSchema: func() map[string]interface{} {
				if tp.ConfigSchema == nil {
					return map[string]interface{}{}
				}
				return tp.ConfigSchema
			}(),
		})
	}

	c.JSON(http.StatusOK, generated.AuthProviderTypeList{Items: items})
}

// ListAuthProviders handles GET /admin/auth-providers.
func (s *Server) ListAuthProviders(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

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
		items = append(items, authProviderToAPI(provider))
	}

	c.JSON(http.StatusOK, generated.AuthProviderList{Items: items})
}

// CreateAuthProvider handles POST /admin/auth-providers.
func (s *Server) CreateAuthProvider(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:configure", "auth_provider:manage")
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
		create = create.SetConfig(req.Config)
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

	c.JSON(http.StatusCreated, authProviderToAPI(provider))
}

// UpdateAuthProvider handles PATCH /admin/auth-providers/{provider_id}.
func (s *Server) UpdateAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:update", "auth_provider:manage")
	if !ok {
		return
	}

	var req authProviderUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	update := s.client.AuthProvider.UpdateOneID(providerID)
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "name cannot be empty"})
			return
		}
		update = update.SetName(name)
	}
	if req.Config != nil {
		existing, err := s.client.AuthProvider.Get(ctx, providerID)
		if err != nil {
			if ent.IsNotFound(err) {
				c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
				return
			}
			logger.Error("failed to query auth provider for update validation", zap.Error(err), zap.String("provider_id", providerID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if err := validateAuthProviderConfig(existing.AuthType, *req.Config); err != nil {
			c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		update = update.SetConfig(*req.Config)
	}
	if req.Enabled != nil {
		update = update.SetEnabled(*req.Enabled)
	}
	if req.SortOrder != nil {
		update = update.SetSortOrder(*req.SortOrder)
	}

	provider, err := update.Save(ctx)
	if err != nil {
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

	c.JSON(http.StatusOK, authProviderToAPI(provider))
}

// DeleteAuthProvider handles DELETE /admin/auth-providers/{provider_id}.
func (s *Server) DeleteAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:delete", "auth_provider:manage")
	if !ok {
		return
	}

	userCount, err := s.client.User.Query().Where(entuser.AuthProviderIDEQ(providerID)).Count(ctx)
	if err != nil {
		logger.Error("failed to count provider-linked users", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if userCount > 0 {
		c.JSON(http.StatusConflict, generated.Error{Code: "AUTH_PROVIDER_IN_USE"})
		return
	}

	if err := s.client.AuthProvider.DeleteOneID(providerID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
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

// TestAuthProviderConnection handles POST /admin/auth-providers/{provider_id}/test-connection.
func (s *Server) TestAuthProviderConnection(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:configure", "auth_provider:manage")
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

	okConn, message, err := testAuthProviderConnection(ctx, provider.AuthType, provider.Config)
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
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
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

	syncedGroups, err := s.client.IdPSyncedGroup.Query().
		Where(idpsyncedgroup.ProviderIDEQ(providerID)).
		Order(ent.Asc(idpsyncedgroup.FieldGroupName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to query synced groups for sample", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	fields, err := buildAuthProviderSampleFields(ctx, provider.AuthType, provider.Config, syncedGroups)
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

// SyncAuthProviderGroups handles POST /admin/auth-providers/{provider_id}/sync.
func (s *Server) SyncAuthProviderGroups(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:sync", "auth_provider:manage")
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for group sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.AuthProviderGroupSyncRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	sourceField := strings.TrimSpace(req.SourceField)
	if sourceField == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "source_field is required"})
		return
	}
	groups := normalizeStringList(req.Groups)
	if len(groups) == 0 {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "groups must not be empty"})
		return
	}

	now := time.Now().UTC()
	for _, grp := range groups {
		existing, err := s.client.IdPSyncedGroup.Query().
			Where(
				idpsyncedgroup.ProviderIDEQ(providerID),
				idpsyncedgroup.ExternalGroupIDEQ(grp),
			).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			logger.Error("failed to query synced group", zap.Error(err), zap.String("provider_id", providerID), zap.String("group", grp))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if ent.IsNotFound(err) {
			id, _ := uuid.NewV7()
			_, err = s.client.IdPSyncedGroup.Create().
				SetID(id.String()).
				SetProviderID(providerID).
				SetExternalGroupID(grp).
				SetGroupName(grp).
				SetSourceField(sourceField).
				SetLastSyncedAt(now).
				Save(ctx)
			if err != nil {
				logger.Error("failed to create synced group", zap.Error(err), zap.String("provider_id", providerID), zap.String("group", grp))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			continue
		}

		if _, err := existing.Update().
			SetGroupName(grp).
			SetSourceField(sourceField).
			SetLastSyncedAt(now).
			Save(ctx); err != nil {
			logger.Error("failed to update synced group", zap.Error(err), zap.String("provider_id", providerID), zap.String("group", grp))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	syncedGroups, err := s.client.IdPSyncedGroup.Query().
		Where(idpsyncedgroup.ProviderIDEQ(providerID)).
		Order(ent.Asc(idpsyncedgroup.FieldGroupName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list synced groups after sync", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.sync", "auth_provider", providerID, actor, map[string]interface{}{
			"source_field": sourceField,
			"group_count":  len(groups),
		})
	}

	items := make([]generated.IdPSyncedGroup, 0, len(syncedGroups))
	for _, grp := range syncedGroups {
		items = append(items, syncedGroupToAPI(grp))
	}
	c.JSON(http.StatusOK, generated.AuthProviderGroupSyncResponse{Items: items})
}

// ListAuthProviderGroupMappings handles GET /admin/auth-providers/{provider_id}/group-mappings.
func (s *Server) ListAuthProviderGroupMappings(c *gin.Context, providerID generated.ProviderID) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:read", "auth_provider:manage")
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for mapping list", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	mappings, err := s.client.IdPGroupMapping.Query().
		Where(idpgroupmapping.ProviderIDEQ(providerID)).
		Order(ent.Asc(idpgroupmapping.FieldExternalGroupID)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list idp group mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	roleNameByID, err := s.roleNameMapByMappings(ctx, mappings)
	if err != nil {
		logger.Error("failed to resolve role names for mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	groupNameByID, err := s.syncedGroupNameMapByProvider(ctx, providerID)
	if err != nil {
		logger.Error("failed to resolve group names for mappings", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.IdPGroupMapping, 0, len(mappings))
	for _, m := range mappings {
		items = append(items, idpGroupMappingToAPI(m, roleNameByID[m.RoleID], groupNameByID[m.ExternalGroupID]))
	}
	c.JSON(http.StatusOK, generated.IdPGroupMappingList{Items: items})
}

// CreateAuthProviderGroupMapping handles POST /admin/auth-providers/{provider_id}/group-mappings.
func (s *Server) CreateAuthProviderGroupMapping(c *gin.Context, providerID generated.ProviderID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_create", "auth_provider:manage")
	if !ok {
		return
	}

	if _, err := s.client.AuthProvider.Get(ctx, providerID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			return
		}
		logger.Error("failed to get auth provider for mapping create", zap.Error(err), zap.String("provider_id", providerID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.IdPGroupMappingCreateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	externalGroupID := strings.TrimSpace(req.ExternalGroupId)
	roleID := strings.TrimSpace(req.RoleId)
	if externalGroupID == "" || roleID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "external_group_id and role_id are required"})
		return
	}
	roleEnt, err := s.client.Role.Get(ctx, roleID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
			return
		}
		logger.Error("failed to query role for idp mapping create", zap.Error(err), zap.String("role_id", roleID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	scopeType := strings.TrimSpace(req.ScopeType)
	if scopeType == "" {
		scopeType = "global"
	}
	scopeID := strings.TrimSpace(req.ScopeId)
	allowedEnvs := normalizeIDPAllowedEnvironmentsCreate(req.AllowedEnvironments)

	groupName := strings.TrimSpace(req.GroupName)
	if groupName == "" {
		groupName = externalGroupID
	}
	if syncErr := s.ensureSyncedGroup(ctx, providerID, externalGroupID, groupName); syncErr != nil {
		logger.Error("failed to ensure synced group before mapping create", zap.Error(syncErr), zap.String("provider_id", providerID), zap.String("group", externalGroupID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	id, _ := uuid.NewV7()
	mapping, err := s.client.IdPGroupMapping.Create().
		SetID(id.String()).
		SetProviderID(providerID).
		SetExternalGroupID(externalGroupID).
		SetRoleID(roleID).
		SetScopeType(scopeType).
		SetScopeID(scopeID).
		SetAllowedEnvironments(allowedEnvs).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "IDP_GROUP_MAPPING_EXISTS"})
			return
		}
		logger.Error("failed to create idp mapping", zap.Error(err), zap.String("provider_id", providerID), zap.String("group", externalGroupID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_create", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": mapping.ID,
		})
	}

	c.JSON(http.StatusCreated, idpGroupMappingToAPI(mapping, roleEnt.Name, groupName))
}

// UpdateAuthProviderGroupMapping handles PATCH /admin/auth-providers/{provider_id}/group-mappings/{mapping_id}.
func (s *Server) UpdateAuthProviderGroupMapping(c *gin.Context, providerID generated.ProviderID, mappingID generated.MappingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_update", "auth_provider:manage")
	if !ok {
		return
	}

	var req generated.IdPGroupMappingUpdateRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	mapping, err := s.client.IdPGroupMapping.Query().
		Where(idpgroupmapping.IDEQ(mappingID), idpgroupmapping.ProviderIDEQ(providerID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "IDP_GROUP_MAPPING_NOT_FOUND"})
			return
		}
		logger.Error("failed to query idp mapping for update", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	update := mapping.Update()
	roleName := ""
	if roleID := strings.TrimSpace(req.RoleId); roleID != "" {
		roleEnt, roleErr := s.client.Role.Get(ctx, roleID)
		if roleErr != nil {
			if ent.IsNotFound(roleErr) {
				c.JSON(http.StatusNotFound, generated.Error{Code: "ROLE_NOT_FOUND"})
				return
			}
			logger.Error("failed to query role for idp mapping update", zap.Error(roleErr), zap.String("role_id", roleID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		update = update.SetRoleID(roleID)
		roleName = roleEnt.Name
	}

	if scopeType := strings.TrimSpace(req.ScopeType); scopeType != "" {
		update = update.SetScopeType(scopeType)
	}
	if req.ScopeId != "" {
		update = update.SetScopeID(strings.TrimSpace(req.ScopeId))
	}
	if req.AllowedEnvironments != nil {
		update = update.SetAllowedEnvironments(normalizeIDPAllowedEnvironmentsUpdate(req.AllowedEnvironments))
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, generated.Error{Code: "IDP_GROUP_MAPPING_EXISTS"})
			return
		}
		logger.Error("failed to update idp mapping", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if roleName == "" {
		roleName = roleNameByID(ctx, s.client, updated.RoleID)
	}
	groupName := syncedGroupNameByExternalID(ctx, s.client, providerID, updated.ExternalGroupID)
	if groupName == "" {
		groupName = updated.ExternalGroupID
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_update", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": updated.ID,
		})
	}

	c.JSON(http.StatusOK, idpGroupMappingToAPI(updated, roleName, groupName))
}

// DeleteAuthProviderGroupMapping handles DELETE /admin/auth-providers/{provider_id}/group-mappings/{mapping_id}.
func (s *Server) DeleteAuthProviderGroupMapping(c *gin.Context, providerID generated.ProviderID, mappingID generated.MappingID) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "auth_provider:mapping_delete", "auth_provider:manage")
	if !ok {
		return
	}

	count, err := s.client.IdPGroupMapping.Delete().
		Where(idpgroupmapping.IDEQ(mappingID), idpgroupmapping.ProviderIDEQ(providerID)).
		Exec(ctx)
	if err != nil {
		logger.Error("failed to delete idp mapping", zap.Error(err), zap.String("mapping_id", mappingID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, generated.Error{Code: "IDP_GROUP_MAPPING_NOT_FOUND"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "auth_provider.mapping_delete", "auth_provider", providerID, actor, map[string]interface{}{
			"mapping_id": mappingID,
		})
	}

	c.Status(http.StatusNoContent)
}

func testAuthProviderConnection(ctx context.Context, authType string, config map[string]interface{}) (ok bool, message string, err error) {
	adapter := providerregistry.ResolveAuthProviderAdminAdapter(authType)
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
	syncedGroups []*ent.IdPSyncedGroup,
) ([]generated.AuthProviderSampleField, error) {
	acc := map[string]*sampleFieldAccumulator{}

	if adapter := providerregistry.ResolveAuthProviderAdminAdapter(authType); adapter != nil {
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
	if len(syncedGroups) > 0 {
		groups := make([]interface{}, 0, len(syncedGroups))
		for _, grp := range syncedGroups {
			groups = append(groups, grp.ExternalGroupID)
		}
		addSampleValue(acc, "groups", groups)
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

func normalizeIDPAllowedEnvironmentsCreate(raw []generated.IdPGroupMappingCreateRequestAllowedEnvironments) []string {
	plain := make([]string, 0, len(raw))
	for _, env := range raw {
		plain = append(plain, string(env))
	}
	return normalizeIDPAllowedEnvironments(plain)
}

func normalizeIDPAllowedEnvironmentsUpdate(raw []generated.IdPGroupMappingUpdateRequestAllowedEnvironments) []string {
	plain := make([]string, 0, len(raw))
	for _, env := range raw {
		plain = append(plain, string(env))
	}
	return normalizeIDPAllowedEnvironments(plain)
}

func normalizeIDPAllowedEnvironments(raw []string) []string {
	const (
		envTest = "test"
		envProd = "prod"
	)

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

func (s *Server) ensureSyncedGroup(ctx context.Context, providerID, externalGroupID, groupName string) error {
	_, err := s.client.IdPSyncedGroup.Query().
		Where(idpsyncedgroup.ProviderIDEQ(providerID), idpsyncedgroup.ExternalGroupIDEQ(externalGroupID)).
		Only(ctx)
	if err == nil {
		return nil
	}
	if !ent.IsNotFound(err) {
		return err
	}
	id, _ := uuid.NewV7()
	_, err = s.client.IdPSyncedGroup.Create().
		SetID(id.String()).
		SetProviderID(providerID).
		SetExternalGroupID(externalGroupID).
		SetGroupName(groupName).
		SetLastSyncedAt(time.Now().UTC()).
		Save(ctx)
	return err
}

func (s *Server) roleNameMapByMappings(ctx context.Context, mappings []*ent.IdPGroupMapping) (map[string]string, error) {
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

func (s *Server) syncedGroupNameMapByProvider(ctx context.Context, providerID string) (map[string]string, error) {
	groups, err := s.client.IdPSyncedGroup.Query().
		Where(idpsyncedgroup.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(groups))
	for _, g := range groups {
		out[g.ExternalGroupID] = g.GroupName
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

func syncedGroupNameByExternalID(ctx context.Context, client *ent.Client, providerID, externalGroupID string) string {
	grp, err := client.IdPSyncedGroup.Query().
		Where(
			idpsyncedgroup.ProviderIDEQ(providerID),
			idpsyncedgroup.ExternalGroupIDEQ(externalGroupID),
		).
		Only(ctx)
	if err != nil {
		return ""
	}
	return grp.GroupName
}

func syncedGroupToAPI(g *ent.IdPSyncedGroup) generated.IdPSyncedGroup {
	last := time.Time{}
	if g.LastSyncedAt != nil {
		last = *g.LastSyncedAt
	}
	return generated.IdPSyncedGroup{
		Id:              g.ID,
		ProviderId:      g.ProviderID,
		ExternalGroupId: g.ExternalGroupID,
		GroupName:       g.GroupName,
		SourceField:     g.SourceField,
		LastSyncedAt:    last,
	}
}

func idpGroupMappingToAPI(m *ent.IdPGroupMapping, roleName, groupName string) generated.IdPGroupMapping {
	allowed := make([]generated.IdPGroupMappingAllowedEnvironments, 0, len(m.AllowedEnvironments))
	for _, env := range m.AllowedEnvironments {
		allowed = append(allowed, generated.IdPGroupMappingAllowedEnvironments(env))
	}
	return generated.IdPGroupMapping{
		Id:                  m.ID,
		ProviderId:          m.ProviderID,
		ExternalGroupId:     m.ExternalGroupID,
		GroupName:           groupName,
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
	if dedicated && req.CPURequest != nil && *req.CPURequest != req.CPUCores {
		return fmt.Errorf("cpu_request must equal cpu_cores when dedicated_cpu is true")
	}
	requiresHugepages := req.RequiresHugepages != nil && *req.RequiresHugepages
	hasHugepagesSize := req.HugepagesSize != nil && strings.TrimSpace(*req.HugepagesSize) != ""
	if requiresHugepages && !hasHugepagesSize {
		return fmt.Errorf("hugepages_size is required when requires_hugepages is true")
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
// existingDedicatedCPU is the current value stored in the database. The effective
// dedicated_cpu for validation is determined by merging:
//   - If req.DedicatedCPU is set: use the request value (user is explicitly changing it).
//   - If req.DedicatedCPU is nil: use the existing DB value (partial update, field unchanged).
//
// This prevents false-positive errors where a PATCH only updates spec_overrides
// while leaving dedicated_cpu unchanged, but the validator would default to false.
func validateInstanceSizeUpdate(req instanceSizeUpdateRequest, existingDedicatedCPU bool) error {
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
	if req.CPUCores != nil && req.DedicatedCPU != nil && *req.DedicatedCPU && req.CPURequest != nil && *req.CPURequest > 0 && *req.CPURequest != *req.CPUCores {
		return fmt.Errorf("cpu_request must equal cpu_cores when dedicated_cpu is true")
	}

	// ADR-0036 constraint: dedicated_cpu indexed field must agree with spec_overrides.
	// Use the effective dedicated_cpu: request value if provided, else the existing DB value.
	effectiveDedicated := existingDedicatedCPU
	if req.DedicatedCPU != nil {
		effectiveDedicated = *req.DedicatedCPU
	}

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
	adapter := providerregistry.ResolveAuthProviderAdminAdapter(authType)
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

func authProviderToAPI(p *ent.AuthProvider) generated.AuthProvider {
	return generated.AuthProvider{
		Id:        p.ID,
		Name:      p.Name,
		AuthType:  p.AuthType,
		Config:    p.Config,
		Enabled:   p.Enabled,
		SortOrder: p.SortOrder,
		CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
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
