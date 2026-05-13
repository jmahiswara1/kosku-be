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

// PropertyServicer is the interface that PropertyHandler depends on.
// It is satisfied by *service.PropertyService and can be implemented by test mocks.
type PropertyServicer interface {
	ListProperties(ctx context.Context, ownerID uuid.UUID) ([]dto.PropertyResponse, error)
	CreateProperty(ctx context.Context, ownerID uuid.UUID, req dto.CreatePropertyRequest) (dto.PropertyResponse, error)
	GetProperty(ctx context.Context, ownerID, propertyID uuid.UUID) (dto.PropertyResponse, error)
	UpdateProperty(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (dto.PropertyResponse, error)
	ArchiveProperty(ctx context.Context, ownerID, propertyID uuid.UUID) error
}

// Ensure *service.PropertyService satisfies PropertyServicer at compile time.
var _ PropertyServicer = (*service.PropertyService)(nil)

// PropertyHandler holds the dependencies for property-related HTTP handlers.
type PropertyHandler struct {
	svc PropertyServicer
}

// NewPropertyHandler creates a new PropertyHandler backed by a *service.PropertyService.
func NewPropertyHandler(svc *service.PropertyService) *PropertyHandler {
	return &PropertyHandler{svc: svc}
}

// NewPropertyHandlerWithService creates a new PropertyHandler with any PropertyServicer.
// Intended for use in tests.
func NewPropertyHandlerWithService(svc PropertyServicer) *PropertyHandler {
	return &PropertyHandler{svc: svc}
}

// ListProperties handles GET /v1/properties.
// Returns all non-archived properties for the authenticated owner with summary stats.
//
//	@Summary		List owner's properties
//	@Description	Returns all properties owned by the authenticated user, including room stats.
//	@Tags			properties
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/properties [get]
//	@Security		BearerAuth
func (h *PropertyHandler) ListProperties(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	properties, err := h.svc.ListProperties(c.Request.Context(), ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_PROPERTIES_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    properties,
		"meta":    gin.H{"total": len(properties)},
	})
}

// CreateProperty handles POST /v1/properties.
// Validates required fields (name, address, phone), inserts a row, and writes an audit log.
//
//	@Summary		Create a property
//	@Description	Creates a new property for the authenticated owner.
//	@Tags			properties
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CreatePropertyRequest	true	"Property data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/properties [post]
//	@Security		BearerAuth
func (h *PropertyHandler) CreateProperty(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.CreatePropertyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	prop, err := h.svc.CreateProperty(c.Request.Context(), ownerID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("CREATE_PROPERTY_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(prop))
}

// GetProperty handles GET /v1/properties/:id.
// Returns property detail with stats; enforces owner ownership.
//
//	@Summary		Get property detail
//	@Description	Returns a single property with room statistics. Only accessible by the owning user.
//	@Tags			properties
//	@Produce		json
//	@Param			id	path		string	true	"Property UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/properties/{id} [get]
//	@Security		BearerAuth
func (h *PropertyHandler) GetProperty(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	prop, err := h.svc.GetProperty(c.Request.Context(), ownerID, propertyID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_PROPERTY_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(prop))
}

// UpdateProperty handles PUT /v1/properties/:id.
// Updates property fields and writes an audit log entry.
//
//	@Summary		Update a property
//	@Description	Updates a property's details. Only accessible by the owning user.
//	@Tags			properties
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Property UUID"
//	@Param			body	body		dto.UpdatePropertyRequest	true	"Updated property data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/properties/{id} [put]
//	@Security		BearerAuth
func (h *PropertyHandler) UpdateProperty(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	var req dto.UpdatePropertyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	prop, err := h.svc.UpdateProperty(c.Request.Context(), ownerID, propertyID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_PROPERTY_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(prop))
}

// DeleteProperty handles DELETE /v1/properties/:id.
// Soft-archives the property and cascade-archives rooms/tenants.
// Requires explicit `confirm=true` query parameter.
//
//	@Summary		Archive a property
//	@Description	Soft-archives a property and all associated rooms and tenants. Requires confirm=true query param.
//	@Tags			properties
//	@Produce		json
//	@Param			id		path		string	true	"Property UUID"
//	@Param			confirm	query		string	true	"Must be 'true' to confirm archival"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/properties/{id} [delete]
//	@Security		BearerAuth
func (h *PropertyHandler) DeleteProperty(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property ID"))
		return
	}

	// Require explicit confirmation.
	if c.Query("confirm") != "true" {
		c.JSON(http.StatusBadRequest, errorResponse(
			"CONFIRMATION_REQUIRED",
			"Deleting a property will archive all associated rooms, tenants, and data. Add ?confirm=true to proceed.",
		))
		return
	}

	if err := h.svc.ArchiveProperty(c.Request.Context(), ownerID, propertyID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("ARCHIVE_PROPERTY_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"message": "Property archived successfully"},
	})
}

// ownerIDFromContext extracts and parses the owner UUID from the Gin context.
// It writes an error response and returns false if the ID is missing or invalid.
func ownerIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	idStr := c.GetString(middleware.ContextKeyUserID)
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return uuid.UUID{}, false
	}
	return id, true
}
