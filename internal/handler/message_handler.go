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

// MessageServicer is the interface that MessageHandler depends on.
// It is satisfied by *service.MessageService and can be implemented by test mocks.
type MessageServicer interface {
	ListConversations(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error)
	GetThread(ctx context.Context, userID, partnerID uuid.UUID) ([]dto.MessageResponse, error)
	SendMessage(ctx context.Context, senderID uuid.UUID, req dto.SendMessageRequest) (dto.MessageResponse, error)
}

// Ensure *service.MessageService satisfies MessageServicer at compile time.
var _ MessageServicer = (*service.MessageService)(nil)

// MessageHandler holds the dependencies for messaging HTTP handlers.
type MessageHandler struct {
	svc MessageServicer
}

// NewMessageHandler creates a new MessageHandler backed by a *service.MessageService.
func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// NewMessageHandlerWithService creates a new MessageHandler with any MessageServicer.
// Intended for use in tests.
func NewMessageHandlerWithService(svc MessageServicer) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// ListConversations handles GET /v1/messages.
// Returns the list of distinct conversation partners for the authenticated user,
// each with the most recent message in the conversation.
//
//	@Summary		List conversations
//	@Description	Returns distinct conversation partners for the authenticated user with the latest message per conversation.
//	@Tags			messages
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/messages [get]
//	@Security		BearerAuth
func (h *MessageHandler) ListConversations(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextKeyUserID)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	conversations, err := h.svc.ListConversations(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_CONVERSATIONS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    conversations,
		"meta":    gin.H{"total": len(conversations)},
	})
}

// GetThread handles GET /v1/messages/:userId.
// Returns the full message thread between the authenticated user and the given
// partner, ordered by created_at ASC.
//
//	@Summary		Get message thread
//	@Description	Returns all messages between the authenticated user and the specified partner, ordered chronologically.
//	@Tags			messages
//	@Produce		json
//	@Param			userId	path		string	true	"Partner user UUID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/messages/{userId} [get]
//	@Security		BearerAuth
func (h *MessageHandler) GetThread(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextKeyUserID)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	partnerID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid user ID"))
		return
	}

	messages, err := h.svc.GetThread(c.Request.Context(), userID, partnerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("GET_THREAD_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
		"meta":    gin.H{"total": len(messages)},
	})
}

// SendMessage handles POST /v1/messages.
// Inserts a new message row. If the recipient has unread messages from the
// sender older than 30 minutes, an email notification is triggered.
//
//	@Summary		Send a message
//	@Description	Sends a message to another user. Triggers an email notification if the recipient has unread messages older than 30 minutes.
//	@Tags			messages
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.SendMessageRequest	true	"Message data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/messages [post]
//	@Security		BearerAuth
func (h *MessageHandler) SendMessage(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextKeyUserID)
	senderID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	msg, err := h.svc.SendMessage(c.Request.Context(), senderID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Recipient user not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("SEND_MESSAGE_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(msg))
}
