package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
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
	vncWebSocketWriteTimeout    = 10 * time.Second
	vncWebSocketPongWait        = 60 * time.Second
	vncWebSocketPingInterval    = 30 * time.Second
	vncWebSocketReadLimit       = 512 * 1024
)

type vncRequestPayload struct {
	VMID                 string `json:"vm_id"`
	ClusterID            string `json:"cluster_id"`
	Namespace            string `json:"namespace"`
	RequesterID          string `json:"requester_id"`
	PreferredConsoleType string `json:"preferred_console_type,omitempty"`
}

type issuedConsole struct {
	ConsoleType generated.VMConsoleType
	ConsoleURL  string
}

func bindOptionalJSON[T any](c *gin.Context) (*T, error) {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return nil, nil
	}
	var payload T
	if err := c.ShouldBindJSON(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return &payload, nil
}

// RequestVMConsoleAccess handles POST /vms/{vm_id}/console/request.
func (s *Server) RequestVMConsoleAccess(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}
	req, bindErr := bindOptionalJSON[generated.VMConsoleRequestInput](c)
	if bindErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "BAD_REQUEST",
			Message: bindErr.Error(),
		})
		return
	}
	preferredConsoleType := normalizePreferredConsoleType(req)

	vm, ok := s.loadAccessibleVM(ctx, c, vmID, "create")
	if !ok {
		return
	}
	vm = s.refreshVMLiveState(ctx, vm)

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
	requiresApproval, err := s.requiresVNCApproval(ctx, env)
	if err != nil {
		logger.Error("failed to evaluate vnc approval requirement", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("namespace", vm.Namespace))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	decision := service.EvaluateVNCRequest(
		vm.Status,
		hasVNCConsoleAccess(c),
		hasPending,
		requiresApproval,
	)
	if decision.RejectCode != "" {
		writeVNCReject(c, decision.RejectCode)
		return
	}

	if !decision.RequireApproval {
		console, claims, issueErr := s.issuePreferredConsoleURL(c, actor, vm, preferredConsoleType)
		if issueErr != nil {
			logger.Error("failed to issue direct console token", zap.Error(issueErr), zap.String("vm_id", vm.ID))
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
			Status:      generated.VMConsoleRequestStatusAPPROVED,
			ConsoleType: console.ConsoleType,
			ConsoleUrl:  console.ConsoleURL,
			VncUrl:      legacyVNCURL(console),
		})
		return
	}

	approvedTicket, err := s.latestApprovedVNCRequest(ctx, vm.ID, actor)
	if err != nil {
		logger.Error("failed to query latest approved vnc request", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if approvedTicket != nil {
		consolePreference := preferredConsoleType
		if consolePreference == nil {
			consolePreference, err = s.preferredConsoleTypeForTicket(ctx, approvedTicket)
			if err != nil {
				logger.Warn("failed to parse preferred console type from approved vnc ticket payload", zap.Error(err), zap.String("ticket_id", approvedTicket.ID))
			}
		}

		console, claims, issueErr := s.issuePreferredConsoleURL(c, actor, vm, consolePreference)
		if issueErr != nil {
			logger.Error("failed to issue approved console token", zap.Error(issueErr), zap.String("vm_id", vm.ID), zap.String("ticket_id", approvedTicket.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if s.audit != nil {
			_ = s.audit.LogAction(ctx, "vnc.access", "vm", vm.ID, actor, map[string]interface{}{
				"token_id":    claims.JTI,
				"environment": string(env),
				"ticket_id":   approvedTicket.ID,
				"source":      "console_request",
			})
		}

		c.JSON(http.StatusOK, generated.VMConsoleRequestResponse{
			Status:      generated.VMConsoleRequestStatusAPPROVED,
			TicketId:    approvedTicket.ID,
			ConsoleType: console.ConsoleType,
			ConsoleUrl:  console.ConsoleURL,
			VncUrl:      legacyVNCURL(console),
		})
		return
	}

	if _, _, preflightErr := s.resolvePreferredConsolePath(ctx, vm, preferredConsoleType); preflightErr != nil {
		logger.Error("failed to preflight requested console path", zap.Error(preflightErr), zap.String("vm_id", vm.ID))
		c.JSON(http.StatusBadGateway, generated.Error{
			Code:    "VNC_UNAVAILABLE",
			Message: preflightErr.Error(),
		})
		return
	}
	ticketID, err := s.createVNCApprovalRequest(ctx, vm, actor, preferredConsoleType)
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
		Status:      generated.VMConsoleRequestStatusPENDINGAPPROVAL,
		TicketId:    ticketID,
		ConsoleType: derefConsoleType(preferredConsoleType),
	})
}

// GetVMConsoleStatus handles GET /vms/{vm_id}/console/status.
func (s *Server) GetVMConsoleStatus(c *gin.Context, vmID generated.VMID) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vnc:access") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if actor == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	vm, ok := s.loadAccessibleVM(ctx, c, vmID, "create")
	if !ok {
		return
	}
	vm = s.refreshVMLiveState(ctx, vm)

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
	if vm.Status != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{Code: "VM_NOT_RUNNING"})
		return
	}

	requiresApproval, err := s.requiresVNCApproval(ctx, env)
	if err != nil {
		logger.Error("failed to evaluate vnc approval requirement", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("namespace", vm.Namespace))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !requiresApproval {
		console, claims, issueErr := s.issuePreferredConsoleURL(c, actor, vm, nil)
		if issueErr != nil {
			logger.Error("failed to issue direct console token", zap.Error(issueErr), zap.String("vm_id", vm.ID))
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
			Status:      generated.VMConsoleStatusAPPROVED,
			ConsoleType: console.ConsoleType,
			ConsoleUrl:  console.ConsoleURL,
			VncUrl:      legacyVNCURL(console),
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
	case entticket.StatusPENDING, entticket.StatusEXECUTING:
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status:   generated.VMConsoleStatusPENDINGAPPROVAL,
			TicketId: ticket.ID,
		})
		return
	case entticket.StatusREJECTED, entticket.StatusCANCELLED, entticket.StatusFAILED:
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status:   generated.VMConsoleStatusREJECTED,
			TicketId: ticket.ID,
		})
		return
	case entticket.StatusAPPROVED, entticket.StatusSUCCESS:
		// Continue below and issue the short-lived VNC URL.
	default:
		c.JSON(http.StatusOK, generated.VMConsoleStatusResponse{
			Status:   generated.VMConsoleStatusPENDINGAPPROVAL,
			TicketId: ticket.ID,
		})
		return
	}

	consolePreference, consolePrefErr := s.preferredConsoleTypeForTicket(ctx, ticket)
	if consolePrefErr != nil {
		logger.Warn("failed to parse preferred console type from ticket payload", zap.Error(consolePrefErr), zap.String("ticket_id", ticket.ID))
	}
	console, claims, err := s.issuePreferredConsoleURL(c, actor, vm, consolePreference)
	if err != nil {
		logger.Error("failed to issue approved console token", zap.Error(err), zap.String("vm_id", vm.ID), zap.String("ticket_id", ticket.ID))
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
		Status:      generated.VMConsoleStatusAPPROVED,
		TicketId:    ticket.ID,
		ConsoleType: console.ConsoleType,
		ConsoleUrl:  console.ConsoleURL,
		VncUrl:      legacyVNCURL(console),
	})
}

