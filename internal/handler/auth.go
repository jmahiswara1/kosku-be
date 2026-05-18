// Package handler contains the Gin HTTP handler functions for the KosKu API.
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

// AuthServicer is the interface that the AuthHandler depends on.
// It is satisfied by *service.AuthService and can be implemented by test mocks.
type AuthServicer interface {
	Register(ctx context.Context, userID uuid.UUID, req dto.RegisterRequest) (dto.ProfileResponse, error)
	Invite(ctx context.Context, ownerID uuid.UUID, req dto.InviteRequest) (dto.InvitationResponse, error)
	Approve(ctx context.Context, profileID uuid.UUID, tenantEmail string) (dto.ProfileResponse, error)
	Reject(ctx context.Context, profileID uuid.UUID, tenantEmail string) error
}

// Ensure *service.AuthService satisfies AuthServicer at compile time.
var _ AuthServicer = (*service.AuthService)(nil)

// AuthHandler holds the dependencies for auth-related HTTP handlers.
type AuthHandler struct {
	svc AuthServicer
}

// NewAuthHandler creates a new AuthHandler backed by a *service.AuthService.
func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// NewAuthHandlerWithService creates a new AuthHandler with any AuthServicer
// implementation. This constructor is intended for use in tests.
func NewAuthHandlerWithService(svc AuthServicer) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Register handles POST /v1/auth/register.
// It reads the authenticated user's ID from the JWT (set by the Auth middleware),
// upserts a profiles row, and returns the profile with role.
//
//	@Summary		Register / sync user profile
//	@Description	Verifies the Supabase JWT, upserts the profiles row, and returns the profile with role.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.RegisterRequest	true	"Profile data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/auth/register [post]
//	@Security		BearerAuth
func (h *AuthHandler) Register(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextKeyUserID)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	var req dto.RegisterRequest
	// Body is optional — if empty, use defaults from JWT claims
	_ = c.ShouldBindJSON(&req)

	// Validate required fields only if body was provided with content
	if req.FullName == "" && c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "full_name is required"))
		return
	}

	profile, err := h.svc.Register(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("REGISTER_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(profile))
}

// Invite handles POST /v1/auth/invite.
// Creates an invitation record with a unique token (UUID), sets expires_at to
// NOW() + 7 days, and triggers a Resend email.
//
//	@Summary		Send tenant invitation
//	@Description	Creates an invitation record and sends an invitation email to the specified address.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.InviteRequest	true	"Invitation data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/auth/invite [post]
//	@Security		BearerAuth
func (h *AuthHandler) Invite(c *gin.Context) {
	ownerIDStr := c.GetString(middleware.ContextKeyUserID)
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	var req dto.InviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	inv, err := h.svc.Invite(c.Request.Context(), ownerID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVITE_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(inv))
}

// Approve handles POST /v1/auth/approve/:id.
// Sets the tenant profile to active and sends a confirmation email.
//
//	@Summary		Approve pending tenant registration
//	@Description	Activates a pending tenant account and sends a confirmation email.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Profile UUID"
//	@Param			body	body		dto.ApproveRequest	false	"Optional email for notification"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/auth/approve/{id} [post]
//	@Security		BearerAuth
func (h *AuthHandler) Approve(c *gin.Context) {
	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid profile ID"))
		return
	}

	var req dto.ApproveRequest
	// Body is optional — ignore bind errors.
	_ = c.ShouldBindJSON(&req)

	profile, err := h.svc.Approve(c.Request.Context(), profileID, req.Email)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Profile not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("APPROVE_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(profile))
}

// Reject handles POST /v1/auth/reject/:id.
// Sends a rejection email and deletes the pending profile.
//
//	@Summary		Reject pending tenant registration
//	@Description	Sends a rejection email to the tenant and removes the pending profile record.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Profile UUID"
//	@Param			body	body		dto.RejectRequest	false	"Optional email for notification"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/auth/reject/{id} [post]
//	@Security		BearerAuth
func (h *AuthHandler) Reject(c *gin.Context) {
	profileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid profile ID"))
		return
	}

	var req dto.RejectRequest
	// Body is optional — ignore bind errors.
	_ = c.ShouldBindJSON(&req)

	if err := h.svc.Reject(c.Request.Context(), profileID, req.Email); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Profile not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("REJECT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Registration rejected"}})
}

// successResponse wraps data in the standard KosKu success envelope.
func successResponse(data interface{}) gin.H {
	return gin.H{
		"success": true,
		"data":    data,
	}
}

// errorResponse wraps an error code and message in the standard KosKu error envelope.
func errorResponse(code, message string) gin.H {
	return gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
}
