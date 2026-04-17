package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/ratelimitexemption"
	"kv-shepherd.io/shepherd/ent/ratelimituseroverride"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

const (
	maxBatchItems                   = 100
	maxPendingBatchParents          = 100
	maxPendingBatchParentsUser      = 3
	maxPendingBatchChildrenUser     = 30
	maxGlobalBatchRequestsPerMinute = 1000
	batchSubmitCooldown             = 2 * time.Minute
	batchRetryAfterSeconds          = 2
	batchActionRetry                = "retry"
	batchActionCancel               = "cancel"
)

var errBatchNotFound = errors.New("batch not found")

type preparedBatchChild struct {
	eventType        domain.EventType
	aggregateID      string
	payload          []byte
	operationType    entticket.OperationType
	reason           string
	requiresApproval bool
}

type batchVMContextSnapshot struct {
	SystemID           string
	SystemName         string
	ServiceID          string
	ServiceName        string
	ClusterName        string
	ClusterEnvironment string
	OwnerID            string
	OwnerDisplayName   string
	OwnerUsername      string
	TemplateID         string
	TemplateName       string
	InstanceSizeID     string
	InstanceSizeName   string
	CurrentCPUCores    float64
	CurrentMemoryGi    float64
	CurrentDiskGB      int
}

type batchSnapshotLoader struct {
	server                    *Server
	actorByID                 map[string]approvalActorLookup
	clusterNamesByID          map[string]string
	clusterEnvironmentsByID   map[string]string
	creationPayloadByTicketID map[string]domain.VMCreationPayload
	serviceByID               map[string]approvalServiceLookup
	templateByID              map[string]*ent.Template
	instanceSizeByID          map[string]*ent.InstanceSize
}

func newBatchSnapshotLoader(server *Server) *batchSnapshotLoader {
	return &batchSnapshotLoader{
		server:                    server,
		actorByID:                 make(map[string]approvalActorLookup),
		clusterNamesByID:          make(map[string]string),
		clusterEnvironmentsByID:   make(map[string]string),
		creationPayloadByTicketID: make(map[string]domain.VMCreationPayload),
		serviceByID:               make(map[string]approvalServiceLookup),
		templateByID:              make(map[string]*ent.Template),
		instanceSizeByID:          make(map[string]*ent.InstanceSize),
	}
}

func (l *batchSnapshotLoader) actorIdentity(ctx context.Context, actorID string) (displayName, username string) {
	trimmedID := strings.TrimSpace(actorID)
	if trimmedID == "" || l == nil || l.server == nil {
		return "", ""
	}
	if actor, ok := l.actorByID[trimmedID]; ok {
		return approvalActorIdentity(trimmedID, map[string]approvalActorLookup{trimmedID: actor})
	}
	user, err := l.server.client.User.Get(ctx, trimmedID)
	if err != nil {
		l.actorByID[trimmedID] = approvalActorLookup{}
		return trimmedID, trimmedID
	}
	l.actorByID[trimmedID] = approvalActorLookup{
		DisplayName: strings.TrimSpace(user.DisplayName),
		Username:    strings.TrimSpace(user.Username),
	}
	return approvalActorIdentity(trimmedID, l.actorByID)
}

func (l *batchSnapshotLoader) clusterPresentation(ctx context.Context, clusterID string) (clusterName, clusterEnvironment string) {
	trimmedID := strings.TrimSpace(clusterID)
	if trimmedID == "" || l == nil || l.server == nil {
		return "", ""
	}
	if name, ok := l.clusterNamesByID[trimmedID]; ok {
		return name, l.clusterEnvironmentsByID[trimmedID]
	}
	cluster, err := l.server.client.Cluster.Get(ctx, trimmedID)
	if err != nil {
		l.clusterNamesByID[trimmedID] = ""
		l.clusterEnvironmentsByID[trimmedID] = ""
		return "", ""
	}
	name := firstNonEmptyString(cluster.DisplayName, cluster.Name, cluster.ID)
	environment := string(cluster.Environment)
	l.clusterNamesByID[trimmedID] = name
	l.clusterEnvironmentsByID[trimmedID] = environment
	return name, environment
}

func (l *batchSnapshotLoader) serviceLookup(ctx context.Context, serviceID string) approvalServiceLookup {
	trimmedID := strings.TrimSpace(serviceID)
	if trimmedID == "" || l == nil || l.server == nil {
		return approvalServiceLookup{}
	}
	if lookup, ok := l.serviceByID[trimmedID]; ok {
		return lookup
	}
	serviceRow, err := l.server.client.Service.Query().
		Where(entservice.IDEQ(trimmedID)).
		WithSystem().
		Only(ctx)
	if err != nil {
		l.serviceByID[trimmedID] = approvalServiceLookup{}
		return approvalServiceLookup{}
	}
	lookup := approvalServiceLookup{
		ServiceID:   serviceRow.ID,
		ServiceName: serviceRow.Name,
	}
	if serviceRow.Edges.System != nil {
		lookup.SystemID = serviceRow.Edges.System.ID
		lookup.SystemName = serviceRow.Edges.System.Name
	}
	l.serviceByID[trimmedID] = lookup
	return lookup
}

func (l *batchSnapshotLoader) template(ctx context.Context, templateID string) *ent.Template {
	trimmedID := strings.TrimSpace(templateID)
	if trimmedID == "" || l == nil || l.server == nil {
		return nil
	}
	if tpl, ok := l.templateByID[trimmedID]; ok {
		return tpl
	}
	tpl, err := l.server.client.Template.Get(ctx, trimmedID)
	if err != nil {
		l.templateByID[trimmedID] = nil
		return nil
	}
	l.templateByID[trimmedID] = tpl
	return tpl
}

func (l *batchSnapshotLoader) instanceSize(ctx context.Context, instanceSizeID string) *ent.InstanceSize {
	trimmedID := strings.TrimSpace(instanceSizeID)
	if trimmedID == "" || l == nil || l.server == nil {
		return nil
	}
	if size, ok := l.instanceSizeByID[trimmedID]; ok {
		return size
	}
	size, err := l.server.client.InstanceSize.Get(ctx, trimmedID)
	if err != nil {
		l.instanceSizeByID[trimmedID] = nil
		return nil
	}
	l.instanceSizeByID[trimmedID] = size
	return size
}

func (l *batchSnapshotLoader) creationPayload(ctx context.Context, ticketID string) domain.VMCreationPayload {
	trimmedID := strings.TrimSpace(ticketID)
	if trimmedID == "" || l == nil || l.server == nil {
		return domain.VMCreationPayload{}
	}
	if payload, ok := l.creationPayloadByTicketID[trimmedID]; ok {
		return payload
	}
	ticket, err := l.server.client.Ticket.Get(ctx, trimmedID)
	if err != nil || strings.TrimSpace(ticket.EventID) == "" {
		l.creationPayloadByTicketID[trimmedID] = domain.VMCreationPayload{}
		return domain.VMCreationPayload{}
	}
	event, err := l.server.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		l.creationPayloadByTicketID[trimmedID] = domain.VMCreationPayload{}
		return domain.VMCreationPayload{}
	}
	var payload domain.VMCreationPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		l.creationPayloadByTicketID[trimmedID] = domain.VMCreationPayload{}
		return domain.VMCreationPayload{}
	}
	l.creationPayloadByTicketID[trimmedID] = payload
	return payload
}

func (l *batchSnapshotLoader) enrichCreatePayload(ctx context.Context, payload *domain.VMCreationPayload) {
	if payload == nil {
		return
	}
	payload.OwnerID = firstNonEmptyString(strings.TrimSpace(payload.OwnerID), strings.TrimSpace(payload.RequesterID))
	payload.OwnerDisplayName, payload.OwnerUsername = l.actorIdentity(ctx, payload.OwnerID)

	serviceLookup := l.serviceLookup(ctx, payload.ServiceID)
	payload.ServiceName = firstNonEmptyString(strings.TrimSpace(payload.ServiceName), strings.TrimSpace(serviceLookup.ServiceName))
	payload.SystemID = firstNonEmptyString(strings.TrimSpace(payload.SystemID), strings.TrimSpace(serviceLookup.SystemID))
	payload.SystemName = firstNonEmptyString(strings.TrimSpace(payload.SystemName), strings.TrimSpace(serviceLookup.SystemName))

	if tpl := l.template(ctx, payload.TemplateID); tpl != nil {
		payload.TemplateName = firstNonEmptyString(strings.TrimSpace(payload.TemplateName), firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID))
	}
	if size := l.instanceSize(ctx, payload.InstanceSizeID); size != nil {
		payload.InstanceSizeName = firstNonEmptyString(strings.TrimSpace(payload.InstanceSizeName), firstNonEmptyString(size.DisplayName, size.Name, size.ID))
		if payload.TargetCPUCores <= 0 {
			payload.TargetCPUCores = size.CPUCores
		}
		if payload.TargetMemoryGi <= 0 {
			payload.TargetMemoryGi = size.MemoryGi
		}
		if payload.TargetDiskGB <= 0 {
			payload.TargetDiskGB = size.DiskGB
		}
	}
}