func (s *Server) requiresVNCApproval(ctx context.Context, env namespaceregistry.Environment) (bool, error) {
	if s.approvalReqs == nil {
		return env == namespaceregistry.EnvironmentProd, nil
	}
	return s.approvalReqs.RequiresApproval(ctx, approvalpolicy.OperationVNC_ACCESS, env)
}

// OpenVMVNC handles GET /vms/{vm_id}/vnc.
func (s *Server) OpenVMVNC(c *gin.Context, vmID generated.VMID) {
	s.openVMConsole(c, vmID, generated.VNC)
}

// OpenVMSerial handles GET /vms/{vm_id}/serial.
func (s *Server) OpenVMSerial(c *gin.Context, vmID generated.VMID) {
	s.openVMConsole(c, vmID, generated.SERIAL)
}

func (s *Server) openVMConsole(c *gin.Context, vmID generated.VMID, consoleType generated.VMConsoleType) {
	vm, claims, websocketUpgrade, ok := s.resolveConsoleTarget(c, vmID)
	if !ok {
		return
	}

	if websocketUpgrade {
		s.streamVMConsole(c, vm, claims, consoleType)
		return
	}

	if s.vmService == nil {
		c.JSON(http.StatusServiceUnavailable, generated.Error{Code: "VNC_UNAVAILABLE"})
		return
	}

	c.JSON(http.StatusOK, generated.VMConsoleSessionResponse{
		Status:        generated.VMConsoleSessionResponseStatusSESSIONREADY,
		VmId:          vm.ID,
		ConsoleType:   consoleType,
		WebsocketPath: consolePathForType(vm.ID, consoleType),
	})
}

