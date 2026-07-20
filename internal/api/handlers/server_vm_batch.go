package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
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
	batchNothingToRetryErrorCode    = "BATCH_NOTHING_TO_RETRY"
	batchRateLimitedMessage         = "batch submission rate limited"
	batchResourceType               = "batch"
	batchPowerOperationStart        = "POWER_START"
	batchPowerOperationStop         = "POWER_STOP"
	batchPowerOperationRestart      = "POWER_RESTART"
)

var (
	errBatchNotFound                    = errors.New("batch not found")
	errBatchSubmissionActorNotFound     = errors.New("batch submission actor no longer exists")
	errBatchSubmissionActorNotAvailable = errors.New("batch submission actor is disabled")
)

type batchChildStateConflictError struct {
	TicketID string
	EventID  string
}

func (e *batchChildStateConflictError) Error() string {
	return fmt.Sprintf("batch child ticket %s event %s is no longer eligible for the requested transition", e.TicketID, e.EventID)
}

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

func (s *Server) writeExistingBatchSubmitResponse(c *gin.Context, batchID, batchKind string) {
	ctx := c.Request.Context()
	view, _, err := s.loadBatchView(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, errBatchNotFound) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading existing batch view",
				zap.Error(err),
				zap.String("batch_id", batchID),
				zap.String("batch_kind", batchKind),
			)
			return
		}
		logger.Error("failed to load existing batch view",
			zap.Error(err),
			zap.String("batch_id", batchID),
			zap.String("batch_kind", batchKind),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusAccepted, generated.VMBatchSubmitResponse{
		BatchId:           batchID,
		Status:            view.Status,
		StatusUrl:         "/api/v1/vms/batch/" + batchID,
		RetryAfterSeconds: batchRetryAfterSeconds,
	})
}