func (l *batchSnapshotLoader) buildVMContextSnapshot(
	ctx context.Context,
	vmRow *ent.VM,
) batchVMContextSnapshot {
	if vmRow == nil {
		return batchVMContextSnapshot{}
	}

	snapshot := batchVMContextSnapshot{
		OwnerID: strings.TrimSpace(vmRow.CreatedBy),
	}
	snapshot.OwnerDisplayName, snapshot.OwnerUsername = l.actorIdentity(ctx, snapshot.OwnerID)

	if serviceRow := vmRow.Edges.Service; serviceRow != nil {
		snapshot.ServiceID = serviceRow.ID
		snapshot.ServiceName = serviceRow.Name
		if serviceRow.Edges.System != nil {
			snapshot.SystemID = serviceRow.Edges.System.ID
			snapshot.SystemName = serviceRow.Edges.System.Name
		}
	}

	snapshot.ClusterName, snapshot.ClusterEnvironment = l.clusterPresentation(ctx, vmRow.ClusterID)

	createPayload := l.creationPayload(ctx, vmRow.TicketID)
	snapshot.TemplateID = strings.TrimSpace(createPayload.TemplateID)
	snapshot.InstanceSizeID = strings.TrimSpace(createPayload.InstanceSizeID)
	snapshot.TemplateName = strings.TrimSpace(createPayload.TemplateName)
	snapshot.InstanceSizeName = strings.TrimSpace(createPayload.InstanceSizeName)
	snapshot.CurrentCPUCores = createPayload.TargetCPUCores
	snapshot.CurrentMemoryGi = createPayload.TargetMemoryGi
	snapshot.CurrentDiskGB = createPayload.TargetDiskGB

	if snapshot.ServiceID == "" {
		snapshot.ServiceID = strings.TrimSpace(createPayload.ServiceID)
	}
	if snapshot.ServiceName == "" {
		snapshot.ServiceName = strings.TrimSpace(createPayload.ServiceName)
	}
	if snapshot.SystemID == "" {
		snapshot.SystemID = strings.TrimSpace(createPayload.SystemID)
	}
	if snapshot.SystemName == "" {
		snapshot.SystemName = strings.TrimSpace(createPayload.SystemName)
	}
	if snapshot.OwnerDisplayName == "" && strings.TrimSpace(createPayload.OwnerDisplayName) != "" {
		snapshot.OwnerDisplayName = strings.TrimSpace(createPayload.OwnerDisplayName)
	}
	if snapshot.OwnerUsername == "" && strings.TrimSpace(createPayload.OwnerUsername) != "" {
		snapshot.OwnerUsername = strings.TrimSpace(createPayload.OwnerUsername)
	}

	if tpl := l.template(ctx, snapshot.TemplateID); tpl != nil {
		snapshot.TemplateName = firstNonEmptyString(snapshot.TemplateName, firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID))
	}
	if size := l.instanceSize(ctx, snapshot.InstanceSizeID); size != nil {
		snapshot.InstanceSizeName = firstNonEmptyString(snapshot.InstanceSizeName, firstNonEmptyString(size.DisplayName, size.Name, size.ID))
		if snapshot.CurrentCPUCores <= 0 {
			snapshot.CurrentCPUCores = size.CPUCores
		}
		if snapshot.CurrentMemoryGi <= 0 {
			snapshot.CurrentMemoryGi = size.MemoryGi
		}
		if snapshot.CurrentDiskGB <= 0 {
			snapshot.CurrentDiskGB = size.DiskGB
		}
	}

	return snapshot
}

type batchValidationError struct {
	status int
	body   generated.Error
}

func (e *batchValidationError) Error() string {
	return e.body.Code + ": " + e.body.Message
}

// SubmitVMBatch handles POST /vms/batch.
func (s *Server) SubmitVMBatch(c *gin.Context) {
	s.submitBatch(c)
}

// SubmitVMBatchPower handles POST /vms/batch/power compatibility endpoint.
func (s *Server) SubmitVMBatchPower(c *gin.Context) {
	s.submitBatchPower(c)
}

func (s *Server) submitBatch(c *gin.Context) {
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMBatchSubmitRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	if len(req.Items) == 0 || len(req.Items) > maxBatchItems {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_SIZE",
			Message: fmt.Sprintf("batch size must be between 1 and %d", maxBatchItems),
		})
		return
	}

	op, parentEventType, err := normalizeBatchOperation(req.Operation)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_OPERATION", Message: err.Error()})
		return
	}
	switch op {
	case string(generated.VMBatchOperation("DELETE")):
		if !requireGlobalPermission(c, "vm:delete") {
			return
		}
	case string(generated.VMBatchOperation("MODIFY")):
		if !requireGlobalPermission(c, "vm:operate") {
			return
		}
	default:
		if !requireGlobalPermission(c, "vm:create") {
			return
		}
	}

	if strings.TrimSpace(req.RequestId) != "" {
		if existingID, ok, findErr := s.findBatchByRequestID(ctx, actor, op, req.RequestId); findErr != nil {
			logger.Error("failed to query batch idempotency", zap.Error(findErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		} else if ok {
			c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
				BatchId:           existingID,
				Status:            generated.VMBatchParentStatusPENDINGAPPROVAL,
				StatusUrl:         "/api/v1/vms/batch/" + existingID,
				RetryAfterSeconds: batchRetryAfterSeconds,
			})
			return
		}
	}

	globalPending, userPending, err := s.pendingBatchParentCounters(ctx, actor)
	if err != nil {
		logger.Error("failed to evaluate batch submission limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	limitPolicy, err := s.resolveBatchUserLimitPolicy(ctx, actor)
	if err != nil {
		logger.Error("failed to resolve batch user limit policy", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	userPendingParentExceeded := !limitPolicy.Exempt && userPending >= limitPolicy.MaxPendingParents
	if globalPending >= maxPendingBatchParents || userPendingParentExceeded {
		c.Header("Retry-After", strconv.Itoa(batchRetryAfterSeconds))
		contactAdmin := !limitPolicy.Exempt && limitPolicy.UsesDefault
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch submission throttled by pending parent limits",
			Params: map[string]interface{}{
				"retry_after_seconds": batchRetryAfterSeconds,
				"global_pending":      globalPending,
				"user_pending":        userPending,
				"user_exempted":       limitPolicy.Exempt,
				"max_user_pending":    limitPolicy.MaxPendingParents,
				"contact_admin":       contactAdmin,
			},
		})
		return
	}
	if extraLimit, limitErr := s.evaluateAdditionalBatchSubmissionLimits(ctx, actor, len(req.Items), limitPolicy, parentEventType); limitErr != nil {
		logger.Error("failed to evaluate additional batch submission limits", zap.Error(limitErr))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	} else if extraLimit != nil {
		retryAfter := extraLimit.RetryAfterSeconds
		if retryAfter <= 0 {
			retryAfter = batchRetryAfterSeconds
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch submission throttled by additional rate limits",
			Params: map[string]interface{}{
				"reason":                    extraLimit.Reason,
				"retry_after_seconds":       retryAfter,
				"global_recent_submits":     extraLimit.GlobalRecentSubmits,
				"user_pending_children":     extraLimit.UserPendingChildren,
				"user_cooldown_seconds":     extraLimit.UserCooldownSeconds,
				"requested_child_count":     len(req.Items),
				"max_global_per_minute":     maxGlobalBatchRequestsPerMinute,
				"max_user_pending_children": limitPolicy.MaxPendingChildren,
				"user_exempted":             limitPolicy.Exempt,
				"contact_admin":             !limitPolicy.Exempt && limitPolicy.UsesDefault,
			},
		})
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility for batch submit", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	children, err := s.prepareBatchChildren(ctx, c, actor, op, req, visibility)
	if err != nil {
		var appErr *batchValidationError
		if errors.As(err, &appErr) {
			c.JSON(appErr.status, appErr.body)
			return
		}
		logger.Error("failed to prepare batch child tickets", zap.Error(err))
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_ITEMS", Message: err.Error()})
		return
	}

	parentID := generateIDV7()
	parentPayload := domain.BatchVMRequestPayload{
		Operation:   op,
		RequestID:   strings.TrimSpace(req.RequestId),
		Reason:      strings.TrimSpace(req.Reason),
		SubmittedBy: actor,
		SubmittedAt: time.Now().UTC(),
		Items:       buildBatchPayloadItems(op, req.Items, children...),
	}
	parentPayloadBytes, err := parentPayload.ToJSON()
	if err != nil {
		logger.Error("failed to marshal parent batch payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to begin batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()

	parentEventID := generateIDV7()
	_, err = tx.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(parentEventType)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create parent batch domain event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	parentBuilder := tx.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetRequester(actor).
		SetStatus(entticket.StatusPENDING)
	switch op {
	case string(generated.VMBatchOperation("DELETE")):
		parentBuilder = parentBuilder.SetOperationType(entticket.OperationTypeDELETE)
	case string(generated.VMBatchOperation("MODIFY")):
		parentBuilder = parentBuilder.SetOperationType(entticket.OperationTypeMODIFY)
	default:
		parentBuilder = parentBuilder.SetOperationType(entticket.OperationTypeCREATE)
	}
	parentReason := strings.TrimSpace(req.Reason)
	if parentReason == "" {
		parentReason = fmt.Sprintf("batch %s request (%d items)", strings.ToLower(op), len(children))
	}
	parentBuilder = parentBuilder.SetReason(parentReason)
	if _, err := parentBuilder.Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create parent batch ticket", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if _, err := tx.BatchTicket.Create().
		SetID(parentID).
		SetBatchType(toBatchProjectionType(op)).
		SetChildCount(len(children)).
		SetPendingCount(len(children)).
		SetStatus(entbatchticket.StatusPENDING_APPROVAL).
		SetCreatedBy(actor).
		SetReason(parentReason).
		SetNillableRequestID(nillableTrimmed(req.RequestId)).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create batch projection row", zap.Error(err), zap.String("batch_id", parentID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	for _, child := range children {
		childEventID := generateIDV7()
		_, err := tx.DomainEvent.Create().
			SetID(childEventID).
			SetEventType(string(child.eventType)).
			SetAggregateType("vm").
			SetAggregateID(child.aggregateID).
			SetPayload(child.payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy(actor).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create child domain event", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		_, err = tx.Ticket.Create().
			SetID(generateIDV7()).
			SetEventID(childEventID).
			SetOperationType(child.operationType).
			SetStatus(entticket.StatusPENDING).
			SetRequester(actor).
			SetReason(child.reason).
			SetParentTicketID(parentID).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create child ticket", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm.batch.submit", "ticket", parentID, actor, map[string]interface{}{
			"operation":  op,
			"item_count": len(children),
		})
	}

	c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
		BatchId:           parentID,
		Status:            generated.VMBatchParentStatusPENDINGAPPROVAL,
		StatusUrl:         "/api/v1/vms/batch/" + parentID,
		RetryAfterSeconds: batchRetryAfterSeconds,
	})
}

