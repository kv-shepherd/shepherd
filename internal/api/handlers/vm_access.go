package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type vmQueryVisibility struct {
	unrestricted      bool
	namespaceLimited  bool
	visibleNamespaces []string
	visibleSystemIDs  []string
}

func (s *Server) resolveVMQueryVisibility(ctx context.Context, c *gin.Context) (vmQueryVisibility, error) {
	if hasPlatformAdmin(c) {
		return vmQueryVisibility{unrestricted: true}, nil
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		return vmQueryVisibility{}, err
	}

	scope := vmQueryVisibility{
		namespaceLimited: visibility.restricted,
	}
	if visibility.restricted {
		scope.visibleNamespaces, err = s.listVisibleNamespaceNames(ctx, visibility)
		if err != nil {
			return vmQueryVisibility{}, err
		}
	}

	actor := strings.TrimSpace(middleware.GetUserID(ctx))
	if actor == "" {
		return scope, nil
	}

	scope.visibleSystemIDs, err = s.visibleSystemIDs(ctx, actor)
	if err != nil {
		return vmQueryVisibility{}, err
	}

	return scope, nil
}

func (v vmQueryVisibility) empty() bool {
	if v.unrestricted {
		return false
	}
	if len(v.visibleSystemIDs) == 0 {
		return true
	}
	if v.namespaceLimited && len(v.visibleNamespaces) == 0 {
		return true
	}
	return false
}

func (v vmQueryVisibility) apply(query *ent.VMQuery) *ent.VMQuery {
	if v.unrestricted {
		return query
	}

	query = query.Where(
		entvm.HasServiceWith(
			entservice.HasSystemWith(entsystem.IDIn(v.visibleSystemIDs...)),
		),
	)
	if v.namespaceLimited {
		query = query.Where(entvm.NamespaceIn(v.visibleNamespaces...))
	}
	return query
}

func (s *Server) loadAccessibleVM(
	ctx context.Context,
	c *gin.Context,
	vmID string,
	action string,
) (*ent.VM, bool) {
	vmRow, err := s.client.VM.Query().
		Where(entvm.IDEQ(vmID)).
		WithService(func(serviceQuery *ent.ServiceQuery) {
			serviceQuery.WithSystem()
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return nil, false
		}
		logger.Error("failed to load vm for access check", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}

	allowed, err := s.vmAccessibleForAction(ctx, c, vmRow.ID, vmRow.Namespace, action)
	if err != nil {
		logger.Error("failed to evaluate vm access",
			zap.Error(err),
			zap.String("vm_id", vmID),
			zap.String("action", action),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}
	if !allowed {
		c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
		return nil, false
	}

	return vmRow, true
}

func (s *Server) loadAccessibleService(
	ctx context.Context,
	c *gin.Context,
	serviceID string,
	action string,
) (*ent.Service, bool) {
	serviceRow, allowed, err := s.serviceAccessibleForAction(ctx, c, serviceID, action)
	if err != nil {
		logger.Error("failed to evaluate service access",
			zap.Error(err),
			zap.String("service_id", serviceID),
			zap.String("action", action),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, false
	}
	if serviceRow == nil || !allowed {
		c.JSON(http.StatusNotFound, generated.Error{Code: "SERVICE_NOT_FOUND"})
		return nil, false
	}

	return serviceRow, true
}

func (s *Server) serviceAccessibleForAction(
	ctx context.Context,
	c *gin.Context,
	serviceID string,
	action string,
) (*ent.Service, bool, error) {
	serviceRow, err := s.client.Service.Query().
		Where(entservice.IDEQ(strings.TrimSpace(serviceID))).
		WithSystem().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if hasPlatformAdmin(c) {
		return serviceRow, true, nil
	}

	allowed, err := s.resourceActionAllowed(ctx, "service", serviceRow.ID, action)
	if err != nil {
		return nil, false, err
	}
	return serviceRow, allowed, nil
}

func (s *Server) vmAccessibleForAction(
	ctx context.Context,
	c *gin.Context,
	vmID string,
	namespace string,
	action string,
) (bool, error) {
	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		return false, err
	}
	return s.vmAccessibleForActionWithVisibility(ctx, c, vmID, namespace, action, visibility)
}

func (s *Server) vmAccessibleForActionWithVisibility(
	ctx context.Context,
	c *gin.Context,
	vmID string,
	namespace string,
	action string,
	visibility namespaceVisibility,
) (bool, error) {
	visible, err := s.isNamespaceVisible(ctx, namespace, visibility)
	if err != nil || !visible {
		return false, err
	}
	if hasPlatformAdmin(c) {
		return true, nil
	}

	return s.resourceActionAllowed(ctx, "vm", vmID, action)
}

func (s *Server) resourceActionAllowed(
	ctx context.Context,
	resourceType string,
	resourceID string,
	action string,
) (bool, error) {
	actor := strings.TrimSpace(middleware.GetUserID(ctx))
	if actor == "" {
		return false, nil
	}

	checker := middleware.NewResourceRoleChecker(s.client)
	role, found, err := checker.CheckResourceRole(ctx, actor, resourceType, strings.TrimSpace(resourceID))
	if err != nil {
		return false, err
	}

	return found && middleware.RoleCanPerform(role, action), nil
}
