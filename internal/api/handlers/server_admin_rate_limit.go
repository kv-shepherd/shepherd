package handlers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/ratelimitexemption"
	"kv-shepherd.io/shepherd/ent/ratelimituseroverride"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
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
	if _, err := s.client.User.Get(ctx, userID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for rate-limit exemption", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}
	existing, err := s.client.RateLimitExemption.Query().
		Where(ratelimitexemption.IDEQ(userID)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to query rate-limit exemption", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var saved *ent.RateLimitExemption
	if ent.IsNotFound(err) {
		create := s.client.RateLimitExemption.Create().
			SetID(userID).
			SetExemptedBy(actor)
		if reason != "" {
			create = create.SetReason(reason)
		}
		if req.ExpiresAt != nil {
			create = create.SetExpiresAt(*req.ExpiresAt)
		}
		saved, err = create.Save(ctx)
	} else {
		update := existing.Update().
			SetExemptedBy(actor)
		if reason == "" {
			update = update.ClearReason()
		} else {
			update = update.SetReason(reason)
		}
		if req.ExpiresAt == nil {
			update = update.ClearExpiresAt()
		} else {
			update = update.SetExpiresAt(*req.ExpiresAt)
		}
		saved, err = update.Save(ctx)
	}
	if err != nil {
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
		UserId:     saved.ID,
		ExemptedBy: saved.ExemptedBy,
		Reason:     saved.Reason,
		ExpiresAt:  expiresAt,
		CreatedAt:  saved.CreatedAt,
		UpdatedAt:  saved.UpdatedAt,
	})
}

// DeleteRateLimitExemption handles DELETE /admin/rate-limits/exemptions/{user_id}.
func (s *Server) DeleteRateLimitExemption(c *gin.Context, userId string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	userID := strings.TrimSpace(userId)
	if userID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}

	if err := s.client.RateLimitExemption.DeleteOneID(userID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "RATE_LIMIT_EXEMPTION_NOT_FOUND"})
			return
		}
		logger.Error("failed to delete rate-limit exemption", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "admin.rate_limit.exemption.delete", "user", userID, actor, nil)
	}

	c.Status(http.StatusNoContent)
}

// UpdateRateLimitUserOverrides handles PUT /admin/rate-limits/users/{user_id}.
func (s *Server) UpdateRateLimitUserOverrides(c *gin.Context, userId string) {
	ctx, actor, ok := requireActorWithAnyGlobalPermission(c, "rate_limit:manage")
	if !ok {
		return
	}

	userID := strings.TrimSpace(userId)
	if userID == "" {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return
	}
	if _, err := s.client.User.Get(ctx, userID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "USER_NOT_FOUND"})
			return
		}
		logger.Error("failed to query user for rate-limit override", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var req rateLimitUserOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
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
	existing, err := s.client.RateLimitUserOverride.Query().
		Where(ratelimituseroverride.IDEQ(userID)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		logger.Error("failed to query rate-limit override", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	var saved *ent.RateLimitUserOverride
	if ent.IsNotFound(err) {
		create := s.client.RateLimitUserOverride.Create().
			SetID(userID).
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
	} else {
		update := existing.Update().
			SetUpdatedBy(actor)
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
	}
	if err != nil {
		logger.Error("failed to save rate-limit override", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if s.audit != nil {
		_ = s.audit.LogAction(ctx, "admin.rate_limit.override.upsert", "user", userID, actor, map[string]interface{}{
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
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(
				string(domain.EventBatchCreateRequested),
				string(domain.EventBatchDeleteRequested),
			),
			domainevent.StatusEQ(domainevent.StatusPENDING),
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

	childTickets, err := s.client.ApprovalTicket.Query().
		Where(
			approvalticket.ParentTicketIDNotNil(),
			approvalticket.StatusIn(
				approvalticket.StatusPENDING,
				approvalticket.StatusAPPROVED,
				approvalticket.StatusEXECUTING,
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
			domainevent.AggregateTypeEQ("batch"),
			domainevent.EventTypeIn(
				string(domain.EventBatchCreateRequested),
				string(domain.EventBatchDeleteRequested),
			),
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
					cooldownRemaining = int(remaining.Seconds())
				}
			}
		}

		items = append(items, generated.RateLimitUserStatus{
			UserId:                      userID,
			Exempted:                    policy.Exempt,
			ExemptionExpiresAt:          effectiveExemptionExpiry(policy.ExemptionExpiresAt),
			EffectiveMaxPendingParents:  policy.MaxPendingParents,
			EffectiveMaxPendingChildren: policy.MaxPendingChildren,
			EffectiveCooldownSeconds:    int(policy.Cooldown.Seconds()),
			CurrentPendingParents:       parentCounts[userID],
			CurrentPendingChildren:      childCounts[userID],
			CooldownRemainingSeconds:    cooldownRemaining,
		})
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