func (s *Server) submitBatchPower(c *gin.Context) {
	ctx := c.Request.Context()
	if !requireGlobalPermission(c, "vm:operate") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	var req generated.VMBatchPowerRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	if len(req.Items) == 0 || len(req.Items) > maxBatchItems {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_SIZE",
			Message: fmt.Sprintf("batch size must be between 1 and %d", maxBatchItems),
		})
		return
	}

	opKey, jobOperation, childEventType, err := normalizeBatchPowerOperation(req.Operation)
	if err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_BATCH_OPERATION",
			Message: err.Error(),
		})
		return
	}

	if strings.TrimSpace(req.RequestId) != "" {
		if existingID, ok, findErr := s.findBatchByRequestID(ctx, actor, opKey, req.RequestId); findErr != nil {
			logger.Error("failed to query power-batch idempotency", zap.Error(findErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		} else if ok {
			status := generated.VMBatchParentStatusINPROGRESS
			if view, _, loadErr := s.loadBatchView(ctx, existingID); loadErr == nil {
				status = view.Status
			}
			c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
				BatchId:           existingID,
				Status:            status,
				StatusUrl:         "/api/v1/vms/batch/" + existingID,
				RetryAfterSeconds: batchRetryAfterSeconds,
			})
			return
		}
	}

	globalPending, userPending, err := s.pendingBatchParentCounters(ctx, actor)
	if err != nil {
		logger.Error("failed to evaluate power-batch submission limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	limitPolicy, err := s.resolveBatchUserLimitPolicy(ctx, actor)
	if err != nil {
		logger.Error("failed to resolve power-batch user limit policy", zap.Error(err), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	userPendingParentExceeded := !limitPolicy.Exempt && userPending >= limitPolicy.MaxPendingParents
	if globalPending >= maxPendingBatchParents || userPendingParentExceeded {
		c.Header("Retry-After", strconv.Itoa(batchRetryAfterSeconds))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch power submission throttled by pending parent limits",
			Params: map[string]interface{}{
				"retry_after_seconds": batchRetryAfterSeconds,
				"global_pending":      globalPending,
				"user_pending":        userPending,
				"user_exempted":       limitPolicy.Exempt,
				"max_user_pending":    limitPolicy.MaxPendingParents,
			},
		})
		return
	}
	if extraLimit, limitErr := s.evaluateAdditionalBatchSubmissionLimits(ctx, actor, len(req.Items), limitPolicy, domain.EventBatchPowerRequested); limitErr != nil {
		logger.Error("failed to evaluate power-batch additional limits", zap.Error(limitErr))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	} else if extraLimit != nil {
		retryAfter := extraLimit.RetryAfterSeconds
		if retryAfter <= 0 {
			retryAfter = batchRetryAfterSeconds
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: "batch power submission throttled by additional rate limits",
			Params: map[string]interface{}{
				"reason":                    extraLimit.Reason,
				"retry_after_seconds":       retryAfter,
				"global_recent_submits":     extraLimit.GlobalRecentSubmits,
				"user_pending_children":     extraLimit.UserPendingChildren,
				"user_cooldown_seconds":     extraLimit.UserCooldownSeconds,
				"requested_child_count":     len(req.Items),
				"max_global_per_minute":     maxGlobalBatchRequestsPerMinute,
				"max_user_pending_children": limitPolicy.MaxPendingChildren,
				"user_exempted":             limitPolicy.Exempt,
			},
		})
		return
	}

	visibility, err := s.resolveNamespaceVisibility(c)
	if err != nil {
		logger.Error("failed to resolve namespace visibility for power-batch submit", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	children, err := s.prepareBatchPowerChildren(ctx, c, actor, jobOperation, childEventType, req, visibility)
	if err != nil {
		var appErr *batchValidationError
		if errors.As(err, &appErr) {
			c.JSON(appErr.status, appErr.body)
			return
		}
		logger.Error("failed to prepare power-batch child tickets", zap.Error(err))
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_BATCH_ITEMS", Message: err.Error()})
		return
	}
	batchRequiresApproval := false
	for _, child := range children {
		if child.requiresApproval {
			batchRequiresApproval = true
			break
		}
	}

	parentID := generateIDV7()
	parentPayload := domain.BatchVMRequestPayload{
		Operation:   opKey,
		RequestID:   strings.TrimSpace(req.RequestId),
		Reason:      strings.TrimSpace(req.Reason),
		SubmittedBy: actor,
		SubmittedAt: time.Now().UTC(),
		Items:       buildBatchPowerPayloadItems(req.Items, children...),
	}
	parentPayloadBytes, err := parentPayload.ToJSON()
	if err != nil {
		logger.Error("failed to marshal power-batch parent payload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		logger.Error("failed to begin power-batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()

	parentEventID := generateIDV7()
	_, err = tx.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayloadBytes).
		SetStatus(func() domainevent.Status {
			if batchRequiresApproval {
				return domainevent.StatusPENDING
			}
			return domainevent.StatusPROCESSING
		}()).
		SetCreatedBy(actor).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch parent domain event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	parentReason := strings.TrimSpace(req.Reason)
	if parentReason == "" {
		parentReason = fmt.Sprintf("batch power %s request (%d items)", strings.ToLower(jobOperation), len(children))
	}
	if _, err := tx.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetOperationType(entticket.OperationTypePOWER).
		SetStatus(func() entticket.Status {
			if batchRequiresApproval {
				return entticket.StatusPENDING
			}
			return entticket.StatusEXECUTING
		}()).
		SetRequester(actor).
		SetReason(parentReason).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch parent ticket", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if _, err := tx.BatchTicket.Create().
		SetID(parentID).
		SetBatchType(entbatchticket.BatchTypeBATCH_POWER).
		SetChildCount(len(children)).
		SetPendingCount(len(children)).
		SetStatus(func() entbatchticket.Status {
			if batchRequiresApproval {
				return entbatchticket.StatusPENDING_APPROVAL
			}
			return entbatchticket.StatusIN_PROGRESS
		}()).
		SetCreatedBy(actor).
		SetReason(parentReason).
		SetNillableRequestID(nillableTrimmed(req.RequestId)).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		logger.Error("failed to create power-batch projection row", zap.Error(err), zap.String("batch_id", parentID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	childEventIDs := make([]string, 0, len(children))
	for _, child := range children {
		childEventID := generateIDV7()
		_, err := tx.DomainEvent.Create().
			SetID(childEventID).
			SetEventType(string(child.eventType)).
			SetAggregateType("vm").
			SetAggregateID(child.aggregateID).
			SetPayload(child.payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy(actor).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create power-batch child domain event", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		if _, err := tx.Ticket.Create().
			SetID(generateIDV7()).
			SetEventID(childEventID).
			SetOperationType(entticket.OperationTypePOWER).
			SetStatus(func() entticket.Status {
				if batchRequiresApproval {
					return entticket.StatusPENDING
				}
				return entticket.StatusEXECUTING
			}()).
			SetRequester(actor).
			SetReason(child.reason).
			SetParentTicketID(parentID).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			logger.Error("failed to create power-batch child ticket", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		childEventIDs = append(childEventIDs, childEventID)
	}

	if err := tx.Commit(); err != nil {
		logger.Error("failed to commit power-batch submission tx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if batchRequiresApproval {
		if s.approvalRouter != nil {
			if _, routerErr := s.approvalRouter.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{
				EventID:   parentID,
				Requester: actor,
				Action:    "batch_power_" + strings.ToLower(jobOperation),
			}); routerErr != nil {
				logger.Warn("approval router SubmitForApproval failed for power batch ticket (already PENDING in DB)",
					zap.String("ticket_id", parentID),
					zap.Error(routerErr),
				)
			}
		}
		if s.notifier != nil {
			s.notifier.OnTicketSubmitted(ctx, parentID, actor, "")
		}
	} else {
		for _, eventID := range childEventIDs {
			if err := s.enqueueBatchPowerJob(ctx, eventID, strings.ToLower(jobOperation)); err != nil {
				logger.Warn("failed to enqueue power-batch child job",
					zap.String("event_id", eventID),
					zap.String("batch_id", parentID),
					zap.Error(err),
				)
				_, _ = s.client.Ticket.Update().
					Where(entticket.EventIDEQ(eventID)).
					SetStatus(entticket.StatusFAILED).
					SetRejectReason("enqueue vm_power job failed").
					Save(ctx)
				_, _ = s.client.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
			}
		}
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "vm.batch.power.submit", "ticket", parentID, actor, map[string]interface{}{
			"operation":  strings.ToLower(jobOperation),
			"item_count": len(children),
		})
	}

	status := generated.VMBatchParentStatusINPROGRESS
	if batchRequiresApproval {
		status = generated.VMBatchParentStatusPENDINGAPPROVAL
	} else if view, _, err := s.loadBatchView(ctx, parentID); err == nil {
		status = view.Status
	}
	c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
		BatchId:           parentID,
		Status:            status,
		StatusUrl:         "/api/v1/vms/batch/" + parentID,
		RetryAfterSeconds: batchRetryAfterSeconds,
	})
}

// GetVMBatch handles GET /vms/batch/{batch_id}.
func (s *Server) GetVMBatch(c *gin.Context, batchID generated.BatchID) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:read", "vm:create", "vm:delete", "vm:operate", "builtin_approval:view") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	resp, _, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, errBatchNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading batch view", zap.Error(err), zap.String("batch_id", batchID))
			return
		}
		logger.Error("failed to load batch view", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !hasPlatformAdmin(c) && resp.CreatedBy != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RetryVMBatch handles POST /vms/batch/{batch_id}/retry.
func (s *Server) RetryVMBatch(c *gin.Context, batchID generated.BatchID) {
	s.mutateBatchChildren(c, batchID, batchActionRetry)
}

// CancelVMBatch handles POST /vms/batch/{batch_id}/cancel.
func (s *Server) CancelVMBatch(c *gin.Context, batchID generated.BatchID) {
	s.mutateBatchChildren(c, batchID, batchActionCancel)
}

func (s *Server) mutateBatchChildren(c *gin.Context, batchID, action string) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:create", "vm:delete", "vm:operate", "builtin_approval:approve") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	resp, children, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, errBatchNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading batch for action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
			return
		}
		logger.Error("failed to load batch for action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !hasPlatformAdmin(c) && resp.CreatedBy != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
		return
	}
	parentTicket, err := s.client.Ticket.Get(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading parent batch ticket", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
			return
		}
		logger.Error("failed to load parent batch ticket", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	parentEvent, err := s.client.DomainEvent.Get(ctx, parentTicket.EventID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading parent batch event", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
			return
		}
		logger.Error("failed to load parent batch event", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	isPowerBatch := domain.EventType(parentEvent.EventType) == domain.EventBatchPowerRequested

	targetIDs := make([]string, 0)
	targetEventIDs := make([]string, 0)
	targetChildren := make([]*ent.Ticket, 0)
	for _, child := range children {
		switch action {
		case batchActionRetry:
			if child.Status == entticket.StatusFAILED || child.Status == entticket.StatusREJECTED {
				targetIDs = append(targetIDs, child.ID)
				targetEventIDs = append(targetEventIDs, child.EventID)
				targetChildren = append(targetChildren, child)
			}
		case batchActionCancel:
			if child.Status == entticket.StatusPENDING {
				targetIDs = append(targetIDs, child.ID)
				targetEventIDs = append(targetEventIDs, child.EventID)
				targetChildren = append(targetChildren, child)
			}
		}
	}
	if len(targetIDs) == 0 {
		var errCode string
		var errMessage string
		switch action {
		case batchActionRetry:
			errCode = "BATCH_NOTHING_TO_RETRY"
			errMessage = "no failed items are currently available for retry"
		case batchActionCancel:
			errCode = "BATCH_NOTHING_TO_CANCEL"
			errMessage = "no pending items are currently available for cancel"
		default:
			errCode = "BATCH_ACTION_NOT_APPLICABLE"
			errMessage = "batch action is not applicable to the current child states"
		}
		c.JSON(http.StatusConflict, generated.Error{
			Code:    errCode,
			Message: errMessage,
			Params: map[string]interface{}{
				"batch_id":         batchID,
				"batch_status":     resp.Status,
				"child_count":      resp.ChildCount,
				"success_count":    resp.SuccessCount,
				"failed_count":     resp.FailedCount,
				"pending_count":    resp.PendingCount,
				"requested_action": action,
			},
		})
		return
	}

	affectedCount := 0
	affectedTicketIDs := make([]string, 0)
	if action == batchActionRetry {
		retryStatus := entticket.StatusPENDING
		if isPowerBatch {
			retryStatus = entticket.StatusEXECUTING
		}
		if _, updateTicketErr := s.client.Ticket.Update().
			Where(entticket.IDIn(targetIDs...)).
			SetStatus(retryStatus).
			ClearRejectReason().
			Save(ctx); updateTicketErr != nil {
			if isRequestContextCanceled(updateTicketErr) {
				logger.Debug("request canceled while resetting child tickets for retry", zap.Error(updateTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
				return
			}
			logger.Error("failed to reset child tickets for retry", zap.Error(updateTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if _, updateEventErr := s.client.DomainEvent.Update().
			Where(domainevent.IDIn(targetEventIDs...)).
			SetStatus(domainevent.StatusPENDING).
			Save(ctx); updateEventErr != nil {
			if isRequestContextCanceled(updateEventErr) {
				logger.Debug("request canceled while resetting child events for retry", zap.Error(updateEventErr), zap.String("batch_id", batchID), zap.String("action", action))
				return
			}
			logger.Error("failed to reset child events for retry", zap.Error(updateEventErr), zap.String("batch_id", batchID), zap.String("action", action))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}

		for _, child := range targetChildren {
			if isPowerBatch {
				ev, eventErr := s.client.DomainEvent.Get(ctx, child.EventID)
				if eventErr != nil {
					_, _ = s.client.Ticket.UpdateOneID(child.ID).
						SetStatus(entticket.StatusFAILED).
						SetRejectReason("failed to load child event during power retry").
						Save(ctx)
					_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
						SetStatus(domainevent.StatusFAILED).
						Save(ctx)
					logger.Warn("failed to load child event during power-batch retry",
						zap.String("ticket_id", child.ID),
						zap.String("batch_id", batchID),
						zap.Error(eventErr),
					)
					continue
				}
				var powerPayload domain.VMPowerPayload
				if decodeErr := json.Unmarshal(ev.Payload, &powerPayload); decodeErr != nil {
					_, _ = s.client.Ticket.UpdateOneID(child.ID).
						SetStatus(entticket.StatusFAILED).
						SetRejectReason("invalid power payload for retry").
						Save(ctx)
					_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
						SetStatus(domainevent.StatusFAILED).
						Save(ctx)
					logger.Warn("failed to parse child power payload during retry",
						zap.String("ticket_id", child.ID),
						zap.String("batch_id", batchID),
						zap.Error(decodeErr),
					)
					continue
				}
				op := strings.ToLower(strings.TrimSpace(powerPayload.Operation))
				if op != "start" && op != "stop" && op != "restart" {
					_, _ = s.client.Ticket.UpdateOneID(child.ID).
						SetStatus(entticket.StatusFAILED).
						SetRejectReason("unknown power operation for retry").
						Save(ctx)
					_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
						SetStatus(domainevent.StatusFAILED).
						Save(ctx)
					logger.Warn("unknown power operation in child payload during retry",
						zap.String("ticket_id", child.ID),
						zap.String("batch_id", batchID),
						zap.String("operation", powerPayload.Operation),
					)
					continue
				}
				if enqueueErr := s.enqueueBatchPowerJob(ctx, child.EventID, op); enqueueErr != nil {
					_, _ = s.client.Ticket.UpdateOneID(child.ID).
						SetStatus(entticket.StatusFAILED).
						SetRejectReason("failed to enqueue vm_power job").
						Save(ctx)
					_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
						SetStatus(domainevent.StatusFAILED).
						Save(ctx)
					logger.Warn("failed to enqueue power child during batch retry",
						zap.String("ticket_id", child.ID),
						zap.String("batch_id", batchID),
						zap.Error(enqueueErr),
					)
					continue
				}
				affectedCount++
				affectedTicketIDs = append(affectedTicketIDs, child.ID)
				continue
			}
			approveErr := fmt.Errorf("approval provider is not configured")
			if s.approvalRouter != nil {
				approveErr = s.approvalRouter.ProcessApproval(ctx, child.ID, approvalcontract.ApprovalDecision{
					Approved: true,
					Approver: actor,
					Execution: approvalcontract.ApprovalExecutionOptions{
						ClusterID:    parentTicket.SelectedClusterID,
						StorageClass: parentTicket.SelectedStorageClass,
					},
				})
			}
			if approveErr != nil {
				message := approveErr.Error()
				if len(message) > 512 {
					message = message[:512]
				}
				_, _ = s.client.Ticket.UpdateOneID(child.ID).
					SetStatus(entticket.StatusFAILED).
					SetRejectReason(message).
					Save(ctx)
				_, _ = s.client.DomainEvent.UpdateOneID(child.EventID).
					SetStatus(domainevent.StatusFAILED).
					Save(ctx)
				logger.Warn("failed to re-approve child ticket during batch retry",
					zap.String("ticket_id", child.ID),
					zap.String("batch_id", batchID),
					zap.Error(approveErr),
				)
				continue
			}
			affectedCount++
			affectedTicketIDs = append(affectedTicketIDs, child.ID)
		}
	} else {
		if _, cancelTicketErr := s.client.Ticket.Update().
			Where(entticket.IDIn(targetIDs...)).
			SetStatus(entticket.StatusCANCELLED).
			Save(ctx); cancelTicketErr != nil {
			if isRequestContextCanceled(cancelTicketErr) {
				logger.Debug("request canceled while mutating child tickets", zap.Error(cancelTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
				return
			}
			logger.Error("failed to mutate child tickets", zap.Error(cancelTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		if _, cancelEventErr := s.client.DomainEvent.Update().
			Where(domainevent.IDIn(targetEventIDs...)).
			SetStatus(domainevent.StatusCANCELLED).
			Save(ctx); cancelEventErr != nil {
			if isRequestContextCanceled(cancelEventErr) {
				logger.Debug("request canceled while mutating child events", zap.Error(cancelEventErr), zap.String("batch_id", batchID), zap.String("action", action))
				return
			}
			logger.Error("failed to mutate child events", zap.Error(cancelEventErr), zap.String("batch_id", batchID), zap.String("action", action))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		affectedCount = len(targetIDs)
		affectedTicketIDs = append(affectedTicketIDs, targetIDs...)
	}
	jobs.SyncParentBatchStatus(ctx, s.client, batchID)

	updated, _, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while reloading batch after action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
			return
		}
		logger.Error("failed to reload batch after action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, generated.VMBatchActionResponse{
		BatchId:           batchID,
		Status:            updated.Status,
		AffectedCount:     affectedCount,
		AffectedTicketIds: affectedTicketIDs,
	})
}

func (s *Server) prepareBatchChildren(
	ctx context.Context,
	c *gin.Context,
	actor string,
	op string,
	req generated.VMBatchSubmitRequest,
	visibility namespaceVisibility,
) ([]preparedBatchChild, error) {
	snapshotLoader := newBatchSnapshotLoader(s)
	children := make([]preparedBatchChild, 0, len(req.Items))

	for idx, item := range req.Items {
		itemReason := strings.TrimSpace(item.Reason)
		if itemReason == "" {
			itemReason = strings.TrimSpace(req.Reason)
		}
		if itemReason == "" {
			itemReason = fmt.Sprintf("batch %s item #%d", strings.ToLower(op), idx+1)
		}

		switch op {
		case string(generated.VMBatchOperation("CREATE")):
			serviceID := strings.TrimSpace(item.ServiceId.String())
			templateID := strings.TrimSpace(item.TemplateId.String())
			instanceSizeID := strings.TrimSpace(item.InstanceSizeId.String())
			namespace := strings.TrimSpace(item.Namespace)
			if isZeroUUID(item.ServiceId) || isZeroUUID(item.TemplateId) || isZeroUUID(item.InstanceSizeId) || namespace == "" {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_BATCH_ITEM",
						Message: fmt.Sprintf("create item #%d requires service_id/template_id/instance_size_id/namespace", idx+1),
					},
				}
			}
			serviceRow, allowed, err := s.serviceAccessibleForAction(ctx, c, serviceID, "create")
			if err != nil {
				return nil, err
			}
			if serviceRow == nil || !allowed {
				return nil, &batchValidationError{
					status: http.StatusNotFound,
					body: generated.Error{
						Code:    "SERVICE_NOT_FOUND",
						Message: fmt.Sprintf("service %q not found", serviceID),
					},
				}
			}
			visible, err := s.isNamespaceVisible(ctx, namespace, visibility)
			if err != nil {
				return nil, err
			}
			if !visible {
				return nil, &batchValidationError{
					status: http.StatusForbidden,
					body: generated.Error{
						Code:    "NAMESPACE_ENV_FORBIDDEN",
						Message: fmt.Sprintf("namespace %q is outside allowed environment visibility", namespace),
					},
				}
			}
			targetCPU := normalizeOptionalTargetFloat64(float64(item.TargetCpuCores))
			targetMemory := normalizeOptionalTargetFloat64(float64(item.TargetMemoryGi))
			targetDisk := normalizeOptionalTargetInt(item.TargetDiskGb)
			if validateErr := service.ValidateVMRequestTargets(service.VMRequestTargets{
				TargetCPUCores: targetCPU,
				TargetMemoryGi: targetMemory,
				TargetDiskGB:   targetDisk,
			}); validateErr != nil {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_RESOURCE_TARGET",
						Message: fmt.Sprintf("create item #%d has invalid resource target: %v", idx+1, validateErr),
					},
				}
			}
			payload := domain.VMCreationPayload{
				RequesterID:    actor,
				ServiceID:      serviceID,
				TemplateID:     templateID,
				InstanceSizeID: instanceSizeID,
				Namespace:      namespace,
				Reason:         itemReason,
				TargetCPUCores: derefFloat64(targetCPU),
				TargetMemoryGi: derefFloat64(targetMemory),
				TargetDiskGB:   derefInt(targetDisk),
			}
			snapshotLoader.enrichCreatePayload(ctx, &payload)
			payloadBytes, err := payload.ToJSON()
			if err != nil {
				return nil, err
			}
			children = append(children, preparedBatchChild{
				eventType:     domain.EventVMCreationRequested,
				aggregateID:   serviceID,
				payload:       payloadBytes,
				operationType: entticket.OperationTypeCREATE,
				reason:        itemReason,
			})

		case string(generated.VMBatchOperation("DELETE")):
			vmID := strings.TrimSpace(item.VmId)
			if vmID == "" {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_BATCH_ITEM",
						Message: fmt.Sprintf("delete item #%d requires vm_id", idx+1),
					},
				}
			}
			vmObj, err := s.client.VM.Query().
				Where(entvm.IDEQ(vmID)).
				WithService(func(query *ent.ServiceQuery) {
					query.WithSystem()
				}).
				Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					return nil, &batchValidationError{
						status: http.StatusBadRequest,
						body: generated.Error{
							Code:    "VM_NOT_FOUND",
							Message: fmt.Sprintf("vm %q not found", vmID),
						},
					}
				}
				return nil, err
			}
			visible, err := s.vmAccessibleForActionWithVisibility(ctx, c, vmObj.ID, vmObj.Namespace, "create", visibility)
			if err != nil {
				return nil, err
			}
			if !visible {
				return nil, &batchValidationError{
					status: http.StatusNotFound,
					body: generated.Error{
						Code:    "VM_NOT_FOUND",
						Message: fmt.Sprintf("vm %q not found", vmID),
					},
				}
			}
			snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vmObj)
			vmObj = s.refreshVMLiveState(ctx, vmObj)
			if !usecase.VMDeleteAllowedStatus(vmObj.Status) {
				return nil, &batchValidationError{
					status: http.StatusConflict,
					body: generated.Error{
						Code:    usecase.VMDeleteInvalidStateCode,
						Message: usecase.VMDeleteInvalidStateMessage(vmObj.Status),
						Params:  usecase.VMDeleteInvalidStateParams(vmObj.Status),
					},
				}
			}
			payload := domain.VMDeletePayload{
				VMID:               vmObj.ID,
				VMName:             vmObj.Name,
				ClusterID:          vmObj.ClusterID,
				ClusterName:        snapshot.ClusterName,
				ClusterEnvironment: snapshot.ClusterEnvironment,
				Namespace:          vmObj.Namespace,
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
				RequestVMStatus:    string(vmObj.Status),
				CurrentCPUCores:    snapshot.CurrentCPUCores,
				CurrentMemoryGi:    snapshot.CurrentMemoryGi,
				CurrentDiskGB:      snapshot.CurrentDiskGB,
				Actor:              actor,
			}
			payloadBytes, err := payload.ToJSON()
			if err != nil {
				return nil, err
			}
			children = append(children, preparedBatchChild{
				eventType:     domain.EventVMDeletionRequested,
				aggregateID:   vmObj.ID,
				payload:       payloadBytes,
				operationType: entticket.OperationTypeDELETE,
				reason:        itemReason,
			})

		case string(generated.VMBatchOperation("MODIFY")):
			vmID := strings.TrimSpace(item.VmId)
			if vmID == "" {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "INVALID_BATCH_ITEM",
						Message: fmt.Sprintf("modify item #%d requires vm_id", idx+1),
					},
				}
			}
			vmObj, err := s.client.VM.Query().
				Where(entvm.IDEQ(vmID)).
				WithService(func(query *ent.ServiceQuery) {
					query.WithSystem()
				}).
				Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					return nil, &batchValidationError{
						status: http.StatusBadRequest,
						body: generated.Error{
							Code:    "VM_NOT_FOUND",
							Message: fmt.Sprintf("vm %q not found", vmID),
						},
					}
				}
				return nil, err
			}
			visible, err := s.vmAccessibleForActionWithVisibility(ctx, c, vmObj.ID, vmObj.Namespace, "create", visibility)
			if err != nil {
				return nil, err
			}
			if !visible {
				return nil, &batchValidationError{
					status: http.StatusNotFound,
					body: generated.Error{
						Code:    "VM_NOT_FOUND",
						Message: fmt.Sprintf("vm %q not found", vmID),
					},
				}
			}

			snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vmObj)
			payload, err := s.buildVMModifyPayload(ctx, vmObj, actor, generated.VMModifyRequest{
				Reason:         itemReason,
				TargetCpuCores: item.TargetCpuCores,
				TargetMemoryGi: item.TargetMemoryGi,
				TargetDiskGb:   item.TargetDiskGb,
			})
			if err != nil {
				return nil, &batchValidationError{
					status: http.StatusConflict,
					body: generated.Error{
						Code:    "VM_MODIFY_REQUEST_INVALID",
						Message: err.Error(),
					},
				}
			}
			payload.SystemID = snapshot.SystemID
			payload.SystemName = snapshot.SystemName
			payload.ServiceID = snapshot.ServiceID
			payload.ServiceName = snapshot.ServiceName
			payload.OwnerID = snapshot.OwnerID
			payload.OwnerDisplayName = snapshot.OwnerDisplayName
			payload.OwnerUsername = snapshot.OwnerUsername
			payload.ClusterName = firstNonEmptyString(payload.ClusterName, snapshot.ClusterName)
			payload.ClusterEnvironment = firstNonEmptyString(payload.ClusterEnvironment, snapshot.ClusterEnvironment)
			payload.TemplateID = firstNonEmptyString(payload.TemplateID, snapshot.TemplateID)
			payload.TemplateName = firstNonEmptyString(payload.TemplateName, snapshot.TemplateName)
			payload.InstanceSizeID = firstNonEmptyString(payload.InstanceSizeID, snapshot.InstanceSizeID)
			payload.InstanceSizeName = firstNonEmptyString(payload.InstanceSizeName, snapshot.InstanceSizeName)
			payloadBytes, err := payload.ToJSON()
			if err != nil {
				return nil, err
			}
			children = append(children, preparedBatchChild{
				eventType:     domain.EventVMModifyRequested,
				aggregateID:   vmObj.ID,
				payload:       payloadBytes,
				operationType: entticket.OperationTypeMODIFY,
				reason:        itemReason,
			})
		}
	}

	return children, nil
}