// writeEarlyBatchReplayResponse resolves a durable idempotency replay before
// namespace, VM, catalog, approval-policy, or live-state preparation. Those
// mutable dependencies may legitimately change after the original commit and
// must not prevent a client from recovering a lost accepted response. The
// transaction-scoped replay check remains the authority for concurrent first
// submissions.
func (s *Server) writeEarlyBatchReplayResponse(
	c *gin.Context,
	actor string,
	operation string,
	requestID string,
	batchKind string,
) bool {
	if normalizeBatchRequestID(requestID) == "" {
		return false
	}
	existingID, found, err := findBatchByRequestIDWithClient(
		c.Request.Context(),
		s.client,
		actor,
		operation,
		requestID,
	)
	if err != nil {
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while resolving early batch replay",
				zap.Error(err),
				zap.String("batch_kind", batchKind),
			)
			return true
		}
		logger.Error("failed to resolve early batch replay",
			zap.Error(err),
			zap.String("batch_kind", batchKind),
		)
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return true
	}
	if !found {
		return false
	}
	s.writeExistingBatchSubmitResponse(c, existingID, batchKind)
	return true
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
	requestID := normalizeBatchRequestID(req.RequestId)
	if s.writeEarlyBatchReplayResponse(c, actor, op, requestID, batchResourceType) {
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
		RequestID:   requestID,
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

	releaseBatchGuard, err := s.acquireBatchSubmissionGuard(ctx)
	if err != nil {
		logger.Error("failed to lock batch submission guard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer releaseBatchGuard()

	// READ COMMITTED takes a fresh snapshot after a waiter acquires the global
	// advisory lock. An operator-level REPEATABLE READ default could otherwise
	// retain a pre-wait snapshot and miss the preceding submission's commit.
	tx, err := s.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
	if lockErr := lockBatchSubmissionTransaction(ctx, tx); lockErr != nil {
		_ = tx.Rollback()
		logger.Error("failed to lock batch submissions in business transaction", zap.Error(lockErr))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	// Serialize policy reads with exemption/override and actor mutations. The
	// global -> actor order is one-way: policy writers never take the global lock.
	if lockErr := lockUserMutation(ctx, tx, actor); lockErr != nil {
		_ = tx.Rollback()
		logger.Error("failed to lock batch submission actor", zap.Error(lockErr), zap.String("actor", actor))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if requestID != "" {
		if lockErr := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			usecase.BatchIdempotencyLockKey(actor, op, requestID),
		); lockErr != nil {
			_ = tx.Rollback()
			logger.Error("failed to lock batch idempotency key", zap.Error(lockErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		existingID, found, findErr := findBatchByRequestIDWithClient(
			ctx,
			tx.Client(),
			actor,
			op,
			requestID,
		)
		switch {
		case findErr != nil:
			_ = tx.Rollback()
			logger.Error("failed to recheck batch idempotency in transaction", zap.Error(findErr))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		case found:
			_ = tx.Rollback()
			s.writeExistingBatchSubmitResponse(c, existingID, batchResourceType)
			return
		}
	}

	rateLimit, limitErr := evaluateBatchSubmissionRateLimits(
		ctx,
		entBatchSubmissionLimitReader{client: tx.Client()},
		actor,
		len(req.Items),
		time.Now().UTC(),
	)
	if limitErr != nil {
		_ = tx.Rollback()
		if writeBatchSubmissionActorStateError(c, limitErr) {
			return
		}
		logger.Error("failed to evaluate batch submission limits", zap.Error(limitErr))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if rateLimit != nil {
		_ = tx.Rollback()
		writeBatchSubmissionRateLimitResponse(c, rateLimit, false)
		return
	}

	parentEventID := generateIDV7()
	_, err = tx.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(parentEventType)).
		SetAggregateType(batchResourceType).
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
		SetNillableRequestID(nillableTrimmed(requestID)).
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
	releaseBatchGuard()

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
	requestID := normalizeBatchRequestID(req.RequestId)
	if s.writeEarlyBatchReplayResponse(c, actor, opKey, requestID, "power-batch") {
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
		RequestID:   requestID,
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

	parentReason := strings.TrimSpace(req.Reason)
	if parentReason == "" {
		parentReason = fmt.Sprintf("batch power %s request (%d items)", strings.ToLower(jobOperation), len(children))
	}
	releaseBatchGuard, err := s.acquireBatchSubmissionGuard(ctx)
	if err != nil {
		logger.Error("failed to lock power-batch submission guard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	defer releaseBatchGuard()

	atomicWriter := usecase.NewApprovalAtomicWriter(s.pool, s.riverClient)
	writeErr := atomicWriter.CreateBatchPowerAndMaybeEnqueue(ctx, usecase.BatchPowerSubmissionInput{
		ParentID:         parentID,
		Actor:            actor,
		Operation:        opKey,
		RequestID:        requestID,
		Reason:           parentReason,
		ParentPayload:    parentPayloadBytes,
		RequiresApproval: batchRequiresApproval,
		Children:         batchPowerChildInputs(children),
	}, &usecase.BatchPowerSubmissionTxPolicy{
		LockActor: func(validationCtx context.Context, tx pgx.Tx) error {
			return lockBatchSubmissionActor(validationCtx, tx, actor)
		},
		Validate: func(validationCtx context.Context, tx pgx.Tx) error {
			rateLimit, validationErr := evaluateBatchSubmissionRateLimits(
				validationCtx,
				pgxBatchSubmissionLimitReader{tx: tx},
				actor,
				len(req.Items),
				time.Now().UTC(),
			)
			if validationErr != nil {
				return fmt.Errorf("evaluate power-batch submission limits: %w", validationErr)
			}
			if rateLimit != nil {
				return rateLimit
			}
			return nil
		},
	})
	releaseBatchGuard()
	if writeErr != nil {
		var replay *usecase.BatchSubmissionReplayError
		if errors.As(writeErr, &replay) {
			s.writeExistingBatchSubmitResponse(c, replay.BatchID, "power-batch")
			return
		}
		var active *usecase.ActivePowerEventError
		if errors.As(writeErr, &active) {
			writeActivePowerOperationConflict(c, active)
			return
		}
		var rateLimit *batchSubmissionRateLimitError
		if errors.As(writeErr, &rateLimit) {
			writeBatchSubmissionRateLimitResponse(c, rateLimit, true)
			return
		}
		if writeBatchSubmissionActorStateError(c, writeErr) {
			return
		}
		logger.Error("failed to create power-batch rows and jobs atomically", zap.Error(writeErr), zap.String("batch_id", parentID))
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
	} else if isRequestContextCanceled(err) {
		logger.Debug("request canceled while loading submitted power-batch view", zap.Error(err), zap.String("batch_id", parentID))
		return
	} else {
		logger.Warn("failed to load submitted power-batch view; returning default accepted status",
			zap.String("batch_id", parentID),
			zap.Error(err),
		)
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
	owner, err := s.loadBatchOwner(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading batch owner", zap.Error(err), zap.String("batch_id", batchID))
			return
		}
		logger.Error("failed to load batch owner", zap.Error(err), zap.String("batch_id", batchID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !hasPlatformAdmin(c) && owner != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
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

	c.JSON(http.StatusOK, resp)
}

// RetryVMBatch handles POST /vms/batch/{batch_id}/retry.
func (s *Server) RetryVMBatch(c *gin.Context, batchID generated.BatchID) {
	req, bindErr := bindOptionalJSON[generated.ApprovalDecisionRequest](c)
	if bindErr != nil {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "BAD_REQUEST",
			Message: bindErr.Error(),
		})
		return
	}
	s.mutateBatchChildren(c, batchID, batchActionRetry, req)
}

// CancelVMBatch handles POST /vms/batch/{batch_id}/cancel.
func (s *Server) CancelVMBatch(c *gin.Context, batchID generated.BatchID) {
	s.mutateBatchChildren(c, batchID, batchActionCancel, nil)
}

func (s *Server) mutateBatchChildren(
	c *gin.Context,
	batchID, action string,
	retryReview *generated.ApprovalDecisionRequest,
) {
	ctx := c.Request.Context()
	if !requireAnyGlobalPermission(c, "vm:create", "vm:delete", "vm:operate", "builtin_approval:approve") {
		return
	}
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}
	owner, err := s.loadBatchOwner(ctx, batchID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
			return
		}
		if isRequestContextCanceled(err) {
			logger.Debug("request canceled while loading batch owner for action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
			return
		}
		logger.Error("failed to load batch owner for action", zap.Error(err), zap.String("batch_id", batchID), zap.String("action", action))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	if !hasPlatformAdmin(c) && owner != actor {
		c.JSON(http.StatusNotFound, generated.Error{Code: "BATCH_NOT_FOUND"})
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
	isCreateBatch := parentTicket.OperationType == entticket.OperationTypeCREATE
	if !requireBatchMutationPermission(c, parentTicket.OperationType) {
		return
	}
	powerOperation := parentTicket.OperationType == entticket.OperationTypePOWER
	identityMatches := strings.TrimSpace(parentEvent.AggregateType) == batchResourceType &&
		strings.TrimSpace(parentEvent.AggregateID) == strings.TrimSpace(batchID) &&
		batchParentEventMatchesOperation(parentTicket.OperationType, domain.EventType(parentEvent.EventType)) &&
		isPowerBatch == powerOperation
	switch action {
	case batchActionRetry:
		parentStateEligible := parentTicket.Status == entticket.StatusEXECUTING ||
			parentTicket.Status == entticket.StatusFAILED
		eventStateEligible := parentEvent.Status == domainevent.StatusPROCESSING ||
			parentEvent.Status == domainevent.StatusFAILED
		if !parentStateEligible || !eventStateEligible || !identityMatches {
			writeBatchParentStateConflict(c, batchID, action, parentTicket, parentEvent)
			return
		}
		if !isPowerBatch && strings.TrimSpace(parentTicket.Approver) == "" {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "BATCH_ACTION_NOT_APPLICABLE",
				Message: "this legacy batch has no durable approval provenance; create a new approval batch instead of retrying it",
				Params: map[string]interface{}{
					"batch_id":         batchID,
					"batch_status":     resp.Status,
					"operation_type":   string(parentTicket.OperationType),
					"requested_action": action,
					"reason":           "missing_approval_provenance",
				},
			})
			return
		}
	case batchActionCancel:
		parentStateEligible := parentTicket.Status == entticket.StatusPENDING ||
			parentTicket.Status == entticket.StatusEXECUTING ||
			parentTicket.Status == entticket.StatusFAILED
		eventStateEligible := parentEvent.Status == domainevent.StatusPENDING ||
			parentEvent.Status == domainevent.StatusPROCESSING ||
			parentEvent.Status == domainevent.StatusFAILED
		if !parentStateEligible || !eventStateEligible || !identityMatches {
			writeBatchParentStateConflict(c, batchID, action, parentTicket, parentEvent)
			return
		}
	}
	if action == batchActionRetry && retryReview != nil && !isCreateBatch {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "BATCH_RETRY_REVIEW_NOT_APPLICABLE",
			Message: "review inputs are supported only when retrying a failed CREATE batch; omit the request body for execution-only retries",
			Params: map[string]interface{}{
				"batch_id":         batchID,
				"operation_type":   string(parentTicket.OperationType),
				"requested_action": action,
			},
		})
		return
	}

	useRetryReviewExecution := action == batchActionRetry && retryReview != nil
	retryExecution := approvalcontract.ApprovalExecutionOptions{}
	if useRetryReviewExecution {
		if !hasGlobalPermission(c, "builtin_approval:approve") {
			c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
			return
		}
		retryExecution = approvalDecisionRequestToExecutionOptions(*retryReview)
		if isCreateBatch && strings.TrimSpace(retryExecution.ClusterID) == "" {
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    "VALIDATION_FAILED",
				Message: "selected cluster is required for create approval",
				FieldErrors: []generated.FieldError{{
					Field:   "selected_cluster_id",
					Code:    "REQUIRED",
					Message: "selected cluster is required for create approval",
				}},
			})
			return
		}
	}

	targetIDs := make([]string, 0)
	targetRefs := make([]batchChildTicketEventRef, 0)
	targetChildren := make([]*ent.Ticket, 0)
	exhaustedTicketIDs := make([]string, 0)
	for _, child := range children {
		switch action {
		case batchActionRetry:
			if child.Status == entticket.StatusFAILED {
				if child.AttemptCount >= domain.BatchChildMaxAttempts {
					exhaustedTicketIDs = append(exhaustedTicketIDs, child.ID)
					continue
				}
				targetIDs = append(targetIDs, child.ID)
				targetRefs = append(targetRefs, batchChildTicketEventRef{TicketID: child.ID, EventID: child.EventID})
				targetChildren = append(targetChildren, child)
			}
		case batchActionCancel:
			if child.Status == entticket.StatusPENDING {
				targetIDs = append(targetIDs, child.ID)
				targetRefs = append(targetRefs, batchChildTicketEventRef{TicketID: child.ID, EventID: child.EventID})
				targetChildren = append(targetChildren, child)
			}
		}
	}
	if len(targetIDs) == 0 {
		var errCode string
		var errMessage string
		switch action {
		case batchActionRetry:
			if len(exhaustedTicketIDs) > 0 {
				errCode = "BATCH_RETRY_ATTEMPTS_EXHAUSTED"
				errMessage = "all failed items have exhausted their retry attempts"
			} else {
				errCode = batchNothingToRetryErrorCode
				errMessage = "no failed items are currently available for retry"
			}
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
				"batch_id":             batchID,
				"batch_status":         resp.Status,
				"child_count":          resp.ChildCount,
				"success_count":        resp.SuccessCount,
				"failed_count":         resp.FailedCount,
				"pending_count":        resp.PendingCount,
				"requested_action":     action,
				"exhausted_ticket_ids": exhaustedTicketIDs,
				"max_attempts":         domain.BatchChildMaxAttempts,
			},
		})
		return
	}

	affectedCount := 0
	affectedTicketIDs := make([]string, 0)
	var updatedStatus generated.VMBatchParentStatus
	if action == batchActionRetry {
		if isCreateBatch &&
			resp.Status == generated.VMBatchParentStatusFAILED &&
			!useRetryReviewExecution &&
			strings.TrimSpace(parentTicket.SelectedClusterID) == "" {
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "BATCH_RETRY_REVIEW_REQUIRED",
				Message: "selected cluster is required before retrying a failed create batch; review the approval inputs first",
				Params: map[string]interface{}{
					"batch_id":         batchID,
					"batch_status":     resp.Status,
					"operation_type":   string(parentTicket.OperationType),
					"requested_action": action,
				},
				FieldErrors: []generated.FieldError{{
					Field:   "selected_cluster_id",
					Code:    "REQUIRED",
					Message: "selected cluster is required before retrying a failed create batch",
				}},
			})
			return
		}
		if isPowerBatch {
			retryChildren := make([]usecase.BatchPowerRetryChildInput, 0, len(targetChildren))
			for _, child := range targetChildren {
				retryChildren = append(retryChildren, usecase.BatchPowerRetryChildInput{
					TicketID: child.ID,
					EventID:  child.EventID,
				})
			}
			if len(retryChildren) > 0 {
				atomicWriter := usecase.NewApprovalAtomicWriter(s.pool, s.riverClient)
				if retryErr := atomicWriter.RetryBatchPowerAndEnqueue(ctx, usecase.BatchPowerRetryInput{
					ParentID: batchID,
					Children: retryChildren,
				}); retryErr != nil {
					var active *usecase.ActivePowerEventError
					if errors.As(retryErr, &active) {
						writeActivePowerOperationConflict(c, active)
						return
					}
					var jobConflict *usecase.PowerRetryJobConflictError
					if errors.As(retryErr, &jobConflict) {
						c.JSON(http.StatusConflict, generated.Error{
							Code:    "BATCH_RETRY_IN_PROGRESS",
							Message: "an equivalent power job is still active; retry after it finishes",
							Params: map[string]interface{}{
								"batch_id":           batchID,
								"event_id":           jobConflict.EventID,
								"existing_job_id":    jobConflict.ExistingJobID,
								"existing_job_state": jobConflict.ExistingJobState,
							},
						})
						return
					}
					var notEligible *usecase.PowerRetryNotEligibleError
					if errors.As(retryErr, &notEligible) {
						c.JSON(http.StatusConflict, generated.Error{
							Code:    batchNothingToRetryErrorCode,
							Message: "no failed items are currently available for retry",
							Params: map[string]interface{}{
								"batch_id":  batchID,
								"ticket_id": notEligible.TicketID,
								"event_id":  notEligible.EventID,
							},
						})
						return
					}
					var parentNotEligible *usecase.BatchRetryParentNotEligibleError
					if errors.As(retryErr, &parentNotEligible) {
						writeBatchParentStateConflict(c, batchID, batchActionRetry, nil, nil)
						return
					}
					var exhausted *usecase.BatchChildAttemptsExhaustedError
					if errors.As(retryErr, &exhausted) {
						writeBatchRetryAttemptsExhausted(c, batchID, exhausted)
						return
					}
					if isRequestContextCanceled(retryErr) {
						logger.Debug("request canceled while resetting power children for retry", zap.Error(retryErr), zap.String("batch_id", batchID))
						return
					}
					logger.Error("failed to reset and enqueue power children during batch retry",
						zap.String("batch_id", batchID),
						zap.Error(retryErr),
					)
					c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
					return
				} else {
					for _, child := range retryChildren {
						affectedCount++
						affectedTicketIDs = append(affectedTicketIDs, child.TicketID)
					}
				}
			}
			updatedStatus = generated.VMBatchParentStatusINPROGRESS
		} else {
			execution, executionErr := ticketing.BatchApprovalExecutionFromTicket(parentTicket)
			if executionErr != nil {
				logger.Error("failed to load durable batch retry execution plan", zap.Error(executionErr), zap.String("batch_id", batchID))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			if isCreateBatch && useRetryReviewExecution {
				execution = domain.BatchApprovalExecutionOptions{
					ClusterID:       retryExecution.ClusterID,
					StorageClass:    retryExecution.StorageClass,
					DVAccessModes:   append([]string(nil), retryExecution.DVAccessModes...),
					DVVolumeMode:    retryExecution.DVVolumeMode,
					EnableOverride:  retryExecution.EnableOverride,
					CPURequest:      retryExecution.CPURequest,
					CPULimit:        retryExecution.CPULimit,
					MemoryRequestGi: retryExecution.MemoryRequestGi,
					MemoryLimitGi:   retryExecution.MemoryLimitGi,
					DiskGB:          retryExecution.DiskGB,
				}
			}
			retryApprover := strings.TrimSpace(parentTicket.Approver)
			if useRetryReviewExecution {
				// A reviewer who deliberately supplies a replacement execution
				// plan owns that new approval decision. An ordinary execution
				// retry preserves the original approver instead of misattributing
				// approval to the requesting VM operator.
				retryApprover = actor
			}
			retryChildren := make([]domain.BatchApprovalRetryChild, 0, len(targetChildren))
			for _, child := range targetChildren {
				retryChildren = append(retryChildren, domain.BatchApprovalRetryChild{
					TicketID: child.ID,
					EventID:  child.EventID,
				})
			}
			atomicWriter := usecase.NewApprovalAtomicWriter(s.pool, s.riverClient)
			retryErr := atomicWriter.RetryBatchApprovalAndEnqueue(ctx, domain.BatchApprovalRetryInput{
				ParentTicketID: parentTicket.ID,
				ParentEventID:  parentTicket.EventID,
				Approver:       retryApprover,
				Children:       retryChildren,
				Execution:      execution,
			})
			if retryErr != nil {
				var exhausted *usecase.BatchChildAttemptsExhaustedError
				if errors.As(retryErr, &exhausted) {
					writeBatchRetryAttemptsExhausted(c, batchID, exhausted)
					return
				}
				var inProgress *usecase.BatchApprovalDispatchConflictError
				if errors.As(retryErr, &inProgress) {
					c.JSON(http.StatusConflict, generated.Error{
						Code:    "BATCH_RETRY_IN_PROGRESS",
						Message: "batch retry dispatch is already in progress",
						Params: map[string]interface{}{
							"batch_id":           batchID,
							"existing_job_id":    inProgress.ExistingJobID,
							"existing_job_state": inProgress.ExistingJobState,
						},
					})
					return
				}
				var notEligible *usecase.BatchApprovalRetryNotEligibleError
				if errors.As(retryErr, &notEligible) {
					writeBatchChildStateConflict(c, batchID, batchActionRetry, &batchChildStateConflictError{
						TicketID: notEligible.TicketID,
						EventID:  notEligible.EventID,
					})
					return
				}
				var parentNotEligible *usecase.BatchRetryParentNotEligibleError
				if errors.As(retryErr, &parentNotEligible) {
					writeBatchParentStateConflict(c, batchID, batchActionRetry, nil, nil)
					return
				}
				if isRequestContextCanceled(retryErr) {
					logger.Debug("request canceled while scheduling durable batch retry", zap.Error(retryErr), zap.String("batch_id", batchID))
					return
				}
				logger.Error("failed to schedule durable batch retry", zap.Error(retryErr), zap.String("batch_id", batchID))
				c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
				return
			}
			for _, child := range targetChildren {
				affectedCount++
				affectedTicketIDs = append(affectedTicketIDs, child.ID)
			}
			updatedStatus = generated.VMBatchParentStatusINPROGRESS
		}
	} else {
		sort.Slice(targetRefs, func(i, j int) bool {
			return targetRefs[i].TicketID < targetRefs[j].TicketID
		})
		cancelledStatus, cancelTicketErr := s.updateBatchChildTicketAndEventStatus(
			ctx,
			batchID,
			targetRefs,
			entticket.StatusCANCELLED,
			domainevent.StatusCANCELLED,
			false,
			[]entticket.Status{entticket.StatusPENDING},
			[]domainevent.Status{domainevent.StatusPENDING},
		)
		if cancelTicketErr != nil {
			var stateConflict *batchChildStateConflictError
			if errors.As(cancelTicketErr, &stateConflict) {
				writeBatchChildStateConflict(c, batchID, batchActionCancel, stateConflict)
				return
			}
			if isRequestContextCanceled(cancelTicketErr) {
				logger.Debug("request canceled while mutating child tickets", zap.Error(cancelTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
				return
			}
			logger.Error("failed to mutate child tickets", zap.Error(cancelTicketErr), zap.String("batch_id", batchID), zap.String("action", action))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		updatedStatus = cancelledStatus
		affectedCount = len(targetIDs)
		affectedTicketIDs = append(affectedTicketIDs, targetIDs...)
	}
	if s.audit != nil {
		details := map[string]interface{}{
			"affected_count":      affectedCount,
			"affected_ticket_ids": append([]string(nil), affectedTicketIDs...),
			"batch_status":        updatedStatus,
			"operation_type":      parentTicket.OperationType,
		}
		if action == batchActionRetry {
			effectiveApprover := strings.TrimSpace(parentTicket.Approver)
			if useRetryReviewExecution {
				effectiveApprover = actor
			}
			details["review_replaced"] = useRetryReviewExecution
			details["original_approver"] = strings.TrimSpace(parentTicket.Approver)
			details["effective_approver"] = effectiveApprover
		}
		_ = s.audit.LogAction(ctx, "vm.batch."+action, "ticket", batchID, actor, details)
	}
	c.JSON(http.StatusOK, generated.VMBatchActionResponse{
		BatchId:           batchID,
		Status:            updatedStatus,
		AffectedCount:     affectedCount,
		AffectedTicketIds: affectedTicketIDs,
	})
}

// requireBatchMutationPermission derives authorization from the durable
// parent operation instead of accepting any VM mutation permission presented
// by the caller. This prevents a requester from using retry/cancel to cross an
// operation boundary (for example vm:create to retry a failed DELETE).
func requireBatchMutationPermission(c *gin.Context, operation entticket.OperationType) bool {
	// The OpenAPI contract deliberately allows approval reviewers as a separate
	// principal class. Preserve that path while preventing ordinary VM actors
	// from crossing operation-specific permission boundaries.
	if hasGlobalPermission(c, "builtin_approval:approve") {
		return true
	}
	switch operation {
	case entticket.OperationTypeCREATE:
		return requireGlobalPermission(c, "vm:create")
	case entticket.OperationTypeDELETE:
		return requireGlobalPermission(c, "vm:delete")
	case entticket.OperationTypeMODIFY, entticket.OperationTypePOWER:
		return requireGlobalPermission(c, "vm:operate")
	default:
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return false
	}
}

func batchParentEventMatchesOperation(operation entticket.OperationType, eventType domain.EventType) bool {
	switch operation {
	case entticket.OperationTypeCREATE:
		return eventType == domain.EventBatchCreateRequested
	case entticket.OperationTypeMODIFY:
		return eventType == domain.EventBatchModifyRequested
	case entticket.OperationTypeDELETE:
		return eventType == domain.EventBatchDeleteRequested
	case entticket.OperationTypePOWER:
		return eventType == domain.EventBatchPowerRequested
	default:
		return false
	}
}

func writeBatchParentStateConflict(
	c *gin.Context,
	batchID string,
	action string,
	parent *ent.Ticket,
	event *ent.DomainEvent,
) {
	params := map[string]interface{}{
		"batch_id":         batchID,
		"requested_action": action,
	}
	if parent != nil {
		params["parent_status"] = parent.Status
		params["operation_type"] = parent.OperationType
	}
	if event != nil {
		params["event_status"] = event.Status
		params["event_type"] = event.EventType
	}
	message := "the batch parent is not eligible for the requested action"
	switch action {
	case batchActionRetry:
		message = "the batch parent is not in an approved retryable execution state"
	case batchActionCancel:
		message = "the batch parent is not in a cancellable state"
	}
	c.JSON(http.StatusConflict, generated.Error{
		Code:    "BATCH_ACTION_NOT_APPLICABLE",
		Message: message,
		Params:  params,
	})
}

func writeBatchRetryAttemptsExhausted(
	c *gin.Context,
	batchID string,
	exhausted *usecase.BatchChildAttemptsExhaustedError,
) {
	params := map[string]interface{}{
		"batch_id":     batchID,
		"max_attempts": domain.BatchChildMaxAttempts,
	}
	if exhausted != nil {
		params["ticket_id"] = exhausted.TicketID
		params["attempt_count"] = exhausted.AttemptCount
		params["max_attempts"] = exhausted.MaxAttempts
	}
	c.JSON(http.StatusConflict, generated.Error{
		Code:    "BATCH_RETRY_ATTEMPTS_EXHAUSTED",
		Message: "the batch child has exhausted its retry attempts",
		Params:  params,
	})
}

func writeBatchChildStateConflict(
	c *gin.Context,
	batchID string,
	action string,
	conflict *batchChildStateConflictError,
) {
	code := "BATCH_ACTION_NOT_APPLICABLE"
	message := "the batch child is no longer available for the requested action"
	switch action {
	case batchActionRetry:
		code = batchNothingToRetryErrorCode
		message = "no failed items are currently available for retry"
	case batchActionCancel:
		code = "BATCH_NOTHING_TO_CANCEL"
		message = "no pending items are currently available for cancel"
	}
	params := map[string]interface{}{
		"batch_id":         batchID,
		"requested_action": action,
	}
	if conflict != nil {
		params["ticket_id"] = conflict.TicketID
		params["event_id"] = conflict.EventID
	}
	c.JSON(http.StatusConflict, generated.Error{
		Code:    code,
		Message: message,
		Params:  params,
	})
}

type batchChildTicketEventRef struct {
	TicketID string
	EventID  string
}

func (s *Server) updateBatchChildTicketAndEventStatus(
	ctx context.Context,
	parentTicketID string,
	children []batchChildTicketEventRef,
	ticketStatus entticket.Status,
	eventStatus domainevent.Status,
	clearRejectReason bool,
	allowedTicketStatuses []entticket.Status,
	allowedEventStatuses []domainevent.Status,
) (generated.VMBatchParentStatus, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("server dependencies are not initialized")
	}
	if len(children) == 0 {
		return "", nil
	}

	var updatedStatus generated.VMBatchParentStatus
	err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		parentTicketID = strings.TrimSpace(parentTicketID)
		if parentTicketID == "" {
			return fmt.Errorf("batch child status update is missing parent ticket id")
		}
		if err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			usecase.BatchMutationLockKey(parentTicketID),
		); err != nil {
			return fmt.Errorf("lock batch child mutation parent %s: %w", parentTicketID, err)
		}
		for _, child := range children {
			ticketID := strings.TrimSpace(child.TicketID)
			eventID := strings.TrimSpace(child.EventID)
			if ticketID == "" || eventID == "" {
				return fmt.Errorf("batch child status update is missing ticket/event id")
			}
			ticketUpdate := tx.Ticket.Update().
				Where(
					entticket.ID(ticketID),
					entticket.EventID(eventID),
					entticket.ParentTicketIDEQ(parentTicketID),
				).
				SetStatus(ticketStatus)
			if len(allowedTicketStatuses) > 0 {
				ticketUpdate = ticketUpdate.Where(entticket.StatusIn(allowedTicketStatuses...))
			}
			if clearRejectReason {
				ticketUpdate = ticketUpdate.ClearRejectReason()
			}
			affected, err := ticketUpdate.Save(ctx)
			if err != nil {
				return fmt.Errorf("update batch child ticket %s: %w", ticketID, err)
			}
			if affected != 1 {
				stored, loadErr := tx.Ticket.Get(ctx, ticketID)
				if ent.IsNotFound(loadErr) {
					return &batchChildStateConflictError{TicketID: ticketID, EventID: eventID}
				}
				if loadErr != nil {
					return fmt.Errorf("reload batch child ticket %s after transition conflict: %w", ticketID, loadErr)
				}
				if strings.TrimSpace(stored.EventID) != eventID ||
					strings.TrimSpace(stored.ParentTicketID) != parentTicketID ||
					!ticketStatusAllowed(stored.Status, allowedTicketStatuses) {
					return &batchChildStateConflictError{TicketID: ticketID, EventID: eventID}
				}
				return fmt.Errorf("update batch child ticket %s: expected 1 row, got %d", ticketID, affected)
			}

			eventUpdate := tx.DomainEvent.Update().
				Where(
					domainevent.ID(eventID),
					domainEventBoundToBatchChild(ticketID, parentTicketID),
				).
				SetStatus(eventStatus)
			if len(allowedEventStatuses) > 0 {
				eventUpdate = eventUpdate.Where(domainevent.StatusIn(allowedEventStatuses...))
			}
			affected, err = eventUpdate.Save(ctx)
			if err != nil {
				return fmt.Errorf("update batch child event %s: %w", eventID, err)
			}
			if affected != 1 {
				stored, loadErr := tx.DomainEvent.Get(ctx, eventID)
				if ent.IsNotFound(loadErr) {
					return &batchChildStateConflictError{TicketID: ticketID, EventID: eventID}
				}
				if loadErr != nil {
					return fmt.Errorf("reload batch child event %s after transition conflict: %w", eventID, loadErr)
				}
				if !domainEventStatusAllowed(stored.Status, allowedEventStatuses) {
					return &batchChildStateConflictError{TicketID: ticketID, EventID: eventID}
				}
				return fmt.Errorf("update batch child event %s: expected 1 row, got %d", eventID, affected)
			}
		}
		if err := jobs.SyncParentBatchStatusInTx(ctx, tx, parentTicketID); err != nil {
			return fmt.Errorf("sync parent batch %s after child status update: %w", parentTicketID, err)
		}
		projection, err := tx.BatchTicket.Get(ctx, parentTicketID)
		if err != nil {
			return fmt.Errorf("load authoritative batch projection %s after child status update: %w", parentTicketID, err)
		}
		updatedStatus, err = batchParentStatusFromProjection(projection.Status)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return updatedStatus, nil
}

func batchParentStatusFromProjection(status entbatchticket.Status) (generated.VMBatchParentStatus, error) {
	switch status {
	case entbatchticket.StatusPENDING_APPROVAL:
		return generated.VMBatchParentStatusPENDINGAPPROVAL, nil
	case entbatchticket.StatusIN_PROGRESS:
		return generated.VMBatchParentStatusINPROGRESS, nil
	case entbatchticket.StatusCOMPLETED:
		return generated.VMBatchParentStatusCOMPLETED, nil
	case entbatchticket.StatusFAILED:
		return generated.VMBatchParentStatusFAILED, nil
	case entbatchticket.StatusPARTIAL_SUCCESS:
		return generated.VMBatchParentStatusPARTIALSUCCESS, nil
	case entbatchticket.StatusCANCELLED:
		return generated.VMBatchParentStatusCANCELLED, nil
	default:
		return "", fmt.Errorf("unsupported batch projection status %q", status)
	}
}

func ticketStatusAllowed(status entticket.Status, allowed []entticket.Status) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func domainEventStatusAllowed(status domainevent.Status, allowed []domainevent.Status) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
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
			if labelErr := s.validateBatchCreateCatalogLabels(ctx, templateID, instanceSizeID); labelErr != nil {
				return nil, labelErr
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

func (s *Server) validateBatchCreateCatalogLabels(ctx context.Context, templateID, instanceSizeID string) *batchValidationError {
	tpl, err := s.client.Template.Get(ctx, templateID)
	if err != nil {
		if ent.IsNotFound(err) {
			return &batchValidationError{
				status: http.StatusBadRequest,
				body: generated.Error{
					Code:    "TEMPLATE_NOT_FOUND",
					Message: "template not found",
				},
			}
		}
		return &batchValidationError{
			status: http.StatusInternalServerError,
			body:   generated.Error{Code: "INTERNAL_ERROR"},
		}
	}
	size, err := s.client.InstanceSize.Get(ctx, instanceSizeID)
	if err != nil {
		if ent.IsNotFound(err) {
			return &batchValidationError{
				status: http.StatusBadRequest,
				body: generated.Error{
					Code:    "INSTANCE_SIZE_NOT_FOUND",
					Message: "instance size not found",
				},
			}
		}
		return &batchValidationError{
			status: http.StatusInternalServerError,
			body:   generated.Error{Code: "INTERNAL_ERROR"},
		}
	}
	if service.TemplateInstanceSizeCompatible(tpl.SystemLabels, size.SystemLabels) {
		return nil
	}
	return &batchValidationError{
		status: http.StatusBadRequest,
		body: generated.Error{
			Code:    "TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH",
			Message: "selected instance size is not compatible with selected template system labels",
			Params: map[string]interface{}{
				"template_system_labels":      service.NormalizeSystemLabelsForRead(tpl.SystemLabels),
				"instance_size_system_labels": service.NormalizeSystemLabelsForRead(size.SystemLabels),
			},
		},
	}
}

func normalizeBatchOperation(op generated.VMBatchSubmitOperation) (string, domain.EventType, error) {
	switch op {
	case generated.VMBatchSubmitOperation("CREATE"):
		return string(op), domain.EventBatchCreateRequested, nil
	case generated.VMBatchSubmitOperation("MODIFY"):
		return string(op), domain.EventBatchModifyRequested, nil
	case generated.VMBatchSubmitOperation("DELETE"):
		return string(op), domain.EventBatchDeleteRequested, nil
	default:
		return "", "", fmt.Errorf("unsupported operation %q", op)
	}
}

func normalizeBatchRequestID(value string) string {
	return batchreplay.Normalize(value)
}

func normalizeBatchPowerOperation(op generated.VMBatchPowerAction) (opKey, jobOperation string, childEventType domain.EventType, err error) {
	switch strings.TrimSpace(strings.ToUpper(string(op))) {
	case "START":
		return batchPowerOperationStart, "START", domain.EventVMStartRequested, nil
	case "STOP":
		return batchPowerOperationStop, "STOP", domain.EventVMStopRequested, nil
	case "RESTART":
		return batchPowerOperationRestart, "RESTART", domain.EventVMRestartRequested, nil
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
	seenVMs := make(map[string]int, len(req.Items))
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
		if firstIndex, exists := seenVMs[vmID]; exists {
			return nil, &batchValidationError{
				status: http.StatusBadRequest,
				body: generated.Error{
					Code:    "INVALID_BATCH_ITEM",
					Message: fmt.Sprintf("power item #%d repeats vm_id %q from item #%d", idx+1, vmID, firstIndex+1),
					Params: map[string]interface{}{
						"vm_id":           vmID,
						"first_index":     firstIndex + 1,
						"duplicate_index": idx + 1,
					},
				},
			}
		}
		seenVMs[vmID] = idx
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
			DispatchMode:       domain.VMPowerDispatchTicket,
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

func batchPowerChildInputs(children []preparedBatchChild) []usecase.BatchPowerChildInput {
	out := make([]usecase.BatchPowerChildInput, 0, len(children))
	for _, child := range children {
		out = append(out, usecase.BatchPowerChildInput{
			EventType:   string(child.eventType),
			AggregateID: child.aggregateID,
			Payload:     child.payload,
			Reason:      child.reason,
		})
	}
	return out
}

func batchParentEventTypes() []string {
	return []string{
		string(domain.EventBatchCreateRequested),
		string(domain.EventBatchModifyRequested),
		string(domain.EventBatchDeleteRequested),
		string(domain.EventBatchPowerRequested),
	}
}

type batchSubmissionLimitViolation struct {
	Reason              string
	RetryAfterSeconds   int
	GlobalRecentSubmits int
	UserPendingChildren int
	UserCooldownSeconds int
	ContactAdmin        bool
}

type batchUserLimitPolicy struct {
	Exempt              bool
	UsesDefaultParents  bool
	UsesDefaultChildren bool
	UsesDefaultCooldown bool
	MaxPendingParents   int
	MaxPendingChildren  int
	Cooldown            time.Duration
	ExemptionExpiresAt  *time.Time
}

type batchUserLimitConfig struct {
	ExemptionFound     bool
	ExemptionExpiresAt *time.Time
	MaxPendingParents  *int
	MaxPendingChildren *int
	CooldownSeconds    *int
}

type batchSubmissionLimitReader interface {
	activeUser(context.Context, string) (found, enabled bool, err error)
	pendingParentCounters(context.Context, string) (global, user int, err error)
	userLimitConfig(context.Context, string) (batchUserLimitConfig, error)
	recentSubmissionCount(context.Context, time.Time) (int, error)
	pendingChildCount(context.Context, string) (int, error)
	latestSubmissionAt(context.Context, string) (time.Time, bool, error)
}

type entBatchSubmissionLimitReader struct {
	client *ent.Client
}

func (r entBatchSubmissionLimitReader) activeUser(
	ctx context.Context,
	actor string,
) (found, enabled bool, err error) {
	if r.client == nil {
		return false, false, fmt.Errorf("batch submission Ent client is required")
	}
	userRow, err := r.client.User.Get(ctx, actor)
	if ent.IsNotFound(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, userRow.Enabled, nil
}

func (r entBatchSubmissionLimitReader) pendingParentCounters(
	ctx context.Context,
	actor string,
) (global, user int, err error) {
	if r.client == nil {
		return 0, 0, fmt.Errorf("batch submission Ent client is required")
	}
	events, err := r.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ(batchResourceType),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.StatusIn(domainevent.StatusPENDING, domainevent.StatusPROCESSING),
		).
		All(ctx)
	if err != nil {
		return 0, 0, err
	}
	global = len(events)
	for _, event := range events {
		if event.CreatedBy == actor {
			user++
		}
	}
	return global, user, nil
}

func (r entBatchSubmissionLimitReader) userLimitConfig(
	ctx context.Context,
	actor string,
) (batchUserLimitConfig, error) {
	var result batchUserLimitConfig
	if r.client == nil {
		return result, fmt.Errorf("batch submission Ent client is required")
	}
	exemption, err := r.client.RateLimitExemption.Query().
		Where(ratelimitexemption.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return result, err
	}
	if err == nil {
		result.ExemptionFound = true
		if exemption.ExpiresAt != nil {
			expiresAt := *exemption.ExpiresAt
			result.ExemptionExpiresAt = &expiresAt
		}
	}

	override, err := r.client.RateLimitUserOverride.Query().
		Where(ratelimituseroverride.IDEQ(actor)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return result, err
	}
	if err == nil {
		if override.MaxPendingParents != nil {
			value := *override.MaxPendingParents
			result.MaxPendingParents = &value
		}
		if override.MaxPendingChildren != nil {
			value := *override.MaxPendingChildren
			result.MaxPendingChildren = &value
		}
		if override.CooldownSeconds != nil {
			value := *override.CooldownSeconds
			result.CooldownSeconds = &value
		}
	}
	return result, nil
}

func (r entBatchSubmissionLimitReader) recentSubmissionCount(
	ctx context.Context,
	since time.Time,
) (int, error) {
	if r.client == nil {
		return 0, fmt.Errorf("batch submission Ent client is required")
	}
	return r.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ(batchResourceType),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedAtGTE(since),
		).
		Count(ctx)
}

func (r entBatchSubmissionLimitReader) pendingChildCount(ctx context.Context, actor string) (int, error) {
	if r.client == nil {
		return 0, fmt.Errorf("batch submission Ent client is required")
	}
	return r.client.Ticket.Query().
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
}

func (r entBatchSubmissionLimitReader) latestSubmissionAt(
	ctx context.Context,
	actor string,
) (time.Time, bool, error) {
	if r.client == nil {
		return time.Time{}, false, fmt.Errorf("batch submission Ent client is required")
	}
	lastEvent, err := r.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ(batchResourceType),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.CreatedByEQ(actor),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return lastEvent.CreatedAt, true, nil
}

type pgxBatchSubmissionLimitReader struct {
	tx pgx.Tx
}

func (r pgxBatchSubmissionLimitReader) activeUser(
	ctx context.Context,
	actor string,
) (found, enabled bool, err error) {
	if r.tx == nil {
		return false, false, fmt.Errorf("batch submission pgx transaction is required")
	}
	err = r.tx.QueryRow(ctx, `SELECT enabled FROM users WHERE id = $1`, actor).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, enabled, nil
}

func (r pgxBatchSubmissionLimitReader) pendingParentCounters(
	ctx context.Context,
	actor string,
) (global, user int, err error) {
	if r.tx == nil {
		return 0, 0, fmt.Errorf("batch submission pgx transaction is required")
	}
	err = r.tx.QueryRow(ctx, `
SELECT count(*), count(*) FILTER (WHERE created_by = $1)
FROM domain_events
WHERE aggregate_type = 'batch'
  AND event_type = ANY($2::text[])
  AND status = ANY($3::text[])
`, actor, batchParentEventTypes(), []string{
		string(domainevent.StatusPENDING),
		string(domainevent.StatusPROCESSING),
	}).Scan(&global, &user)
	return global, user, err
}

func (r pgxBatchSubmissionLimitReader) userLimitConfig(
	ctx context.Context,
	actor string,
) (batchUserLimitConfig, error) {
	var result batchUserLimitConfig
	if r.tx == nil {
		return result, fmt.Errorf("batch submission pgx transaction is required")
	}
	var expiresAt pgtype.Timestamptz
	err := r.tx.QueryRow(ctx, `
SELECT expires_at
FROM rate_limit_exemptions
WHERE id = $1
`, actor).Scan(&expiresAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return result, err
	default:
		result.ExemptionFound = true
		if expiresAt.Valid {
			value := expiresAt.Time
			result.ExemptionExpiresAt = &value
		}
	}

	var maxParents, maxChildren, cooldownSeconds pgtype.Int8
	err = r.tx.QueryRow(ctx, `
SELECT max_pending_parents, max_pending_children, cooldown_seconds
FROM rate_limit_user_overrides
WHERE id = $1
`, actor).Scan(&maxParents, &maxChildren, &cooldownSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	var convertErr error
	if result.MaxPendingParents, convertErr = nullablePGXInt(maxParents); convertErr != nil {
		return result, fmt.Errorf("decode max pending parents: %w", convertErr)
	}
	if result.MaxPendingChildren, convertErr = nullablePGXInt(maxChildren); convertErr != nil {
		return result, fmt.Errorf("decode max pending children: %w", convertErr)
	}
	if result.CooldownSeconds, convertErr = nullablePGXInt(cooldownSeconds); convertErr != nil {
		return result, fmt.Errorf("decode cooldown seconds: %w", convertErr)
	}
	return result, nil
}

func nullablePGXInt(value pgtype.Int8) (*int, error) {
	if !value.Valid {
		return nil, nil
	}
	converted, err := strconv.Atoi(strconv.FormatInt(value.Int64, 10))
	if err != nil {
		return nil, err
	}
	return &converted, nil
}

func (r pgxBatchSubmissionLimitReader) recentSubmissionCount(
	ctx context.Context,
	since time.Time,
) (int, error) {
	if r.tx == nil {
		return 0, fmt.Errorf("batch submission pgx transaction is required")
	}
	var count int
	err := r.tx.QueryRow(ctx, `
SELECT count(*)
FROM domain_events
WHERE aggregate_type = 'batch'
  AND event_type = ANY($1::text[])
  AND created_at >= $2
`, batchParentEventTypes(), since).Scan(&count)
	return count, err
}

func (r pgxBatchSubmissionLimitReader) pendingChildCount(
	ctx context.Context,
	actor string,
) (int, error) {
	if r.tx == nil {
		return 0, fmt.Errorf("batch submission pgx transaction is required")
	}
	var count int
	err := r.tx.QueryRow(ctx, `
SELECT count(*)
FROM tickets
WHERE requester = $1
  AND parent_ticket_id IS NOT NULL
  AND status = ANY($2::text[])
`, actor, []string{
		string(entticket.StatusPENDING),
		string(entticket.StatusAPPROVED),
		string(entticket.StatusEXECUTING),
	}).Scan(&count)
	return count, err
}

func (r pgxBatchSubmissionLimitReader) latestSubmissionAt(
	ctx context.Context,
	actor string,
) (time.Time, bool, error) {
	if r.tx == nil {
		return time.Time{}, false, fmt.Errorf("batch submission pgx transaction is required")
	}
	var createdAt time.Time
	err := r.tx.QueryRow(ctx, `
SELECT created_at
FROM domain_events
WHERE aggregate_type = 'batch'
  AND event_type = ANY($1::text[])
  AND created_by = $2
ORDER BY created_at DESC
LIMIT 1
`, batchParentEventTypes(), actor).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return createdAt, true, nil
}

func defaultBatchUserLimitPolicy() batchUserLimitPolicy {
	return batchUserLimitPolicy{
		Exempt:              false,
		UsesDefaultParents:  true,
		UsesDefaultChildren: true,
		UsesDefaultCooldown: true,
		MaxPendingParents:   maxPendingBatchParentsUser,
		MaxPendingChildren:  maxPendingBatchChildrenUser,
		Cooldown:            batchSubmitCooldown,
	}
}

func (s *Server) resolveBatchUserLimitPolicy(ctx context.Context, actor string) (batchUserLimitPolicy, error) {
	return resolveBatchUserLimitPolicyWithClient(ctx, s.client, actor)
}

func resolveBatchUserLimitPolicyWithClient(
	ctx context.Context,
	client *ent.Client,
	actor string,
) (batchUserLimitPolicy, error) {
	return resolveBatchUserLimitPolicyWithReader(
		ctx,
		entBatchSubmissionLimitReader{client: client},
		actor,
		time.Now().UTC(),
	)
}

func resolveBatchUserLimitPolicyWithReader(
	ctx context.Context,
	reader batchSubmissionLimitReader,
	actor string,
	now time.Time,
) (batchUserLimitPolicy, error) {
	policy := defaultBatchUserLimitPolicy()
	config, err := reader.userLimitConfig(ctx, actor)
	if err != nil {
		return policy, err
	}
	if config.ExemptionFound {
		// Policy resolution is intentionally read-only. Deleting an expired row
		// here can race with an administrator extending the same exemption and
		// erase the newly committed state. Expired rows are simply ignored.
		if config.ExemptionExpiresAt == nil || !config.ExemptionExpiresAt.Before(now) {
			policy.Exempt = true
			if config.ExemptionExpiresAt != nil {
				exp := *config.ExemptionExpiresAt
				policy.ExemptionExpiresAt = &exp
			}
		}
	}
	if config.MaxPendingParents != nil && *config.MaxPendingParents > 0 {
		policy.MaxPendingParents = *config.MaxPendingParents
		policy.UsesDefaultParents = false
	}
	if config.MaxPendingChildren != nil && *config.MaxPendingChildren > 0 {
		policy.MaxPendingChildren = *config.MaxPendingChildren
		policy.UsesDefaultChildren = false
	}
	if config.CooldownSeconds != nil && *config.CooldownSeconds >= 0 {
		policy.Cooldown = time.Duration(*config.CooldownSeconds) * time.Second
		policy.UsesDefaultCooldown = false
	}
	return policy, nil
}

func evaluateAdditionalBatchSubmissionLimitsWithReader(
	ctx context.Context,
	reader batchSubmissionLimitReader,
	actor string,
	requestedChildCount int,
	policy batchUserLimitPolicy,
	now time.Time,
) (*batchSubmissionLimitViolation, error) {
	recentSince := now.Add(-time.Minute)
	globalRecentSubmits, err := reader.recentSubmissionCount(ctx, recentSince)
	if err != nil {
		return nil, err
	}
	if globalRecentSubmits >= maxGlobalBatchRequestsPerMinute {
		return &batchSubmissionLimitViolation{
			Reason:              "global_request_rate_limit",
			RetryAfterSeconds:   60,
			GlobalRecentSubmits: globalRecentSubmits,
			ContactAdmin:        true,
		}, nil
	}

	if policy.Exempt {
		return nil, nil
	}

	userPendingChildren, err := reader.pendingChildCount(ctx, actor)
	if err != nil {
		return nil, err
	}
	if userPendingChildren+requestedChildCount > policy.MaxPendingChildren {
		return &batchSubmissionLimitViolation{
			Reason:              "user_pending_child_limit",
			RetryAfterSeconds:   batchRetryAfterSeconds,
			GlobalRecentSubmits: globalRecentSubmits,
			UserPendingChildren: userPendingChildren,
			ContactAdmin:        policy.UsesDefaultChildren,
		}, nil
	}

	lastSubmittedAt, found, err := reader.latestSubmissionAt(ctx, actor)
	if err != nil {
		return nil, err
	}
	if found {
		cooldownRemaining := lastSubmittedAt.Add(policy.Cooldown).Sub(now)
		if cooldownRemaining > 0 {
			cooldownSeconds := int(math.Ceil(cooldownRemaining.Seconds()))
			return &batchSubmissionLimitViolation{
				Reason:              "user_submit_cooldown",
				RetryAfterSeconds:   cooldownSeconds,
				GlobalRecentSubmits: globalRecentSubmits,
				UserPendingChildren: userPendingChildren,
				UserCooldownSeconds: cooldownSeconds,
				ContactAdmin:        policy.UsesDefaultCooldown,
			}, nil
		}
	}

	return nil, nil
}

type batchSubmissionRateLimitError struct {
	PendingParentLimit bool
	GlobalPending      int
	UserPending        int
	Policy             batchUserLimitPolicy
	Additional         *batchSubmissionLimitViolation
	RequestedChildren  int
}

func (e *batchSubmissionRateLimitError) Error() string {
	if e == nil {
		return batchRateLimitedMessage
	}
	if e.PendingParentLimit {
		return "batch submission throttled by pending parent limits"
	}
	if e.Additional != nil {
		return "batch submission throttled by " + e.Additional.Reason
	}
	return batchRateLimitedMessage
}

func evaluateBatchSubmissionRateLimits(
	ctx context.Context,
	reader batchSubmissionLimitReader,
	actor string,
	requestedChildCount int,
	now time.Time,
) (*batchSubmissionRateLimitError, error) {
	found, enabled, err := reader.activeUser(ctx, actor)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errBatchSubmissionActorNotFound
	}
	if !enabled {
		return nil, errBatchSubmissionActorNotAvailable
	}
	globalPending, userPending, err := reader.pendingParentCounters(ctx, actor)
	if err != nil {
		return nil, err
	}
	policy, err := resolveBatchUserLimitPolicyWithReader(ctx, reader, actor, now)
	if err != nil {
		return nil, err
	}
	userPendingParentExceeded := !policy.Exempt && userPending >= policy.MaxPendingParents
	if globalPending >= maxPendingBatchParents || userPendingParentExceeded {
		return &batchSubmissionRateLimitError{
			PendingParentLimit: true,
			GlobalPending:      globalPending,
			UserPending:        userPending,
			Policy:             policy,
			RequestedChildren:  requestedChildCount,
		}, nil
	}
	additional, err := evaluateAdditionalBatchSubmissionLimitsWithReader(
		ctx,
		reader,
		actor,
		requestedChildCount,
		policy,
		now,
	)
	if err != nil {
		return nil, err
	}
	if additional == nil {
		return nil, nil
	}
	return &batchSubmissionRateLimitError{
		GlobalPending:     globalPending,
		UserPending:       userPending,
		Policy:            policy,
		Additional:        additional,
		RequestedChildren: requestedChildCount,
	}, nil
}

func writeBatchSubmissionActorStateError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, errBatchSubmissionActorNotFound):
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return true
	case errors.Is(err, errBatchSubmissionActorNotAvailable):
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return true
	default:
		return false
	}
}

func writeBatchSubmissionRateLimitResponse(
	c *gin.Context,
	rateLimit *batchSubmissionRateLimitError,
	powerBatch bool,
) {
	if rateLimit == nil {
		return
	}
	if rateLimit.PendingParentLimit {
		message := "batch submission throttled by pending parent limits"
		if powerBatch {
			message = "batch power submission throttled by pending parent limits"
		}
		userPendingParentExceeded := !rateLimit.Policy.Exempt &&
			rateLimit.UserPending >= rateLimit.Policy.MaxPendingParents
		contactAdmin := rateLimit.GlobalPending >= maxPendingBatchParents ||
			(userPendingParentExceeded && rateLimit.Policy.UsesDefaultParents)
		c.Header("Retry-After", strconv.Itoa(batchRetryAfterSeconds))
		c.JSON(http.StatusTooManyRequests, generated.Error{
			Code:    "BATCH_RATE_LIMITED",
			Message: message,
			Params: map[string]interface{}{
				"retry_after_seconds": batchRetryAfterSeconds,
				"global_pending":      rateLimit.GlobalPending,
				"user_pending":        rateLimit.UserPending,
				"user_exempted":       rateLimit.Policy.Exempt,
				"max_user_pending":    rateLimit.Policy.MaxPendingParents,
				"contact_admin":       contactAdmin,
			},
		})
		return
	}

	additional := rateLimit.Additional
	if additional == nil {
		return
	}
	retryAfter := additional.RetryAfterSeconds
	if retryAfter <= 0 {
		retryAfter = batchRetryAfterSeconds
	}
	message := "batch submission throttled by additional rate limits"
	if powerBatch {
		message = "batch power submission throttled by additional rate limits"
	}
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	c.JSON(http.StatusTooManyRequests, generated.Error{
		Code:    "BATCH_RATE_LIMITED",
		Message: message,
		Params: map[string]interface{}{
			"reason":                    additional.Reason,
			"retry_after_seconds":       retryAfter,
			"global_recent_submits":     additional.GlobalRecentSubmits,
			"user_pending_children":     additional.UserPendingChildren,
			"user_cooldown_seconds":     additional.UserCooldownSeconds,
			"requested_child_count":     rateLimit.RequestedChildren,
			"max_global_per_minute":     maxGlobalBatchRequestsPerMinute,
			"max_user_pending_children": rateLimit.Policy.MaxPendingChildren,
			"user_exempted":             rateLimit.Policy.Exempt,
			"contact_admin":             additional.ContactAdmin,
		},
	})
}

func findBatchByRequestIDWithClient(
	ctx context.Context,
	client *ent.Client,
	actor string,
	op string,
	requestID string,
) (batchID string, found bool, err error) {
	requestID = batchreplay.Normalize(requestID)
	if requestID == "" {
		return "", false, nil
	}
	wantedOperation := strings.ToUpper(strings.TrimSpace(op))
	projectionType, parentOperation, identityOK := batchReplayIdentityForOperation(wantedOperation)
	if !identityOK {
		return "", false, fmt.Errorf("unsupported batch replay operation %q", op)
	}

	batches, err := client.BatchTicket.Query().
		Where(
			entbatchticket.CreatedByEQ(actor),
			entbatchticket.BatchTypeEQ(projectionType),
			entbatchticket.RequestIDNotNil(),
			batchTicketNormalizedRequestIDEquals(requestID),
		).
		Order(
			ent.Asc(entbatchticket.FieldCreatedAt),
			ent.Asc(entbatchticket.FieldID),
		).
		Limit(batchreplay.CandidateLimit + 1).
		All(ctx)
	if err != nil {
		return "", false, err
	}
	if len(batches) > batchreplay.CandidateLimit {
		return "", false, fmt.Errorf("batch replay integrity violation: more than %d matching projections", batchreplay.CandidateLimit)
	}

	matchedBatchID := ""
	for _, batch := range batches {
		parent, parentErr := client.Ticket.Query().
			Where(
				entticket.IDEQ(batch.ID),
				entticket.ParentTicketIDIsNil(),
			).
			Only(ctx)
		if ent.IsNotFound(parentErr) {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: root ticket is missing", batch.ID)
		}
		if parentErr != nil {
			return "", false, parentErr
		}
		if parent.OperationType != parentOperation ||
			strings.TrimSpace(parent.Requester) != strings.TrimSpace(actor) ||
			strings.TrimSpace(parent.EventID) == "" {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: root ticket identity is inconsistent", batch.ID)
		}
		event, eventErr := client.DomainEvent.Get(ctx, parent.EventID)
		if ent.IsNotFound(eventErr) {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: parent event is missing", batch.ID)
		}
		if eventErr != nil {
			return "", false, eventErr
		}
		if !batchParentEventMatchesOperation(parent.OperationType, domain.EventType(event.EventType)) ||
			strings.TrimSpace(event.AggregateType) != batchResourceType ||
			strings.TrimSpace(event.AggregateID) != batch.ID ||
			strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(actor) {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: parent event identity is inconsistent", batch.ID)
		}
		var payload domain.BatchVMRequestPayload
		if decodeErr := json.Unmarshal(event.Payload, &payload); decodeErr != nil {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: parent event payload is malformed: %w", batch.ID, decodeErr)
		}
		payloadOperation := strings.ToUpper(strings.TrimSpace(payload.Operation))
		if normalizeBatchRequestID(payload.RequestID) != requestID {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: payload request ID does not match its projection", batch.ID)
		}
		if submittedBy := strings.TrimSpace(payload.SubmittedBy); submittedBy != strings.TrimSpace(actor) {
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: payload submitter does not match its durable owner", batch.ID)
		}
		if payloadOperation != wantedOperation {
			// All power actions intentionally share one projection type while
			// retaining separate idempotency scopes. A valid sibling action is
			// not corruption and must be skipped deterministically.
			if parentOperation == entticket.OperationTypePOWER && isBatchPowerReplayOperation(payloadOperation) {
				continue
			}
			return "", false, fmt.Errorf("batch replay integrity violation for projection %s: payload operation does not match its durable type", batch.ID)
		}
		if matchedBatchID == "" {
			matchedBatchID = batch.ID
		}
	}
	if matchedBatchID != "" {
		return matchedBatchID, true, nil
	}
	return "", false, nil
}

func batchReplayIdentityForOperation(operation string) (entbatchticket.BatchType, entticket.OperationType, bool) {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case "CREATE":
		return entbatchticket.BatchTypeBATCH_CREATE, entticket.OperationTypeCREATE, true
	case "MODIFY":
		return entbatchticket.BatchTypeBATCH_MODIFY, entticket.OperationTypeMODIFY, true
	case "DELETE":
		return entbatchticket.BatchTypeBATCH_DELETE, entticket.OperationTypeDELETE, true
	case batchPowerOperationStart, batchPowerOperationStop, batchPowerOperationRestart:
		return entbatchticket.BatchTypeBATCH_POWER, entticket.OperationTypePOWER, true
	default:
		return "", "", false
	}
}

func isBatchPowerReplayOperation(operation string) bool {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case batchPowerOperationStart, batchPowerOperationStop, batchPowerOperationRestart:
		return true
	default:
		return false
	}
}

