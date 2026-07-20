package handlers

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/ratelimitexemption"
	"kv-shepherd.io/shepherd/ent/ratelimituseroverride"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type rateLimitExemptionRequest struct {
	UserID    string     `json:"user_id" binding:"required"`
	Reason    *string    `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type rateLimitUserOverrideRequest struct {
	MaxPendingParents  *int    `json:"max_pending_parents"`
	MaxPendingChildren *int    `json:"max_pending_children"`
	CooldownSeconds    *int    `json:"cooldown_seconds"`
	Reason             *string `json:"reason"`
}

// CreateRateLimitExemption handles POST /admin/rate-limits/exemptions.
func (s *Server) CreateRateLimitExemption(c *gin.Context) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	var req rateLimitExemptionRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: "user_id is required"})
		return
	}
	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now().UTC()) {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "expires_at must be in the future",
		})
		return
	}
	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}

	var (
		userEnt *ent.User
		saved   *ent.RateLimitExemption
	)
	err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if err := lockUserMutation(ctx, tx, userID); err != nil {
			return err
		}

		txClient := tx.Client()
		var err error
		userEnt, err = txClient.User.Get(ctx, userID)
		if err != nil {
			return err
		}

		existing, err := txClient.RateLimitExemption.Query().
			Where(ratelimitexemption.IDEQ(userID)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return err
		}

		if ent.IsNotFound(err) {
			create := txClient.RateLimitExemption.Create().
				SetID(userID).
				SetExemptedBy(actor)
			if reason != "" {
				create = create.SetReason(reason)
			}
			if req.ExpiresAt != nil {
				create = create.SetExpiresAt(*req.ExpiresAt)
			}
			saved, err = create.Save(ctx)
			return err
		}

		update := existing.Update().SetExemptedBy(actor)
		if reason != "" {
			update = update.SetReason(reason)
		} else {
			update = update.ClearReason()
		}
		if req.ExpiresAt != nil {
			update = update.SetExpiresAt(*req.ExpiresAt)
		} else {
			update = update.ClearExpiresAt()
		}
		saved, err = update.Save(ctx)
		return err
	})
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to save rate-limit exemption", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "admin.rate_limit.exemption.upsert", "user", userID, actor, map[string]interface{}{
			"expires_at": saved.ExpiresAt,
		})
	}

	expiresAt := time.Time{}
	if saved.ExpiresAt != nil {
		expiresAt = *saved.ExpiresAt
	}

	c.JSON(http.StatusOK, generated.RateLimitExemption{
		UserId:      saved.ID,
		Username:    userEnt.Username,
		DisplayName: userEnt.DisplayName,
		Email:       userEnt.Email,
		ExemptedBy:  saved.ExemptedBy,
		Reason:      saved.Reason,
		ExpiresAt:   expiresAt,
		CreatedAt:   saved.CreatedAt,
		UpdatedAt:   saved.UpdatedAt,
	})
}

// DeleteRateLimitExemption handles DELETE /admin/rate-limits/exemptions/{user_id}.
func (s *Server) DeleteRateLimitExemption(c *gin.Context, userID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if err := lockUserMutation(ctx, tx, trimmedUserID); err != nil {
			return err
		}
		return tx.Client().RateLimitExemption.DeleteOneID(trimmedUserID).Exec(ctx)
	})
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "RATE_LIMIT_EXEMPTION_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete rate-limit exemption", zap.Error(err), zap.String("user_id", trimmedUserID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "admin.rate_limit.exemption.delete", "user", trimmedUserID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// UpdateRateLimitUserOverrides handles PUT /admin/rate-limits/users/{user_id}.
func (s *Server) UpdateRateLimitUserOverrides(c *gin.Context, userID string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	var req rateLimitUserOverrideRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	if req.MaxPendingParents == nil && req.MaxPendingChildren == nil && req.CooldownSeconds == nil && req.Reason == nil {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "at least one override field must be provided",
		})
		return
	}
	if req.MaxPendingParents != nil && *req.MaxPendingParents < 1 {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "max_pending_parents must be >= 1",
		})
		return
	}
	if req.MaxPendingChildren != nil && *req.MaxPendingChildren < 1 {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "max_pending_children must be >= 1",
		})
		return
	}
	if req.CooldownSeconds != nil && *req.CooldownSeconds < 0 {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "cooldown_seconds must be >= 0",
		})
		return
	}

	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}

	var saved *ent.RateLimitUserOverride
	err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if err := lockUserMutation(ctx, tx, trimmedUserID); err != nil {
			return err
		}

		txClient := tx.Client()
		if _, err := txClient.User.Get(ctx, trimmedUserID); err != nil {
			return err
		}

		existing, err := txClient.RateLimitUserOverride.Query().
			Where(ratelimituseroverride.IDEQ(trimmedUserID)).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return err
		}
		if ent.IsNotFound(err) {
			create := txClient.RateLimitUserOverride.Create().
				SetID(trimmedUserID).
				SetUpdatedBy(actor)
			if req.MaxPendingParents != nil {
				create = create.SetMaxPendingParents(*req.MaxPendingParents)
			}
			if req.MaxPendingChildren != nil {
				create = create.SetMaxPendingChildren(*req.MaxPendingChildren)
			}
			if req.CooldownSeconds != nil {
				create = create.SetCooldownSeconds(*req.CooldownSeconds)
			}
			if req.Reason != nil && reason != "" {
				create = create.SetReason(reason)
			}
			saved, err = create.Save(ctx)
			return err
		}

		update := existing.Update().SetUpdatedBy(actor)
		if req.MaxPendingParents != nil {
			update = update.SetMaxPendingParents(*req.MaxPendingParents)
		}
		if req.MaxPendingChildren != nil {
			update = update.SetMaxPendingChildren(*req.MaxPendingChildren)
		}
		if req.CooldownSeconds != nil {
			update = update.SetCooldownSeconds(*req.CooldownSeconds)
		}
		if req.Reason != nil {
			if reason == "" {
				update = update.ClearReason()
			} else {
				update = update.SetReason(reason)
			}
		}
		saved, err = update.Save(ctx)
		return err
	})
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to save rate-limit override", zap.Error(err), zap.String("user_id", trimmedUserID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "admin.rate_limit.override.upsert", "user", trimmedUserID, actor, map[string]interface{}{
			"max_pending_parents":  saved.MaxPendingParents,
			"max_pending_children": saved.MaxPendingChildren,
			"cooldown_seconds":     saved.CooldownSeconds,
		})
	}

	maxPendingParents := 0
	if saved.MaxPendingParents != nil {
		maxPendingParents = *saved.MaxPendingParents
	}
	maxPendingChildren := 0
	if saved.MaxPendingChildren != nil {
		maxPendingChildren = *saved.MaxPendingChildren
	}
	cooldownSeconds := 0
	if saved.CooldownSeconds != nil {
		cooldownSeconds = *saved.CooldownSeconds
	}

	c.JSON(http.StatusOK, generated.RateLimitUserOverride{
		UserId:             saved.ID,
		MaxPendingParents:  maxPendingParents,
		MaxPendingChildren: maxPendingChildren,
		CooldownSeconds:    cooldownSeconds,
		Reason:             saved.Reason,
		UpdatedBy:          saved.UpdatedBy,
		CreatedAt:          saved.CreatedAt,
		UpdatedAt:          saved.UpdatedAt,
	})
}

// ListRateLimitStatus handles GET /admin/rate-limits/status.
func (s *Server) ListRateLimitStatus(c *gin.Context) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	now := time.Now().UTC()
	parentCounts := map[string]int{}
	childCounts := map[string]int{}
	lastBatchEventByUser := map[string]time.Time{}
	candidates := map[string]struct{}{}

	parentEvents, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ(batchResourceType),
			domainevent.EventTypeIn(batchParentEventTypes()...),
			domainevent.StatusIn(domainevent.StatusPENDING, domainevent.StatusPROCESSING),
		).
		All(ctx)
	if err != nil {
		logger.Error("failed to list pending parent batch events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, ev := range parentEvents {
		actor := strings.TrimSpace(ev.CreatedBy)
		if actor == "" {
			continue
		}
		parentCounts[actor]++
		candidates[actor] = struct{}{}
	}

	childTickets, err := s.client.Ticket.Query().
		Where(
			entticket.ParentTicketIDNotNil(),
			entticket.StatusIn(
				entticket.StatusPENDING,
				entticket.StatusAPPROVED,
				entticket.StatusEXECUTING,
			),
		).
		All(ctx)
	if err != nil {
		logger.Error("failed to list pending child batch tickets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, ticket := range childTickets {
		requester := strings.TrimSpace(ticket.Requester)
		if requester == "" {
			continue
		}
		childCounts[requester]++
		candidates[requester] = struct{}{}
	}

	recentEvents, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ(batchResourceType),
			domainevent.EventTypeIn(batchParentEventTypes()...),
		).
		Order(ent.Desc(domainevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list batch events for cooldown status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, ev := range recentEvents {
		actor := strings.TrimSpace(ev.CreatedBy)
		if actor == "" {
			continue
		}
		if _, exists := lastBatchEventByUser[actor]; exists {
			continue
		}
		lastBatchEventByUser[actor] = ev.CreatedAt
		candidates[actor] = struct{}{}
	}

	exemptions, err := s.client.RateLimitExemption.Query().All(ctx)
	if err != nil {
		logger.Error("failed to list rate-limit exemptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, ex := range exemptions {
		if ex.ExpiresAt != nil && ex.ExpiresAt.Before(now) {
			continue
		}
		candidates[ex.ID] = struct{}{}
	}

	overrides, err := s.client.RateLimitUserOverride.Query().All(ctx)
	if err != nil {
		logger.Error("failed to list rate-limit overrides", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}
	for _, ov := range overrides {
		candidates[ov.ID] = struct{}{}
	}

	userIDs := make([]string, 0, len(candidates))
	for userID := range candidates {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	userMetaByID, err := s.loadUsersByID(c, userIDs)
	if err != nil {
		logger.Error("failed to list users for rate-limit status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.RateLimitUserStatus, 0, len(userIDs))
	for _, userID := range userIDs {
		policy, err := s.resolveBatchUserLimitPolicy(ctx, userID)
		if err != nil {
			logger.Warn("failed to resolve user policy in rate-limit status",
				zap.Error(err),
				zap.String("user_id", userID),
			)
			continue
		}

		cooldownRemaining := 0
		if !policy.Exempt && policy.Cooldown > 0 {
			if last, ok := lastBatchEventByUser[userID]; ok {
				remaining := time.Until(last.Add(policy.Cooldown))
				if remaining > 0 {
					cooldownRemaining = int(math.Ceil(remaining.Seconds()))
				}
			}
		}

		status := generated.RateLimitUserStatus{
			UserId:                      userID,
			Exempted:                    policy.Exempt,
			ExemptionExpiresAt:          effectiveExemptionExpiry(policy.ExemptionExpiresAt),
			EffectiveMaxPendingParents:  policy.MaxPendingParents,
			EffectiveMaxPendingChildren: policy.MaxPendingChildren,
			EffectiveCooldownSeconds:    int(policy.Cooldown.Seconds()),
			CurrentPendingParents:       parentCounts[userID],
			CurrentPendingChildren:      childCounts[userID],
			CooldownRemainingSeconds:    cooldownRemaining,
		}
		if userEnt := userMetaByID[userID]; userEnt != nil {
			status.Username = userEnt.Username
			status.DisplayName = userEnt.DisplayName
			status.Email = userEnt.Email
		}
		items = append(items, status)
	}

	c.JSON(http.StatusOK, generated.RateLimitStatusList{
		Items:       items,
		GeneratedAt: now,
	})
}

func effectiveExemptionExpiry(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}

func (s *Server) loadUsersByID(ctx *gin.Context, userIDs []string) (map[string]*ent.User, error) {
	if len(userIDs) == 0 {
		return map[string]*ent.User{}, nil
	}

	users, err := s.client.User.Query().
		Where(entuser.IDIn(userIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	userMetaByID := make(map[string]*ent.User, len(users))
	for _, userEnt := range users {
		userMetaByID[userEnt.ID] = userEnt
	}
	return userMetaByID, nil
}

// ListRateLimitExemptions handles GET /admin/rate-limits/exemptions.
func (s *Server) ListRateLimitExemptions(c *gin.Context, params generated.ListRateLimitExemptionsParams) {
	ctx, _, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	query := s.client.RateLimitExemption.Query()

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count rate-limit exemptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items, err := query.Offset(offset).Limit(perPage).All(ctx)
	if err != nil {
		logger.Error("failed to list rate-limit exemptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	userIDs := make([]string, 0, len(items))
	for _, ex := range items {
		userIDs = append(userIDs, ex.ID)
	}
	userMetaByID, err := s.loadUsersByID(c, userIDs)
	if err != nil {
		logger.Error("failed to list users for rate-limit exemptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	resp := make([]generated.RateLimitExemption, 0, len(items))
	for _, ex := range items {
		expiresAt := time.Time{}
		if ex.ExpiresAt != nil {
			expiresAt = *ex.ExpiresAt
		}
		item := generated.RateLimitExemption{
			UserId:     ex.ID,
			ExemptedBy: ex.ExemptedBy,
			Reason:     ex.Reason,
			ExpiresAt:  expiresAt,
			CreatedAt:  ex.CreatedAt,
			UpdatedAt:  ex.UpdatedAt,
		}
		if userEnt := userMetaByID[ex.ID]; userEnt != nil {
			item.Username = userEnt.Username
			item.DisplayName = userEnt.DisplayName
			item.Email = userEnt.Email
		}
		resp = append(resp, item)
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.RateLimitExemptionList{
		Items: resp,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