func normalizeBatchOperation(op generated.VMBatchOperation) (string, domain.EventType, error) {
	switch op {
	case generated.VMBatchOperation("CREATE"):
		return string(op), domain.EventBatchCreateRequested, nil
	case generated.VMBatchOperation("MODIFY"):
		return string(op), domain.EventBatchModifyRequested, nil
	case generated.VMBatchOperation("DELETE"):
		return string(op), domain.EventBatchDeleteRequested, nil
	default:
		return "", "", fmt.Errorf("unsupported operation %q", op)
	}
}

func normalizeBatchPowerOperation(op generated.VMBatchPowerAction) (opKey, jobOperation string, childEventType domain.EventType, err error) {
	switch strings.TrimSpace(strings.ToUpper(string(op))) {
	case "START":
		return "POWER_START", "START", domain.EventVMStartRequested, nil
	case "STOP":
		return "POWER_STOP", "STOP", domain.EventVMStopRequested, nil
	case "RESTART":
		return "POWER_RESTART", "RESTART", domain.EventVMRestartRequested, nil
	default:
		return "", "", "", fmt.Errorf("unsupported power operation %q", op)
	}
}

func (s *Server) prepareBatchPowerChildren(
	ctx context.Context,
	c *gin.Context,
	actor string,
	jobOperation string,
	childEventType domain.EventType,
	req generated.VMBatchPowerRequest,
	visibility namespaceVisibility,
) ([]preparedBatchChild, error) {
	snapshotLoader := newBatchSnapshotLoader(s)
	children := make([]preparedBatchChild, 0, len(req.Items))
	for idx, item := range req.Items {
		vmID := strings.TrimSpace(item.VmId)
		if vmID == "" {
			return nil, &batchValidationError{
				status: http.StatusBadRequest,
				body: generated.Error{
					Code:    "INVALID_BATCH_ITEM",
					Message: fmt.Sprintf("power item #%d requires vm_id", idx+1),
				},
			}
		}
		vmObj, err := s.client.VM.Query().
			Where(entvm.IDEQ(vmID)).
			WithService(func(query *ent.ServiceQuery) {
				query.WithSystem()
			}).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "VM_NOT_FOUND",
						Message: fmt.Sprintf("vm %q not found", vmID),
					},
				}
			}
			return nil, err
		}
		visible, err := s.vmAccessibleForActionWithVisibility(ctx, c, vmObj.ID, vmObj.Namespace, "create", visibility)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, &batchValidationError{
				status: http.StatusNotFound,
				body: generated.Error{
					Code:    "VM_NOT_FOUND",
					Message: fmt.Sprintf("vm %q not found", vmID),
				},
			}
		}
		snapshot := snapshotLoader.buildVMContextSnapshot(ctx, vmObj)
		vmObj = s.refreshVMLiveState(ctx, vmObj)

		itemReason := strings.TrimSpace(item.Reason)
		if itemReason == "" {
			itemReason = strings.TrimSpace(req.Reason)
		}
		if itemReason == "" {
			itemReason = fmt.Sprintf("batch power item #%d", idx+1)
		}
		requiresApproval, err := s.requiresPowerApproval(ctx, vmObj.Namespace, strings.ToLower(jobOperation))
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, &batchValidationError{
					status: http.StatusBadRequest,
					body: generated.Error{
						Code:    "NAMESPACE_NOT_REGISTERED",
						Message: fmt.Sprintf("vm namespace %q is not registered in namespace_registry", vmObj.Namespace),
					},
				}
			}
			return nil, err
		}

		payload := domain.VMPowerPayload{
			VMID:               vmObj.ID,
			VMName:             vmObj.Name,
			ClusterID:          vmObj.ClusterID,
			ClusterName:        snapshot.ClusterName,
			ClusterEnvironment: snapshot.ClusterEnvironment,
			Namespace:          vmObj.Namespace,
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
			RequestVMStatus:    string(vmObj.Status),
			CurrentCPUCores:    snapshot.CurrentCPUCores,
			CurrentMemoryGi:    snapshot.CurrentMemoryGi,
			CurrentDiskGB:      snapshot.CurrentDiskGB,
			Operation:          strings.ToLower(jobOperation),
			Actor:              actor,
		}
		payloadBytes, err := payload.ToJSON()
		if err != nil {
			return nil, err
		}
		children = append(children, preparedBatchChild{
			eventType:        childEventType,
			aggregateID:      vmObj.ID,
			payload:          payloadBytes,
			operationType:    entticket.OperationTypePOWER,
			reason:           itemReason,
			requiresApproval: requiresApproval,
		})
	}

	return children, nil
}

