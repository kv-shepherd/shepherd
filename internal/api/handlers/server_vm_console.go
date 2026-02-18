package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

const (
	vncBootstrapCookieName      = "vnc_bootstrap"
	vncBootstrapCookieMaxAgeSec = 60
)

type vncRequestPayload struct {
	VMID        string `json:"vm_id"`
	ClusterID   string `json:"cluster_id"`
	Namespace   string `json:"namespace"`
	RequesterID string `json:"requester_id"`
}

// RequestVMConsoleAccess handles POST /vms/{vm_id}/console/request.
func (s *Server) RequestVMConsoleAccess(c *gin.Context, vmId generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	vm, err := s.client.VM.Get(ctx, vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for console request", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	env, err := s.resolveNamespaceEnvironment(ctx, vm.Namespace)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "NAMESPACE_NOT_REGISTERED",
				Message: "namespace is not registered in namespace_registry",
			})
			return
		}
		logger.Error("failed to resolve namespace environment", zap.Error(err), zap.String("namespace", vm.Namespace))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	hasPending, err := s.hasPendingVNCRequest(ctx, vm.ID, actor)
	if err != nil {
		logger.Error("failed to check pending vnc request", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	decision := service.EvaluateVNCRequest(
		env,
		entvm.Status(vm.Status),
		hasVNCConsoleAccess(c),
		hasPending,
	)
	if decision.RejectCode != "" {
		writeVNCReject(c, decision.RejectCode)
		return
	}

	if !decision.RequireApproval {
		vncURL, claims, err := s.issueVNCURL(c, actor, vm)
		if err != nil {
			logger.Error("failed to issue direct vnc token", zap.Error(err), zap.String("vm_id", vm.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if s.audit != nil {
			_ = s.audit.LogAction(ctx, "vnc.access", "vm", vm.ID, actor, map[string]interface{}{
				"token_id":    claims.JTI,
				"environment": string(env),
				"source":      "console_request",
			})
		}

		c.JSON(http.StatusOK, generated.VMConsoleRequestResponse{
			Status: generated.VMConsoleRequestStatusAPPROVED,
			VncUrl: vncURL,
		})
		return
	}

	ticketID, err := s.createVNCApprovalRequest(ctx, vm, actor)
	if err != nil {
		logger.Error("failed to create vnc approval request", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vnc.request_submitted", "vm", vm.ID, actor, map[string]interface{}{
			"ticket_id": ticketID,
		})
	}

	c.JSON(http.StatusAccepted, generated.VMConsoleRequestResponse{
		Status:   generated.VMConsoleRequestStatusPENDINGAPPROVAL,
		TicketId: ticketID,
	})
}

// GetVMConsoleStatus handles GET /vms/{vm_id}/console/status.
func (s *Server) GetVMConsoleStatus(c *gin.Context, vmId generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	vm, err := s.client.VM.Get(ctx, vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for console status", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	env, err := s.resolveNamespaceEnvironment(ctx, vm.Namespace)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "NAMESPACE_NOT_REGISTERED",
				Message: "namespace is not registered in namespace_registry",
			})
			return
		}
		logger.Error("failed to resolve namespace environment", zap.Error(err), zap.String("namespace", vm.Namespace))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !hasVNCConsoleAccess(c) {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return
	}
	if entvm.Status(vm.Status) != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{Code: "VM_NOT_RUNNING"})
		return
	}

	if env == namespaceregistry.EnvironmentTest {
		vncURL, claims, err := s.issueVNCURL(c, actor, vm)
		if err != nil {
			logger.Error("failed to issue test-env vnc token", zap.Error(err), zap.String("vm_id", vm.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if s.audit != nil {
			_ = s.audit.LogAction(ctx, "vnc.access", "vm", vm.ID, actor, map[string]interface{}{
				"token_id":    claims.JTI,
				"environment": string(env),
				"source":      "console_status",
			})
		}
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status: generated.VMConsoleStatusAPPROVED,
			VncUrl: vncURL,
		})
		return
	}

	ticket, err := s.latestVNCRequest(ctx, vm.ID, actor)
	if err != nil {
		logger.Error("failed to query latest vnc request", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if ticket == nil {
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status: generated.VMConsoleStatusNOTREQUESTED,
		})
		return
	}

	switch ticket.Status {
	case approvalticket.StatusPENDING, approvalticket.StatusEXECUTING:
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status:   generated.VMConsoleStatusPENDINGAPPROVAL,
			TicketId: ticket.ID,
		})
		return
	case approvalticket.StatusREJECTED, approvalticket.StatusCANCELLED, approvalticket.StatusFAILED:
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status:   generated.VMConsoleStatusREJECTED,
			TicketId: ticket.ID,
		})
		return
	}

	vncURL, claims, err := s.issueVNCURL(c, actor, vm)
	if err != nil {
		logger.Error("failed to issue approved vnc token", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("ticket_id", ticket.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vnc.access", "vm", vm.ID, actor, map[string]interface{}{
			"token_id":    claims.JTI,
			"environment": string(env),
			"ticket_id":   ticket.ID,
			"source":      "console_status",
		})
	}

	c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
		Status:   generated.VMConsoleStatusAPPROVED,
		TicketId: ticket.ID,
		VncUrl:   vncURL,
	})
}