func (s *Server) resolveConsoleTarget(
	c *gin.Context,
	vmID generated.VMID,
) (*ent.VM, *service.VNCJWTClaims, bool, bool) {
	ctx := c.Request.Context()
	token, cookieErr := c.Cookie(vncBootstrapCookieName)
	token = strings.TrimSpace(token)
	if cookieErr != nil || token == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_VNC_TOKEN"})
		return nil, nil, false, false
	}

	websocketUpgrade := websocket.IsWebSocketUpgrade(c.Request)
	claims, validateErr := s.validateConsoleBootstrapToken(ctx, token, vmID, websocketUpgrade)
	if validateErr != nil {
		writeConsoleBootstrapError(c, validateErr)
		return nil, nil, false, false
	}

	vm, err := s.client.VM.Get(ctx, vmID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "VM_NOT_FOUND"})
			return nil, nil, false, false
		}
		logger.Error("failed to get VM for console open", zap.Error(err), zap.String("vm_id", vmID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return nil, nil, false, false
	}
	vm = s.refreshVMLiveState(ctx, vm)
	if vm.Status != entvm.StatusRUNNING {
		c.JSON(http.StatusConflict, generated.Error{Code: "VM_NOT_RUNNING"})
		return nil, nil, false, false
	}
	if claims.ClusterID != "" && claims.ClusterID != vm.ClusterID {
		c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_VM_MISMATCH"})
		return nil, nil, false, false
	}
	if claims.Namespace != "" && claims.Namespace != vm.Namespace {
		c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_VM_MISMATCH"})
		return nil, nil, false, false
	}

	return vm, claims, websocketUpgrade, true
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

	return s.client.Ticket.Query().
		Where(
			entticket.RequesterEQ(requester),
			entticket.OperationTypeEQ(entticket.OperationTypeVNC_ACCESS),
			entticket.StatusEQ(entticket.StatusPENDING),
			entticket.EventIDIn(eventIDs...),
		).
		Exist(ctx)
}