func (s *Server) enqueueBatchPowerJob(ctx context.Context, eventID, operation string) error {
	if s.riverClient == nil {
		return fmt.Errorf("river client is not configured")
	}
	_, err := s.riverClient.Insert(ctx, jobs.VMPowerArgs{
		EventID:   eventID,
		Operation: operation,
	}, nil)
	return err
}

func batchParentEventTypes() []string {
	return []string{
		string(domain.EventBatchCreateRequested),
		string(domain.EventBatchModifyRequested),
		string(domain.EventBatchDeleteRequested),
		string(domain.EventBatchPowerRequested),
	}
}

func (s *Server) pendingBatchParentCounters(ctx context.Context, actor string) (global, user int, err error) {
	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.StatusIn(domainevent.StatusPENDING, domainevent.StatusPROCESSING),
		).
		All(ctx)
	if err != nil {
		return 0, 0, err
	}

	global = len(events)
	user = 0
	for _, ev := range events {
		if ev.CreatedBy == actor {
			user++
		}
	}
	return global, user, nil
}

type batchSubmissionLimitViolation struct {
	Reason              string
	RetryAfterSeconds   int
	GlobalRecentSubmits int
	UserPendingChildren int
	UserCooldownSeconds int
}

type batchUserLimitPolicy struct {
	Exempt             bool
	UsesDefault        bool
	MaxPendingParents  int
	MaxPendingChildren int
	Cooldown           time.Duration
	ExemptionExpiresAt *time.Time
}

