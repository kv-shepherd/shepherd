package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/pendingadoption"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	maxPendingAdoptionDecisionReasonLen = 512
	pendingAdoptionVMResourceType       = "VirtualMachine"
)

type pendingAdoptionDecisionError struct {
	status  int
	code    string
	message string
}

func (e pendingAdoptionDecisionError) Error() string {
	return e.code
}

// ListPendingAdoptions handles GET /admin/pending-adoptions.
func (s *Server) ListPendingAdoptions(c *gin.Context, params generated.ListPendingAdoptionsParams) {
	if !requireAnyGlobalPermission(c, "platform:admin") {
		return
	}
	ctx := c.Request.Context()

	statusFilter := pendingadoption.Status("")
	if status := strings.TrimSpace(string(params.Status)); status != "" {
		statusFilter = pendingadoption.Status(status)
		if err := pendingadoption.StatusValidator(statusFilter); err != nil {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "INVALID_REQUEST",
				Message: "invalid pending adoption status",
			})
			return
		}
	}
	resourceTypeFilter := strings.TrimSpace(string(params.ResourceType))
	if rejectInvalidEnumQuery(c, "resource_type", resourceTypeFilter, pendingAdoptionVMResourceType) {
		return
	}

	query := s.client.PendingAdoption.Query()
	if statusFilter != "" {
		query = query.Where(pendingadoption.StatusEQ(statusFilter))
	}
	if clusterID := strings.TrimSpace(params.ClusterId); clusterID != "" {
		query = query.Where(pendingadoption.ClusterIDEQ(clusterID))
	}
	if namespace := strings.TrimSpace(params.Namespace); namespace != "" {
		query = query.Where(pendingadoption.NamespaceEQ(namespace))
	}
	if resourceTypeFilter != "" {
		query = query.Where(pendingadoption.ResourceTypeEQ(resourceTypeFilter))
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		query = query.Where(
			pendingadoption.Or(
				pendingadoption.ClusterIDContainsFold(search),
				pendingadoption.NamespaceContainsFold(search),
				pendingadoption.ResourceNameContainsFold(search),
				pendingadoption.ResourceTypeContainsFold(search),
				pendingadoption.DiscoveredByContainsFold(search),
			),
		)
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count pending adoptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	rows, err := query.
		Order(ent.Desc(pendingadoption.FieldCreatedAt)).
		Offset(offset).
		Limit(perPage).
		All(ctx)
	if err != nil {
		logger.Error("failed to list pending adoptions", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.PendingAdoption, 0, len(rows))
	for _, row := range rows {
		items = append(items, pendingAdoptionToAPI(row))
	}
	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.PendingAdoptionList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// RejectPendingAdoption handles POST /admin/pending-adoptions/{id}/reject.
func (s *Server) RejectPendingAdoption(c *gin.Context, pendingAdoptionID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	id := strings.TrimSpace(pendingAdoptionID)
	if id == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "pending_adoption_id is required"})
		return
	}

	var req generated.PendingAdoptionRejectRequest
	if c.Request != nil && c.Request.Body != nil && c.Request.ContentLength != 0 {
		if !bindAndValidateJSON(c, &req) {
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > maxPendingAdoptionDecisionReasonLen {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "reason must be at most 512 characters",
		})
		return
	}

	row, err := s.client.PendingAdoption.Query().
		Where(pendingadoption.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "PENDING_ADOPTION_NOT_FOUND"})
			return
		}
		logger.Error("failed to query pending adoption", zap.Error(err), zap.String("pending_adoption_id", id))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if row.Status != pendingadoption.StatusPENDING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_NOT_PENDING",
			Message: "only pending adoption candidates can be rejected",
		})
		return
	}

	updated, err := s.client.PendingAdoption.UpdateOneID(row.ID).
		Where(pendingadoption.StatusEQ(pendingadoption.StatusPENDING)).
		SetStatus(pendingadoption.StatusREJECTED).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "PENDING_ADOPTION_NOT_PENDING",
				Message: "only pending adoption candidates can be rejected",
			})
			return
		}
		logger.Error("failed to reject pending adoption", zap.Error(err), zap.String("pending_adoption_id", id))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		details := map[string]interface{}{
			"cluster_id":      row.ClusterID,
			"namespace":       row.Namespace,
			"resource_name":   row.ResourceName,
			"resource_type":   row.ResourceType,
			"previous_status": string(row.Status),
		}
		if reason != "" {
			details["reason"] = reason
		}
		if err := s.audit.LogAction(ctx, "adoption.rejected", "pending_adoption", row.ID, actor, details); err != nil {
			logger.Warn("failed to write pending adoption rejection audit log",
				zap.String("pending_adoption_id", row.ID),
				zap.Error(err),
			)
		}
	}

	c.JSON(http.StatusOK, pendingAdoptionToAPI(updated))
}

