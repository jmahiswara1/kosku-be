package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/service"
)

// NotificationServicer is the interface that NotificationHandler depends on.
type NotificationServicer interface {
	ListNotifications(ctx context.Context, userID uuid.UUID) ([]dto.NotificationResponse, error)
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	MarkOneRead(ctx context.Context, userID, notificationID uuid.UUID) error
}

// Ensure *service.NotificationService satisfies NotificationServicer at compile time.
var _ NotificationServicer = (*service.NotificationService)(nil)

// NotificationHandler holds the dependencies for notification-related HTTP handlers.
type NotificationHandler struct {
	svc NotificationServicer
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// NewNotificationHandlerWithService creates a new NotificationHandler with any
// NotificationServicer. Intended for use in tests.
func NewNotificationHandlerWithService(svc NotificationServicer) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// ListNotifications handles GET /v1/notifications.
// Returns all notifications for the authenticated user, ordered by created_at DESC.
//
//	@Summary		List notifications
//	@Description	Returns all notifications for the authenticated user, ordered by newest first.
//	@Tags			notifications
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/notifications [get]
//	@Security		BearerAuth
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		return
	}

	notifications, err := h.svc.ListNotifications(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_NOTIFICATIONS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    notifications,
		"meta":    gin.H{"total": len(notifications)},
	})
}

// MarkAllRead handles PUT /v1/notifications/read.
// Marks all notifications for the authenticated user as read.
//
//	@Summary		Mark all notifications as read
//	@Description	Marks all unread notifications for the authenticated user as read.
//	@Tags			notifications
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/notifications/read [put]
//	@Security		BearerAuth
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		return
	}

	if err := h.svc.MarkAllRead(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("MARK_ALL_READ_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "All notifications marked as read"},
	})
}

// MarkOneRead handles PUT /v1/notifications/:id/read.
// Marks a single notification as read for the authenticated user.
//
//	@Summary		Mark a notification as read
//	@Description	Marks a single notification as read. Only the owning user can mark their own notifications.
//	@Tags			notifications
//	@Produce		json
//	@Param			id	path		string	true	"Notification UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/notifications/{id}/read [put]
//	@Security		BearerAuth
func (h *NotificationHandler) MarkOneRead(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		return
	}

	notificationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid notification ID"))
		return
	}

	if err := h.svc.MarkOneRead(c.Request.Context(), userID, notificationID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Notification not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("MARK_READ_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Notification marked as read"},
	})
}

// userIDFromContext extracts and parses the user UUID from the Gin context.
// It writes an error response and returns false if the ID is missing or invalid.
func userIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	idStr := c.GetString(middleware.ContextKeyUserID)
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return uuid.UUID{}, false
	}
	return id, true
}