func defaultBatchUserLimitPolicy() batchUserLimitPolicy {
	return batchUserLimitPolicy{
		Exempt:             false,
		UsesDefault:        true,
		MaxPendingParents:  maxPendingBatchParentsUser,
		MaxPendingChildren: maxPendingBatchChildrenUser,
		Cooldown:           batchSubmitCooldown,
	}
}

func (s *Server) resolveBatchUserLimitPolicy(ctx context.Context, actor string) (batchUserLimitPolicy, error) {
	policy := defaultBatchUserLimitPolicy()

	exemption, err := s.client.RateLimitExemption.Query().
		Where(ratelimitexemption.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return policy, err
	}
	if err == nil {
		if exemption.ExpiresAt != nil && exemption.ExpiresAt.Before(time.Now().UTC()) {
			if delErr := s.client.RateLimitExemption.DeleteOneID(actor).Exec(ctx); delErr != nil {
				logger.Warn("failed to purge expired rate-limit exemption",
					zap.String("user_id", actor),
					zap.Error(delErr),
				)
			}
		} else {
			policy.Exempt = true
			policy.UsesDefault = false
			if exemption.ExpiresAt != nil {
				exp := *exemption.ExpiresAt
				policy.ExemptionExpiresAt = &exp
			}
		}
	}

	override, err := s.client.RateLimitUserOverride.Query().
		Where(ratelimituseroverride.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return policy, err
	}
	if err == nil {
		policy.UsesDefault = false
		if override.MaxPendingParents != nil && *override.MaxPendingParents > 0 {
			policy.MaxPendingParents = *override.MaxPendingParents
		}
		if override.MaxPendingChildren != nil && *override.MaxPendingChildren > 0 {
			policy.MaxPendingChildren = *override.MaxPendingChildren
		}
		if override.CooldownSeconds != nil && *override.CooldownSeconds >= 0 {
			policy.Cooldown = time.Duration(*override.CooldownSeconds) * time.Second
		}
	}

	return policy, nil
}

func (s *Server) evaluateAdditionalBatchSubmissionLimits(
	ctx context.Context,
	actor string,
	requestedChildCount int,
	policy batchUserLimitPolicy,
	cooldownEventType domain.EventType,
) (*batchSubmissionLimitViolation, error) {
	recentSince := time.Now().UTC().Add(-time.Minute)
	globalRecentSubmits, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedAtGTE(recentSince),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if globalRecentSubmits >= maxGlobalBatchRequestsPerMinute {
		return &batchSubmissionLimitViolation{
			Reason:              "global_request_rate_limit",
			RetryAfterSeconds:   60,
			GlobalRecentSubmits: globalRecentSubmits,
		}, nil
	}

	if policy.Exempt {
		return nil, nil
	}

	userPendingChildren, err := s.client.Ticket.Query().
		Where(
			entticket.RequesterEQ(actor),
			entticket.ParentTicketIDNotNil(),
			entticket.StatusIn(
				entticket.StatusPENDING,
				entticket.StatusAPPROVED,
				entticket.StatusEXECUTING,
			),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if userPendingChildren+requestedChildCount > policy.MaxPendingChildren {
		return &batchSubmissionLimitViolation{
			Reason:              "user_pending_child_limit",
			RetryAfterSeconds:   batchRetryAfterSeconds,
			GlobalRecentSubmits: globalRecentSubmits,
			UserPendingChildren: userPendingChildren,
		}, nil
	}

	lastEvent, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeEQ(string(cooldownEventType)),
			domainevent.CreatedByEQ(actor),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		cooldownRemaining := time.Until(lastEvent.CreatedAt.Add(policy.Cooldown))
		if cooldownRemaining > 0 {
			cooldownSeconds := int(math.Ceil(cooldownRemaining.Seconds()))
			return &batchSubmissionLimitViolation{
				Reason:              "user_submit_cooldown",
				RetryAfterSeconds:   cooldownSeconds,
				GlobalRecentSubmits: globalRecentSubmits,
				UserPendingChildren: userPendingChildren,
				UserCooldownSeconds: cooldownSeconds,
			}, nil
		}
	}

	return nil, nil
}

func (s *Server) findBatchByRequestID(ctx context.Context, actor, op, requestID string) (batchID string, found bool, err error) {
	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedByEQ(actor),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return "", false, err
	}

	for _, ev := range events {
		var payload domain.BatchVMRequestPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.RequestID) != requestID || strings.TrimSpace(payload.Operation) != op {
			continue
		}
		parentExists, err := s.client.Ticket.Query().
			Where(
				entticket.IDEQ(ev.AggregateID),
				entticket.EventIDEQ(ev.ID),
				entticket.ParentTicketIDIsNil(),
			).
			Exist(ctx)
		if err != nil {
			return "", false, err
		}
		if parentExists {
			return ev.AggregateID, true, nil
		}
	}

	return "", false, nil
}

