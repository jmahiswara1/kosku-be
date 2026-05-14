package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// TenantServicer is the interface that TenantHandler depends on.
// It is satisfied by *service.TenantService and can be implemented by test mocks.
type TenantServicer interface {
	ListTenants(ctx context.Context, ownerID uuid.UUID, propertyID *uuid.UUID, limit, offset int) ([]dto.TenantResponse, int, error)
	GetTenant(ctx context.Context, ownerID, tenantID uuid.UUID) (dto.TenantResponse, error)
	UpdateTenant(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.UpdateTenantRequest) (dto.TenantResponse, error)
	Checkin(ctx context.Context, ownerID uuid.UUID, req dto.CheckinRequest) (dto.CheckinResponse, error)
	Checkout(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.CheckoutRequest) (dto.ContractResponse, error)
	Blacklist(ctx context.Context, ownerID, tenantID uuid.UUID, req dto.BlacklistRequest) (dto.TenantResponse, error)
}

// Ensure *service.TenantService satisfies TenantServicer at compile time.
var _ TenantServicer = (*service.TenantService)(nil)

// TenantHandler holds the dependencies for tenant-related HTTP handlers.
type TenantHandler struct {
	svc TenantServicer
}

// NewTenantHandler creates a new TenantHandler backed by a *service.TenantService.
func NewTenantHandler(svc *service.TenantService) *TenantHandler {
	return &TenantHandler{svc: svc}
}

// NewTenantHandlerWithService creates a new TenantHandler with any TenantServicer.
// Intended for use in tests.
func NewTenantHandlerWithService(svc TenantServicer) *TenantHandler {
	return &TenantHandler{svc: svc}
}

// ListTenants handles GET /v1/tenants.
// Returns all tenants for the owner's properties with pagination.
//
//	@Summary		List tenants
//	@Description	Returns all tenants for the authenticated owner's properties with pagination.
//	@Tags			tenants
//	@Produce		json
//	@Param			property_id	query		string	false	"Filter by property UUID"
//	@Param			page		query		int		false	"Page number (default 1)"
//	@Param			per_page	query		int		false	"Items per page (default 20)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]interface{}
//	@Router			/tenants [get]
//	@Security		BearerAuth
func (h *TenantHandler) ListTenants(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	// Parse pagination params.
	page := 1
	perPage := 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := c.Query("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	offset := (page - 1) * perPage

	// Optional property filter.
	var propertyID *uuid.UUID
	if pidStr := c.Query("property_id"); pidStr != "" {
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property_id"))
			return
		}
		propertyID = &pid
	}

	tenants, total, err := h.svc.ListTenants(c.Request.Context(), ownerID, propertyID, perPage, offset)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_TENANTS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tenants,
		"meta": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
		},
	})
}

// GetTenant handles GET /v1/tenants/:id.
// Returns a single tenant profile.
//
//	@Summary		Get tenant profile
//	@Description	Returns a single tenant's profile. Only accessible by the owning user.
//	@Tags			tenants
//	@Produce		json
//	@Param			id	path		string	true	"Tenant UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/tenants/{id} [get]
//	@Security		BearerAuth
func (h *TenantHandler) GetTenant(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid tenant ID"))
		return
	}

	tenant, err := h.svc.GetTenant(c.Request.Context(), ownerID, tenantID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this tenant's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_TENANT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(tenant))
}

// UpdateTenant handles PUT /v1/tenants/:id.
// Updates a tenant's profile fields.
//
//	@Summary		Update tenant profile
//	@Description	Updates a tenant's profile fields. Only accessible by the owning user.
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Tenant UUID"
//	@Param			body	body		dto.UpdateTenantRequest		true	"Updated tenant data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/tenants/{id} [put]
//	@Security		BearerAuth
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid tenant ID"))
		return
	}

	var req dto.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	tenant, err := h.svc.UpdateTenant(c.Request.Context(), ownerID, tenantID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this tenant's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_TENANT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(tenant))
}

// Checkin handles POST /v1/tenants/checkin.
// Validates room is vacant, creates a contract, updates room status to occupied.
//
//	@Summary		Check in a tenant
//	@Description	Validates room is vacant and tenant is not blacklisted, creates a contract, and updates room status.
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CheckinRequest	true	"Check-in data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Failure		409		{object}	map[string]interface{}
//	@Router			/tenants/checkin [post]
//	@Security		BearerAuth
func (h *TenantHandler) Checkin(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.CheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	result, err := h.svc.Checkin(c.Request.Context(), ownerID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant, room, or property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		if errors.Is(err, service.ErrRoomNotVacant) {
			c.JSON(http.StatusConflict, errorResponse("ROOM_NOT_VACANT", "Room is not vacant"))
			return
		}
		if errors.Is(err, service.ErrTenantBlacklisted) {
			c.JSON(http.StatusConflict, errorResponse("TENANT_BLACKLISTED", "Tenant is blacklisted and cannot be checked in"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("CHECKIN_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(result))
}

// Checkout handles POST /v1/tenants/checkout/:id.
// Records check-out, updates room status to vacant, terminates contract.
//
//	@Summary		Check out a tenant
//	@Description	Records check-out date, updates room status to vacant, and archives the active contract.
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Tenant UUID"
//	@Param			body	body		dto.CheckoutRequest		false	"Check-out data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/tenants/checkout/{id} [post]
//	@Security		BearerAuth
func (h *TenantHandler) Checkout(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid tenant ID"))
		return
	}

	var req dto.CheckoutRequest
	// Body is optional — ignore bind errors.
	_ = c.ShouldBindJSON(&req)

	contract, err := h.svc.Checkout(c.Request.Context(), ownerID, tenantID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this tenant's property"))
			return
		}
		if errors.Is(err, service.ErrNoActiveContract) {
			c.JSON(http.StatusNotFound, errorResponse("NO_ACTIVE_CONTRACT", "No active contract found for this tenant"))
			return
		}
		if errors.Is(err, service.ErrRefundExceedsDeposit) {
			c.JSON(http.StatusBadRequest, errorResponse("REFUND_EXCEEDS_DEPOSIT", "Refund amount exceeds the deposit amount"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("CHECKOUT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(contract))
}

// Blacklist handles POST /v1/tenants/:id/blacklist.
// Sets is_blacklisted = true and stores the reason.
//
//	@Summary		Blacklist a tenant
//	@Description	Flags a tenant as blacklisted with a reason. Blacklisted tenants cannot be checked in.
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Tenant UUID"
//	@Param			body	body		dto.BlacklistRequest	true	"Blacklist reason"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/tenants/{id}/blacklist [post]
//	@Security		BearerAuth
func (h *TenantHandler) Blacklist(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid tenant ID"))
		return
	}

	var req dto.BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	tenant, err := h.svc.Blacklist(c.Request.Context(), ownerID, tenantID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Tenant not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this tenant's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("BLACKLIST_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(tenant))
}