// loadBatchOwner performs the authorization preflight without touching the
// projection. Callers must complete this check before loadBatchView, whose
// legacy missing-projection path can perform a controlled backfill.
func (s *Server) loadBatchOwner(ctx context.Context, batchID string) (string, error) {
	parent, err := s.client.Ticket.Query().
		Where(
			entticket.IDEQ(strings.TrimSpace(batchID)),
			entticket.ParentTicketIDIsNil(),
		).
		Select(entticket.FieldRequester).
		Only(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(parent.Requester), nil
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
	if projection == nil {
		if backfillErr := s.backfillMissingBatchProjection(ctx, parent.ID); backfillErr != nil {
			return generated.VMBatchStatusResponse{}, nil, backfillErr
		}
		// The backfill rereads and locks the complete graph. Reload once so the
		// returned view is based on state at or after that authoritative snapshot.
		return s.loadBatchView(ctx, parent.ID)
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

		childStatuses = append(childStatuses, generated.VMBatchChildStatus{
			TicketId:     child.ID,
			EventId:      child.EventID,
			Status:       generated.VMBatchChildStatusStatus(child.Status),
			ResourceId:   resourceID,
			ResourceName: resourceName,
			LastError:    lastError,
			AttemptCount: int(child.AttemptCount),
			Provisioning: s.loadVMProvisioning(ctx, createVMByTicketID[child.ID]),
		})
	}

	status := aggregateBatchParentStatus(
		len(children),
		successCount,
		failedCount,
		pendingCount,
		pendingOnly,
		executing,
		cancelled,
		parent.Status == entticket.StatusPENDING,
	)
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

func (s *Server) backfillMissingBatchProjection(ctx context.Context, parentID string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("batch projection backfill dependencies are not initialized")
	}
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return fmt.Errorf("batch projection backfill parent id is required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin batch projection backfill %s: %w", parentID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, lockErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		usecase.BatchMutationLockKey(parentID),
	); lockErr != nil {
		return fmt.Errorf("lock batch projection backfill %s: %w", parentID, lockErr)
	}

	var (
		parentStatus string
		requester    string
		reason       string
		eventType    string
	)
	if reloadErr := tx.QueryRow(ctx, `
SELECT parent.status, parent.requester, COALESCE(parent.reason, ''), event.event_type
FROM tickets AS parent
JOIN domain_events AS event ON event.id = parent.event_id
WHERE parent.id = $1
  AND parent.parent_ticket_id IS NULL
`, parentID).Scan(&parentStatus, &requester, &reason, &eventType); reloadErr != nil {
		return fmt.Errorf("reload batch projection parent %s: %w", parentID, reloadErr)
	}
	var projectionExists bool
	if existsErr := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM batch_tickets WHERE id = $1)`, parentID).Scan(&projectionExists); existsErr != nil {
		return fmt.Errorf("recheck batch projection %s: %w", parentID, existsErr)
	}
	if projectionExists {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return fmt.Errorf("commit batch projection recheck %s: %w", parentID, commitErr)
		}
		return nil
	}

	_, ok := batchViewOperationForEvent(eventType)
	if !ok {
		return errBatchNotFound
	}
	rows, err := tx.Query(ctx, `
SELECT status
FROM tickets
WHERE parent_ticket_id = $1
ORDER BY id
FOR UPDATE
`, parentID)
	if err != nil {
		return fmt.Errorf("lock batch projection children %s: %w", parentID, err)
	}
	var childCount, successCount, failedCount, pendingCount, cancelledCount, pendingOnly, executingCount int
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			rows.Close()
			return fmt.Errorf("scan batch projection child %s: %w", parentID, err)
		}
		childCount++
		switch status {
		case string(entticket.StatusSUCCESS):
			successCount++
		case string(entticket.StatusFAILED), string(entticket.StatusREJECTED):
			failedCount++
		case string(entticket.StatusCANCELLED):
			cancelledCount++
		case string(entticket.StatusPENDING):
			pendingCount++
			pendingOnly++
		default:
			pendingCount++
			executingCount++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate batch projection children %s: %w", parentID, err)
	}
	rows.Close()
	// Follow the batch mutation lock order: all child tickets first, then the
	// parent/event identity. This second read is authoritative for the backfill
	// and prevents a parent transition from racing the insert.
	if lockParentErr := tx.QueryRow(ctx, `
SELECT parent.status, parent.requester, COALESCE(parent.reason, ''), event.event_type
FROM tickets AS parent
JOIN domain_events AS event ON event.id = parent.event_id
WHERE parent.id = $1
  AND parent.parent_ticket_id IS NULL
FOR UPDATE OF parent, event
`, parentID).Scan(&parentStatus, &requester, &reason, &eventType); lockParentErr != nil {
		return fmt.Errorf("lock batch projection parent %s: %w", parentID, lockParentErr)
	}
	operation, ok := batchViewOperationForEvent(eventType)
	if !ok {
		return errBatchNotFound
	}
	status := mapProjectionStatus(aggregateBatchParentStatus(
		childCount,
		successCount,
		failedCount,
		pendingCount,
		pendingOnly,
		executingCount,
		cancelledCount,
		parentStatus == string(entticket.StatusPENDING),
	))
	if _, err := tx.Exec(ctx, `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by, reason
)
VALUES ($1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''))
ON CONFLICT (id) DO NOTHING
`,
		parentID,
		string(toBatchProjectionType(string(operation))),
		childCount,
		successCount,
		failedCount,
		pendingCount,
		string(status),
		strings.TrimSpace(requester),
		strings.TrimSpace(reason),
	); err != nil {
		return fmt.Errorf("backfill batch projection %s: %w", parentID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch projection backfill %s: %w", parentID, err)
	}
	return nil
}

func batchViewOperationForEvent(eventType string) (generated.VMBatchOperation, bool) {
	switch domain.EventType(strings.TrimSpace(eventType)) {
	case domain.EventBatchDeleteRequested:
		return generated.VMBatchOperation("DELETE"), true
	case domain.EventBatchCreateRequested:
		return generated.VMBatchOperation("CREATE"), true
	case domain.EventBatchModifyRequested:
		return generated.VMBatchOperation("MODIFY"), true
	case domain.EventBatchPowerRequested:
		return generated.VMBatchOperation("POWER"), true
	default:
		return "", false
	}
}

func aggregateBatchParentStatus(
	total int,
	successCount int,
	failedCount int,
	pendingCount int,
	pendingOnly int,
	executingCount int,
	cancelledCount int,
	parentPending bool,
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
	if pendingOnly == total && parentPending {
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
	case string(generated.VMBatchOperation("POWER")), batchPowerOperationStart, batchPowerOperationStop, batchPowerOperationRestart:
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
	if rejectInvalidEnumQuery(
		c,
		"sort_order",
		string(params.SortOrder),
		string(generated.ListVMBatchesParamsSortOrderAsc),
		string(generated.ListVMBatchesParamsSortOrderDesc),
	) {
		return
	}
	sortBy := strings.TrimSpace(string(params.SortBy))
	if rejectInvalidEnumQuery(c, "sort_by", sortBy, queryFieldCreatedAt, queryFieldStatus, "child_count", "success_count", "failed_count", "pending_count") {
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
	query := s.client.BatchTicket.Query().Where(entbatchticket.BatchTypeIn(
		entbatchticket.BatchTypeBATCH_CREATE,
		entbatchticket.BatchTypeBATCH_MODIFY,
		entbatchticket.BatchTypeBATCH_DELETE,
		entbatchticket.BatchTypeBATCH_POWER,
	))
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
	orderField := entbatchticket.FieldCreatedAt
	switch sortBy {
	case queryFieldStatus:
		orderField = entbatchticket.FieldStatus
	case "child_count":
		orderField = entbatchticket.FieldChildCount
	case "success_count":
		orderField = entbatchticket.FieldSuccessCount
	case "failed_count":
		orderField = entbatchticket.FieldFailedCount
	case "pending_count":
		orderField = entbatchticket.FieldPendingCount
	}
	orderFn := ent.Desc(orderField)
	if !desc {
		orderFn = ent.Asc(orderField)
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
		operation, ok := publicBatchOperation(row.BatchType)
		if !ok {
			logger.Error("unsupported persisted batch type reached public list", zap.String("batch_id", row.ID), zap.String("batch_type", string(row.BatchType)))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
		items = append(items, generated.VMBatchJobSummary{
			Id:           row.ID,
			Operation:    operation,
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

func publicBatchOperation(batchType entbatchticket.BatchType) (generated.VMBatchOperation, bool) {
	switch batchType {
	case entbatchticket.BatchTypeBATCH_CREATE:
		return generated.VMBatchOperationCREATE, true
	case entbatchticket.BatchTypeBATCH_MODIFY:
		return generated.VMBatchOperationMODIFY, true
	case entbatchticket.BatchTypeBATCH_DELETE:
		return generated.VMBatchOperationDELETE, true
	case entbatchticket.BatchTypeBATCH_POWER:
		return generated.VMBatchOperationPOWER, true
	default:
		return "", false
	}
}