// OpenVMVNC handles GET /vms/{vm_id}/vnc.
func (s *Server) OpenVMVNC(c *gin.Context, vmId generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	token, cookieErr := c.Cookie(vncBootstrapCookieName)
	token = strings.TrimSpace(token)
	if cookieErr != nil || token == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_VNC_TOKEN"})
		return
	}
	// Stage 6 baseline: bootstrap credential is one-time and must not persist.
	defer s.clearVNCBootstrapCookie(c, vmId)

	claims, validateErr := s.vncTokens.ValidateAndConsume(ctx, token, vmId)
	if validateErr != nil {
		switch {
		case errors.Is(validateErr, service.ErrVNCTokenReplayed):
			c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_REPLAYED"})
		case errors.Is(validateErr, service.ErrVNCTokenVMMismatch):
			c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_VM_MISMATCH"})
		default:
			c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_VNC_TOKEN"})
		}
		return
	}

	if claims.Subject != actor && !hasPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return
	}

	vm, err := s.client.VM.Get(ctx, vmId)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return
		}
		logger.Error("failed to get VM for vnc open", zap.Error(err), zap.String("vm_id", vmId))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if entvm.Status(vm.Status) != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{Code: "VM_NOT_RUNNING"})
		return
	}

	c.JSON(http.StatusOK, generated.VMVNCSessionResponse{
		Status:        generated.SESSIONREADY,
		VmId:          vm.ID,
		WebsocketPath: fmt.Sprintf("/api/v1/vms/%s/vnc", vm.ID),
	})
}

func (s *Server) resolveNamespaceEnvironment(ctx context.Context, namespace string) (namespaceregistry.Environment, error) {
	ns, err := s.client.NamespaceRegistry.Query().
		Where(namespaceregistry.NameEQ(strings.TrimSpace(namespace))).
		Only(ctx)
	if err != nil {
		return "", err
	}
	return ns.Environment, nil
}

func (s *Server) hasPendingVNCRequest(ctx context.Context, vmID, requester string) (bool, error) {
	eventIDs, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDEQ(vmID),
			domainevent.EventTypeEQ(string(domain.EventVNCAccessRequested)),
		).
		Select(domainevent.FieldID).
		Strings(ctx)
	if err != nil {
		return false, err
	}
	if len(eventIDs) == 0 {
		return false, nil
	}

	return s.client.ApprovalTicket.Query().
		Where(
			approvalticket.RequesterEQ(requester),
			approvalticket.OperationTypeEQ(approvalticket.OperationTypeVNC_ACCESS),
			approvalticket.StatusEQ(approvalticket.StatusPENDING),
			approvalticket.EventIDIn(eventIDs...),
		).
		Exist(ctx)
}