func (s *Server) loadBatchView(ctx context.Context, batchID string) (generated.VMBatchStatusResponse, []*ent.Ticket, error) {
	parent, err := s.client.Ticket.Query().
		Where(
			entticket.IDEQ(batchID),
			entticket.ParentTicketIDIsNil(),
		).
		Only(ctx)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}

	parentEvent, err := s.client.DomainEvent.Get(ctx, parent.EventID)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}
	projection, err := s.client.BatchTicket.Get(ctx, parent.ID)
	if err != nil && !ent.IsNotFound(err) {
		return generated.VMBatchStatusResponse{}, nil, err
	}

	var operation generated.VMBatchOperation
	switch domain.EventType(parentEvent.EventType) {
	case domain.EventBatchDeleteRequested:
		operation = generated.VMBatchOperation("DELETE")
	case domain.EventBatchCreateRequested:
		operation = generated.VMBatchOperation("CREATE")
	case domain.EventBatchModifyRequested:
		operation = generated.VMBatchOperation("MODIFY")
	case domain.EventBatchPowerRequested:
		operation = generated.VMBatchOperation("POWER")
	default:
		return generated.VMBatchStatusResponse{}, nil, errBatchNotFound
	}

	children, err := s.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		Order(ent.Asc(entticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return generated.VMBatchStatusResponse{}, nil, err
	}
	createChildTicketIDs := make([]string, 0, len(children))
	for _, child := range children {
		if child.OperationType == entticket.OperationTypeCREATE {
			createChildTicketIDs = append(createChildTicketIDs, child.ID)
		}
	}
	createVMByTicketID := make(map[string]*ent.VM)
	if len(createChildTicketIDs) > 0 {
		vms, vmErr := s.client.VM.Query().
			Where(entvm.TicketIDIn(createChildTicketIDs...)).
			All(ctx)
		if vmErr != nil {
			return generated.VMBatchStatusResponse{}, nil, vmErr
		}
		for _, vm := range vms {
			if vm == nil || strings.TrimSpace(vm.TicketID) == "" {
				continue
			}
			createVMByTicketID[vm.TicketID] = vm
		}
	}

	eventIDs := make([]string, 0, len(children))
	for _, child := range children {
		eventIDs = append(eventIDs, child.EventID)
	}
	eventByID := map[string]*ent.DomainEvent{}
	if len(eventIDs) > 0 {
		events, eventsErr := s.client.DomainEvent.Query().Where(domainevent.IDIn(eventIDs...)).All(ctx)
		if eventsErr != nil {
			return generated.VMBatchStatusResponse{}, nil, eventsErr
		}
		for _, ev := range events {
			eventByID[ev.ID] = ev
		}
	}

	var (
		successCount int
		failedCount  int
		pendingCount int
		cancelled    int
		pendingOnly  int
		executing    int
	)
	childStatuses := make([]generated.VMBatchChildStatus, 0, len(children))
	for _, child := range children {
		switch child.Status {
		case entticket.StatusSUCCESS:
			successCount++
		case entticket.StatusFAILED, entticket.StatusREJECTED:
			failedCount++
		case entticket.StatusCANCELLED:
			cancelled++
		case entticket.StatusPENDING:
			pendingCount++
			pendingOnly++
		default:
			pendingCount++
			executing++
		}

		resourceID := ""
		resourceName := ""
		lastError := strings.TrimSpace(child.RejectReason)
		if ev := eventByID[child.EventID]; ev != nil {
			resourceID = strings.TrimSpace(ev.AggregateID)
			switch domain.EventType(ev.EventType) {
			case domain.EventVMDeletionRequested:
				var payload domain.VMDeletePayload
				if decodeErr := json.Unmarshal(ev.Payload, &payload); decodeErr == nil {
					if strings.TrimSpace(payload.VMName) != "" {
						resourceName = payload.VMName
					}
				}
			case domain.EventVMModifyRequested:
				var payload domain.VMModifyPayload
				if decodeErr := json.Unmarshal(ev.Payload, &payload); decodeErr == nil {
					if strings.TrimSpace(payload.VMName) != "" {
						resourceName = payload.VMName
					}
				}
			case domain.EventVMCreationRequested:
				var payload domain.VMCreationPayload
				if decodeErr := json.Unmarshal(ev.Payload, &payload); decodeErr == nil {
					if resourceID == "" {
						resourceID = strings.TrimSpace(payload.ServiceID)
					}
				}
			default:
				// Other event types don't encode VM name/ID in a known payload shape.
			}
		}

		attemptCount := 0
		if child.Status != entticket.StatusPENDING {
			attemptCount = 1
		}

		childStatuses = append(childStatuses, generated.VMBatchChildStatus{
			TicketId:     child.ID,
			EventId:      child.EventID,
			Status:       generated.VMBatchChildStatusStatus(child.Status),
			ResourceId:   resourceID,
			ResourceName: resourceName,
			LastError:    lastError,
			AttemptCount: attemptCount,
			Provisioning: s.loadVMProvisioning(ctx, createVMByTicketID[child.ID]),
		})
	}

	status := aggregateBatchParentStatus(len(children), successCount, failedCount, pendingCount, pendingOnly, executing, cancelled)
	projectionStatus := mapProjectionStatus(status)
	if projection == nil {
		createBuilder := s.client.BatchTicket.Create().
			SetID(parent.ID).
			SetBatchType(toBatchProjectionType(string(operation))).
			SetChildCount(len(children)).
			SetSuccessCount(successCount).
			SetFailedCount(failedCount).
			SetPendingCount(pendingCount).
			SetStatus(projectionStatus).
			SetCreatedBy(parent.Requester).
			SetReason(parent.Reason)
		if _, saveErr := createBuilder.Save(ctx); saveErr != nil && !ent.IsConstraintError(saveErr) {
			logger.Warn("failed to backfill batch projection row", zap.String("batch_id", parent.ID), zap.Error(saveErr))
		}
	} else {
		_, err = s.client.BatchTicket.UpdateOneID(parent.ID).
			SetChildCount(len(children)).
			SetSuccessCount(successCount).
			SetFailedCount(failedCount).
			SetPendingCount(pendingCount).
			SetStatus(projectionStatus).
			Save(ctx)
		if err != nil {
			logger.Warn("failed to sync batch projection counters", zap.String("batch_id", parent.ID), zap.Error(err))
		}
	}

	response := generated.VMBatchStatusResponse{
		BatchId:      parent.ID,
		Operation:    operation,
		Status:       status,
		ChildCount:   len(children),
		SuccessCount: successCount,
		FailedCount:  failedCount,
		PendingCount: pendingCount,
		Children:     childStatuses,
		CreatedBy:    parent.Requester,
		CreatedAt:    parent.CreatedAt,
		UpdatedAt:    parent.UpdatedAt,
	}
	return response, children, nil
}

func aggregateBatchParentStatus(
	total int,
	successCount int,
	failedCount int,
	pendingCount int,
	pendingOnly int,
	executingCount int,
	cancelledCount int,
) generated.VMBatchParentStatus {
	if total == 0 {
		return generated.VMBatchParentStatusFAILED
	}
	if cancelledCount == total {
		return generated.VMBatchParentStatusCANCELLED
	}
	if successCount == total {
		return generated.VMBatchParentStatusCOMPLETED
	}
	if failedCount+cancelledCount == total {
		return generated.VMBatchParentStatusFAILED
	}
	if pendingOnly == total {
		return generated.VMBatchParentStatusPENDINGAPPROVAL
	}
	if pendingCount > 0 || executingCount > 0 {
		return generated.VMBatchParentStatusINPROGRESS
	}
	if successCount > 0 && failedCount+cancelledCount > 0 {
		return generated.VMBatchParentStatusPARTIALSUCCESS
	}
	return generated.VMBatchParentStatusINPROGRESS
}

func mapProjectionStatus(status generated.VMBatchParentStatus) entbatchticket.Status {
	switch status {
	case generated.VMBatchParentStatusPENDINGAPPROVAL:
		return entbatchticket.StatusPENDING_APPROVAL
	case generated.VMBatchParentStatusINPROGRESS:
		return entbatchticket.StatusIN_PROGRESS
	case generated.VMBatchParentStatusCOMPLETED:
		return entbatchticket.StatusCOMPLETED
	case generated.VMBatchParentStatusPARTIALSUCCESS:
		return entbatchticket.StatusPARTIAL_SUCCESS
	case generated.VMBatchParentStatusCANCELLED:
		return entbatchticket.StatusCANCELLED
	default:
		return entbatchticket.StatusFAILED
	}
}

func toBatchProjectionType(op string) entbatchticket.BatchType {
	switch strings.TrimSpace(strings.ToUpper(op)) {
	case string(generated.VMBatchOperation("DELETE")):
		return entbatchticket.BatchTypeBATCH_DELETE
	case string(generated.VMBatchOperation("MODIFY")):
		return entbatchticket.BatchTypeBATCH_MODIFY
	case string(generated.VMBatchOperation("POWER")), "POWER_START", "POWER_STOP", "POWER_RESTART":
		return entbatchticket.BatchTypeBATCH_POWER
	default:
		return entbatchticket.BatchTypeBATCH_CREATE
	}
}

