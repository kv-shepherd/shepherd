package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/instancesize"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ListClusters handles GET /admin/clusters.
func (s *Server) ListClusters(c *gin.Context, params generated.ListClustersParams) {
	ctx := c.Request.Context()

	query := s.client.Cluster.Query()

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	clusters, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(cluster.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list clusters", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Cluster, 0, len(clusters))
	for _, cl := range clusters {
		items = append(items, clusterToAPI(cl))
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
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.ClusterCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	id, _ := uuid.NewV7()
	create := s.client.Cluster.Create().
		SetID(id.String()).
		SetName(req.Name).
		SetAPIServerURL(""). // Extracted from kubeconfig in Phase 2.
		SetEncryptedKubeconfig(req.Kubeconfig).
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

	if s.audit != nil {
		if err := s.audit.LogAction(ctx, "cluster.create", "cluster", cl.ID, actor, nil); err != nil {
			logger.Warn("audit log write failed",
				zap.Error(err),
				zap.String("action", "cluster.create"),
				zap.String("resource_id", cl.ID),
			)
		}
	}

	c.JSON(http.StatusCreated, clusterToAPI(cl))
}

// UpdateClusterEnvironment handles PUT /admin/clusters/{cluster_id}/environment.
func (s *Server) UpdateClusterEnvironment(c *gin.Context, clusterId string) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.ClusterEnvironmentUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	cl, err := s.client.Cluster.UpdateOneID(clusterId).
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

	c.JSON(http.StatusOK, clusterToAPI(cl))
}

// ListTemplates handles GET /templates.
func (s *Server) ListTemplates(c *gin.Context, params generated.ListTemplatesParams) {
	ctx := c.Request.Context()

	query := s.client.Template.Query().
		Where(enttemplate.EnabledEQ(true))

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	templates, err := query.
		Offset(offset).
		Limit(perPage).
		All(ctx)
	if err != nil {
		logger.Error("failed to list templates", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Template, 0, len(templates))
	for _, t := range templates {
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
	ctx := c.Request.Context()

	sizes, err := s.client.InstanceSize.Query().
		Where(instancesize.EnabledEQ(true)).
		Order(ent.Asc(instancesize.FieldSortOrder)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list instance sizes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.InstanceSize, 0, len(sizes))
	for _, sz := range sizes {
		items = append(items, instanceSizeToAPI(sz))
	}

	c.JSON(http.StatusOK, generated.InstanceSizeList{
		Items: items,
	})
}

// ListAuditLogs handles GET /audit-logs.
func (s *Server) ListAuditLogs(c *gin.Context, params generated.ListAuditLogsParams) {
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

func clusterToAPI(cl *ent.Cluster) generated.Cluster {
	return generated.Cluster{
		Id:              cl.ID,
		Name:            cl.Name,
		DisplayName:     cl.DisplayName,
		ApiServerUrl:    cl.APIServerURL,
		Status:          generated.ClusterStatus(cl.Status),
		Environment:     generated.ClusterEnvironment(cl.Environment),
		KubevirtVersion: cl.KubevirtVersion,
		StorageClasses:  cl.StorageClasses,
		Enabled:         cl.Enabled,
		CreatedAt:       cl.CreatedAt,
	}
}

func templateToAPI(t *ent.Template) generated.Template {
	return generated.Template{
		Id:          t.ID,
		Name:        t.Name,
		DisplayName: t.DisplayName,
		Description: t.Description,
		OsFamily:    t.OsFamily,
		OsVersion:   t.OsVersion,
		Version:     t.Version,
		Enabled:     t.Enabled,
	}
}

func instanceSizeToAPI(sz *ent.InstanceSize) generated.InstanceSize {
	return generated.InstanceSize{
		Id:                sz.ID,
		Name:              sz.Name,
		DisplayName:       sz.DisplayName,
		Description:       sz.Description,
		CpuCores:          sz.CPUCores,
		MemoryMb:          sz.MemoryMB,
		DiskGb:            sz.DiskGB,
		DedicatedCpu:      sz.DedicatedCPU,
		RequiresGpu:       sz.RequiresGpu,
		RequiresSriov:     sz.RequiresSriov,
		RequiresHugepages: sz.RequiresHugepages,
		HugepagesSize:     sz.HugepagesSize,
		SpecOverrides:     sz.SpecOverrides,
		Enabled:           sz.Enabled,
	}
}