func (s *Server) latestVNCRequest(ctx context.Context, vmID, requester string) (*ent.Ticket, error) {
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

	ticket, err := s.client.Ticket.Query().
		Where(
			entticket.RequesterEQ(requester),
			entticket.OperationTypeEQ(entticket.OperationTypeVNC_ACCESS),
			entticket.EventIDIn(eventIDs...),
		).
		Order(ent.Desc(entticket.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ticket, nil
}

func (s *Server) latestApprovedVNCRequest(ctx context.Context, vmID, requester string) (*ent.Ticket, error) {
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

	ticket, err := s.client.Ticket.Query().
		Where(
			entticket.RequesterEQ(requester),
			entticket.OperationTypeEQ(entticket.OperationTypeVNC_ACCESS),
			entticket.EventIDIn(eventIDs...),
			entticket.StatusIn(entticket.StatusAPPROVED, entticket.StatusSUCCESS),
		).
		Order(ent.Desc(entticket.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ticket, nil
}

func (s *Server) createVNCApprovalRequest(ctx context.Context, vm *ent.VM, actor string, preferredConsoleType *generated.VMConsoleType) (string, error) {
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
		VMID:                 vm.ID,
		ClusterID:            vm.ClusterID,
		Namespace:            vm.Namespace,
		RequesterID:          actor,
		PreferredConsoleType: stringValueForConsoleType(preferredConsoleType),
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

	if _, err := tx.Ticket.Create().
		SetID(ticketID.String()).
		SetEventID(eventID.String()).
		SetOperationType(entticket.OperationTypeVNC_ACCESS).
		SetStatus(entticket.StatusPENDING).
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

func legacyVNCURL(console *issuedConsole) string {
	if console == nil || console.ConsoleType != generated.VNC {
		return ""
	}
	return console.ConsoleURL
}

func (s *Server) issuePreferredConsoleURL(c *gin.Context, actor string, vm *ent.VM, preferredConsoleType *generated.VMConsoleType) (*issuedConsole, service.VNCTokenClaims, error) {
	consoleType, consolePath, err := s.resolvePreferredConsolePath(c.Request.Context(), vm, preferredConsoleType)
	if err != nil {
		return nil, service.VNCTokenClaims{}, err
	}
	token, claims, err := s.vncTokens.Issue(actor, vm.ID, vm.ClusterID, vm.Namespace)
	if err != nil {
		return nil, service.VNCTokenClaims{}, err
	}
	s.setConsoleBootstrapCookie(c, token, consolePath)
	return &issuedConsole{
		ConsoleType: consoleType,
		ConsoleURL:  consolePath,
	}, claims, nil
}

func (s *Server) resolvePreferredConsolePath(ctx context.Context, vm *ent.VM, preferredConsoleType *generated.VMConsoleType) (generated.VMConsoleType, string, error) {
	if s.vmService == nil {
		return "", "", fmt.Errorf("console service is unavailable")
	}

	serialPath := fmt.Sprintf("/api/v1/vms/%s/serial", vm.ID)
	trySerial := preferredConsoleType == nil || *preferredConsoleType == generated.SERIAL
	tryVNC := preferredConsoleType == nil || *preferredConsoleType == generated.VNC
	if preferredConsoleType != nil {
		switch *preferredConsoleType {
		case generated.SERIAL:
			return generated.SERIAL, serialPath, nil
		case generated.VNC:
			return generated.VNC, fmt.Sprintf("/api/v1/vms/%s/vnc", vm.ID), nil
		default:
			return "", "", fmt.Errorf("requested console type is unavailable")
		}
	}
	if trySerial {
		if backend, err := s.vmService.OpenSerialConsoleStream(ctx, vm.ClusterID, vm.Namespace, vm.Name); err == nil {
			_ = backend.Close()
			return generated.SERIAL, serialPath, nil
		}
	}

	vncPath := fmt.Sprintf("/api/v1/vms/%s/vnc", vm.ID)
	if tryVNC {
		if backend, err := s.vmService.OpenVNCStream(ctx, vm.ClusterID, vm.Namespace, vm.Name); err == nil {
			_ = backend.Close()
			return generated.VNC, vncPath, nil
		} else {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("requested console type is unavailable")
}

func normalizePreferredConsoleType(req *generated.VMConsoleRequestInput) *generated.VMConsoleType {
	if req == nil || req.PreferredConsoleType == nil {
		return nil
	}
	switch *req.PreferredConsoleType {
	case generated.SERIAL, generated.VNC:
		return req.PreferredConsoleType
	default:
		return nil
	}
}

func stringValueForConsoleType(consoleType *generated.VMConsoleType) string {
	if consoleType == nil {
		return ""
	}
	return string(*consoleType)
}

func derefConsoleType(consoleType *generated.VMConsoleType) generated.VMConsoleType {
	if consoleType == nil {
		return ""
	}
	return *consoleType
}

func (s *Server) preferredConsoleTypeForTicket(ctx context.Context, ticket *ent.Ticket) (*generated.VMConsoleType, error) {
	if ticket == nil || ticket.EventID == "" {
		return nil, nil
	}
	event, err := s.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return nil, err
	}
	var payload vncRequestPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.PreferredConsoleType == "" {
		return nil, nil
	}
	consoleType := generated.VMConsoleType(payload.PreferredConsoleType)
	switch consoleType {
	case generated.SERIAL, generated.VNC:
		return &consoleType, nil
	default:
		return nil, nil
	}
}

func (s *Server) validateConsoleBootstrapToken(
	ctx context.Context,
	token string,
	vmID generated.VMID,
	consume bool,
) (*service.VNCJWTClaims, error) {
	if consume {
		return s.vncTokens.ValidateAndConsume(ctx, token, vmID)
	}
	return s.vncTokens.Validate(token, vmID)
}

func writeConsoleBootstrapError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrVNCTokenReplayed):
		c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_REPLAYED"})
	case errors.Is(err, service.ErrVNCTokenVMMismatch):
		c.JSON(http.StatusConflict, generated.Error{Code: "VNC_TOKEN_VM_MISMATCH"})
	default:
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "INVALID_VNC_TOKEN"})
	}
}

func (s *Server) openConsoleStream(ctx context.Context, vm *ent.VM, consoleType generated.VMConsoleType) (net.Conn, error) {
	if s.vmService == nil {
		return nil, fmt.Errorf("console service is unavailable")
	}
	switch consoleType {
	case generated.SERIAL:
		return s.vmService.OpenSerialConsoleStream(ctx, vm.ClusterID, vm.Namespace, vm.Name)
	case generated.VNC:
		return s.vmService.OpenVNCStream(ctx, vm.ClusterID, vm.Namespace, vm.Name)
	default:
		return nil, fmt.Errorf("unsupported console type %q", consoleType)
	}
}

func (s *Server) streamVMConsole(c *gin.Context, vm *ent.VM, claims *service.VNCJWTClaims, consoleType generated.VMConsoleType) {
	backend, err := s.openConsoleStream(c.Request.Context(), vm, consoleType)
	if err != nil {
		logger.Error("failed to open kubevirt console stream",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("console_type", string(consoleType)),
		)
		c.JSON(http.StatusBadGateway, generated.Error{
			Code:    "VNC_UNAVAILABLE",
			Message: err.Error(),
		})
		return
	}

	conn, err := s.upgradeConsoleWebSocket(c, consolePathForType(vm.ID, consoleType))
	if err != nil {
		_ = backend.Close()
		logger.Error("failed to upgrade console websocket",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("console_type", string(consoleType)),
		)
		return
	}
	defer conn.Close()
	defer backend.Close()

	if s.audit != nil && claims != nil {
		_ = s.audit.LogAction(c.Request.Context(), consoleAuditAction(consoleType), "vm", vm.ID, claims.Subject, map[string]interface{}{
			"token_id": claims.ID,
		})
	}

	if err := proxyConsoleWebSocket(conn, backend); err != nil &&
		!errors.Is(err, io.EOF) &&
		!websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) &&
		!websocket.IsUnexpectedCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseGoingAway) {
		logger.Warn("console websocket proxy terminated with error",
			zap.Error(err),
			zap.String("vm_id", vm.ID),
			zap.String("console_type", string(consoleType)),
		)
	}
}

