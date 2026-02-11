package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/notification"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ListNotifications handles GET /notifications.
func (s *Server) ListNotifications(c *gin.Context, params generated.ListNotificationsParams) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	query := s.client.Notification.Query().
		Where(notification.HasUserWith(entuser.IDEQ(userID)))

	if params.UnreadOnly {
		query = query.Where(notification.ReadEQ(false))
	}

	page, perPage := defaultPagination(params.Page, params.PerPage)
	offset := (page - 1) * perPage

	total, err := query.Clone().Count(ctx)
	if err != nil {
		logger.Error("failed to count notifications", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	notifications, err := query.
		Offset(offset).
		Limit(perPage).
		Order(ent.Desc(notification.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list notifications", zap.Error(err), zap.Int("page", page))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.Notification, 0, len(notifications))
	for _, n := range notifications {
		items = append(items, notificationToAPI(n))
	}

	totalPages := (total + perPage - 1) / perPage
	c.JSON(http.StatusOK, generated.NotificationList{
		Items: items,
		Pagination: generated.Pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetUnreadCount handles GET /notifications/unread-count.
func (s *Server) GetUnreadCount(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	count, err := s.client.Notification.Query().
		Where(
			notification.HasUserWith(entuser.IDEQ(userID)),
			notification.ReadEQ(false),
		).
		Count(ctx)
	if err != nil {
		logger.Error("failed to count unread notifications", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, generated.UnreadCount{Count: count})
}

// MarkNotificationRead handles POST /notifications/{notification_id}/read.
func (s *Server) MarkNotificationRead(c *gin.Context, notificationId generated.NotificationID) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	// Verify notification exists and belongs to user.
	n, err := s.client.Notification.Query().
		Where(
			notification.IDEQ(notificationId),
			notification.HasUserWith(entuser.IDEQ(userID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, generated.Error{Code: "NOTIFICATION_NOT_FOUND"})
			return
		}
		logger.Error("failed to get notification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	if !n.Read {
		now := time.Now()
		if _, err := s.client.Notification.UpdateOneID(notificationId).
			SetRead(true).
			SetReadAt(now).
			Save(ctx); err != nil {
			logger.Error("failed to mark notification read", zap.Error(err))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
			return
		}
	}

	c.Status(http.StatusNoContent)
}

// MarkAllNotificationsRead handles POST /notifications/read-all.
func (s *Server) MarkAllNotificationsRead(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return
	}

	now := time.Now()
	_, err := s.client.Notification.Update().
		Where(
			notification.HasUserWith(entuser.IDEQ(userID)),
			notification.ReadEQ(false),
		).
		SetRead(true).
		SetReadAt(now).
		Save(ctx)
	if err != nil {
		logger.Error("failed to mark all notifications read", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ---- Converter ----

func notificationToAPI(n *ent.Notification) generated.Notification {
	result := generated.Notification{
		Id:           n.ID,
		Type:         generated.NotificationType(n.Type.String()),
		Title:        n.Title,
		Message:      n.Message,
		Read:         n.Read,
		ResourceType: n.ResourceType,
		ResourceId:   n.ResourceID,
		CreatedAt:    n.CreatedAt,
	}
	if n.ReadAt != nil {
		result.ReadAt = *n.ReadAt
	}
	return result
}