func nillableTrimmed(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func buildBatchPayloadItems(op string, items []generated.VMBatchChildItem, children ...preparedBatchChild) []domain.BatchVMItemPayload {
	out := make([]domain.BatchVMItemPayload, 0, len(items))
	for idx, item := range items {
		payloadItem := domain.BatchVMItemPayload{
			VMID:           strings.TrimSpace(item.VmId),
			ServiceID:      strings.TrimSpace(item.ServiceId.String()),
			TemplateID:     strings.TrimSpace(item.TemplateId.String()),
			InstanceSizeID: strings.TrimSpace(item.InstanceSizeId.String()),
			Namespace:      strings.TrimSpace(item.Namespace),
			Reason:         strings.TrimSpace(item.Reason),
			TargetCPUCores: normalizeOptionalTargetFloat64(float64(item.TargetCpuCores)),
			TargetMemoryGi: normalizeOptionalTargetFloat64(float64(item.TargetMemoryGi)),
			TargetDiskGB:   normalizeOptionalTargetInt(item.TargetDiskGb),
		}
		if idx < len(children) {
			switch strings.TrimSpace(strings.ToUpper(op)) {
			case string(generated.VMBatchOperation("CREATE")):
				var childPayload domain.VMCreationPayload
				if err := json.Unmarshal(children[idx].payload, &childPayload); err == nil {
					payloadItem.SystemID = strings.TrimSpace(childPayload.SystemID)
					payloadItem.SystemName = strings.TrimSpace(childPayload.SystemName)
					payloadItem.ServiceID = strings.TrimSpace(childPayload.ServiceID)
					payloadItem.ServiceName = strings.TrimSpace(childPayload.ServiceName)
					payloadItem.TemplateID = strings.TrimSpace(childPayload.TemplateID)
					payloadItem.TemplateName = strings.TrimSpace(childPayload.TemplateName)
					payloadItem.InstanceSizeID = strings.TrimSpace(childPayload.InstanceSizeID)
					payloadItem.InstanceSizeName = strings.TrimSpace(childPayload.InstanceSizeName)
					payloadItem.Namespace = strings.TrimSpace(childPayload.Namespace)
					payloadItem.OwnerID = firstNonEmptyString(strings.TrimSpace(childPayload.OwnerID), strings.TrimSpace(childPayload.RequesterID))
					payloadItem.OwnerDisplayName = strings.TrimSpace(childPayload.OwnerDisplayName)
					payloadItem.OwnerUsername = strings.TrimSpace(childPayload.OwnerUsername)
					if payloadItem.TargetCPUCores == nil && childPayload.TargetCPUCores > 0 {
						target := childPayload.TargetCPUCores
						payloadItem.TargetCPUCores = &target
					}
					if payloadItem.TargetMemoryGi == nil && childPayload.TargetMemoryGi > 0 {
						target := childPayload.TargetMemoryGi
						payloadItem.TargetMemoryGi = &target
					}
					if payloadItem.TargetDiskGB == nil && childPayload.TargetDiskGB > 0 {
						target := childPayload.TargetDiskGB
						payloadItem.TargetDiskGB = &target
					}
				}
			case string(generated.VMBatchOperation("DELETE")):
				var childPayload domain.VMDeletePayload
				if err := json.Unmarshal(children[idx].payload, &childPayload); err == nil {
					payloadItem.VMID = strings.TrimSpace(childPayload.VMID)
					payloadItem.VMName = strings.TrimSpace(childPayload.VMName)
					payloadItem.SystemID = strings.TrimSpace(childPayload.SystemID)
					payloadItem.SystemName = strings.TrimSpace(childPayload.SystemName)
					payloadItem.ServiceID = strings.TrimSpace(childPayload.ServiceID)
					payloadItem.ServiceName = strings.TrimSpace(childPayload.ServiceName)
					payloadItem.TemplateID = strings.TrimSpace(childPayload.TemplateID)
					payloadItem.TemplateName = strings.TrimSpace(childPayload.TemplateName)
					payloadItem.InstanceSizeID = strings.TrimSpace(childPayload.InstanceSizeID)
					payloadItem.InstanceSizeName = strings.TrimSpace(childPayload.InstanceSizeName)
					payloadItem.Namespace = strings.TrimSpace(childPayload.Namespace)
					payloadItem.ClusterID = strings.TrimSpace(childPayload.ClusterID)
					payloadItem.ClusterName = strings.TrimSpace(childPayload.ClusterName)
					payloadItem.ClusterEnvironment = strings.TrimSpace(childPayload.ClusterEnvironment)
					payloadItem.OwnerID = strings.TrimSpace(childPayload.OwnerID)
					payloadItem.OwnerDisplayName = strings.TrimSpace(childPayload.OwnerDisplayName)
					payloadItem.OwnerUsername = strings.TrimSpace(childPayload.OwnerUsername)
					payloadItem.RequestVMStatus = strings.TrimSpace(childPayload.RequestVMStatus)
					payloadItem.CurrentCPUCores = childPayload.CurrentCPUCores
					payloadItem.CurrentMemoryGi = childPayload.CurrentMemoryGi
					payloadItem.CurrentDiskGB = childPayload.CurrentDiskGB
				}
			case string(generated.VMBatchOperation("MODIFY")):
				var childPayload domain.VMModifyPayload
				if err := json.Unmarshal(children[idx].payload, &childPayload); err == nil {
					payloadItem.VMID = strings.TrimSpace(childPayload.VMID)
					payloadItem.VMName = strings.TrimSpace(childPayload.VMName)
					payloadItem.SystemID = strings.TrimSpace(childPayload.SystemID)
					payloadItem.SystemName = strings.TrimSpace(childPayload.SystemName)
					payloadItem.ServiceID = strings.TrimSpace(childPayload.ServiceID)
					payloadItem.ServiceName = strings.TrimSpace(childPayload.ServiceName)
					payloadItem.TemplateID = strings.TrimSpace(childPayload.TemplateID)
					payloadItem.TemplateName = strings.TrimSpace(childPayload.TemplateName)
					payloadItem.InstanceSizeID = strings.TrimSpace(childPayload.InstanceSizeID)
					payloadItem.InstanceSizeName = strings.TrimSpace(childPayload.InstanceSizeName)
					payloadItem.Namespace = strings.TrimSpace(childPayload.Namespace)
					payloadItem.ClusterID = strings.TrimSpace(childPayload.ClusterID)
					payloadItem.ClusterName = strings.TrimSpace(childPayload.ClusterName)
					payloadItem.ClusterEnvironment = strings.TrimSpace(childPayload.ClusterEnvironment)
					payloadItem.OwnerID = strings.TrimSpace(childPayload.OwnerID)
					payloadItem.OwnerDisplayName = strings.TrimSpace(childPayload.OwnerDisplayName)
					payloadItem.OwnerUsername = strings.TrimSpace(childPayload.OwnerUsername)
					payloadItem.RequestVMStatus = strings.TrimSpace(childPayload.RequestVMStatus)
					payloadItem.CurrentCPUCores = childPayload.CurrentCPUCores
					payloadItem.CurrentMemoryGi = childPayload.CurrentMemoryGi
					payloadItem.CurrentDiskGB = childPayload.CurrentDiskGB
					payloadItem.TargetCPUCores = childPayload.TargetCPUCores
					payloadItem.TargetMemoryGi = childPayload.TargetMemoryGi
					payloadItem.TargetDiskGB = childPayload.TargetDiskGB
				}
			}
		}
		if op == string(generated.VMBatchOperation("CREATE")) {
			payloadItem.VMID = ""
		}
		out = append(out, payloadItem)
	}
	return out
}

func buildBatchPowerPayloadItems(items []generated.VMBatchPowerItem, children ...preparedBatchChild) []domain.BatchVMItemPayload {
	out := make([]domain.BatchVMItemPayload, 0, len(items))
	for idx, item := range items {
		payloadItem := domain.BatchVMItemPayload{
			VMID:   strings.TrimSpace(item.VmId),
			Reason: strings.TrimSpace(item.Reason),
		}
		if idx < len(children) {
			var childPayload domain.VMPowerPayload
			if err := json.Unmarshal(children[idx].payload, &childPayload); err == nil {
				payloadItem.VMID = strings.TrimSpace(childPayload.VMID)
				payloadItem.VMName = strings.TrimSpace(childPayload.VMName)
				payloadItem.SystemID = strings.TrimSpace(childPayload.SystemID)
				payloadItem.SystemName = strings.TrimSpace(childPayload.SystemName)
				payloadItem.ServiceID = strings.TrimSpace(childPayload.ServiceID)
				payloadItem.ServiceName = strings.TrimSpace(childPayload.ServiceName)
				payloadItem.TemplateID = strings.TrimSpace(childPayload.TemplateID)
				payloadItem.TemplateName = strings.TrimSpace(childPayload.TemplateName)
				payloadItem.InstanceSizeID = strings.TrimSpace(childPayload.InstanceSizeID)
				payloadItem.InstanceSizeName = strings.TrimSpace(childPayload.InstanceSizeName)
				payloadItem.Namespace = strings.TrimSpace(childPayload.Namespace)
				payloadItem.ClusterID = strings.TrimSpace(childPayload.ClusterID)
				payloadItem.ClusterName = strings.TrimSpace(childPayload.ClusterName)
				payloadItem.ClusterEnvironment = strings.TrimSpace(childPayload.ClusterEnvironment)
				payloadItem.OwnerID = strings.TrimSpace(childPayload.OwnerID)
				payloadItem.OwnerDisplayName = strings.TrimSpace(childPayload.OwnerDisplayName)
				payloadItem.OwnerUsername = strings.TrimSpace(childPayload.OwnerUsername)
				payloadItem.RequestVMStatus = strings.TrimSpace(childPayload.RequestVMStatus)
				payloadItem.CurrentCPUCores = childPayload.CurrentCPUCores
				payloadItem.CurrentMemoryGi = childPayload.CurrentMemoryGi
				payloadItem.CurrentDiskGB = childPayload.CurrentDiskGB
				payloadItem.Operation = strings.TrimSpace(childPayload.Operation)
			}
		}
		out = append(out, payloadItem)
	}
	return out
}

func isZeroUUID(id openapi_types.UUID) bool {
	s := strings.TrimSpace(id.String())
	return s == "" || s == "00000000-0000-0000-0000-000000000000"
}

func generateIDV7() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return id.String()
}

// ListVMBatches handles GET /vms/batch.
// Queries the BatchTicket projection table for efficient pagination.
// Follows oapi-codegen ServerInterface contract (ADR-0021).
func (s *Server) ListVMBatches(c *gin.Context, params generated.ListVMBatchesParams) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:read", "vm:create", "vm:delete", "vm:operate", "builtin_approval:view") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	// Build ordering: default newest-first per ADR-0015 list semantics.
	desc := params.SortOrder != generated.ListVMBatchesParamsSortOrderAsc

	// Count total (without pagination).
	query := s.client.BatchTicket.Query()
	// Non-admin users see only their own batches.
	if !hasPlatformAdmin(c) {
		query = query.Where(entbatchticket.CreatedByEQ(actor))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while counting vm batches", zap.Error(err))
			return
		}
		logger.Error("failed to count vm batches", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	// Apply ordering.
	orderFn := ent.Desc(entbatchticket.FieldCreatedAt)
	if !desc {
		orderFn = ent.Asc(entbatchticket.FieldCreatedAt)
	}

	rows, err := query.Order(orderFn).Offset(offset).Limit(perPage).All(ctx)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while listing vm batches", zap.Error(err), zap.Int("page", page))
			return
		}
		logger.Error("failed to list vm batches", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.VMBatchJobSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, generated.VMBatchJobSummary{
			Id:           row.ID,
			Operation:    string(row.BatchType),
			Status:       generated.VMBatchJobSummaryStatus(row.Status),
			ChildCount:   row.ChildCount,
			SuccessCount: row.SuccessCount,
			FailedCount:  row.FailedCount,
			PendingCount: row.PendingCount,
			CreatedAt:    row.CreatedAt,
		})
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.VMBatchList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