func (s *Server) latestVNCRequest(ctx context.Context, vmID, requester string) (*ent.ApprovalTicket, error) {
	eventIDs, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDEQ(vmID),
			domainevent.EventTypeEQ(string(domain.EventVNCAccessRequested)),
		).
		Select(domainevent.FieldID).
		Strings(ctx)
	if err != nil {
		return nil, err
	}
	if len(eventIDs) == 0 {
		return nil, nil
	}

	ticket, err := s.client.ApprovalTicket.Query().
		Where(
			approvalticket.RequesterEQ(requester),
			approvalticket.OperationTypeEQ(approvalticket.OperationTypeVNC_ACCESS),
			approvalticket.EventIDIn(eventIDs...),
		).
		Order(ent.Desc(approvalticket.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ticket, nil
}

func (s *Server) createVNCApprovalRequest(ctx context.Context, vm *ent.VM, actor string) (string, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	eventID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	ticketID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate ticket id: %w", err)
	}

	payload, err := json.Marshal(vncRequestPayload{
		VMID:        vm.ID,
		ClusterID:   vm.ClusterID,
		Namespace:   vm.Namespace,
		RequesterID: actor,
	})
	if err != nil {
		return "", err
	}

	if _, err := tx.DomainEvent.Create().
		SetID(eventID.String()).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx); err != nil {
		return "", err
	}

	if _, err := tx.ApprovalTicket.Create().
		SetID(ticketID.String()).
		SetEventID(eventID.String()).
		SetOperationType(approvalticket.OperationTypeVNC_ACCESS).
		SetStatus(approvalticket.StatusPENDING).
		SetRequester(actor).
		SetReason("vnc access request").
		Save(ctx); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return ticketID.String(), nil
}

func (s *Server) issueVNCURL(c *gin.Context, actor string, vm *ent.VM) (string, service.VNCTokenClaims, error) {
	token, claims, err := s.vncTokens.Issue(actor, vm.ID, vm.ClusterID, vm.Namespace)
	if err != nil {
		return "", service.VNCTokenClaims{}, err
	}
	s.setVNCBootstrapCookie(c, vm.ID, token)
	return fmt.Sprintf("/api/v1/vms/%s/vnc", vm.ID), claims, nil
}

func (s *Server) setVNCBootstrapCookie(c *gin.Context, vmID, token string) {
	if c == nil {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     vncBootstrapCookieName,
		Value:    token,
		Path:     fmt.Sprintf("/api/v1/vms/%s/vnc", strings.TrimSpace(vmID)),
		MaxAge:   vncBootstrapCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearVNCBootstrapCookie(c *gin.Context, vmID string) {
	if c == nil {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     vncBootstrapCookieName,
		Value:    "",
		Path:     fmt.Sprintf("/api/v1/vms/%s/vnc", strings.TrimSpace(vmID)),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func isSecureRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}

	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if strings.EqualFold(proto, "https") {
		return true
	}

	ssl := strings.TrimSpace(c.GetHeader("X-Forwarded-Ssl"))
	return strings.EqualFold(ssl, "on")
}

func writeVNCReject(c *gin.Context, code string) {
	switch code {
	case "FORBIDDEN":
		c.JSON(http.StatusForbidden, generated.Error{Code: code})
	case "VM_NOT_RUNNING":
		c.JSON(http.StatusConflict, generated.Error{Code: code})
	case "DUPLICATE_PENDING_VNC_REQUEST":
		c.JSON(http.StatusConflict, generated.Error{Code: code})
	default:
		c.JSON(http.StatusBadRequest, generated.Error{Code: code})
	}
}

func hasVNCConsoleAccess(c *gin.Context) bool {
	if hasPlatformAdmin(c) {
		return true
	}

	raw, ok := c.Get("permissions")
	if !ok {
		return false
	}
	perms, ok := raw.([]string)
	if !ok {
		return false
	}

	for _, p := range perms {
		if strings.TrimSpace(p) == "vnc:access" {
			return true
		}
	}
	return false
}