func (s *Server) upgradeConsoleWebSocket(c *gin.Context, consolePath string) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		CheckOrigin:      s.consoleOriginAllowed,
	}
	headers := http.Header{}
	headers.Add("Set-Cookie", s.consoleBootstrapCookie("", -1, secureCookieByPolicy(c, true, s.publicBaseURL), consolePath).String())
	return upgrader.Upgrade(c.Writer, c.Request, headers)
}

func (s *Server) consoleOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	requestOrigin := requestOriginForConsole(r)
	if requestOrigin != "" && sameExternalAuthOrigin(origin, requestOrigin) {
		return true
	}
	for _, allowedOrigin := range s.effectiveExternalAuthAllowedOrigins() {
		if sameExternalAuthOrigin(origin, allowedOrigin) {
			return true
		}
	}
	return false
}

func requestOriginForConsole(r *http.Request) string {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(r.Host)
}

func proxyConsoleWebSocket(ws *websocket.Conn, backend net.Conn) error {
	ws.SetReadLimit(vncWebSocketReadLimit)
	_ = ws.SetReadDeadline(time.Now().Add(vncWebSocketPongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(vncWebSocketPongWait))
	})

	proxyCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			_ = backend.Close()
			_ = ws.Close()
		})
	}

	group, groupCtx := errgroup.WithContext(proxyCtx)
	group.Go(func() error {
		defer func() {
			cancel()
			shutdown()
		}()
		return pumpConsoleWebSocketToBackend(ws, backend)
	})
	group.Go(func() error {
		defer func() {
			cancel()
			shutdown()
		}()
		return pumpConsoleBackendToWebSocket(backend, ws)
	})
	group.Go(func() error {
		return pumpConsolePing(groupCtx, ws)
	})

	err := group.Wait()
	shutdown()
	return err
}

func pumpConsoleWebSocketToBackend(ws *websocket.Conn, backend net.Conn) error {
	for {
		messageType, reader, err := ws.NextReader()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}
		if _, err := io.Copy(backend, reader); err != nil {
			return err
		}
	}
}

func pumpConsoleBackendToWebSocket(backend net.Conn, ws *websocket.Conn) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := backend.Read(buf)
		if n > 0 {
			_ = ws.SetWriteDeadline(time.Now().Add(vncWebSocketWriteTimeout))
			if writeErr := ws.WriteMessage(websocket.BinaryMessage, append([]byte(nil), buf[:n]...)); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
}

func pumpConsolePing(ctx context.Context, ws *websocket.Conn) error {
	ticker := time.NewTicker(vncWebSocketPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(vncWebSocketWriteTimeout)); err != nil {
				return err
			}
		}
	}
}

func (s *Server) consoleBootstrapCookie(value string, maxAge int, secure bool, consolePath string) *http.Cookie {
	return &http.Cookie{
		Name:     vncBootstrapCookieName,
		Value:    value,
		Path:     consolePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (s *Server) setConsoleBootstrapCookie(c *gin.Context, token, consolePath string) {
	if c == nil {
		return
	}
	http.SetCookie(c.Writer, s.consoleBootstrapCookie(token, vncBootstrapCookieMaxAgeSec, secureCookieByPolicy(c, true, s.publicBaseURL), consolePath))
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

func consolePathForType(vmID string, consoleType generated.VMConsoleType) string {
	switch consoleType {
	case generated.SERIAL:
		return fmt.Sprintf("/api/v1/vms/%s/serial", vmID)
	default:
		return fmt.Sprintf("/api/v1/vms/%s/vnc", vmID)
	}
}

func consoleAuditAction(consoleType generated.VMConsoleType) string {
	switch consoleType {
	case generated.SERIAL:
		return "serial.websocket_opened"
	default:
		return "vnc.websocket_opened"
	}
}