// AdoptPendingAdoption handles POST /admin/pending-adoptions/{id}/adopt.
func (s *Server) AdoptPendingAdoption(c *gin.Context, pendingAdoptionID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "platform:admin")
	if !ok {
		return
	}
	id := strings.TrimSpace(pendingAdoptionID)
	if id == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "pending_adoption_id is required"})
		return
	}
	if s.vmService == nil {
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req generated.PendingAdoptionAdoptRequest
	if c.Request != nil && c.Request.Body != nil && c.Request.ContentLength != 0 {
		if !bindAndValidateJSON(c, &req) {
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > maxPendingAdoptionDecisionReasonLen {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "reason must be at most 512 characters",
		})
		return
	}

	row, err := s.client.PendingAdoption.Query().
		Where(pendingadoption.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "PENDING_ADOPTION_NOT_FOUND"})
			return
		}
		logger.Error("failed to query pending adoption", zap.Error(err), zap.String("pending_adoption_id", id))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if row.Status != pendingadoption.StatusPENDING {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_NOT_PENDING",
			Message: "only pending adoption candidates can be adopted",
		})
		return
	}
	if strings.TrimSpace(row.ResourceType) != pendingAdoptionVMResourceType {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_UNSUPPORTED_RESOURCE",
			Message: "only VirtualMachine adoption candidates are supported",
		})
		return
	}

	pendingLabels := clonePendingAdoptionLabels(row.Labels)
	serviceID := strings.TrimSpace(pendingLabels[domain.ShepherdServiceIDLabel])
	if serviceID == "" {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_SERVICE_LABEL_MISSING",
			Message: "pending adoption candidate has no Shepherd service label",
		})
		return
	}

	liveVM, err := s.vmService.GetVM(ctx, row.ClusterID, row.Namespace, row.ResourceName)
	if err != nil {
		logger.Warn("failed to verify live VM for pending adoption",
			zap.String("pending_adoption_id", row.ID),
			zap.String("cluster_id", row.ClusterID),
			zap.String("namespace", row.Namespace),
			zap.String("resource_name", row.ResourceName),
			zap.Error(err),
		)
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_RESOURCE_UNAVAILABLE",
			Message: "live Kubernetes resource could not be verified",
		})
		return
	}
	liveNamespace := strings.TrimSpace(liveVM.Namespace)
	if liveNamespace == "" {
		liveNamespace = row.Namespace
	}
	liveName := strings.TrimSpace(liveVM.Name)
	if liveName == "" {
		liveName = row.ResourceName
	}
	if liveNamespace != row.Namespace || liveName != row.ResourceName {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_RESOURCE_MISMATCH",
			Message: "live Kubernetes resource identity does not match pending adoption candidate",
		})
		return
	}

	liveLabels := clonePendingAdoptionLabels(liveVM.Spec.Labels)
	liveServiceID := strings.TrimSpace(liveLabels[domain.ShepherdServiceIDLabel])
	if liveServiceID == "" {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_SERVICE_LABEL_MISSING",
			Message: "live Kubernetes resource has no Shepherd service label",
		})
		return
	}
	if liveServiceID != serviceID {
		c.JSON(http.StatusConflict, generated.Error{
			Code:    "PENDING_ADOPTION_SERVICE_LABEL_MISMATCH",
			Message: "live Kubernetes resource service label no longer matches pending adoption candidate",
		})
		return
	}

	var updated *ent.PendingAdoption
	var createdVM *ent.VM
	err = WithTx(ctx, s.client, func(tx *ent.Tx) error {
		txRow, txErr := tx.PendingAdoption.Query().
			Where(pendingadoption.IDEQ(row.ID)).
			Only(ctx)
		if txErr != nil {
			if ent.IsNotFound(txErr) {
				return pendingAdoptionDecisionError{status: http.StatusNotFound, code: "PENDING_ADOPTION_NOT_FOUND"}
			}
			return fmt.Errorf("query pending adoption %s in tx: %w", row.ID, txErr)
		}
		if txRow.Status != pendingadoption.StatusPENDING {
			return pendingAdoptionDecisionError{
				status:  http.StatusConflict,
				code:    "PENDING_ADOPTION_NOT_PENDING",
				message: "only pending adoption candidates can be adopted",
			}
		}

		serviceExists, txErr := tx.Service.Query().
			Where(entservice.IDEQ(serviceID)).
			Exist(ctx)
		if txErr != nil {
			return fmt.Errorf("check adoption service %s: %w", serviceID, txErr)
		}
		if !serviceExists {
			return pendingAdoptionDecisionError{
				status:  http.StatusConflict,
				code:    "PENDING_ADOPTION_SERVICE_MISSING",
				message: "referenced Service no longer exists",
			}
		}

		vmExists, txErr := tx.VM.Query().
			Where(entvm.NamespaceEQ(liveNamespace), entvm.NameEQ(liveName)).
			Exist(ctx)
		if txErr != nil {
			return fmt.Errorf("check existing adopted vm %s/%s: %w", liveNamespace, liveName, txErr)
		}
		if vmExists {
			return pendingAdoptionDecisionError{
				status:  http.StatusConflict,
				code:    "PENDING_ADOPTION_VM_EXISTS",
				message: "a VM inventory row already exists for this Kubernetes resource",
			}
		}

		vmID, txErr := newPendingAdoptionVMID()
		if txErr != nil {
			return txErr
		}
		create := tx.VM.Create().
			SetID(vmID).
			SetName(liveName).
			SetInstance(adoptedVMInstanceFromName(liveName)).
			SetNamespace(liveNamespace).
			SetClusterID(row.ClusterID).
			SetStatus(mapAdoptedVMStatusToRow(liveVM)).
			SetHostname(liveName).
			SetCreatedBy(actor).
			SetServiceID(serviceID)
		if rv := strings.TrimSpace(liveVM.ResourceVersion); rv != "" {
			create.SetLastK8sRv(rv)
		}
		createdVM, txErr = create.Save(ctx)
		if txErr != nil {
			if ent.IsConstraintError(txErr) {
				return pendingAdoptionDecisionError{
					status:  http.StatusConflict,
					code:    "PENDING_ADOPTION_VM_EXISTS",
					message: "a VM inventory row already exists for this Kubernetes resource",
				}
			}
			return fmt.Errorf("create adopted vm row for pending adoption %s: %w", row.ID, txErr)
		}

		updated, txErr = tx.PendingAdoption.UpdateOneID(txRow.ID).
			Where(pendingadoption.StatusEQ(pendingadoption.StatusPENDING)).
			SetStatus(pendingadoption.StatusADOPTED).
			SetLabels(liveLabels).
			Save(ctx)
		if txErr != nil {
			if ent.IsNotFound(txErr) {
				return pendingAdoptionDecisionError{
					status:  http.StatusConflict,
					code:    "PENDING_ADOPTION_NOT_PENDING",
					message: "only pending adoption candidates can be adopted",
				}
			}
			return fmt.Errorf("mark pending adoption %s adopted: %w", row.ID, txErr)
		}
		return nil
	})
	if err != nil {
		var decisionErr pendingAdoptionDecisionError
		if errors.As(err, &decisionErr) {
			c.JSON(decisionErr.status, generated.Error{Code: decisionErr.code, Message: decisionErr.message})
			return
		}
		logger.Error("failed to adopt pending adoption", zap.Error(err), zap.String("pending_adoption_id", row.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		details := map[string]interface{}{
			"cluster_id":      row.ClusterID,
			"namespace":       row.Namespace,
			"resource_name":   row.ResourceName,
			"resource_type":   row.ResourceType,
			"service_id":      serviceID,
			"previous_status": string(row.Status),
			"vm_id":           createdVM.ID,
			"vm_name":         createdVM.Name,
		}
		if reason != "" {
			details["reason"] = reason
		}
		if err := s.audit.LogAction(ctx, "adoption.adopted", "pending_adoption", row.ID, actor, details); err != nil {
			logger.Warn("failed to write pending adoption adoption audit log",
				zap.String("pending_adoption_id", row.ID),
				zap.Error(err),
			)
		}
	}

	c.JSON(http.StatusOK, generated.PendingAdoptionAdoptResponse{
		PendingAdoption: pendingAdoptionToAPI(updated),
		VmId:            createdVM.ID,
		VmName:          createdVM.Name,
	})
}

func pendingAdoptionToAPI(row *ent.PendingAdoption) generated.PendingAdoption {
	if row == nil {
		return generated.PendingAdoption{}
	}
	return generated.PendingAdoption{
		Id:           row.ID,
		ClusterId:    row.ClusterID,
		Namespace:    row.Namespace,
		ResourceName: row.ResourceName,
		ResourceType: generated.PendingAdoptionResourceType(row.ResourceType),
		Status:       generated.PendingAdoptionStatus(row.Status),
		DiscoveredBy: row.DiscoveredBy,
		Labels:       clonePendingAdoptionLabels(row.Labels),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func clonePendingAdoptionLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func newPendingAdoptionVMID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate adopted vm id: %w", err)
	}
	return id.String(), nil
}

func adoptedVMInstanceFromName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "adopted"
	}
	start := len(trimmed)
	for start > 0 {
		ch := trimmed[start-1]
		if ch < '0' || ch > '9' {
			break
		}
		start--
	}
	if start < len(trimmed) {
		return trimmed[start:]
	}
	return "adopted"
}

func mapAdoptedVMStatusToRow(liveVM *domain.VM) entvm.Status {
	if liveVM == nil {
		return entvm.StatusUNKNOWN
	}
	switch liveVM.Status {
	case domain.VMStatusCreating:
		return entvm.StatusCREATING
	case domain.VMStatusStarting:
		return entvm.StatusSTARTING
	case domain.VMStatusRunning:
		return entvm.StatusRUNNING
	case domain.VMStatusStopping:
		return entvm.StatusSTOPPING
	case domain.VMStatusStopped:
		return entvm.StatusSTOPPED
	case domain.VMStatusDeleting:
		return entvm.StatusDELETING
	case domain.VMStatusFailed:
		return entvm.StatusFAILED
	case domain.VMStatusPending:
		return entvm.StatusPENDING
	case domain.VMStatusMigrating:
		return entvm.StatusMIGRATING
	case domain.VMStatusPaused:
		return entvm.StatusPAUSED
	case domain.VMStatusNotFound:
		return entvm.StatusNOT_FOUND
	default:
		return entvm.StatusUNKNOWN
	}
}
